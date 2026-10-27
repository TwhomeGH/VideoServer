# 待辦清單 TODO

> [!TIP]
> 未來的新待辦規劃由此集中管理

## Phase 1 — 專案基礎

- [x] 定案 Go 後端與 React + Vite + TypeScript 前端。
- [x] 建立三服務入口、共用 JSON 設定、日誌及優雅關閉。
- [x] 版本資訊、Git commit / dirty 識別、建置時間注入。
- [x] Port 遞補、健康檢查、管理狀態 API。
- [x] 共用管理頁面與 Light / Dark / System 外觀。
- [x] 管理面板拆分總覽、影片庫、儲存池、匯入與服務資訊分頁，支援網址連結及返回。
- [x] 單一開發啟動入口、集中日誌與停止、自動對應遞補後的 API 代理。
- [x] 持久 NodeID（SQLite）、最後成功端口與 runtime.json 原子保存。
- [x] 設定驗證與原子保存底層，替換前失敗保留舊設定。
- [x] 管理頁面設定編輯、授權與套用／重啟流程。
- [x] 開發各服務實際 JSON 設定、設定來源診斷及固定 testing Compose 工作區。
- [x] OpenAPI 現有路由規格、依服務提供文件與契約欄位檢查。
- [x] 內建 Swagger UI 參考與測試頁、API 分組與範例、登入沿用及離線打包。
- [x] 共用協定版本、能力公告與 HTTP 請求相容性檢查。
- [x] SSE 工作／掃描／設定事件、重連與快照同步。
- [ ] 服務間握手與啟動時遠端協定檢查（隨 Central／Worker 協作接入）。
- [x] 三服務工作監看頁、程序 CPU、Go 記憶體、HTTP 流量及 Video Node 工作／掃描狀態。
- [ ] 宿主機／容器完整資源、子程序、網卡監控、跨服務彙整與歷史告警。
- [x] 單一管理員登入、HttpOnly session、設定 API 授權與密碼變更。
- [x] 管理介面 i18n：zh_TW 來源、zh_CN 自動轉換、en、語言註冊器與缺值檢查。
- [x] 本機多階段 Docker image 建置、三服務 Compose 與 Linux 原生建置／systemd 範本。
- [x] Compose 外掛 config 與完整部署驗收（影片匯入、傳輸及容器重建後資料保留），拆分新手與進階手冊。
- [x] 部署測試保留最近 5 組的規劃、來源驗證、預覽與回收／永久清理政策入口。
- [ ] 確認並啟用測試產物永久清理政策（目前系統回收失敗，舊產物仍保留）。
- [x] 僅正式版本標籤觸發的 CI 測試與 image 建置。
- [ ] 確認 registry／命名空間／可見性後接上正式 image 發布。
- [ ] Linux systemd 主機實測、Windows 服務安裝與正式二進位發行封裝。

## Phase 2 — Video Node MVP

- [x] SQLite 初版 schema 遷移、Storage Pool 註冊及離線狀態。
- [x] 本機既有影片匯入、SHA-256、影片與單一原始副本資料模型。
- [x] 影片清單與 HTTP Range 播放。
- [x] 已註冊池遞迴自動掃描新增影片、穩定等待、去重與手動排程，管理頁面定期更新。
- [x] SQLite 保存 NodeID，重啟後沿用。
- [x] 內容簡介、分類、18+ 標記、選填授權資訊與 SQLite v2 遷移。
- [x] 匯入後修改描述、Tag 索引、語言、多位原作者與原創／轉載來源聲明。
- [ ] 封面擷取與自訂封面、分類管理及內容標記篩選。
- [x] 持久背景匯入佇列、重啟恢復、失敗重試及進度管理頁面。
- [x] FFprobe 影片中繼資料檢查與保存。
- [x] 既有影片背景重新探測、SHA-256 一致性驗證、保留描述與失敗重試。
- [x] 缺少技術資訊且無 reprobe 記錄的既有影片自動補齊（一次一筆；離線或失敗不自動重試）。
- [x] 儲存池 volume GUID（原生 Windows）、檔案系統顯示與啟用／停用開關。
- [x] 影片不可用原因（所屬池停用、離線、來源遺失或變更）。
- [x] 下次掃描／補齊倒數、候選數、自動補齊預設關閉、佇列背壓與 SHA-256 讀取限速。
- [x] 儲存池安全掃描預設、全盤確認與系統目錄排除。
- [x] 影片格式 allowlist 依 FFprobe demuxer 驗證；`.ts`／`.mts` 預設關閉並做 MPEG-TS 內容嗅探。
- [x] Index-only 全盤查找模式、找到的影片清單與手動匯入。
- [x] 唯讀 WebDAV 儲存池：PROPFIND 掃描、index-only discovery、憑證檔。
- [x] WebDAV Range 代理播放；播放器不接觸 OpenList 帳密。
- [x] WebDAV 遠端 library 參考、OpenList SHA-256 優先與背景驗證。
- [x] 遠端 FFprobe 技術資訊與背景驗證 hash 限速。
- [ ] 背景驗證進度／取消與 TLS 憑證政策。
- [ ] 儲存池自訂 include／exclude globs。
- [ ] 儲存池 SMART 健康診斷（選配；需平台權限與裝置支援，不影響可用性）。
- [x] 影片庫分頁、標題／簡介搜尋、Tag／分類／儲存池／18+ 篩選與排序。
- [ ] 刪除與重新索引。
- [ ] 本機 FFmpeg 轉碼及輸出驗證。

## 後續階段

- [x] Reconciliation diff：週期掃描統計 new／changed／deleted；changed／deleted 只標狀態，不自動改資料。
- [x] OS watcher 新檔事件提示；debounce 後觸發掃描，9p／整碟池降級輪詢。
- [ ] File identity、rename 配對與 persistent catalog（設計文件完成，待實作）。
- [ ] Central／Worker peer 註冊、heartbeat、lease 與版本／能力協商（設計文件完成，待實作）。
- [x] Protocol 版本範圍、`Negotiate` 與向後相容的 `capabilityVersions`；peer 註冊尚未實作。
- [ ] Phase 3：Worker 註冊、能力與條件公告、派工、政策比對、版本快照及本機降級。
- [ ] Phase 4：Central 節點註冊、影片索引、搜尋、Endpoint 更新與來源選擇。
- [ ] Phase 5：多副本、聯邦同步、來源故障切換及跨節點診斷。
- [ ] Phase 6：廣告、公開 Worker 市集、付費條件及推薦統計。
