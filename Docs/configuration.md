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
NodeID、儲存池與影片資料存在 SQLite，不寫回主機設定檔。管理頁的設定修改會寫入資料目錄的 `config.override.json`，主機 JSON 仍是唯讀基準；儲存後需重新啟動才會套用。

## 儲存池狀態

儲存池保存 `enabled`、實際 `online`、`filesystem`、Windows `volumeId` 與 `volumeStatus`。`enabled` 是管理者的開關；`online` 只表示目前能不能解析並讀取路徑。兩者分開，所以「已停用」和「離線」不會混在一起。

Windows 原生模式會在註冊時記錄磁碟區 GUID 與相對於掛載點的路徑；之後盤符改變時，服務會優先使用 GUID 路徑，不依賴舊盤符。Docker／Linux 沒有同等的 volume GUID，仍以容器內路徑為準；顯示的檔案系統是容器看到的類型（可能是 9p、virtiofs、overlay 或 fuse），不一定是主機磁碟的 NTFS。Docker 的 `volumeStatus` 會是 `unsupported`，前端顯示「不適用（容器或此平台）」，不是漏抓。

儲存池頁的「重新檢查儲存池資訊」會重新查詢在線池的 `filesystem`／`volumeId` 並更新顯示；啟動時只會補一次，沒有定期自動重查。離線池保留舊值，避免把暫時讀不到誤判成沒有資訊。

停用儲存池會停止掃描、匯入、重新探測與自動補齊；既有影片索引、描述與原始檔案保留。影片會標示所屬儲存池已停用，播放與重新讀取停用，描述仍可編輯；重新啟用後會重新檢查來源。

## 管理頁設定編輯

主機 JSON 是唯讀基準；管理頁只寫入資料目錄的 `<dataDirectory>/config.override.json`，因此 Docker 的單檔唯讀掛載不需要改成可寫。測試環境的基準檔是 `testing/local/config/video-node.json`，掛載到容器 `/app/config.json`；Docker 內的覆寫檔則是資料卷中的 `/data/config.override.json`。網頁「設定覆寫（進階）」顯示實際的基準檔與覆寫檔路徑。
可編輯欄位是 host、preferredPort、fallbackPorts、rememberLastSuccessfulPort、autoScan、scanIntervalSeconds、scanIdleMaxSeconds、fileStableSeconds、autoBackfill、backfillIntervalSeconds、backfillDailyLimit、backfillDailyGiB、hashLimitMiBps。dataDirectory、webDirectory 與基準檔路徑只顯示，不能從網頁變更。
儲存時只記錄與基準不同的欄位；欄位改回基準值會從覆寫檔移除。按「清除設定覆寫」會刪除整個 `<dataDirectory>/config.override.json`，回到基準設定檔；這只影響設定，不會修改基準檔、影片、儲存池或自動補齊今日用量。

程序啟動時先讀基準 JSON，再套用覆寫並重新驗證；未知欄位、無效端口或無效秒數會讓啟動失敗。管理頁儲存後會顯示「需重新啟動」，按下確認後服務以 exit code 75 結束，交由 Docker `restart: unless-stopped`、systemd `Restart=on-failure` 或 `npm run dev` 的監督流程重新啟動並套用。直接手動執行二進位且沒有監督程式時，服務會停止，需自行重新啟動。

## 自動排程與背景負載

`autoScan` 控制目錄 metadata 掃描（`WalkDir`／`Stat`），不讀影片內容；`scanIntervalSeconds` 是活躍池的基礎間隔。每個儲存池有自己的下次掃描時間：連續沒有新檔／變更／待穩定檔案時，間隔會逐步加倍，最高到 `scanIdleMaxSeconds`（預設 300 秒）；有變化或手動掃描時重置回基礎間隔。只掃描到期的池，不會因為一個池活躍就讓全部池一直付出完整 traversal。退避狀態存在記憶體，服務重啟後從基礎間隔重新開始。

儲存池頁可對單一池按「掃描此池」，對應 `POST /api/v1/pools/{id}/scan`；完成後重置該池退避。全域「立即掃描新增影片」仍會掃描所有池。真正大量讀取的是匯入／重新探測的 FFprobe＋SHA-256。

