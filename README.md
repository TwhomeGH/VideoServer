# DVN · Distributed Video Network

可自行部署的分散式影片網路。Go 後端，React + Vite + TypeScript 管理介面。

目前提供三服務基礎與 Video Node 的儲存池、影片匯入索引及原始檔播放；管理頁已有單一管理員登入、設定編輯與重新啟動流程。轉碼、Central 索引及遠端 Worker 尚未實作。

> **第一次接手？先讀 [接手開發與部署：從這裡開始](Docs/onboarding.md)。** 它會帶你選本機開發、Docker 測試或正式部署，並列出下一步要看哪份文件。

## 開發啟動

需求：Go 1.26 以上（SQLite 驅動需求）、Node.js 22.12 以上及 npm。SQLite 不需要額外安裝或 C 編譯器。

在專案根目錄執行：

```sh
npm --prefix web/admin ci
npm run dev
```

單一終端機會建置並啟動 Video Node 與 Vite，自動將 API 代理指向後端實際取得的端口，日誌附上服務名稱。按 Ctrl+C 一次關閉整組服務。修改 Go 後重新啟動，前端保留 Vite 熱更新。
開發模式自動建立並載入 `local/development/config/服務名.json`，修改後重啟即可生效；既有設定不會被覆蓋。設定與資料位置詳見 [設定管理](Docs/configuration.md)。
啟用端口記憶時，下次啟動優先沿用仍在候選清單內的上次成功端口。執行結果另存資料目錄的 `runtime.json`，不改寫 config；Docker 無遞補端口時自動停用記憶，仍維持固定容器端口。
開發與正式建置共用固定執行檔路徑：`bin/video-node.exe`、`bin/central.exe`、`bin/transcode-worker.exe`（Linux 無 `.exe`）。每次啟動不再產生帶程序 ID 的名稱，讓防火牆規則可沿用相同路徑。重新啟動前請先關閉上一組服務，避免 Windows 鎖住正在執行的檔案。
需要同時開發三服務時使用：

```sh
npm run dev:all
```

各服務仍可獨立執行，例如 `go run ./apps/video-node -config configs/video-node.example.json`。單獨執行 Vite 時代理預設為 `127.0.0.1:18610`；整合啟動會覆蓋為本次後端的實際端口，避免端口遞補造成代理失敗。

## 建置與驗證

```sh
npm --prefix web/admin run build
node scripts/build.mjs video-node
node scripts/build.mjs central
node scripts/build.mjs transcode-worker
go test ./...
go vet ./...
```

從專案根目錄啟動 `bin/video-node`（Windows 為 `bin/video-node.exe`）。預設監聽 `0.0.0.0`，接受所有 IPv4 網路介面的連線；使用主機實際 IP 與日誌所列端口瀏覽，例如 `http://192.168.0.102:18610`。`0.0.0.0` 是監聽設定，不是用戶端連線位址。
多張網卡不必逐一綁定；需要限制介面時，將 `host` 設為該介面的 IP，或以 `127.0.0.1` 限制為本機。IPv6 可明確設定 `host` 為 `::`，雙協定行為依作業系統而定。
部署時一併放置 `web/admin/dist`，或用設定中的 `webDirectory` 指向打包目錄；不需要執行 Vite。
三個執行檔互相獨立，預設端口依序為 18610、18620、18630，各有三個遞補候選。

`GET /healthz` 提供程序健康檢查；`GET /api/v1/status` 提供版本與本機監聽資訊。
建置腳本注入時間，Go 自動寫入 commit 與 dirty 狀態；`go run` 無 VCS 識別時顯示 unknown。
各服務可透過 `-ldflags -X` 獨立設定 `pkg/buildinfo.Version` 與 `Channel`。

Vite 開發介面同樣監聽所有 IPv4 介面，其 API 代理仍透過本機連線到 Go 服務。
區網連線需允許防火牆的實際服務端口。管理 API 需先登入；仍只適合可信任的本機／區網環境，尚無多使用者與角色權限。
架構與範圍請見 [開發基準](Docs/architecture.md) 與 [TODO](TODO.md)。

## Video Node 影片庫

重新啟動後，管理頁面提供儲存池與影片庫：

1. 註冊伺服器上的既有目錄，例如 `D:\Videos` 或 `/srv/videos`。
2. 選擇儲存池，輸入池內相對路徑，例如 `旅行/第一天.mp4`。
3. 匯入時計算 SHA-256 並建立 SQLite 索引，原始檔案維持原位。
4. 在影片清單播放；支援 HTTP Range / HEAD，實際編碼需瀏覽器能解碼，尚無自動轉碼。

預設索引與持久 NodeID 保存在 `data/library.sqlite`，可由 `dataDirectory` 調整；備份時請先停止服務再完整備份資料目錄。不要讓不同 Video Node 共用同一資料目錄。
儲存池離線會保留影片紀錄。原始檔案大小或修改時間改變後，該副本會標示不可用，以免提供失準的雜湊與內容。
目前支援 mp4、webm、mkv、mov、avi、m4v 副檔名，透過背景工作執行 FFprobe 中繼資料檢查與 SHA-256 計算。
在「影片工作」查看進度、錯誤及重試；重啟後會恢復未完成工作。既有影片可按「重新讀取影片資訊」補齊片長與編碼，詳見[背景匯入說明](Docs/import-processing.md)。[工作監看](Docs/monitoring.md)整合工作、掃描、程序 CPU、Go 記憶體與 HTTP 流量。影片庫已支援[分頁與搜尋篩選](Docs/video-library.md)；刪除、重新索引與轉碼仍待實作。
已註冊儲存池預設每 30 秒自動檢查新增影片，等待至少 10 秒穩定觀察後背景匯入；儲存池頁面可手動排程與查看結果。已有影片不重複匯入，不會覆蓋你填寫的描述。詳見 [掃描設定](Docs/configuration.md)。

