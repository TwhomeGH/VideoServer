# ChangeHistory

> 格式：問題／修改／驗證各自成段；多項內容使用清單，避免單行長段落。行長上限 120 字元，超過時優先拆成多個清單項目。每個變更項目重複使用這三個標題是刻意的，MD024 以 `siblings_only` 允許不同變更項目下的同名標題。

## 2026.10.28 03:45 Index-only 全盤查找與找到的影片

**類型**: 功能

**檔案**:

- `internal/library/discovery.go`
- `internal/library/discovery_test.go`
- `internal/library/store.go`
- `internal/library/scanner.go`
- `internal/library/http.go`
- `web/admin/src/Discovered.tsx`
- `web/admin/src/Library.tsx`
- `web/admin/src/navigation.ts`
- `web/admin/src/style.css`
- `web/admin/src/lang/`
- `pkg/protocol/openapi.json`

### 問題

- 全盤掃描仍會自動 hash／匯入所有找到的影片，finder 成本太高
- 開關介面只有系統目錄排除，沒有「只索引不匯入」模式

### 修改

- 每池新增 `indexOnly`；掃描只寫入 `discovered_media`（schema v9），不建立 import job。
- 新增「找到的影片」頁，支援搜尋、分頁、選取與批量／單筆手動匯入。
- `GET /api/v1/discovered` 與 `POST /api/v1/discovered/import`；匯入成功後項目從清單消失。
- 關閉 index-only 後，既有發現項目仍保留手動匯入，不一次自動 hash；新檔才恢復自動匯入。
- 掃描完成會把未再看到的 discovery 標記 missing；發現列同時隱藏已在影片庫的路徑。
- 格式與安全掃描政策沿用既有設定。

### 驗證

- 測試站建立 index-only 檔案後，掃描出現於「找到的影片」，且沒有建立 import job。
- 手動掃描與頁面搜尋、選取、匯入按鈕正常；390px 無水平溢出。
- 移除檔案後掃描會標記 missing，頁面回到空狀態；pool 已恢復 index-only=false。
- Go 測試、前端建置、`lang:check` 與 `check:testing` 通過。

---

## 2026.10.28 03:15 能力描述加入 serviceType 與 contentTypes

**類型**: 改善

**檔案**:

- `pkg/protocol/protocol.go`
- `pkg/protocol/http.go`
- `pkg/protocol/protocol_test.go`
- `pkg/protocol/openapi.json`
- `Docs/api/README.md`
- `Docs/peer-protocol.md`

### 問題

- `/api/v1/protocol` 只有服務名稱與能力，沒有明確的服務類型與內容類型
- 未來 Image／Post 節點需要一個穩定的協商入口

### 修改

- 新增 `serviceType`，目前為 video-node／central／transcode-worker。
- 新增 `contentTypes`，Video Node 公告 `["video"]`；Central／Worker 為空。
- OpenAPI、協定文件與內容節點設計同步。
- 尚未加入 peer register／heartbeat／registry；Central 目前不會自動發現節點，需先以靜態 endpoint 查詢 `/api/v1/protocol`。

### 驗證

- 協定測試驗證 Video Node 的 serviceType 與 contentTypes。
- Go 測試、OpenAPI 契約、前端建置與 `check:testing` 通過。

---

## 2026.10.28 03:00 儲存池安全掃描與影片格式能力

**類型**: 功能

**檔案**:

- `internal/media/formats.go`
- `internal/media/formats_test.go`
- `internal/library/store.go`
- `internal/library/scanner.go`
- `internal/library/jobs.go`
- `internal/library/videos.go`
- `internal/library/http.go`
- `internal/library/scan_policy_test.go`
- `internal/library/entry_windows.go`
- `internal/library/entry_other.go`
- `Docs/content-nodes.md`
- `internal/runtime/config.go`
- `internal/runtime/server.go`
- `web/admin/src/Library.tsx`
- `web/admin/src/lang/`
- `pkg/protocol/openapi.json`

### 問題

