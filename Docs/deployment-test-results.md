# Docker 部署驗收紀錄

## 工作進度與排程驗證

2026-10-27（Asia/Taipei）Go 測試確認單工 FIFO、第一部完成後直接處理後續工作，以及來源離線時仍可查詢輕量工作列表。API 契約與 OpenAPI 檢查、前端建置均通過。

瀏覽器重用既有測試容器，以模擬工作回應驗證前端：實測更新間隔約 504–517 毫秒、進度過渡 450 毫秒；FFprobe 階段不含百分比，雜湊階段的畫面在相鄰影格間持續移動。「減少動態效果」設定下過渡為 0 秒。此瀏覽器驗證不代表新增輕量 API 已在舊容器後端部署；真實 API 行為由 Go HTTP 測試驗證。

## Swagger UI 互動文件驗收

2026-10-27（Asia/Taipei）使用 Playwright 操作獨立 Docker 測試環境：登入後開啟「API 文件與測試」，驗證分組與路徑搜尋、參數編輯、GET 影片清單回應 200、修改協定版本後回應 409，以及使用空 JSON 範例 POST 重新探測回應 202。

確認沿用 HttpOnly session、curl 範例與實際回應可見，深／淺色外觀與 390px 窄螢幕下無整頁橫向溢出。瀏覽器請求清單僅有本機服務，無 CDN 或線上規格驗證請求；測試最終頁面無 JavaScript 執行錯誤。

Swagger UI 隨前端打包並延後至文件頁載入；Go 測試、vet、Node 單元測試、前端建置、OpenAPI 格式檢查及 Docker 部署回歸通過。測試容器與瀏覽器使用獨立資源，固定 testing 環境未更新。

## OpenAPI 與協定相容性驗收

2026-10-27（Asia/Taipei）完成獨立 Docker 部署驗收：協定能力可公開讀取、OpenAPI 下載需登入、Video Node 規格包含影片 API。帶 v2 標頭建立儲存池回應 409 與 `protocol_incompatible`，確認資料庫未新增儲存池；使用 v1 後正常匯入、播放、探測、監看及容器重建資料保留皆通過。

Windows 實際服務測試另驗證 Central 規格不含 Video Node API，以及不相容的重啟請求遭拒絕後服務仍可使用。Go 測試、vet、Node 測試、前端建置與 Redocly OpenAPI minimal 檢查皆通過；尚未驗收跨服務握手或 SSE。

## 重新探測與工作監看驗收

2026-10-27（Asia/Taipei）在獨立 Docker 專案執行 `npm run test:deployment` 通過。確認重新探測更新 FFprobe 資訊並保留影片識別、SHA-256、標題、Tag 與作者；來源離線時工作失敗且保留舊技術資訊，恢復後可重試完成。

確認監看與重新探測 API 均需登入，Linux 監看回傳有效 CPU、Go 記憶體、HTTP 收送計數及完整工作狀態；Range／HEAD 播放與容器重建資料保留仍通過。固定 testing 環境未更新。

同批 Windows `go test ./...`、`go vet ./...`、Node 單元測試與前端建置通過。單元測試另覆蓋處理期間修改描述、重複排程、來源異動、完整佇列統計與流量計數。

## 背景匯入與 FFprobe 驗收

2026-10-27（Asia/Taipei）執行 `npm run test:deployment`，以獨立 Docker 專案驗證通過：未登入 API 拒絕存取、登入後建立匯入工作、FFprobe 解析 320×180 H.264/AAC 影片與片長、SHA-256、Range／HEAD 播放、描述編輯、容器重建後資料保留及來源離線恢復。

同批 `go test ./...`、`go vet ./...`、5 項 Node 單元測試、前端 TypeScript／正式建置與 Markdown MD034／MD060 檢查均通過。測試未更新固定 testing 服務。

日期：2026-10-27（Asia/Taipei）