匯入描述欄位包含標題、內容簡介（最多 10000 字）、分類（最多 80 字）、18+ 標記、授權資訊（最多 500 字）。分類可選建議值或自行輸入；授權是發布者提供的文字聲明，留空顯示「未提供」，不能推導轉載權利。18+ 僅為內容標記，尚未實作年齡驗證與存取限制。SQLite v2 會自動升級 v1 並保留原有影片；舊資料描述為空且未標示 18+，不代表通過普遍級審核。

API：`GET /api/v1/library`、`POST /api/v1/pools`、`POST /api/v1/videos/import`、`GET /api/v1/videos/{id}/content`。管理 API 需先登入取得 HttpOnly session cookie；寫入請求仍檢查瀏覽器 Origin。首次啟動的管理員密碼會顯示在服務日誌，也可用 `DVN_ADMIN_PASSWORD` 指定。登入後可在「設定」頁變更密碼、編輯設定與重新啟動。

## Tag 與作品來源

匯入與「編輯資料」共用以下欄位：Tag（逗號分隔，最多 20 個、每個 40 字，忽略大小寫去重）、影片語言、作品類型、最多 20 位原作者及其頁面連結。
作品類型預設「未確認」，可改為「原創」或「轉載」。轉載需填原作者名稱，並提供原始作品連結或勾選「來源待補」；另可填來源平台及轉載說明。填來源不代表取得轉載授權，授權資訊仍獨立保存。
外部連結只接受 HTTP(S)，不由伺服器抓取內容。播放區與清單顯示原作者、作品來源及 Tag；推薦引擎尚未實作。
編輯描述不改變 VideoID、PublicationID、ReplicaID、來源檔案或雜湊。`PUT /api/v1/videos/{id}/metadata` 以完整描述取代現有描述，省略欄位會回到預設值；不是局部更新。
SQLite v3 使用獨立 Tag 索引與作品來源表，會自動升級 v1/v2；既有作品保持「未確認」。封面擷取與自訂封面尚待影片處理模組實作。

## 管理分頁

- `#overview`：影片與儲存池數量、常用入口及服務摘要。
- `#videos`：影片清單、播放器與描述資料編輯。
- `#pools`：儲存池註冊及在線狀態檢查。
- `#import`：池內影片匯入與作品來源表單。
- `#settings`：可編輯設定、設定來源、管理密碼與重新啟動。
- `#diagnostics`：NodeID、建置識別與監聽資訊。

分頁網址可加入書籤，支援重新整理與瀏覽器返回；切換分頁保留當前表單草稿，重新整理頁面則不保存未送出的草稿。Central / Worker 目前只提供總覽與服務資訊。

## 管理授權與登入

管理 API 使用單一管理員密碼。第一次啟動若沒有 `DVN_ADMIN_PASSWORD`，服務會產生隨機密碼並在日誌顯示一次；登入後可在「設定」頁變更。設定環境變數時，該值是啟動時的管理密碼，會覆寫先前由網頁設定的密碼。

登入成功後伺服器設定 `dvn_session` cookie：HttpOnly、SameSite=Strict，瀏覽器 JavaScript 無法讀取，不會把 token 放進 localStorage。使用中的請求會自動延長閒置期限（預設 7 天），但最長 90 天仍需重新登入；登出或變更密碼會輪替簽章金鑰，使既有 session 全部失效。HTTPS/TLS 由部署層負責；直接以 HTTP 提供時 cookie 不會標記 Secure。

API 用戶端需先 `POST /api/v1/auth/login`，再帶著回傳的 cookie 呼叫其他管理 API；`GET /api/v1/status` 與 `/healthz` 保持公開供健康檢查。

## Docker 與 Linux

本機建置 image 使用 `npm run image -- video-node`，無需 CI；三服務共用多階段 Dockerfile，各自獨立部署。Compose 由主機映射外部端口，影片與 SQLite 資料獨立持久保存。Linux 另提供原生交叉編譯與 systemd 範本。首次啟動的管理員密碼在服務日誌，或可用 `DVN_ADMIN_PASSWORD` 指定；登入後「設定」頁可寫入 `/data/config.override.json` 並確認重新啟動。
CI 只處理正式版本標籤；registry 目的地確認前僅檢查及建置，不自動發布。完整步驟見 [部署說明](Docs/deployment.md)。
第一次操作請直接依照 [Docker 上手手冊](Docs/deployment.md) 的六個步驟。進階部署另見 [Linux 與發版參考](Docs/deployment-reference.md)；已驗證的範圍見 [部署驗收紀錄](Docs/deployment-test-results.md)。
持續開發測試建議使用 [testing 工作區](testing/README.md)：根目錄執行 `npm run testing:up` 會建置、啟動並檢查版本；也可先 `npm run setup:test`，再到 `testing/` 使用專用 Compose。`npm run check:testing` 可隨時確認 image 建置時間、來源摘要與執行中容器是否為目前版本。預設外部端口為 18710／18720／18730，資料不與開發或根目錄部署共用。
主機磁碟、容器路徑、volumes 欄位與多影片目錄的設定，見 [Docker 掛載手冊](Docs/docker-volumes.md)。