`autoBackfill` 預設關閉。開啟後每 `backfillIntervalSeconds`（預設 60 秒）檢查一次，只在整個佇列閒置時排一筆缺少技術資訊的影片，不會與手動匯入或掃描競爭。「閒置」是指 `import_jobs` 沒有任何 queued／running 工作，包含手動匯入、掃描與手動重讀；不是只看自動補齊。有缺少技術資訊的候選時才會排程。

每日上限 `backfillDailyLimit`（預設 50 部）與 `backfillDailyGiB`（預設 20 GiB）取先到者，`0` 表示不限；只限制**自動**補齊，手動「重新讀取影片資訊」不受限。用量以排程當下計入，持久化在 `backfill_usage`，以服務本地日切換，重啟不會歸零；達到上限後停止自動排程，候選保留，隔日重新開始。監看頁在上限用完時提供「重設自動補齊今日用量」，需要二次確認；只清今天，不影響其他日期、既有工作或設定覆寫。

Docker 預設可能使用 UTC；Compose 已預設 `TZ=Asia/Taipei`，也可用環境變數覆寫。Go 二進位內嵌 `time/tzdata`，不依賴映像檔的 zoneinfo，因此容器內也能正確切換日界。

`hashLimitMiBps` 限制 SHA-256 讀取速度（預設 64 MiB/s，0 表示不限），套用到匯入與重新探測；佇列仍一次只處理一個工作。

影音工作頁與工作監看會顯示下次掃描／補齊檢查的倒數、候選數量與目前佇列；`GET /api/v1/schedule` 提供相同摘要。停止自動補齊不影響手動「重新讀取影片資訊」。

## 管理授權與 session

第一次啟動且沒有 `DVN_ADMIN_PASSWORD` 時，服務產生隨機管理員密碼，只在日誌顯示一次；之後可在「設定」頁用目前密碼變更。設定 `DVN_ADMIN_PASSWORD` 時，該值是啟動時的管理密碼；若與已保存的不同，會更新憑證並讓既有 session 失效。憑證保存在 `<dataDirectory>/auth.json`，使用 PBKDF2-HMAC-SHA256 雜湊與隨機 session secret，檔案權限為 0600。

登入成功後只設定 `dvn_session` cookie：HttpOnly、SameSite=Strict，JavaScript 無法讀取，不會在 localStorage 保存 token。使用中的 API 請求會在閒置期限剩餘一半時自動續期；閒置期限 7 天，絕對上限 90 天，超過任一期限需重新登入。登出或變更密碼會輪替 session secret，讓所有裝置的既有 session 失效。登入失敗每個來源連續 5 次會暫停 30 秒。

`GET /api/v1/status`、`GET /healthz`、`/api/v1/auth/session` 與 `/api/v1/auth/login` 保持公開；其他 `/api/v1/*` 需要有效 cookie。API 用戶端請先 `POST /api/v1/auth/login` 並保存回傳的 cookie。瀏覽器 cookie 以主機名稱區分、不以端口區分；同一主機名稱下同時跑多個實例時，切換實例可能需要重新登入，開發與測試可用不同主機名稱區隔。

密碼不會以明文保存，`auth.json` 只有 PBKDF2 雜湊與 session 金鑰。首次隨機密碼只在日誌出現一次；忘記時可設定非空的 `DVN_ADMIN_PASSWORD` 後重啟，或停止服務、備份資料目錄後移除 `auth.json` 再啟動，讓服務重新產生一次性密碼。移除 `auth.json` 不會刪除 NodeID、儲存池或影片索引，但會讓既有 session 失效；不要直接刪除整個資料目錄或 volume。

HTTPS/TLS 尚未內建；直接以 HTTP 提供時 cookie 不會標記 Secure。若在前面放 TLS 反向代理，請確認瀏覽器實際使用 HTTPS。

## 新增影片自動掃描