- 全盤池沒有系統目錄排除，`C:\` 會掃到 Windows、AppData、快取與程式目錄
- 影片格式清單寫死，未參考 FFprobe demuxer；`.ts`／`.mts` 也與 TypeScript 衝突

### 修改

- 新增每池 `scanExcludeSystem`，預設開啟；安全排除 Windows、Program Files、ProgramData、AppData、Recycle Bin、隱藏目錄、`node_modules`、`.git` 等。
- 儲存池 API 回傳 `scanExcludeSystem`、`wholeDisk`；PATCH 可切換，關閉前 UI 二次確認；磁碟根目錄顯示全盤警告。
- 掃描時排除目錄直接 `SkipDir`，不只過濾檔案。
- 新增影片格式 allowlist：啟動時 `ffprobe -formats` 驗證 demuxer；支援 `extraVideoExtensions` 基底設定。
- 預設加入常見容器；`.ts`／`.mts` 預設關閉，額外開啟時做 188／192-byte MPEG-TS sync 檢查，避免 TypeScript。
- 手動匯入與掃描共用同一格式能力；FFprobe 仍是最終驗證。

### 驗證

- 單位測試驗證系統路徑排除、掃描政策切換、TS／MTS 嗅探與 TypeScript 拒絕。
- 測試站顯示預設排除、`wholeDisk=false`；PATCH 可切換並恢復。
- Go 測試、前端建置、`lang:check` 與 `check:testing` 通過。

---

## 2026.10.28 01:30 SSE 工作／掃描／設定事件與快照同步

**類型**: 功能

**檔案**:

- `internal/events/hub.go`
- `internal/events/http.go`
- `internal/events/hub_test.go`
- `internal/library/store.go`
- `internal/library/jobs.go`
- `internal/library/hash.go`
- `internal/library/reprobe.go`
- `internal/library/scanner.go`
- `internal/runtime/configapi.go`
- `internal/runtime/server.go`
- `web/admin/src/live.tsx`
- `web/admin/src/main.tsx`
- `web/admin/src/ImportJobs.tsx`
- `web/admin/src/Monitor.tsx`
- `web/admin/src/Library.tsx`
- `web/admin/src/Settings.tsx`
- `web/admin/src/lang/`
- `pkg/protocol/openapi.json`

### 問題

- 工作、掃描與設定變更只能靠各頁輪詢，延遲與請求量固定
- 多分頁設定變更沒有提示，可能覆蓋未儲存的編輯

### 修改

- 新增受 session 保護的 `GET /api/v1/events`；事件類型為 jobs／scan／config，另有 ready／reset。
- 事件只帶變更通知，不傳完整快照；首次連線、reset 或重播後由前端呼叫既有 API 取得快照。
- 有界 512 事件環、`Last-Event-ID` 重播、重播落差 reset、15 秒 heartbeat、最長 30 分鐘後重連。
- 慢速訂閱者溢出時送出 reset 並結束，由 EventSource 重連。
- 工作進度事件最多每秒一次；掃描開始／完成、設定儲存／重設／重啟都會發布。
- 前端新增 `LiveProvider`，工作、掃描、影片庫與設定頁用版本號觸發快照；SSE 不可用時保留輪詢降級。
- 設定頁在未儲存編輯時收到其他分頁變更，顯示提示與重新載入按鈕；未編輯時自動同步。
- OpenAPI、API 文件、監看與匯入文件同步。

### 驗證

- 實際收到 ready、scan、config、jobs 事件與 `Last-Event-ID` 重播。
- 單一掃描產生開始／完成兩個事件；工作探測產生多個進度事件。
- 設定頁在 dirty 狀態收到 config 事件會顯示提示，重新載入後清除。
- 事件串流經 diagnostics wrapper 正常 flush；401 未登入不會建立串流。
- Go 測試、前端建置、`lang:check` 與 `check:testing` 通過。

---

## 2026.10.27 23:59 掃描資訊加入顏色層級

**類型**: 改善

**檔案**:

- `web/admin/src/ScanTierBadge.tsx`
- `web/admin/src/Monitor.tsx`
- `web/admin/src/Library.tsx`
- `web/admin/src/style.css`
- `web/admin/src/lang/`

### 問題

- 掃描列的名稱、下次掃描、間隔與活動紀錄都是相近的亮色，視線沒有層級
- 圖例的分級名稱沒有沿用 badge 顏色

### 修改

- 名稱保持主要文字色；下次掃描維持主要色，間隔改為次要灰。
- 上次活動與預估轉級維持次要灰，不與主要數據搶眼。
- 圖例只有「熱／暖／冷／休眠」上色，門檻文字維持灰色。
- 不改整句上色，避免變成彩虹；分級顏色仍集中在 badge 與圖例。

### 驗證

- 圖例顏色：熱綠、暖琥珀、冷藍、休眠灰。
- 下次掃描為主要色，間隔與活動為次要灰。
- 390px 無水平溢出；前端建置、`lang:check`、Go 測試與 `check:testing` 通過。

---

## 2026.10.27 23:55 掃描列改為三段式

**類型**: 改善

**檔案**:

- `web/admin/src/Monitor.tsx`
- `web/admin/src/Library.tsx`
- `web/admin/src/style.css`
- `web/admin/src/lang/`

### 問題

- 池名、分級、下次掃描與間隔擠在同一行，窄寬度時壓迫
- 圖例是單一長字串，容易在「休眠」中間斷行

### 修改

- 每池改成三段：名稱＋分級 badge、下次掃描／間隔、上次活動與降級提示。
- 不再依賴視窗寬度才換行，監看側欄固定用單欄堆疊。
- 圖例拆成四個獨立短項目，使用 flex 自動換行，不再從詞中間斷開。
- 390px 與側欄寬度皆無水平溢出。

### 驗證

- 側欄 429px 寬度下，三行各自完整；390px 也維持三段。
- 圖例為熱／暖／冷／休眠四個短項目。
- 前端建置、`lang:check`、Go 測試與 `check:testing` 通過。

---

## 2026.10.27 23:45 暖級加入 15 分中間階

**類型**: 改善

**檔案**:

- `internal/library/scanner.go`
- `internal/library/scheduler_test.go`
- `web/admin/src/ScanTierBadge.tsx`
- `web/admin/src/lang/`
- `Docs/configuration.md`

### 問題

- 暖級固定 5 分，24 小時後直接跳到冷級 30 分，間隔落差太大
- 使用者看不到 6 小時後會改成 15 分

### 修改

- 暖級拆成前後段：1–6 小時 5 分、6–24 小時 15 分；24 小時後才進冷級 30 分。
- 分級 badge 仍只有暖，不必多一種顏色；只改間隔。
- 監看列的預估提示會在 6 小時前顯示「約 … 間隔改 15 分」。
- 圖例與設定說明同步更新。

### 驗證

- 單元測試驗證暖前段 5 分、暖後段 15 分、冷 30 分與休眠上限。
- 測試站顯示暖池「間隔 15 分」，年輕暖池顯示「約 … 間隔改 15 分」。
- Go 測試、前端建置、`lang:check` 與 `check:testing` 通過。

---

## 2026.10.27 23:30 顯示掃描分級的上次活動與降級時間

**類型**: 改善

**檔案**:

- `web/admin/src/ScanTierBadge.tsx`
- `web/admin/src/Monitor.tsx`
- `web/admin/src/Library.tsx`
- `web/admin/src/style.css`
- `web/admin/src/lang/`

### 問題

- 看得到「熱」但不知道熱從何時開始、何時會轉暖
- 分級門檻只在文件與圖例，實際每池狀態沒有時間提示

### 修改

- 監看頁每池新增「上次活動 … · 約 … 轉暖／轉冷／轉休眠」。
- badge tooltip 同步顯示相同資訊；休眠顯示不再自動降級。
- 明確標註實際轉換在下次掃描時判定；熱池最多晚 30 秒、暖池最多 5 分、冷池最多 30 分。
- 儲存池頁 badge 也提供 tooltip。

### 驗證

- 測試站顯示「上次活動 10/27 下午10:15 · 約 10/27 下午11:15 轉暖」。
- 暖池顯示下次轉冷時間；390px 無水平溢出。
- 前端建置、`lang:check`、Go 測試與 `check:testing` 通過。

---

## 2026.10.27 23:15 掃描分級改為獨立有色標籤

**類型**: 改善

**檔案**:

- `web/admin/src/ScanTierBadge.tsx`
- `web/admin/src/Library.tsx`
- `web/admin/src/Monitor.tsx`
- `web/admin/src/style.css`
- `web/admin/src/lang/`

### 問題

- 分級文字和下次掃描時間混在同一行，不容易快速掃視
- 熱／暖／冷／休眠沒有顏色區分

### 修改

- 新增獨立的 `ScanTierBadge`，從下次掃描文字中切開。
- 熱綠、暖琥珀、冷藍、休眠灰；以小圓點與外框呈現，不和上線狀態 pill 混用。
- 儲存池卡片與監看頁都使用同一個 badge。
- 加上分級圖例：熱 1 小時內、暖 1–24 小時、冷 1–7 天、休眠超過 7 天。
- 390px 仍為單欄且無水平溢出。

### 驗證

- 測試站顯示「熱」綠色、「暖」琥珀色；下次掃描與間隔分開閱讀。
- 監看頁每池列同步顯示 badge。
- 前端建置、`lang:check`、Go 測試與 `check:testing` 通過。

---

## 2026.10.27 23:00 時間欄位改為單位選擇與人類可讀顯示

**類型**: 改善

**檔案**:

- `web/admin/src/format.ts`
- `web/admin/src/DurationField.tsx`
- `web/admin/src/Settings.tsx`
- `web/admin/src/Library.tsx`
- `web/admin/src/Monitor.tsx`
- `web/admin/src/ImportJobs.tsx`
- `web/admin/src/style.css`
- `web/admin/src/lang/`

### 問題

- 設定欄位與監看頁大量直接顯示秒數，21600 秒不易理解
- 使用者需要在腦中換算秒、分、小時

### 修改

- 新增 `formatDuration`，顯示如 30 秒、5 分、6 小時、1 天 2 小時。
- 設定頁的掃描間隔、休眠間隔、檔案穩定、補齊間隔改成「數值＋秒／分／小時／天」單位選擇器，儲存仍送整數秒。
- 欄位下方顯示換算結果；監看、儲存池、影片工作同步使用人類可讀時間。
- 後端 API 與設定檔仍以秒為單位，保持相容。

### 驗證

- 設定頁顯示 30 秒、6 小時、10 秒、1 分，單位切換正常。
- 儲存池顯示「暖，間隔 5 分」；監看顯示「暖 · 下次約 4 分 44 秒後 · 間隔 5 分」。
- 390px 無水平溢出；前端建置、`lang:check`、Go 測試與 `check:testing` 通過。

---

## 2026.10.27 22:45 掃描改為熱／暖／冷／休眠分級

**類型**: 改善

**檔案**:

- `internal/library/scanner.go`
- `internal/library/store.go`
- `internal/library/scheduler_test.go`
- `internal/runtime/config.go`
- `internal/runtime/configapi.go`
- `internal/runtime/server.go`
- `web/admin/src/Library.tsx`
- `web/admin/src/Monitor.tsx`
- `web/admin/src/Settings.tsx`
- `web/admin/src/lang/`
- `pkg/protocol/openapi.json`

### 問題

- 單一 300 秒上限對長期沒有變化的冷池仍過於頻繁
- 只有退避秒數，沒有分級標記，難以判斷掃描策略

### 修改

- 每池依 `last_activity_at` 分成熱（1 小時內，基礎間隔）、暖（1–24 小時，5 分）、冷（1–7 天，30 分）、休眠（超過 7 天，`scanIdleMaxSeconds`）。
- `scanIdleMaxSeconds` 預設改為 21600 秒（6 小時），可調到 86400 秒（24 小時）。
- 新檔／變更／待穩定、手動掃描或未來 watcher 事件都會回到熱級。
- 新增 schema v7 `pools.last_activity_at`，重啟後保留分級依據。
- API／監看／儲存池頁顯示 tier、上次活動、下次掃描與目前間隔。
- 設定頁名稱改為「熱池掃描間隔」與「休眠掃描間隔」。

### 驗證

- 單元測試驗證熱／暖／冷／休眠間隔與手動掃描回到熱級。
- 測試站驗證分級標記、休眠 6 小時與 24 小時可調。
- Go 測試、前端建置、`lang:check` 與 `check:testing` 通過。

---

## 2026.10.27 22:30 影片工作改為緊湊清單與分頁

**類型**: 改善

**檔案**:

- `web/admin/src/ImportJobs.tsx`
- `web/admin/src/style.css`
- `web/admin/src/lang/`

### 問題

- 每筆工作是大型卡片，16px padding、完整路徑與動作都直接展開
- 100 筆時要縱向捲動很長，難以快速找到執行中或失敗工作

### 修改

- 改成緊湊表格列：影片／狀態／進度／嘗試／操作；桌面單列約 59px。
- 每頁 20 筆，最多 100 筆分成 5 頁。
- 加入全部／處理中／失敗／完成篩選與數量。
- 路徑、錯誤與更新時間收在「詳細」展開，不再佔用每列高度。
- 820px 以下改為卡片式堆疊，390px 無水平溢出。

### 驗證

- 100 筆工作顯示 20 列、5 頁；第一列高度 59px，桌面無水平溢出。
- 失敗篩選可正確顯示無符合工作；詳細展開顯示路徑與更新時間。
- 390px 單欄沒有水平溢出；Go 測試、前端建置、`lang:check` 與 `check:testing` 通過。

---

## 2026.10.27 22:15 區分設定覆寫與補齊用量重設

**類型**: 改善

**檔案**:

- `web/admin/src/Settings.tsx`
- `web/admin/src/Monitor.tsx`
- `web/admin/src/style.css`
- `web/admin/src/lang/`
- `Docs/configuration.md`

### 問題

- 設定頁「重設為基準」與監看頁「重設今日用量」都可能被理解成重置補齊額度
- 「回到主機 JSON」沒有指出是哪個檔案，容易誤按並清除全部設定覆寫

### 修改

- 設定頁改為「設定覆寫（進階）」危險區塊，按鈕改名「清除設定覆寫」，不再與儲存並排。
- 確認文字直接顯示要刪除的覆寫檔與要回到的基準檔路徑。
- 明確寫出「不影響自動補齊今日用量、影片或儲存池」。
- 監看頁按鈕改名「重設自動補齊今日用量」，確認文字也限定在補齊額度。
- 文件說明測試環境基準檔是 `testing/local/config/video-node.json`，容器內為 `/app/config.json`；覆寫檔是 `/data/config.override.json`。

### 驗證

- 前端建置、`lang:check` 與 `check:testing` 通過。
- 確認兩個按鈕不再同名，且設定頁不再顯示模糊的「主機 JSON」字樣。

---

## 2026.10.27 22:00 自動補齊重置與服務時區

**類型**: 功能

**檔案**:

- `internal/library/reprobe.go`
- `internal/library/http.go`
- `internal/library/backfill_test.go`
- `internal/runtime/server.go`
- `web/admin/src/Monitor.tsx`
- `web/admin/src/lang/`
- `compose.yaml`
- `testing/compose.yaml`
- `pkg/protocol/openapi.json`

### 問題

- 達到每日上限後只能等隔日，沒有手動重設入口
- Docker 預設時區可能是 UTC，日界與本地時間不一致

### 修改

- 新增 `POST /api/v1/backfill/usage/reset`，要求 `confirm: true`，只清除今天的 `backfill_usage`。
- 監看頁在上限用完時顯示「重設今日用量」，按下後再顯示確認面板，二次確認才送出。
- Compose 的 video-node／central／worker 預設 `TZ=Asia/Taipei`，可用環境變數覆寫。
- Go 二進位內嵌 `time/tzdata`，不依賴 Alpine 映像的 zoneinfo。
- 重設只影響每日用量，不刪工作、不重設候選跳過狀態。

### 驗證

- 單元測試驗證用量重設與 `limitReached` 回到 false。
- 測試站驗證 `dailyResetAt` 為台北日界、設定與監看顯示正確。
- Go 測試、前端建置、`lang:check` 與 `check:testing` 通過。

---

## 2026.10.27 21:40 自動補齊每日部數與容量上限

**類型**: 功能

**檔案**:

- `internal/library/reprobe.go`
- `internal/library/store.go`
- `internal/library/backfill_test.go`
- `internal/runtime/config.go`
- `internal/runtime/configapi.go`
- `internal/runtime/server.go`
- `web/admin/src/Settings.tsx`
- `web/admin/src/Monitor.tsx`
- `web/admin/src/lang/`
- `pkg/protocol/openapi.json`

### 問題

- 自動補齊每 60 秒可排一筆，但沒有每日總量上限
- 影片大小差異大，只靠間隔與瞬間速率無法限制一天的總讀取量

### 修改

- 新增 `backfillDailyLimit`（預設 50 部）與 `backfillDailyGiB`（預設 20 GiB），取先到者，0 為不限。
- 只限制自動補齊；手動重新讀取不受影響。
- 新增 schema v6 `backfill_usage(day,videos,bytes)`，重啟後保留；以服務本地日重置。
- 達到上限時停止自動排程，候選保留，監看頁顯示今日用量與重置時間。
- 設定頁放入「自動補齊與 I/O 限速」群組；OpenAPI、i18n 與測試同步。
- 文件明確說明「佇列閒置」是整個 import 佇列沒有 queued／running 工作，不是只看補齊。

### 驗證

- 單元測試驗證每日用量與上限判定。
- 監看頁顯示今日部數／容量、候選目標與達到上限的重置時間。
- Go 測試、前端建置、`lang:check` 與 `check:testing` 通過。

---

## 2026.10.27 21:20 自動補齊顯示下一個候選

**類型**: 功能

**檔案**:

- `internal/library/reprobe.go`
- `internal/library/backfill_test.go`
- `pkg/protocol/openapi.json`
- `web/admin/src/Monitor.tsx`
- `web/admin/src/lang/`

### 問題

- 自動補齊只顯示待檢查候選數，看不到下一個可能排程的影片
- 使用者無法判斷目前要處理哪一部

### 修改

- `BackfillStatus` 新增 `nextVideoId`、`nextVideoTitle`。
- 監看頁顯示「下一個待檢查候選：標題」；沒有候選時顯示明確訊息。
- 候選查詢與數量使用相同條件；仍只提供提示，實際排程前會再做來源檢查。
- OpenAPI、i18n 與測試同步。

### 驗證

- 實測監看頁顯示「下一個待檢查候選：RPReplay_Final1654974526.mp4」。
- 單元測試驗證候選 ID 與標題。
- Go 測試、前端建置、`lang:check` 與 `check:testing` 通過。

---

## 2026.10.27 21:00 工作監看重排與每池掃描明細

**類型**: 功能

**檔案**:

- `web/admin/src/Monitor.tsx`
- `web/admin/src/style.css`
- `web/admin/src/lang/`
- `internal/library/scanner.go`
- `pkg/protocol/openapi.json`

### 問題

- 監看頁的工作、掃描與補齊全部縱向堆疊，寬螢幕沒有利用空間
- 掃描區只有全域摘要，看不到各池的下次掃描與退避
- 儲存池排程缺少名稱，只能靠 pool ID 對照

### 修改

- 寬螢幕改成兩欄：左側影片工作，右側掃描與補齊；四個效能指標保持並排。
- 900px 以下改單欄，維持 390px 無水平溢出。
- 掃描區列出每池名稱、下次掃描與退避；顯示閒置上限。
- `PoolScanStatus` 加入 `name`，掃描器在排程時保存池名稱。
- OpenAPI、i18n 與監看頁文案同步。

### 驗證

- 1920 寬度為 648px／479px 兩欄，工作與掃描卡片對齊；390 寬度為單欄。
- 兩池顯示「下次約 N 秒後 · 退避 60 秒」，名稱正確。
- Go 測試、前端建置、`lang:check` 與 `check:testing` 通過。

---

## 2026.10.27 20:40 每池掃描退避、單池掃描與設定分組

**類型**: 功能

**檔案**:

- `internal/library/scanner.go`
- `internal/library/http.go`
- `internal/runtime/config.go`
- `internal/runtime/configapi.go`
- `internal/runtime/server.go`
- `web/admin/src/Library.tsx`
- `web/admin/src/Settings.tsx`
- `web/admin/src/style.css`
- `web/admin/src/lang/`
- `pkg/protocol/openapi.json`

### 問題

- 固定 30 秒掃描所有池，長期沒有變化的目錄仍持續 traversal
- 缺少只掃單一池的入口
- 設定頁欄位沒有分組

### 修改

- 每池 adaptive backoff、單池掃描 API/按鈕與自動掃描／自動補齊分組
- 每個池維護 `nextScanAt` 與 backoff；連續沒有變化時 ×2，最高 `scanIdleMaxSeconds`（預設 300 秒），有變化或手動掃描重置。
- 掃描器只處理到期池；新增 `RequestPool`、`POST /api/v1/pools/{id}/scan` 與卡片「掃描此池」按鈕。
- `ScanStatus` 增加 `idleMaxSeconds`、每池 `pools`（下次掃描、退避、quiet）；OpenAPI 與前端同步。
- 設定頁用 fieldset 分成「自動掃描與檔案穩定」與「自動補齊與 I/O 限速」，新增閒置退避上限欄位。
- 掃描仍只讀 metadata；檔案內容的 FFprobe／SHA-256 仍受既有佇列與限速控制。

### 驗證

- 新增 adaptive backoff／單池重置測試
- 完整 Go 測試、前端建置與 `lang:check`
- 測試站重建後確認每池倒數、單池掃描與設定分組。

---

## 2026.10.27 20:10 儲存池卡片化、Volume GUID 顯示與手動資訊重查

**類型**: 功能

**檔案**:

- `internal/library/store.go`
- `internal/library/http.go`
- `internal/library/volume_*.go`
- `web/admin/src/Library.tsx`
- `web/admin/src/style.css`
- `web/admin/src/lang/`
- `pkg/protocol/openapi.json`

### 問題

- Docker 池的 volume GUID 空白被誤認為漏抓
- 池資訊只在啟動時補一次
- 在線狀態與中繼資料擠在單行，難以閱讀

### 修改

- 儲存池卡片、狀態 pill、Volume GUID/不適用顯示與手動 refresh API
- `Pool` 新增 `volumeStatus`（tracked／unsupported／unbound）；Docker／Linux 明確顯示不適用，不再靜默隱藏。
- 儲存池改為卡片：狀態 pill、路徑、檔案系統、Volume GUID 分區顯示；GUID 長字串可換行。
- 新增 `POST /api/v1/pools/refresh` 與「重新檢查儲存池資訊」按鈕；重查在線池的 filesystem／volumeId，離線池保留舊值。沒有定期自動重查。
- 文件補上 9p／virtiofs 是容器掛載視角，不是主機磁碟類型。

### 驗證

- Go 測試（含 metadata refresh）、前端建置與 `lang:check`
- 測試站重建後確認 Docker 顯示 9p、Volume GUID 不適用、refresh 正常。

---

## 2026.10.27 13:00 週期掃描加入 changed／deleted reconciliation

**類型**: 功能

**檔案**:

- `internal/library/scanner.go`
- `internal/library/scanner_test.go`
- `pkg/protocol/openapi.json`
- `web/admin/src/Library.tsx`
- `web/admin/src/Monitor.tsx`
- `web/admin/src/lang/`

### 問題

- 掃描只處理新檔，既有影片變更或檔案遺失沒有掃描層級的統計
- 使用者只能逐部開啟後才知道來源不可用

### 修改

- 掃描建立已索引 catalog，比對 size／mtime，回報 changed／deleted，但不改 hash、不刪索引
- `catalog` 讀取 `videos` 與匯入工作路徑；掃描時仍跳過既有索引。
- 已索引檔案 size 或 mtime 不同 → `changed++`；線上池掃描完成後，catalog 中未再看到的路徑 → `deleted++`。
- changed／deleted 只提供統計與來源狀態提示；索引、描述、hash、原始檔都不動。實際播放仍由 `openVideo` 的來源檢查回報。
- `ScanResult` 與工作監看／儲存池頁顯示 changed／deleted；OpenAPI 與測試同步。
- 離線或停用池不參與 deleted 統計；rename 仍視為 deleted＋new，file identity 留待下一階段。

### 驗證

- 新增 changed／deleted reconciliation 測試
- 既有掃描穩定性、離線、手動匯入、volume GUID 測試維持通過。
- 測試站 image `316d1362e1da` 的 `scan.last` 實測包含 changed／deleted，穩定庫為 0／0、skipped 179。
- 儲存池頁顯示統計。

---

## 2026.10.27 12:40 Protocol 版本範圍與能力版本（向後相容）

**類型**: 功能

**檔案**:

- `pkg/protocol/protocol.go`
- `pkg/protocol/http.go`
- `pkg/protocol/openapi.json`
- `pkg/protocol/protocol_test.go`
- `Docs/peer-protocol.md`
- `Docs/api/README.md`

### 問題

- Protocol 只接受與 `Current` 完全相同的版本，能力公告沒有版本資訊，之後的握手與 catalog 協商會被鎖死在單一版本

### 修改

- 加入 min/max、`SupportedVersions`、`Negotiate` 與 `capabilityVersions`，保留既有 capabilities 與 `/api/v1` 行為
- `MinSupported`／`MaxSupported` 目前都是 1；`Compatible` 改為範圍檢查，`Negotiate` 取交集最高版本。
- `/api/v1/protocol` 新增 `minProtocolVersion`、`maxProtocolVersion`、`capabilityVersions`。
- 既有 `capabilities` 名稱與順序不變。
- 協定錯誤回應同步附版本範圍；OpenAPI Protocol schema 更新。
- `capabilityVersions` 讓 client 可先偵測版本，不必等到改成 `name.vN` 才能協商；peer 註冊／heartbeat 仍未實作。

### 驗證

- 新增 protocol 公告與 Negotiate 測試
- 既有 middleware 版本拒絕、OpenAPI 路由與能力測試通過。

---

## 2026.10.27 12:15 新增 peer 握手與檔案監聽設計契約

**類型**: 文件

**檔案**:

- `Docs/peer-protocol.md`
- `Docs/file-monitoring.md`
- `Docs/architecture.md`
- `Docs/onboarding.md`
- `TODO.md`

### 問題

- Central／Worker 尚無註冊與 heartbeat 契約
- 目前自動掃描是完整 polling，沒有檔案變更監聽或 modified/deleted/rename 語意

### 修改

- 先定控制平面與資料平面契約，避免之後被 watcher 或 registry 實作綁死
- Peer 契約：serviceType、nodeId、instanceId、endpointRevision、版本範圍、帶版本能力。
- 握手：register/heartbeat/lease、peer 認證、split brain、catalog cursor 與失敗模式。
- 檔案監聽：以 watcher 作提示、reconciliation 作權威
- catalog/file identity、debounce、overflow、backpressure。
- new/changed/deleted/rename 語意、Docker／Windows／Linux 限制與狀態欄位。
- 明確標示目前尚未實作；先不動 runtime 行為。
- 更新文件地圖、架構連結與 TODO。

### 驗證

- 設計文件交叉檢查現有 protocol、scanner、volume GUID、queue 與 Central/Worker 骨架
- 未變更程式碼。

---

## 2026.10.27 11:30 排程可見度、自動補齊開關與 SHA-256 限速

**類型**: 功能

**檔案**:

- `internal/library/scanner.go`
- `internal/library/reprobe.go`
- `internal/library/hash.go`
- `internal/library/http.go`
- `internal/runtime/config.go`
- `internal/runtime/configapi.go`
- `internal/runtime/server.go`
- `web/admin/src/Settings.tsx`
- `web/admin/src/Monitor.tsx`
- `web/admin/src/ImportJobs.tsx`
- `web/admin/src/Library.tsx`
- `pkg/protocol/openapi.json`

### 問題

- 自動掃描與補齊沒有下次執行時間
- 自動補齊預設一直排程，大檔案完整 SHA-256 可能長時間佔用磁碟
- 設定頁無法停用或限速

### 修改

- 公開排程倒數與候選數、分開自動補齊開關、閒置才補齊，並限制 SHA-256 讀取速度
- `autoBackfill` 預設關閉；開啟後 `backfillIntervalSeconds`（預設 60 秒）檢查一次，只在整個佇列閒置時排一筆，避免與手動匯入／掃描競爭。
- `hashLimitMiBps`（預設 64 MiB/s，0 不限）以 token bucket 限制匯入與重新探測的 SHA-256 完整讀取；佇列仍一次只跑一個工作。
- 掃描狀態加入 `nextRunAt`／`nextRunInSeconds`；補齊狀態加入 `enabled`、`nextCheckAt`、候選數與上次排程影片。
- `GET /api/v1/schedule`、工作監看與影片工作頁顯示下次掃描／補齊倒數、候選數與目前佇列；設定頁可調整自動補齊、間隔與讀取上限。
- 自動掃描仍只讀目錄 metadata，不讀影片內容；只有匯入／重新探測會做 FFprobe 與完整雜湊。

### 驗證

- 新增補齊停用／背壓／排程狀態／掃描倒數／限速測試
- Go 測試、`go vet`、前端建置、`lang:check` 與 OpenAPI 路由檢查通過。
- 測試站重建後自動補齊預設關閉，既有候選保留待手動開啟。
- `GET /api/v1/schedule` 實測下次掃描倒數、候選 16 部與佇列狀態。
- 工作頁、監看頁與設定頁顯示一致。

---

## 2026.10.27 11:10 儲存池 volume GUID、檔案系統與啟停狀態

**類型**: 功能

**檔案**:

- `internal/library/volume_*.go`
- `internal/library/store.go`
- `internal/library/videos.go`
- `internal/library/scanner.go`
- `internal/library/reprobe.go`
- `internal/library/http.go`
- `web/admin/src/Library.tsx`
- `pkg/protocol/openapi.json`
- `web/admin/src/lang/`

### 問題

- 儲存池只存路徑，Windows 盤符改變後找不到同一個 volume
- 也沒有檔案系統、啟停或影片不可用原因

### 修改

- 加入 volume GUID／檔案系統、儲存池啟停與影片來源原因
- SQLite schema v5 新增 `pools.volume_id`、`volume_relative`、`filesystem`、`enabled`；舊池啟動時自動補齊可讀取的中繼資料。
- Windows 原生以 `GetVolumePathNameW`／`GetVolumeNameForVolumeMountPointW` 記錄 volume GUID 與相對路徑，執行時優先使用 GUID
- Linux 顯示容器／主機看到的檔案系統類型，Docker 內沒有可用的 volume GUID。
- `PATCH /api/v1/pools/{id}` 與儲存池 UI 可停用／啟用；停用池不掃描、不匯入、不重新探測、不自動補齊，影片索引與描述保留。
- `Pool` 回傳 `enabled`、`filesystem`、`volumeId`
- 影片新增 `unavailableReason`（`pool_disabled`、`pool_offline`、`source_changed`、`source_missing`）。
- 前端顯示原因並停用播放／重讀，描述仍可編輯。
- 掃描器改用實際解析路徑；播放端點對停用、離線、來源遺失／變更回傳不同錯誤。
- 更新 OpenAPI、i18n 與文件；SMART 健康診斷列為後續選配，不影響目前可用性。

### 驗證

- Go 單元測試（含 Windows volume GUID 盤符變更與 scanner）、`go vet`、前端建置與 `lang:check`
- 測試站重建後檢查 schema v5、檔案系統顯示與啟停切換。

---

## 2026.10.27 10:20 自動補齊缺少技術資訊的既有影片

**類型**: 功能

**檔案**:

- `internal/library/reprobe.go`
- `internal/library/jobs.go`
- `internal/library/backfill_test.go`
- `internal/runtime/server.go`

### 問題

- 自動掃描只處理新檔，既有影片若沒有 `video_media` 不會自動重新探測
- 測試庫 179 部中有 176 部缺少技術資訊

### 修改

- 服務啟動後自動排入缺少技術資訊且沒有任何 reprobe 工作記錄的影片，一次一筆
- 補齊查詢只挑 `video_media` 不存在、且該影片沒有任何 `source='reprobe'` 工作記錄的資料；有 queued／running 工作時不再排下一筆。
- 來源離線、大小或修改時間與索引不符時跳過，不建立失敗工作；曾失敗或已完成的工作也排除，避免自動無限重試。
- 每 10 秒檢查一次，與手動重新讀取共用單一 FIFO 佇列；完成後 media 寫回，下一次查詢自然往下一部。
- 沒有新增設定或 UI；工作會出現在「影片工作」，技術資訊補齊後影片庫會顯示。

### 驗證

- `TestBackfillMissingMediaOneAtATime` 與 `TestBackfillMissingMediaSkipsUnavailableAndRecorded` 通過
- `go test ./...` 與 `go vet ./...` 通過。測試站重建後未按任何按鈕即自動排入並完成多筆，缺少技術資訊由 176 降至 172（背景持續處理）。

---

## 2026.10.27 09:35 修正 Swagger UI 深色 Responses 表與範例區

**類型**: 修正

**檔案**:

- `web/admin/src/api-docs.css`

### 問題

- responses 表頭的 Description 對比不足，範例 JSON 仍是棕色方塊

### 原因

- Swagger UI 的 responses 表頭是 `td.col_header` 而不是 `th`，既有覆寫只涵蓋 `th`
- 範例區 `pre.microlight` 由元件寫入 inline `background:#333; color:white`，一般主題變數無法覆蓋