環境：Windows 主機、Docker Desktop Linux Engine、linux/amd64 image。
執行入口：`npm run test:deployment`。本次從原始碼在本機建置，不經 CI。

| 項目 | 結果 |
| --- | --- |
| 本機建置 Video Node 與打包前端 | 通過 |
| 生成兩秒 H.264/AAC MP4 測試素材 | 通過 |
| Compose 外掛 JSON 設定、唯讀影片掛載與資料卷 | 通過 |
| 主機映射端口讀取 API、HTML 與 JavaScript | 通過 |
| 註冊 /media 並匯入中文檔名 | 通過 |
| SHA-256、Tag 去重與原作者資料保存 | 通過 |
| Range 206、HEAD 200 與完整檔案內容比對 | 通過 |
| 編輯資料與外掛 config 指定資料庫子目錄 | 通過 |
| 強制重建容器並驗證 NodeID、儲存池與影片資料保留 | 通過 |
| 來源離線後保留索引、播放回應 503，恢復後重新可用 | 通過 |

本次測試專案：`dvn-test-1793037058315`。
報告與範例資料：`data/deployment-tests/dvn-test-1793037058315/result.json`。
測試容器已停止，資料卷保留供檢查；未使用現有影片庫。

範圍限制：尚未驗證瀏覽器實際解碼／播放器互動、ARM64、原生 Linux systemd 安裝、NAS 斷線與權限、真實網路環境及轉碼。HTTP Range 傳輸通過不代表所有影片編碼均可在瀏覽器播放。

## Phase 1 管理授權、設定覆寫與重啟驗收

2026-10-27 04:30（Asia/Taipei）在 Windows Docker Desktop 與本機開發模式驗證：

| 項目 | 結果 |
| --- | --- |
| 未登入讀取管理 API 回傳 401 | 通過 |
| 首次密碼／`DVN_ADMIN_PASSWORD` 登入與 HttpOnly session cookie | 通過 |
| 管理頁登入、登出、密碼變更 | 通過 |
| 設定頁讀取來源與覆寫、儲存 `config.override.json`、重設回基準 | 通過 |
| `POST /api/v1/system/restart`、exit code 75、Docker 監督重啟 | 通過 |
| 重啟後設定生效、session 仍有效 | 通過 |
| 開發模式 `npm run dev` 監督重啟與 Vite proxy 更新 | 通過 |
| 登入流程下的完整 `npm run test:deployment` 與 `npm run test:images` | 通過 |

測試時將掃描間隔改為 45 秒、重啟後確認生效，再重設回 30 秒並再次重啟；未留下設定覆寫。密碼變更測試後已改回測試用密碼。瀏覽器 localStorage 只有 `dvn-theme`，session token 只存在 HttpOnly cookie。

範圍限制：HTTPS/TLS、多使用者與角色權限、session 個別撤銷（目前登出/改密碼會讓全部 session 失效）尚未實作。

## 自動補齊缺少技術資訊

2026-10-27 在 `dvn-testing` 重建後，未按「重新讀取影片資訊」即出現「已排程缺少技術資訊的影片」日誌；每 10 秒最多排一筆，4 部完成後缺少技術資訊由 176 降至 172，後續背景持續處理。單元測試另驗證一次只排一筆、既有 `video_media` 或 reprobe 記錄不重排，以及離線、大小/修改時間不符的來源會跳過。

## 時間單位選擇與人類可讀顯示驗收

2026-10-27 在 `dvn-testing`（image 來源 `09c633b1d6b1`）驗證：

- 設定頁四個時間欄位顯示 30 秒、6 小時、10 秒、1 分，並提供秒／分／小時／天選單。
- 儲存池顯示「分級掃描：熱 30 秒、暖 5 分、冷 30 分、休眠 6 小時」。
- 監看頁顯示「暖 · 下次約 4 分 44 秒後 · 間隔 5 分」與「休眠掃描間隔：6 小時」。
- 390px 無水平溢出；前端建置、`lang:check`、Go 測試與 `check:testing` 通過。

