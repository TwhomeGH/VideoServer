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
- [ ] 持久 NodeID、最後成功端口與原子設定更新。
- [ ] 授權機制、OpenAPI、SSE 與協定相容性檢查。
- [x] 本機多階段 Docker image 建置、三服務 Compose 與 Linux 原生建置／systemd 範本。
- [x] Compose 外掛 config 與完整部署驗收（影片匯入、傳輸及容器重建後資料保留），拆分新手與進階手冊。
- [x] 僅正式版本標籤觸發的 CI 測試與 image 建置。
- [ ] 確認 registry／命名空間／可見性後接上正式 image 發布。
- [ ] Linux systemd 主機實測、Windows 服務安裝與正式二進位發行封裝。

## Phase 2 — Video Node MVP

- [x] SQLite 初版 schema 遷移、Storage Pool 註冊及離線狀態。
- [x] 本機既有影片匯入、SHA-256、影片與單一原始副本資料模型。
- [x] 影片清單與 HTTP Range 播放。
- [x] SQLite 保存 NodeID，重啟後沿用。
- [x] 內容簡介、分類、18+ 標記、選填授權資訊與 SQLite v2 遷移。
- [x] 匯入後修改描述、Tag 索引、語言、多位原作者與原創／轉載來源聲明。
- [ ] 封面擷取與自訂封面、分類管理及內容標記篩選。
- [ ] 非同步匯入佇列、清單分頁、刪除與重新索引。
- [ ] FFprobe 中繼資料、本機 FFmpeg 轉碼及輸出驗證。

## 後續階段

- [ ] Phase 3：Worker 註冊、能力與條件公告、派工、政策比對、版本快照及本機降級。
- [ ] Phase 4：Central 節點註冊、影片索引、搜尋、Endpoint 更新與來源選擇。
- [ ] Phase 5：多副本、聯邦同步、來源故障切換及跨節點診斷。
- [ ] Phase 6：廣告、公開 Worker 市集、付費條件及推薦統計。