Video Node 預設啟用 `autoScan: true`，`scanIntervalSeconds: 30`，`fileStableSeconds: 10`。既有 JSON 沒有這些欄位時同樣套用預設。要停用自動掃描可設定 autoScan 為 false，仍可在儲存池頁面手動排程掃描。
服務啟動與註冊新池時會檢查，之後每 30 秒遞迴掃描已註冊池。檔案需至少兩次觀察大小與修改時間一致、觀察間隔達 10 秒且修改時間足夠久，才計算雜湊並建立索引；通常在新增後約 30–60 秒出現在清單。重啟會重新觀察尚未索引的檔案。
自動匯入使用檔名作為標題，作品類型為「未確認」，不推定作者、授權、分類或 18+ 狀態。可以在影片庫編輯補填。管理頁面每五秒重新讀取清單及掃描狀態。
掃描略過既有索引、既有匯入工作、停用池與符號連結，不重寫描述、不刪除離線紀錄，也不自動重新索引已變更檔案；週期掃描會統計 `changed` 與 `deleted`，但只作為狀態提示，不會改 hash 或刪除索引。穩定檔案交由背景工作執行 FFprobe 與雜湊計算；失敗工作須手動重試。儲存池仍須先註冊一次，程式不會擅自搜尋未註冊的磁碟。
穩定觀察只是降低讀到未完成檔案的機率，不能保證來源已完成傳輸。建議複製時使用 `.part` 等暫存副檔名，完成後再改為 `.mp4`。大型影片的 SHA-256 計算在背景序列執行，不會同時重複掃描。

## Git 與測試隔離

範例、Compose 與初始化腳本納入 Git；`local/`、`testing/local/`、`testing/.env` 均忽略，避免提交個人路徑與測試影片。新同事取得原始碼後執行 setup 即可建立自己的設定。
固定測試環境使用說明見 [testing/README.md](../testing/README.md)。

## 最後成功端口與執行狀態

`rememberLastSuccessfulPort: true` 會在下次啟動時優先嘗試上次成功的端口，再依 preferredPort、fallbackPorts 順序遞補。只接受仍在目前候選清單內且監聽介面相同的記錄；重複候選只嘗試一次。
想強制回到 preferredPort，可暫時停用 rememberLastSuccessfulPort，或從候選清單移除不再允許的舊端口。沒有 fallbackPorts 時自動停用端口記憶，因此既有 Docker 設定不會自行更換容器內端口。Docker 的 host port 衝突發生在 port mapping，應用程式看不到也無法處理；請改 Compose 的 host 端口或先釋放佔用者。若在容器內加入 fallbackPorts，還必須發佈對應的容器端口，否則服務可能監聽在未映射的端口，一般不建議。

每個服務完成初始化後，將 schemaVersion、service、host、lastSuccessfulPort、updatedAt 存到自己的 `dataDirectory/runtime.json`。即使端口記憶停用，也會記錄本次端口供診斷，但不拿它改變下次端口順序。
直接執行服務而未指定 dataDirectory 時，預設使用 `data/服務名/`，避免三服務互相覆寫狀態；Video Node 若偵測到舊 `data/library.sqlite` 則先沿用舊 data。明確指定的 JSON 路徑不會被這個預設取代。
狀態檔不包含 NodeID，也不修改 config；NodeID 仍由 SQLite 保存。不同服務或不同節點必須使用獨立資料目錄。管理頁面「服務資訊」可查看上次端口、實際端口、狀態檔位置與保存結果。
狀態檔不存在時正常啟動；內容損壞、服務身分不符或版本不支援時告警、忽略舊記錄，並保留原檔不覆寫。保存失敗時服務仍可運作，日誌及診斷頁會明確顯示未保存。若需處理損壞檔，先停止服務並保留備份，再由管理者恢復有效狀態。

## 原子設定保存底層

`SaveConfig` 先驗證完整設定，再使用與 runtime.json 共用的保存流程：同目錄暫存檔 → 完整寫入 → 同步 → 關閉 → 替換目標。Windows 使用帶替換與 write-through 旗標的 MoveFileEx，Linux 使用同目錄 rename 並同步目錄。
替換前失敗不截斷舊檔；失敗暫存檔以 `.檔名.pending-*` 留在同目錄供診斷，不自動清理。成功後沒有額外暫存檔。若替換成功後的目錄同步失敗，錯誤會明確表示「檔案已替換」，不會假稱已回復舊檔。
這項底層現在用於管理頁的 `config.override.json` 與 `auth.json`；Docker 唯讀掛載的基準 config 仍由主機管理，不會被網頁修改。保存覆寫後須重新啟動才會套用，沒有熱更新。讀寫中的其他程序、權限、唯讀掛載及檔案系統特性仍可能使替換失敗；不保證遠端／虛擬檔案系統與突然斷電下的所有情境。