### 修改

- 補上 `td.col_header`／`thead td` 的顏色與表格框線，並以 `!important` 覆寫範例區的 inline 背景與文字
- 表格標題、內文與框線改用 `--text`／`--line`。
- `pre.microlight` 改用 `--bg`／`--text`，加上 `--line` 邊框與 8px 圓角。
- 同一個規則套用到 request body 與 responses 的範例，light／dark 主題都使用目前變數。

### 驗證

- dark 範例區 `#111820`、文字 `#edf3f9`
- light 範例區 `#f1f4f8`、文字 `#172434`
- Description 標題恢復為 `--text`
- 截圖確認 responses 表與 request body 一致
- `lang:check`、前端建置與 `check:testing` 通過。

---

## 2026.10.27 08:42 調整頁首語言選單間距、字體與簡體登出文字

**類型**: 修正

**檔案**:

- `web/admin/src/style.css`
- `web/admin/src/main.tsx`
- `web/admin/src/lang/zh_CN.overrides.json`

### 問題

- 語言標籤與選單間距不一致，下拉選單未繼承頁面字體
- zh_CN 的登出顯示為「注销」

### 修改

- 統一頁首標籤間距、固定語言選單最小寬度，讓 select／option 繼承頁面字體，並修正簡體登出用語
- `.header-actions label` 改為 inline-flex，固定 8px 間距並避免換行。
- 語言選單固定最小寬度 7.5rem，切換語言時不再因文字長度改變寬度。
- `select` 與 `option` 都使用 `font: inherit`，與頁面其他控制項一致。
- `zh_CN` 轉換後將「注销」修正為「退出登录」。

