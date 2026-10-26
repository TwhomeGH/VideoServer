# 設定與資料工作區

同一套 Go 程式使用相同 JSON 欄位，但開發、Docker 測試與部署有各自的實際設定及資料。

| 模式 | 設定來源 | 資料位置 | 啟動入口 |
| --- | --- | --- | --- |
| 開發 | local/development/config/服務名.json | local/development/data/服務名/；既有節點例外見下方 | 根目錄 npm run dev:all |
| 固定 Docker 測試 | testing/local/config/服務名.json | dvn-testing 的獨立 Docker volumes | testing/ 內 docker compose |
| 根目錄 Docker 部署 | deployments/docker/服務名.json，或 .env 指定檔案 | dvn 的獨立 Docker volumes | 根目錄 docker compose |
| Linux 原生 | /etc/dvn/服務名.json | /var/lib/dvn/服務名 | systemd |

## 開發模式

`npm run setup:dev` 可預先建立設定；`npm run dev`／`dev:all` 也會自動初始化缺少的設定，並明確傳入 `-config`。
每次啟動會印出實際使用的 JSON 路徑，網頁「服務資訊」則顯示設定來源與生效值。
編輯該 JSON 後重新啟動開發程序，不必改 Go 原始碼。檔案不會因下一次初始化而被覆蓋。

新環境預設使用獨立服務資料目錄。若首次初始化偵測到原本的 `data/library.sqlite`，Video Node 的新設定會沿用既有 `data/`，不自動複製或搬移資料庫。Central／Worker 仍使用各自目錄。
之後若要搬資料，應先停止節點並備份，再搬移整個資料目錄與調整 JSON；不要在執行中複製 SQLite。開發模式的儲存池填主機實際路徑，例如 `F:\VideoServer\local\development\media`，不是容器的 `/media`。

## 載入與路徑規則

服務先建立內建預設值，再套用 `-config` 的 JSON；未指定欄位保留預設，未知欄位或無效設定會讓啟動失敗。
相對的 dataDirectory／webDirectory 依程序工作目錄解析，服務會轉成絕對路徑並顯示生效值。自動產生的開發設定直接使用絕對路徑，搬動專案後需調整既有 JSON。
根目錄 `.env` 由根目錄 Compose 讀取；`testing/.env` 由測試 Compose 讀取。Go 不直接讀取這些 .env，開發腳本也不拿它們覆蓋服務 JSON。
NodeID、儲存池與影片資料存在 SQLite，不寫回設定檔。目前設定是啟動時讀取，沒有熱更新或網頁寫入設定功能。

## 新增影片自動掃描

Video Node 預設啟用 `autoScan: true`，`scanIntervalSeconds: 30`，`fileStableSeconds: 10`。既有 JSON 沒有這些欄位時同樣套用預設。要停用自動掃描可設定 autoScan 為 false，仍可在儲存池頁面手動排程掃描。
服務啟動與註冊新池時會檢查，之後每 30 秒遞迴掃描已註冊池。檔案需至少兩次觀察大小與修改時間一致、觀察間隔達 10 秒且修改時間足夠久，才計算雜湊並建立索引；通常在新增後約 30–60 秒出現在清單。重啟會重新觀察尚未索引的檔案。
自動匯入使用檔名作為標題，作品類型為「未確認」，不推定作者、授權、分類或 18+ 狀態。可以在影片庫編輯補填。管理頁面每五秒重新讀取清單及掃描狀態。
掃描略過既有索引與符號連結，不重寫描述、不刪除離線紀錄，也不自動重新索引已變更檔案。支援的副檔名與手動匯入相同；沒有 FFprobe 內容驗證。儲存池仍須先註冊一次，程式不會擅自搜尋未註冊的磁碟。
穩定觀察只是降低讀到未完成檔案的機率，不能保證來源已完成傳輸。建議複製時使用 `.part` 等暫存副檔名，完成後再改為 `.mp4`。大型影片的 SHA-256 計算在背景序列執行，不會同時重複掃描。

## Git 與測試隔離

範例、Compose 與初始化腳本納入 Git；`local/`、`testing/local/`、`testing/.env` 均忽略，避免提交個人路徑與測試影片。新同事取得原始碼後執行 setup 即可建立自己的設定。
固定測試環境使用說明見 [testing/README.md](../testing/README.md)。

## 最後成功端口與執行狀態

`rememberLastSuccessfulPort: true` 會在下次啟動時優先嘗試上次成功的端口，再依 preferredPort、fallbackPorts 順序遞補。只接受仍在目前候選清單內且監聽介面相同的記錄；重複候選只嘗試一次。
想強制回到 preferredPort，可暫時停用 rememberLastSuccessfulPort，或從候選清單移除不再允許的舊端口。沒有 fallbackPorts 時自動停用端口記憶，因此既有 Docker 設定不會自行更換容器內端口。

每個服務完成初始化後，將 schemaVersion、service、host、lastSuccessfulPort、updatedAt 存到自己的 `dataDirectory/runtime.json`。即使端口記憶停用，也會記錄本次端口供診斷，但不拿它改變下次端口順序。
直接執行服務而未指定 dataDirectory 時，預設使用 `data/服務名/`，避免三服務互相覆寫狀態；Video Node 若偵測到舊 `data/library.sqlite` 則先沿用舊 data。明確指定的 JSON 路徑不會被這個預設取代。
狀態檔不包含 NodeID，也不修改 config；NodeID 仍由 SQLite 保存。不同服務或不同節點必須使用獨立資料目錄。管理頁面「服務資訊」可查看上次端口、實際端口、狀態檔位置與保存結果。
狀態檔不存在時正常啟動；內容損壞、服務身分不符或版本不支援時告警、忽略舊記錄，並保留原檔不覆寫。保存失敗時服務仍可運作，日誌及診斷頁會明確顯示未保存。若需處理損壞檔，先停止服務並保留備份，再由管理者恢復有效狀態。

## 原子設定保存底層

`SaveConfig` 先驗證完整設定，再使用與 runtime.json 共用的保存流程：同目錄暫存檔 → 完整寫入 → 同步 → 關閉 → 替換目標。Windows 使用帶替換與 write-through 旗標的 MoveFileEx，Linux 使用同目錄 rename 並同步目錄。
替換前失敗不截斷舊檔；失敗暫存檔以 `.檔名.pending-*` 留在同目錄供診斷，不自動清理。成功後沒有額外暫存檔。若替換成功後的目錄同步失敗，錯誤會明確表示「檔案已替換」，不會假稱已回復舊檔。
這項底層不等於網頁設定編輯或熱更新；目前沒有對外寫入 API，Docker 唯讀掛載的 config 仍由主機管理。保存新設定後須明確重啟才會套用。讀寫中的其他程序、權限、唯讀掛載及檔案系統特性仍可能使替換失敗；不保證遠端／虛擬檔案系統與突然斷電下的所有情境。
