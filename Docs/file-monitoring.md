# 檔案變更監聽與 Reconciliation（設計草案）

> 狀態：reconciliation diff 與第一版 OS watcher 已實作（新檔事件只觸發掃描提示；掃描仍是權威）；file ID rename、持久化 file catalog 與 per-pool watchMode 尚未實作。

## 目的

讓新檔更快被發現、降低大型目錄的重複 traversal，同時保留一個可恢復的權威掃描層，避免 watcher 漏事件或容器 bind mount 不支援時失去一致性。

非目標：本文不自動刪除影片索引、不自動覆寫既有 hash，也不定義 Central 的 catalog 格式（見 [peer protocol](peer-protocol.md)）。

## 現況

- `scanIntervalSeconds`（預設 30 秒）到點就遞迴走所有啟用池，使用 `WalkDir`＋`Stat`，**不讀影片內容**。
- 以 `videos`＋`import_jobs` 建 `indexed` 集合；已知路徑跳過，所以只排入新檔。
- 新檔需在記憶體中的 `observed` 連續觀察相同 size/mtime，且至少經過 `fileStableSeconds`（預設 10 秒），才排入匯入。
- 週期掃描現在會比對已索引影片的 size/mtime，統計 `changed` 與 `deleted`；兩者都**不會**改 hash、刪索引或自動重新匯入。實際來源狀態仍由 `videos` 讀取時的 `openVideo` 判定。
- 不處理 rename；沒有持久化的 file identity；重啟後重新觀察。
- 第一版 watcher 使用 `fsnotify`，只當低延遲提示：事件經 debounce 後呼叫掃描；不直接修改 catalog、不直接匯入。整碟池與 9p 檔案系統直接降級輪詢；其他監聽失敗也會保留 polling。
- 掃描與自動補齊共用單一 FIFO 佇列；真正大量讀取是匯入／重新探測的 FFprobe＋SHA-256，受 `hashLimitMiBps` 限制。

## 目標架構

```
pool lifecycle
   │
   ├── Watcher（提示層）──┐
   │  create/write/rename  │
   │  remove               ▼
   │                 bounded event queue
   │                  debounce/stable
   │                         │
   └── Reconciliation（權威層）── 日誌/狀態
          periodic full walk
          compare catalog
                         │
                         ▼
                  ImportQueue / reprobe
```

1. **Watcher**：低延遲提示，不直接讀內容、不直接改 DB。
2. **Reconciliation**：固定週期完整 metadata 掃描，補漏、處理重啟與 overflow，是正確性的來源。
3. **Catalog**：保存已見檔案與最後狀態，讓 reconciliation 可以做 new/changed/deleted/renamed diff。
4. **ImportQueue**：沿用現有穩定佇列、背壓與 hash 限速。

Watcher 可以關閉或退回 polling；Reconciliation 永遠保留。

## 檔案身分與 Catalog

目前 `videos` 已有 `pool_id`、`relative_path`、`size`、`modified_at`、`file_hash`、`source_hash`，足以做基本 diff；建議補齊：

| 欄位 | 用途 |
| --- | --- |
| `file_id` | Windows file ID／Linux inode+device；用於 rename 與同一檔案判斷 |
| `volume_id` | 已在 pool 記錄的 Windows volume GUID；容器內可能為空 |
| `last_seen_revision` | 最後一次 reconciliation 看到的代次 |
| `state` | `indexed`／`pending`／`changed`／`missing`；不要用刪除表示離線 |

可選擇新增 `pool_files` 觀察表，分開「看過的檔案」與「已索引影片」：

- `pool_files(pool_id, relative_path, size, mtime, file_id, volume_id, last_seen, state)`。
- 好處：新檔尚未匯入、匯入失敗或僅觀察中的檔案也有持久狀態；rename 偵測不必依賴 `videos`。
- 代價：多一份 catalog 與一致性維護。第一版可先擴充 `videos` 與 `import_jobs`，第二版再抽表。

### 邊界

- 不跟隨符號連結；reconciliation 與 watcher 都要拒絕 pool 外的 canonical path。
- Windows 大小寫不敏感、Linux 敏感；rename 與去重需用平台規則。
- Unicode 正規化差異可能造成同一路徑不同字串；以平台檔案 ID 優先。
- `file_id` 在 copy/replace 後可能改變；不能只用 file ID 判定內容相同。
- 同一檔案 hard link／多路徑：目前模型以路徑為 unique，需明確選擇「多筆索引」或「一筆＋別名」。

## 事件管線

1. Watcher 產生粗事件：`create`、`write`、`rename`、`remove`。
2. 以 `pool_id + canonical relative path` 合併重複事件；`.part`、暫存副檔名、隱藏檔可忽略。
3. 有界佇列；超過上限或 watcher overflow 時：
   - 丟棄個別事件，設定 `needsReconcile`；
   - 盡快排一次 reconciliation，不阻塞 watcher。
4. Debounce 後，仍要求 size/mtime 在 `fileStableSeconds` 內一致，才交給 ImportQueue。
5. Watcher／reconciliation 都不在事件迴圈做 FFprobe 或 SHA-256。

### Rename

- `rename` 事件可能只有 old/new 其中一邊，或分成 remove+create；用 file ID 配對。
- 同一 pool 內且 file ID 相符：更新 `relative_path`，保留 videoId、描述、Tag、hash。
- 無法配對或跨 pool：先標舊路徑 missing，新路徑視為新檔；不要自動搬移描述。
- 跨 pool 的安全搬移／replica 複製屬後續資料模型，不在 watcher 自動處理。

## 變更語意