### 驗證

- 頁首間距 8px、select 寬 120px、select／option 皆為 16px system-ui
- en／zh-CN／zh-TW 在 375px 的影片、匯入、設定、儲存池與工作頁皆無水平溢出。

---

## 2026.10.27 08:20 管理介面本地化、語言註冊器與缺值檢查

**類型**: 功能

**檔案**:

- `web/admin/src/lang/`
- `web/admin/src/i18n/`
- `web/admin/scripts/lang.mjs`
- `web/admin/src/*.tsx`
- `web/admin/package.json`
- `package.json`
- `.github/workflows/release.yml`
- `Docs/localization.md`

### 問題

- 管理介面文字直接寫在元件裡，無法切換語言，也沒有缺鍵或翻譯過期的檢查

### 修改

- 以 zh_TW 為來源，zh_CN 由 OpenCC 台灣片語轉換產生，加入 en、語言註冊器與執行期切換
- `zh_TW.json` 是唯一手動維護的中文來源
- `en.json` 手動維護
- `zh_CN.json` 由 `opencc-js` 的 `twp → cn` 產生，轉換後用 `zh_CN.overrides.json` 修正少數用語。
- `src/i18n/registry.ts` 是語言清單；新增 `ja.json` 後加入一筆即可出現在選單，瀏覽器語言前綴也會自動對應。
- `I18nProvider` 處理瀏覽器語言、`localStorage` 的 `dvn-locale`、`html lang`、`{name}` 插值與缺鍵回退；日期時間依語言格式化。
- `npm run lang:build` 產生 `zh_CN.json`；`npm run lang:check` 檢查缺鍵、空值、插值參數、程式使用未定義的 `t()` 鍵與 zh_CN 是否過期，CI 也會執行。
- 管理介面所有元件改用 `t()`；頁首加入語言選單。Go 錯誤訊息與 OpenAPI 文件內容仍維持中文，列為後續工作。
- 驗證：`lang:check`（356 鍵）、前端建置、Go 測試與 vet、測試站三語言切換、登入／登出，以及 375px 無水平溢出。

