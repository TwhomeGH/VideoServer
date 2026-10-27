# 部署測試產物保留

`npm run test:deployment` 透過獨立執行入口啟動驗收，結束後執行保留階段，預設保留最近 5 組已停止且來源已確認的 `dvn-test-<時間戳記>`。驗收成功與失敗都會留下最新資料供診斷。

第一次使用清理功能先在專案根目錄執行 `npm ci`，安裝回收站工具；前端依賴仍由 `npm --prefix web/admin ci` 管理。

## 清理範圍

- 本專案 `data/deployment-tests/dvn-test-<13 位時間戳記>/` 的設定、結果與產生的範例影片。
- 來源標記、工作目錄與環境檔均相符的測試容器。
- 該測試專案的 `_default` 網路與 `_node-data` 資料卷，且未被其他容器共用。

`dvn-testing`、`dvn`、其他專案、原始影片庫、Docker image 與建置快取不在清理範圍。正在執行的測試、未知檔案、連結／掛載跨界或來源不明的資源會跳過並說明原因；這類受保護項目可能使總數暫時超過 5。

新驗收另寫入 `run.json`，記錄工作區、程序與執行狀態。舊紀錄可用既有 `result.json`、設定與環境檔核對來源。沒有對應目錄的孤立 Docker 資源只列為略過，不猜測其歸屬。

## 政策與預覽

[test/deployment-retention.json](../test/deployment-retention.json) 集中設定：

| 設定 | 預設 | 含義 |
| --- | --- | --- |
| `keep` | `5` | 保留最近幾組 |
| `allowDockerRemoval` | `false` | 是否允許永久移除超額的測試 Docker 資源 |
| `hostDataMode` | `recycle` | 本機目錄使用系統回收站；可明確改為 `permanent` |

唯讀預覽：

```sh
npm run test:cleanup
```

依目前政策執行：

```sh
npm run test:cleanup -- --apply
```

預覽會列出絕對目錄、檔案數及 Docker 身分。執行前重新檢查來源、檔案快照與資源使用狀態；不使用 `force` 或全域 `prune`。執行記錄寫入 `data/deployment-retention/last-run.json`，僅保留最近一次，避免清理紀錄自身持續增加。

## 回收與永久刪除

`recycle` 使用系統回收站，失敗即停止，不自動改成永久刪除。此環境的 Windows 回收工具曾回傳錯誤，舊目錄仍保留；程式會以非零結束碼反映清理未完成，不能宣稱已縮減至 5 組。

Docker 資料卷無系統回收站，移除後不能透過回收站還原。只有明確接受這些為可丟棄的測試產物後，才啟用 `allowDockerRemoval`。本機資料同樣需要明確接受不可還原性後，才能將 `hostDataMode` 改為 `permanent`。

也可只對一次已審核的清理使用顯式選項：

```sh
npm run test:cleanup -- --apply --allow-docker-removal --permanently-delete-host-data
```

以上會永久移除預覽選定的舊測試 Docker 資源與本機資料，不是回收。保留政策已啟用永久模式時，後續每次部署驗收都會依同一範圍自動執行，不再逐次詢問。

請勿同時執行多個清理程序。Docker 或回收操作失敗時會停止後續項目；已完成的步驟記錄於報告中，剩餘產物保留。