| 類型 | 行為 |
| --- | --- |
| 新檔 | 穩定後排入匯入；沿用目前 `autoScan` 的排除規則 |
| 既有檔變更 | 先標 `source_changed`、停用播放；**不自動重算 hash、不自動覆寫**。之後由管理員選擇重新探測、建立新 revision 或重新匯入 |
| 刪除 | 標 `source_missing`／離線，保留索引與描述；不自動刪 DB |
| Rename | 同 pool 且 file ID 相符時更新路徑；否則 delete+new |
| 停用池 | 停止 watcher 與 reconciliation；不產生事件 |
| Volume relink | 用實際解析路徑重註冊 watcher；舊 watch 必須移除 |
| 匯入中變更 | 沿用現有 `openVideo`／probe 前後 size、mtime、hash 檢查，工作失敗且不提交 |

現有模型是「一個原始檔＋固定 hash」；若未來要支援同一路徑內容更新，需先定義 revision/replica，不應由 watcher 直接修改 `file_hash`。

## 平台與部署

### Windows 原生

- `ReadDirectoryChangesW`，遞迴監聽；volume GUID 路徑可作為穩定根，但需確認 Go 與 watcher 對 `\\?\Volume{...}\` 的行為。
- 網路磁碟／SMB 事件不保證；失敗時退回 polling。
- 檔案 ID 可由 `GetFileInformationByHandle` 取得。

### Linux 原生

- inotify；每個子目錄 watch；注意 `max_user_watches`、`IN_Q_OVERFLOW`、rename cookie。
- 網路檔案系統（NFS/SMB/FUSE）事件可能缺失；reconciliation 必留。

### Docker

- 容器內 `/media` 常是 9p／virtiofs／overlay bind mount；inotify 事件可能不完整或完全沒有。
- 以「watcher 可用才啟用，否則 polling」為原則；不要在容器內假設主機事件會轉發。
- 主機盤符改變時，更新 Compose source 並保持相同 target；池的容器路徑不變。若 target 改名，重新註冊池或依 volume GUID（原生 Windows）處理。

## 設定與狀態

建議新增／整理：

| 設定 | 預設 | 說明 |
| --- | --- | --- |
| `watchMode` | `auto` | `auto`／`on`／`off`；`auto` 不支援時退回 polling |
| `reconcileIntervalSeconds` | 300–1800 | 權威掃描間隔；比目前 30 秒長 |
| `fileStableSeconds` | 10 | 新檔／變更 debounce，不是掃描頻率 |
| `watchDebounceMs` | 500–2000 | 合併事件 |
| `maxPendingEvents` | 1000 | 超過即排 reconciliation |

狀態 API／UI：

- `watchMode`、`active`、`fallbackReason`。
- 已監聽根目錄數、事件數、佇列長度、overflow 次數、最後事件時間。
- `nextReconcileAt`／倒數、上次 reconciliation 的 new/changed/deleted/renamed 統計。
- 錯誤（權限、watch limit、網路磁碟）與目前是否只剩 polling。

## 失敗與邊界

| 情境 | 行為 |
| --- | --- |
| Watcher 無法啟動 | `auto` 退回 polling；UI 顯示 fallback，不影響匯入 |
| 事件 overflow | 丟棄明細、排 reconciliation；不可只依 watcher |
| 程序重啟 | 先 reconciliation，再掛 watcher；避免啟動期間漏檔 |
| 池停用／移除 | 停止 watch、取消排程；索引保留 |
| Volume 重新掛載 | 以新解析路徑重建 watch；舊 watch 失效 |
| 檔案持續寫入 | stable/debounce 重設計時；必要時 `.part` |
| 檔案被快速 rename 多次 | 以 file ID＋最後狀態收斂；必要時 reconciliation |
| 檔案權限不足 | 記錄錯誤、跳過該路徑；不讓整個掃描失敗 |
| 大量小檔 | 合併事件、限制佇列；必要時只做 reconciliation |

## 安全

- 只監聽已註冊 pool 的 root；事件路徑必須 canonical 且在 root 內。
- 不跟隨符號連結；rename／remove 也要重新驗證邊界。
- Watcher 以服務身分執行；Docker 不因 watcher 需求而開放 privileged。
- 事件內容不可信；不直接執行路徑、不跳過 hash／穩定性檢查。
- 狀態 API 需管理員登入；不要把原始檔名清單暴露給未授權端點。

## 測試與落地順序

1. **Reconciliation diff（第一階段完成）**：週期掃描統計 new/changed/deleted；changed/deleted 只標狀態，不自動改資料。
2. **Reconciliation 強化**：抽出持久化 catalog 與 file identity，加入 rename 偵測。
3. **Watcher 新檔**：Windows/Linux 各一實作；事件只加速新檔；reconciliation 仍是權威；Docker 退回 polling。
4. **Rename/file ID**：同 pool rename 保留 videoId；跨 pool 先不自動。
5. **變更政策**：決定 revision/replica 或手動重新匯入；不要讓 watcher 自動覆寫。
6. **Central catalog**：把穩定變更整理成 revision/cursor，接上 peer protocol 的 `catalog.sync.v1`。

測試至少涵蓋：事件合併與 debounce、overflow 觸發 reconciliation、watcher 不可用 fallback、停用池不處理、volume relink、rename 配對、modified/deleted 只標狀態、重啟後補漏、Docker bind mount。

## 待決策

- Catalog 用獨立 `pool_files` 或先擴充 `videos`／`import_jobs`。
- `watchMode` 預設 `auto` 或先 `off` 讓使用者 opt-in。
- 既有檔內容變更要走「手動重新匯入」、「新 revision」還是「新的 replica」。
- 跨 pool rename／搬移是否保留 videoId 與描述。
- Docker 內是否值得啟用 watcher，或直接固定 polling。