---

## 2026.10.27 07:22 改善背景工作進度動畫與更新頻率

**類型**: 修正

**檔案**:

- `web/admin/src/JobProgress.tsx`

### 問題

- 後端每 250 毫秒保存進度，工作頁每五秒更新，導致進度跳動且原生活動動畫不一致

### 修改

- 新增輕量工作列表 API，工作頁活動時每半秒獨立更新並保留快照，避免反覆載入影片庫
- 共用 GPU transform 進度條，只平順呈現已知實際進度；未知長度使用活動動畫，支援減少動態效果
- 補齊單工 FIFO 與離線查詢測試，實測瀏覽器更新 504–517ms，文件說明重試與重新排程語意

**相關文件**: Docs/import-processing.md

---

## 2026.10.27 06:59 加入部署測試最近五組保留與清理流程

**類型**: 維護

**檔案**:

- `scripts/cleanup-deployment-tests.mjs`

### 問題

- 獨立部署測試持續留下容器、資料卷及本機產物，沒有保留上限

### 修改

- 新增獨立測試執行入口、來源標記與最近五組保留政策，提供唯讀預覽及逐項執行紀錄
- 排除執行中、跨工作區、連結與共用資源；永久刪除需顯式開關，回收失敗不降級
- 13 項 Node 測試通過；實際預覽確認六組超額目錄，Windows 回收工具失敗，原資料保留待確認永久清理政策

