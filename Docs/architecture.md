# DVN v0.1 開發基準

## 技術與責任

- Go + net/http：Central、Video Node、Transcode Worker 三個獨立執行檔。
- React + Vite + TypeScript：共用管理介面，支援 Light / Dark / System。
- SQLite + database/sql + migrations：Video Node MVP 索引；PostgreSQL 後續擴充。
- FFmpeg / FFprobe：本機與遠端共用的轉碼執行能力。
- HTTP JSON + OpenAPI：管理 API、協定版本與能力公告；SSE 事件串流已接入工作、掃描與設定變更；可直接啟用應用程式 TLS，也可由部署層 reverse proxy 處理。
- Windows / Linux / Docker；Valkey 為選配。

Central 負責搜尋、索引與聯邦協調，不承擔所有影片流量。
Video Node 自主管理儲存、影片、播放及政策，不依賴 Central 才能啟動。
Worker 依派工條件轉碼，不能變更影片所有權或放寬政策。

要改程式時的檔案定位、schema migration 與測試順序見 [開發指南](development/README.md)。

未來的 peer 握手與檔案監聽契約分別見 [peer protocol](peer-protocol.md) 與 [file monitoring](file-monitoring.md)；目前尚未實作。

## 識別與儲存

後續資料模型保留 NodeID、VideoID、PublicationID、FileHash、SourceHash、PoolID、ReplicaID、JobID。
檔案雜湊使用 SHA-256；離線儲存池不等於影片已刪除。轉碼輸出保存獨立雜湊及處理資訊。

## 調度

依編碼能力、輸出格式、授權、負載、配額及服務條件篩選 Worker。
依本機優先、遠端優先或平衡策略排序；外部服務不合政策時依設定本機降級或等待。
付費及致謝條件須派工前公告，輸出驗證通過後才提交儲存池。

## 網路與版本

共用協定版本由 `pkg/protocol.Current` 定義，服務提供 `/api/v1/protocol` 能力公告與登入後的 OpenAPI 文件。明確指定不相容版本的 API 請求在操作前拒絕；跨服務連線與啟動時遠端握手尚待 Central／Worker 協作接入。詳見 [API 與協定文件](api/README.md)。

依 preferredPort / fallbackPorts 實際綁定；本機監聽位址不能直接當作已驗證的公開 Endpoint。
預設綁定 `0.0.0.0` 涵蓋所有 IPv4 網卡，可由 host 設定限制單一介面；公開公告仍須使用實際可達位址。
後續加入持久 NodeID、最後成功端口、Endpoint revision 與外部可達性驗證。
Docker 外部映射由部署層處理；容器端口切換不代表外部映射已更新。

Build Identity 包含 service、version、commit、buildTime、channel、dirty、protocolVersion，以及 imageBuildTime、imageSourceDigest。後續握手檢查協定相容性，轉碼任務保存 Node / Worker 當時版本快照。

## Phase 1 實作決策

初版設定採 JSON（Go 標準庫），取代候選 YAML / TOML；需要更多設定能力時再以明確遷移方式擴充。
管理頁可將可編輯設定寫入資料目錄的 `config.override.json`，主機 JSON 保持唯讀基準；套用需要重新啟動。資料目錄與網頁目錄不開放由網頁變更。
管理授權採單一管理員密碼、HttpOnly session cookie 與伺服器端簽章；閒置期限可滑動續期，另有絕對上限。登出或變更密碼會輪替 session secret。
先使用 CSS Variables 實作主題；Tailwind 尚未導入。
本次僅建立程序健康與診斷 API；它們不代表儲存、FFmpeg 或聯邦服務已就緒。

## Phase 2 第一批交付

Video Node 使用 modernc SQLite 驅動（Go 1.26 以上，免 CGO），以交易建立 schema v1、池與影片索引。
NodeID 保存在資料庫，影片目前是一部邏輯影片、一筆發布與一份原始副本的精簡資料模型；多版本／多副本分表將在轉碼與分散式階段遷移。
既有影片以池內相對路徑索引，不搬移原檔；FileHash / SourceHash 記錄匯入當下 SHA-256。
播放前重新檢查路徑邊界、檔案大小與修改時間，來源離線或變更回傳 503，未知影片回傳 404。
管理介面呈現池在線狀態、背景匯入、清單及直接播放。SQLite v4 加入持久匯入工作與 FFprobe 中繼資料；影片庫支援[分頁與搜尋篩選](video-library.md)，轉碼仍待實作。詳見[背景匯入說明](import-processing.md)。

影片描述使用 schema v2：description、category、adult、license。分類與授權先保留可擴充文字，18+ 是內容標記，不能替代後續年齡與存取政策。v1 升級保留既有 ID、雜湊與 NodeID，新增描述預設空值；授權空白不能視為允許轉載。

schema v3 新增 video_tags（標準化 key、顯示名稱、順序及查詢索引）與 video_attribution（作品類型、原作者陣列、來源連結／平台、來源待補、轉載說明與語言）。影片與上述資料以同一交易新增或更新。作品類型預設 unknown，原作者與日後登入使用者分開。連結只保存、不抓取；轉載聲明與授權獨立。推薦系統後續可使用 Tag 索引，仍須優先執行可見性與內容政策。