## 掃描分級與手動提升驗收

2026-10-27 在 `dvn-testing`（image 來源 `a05661c07a85`）驗證：

- 兩個池依 `lastActivityAt` 顯示 warm 分級、間隔 300 秒；`idleMaxSeconds` 為 21600（6 小時）。
- 設定頁休眠掃描間隔值 21600、可輸入範圍 1800–86400（30 分–24 小時）。
- 對單一池按下掃描後回 202，該池由 warm 提升為 hot、間隔 30 秒，`lastActivityAt` 更新；另一個池不受影響。
- 儲存池卡片顯示「暖，間隔 300 秒」；監看頁同步顯示 tier。
- Go 測試、前端建置、`lang:check` 與 `check:testing` 通過。

## 影片工作緊湊清單與分頁驗收

2026-10-27 在 `dvn-testing`（image 來源 `82508eae44b1`）驗證：

- 100 筆工作顯示 20 列，桌面第一列高度 59px，總共 5 頁。
- 篩選列顯示全部 100／處理中 0／失敗 0／完成 100；失敗篩選正確顯示無符合工作。
- 「詳細」可展開路徑與更新時間；390px 為單欄且無水平溢出。
- Go 測試、前端建置、`lang:check` 與 `check:testing` 通過。

## 設定覆寫與補齊用量按鈕區分驗收

2026-10-27 在 `dvn-testing`（image 來源 `702e2905348a`）驗證：

- 設定頁顯示「設定覆寫（進階）／清除設定覆寫」，不再出現「重設為基準」或「主機 JSON」。
- 確認面板明列覆寫檔 `/data/config.override.json` 與基準檔 `/app/config.json`，並註明不影響自動補齊用量、影片或儲存池。
- 監看頁按鈕改名「重設自動補齊今日用量」，確認文字限定在補齊額度。
- 1280 寬度無水平溢出；Go 測試、前端建置、`lang:check` 與 `check:testing` 通過。

## TZ 與自動補齊重置驗收

2026-10-27 在 `dvn-testing`（image 來源 `df524a24e706`）驗證：

- Compose 預設 `TZ=Asia/Taipei`，Go 內嵌 tzdata；`dailyResetAt` 為 `2026-10-27T16:00:00Z`，換算台北時間為隔日 00:00。
- `POST /api/v1/backfill/usage/reset` 未帶 `confirm: true` 回 400；帶確認回 200，今日用量歸零、`limitReached` 為 false。
- 監看頁只在達到每日上限時顯示「重設今日用量」，按下後再顯示確認面板，二次確認才送出。
- 重設只清今天，不影響其他日期、既有工作或候選跳過狀態。
- Go 測試、前端建置、`lang:check` 與 `check:testing` 通過。

## 自動補齊每日上限與佇列閒置驗收

2026-10-27 在 `dvn-testing`（image 來源 `979097139948`）驗證：

- 監看頁顯示「今日 0／50 部 · 0 B／20 GiB」與候選目標；`queueIdle` 為 true 時可排程。
- 設定頁出現每日部數與 GiB 上限，0 表示不限；兩者取先到者。
- 佇列有 queued／running 工作時 `queueIdle` 為 false，候選仍會顯示，但自動補齊不排程。
- 每日用量寫入 schema v6 `backfill_usage`；重置時間依服務時區，Docker 可用 `TZ` 調整。
- Go 測試、前端建置、`lang:check` 與 `check:testing` 通過。

## 自動補齊候選目標提示驗收

2026-10-27 在 `dvn-testing`（image 來源 `7f59876b9f7d`）驗證：

- 監看頁自動補齊區顯示「待檢查候選：7 部」與「下一個待檢查候選：RPReplay_Final1654974526.mp4」。
- `BackfillStatus` 的 `nextVideoId`／`nextVideoTitle` 與影片索引一致。
- 單元測試、Go 測試、前端建置、`lang:check` 與 `check:testing` 通過。

## 工作監看兩欄與每池掃描明細驗收