**相關文件**: Docs/test-retention.md

---

## 2026.10.27 06:41 加入 Swagger UI API 參考與測試頁

**類型**: 功能

**檔案**:

- `web/admin/src/ApiDocs.tsx`

### 問題

- 原始 OpenAPI JSON 不適合直接閱讀，缺少可填參數與執行請求的介面

### 修改

- 加入延後載入的 Swagger UI、功能分組與請求範例，沿用登入 cookie 並限制同源 API 測試
- 透過外掛停用外部驗證元件，資源隨本機與 Docker 打包，適配深淺色與窄螢幕
- 實際瀏覽器驗證 GET 200、協定錯誤 409、POST 探測 202 及無外部連線，補齊操作文件

**相關文件**: Docs/api/README.md

---

## 2026.10.27 06:15 建立 OpenAPI 與協定版本相容性檢查

**類型**: 功能

**檔案**:

- `pkg/protocol/http.go`

### 問題

- 既有 API 缺少正式規格，協定版本只顯示數字而未阻止不相容請求

### 修改

- 嵌入現有 OpenAPI 3.1.1，依服務角色提供規格，加入公開版本與能力公告
- 集中 Build Identity 協定版本並檢查 API 請求標頭，不相容請求在寫入前拒絕；前端宣告支援版本
- 新增路由、回應欄位與參照檢查，完成實際服務及 Docker 驗收；SSE 與跨服務握手列為後續

**相關文件**: Docs/api/README.md

---

## 2026.10.27 05:49 新增既有影片重新探測與服務工作監看

**類型**: 功能

**檔案**:

- `internal/library/reprobe.go`

### 問題

- 既有影片缺少技術資訊補齊操作，服務工作與資源狀態分散

### 修改

- 沿用持久背景佇列重新探測影片，驗證雜湊並保留描述與識別，失敗可重試
- 三服務提供已授權監看 API 與頁面，整合程序 CPU、Go 記憶體、HTTP 流量及影片工作／掃描狀態
- 明確標示服務程序、HTTP 本文與容器觀測範圍，補齊單元與 Docker 驗收

**相關文件**:

- Docs/monitoring.md
- Docs/import-processing.md

---

## 2026.10.27 05:37 加入影片庫分頁、搜尋與內容篩選

**類型**: 功能

**檔案**:

- `internal/library/query.go`

### 問題

- 影片庫每次載入全庫並檢查所有來源，缺少查詢與分頁

### 修改

- 清單 API 預設每頁 24 筆、最多 100 筆，提供筆數與分頁資訊
- 新增標題與簡介搜尋、Tag／分類／儲存池／18+ 篩選、排序及管理頁操作
- 只檢查當頁來源，取消過期查詢並補齊分頁、組合篩選、參數驗證測試

**相關文件**: Docs/video-library.md

---

## 2026.10.27 05:27 加入持久背景匯入與 FFprobe 影片資訊

**類型**: 功能

**檔案**:

- `internal/library/jobs.go`

### 問題

- 同步匯入缺少可恢復工作、進度與影片技術資訊

### 修改

- 新增 SQLite v4 工作佇列、重啟恢復、失敗重試與管理頁面
- FFprobe 檢查影片資訊並保存片長、解析度與編碼；Docker 附帶工具
- 補齊操作文件與測試，修正啟動測試 Scanner 讀取錯誤回報

**相關文件**: Docs/import-processing.md

---

## 2026.10.27 04:50 將 Node 單元測試移出 scripts

**類型**: 整理

**檔案**:

- `test/`
- `package.json`
- `.github/workflows/release.yml`
- `Docs/onboarding.md`

### 問題

- `.test.mjs` 與建置、啟動、檢查用腳本放在同一個 `scripts/`，不容易區分哪些檔案是測試

### 修改

- 將 Node 單元測試集中到 `test/unit/`、整合流程移到 `test/integration/`，新增 `npm run test:unit`，CI 改用同一個入口
- `test/unit/build-inputs.test.mjs`、`test/unit/workspaces.test.mjs` 的 import 改指向 `scripts/`
- `test/integration/deployment.mjs` 由 `npm run test:deployment` 執行。
- `test/integration/images.mjs` 由 `npm run test:images` 執行。
- Go 測試仍依 Go 慣例留在各套件旁，由 `go test ./...` 執行
- `test/README.md` 說明兩者的界線與執行方式

---

## 2026.10.27 04:40 新增管理員登入、設定覆寫與重新啟動流程

**類型**: 功能

**檔案**:

- `internal/auth/`
- `internal/runtime/config.go`
- `internal/runtime/configapi.go`
- `internal/runtime/server.go`
- `web/admin/src/api.ts`
- `web/admin/src/Login.tsx`
- `web/admin/src/Settings.tsx`
- `web/admin/src/main.tsx`
- `web/admin/src/Library.tsx`
- `scripts/dev.mjs`
- `test/integration/deployment.mjs`
- `test/integration/images.mjs`
- Compose 與部署文件

### 問題

- 管理 API 沒有授權
- Docker config 是唯讀單檔掛載，無法安全地從網頁修改設定
- 設定變更後也沒有套用／重啟流程

### 修改

