# API 與協定 v1

目前採 [OpenAPI 3.1.1](https://spec.openapis.org/oas/v3.1.1.html)，來源為 [openapi.json](../../pkg/protocol/openapi.json)。文件覆蓋現有登入、設定、重啟、監看，以及 Video Node 的影片、儲存池、掃描與背景工作。

登入後開啟「API 文件與測試」（網址分頁 `#api`），即可使用內建 Swagger UI；「服務資訊」也提供入口。原始 `/api/v1/openapi.json` 保留供工具使用。服務回傳嵌入該次建置的規格，不依賴開發目錄或網際網路。Central 與 Worker 的規格會移除 Video Node 專屬路由與空分組，未實作的聯邦與派工不會被公告為可用；SSE 已公告。

## 互動參考頁與測試

文件依服務與版本、登入授權、設定重啟、監看、影片庫、背景工作及播放分組，包含欄位、請求範例、回應格式與錯誤說明。可用搜尋框篩選 API。

1. 展開分組，再展開需要的 API，查看必要欄位與描述。
2. 按 **Try it out**，修改參數或 JSON。影片、工作與儲存池 ID 可從影片庫／工作回應取得。
3. 按 **Execute** 送出，頁面會列出實際網址、curl 範例、狀態碼、回應本文與耗時。

沿用目前瀏覽器的管理 session，不需在 Swagger UI 另外填 cookie。密碼不會寫入文件或持久授權設定。產生的 curl 範例不含 HttpOnly cookie；若在終端執行，需自行登入並保存 cookie 檔。

Execute 會真的操作服務，設定保存、匯入、密碼變更與重啟都會生效。範例中的 ID 與路徑要換成自己的資料；容器路徑和 Windows 主機路徑不同。設定範例以 Video Node 為例，其他服務應先讀取當前設定。

版本標頭預設為 1，可改成 2 測試 409。所有測試限制在目前 origin，不能藉修改伺服器地址把登入請求發往其他網站。401 會回到管理登入畫面；登出測試成功後同樣返回登入。

Swagger UI 隨 Vite 與 Docker 打包，只在開啟此分頁時載入。外部規格驗證關閉，不需要 CDN、外部驗證服務或另開文件伺服器。更新前端依賴後，本機開發先執行 `npm --prefix web/admin ci`；Docker 使用 `npm run testing:up` 更新。

## 版本與能力

| 欄位 | 用途 |
| --- | --- |
| Build Identity 的 `version`／`commit` | 軟體版本與具體程式碼建置 |
| `protocolVersion` | 服務通訊契約的主版本，目前為 1 |
| OpenAPI 的 `info.version` | API 規格文件版本，目前為 1.0.0 |

`pkg/protocol.Current` 是後端協定版本的唯一來源，Build Identity 與 HTTP 公告均使用它。目前只支援 v1，不假設較新或較舊版本一定相容；`Compatible` 對缺少的版本 0 及未知版本回傳 false。

`GET /api/v1/protocol` 不需要登入，回傳目前服務、`serviceType`、`contentTypes`、支援版本、能力識別及規格路徑。例如 Video Node 公告 `contentTypes: ["video"]` 與 `library.query`、`video.probe` 等已實作能力；Central 與 Worker 不公告影片內容能力。回應另含 `minProtocolVersion`、`maxProtocolVersion`、`capabilityVersions`；舊的 `capabilities` 名稱清單保留以維持相容。`protocol.Negotiate` 會取雙方版本範圍的交集最高版；register／heartbeat 與 peer registry 尚未實作，Central 目前沒有自動發現節點。

能力公告只描述功能，不等於節點身分、授權或可被信任。未來 Central／Worker 連線需先取得公告、檢查版本與所需能力，再完成服務身分驗證；這批尚未建立跨服務連線或啟動時探測遠端的流程。

## HTTP 相容性檢查

API 回應包含 `X-DVN-Protocol-Version: 1`。用戶端可在請求帶同名標頭選擇版本；管理前端已固定宣告其支援的版本 1。

| 請求標頭 | 結果 |
| --- | --- |
| 未提供 | 沿用目前版本，保留舊用戶端行為 |
| `1` | 相容，繼續既有授權與處理 |
| `2` 等未知正整數 | 409，`code: protocol_incompatible` |
| 空值、0、負數、前導零、非數字或重複標頭 | 400，`code: invalid_protocol_version` |

版本檢查在管理操作執行前完成，不相容請求不會建立工作、修改設定或重啟。此檢查先於登入 middleware，但不會授予任何管理權限；版本相容後仍需登入。靜態資源及 `/healthz` 不套用版本選擇。

保留不帶標頭的行為只是目前管理用戶端的相容措施。未來服務間連線不能因對方沒公告版本而推定相容，也不應在版本不符後悄悄移除標頭重試寫入。

## 授權與錯誤

公開端點為 `/healthz`、`/api/v1/status`、`/api/v1/protocol`、登入與 session 查詢。其他管理 API 使用 `dvn_session` HttpOnly cookie。OpenAPI 下載同樣需要登入。

寫入請求依端點檢查 `Origin`，有提供時必須與 Host 相符；目前不提供跨站 CORS。影片播放仍使用管理 session，尚未提供面向觀眾的公開播放授權。

既有 JSON 錯誤保留 `{ "error": "說明" }`。版本錯誤另含 `code`、`protocolVersion`、`supportedVersions`，讓用戶端可區分格式錯誤與版本不相容。未知路由、方法不符及部分 HTTP Range／條件請求錯誤可能回傳純文字，不能假設所有錯誤都有 JSON。

## 驗證與維護

`go test ./...` 會檢查協定拒絕是否發生在操作之前、依角色提供的規格、所有明確註冊路由是否有文件、回應型別的 JSON 欄位與 required 是否一致，以及本機 `$ref` 是否能解析。

OpenAPI 格式檢查可執行：

```sh
npx --yes @redocly/cli lint pkg/protocol/openapi.json --extends=minimal
```

新增或修改 API 時，需同步更新規格、能力公告及契約測試。新增可選欄位通常可保留協定 v1；改變欄位語意、移除欄位、改動必要授權／工作流程等破壞相容性的變更，須先安排新協定與遷移策略。

`POST /api/v1/pools` 支援本機目錄與 WebDAV：本機用 `{ "name", "path" }`；WebDAV 用 `{ "name", "provider": "webdav", "url", "username", "password", "root" }`，其中 `root` 是 `/dav` 下的子目錄。WebDAV 池固定 index-only，遠端匯入尚未支援。回應不會包含密碼。

## Index-only 找到的影片

`GET /api/v1/discovered` 列出 index-only 掃描找到、尚未進入影片庫的檔案，支援 `poolId`、`q`、`page`、`pageSize`。`POST /api/v1/discovered/import` 接受 `{ "items": [{ "poolId", "relativePath" }] }`，一次最多 100 筆，會建立手動匯入工作；匯入成功後該項目會從清單消失。這兩個端點都需要登入。

## SSE 事件串流

登入後可連線 `GET /api/v1/events`，使用同一個 HttpOnly session cookie。端點只推送變更通知，不包含完整資料：

- `jobs`：匯入／重新讀取工作新增、開始、進度、完成或失敗；雜湊進度最多每秒一次。
- `scan`：儲存池掃描開始與完成。
- `config`：設定儲存、覆寫重設或重新啟動請求。
- `ready`：連線建立，附帶目前事件編號。
- `reset`：需要重新取得快照（例如重播緩衝不足或伺服器重啟）。

事件 `data` 目前是空物件；前端收到後以既有 API 取得最新快照。首次 `ready`、`reset` 與重連後的重播事件都會觸發對應頁面更新。伺服器保留最近 512 個事件，瀏覽器重連時以 `Last-Event-ID` 重播；若落差太大就送 `reset`。心跳每 15 秒、連線最長 30 分鐘後由 EventSource 重連並重新驗證 session。SSE 不可用或斷線時，既有輪詢仍會運作。

實作位置：後端 `internal/events`、`/api/v1/events`，前端 `web/admin/src/live.tsx` 與各頁的版本觸發。事件只在記憶體中保存，服務重啟後編號重新開始，前端會以 `reset` 重新同步。