2026-10-27 在 `dvn-testing`（image 來源 `f6ba2bf0c49d`）驗證：

- 1920 寬度時工作監看為 648px／479px 兩欄，影片工作在左、掃描與補齊在右；四個效能指標並排。
- 掃描區列出「測試影片庫」「測試組2」的下次掃描與退避，名稱由 `PoolScanStatus.name` 提供。
- 390 寬度為單欄且沒有水平溢出。
- Go 測試、前端建置、`lang:check` 與 `check:testing` 通過。

## 每池掃描退避、單池掃描與設定分組驗收

2026-10-27 在 `dvn-testing`（image 來源 `78ed71e6b719`）驗證：

- 兩個閒置池首次掃描後 `backoffSeconds: 60`、`quiet: 1`，下次掃描共用全域最近時間；UI 顯示「下次約 N 秒後（退避 60 秒）」。
- `POST /api/v1/pools/{id}/scan` 回 202；只掃描指定池，該池重置為 30 秒／quiet 0，另一個池仍維持 60 秒。
- 兩張池卡片都有「掃描此池」；390 寬度無水平溢出。
- 設定頁分成「自動掃描與檔案穩定」「自動補齊與 I/O 限速」兩個 fieldset，含閒置退避上限；`lang:check`、Go 測試、前端建置與 `check:testing` 通過。

## 儲存池卡片、Volume 狀態與手動重查驗收

2026-10-27 在 `dvn-testing`（image 來源 `148c756147de`）驗證：

- Docker 池顯示 `filesystem: 9p`、`volumeStatus: unsupported`；前端卡片顯示「Volume GUID：不適用（此環境）」，不再靜默隱藏。
- 在線狀態為小型 pill；1280 與 390 寬度都沒有水平溢出，卡片欄位可換行。
- 按下「重新檢查儲存池資訊」會呼叫 `POST /api/v1/pools/refresh` 並正常回傳兩個池；Docker 的 volume 仍為不適用。
- 掃描說明已拆成「自動檢查／檔案穩定／下次掃描」三行，不再擠在同一段。
- `go test ./...`、`npm --prefix web/admin run build`、`lang:check` 與 `check:testing` 通過。

## Reconciliation diff 驗收

2026-10-27 在 `dvn-testing`（image 來源 `316d1362e1da`）驗證：

- `/api/v1/schedule` 的 `scan.last` 包含 `changed`、`deleted`；穩定測試庫兩者為 0、`skipped` 179，掃描仍未讀取影片內容。
- 儲存池頁的「上次完成」顯示變更／遺失統計；自動補齊預設關閉，下次掃描倒數正常。
- 單元測試會修改既有檔並刪除檔案，確認 changed／deleted 計數、索引與 hash 不變，影片讀取時分別回報 `source_changed`／`source_missing`。
- `go test ./...`、`go vet ./...`、前端建置、`lang:check` 與 `check:testing` 通過。

## Protocol 版本與能力公告驗收

2026-10-27 在 `dvn-testing`（image 來源 `34ce4defbbd7`）驗證：

- `GET /api/v1/protocol` 回傳 `protocolVersion: 1`、`minProtocolVersion: 1`、`maxProtocolVersion: 1`。
- 既有 `capabilities` 名稱與順序保留；新增 `capabilityVersions`，Video Node 的 `library.query` 等能力皆公告版本 1。
- `Negotiate` 與版本範圍測試通過；自動補齊仍預設關閉，測試站下次掃描倒數正常。
- `go test ./...`、`go vet ./...` 與 `check:testing` 通過。

## 儲存池狀態與排程控制驗收

2026-10-27 在 `dvn-testing`（image 來源 `a679d1e2a341`）驗證：