- 加入單一管理員 session、資料目錄設定覆寫、設定頁與監督式重新啟動
- 首次啟動產生隨機管理員密碼並只顯示一次；可用 `DVN_ADMIN_PASSWORD` 覆寫。憑證以 PBKDF2-HMAC-SHA256 保存於資料目錄 `auth.json`。
- 登入使用 HttpOnly、SameSite=Strict 的 `dvn_session` cookie，不把 token 放進 localStorage
- 閒置 7 天滑動續期，絕對上限 90 天，登入失敗每個來源 5 次鎖定 30 秒。
- 登入後可變更密碼；登出或改密碼會輪替 session secret，使既有 session 全部失效。
- 管理頁設定寫入資料目錄的 `config.override.json`，主機 JSON 保持唯讀；只保存與基準不同的欄位，可重設回基準。
- 設定頁顯示來源、覆寫檔、資料／網頁目錄與需重啟狀態；確認後呼叫重啟 API。
- 重新啟動以 exit code 75 結束。
- 交由 Docker `restart: unless-stopped`、systemd `Restart=on-failure` 或 `npm run dev` 監督流程重新啟動。
- 直接手動執行時會停止。
- 修正 Vite proxy 的 `changeOrigin`，讓開發模式的登入 Origin/Host 檢查正常。
- 部署驗收、image 測試與文件加入登入 cookie 流程。

---

## 2026.10.27 04:05 完成 v0.1.0 首次發版本機驗收

**類型**: 驗證

**檔案**:

- `Docs/deployment-test-results.md`

### 問題

- 首次發版流程尚未在本機完整演練，無法確認雙架構 OCI、release 標籤與實際啟動結果

### 修改

- 依 release workflow 參數建置三個服務的 amd64/arm64 OCI，並啟動 amd64 release image 檢查狀態
- Go 測試、vet、腳本測試、npm ci 與前端正式建置全部通過
- 三個 OCI tar 均為雙架構，version=0.1.0、dirty=false、channel=release 與來源摘要正確
- release image 的 `/api/v1/status` 與管理頁可讀；尚未觸發 GitHub Actions 或推送 registry
- 本地 tag `v0.1.0` 指向 `ac05ded`，尚未 push

---

## 2026.10.27 03:55 新增接手開發與部署入口文件

**類型**: 文件

**檔案**:

- `Docs/onboarding.md`
- `README.md`
- `Docs/deployment.md`
- `testing/README.md`

### 問題

- 文件分散在 README、部署手冊、測試工作區與進階參考，新接手者不知道該從本機開發、Docker 測試或正式部署哪一條開始

### 修改

- 新增 onboarding 入口，依目標分流三條路徑並整理文件地圖與交接檢查清單
- 說明前置需求、目前服務範圍與三種環境的資料位置
- 補上程式建置時間、鏡像建置時間與 check:testing 的判讀方式
- README、Docker 上手手冊與 testing 工作區頂部加入入口連結

---

## 2026.10.27 03:45 加入 image 建置時間、來源摘要與測試環境版本檢查

**類型**: 新增

**檔案**:

- `scripts/build-inputs.mjs`
- `scripts/image.mjs`
- `scripts/check-testing.mjs`
- `scripts/testing-up.mjs`
- `Dockerfile`
- `pkg/buildinfo/buildinfo.go`
- `web/admin/src/main.tsx`
- `compose.yaml`
- `testing/compose.yaml`
- `testing/README.md`

### 問題

- 本機 compose 啟動時無法從服務資訊判斷 image 是幾點建置、是否由目前原始碼產生
- Docker 層快取也讓程式編譯時間與 image 建置時間混在一起

### 修改

- 建置腳本注入建置時間、來源摘要與 OCI 標籤，新增測試環境檢查與一鍵建置啟動工具
- `npm run image -- all` 一次建置三服務，寫入 image 建置時間、commit、dirty 與來源摘要
- 服務資訊分開顯示程式建置時間與鏡像建置時間，並附來源摘要前 12 碼
- `npm run check:testing` 比對 image 標籤、Git HEAD 與執行中容器，過期或使用舊 image 時回傳非 0
- `npm run testing:up` 建置、啟動並自動執行版本檢查
- 直接 `docker compose build` 未帶參數時明確標示 unknown，不假裝有建置資訊
- 正式版本 workflow 同步注入建置時間與來源摘要

---

## 2026.10.27 03:32 加寬管理介面並以對話框播放與編輯影片

**類型**: 改善

**檔案**:

- `web/admin/src/style.css`
- `web/admin/src/Library.tsx`
- `web/admin/src/main.tsx`
- `web/admin/index.html`

### 問題

- 固定 1000px 容器與單欄堆疊讓服務資訊、儲存池及匯入表單需要長距離捲動
- 播放器接在整個影片庫之後，影片變多時必須捲到最底部才找得到

### 修改

- 容器加寬至 1200px，儲存池與服務資訊自動分欄，匯入表單拆成來源與影片資料雙欄，播放與編輯改為原生 dialog 模態
- 服務資訊的生效設定在寬螢幕以雙欄鍵值呈現，避免長路徑斷行
- 播放器與編輯表單覆蓋目前頁面，關閉後保留原本的影片庫捲動位置
- 匯入來源面板在寬螢幕保持可見，窄螢幕自動回到單欄
- 補上 favicon，消除瀏覽器 404 紀錄

---

## 2026.10.27 03:09 修正測試手冊裸網址與表格格式

**類型**: 文件

**檔案**:

- `testing/README.md`

### 問題

- 測試服務網址觸發 MD034，部分表格分隔列不符合 MD060 compact

### 修改

- 將三服務與區網測試網址改為具名連結，修正部署手冊兩個表格分隔列
- 使用 markdownlint 驗證 README、Docs 與測試手冊的 MD034 及 MD060 compact 規則

---

## 2026.10.27 03:07 補齊 Docker 多磁碟掛載操作手冊

**類型**: 文件

**檔案**:

- `Docs/docker-volumes.md`
- `testing/compose.local.example.yaml`
- `compose.override.example.yaml`
- `.gitignore`

### 問題

- 既有文件僅說明預設 media 掛載，缺少額外磁碟、多組 volumes 及欄位用途的完整引導

### 修改

- 新增本機 compose.local.yaml / compose.override.yaml 疊加範例、多磁碟追加、主機與容器路徑對應、唯讀及 create_host_path 說明
- 個人掛載由 example 複製並列入 .gitignore，不修改版控中的 compose.yaml
- 說明選對 Compose 環境、執行 up 套用掛載、分別註冊儲存池與讀取權限診斷
- 在部署手冊、測試工作區及 README 加入專用手冊連結，表格採 compact 格式

**相關文件**: Docs/docker-volumes.md

---

## 2026.10.27 02:58 保存最後成功端口並建立原子設定寫入底層

**類型**: 新增

**檔案**:

- `internal/runtime/state.go`

### 問題

- 服務重啟未沿用上次成功端口，設定與執行狀態缺少完整替換保存機制

### 修改

- 各服務以 runtime.json 記錄成功端口，僅在同介面及現有候選清單內優先沿用，Docker 固定端口自動停用記憶
- 狀態損壞或版本不支援時保留原檔並告警；保存失敗仍可啟動，診斷頁顯示實際狀態
- 新增設定驗證及同目錄暫存、同步、替換底層，不新增網頁寫入 API，不改寫管理者 config
- 三服務未明確指定資料位置時使用獨立預設目錄，舊 Node 資料維持沿用
- 測試占用遞補、實際重啟沿用、替換失敗與損壞恢復，Docker 驗證 NodeID 及 config 保留

**相關文件**: Docs/configuration.md

---

## 2026.10.27 02:31 新增儲存池影片自動發現與穩定掃描

**類型**: 新增

**檔案**:

- `internal/library/scanner.go`

### 問題

- 影片必須逐一手動匯入，儲存池無法自動檢查新加入的檔案

### 修改

- Video Node 定期遞迴掃描已註冊儲存池，觀察檔案大小及修改時間穩定後背景匯入
- 提供自動掃描開關、間隔及穩定秒數設定，保留立即掃描、結果統計及錯誤資訊
- 略過已索引檔案與符號連結，不覆寫描述或移除離線紀錄，前端定期更新清單
- 測試穩定等待、寫入中重計時、去重、離線、手動排程與背景週期匯入，更新手冊

**相關文件**: Docs/configuration.md

---

## 2026.10.27 02:14 統一開發設定並建立固定 Docker 測試工作區

**類型**: 新增

**檔案**:

- `scripts/workspaces.mjs`

### 問題

- 開發啟動未載入實際 config，缺少可持續操作且與現有資料隔離的測試目錄

### 修改

- 新增 setup:dev 與 setup:test，只建立缺少的設定，開發啟動明確指定各服務 JSON 並保留既有 Node 資料
- 新增 testing/compose.yaml、獨立測試資料卷及 local/media/config，以 18710、18720、18730 持續測試
- 服務啟動記錄設定來源與絕對資料目錄，新增設定診斷 API 及管理頁面生效值展示
- 驗證初始化不覆寫、舊資料沿用、三服務健康、範例匯入與重建後資料保留，補齊設定及測試手冊

**相關文件**:

- Docs/configuration.md
- testing/README.md

---

## 2026.10.27 01:58 將服務入口目錄更名為 apps

**類型**: 修正

**檔案**:

- `scripts/dev.mjs`

### 問題

- cmd 資料夾與 Windows 檔案總管輸入 cmd 開啟終端的操作衝突

### 修改

- 三服務入口移至 apps，更新開發與正式建置腳本、Dockerfile 及 README 路徑
- 驗證三服務建置與完整 Go 測試通過

---

## 2026.10.27 01:53 完成 Compose 部署驗收並簡化操作手冊

**類型**: 改進

**檔案**:

- `scripts/test-deployment.mjs`

### 問題

- 部署手冊混合建置、設定與多服務細節，首次操作不易跟隨且缺少完整部署驗收

### 修改

- 新增獨立 Compose 測試專案及生成影片，驗證本機建置、外掛設定、匯入、影片傳輸及重建後資料保留
- 三服務 Compose 預設唯讀掛載主機 config，提供自訂設定檔路徑環境變數
- 重寫六步驟上手手冊，分離進階 Linux 與正式發版參考並新增驗收紀錄
- 測試完成後停止專案並保留報告與資料卷，不使用現有影片庫

**相關文件**:

- Docs/deployment.md
- Docs/deployment-test-results.md

---

## 2026.10.27 01:42 新增本機 Docker 建置與 Linux 部署基礎

**類型**: 新增

**檔案**:

- `Dockerfile`

### 問題

- 缺少不依賴 CI 的容器建置、部署設定及 Linux 服務管理方式

### 修改

- 新增三個獨立 target 的多階段 Dockerfile、本機 image 腳本、Compose 與資料持久化掛載
- 容器固定內部端口，Compose 設定外部端口；加入非 root 執行、健康檢查及明確建置識別
- 新增 Linux 交叉編譯與 systemd 範本，CI 僅正式標籤觸發檢查與建置，registry 發布待授權
- 本機實際建置三 image，驗證 Linux API、靜態頁面與 Video Node 重啟 NodeID 持久化

**相關文件**: Docs/deployment.md

---

## 2026.10.27 01:33 將 DVN 管理面板拆分獨立分頁

**類型**: 改進

**檔案**:

- `web/admin/src/navigation.ts`

### 問題

- 所有管理功能堆疊在同一頁，難以找到操作入口

### 修改

- 新增總覽、影片庫、儲存池、匯入影片、服務資訊導覽與對應頁面
- 總覽呈現摘要與快捷入口，分頁網址支援書籤、刷新及瀏覽器返回
- 切換分頁保留匯入與編輯草稿，版本及 NodeID 集中至服務資訊

---

## 2026.10.27 01:33 新增 Tag、原作者及轉載來源與影片資料編輯

**類型**: 新增

**檔案**:

- `internal/library/attribution.go`

### 問題

- 影片缺少推薦分類標記、原作者及原創轉載聲明，匯入後無法修正描述

### 修改

- SQLite v3 新增獨立 Tag 索引與作品來源表，轉載驗證原作者及來源或待補聲明
- 共用匯入與編輯表單，新增多作者連結、來源平台、轉載說明、語言及描述更新 API
- 加入 v2 遷移、Tag 去重、連結驗證、更新原子性與重啟持久化測試

---

## 2026.10.27 01:23 補齊影片匯入描述與內容標記

**類型**: 新增

**檔案**:

- `internal/library/metadata.go`

### 問題

- 影片匯入僅有標題，缺少簡介、分類、18+ 與授權資訊

### 修改

- 新增內容簡介、可自訂分類、18+ 標記與選填授權文字，後端驗證欄位長度
- SQLite 升級 v2 保留原有影片與節點識別，加入資料遷移與描述持久化測試
- 更新匯入表單、影片清單及播放資訊；授權未填不推導轉載權利，18+ 尚不提供年齡存取限制

---

## 2026.10.27 01:21 新增 Video Node 儲存池與影片庫第一階段

**類型**: 新增

**檔案**:

- `internal/library/store.go`

### 問題

- 服務僅有診斷骨架，尚不能索引或播放本機影片

### 修改

- 新增 SQLite schema v1、持久 NodeID、儲存池註冊及離線狀態，Go 需求提升至 1.26
- 匯入池內既有影片並計算 SHA-256，保留發布及副本識別，支援 Range 與 HEAD 播放
- 新增管理介面儲存池、匯入表單、影片清單與播放器，來源離線或變更時拒絕播放
- 加入持久化、路徑越界、跨站拒絕、取消匯入、來源變更與 HTTP 播放測試，更新 README 及 TODO

**相關文件**: Docs/architecture.md

---

## 2026.10.27 01:11 固定三服務執行檔名稱與路徑

**類型**: 修正

**檔案**:

- `scripts/dev.mjs`

### 問題

- 開發啟動使用程序 ID 命名執行檔，導致 Windows 防火牆反覆識別為不同程式

### 修改

- 開發與正式建置共用 bin/video-node、bin/central、bin/transcode-worker 固定路徑，Windows 使用 exe 副檔名
- 建置失敗時補充關閉先前服務的提示並更新啟動文件

---

## 2026.10.27 01:08 調整服務預設端口並防止 Windows 端口共用

**類型**: 修正

**檔案**:

- `internal/runtime/listen_windows.go`

### 問題

- 8080 與其他監聽程序共用時，代理請求遭 ECONNRESET

### 修改

- Video Node、Central、Worker 預設改為 18610、18620、18630，保留三個遞補端口並同步範例與代理
- Windows socket 採獨占綁定，避免同一端口被多個程序共用
- 整合啟動在開放前端前驗證 Video Node 狀態 API

---

## 2026.10.27 01:02 新增統一開發啟動與動態 API 代理

**類型**: 修正

**檔案**:

- `scripts/dev.mjs`

### 問題

- 手動分別啟動難以管理，後端遞補端口後固定代理導致管理頁面連線失敗

### 修改

- 新增 npm run dev 與 dev:all，集中建置啟動、日誌與關閉
- 從本次後端啟動日誌取得實際端口，覆蓋 Vite 代理；後端就緒後才啟動前端
- 驗證前端打包與代理 API 回應 200；更新 README 與 TODO

---

## 2026.10.27 00:58 調整預設監聽以支援多網卡區網存取

**類型**: 修正

**檔案**:

- `internal/runtime/config.go`

### 問題

- 僅綁定 loopback 導致區網 IP 無法連線

### 修改

- 三服務與 Vite 預設監聽 0.0.0.0，涵蓋所有 IPv4 介面並保留指定介面設定
- 更新範例設定及實際連線位址、多網卡與防火牆說明

---

## 2026.10.27 00:50 建立 DVN Phase 1 三服務與管理介面骨架

**類型**: 新增

**檔案**:

- `internal/runtime/server.go`

### 問題

- 架構已定案，專案尚無可執行服務

### 修改

- 建立 Go 三服務、設定、Port 遞補、健康與版本 API、結構化日誌及優雅關閉
- 建立 React Vite TypeScript 管理頁面與三種外觀，支援 Go 提供打包靜態資源
- 新增建置腳本、架構基準、開發說明與分階段 TODO；影片功能仍待 Phase 2

**相關文件**: Docs/architecture.md

---

## 2026.10.26 04:57 初始接入ChangelogKit開發變更管理工具

**類型**: 新增

### 修改

- cli測試

接入開發管理工具

---
