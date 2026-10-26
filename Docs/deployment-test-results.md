# Docker 部署驗收紀錄

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