- schema v5 遷移成功；兩個 Docker 儲存池顯示 `filesystem: 9p`，`volumeId` 在容器內為空；停用／啟用 API 與 UI 正常，停用池影片保留索引。
- `GET /api/v1/schedule` 回傳下次掃描約 12 秒、`autoBackfill: false`、候選 16 部、佇列沒有執行中工作；影片工作頁、工作監看與設定頁都顯示排程／開關資訊。
- 設定頁新欄位 `autoBackfill`、`backfillIntervalSeconds`、`hashLimitMiBps` 預設 false／60／64；先前自動補齊已停止，不再持續讀取。
- Go 測試包含 Windows volume GUID 在儲存路徑改變後仍可解析、掃描器使用實際解析路徑、補齊停用／佇列背壓、下次執行倒數與 SHA-256 限速；`go test ./...`、`go vet ./...`、前端建置與 `lang:check` 通過。

## v0.1.0 首次發版本機驗收

2026-10-27 04:00（Asia/Taipei）在 Windows Docker Desktop（buildx 0.38.1、BuildKit docker driver）模擬 `.github/workflows/release.yml` 的 image job。

| 項目 | 結果 |
| --- | --- |
| `npm --prefix web/admin ci` 與前端正式建置 | 通過 |
| `go test ./...`、`go vet ./...` | 通過 |
| `npm run test:unit`（5 項） | 通過 |
| 三個 service 的 linux/amd64 + linux/arm64 OCI 建置 | 通過 |
| OCI index 同時包含 amd64 與 arm64 manifest | 通過 |
| Image 標籤 version=0.1.0、commit、dirty=false、channel=release、建置時間與來源摘要 | 通過 |
| 以 release 參數啟動 linux/amd64 image，`/api/v1/status` 與靜態 HTML | 通過 |

建置來源：commit `ac05dedb69ba7ae325e68e5ab48c8cc250d4fc25`，本地 tag `v0.1.0`。
建置時間：`2026-10-26T19:58:56Z`；來源摘要：`f69c43aac5a38f965ca059682bab3384254821e0801352f53c7a76275ecd0819`。
產物：`data/release-test/video-node-image.tar`、`central-image.tar`、`transcode-worker-image.tar`，各約 26 MB，未提交。

實際 release image 回報：version `0.1.0`、channel `release`、dirty `false`、image build time 與來源摘要一致；容器內 `/api/v1/status` 與管理頁均可讀取。

範圍限制：本次是 Windows 本機模擬，尚未觸發 GitHub Actions 實跑；OCI 產物未推送任何 registry。ARM64 只驗證建置與 manifest，未在 ARM64 主機執行。tag 尚未 push origin。

## 固定測試工作區與自動掃描補測

2026-10-27 在 `testing/compose.yaml` 的 dvn-testing 工作區完成：三服務健康檢查、設定來源 API、範例影片匯入，以及重建 Video Node 後資料保留。
啟用預設每 30 秒掃描、至少 10 秒穩定觀察後，僅將新的 MP4 複製到 `testing/local/media/`，不呼叫匯入或手動掃描 API。背景工作在後續掃描自動建立一筆索引，作品類型保持 unknown，原有手動描述未變更；自動加入的影片 Range 請求回應 206。
單元測試另驗證檔案繼續寫入會重新計算穩定時間、重複掃描不新增重複索引、離線池保留資料與背景工作可取消。

## 最後端口與原子保存補測

2026-10-27 在 Windows 執行完整 Go 測試及實際服務子程序重啟測試：優先端口占用時遞補、下次啟動優先沿用記憶端口、移除候選或停用時不沿用、損壞狀態回到目前設定且保留原檔、原子替換失敗不修改舊檔、無效設定不落盤。確認執行狀態不改寫管理者 config。
Docker Linux 的 dvn-testing 三服務驗證端口記憶停用、固定內部端口與 runtime.json 成功保存；Video Node 重啟可讀回上次端口，NodeID 不變，三份主機設定逐字比對不變。
網頁設定編輯已於後續「Phase 1 管理授權、設定覆寫與重啟驗收」補測；設定熱更新仍不支援，突然斷電情境仍未涵蓋。
