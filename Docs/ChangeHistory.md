# ChangeHistory

## 2026.10.27 05:49 新增既有影片重新探測與服務工作監看

**類型**: 功能 · **檔案**: `internal/library/reprobe.go`

### 問題 - 既有影片缺少技術資訊補齊操作，服務工作與資源狀態分散

### 修改 - 沿用持久背景佇列重新探測影片，驗證雜湊並保留描述與識別，失敗可重試

- 三服務提供已授權監看 API 與頁面，整合程序 CPU、Go 記憶體、HTTP 流量及影片工作／掃描狀態
- 明確標示服務程序、HTTP 本文與容器觀測範圍，補齊單元與 Docker 驗收

**相關文件**:

- Docs/monitoring.md
- Docs/import-processing.md

---

## 2026.10.27 05:37 加入影片庫分頁、搜尋與內容篩選

**類型**: 功能 · **檔案**: `internal/library/query.go`

### 問題 - 影片庫每次載入全庫並檢查所有來源，缺少查詢與分頁

### 修改 - 清單 API 預設每頁 24 筆、最多 100 筆，提供筆數與分頁資訊

- 新增標題與簡介搜尋、Tag／分類／儲存池／18+ 篩選、排序及管理頁操作
- 只檢查當頁來源，取消過期查詢並補齊分頁、組合篩選、參數驗證測試

**相關文件**: Docs/video-library.md

---

## 2026.10.27 05:27 加入持久背景匯入與 FFprobe 影片資訊

**類型**: 功能 · **檔案**: `internal/library/jobs.go`

### 問題 - 同步匯入缺少可恢復工作、進度與影片技術資訊

### 修改 - 新增 SQLite v4 工作佇列、重啟恢復、失敗重試與管理頁面

- FFprobe 檢查影片資訊並保存片長、解析度與編碼；Docker 附帶工具
- 補齊操作文件與測試，修正啟動測試 Scanner 讀取錯誤回報

**相關文件**: Docs/import-processing.md

---

## 2026.10.27 04:50 將 Node 單元測試移出 scripts

**類型**: 整理 · **檔案**: `test/`、`package.json`、`.github/workflows/release.yml`、`Docs/onboarding.md`

### 問題 - `.test.mjs` 與建置、啟動、檢查用腳本放在同一個 `scripts/`，不容易區分哪些檔案是測試

### 修改 - 將 Node 單元測試集中到 `test/unit/`、整合流程移到 `test/integration/`，新增 `npm run test:unit`，CI 改用同一個入口

- `test/unit/build-inputs.test.mjs`、`test/unit/workspaces.test.mjs` 的 import 改指向 `scripts/`
- `test/integration/deployment.mjs`、`test/integration/images.mjs` 由 `npm run test:deployment`、`npm run test:images` 執行
- Go 測試仍依 Go 慣例留在各套件旁，由 `go test ./...` 執行
- `test/README.md` 說明兩者的界線與執行方式

---

## 2026.10.27 04:40 新增管理員登入、設定覆寫與重新啟動流程

**類型**: 功能 · **檔案**: `internal/auth/`、`internal/runtime/config.go`、`internal/runtime/configapi.go`、`internal/runtime/server.go`、`web/admin/src/api.ts`、`web/admin/src/Login.tsx`、`web/admin/src/Settings.tsx`、`web/admin/src/main.tsx`、`web/admin/src/Library.tsx`、`scripts/dev.mjs`、`test/integration/deployment.mjs`、`test/integration/images.mjs`、Compose 與部署文件

### 問題 - 管理 API 沒有授權；Docker config 是唯讀單檔掛載，無法安全地從網頁修改設定；設定變更後也沒有套用／重啟流程

### 修改 - 加入單一管理員 session、資料目錄設定覆寫、設定頁與監督式重新啟動

- 首次啟動產生隨機管理員密碼並只顯示一次；可用 `DVN_ADMIN_PASSWORD` 覆寫。憑證以 PBKDF2-HMAC-SHA256 保存於資料目錄 `auth.json`。
- 登入使用 HttpOnly、SameSite=Strict 的 `dvn_session` cookie，不把 token 放進 localStorage；閒置 7 天滑動續期，絕對上限 90 天，登入失敗每個來源 5 次鎖定 30 秒。
- 登入後可變更密碼；登出或改密碼會輪替 session secret，使既有 session 全部失效。
- 管理頁設定寫入資料目錄的 `config.override.json`，主機 JSON 保持唯讀；只保存與基準不同的欄位，可重設回基準。
- 設定頁顯示來源、覆寫檔、資料／網頁目錄與需重啟狀態；確認後呼叫重啟 API。
- 重新啟動以 exit code 75 結束，交由 Docker `restart: unless-stopped`、systemd `Restart=on-failure` 或 `npm run dev` 監督流程重新啟動；直接手動執行時會停止。
- 修正 Vite proxy 的 `changeOrigin`，讓開發模式的登入 Origin/Host 檢查正常。
- 部署驗收、image 測試與文件加入登入 cookie 流程。

---

## 2026.10.27 04:05 完成 v0.1.0 首次發版本機驗收

**類型**: 驗證 · **檔案**: `Docs/deployment-test-results.md`

### 問題 - 首次發版流程尚未在本機完整演練，無法確認雙架構 OCI、release 標籤與實際啟動結果

### 修改 - 依 release workflow 參數建置三個服務的 amd64/arm64 OCI，並啟動 amd64 release image 檢查狀態

- Go 測試、vet、腳本測試、npm ci 與前端正式建置全部通過
- 三個 OCI tar 均為雙架構，version=0.1.0、dirty=false、channel=release 與來源摘要正確
- release image 的 `/api/v1/status` 與管理頁可讀；尚未觸發 GitHub Actions 或推送 registry
- 本地 tag `v0.1.0` 指向 `ac05ded`，尚未 push

---

## 2026.10.27 03:55 新增接手開發與部署入口文件

**類型**: 文件 · **檔案**: `Docs/onboarding.md`、`README.md`、`Docs/deployment.md`、`testing/README.md`

### 問題 - 文件分散在 README、部署手冊、測試工作區與進階參考，新接手者不知道該從本機開發、Docker 測試或正式部署哪一條開始

### 修改 - 新增 onboarding 入口，依目標分流三條路徑並整理文件地圖與交接檢查清單

- 說明前置需求、目前服務範圍與三種環境的資料位置
- 補上程式建置時間、鏡像建置時間與 check:testing 的判讀方式
- README、Docker 上手手冊與 testing 工作區頂部加入入口連結

---

## 2026.10.27 03:45 加入 image 建置時間、來源摘要與測試環境版本檢查

**類型**: 新增 · **檔案**: `scripts/build-inputs.mjs`、`scripts/image.mjs`、`scripts/check-testing.mjs`、`scripts/testing-up.mjs`、`Dockerfile`、`pkg/buildinfo/buildinfo.go`、`web/admin/src/main.tsx`、`compose.yaml`、`testing/compose.yaml`、`testing/README.md`

### 問題 - 本機 compose 啟動時無法從服務資訊判斷 image 是幾點建置、是否由目前原始碼產生；Docker 層快取也讓程式編譯時間與 image 建置時間混在一起

### 修改 - 建置腳本注入建置時間、來源摘要與 OCI 標籤，新增測試環境檢查與一鍵建置啟動工具

- `npm run image -- all` 一次建置三服務，寫入 image 建置時間、commit、dirty 與來源摘要
- 服務資訊分開顯示程式建置時間與鏡像建置時間，並附來源摘要前 12 碼
- `npm run check:testing` 比對 image 標籤、Git HEAD 與執行中容器，過期或使用舊 image 時回傳非 0
- `npm run testing:up` 建置、啟動並自動執行版本檢查
- 直接 `docker compose build` 未帶參數時明確標示 unknown，不假裝有建置資訊
- 正式版本 workflow 同步注入建置時間與來源摘要

---

## 2026.10.27 03:32 加寬管理介面並以對話框播放與編輯影片

**類型**: 改善 · **檔案**: `web/admin/src/style.css`、`web/admin/src/Library.tsx`、`web/admin/src/main.tsx`、`web/admin/index.html`

### 問題 - 固定 1000px 容器與單欄堆疊讓服務資訊、儲存池及匯入表單需要長距離捲動；播放器接在整個影片庫之後，影片變多時必須捲到最底部才找得到

### 修改 - 容器加寬至 1200px，儲存池與服務資訊自動分欄，匯入表單拆成來源與影片資料雙欄，播放與編輯改為原生 dialog 模態

- 服務資訊的生效設定在寬螢幕以雙欄鍵值呈現，避免長路徑斷行
- 播放器與編輯表單覆蓋目前頁面，關閉後保留原本的影片庫捲動位置
- 匯入來源面板在寬螢幕保持可見，窄螢幕自動回到單欄
- 補上 favicon，消除瀏覽器 404 紀錄

---

## 2026.10.27 03:09 修正測試手冊裸網址與表格格式

**類型**: 文件 · **檔案**: `testing/README.md`

### 問題 - 測試服務網址觸發 MD034，部分表格分隔列不符合 MD060 compact

### 修改 - 將三服務與區網測試網址改為具名連結，修正部署手冊兩個表格分隔列

- 使用 markdownlint 驗證 README、Docs 與測試手冊的 MD034 及 MD060 compact 規則

---

## 2026.10.27 03:07 補齊 Docker 多磁碟掛載操作手冊

**類型**: 文件 · **檔案**: `Docs/docker-volumes.md`、`testing/compose.local.example.yaml`、`compose.override.example.yaml`、`.gitignore`

### 問題 - 既有文件僅說明預設 media 掛載，缺少額外磁碟、多組 volumes 及欄位用途的完整引導

### 修改 - 新增本機 compose.local.yaml / compose.override.yaml 疊加範例、多磁碟追加、主機與容器路徑對應、唯讀及 create_host_path 說明

- 個人掛載由 example 複製並列入 .gitignore，不修改版控中的 compose.yaml
- 說明選對 Compose 環境、執行 up 套用掛載、分別註冊儲存池與讀取權限診斷
- 在部署手冊、測試工作區及 README 加入專用手冊連結，表格採 compact 格式

**相關文件**: Docs/docker-volumes.md

---

## 2026.10.27 02:58 保存最後成功端口並建立原子設定寫入底層

**類型**: 新增 · **檔案**: `internal/runtime/state.go`

### 問題 - 服務重啟未沿用上次成功端口，設定與執行狀態缺少完整替換保存機制

### 修改 - 各服務以 runtime.json 記錄成功端口，僅在同介面及現有候選清單內優先沿用，Docker 固定端口自動停用記憶

- 狀態損壞或版本不支援時保留原檔並告警；保存失敗仍可啟動，診斷頁顯示實際狀態
- 新增設定驗證及同目錄暫存、同步、替換底層，不新增網頁寫入 API，不改寫管理者 config
- 三服務未明確指定資料位置時使用獨立預設目錄，舊 Node 資料維持沿用
- 測試占用遞補、實際重啟沿用、替換失敗與損壞恢復，Docker 驗證 NodeID 及 config 保留

**相關文件**: Docs/configuration.md

---

## 2026.10.27 02:31 新增儲存池影片自動發現與穩定掃描

**類型**: 新增 · **檔案**: `internal/library/scanner.go`

### 問題 - 影片必須逐一手動匯入，儲存池無法自動檢查新加入的檔案

### 修改 - Video Node 定期遞迴掃描已註冊儲存池，觀察檔案大小及修改時間穩定後背景匯入

- 提供自動掃描開關、間隔及穩定秒數設定，保留立即掃描、結果統計及錯誤資訊
- 略過已索引檔案與符號連結，不覆寫描述或移除離線紀錄，前端定期更新清單
- 測試穩定等待、寫入中重計時、去重、離線、手動排程與背景週期匯入，更新手冊

**相關文件**: Docs/configuration.md

---

## 2026.10.27 02:14 統一開發設定並建立固定 Docker 測試工作區

**類型**: 新增 · **檔案**: `scripts/workspaces.mjs`

### 問題 - 開發啟動未載入實際 config，缺少可持續操作且與現有資料隔離的測試目錄

### 修改 - 新增 setup:dev 與 setup:test，只建立缺少的設定，開發啟動明確指定各服務 JSON 並保留既有 Node 資料

- 新增 testing/compose.yaml、獨立測試資料卷及 local/media/config，以 18710、18720、18730 持續測試
- 服務啟動記錄設定來源與絕對資料目錄，新增設定診斷 API 及管理頁面生效值展示
- 驗證初始化不覆寫、舊資料沿用、三服務健康、範例匯入與重建後資料保留，補齊設定及測試手冊

**相關文件**:

- Docs/configuration.md
- testing/README.md

---

## 2026.10.27 01:58 將服務入口目錄更名為 apps

**類型**: 修正 · **檔案**: `scripts/dev.mjs`

### 問題 - cmd 資料夾與 Windows 檔案總管輸入 cmd 開啟終端的操作衝突

### 修改 - 三服務入口移至 apps，更新開發與正式建置腳本、Dockerfile 及 README 路徑

- 驗證三服務建置與完整 Go 測試通過

---

## 2026.10.27 01:53 完成 Compose 部署驗收並簡化操作手冊

**類型**: 改進 · **檔案**: `scripts/test-deployment.mjs`

### 問題 - 部署手冊混合建置、設定與多服務細節，首次操作不易跟隨且缺少完整部署驗收

### 修改 - 新增獨立 Compose 測試專案及生成影片，驗證本機建置、外掛設定、匯入、影片傳輸及重建後資料保留

- 三服務 Compose 預設唯讀掛載主機 config，提供自訂設定檔路徑環境變數
- 重寫六步驟上手手冊，分離進階 Linux 與正式發版參考並新增驗收紀錄
- 測試完成後停止專案並保留報告與資料卷，不使用現有影片庫

**相關文件**:

- Docs/deployment.md
- Docs/deployment-test-results.md

---

## 2026.10.27 01:42 新增本機 Docker 建置與 Linux 部署基礎

**類型**: 新增 · **檔案**: `Dockerfile`

### 問題 - 缺少不依賴 CI 的容器建置、部署設定及 Linux 服務管理方式

### 修改 - 新增三個獨立 target 的多階段 Dockerfile、本機 image 腳本、Compose 與資料持久化掛載

- 容器固定內部端口，Compose 設定外部端口；加入非 root 執行、健康檢查及明確建置識別
- 新增 Linux 交叉編譯與 systemd 範本，CI 僅正式標籤觸發檢查與建置，registry 發布待授權
- 本機實際建置三 image，驗證 Linux API、靜態頁面與 Video Node 重啟 NodeID 持久化

**相關文件**: Docs/deployment.md

---

## 2026.10.27 01:33 將 DVN 管理面板拆分獨立分頁

**類型**: 改進 · **檔案**: `web/admin/src/navigation.ts`

### 問題 - 所有管理功能堆疊在同一頁，難以找到操作入口

### 修改 - 新增總覽、影片庫、儲存池、匯入影片、服務資訊導覽與對應頁面

- 總覽呈現摘要與快捷入口，分頁網址支援書籤、刷新及瀏覽器返回
- 切換分頁保留匯入與編輯草稿，版本及 NodeID 集中至服務資訊

---

## 2026.10.27 01:33 新增 Tag、原作者及轉載來源與影片資料編輯

**類型**: 新增 · **檔案**: `internal/library/attribution.go`

### 問題 - 影片缺少推薦分類標記、原作者及原創轉載聲明，匯入後無法修正描述

### 修改 - SQLite v3 新增獨立 Tag 索引與作品來源表，轉載驗證原作者及來源或待補聲明

- 共用匯入與編輯表單，新增多作者連結、來源平台、轉載說明、語言及描述更新 API
- 加入 v2 遷移、Tag 去重、連結驗證、更新原子性與重啟持久化測試

---

## 2026.10.27 01:23 補齊影片匯入描述與內容標記

**類型**: 新增 · **檔案**: `internal/library/metadata.go`

### 問題 - 影片匯入僅有標題，缺少簡介、分類、18+ 與授權資訊

### 修改 - 新增內容簡介、可自訂分類、18+ 標記與選填授權文字，後端驗證欄位長度

- SQLite 升級 v2 保留原有影片與節點識別，加入資料遷移與描述持久化測試
- 更新匯入表單、影片清單及播放資訊；授權未填不推導轉載權利，18+ 尚不提供年齡存取限制

---

## 2026.10.27 01:21 新增 Video Node 儲存池與影片庫第一階段

**類型**: 新增 · **檔案**: `internal/library/store.go`

### 問題 - 服務僅有診斷骨架，尚不能索引或播放本機影片

### 修改 - 新增 SQLite schema v1、持久 NodeID、儲存池註冊及離線狀態，Go 需求提升至 1.26

- 匯入池內既有影片並計算 SHA-256，保留發布及副本識別，支援 Range 與 HEAD 播放
- 新增管理介面儲存池、匯入表單、影片清單與播放器，來源離線或變更時拒絕播放
- 加入持久化、路徑越界、跨站拒絕、取消匯入、來源變更與 HTTP 播放測試，更新 README 及 TODO

**相關文件**: Docs/architecture.md

---

## 2026.10.27 01:11 固定三服務執行檔名稱與路徑

**類型**: 修正 · **檔案**: `scripts/dev.mjs`

### 問題 - 開發啟動使用程序 ID 命名執行檔，導致 Windows 防火牆反覆識別為不同程式

### 修改 - 開發與正式建置共用 bin/video-node、bin/central、bin/transcode-worker 固定路徑，Windows 使用 exe 副檔名

- 建置失敗時補充關閉先前服務的提示並更新啟動文件

---

## 2026.10.27 01:08 調整服務預設端口並防止 Windows 端口共用

**類型**: 修正 · **檔案**: `internal/runtime/listen_windows.go`

### 問題 - 8080 與其他監聽程序共用時，代理請求遭 ECONNRESET

### 修改 - Video Node、Central、Worker 預設改為 18610、18620、18630，保留三個遞補端口並同步範例與代理

- Windows socket 採獨占綁定，避免同一端口被多個程序共用
- 整合啟動在開放前端前驗證 Video Node 狀態 API

---

## 2026.10.27 01:02 新增統一開發啟動與動態 API 代理

**類型**: 修正 · **檔案**: `scripts/dev.mjs`

### 問題 - 手動分別啟動難以管理，後端遞補端口後固定代理導致管理頁面連線失敗

### 修改 - 新增 npm run dev 與 dev:all，集中建置啟動、日誌與關閉

- 從本次後端啟動日誌取得實際端口，覆蓋 Vite 代理；後端就緒後才啟動前端
- 驗證前端打包與代理 API 回應 200；更新 README 與 TODO

---

## 2026.10.27 00:58 調整預設監聽以支援多網卡區網存取

**類型**: 修正 · **檔案**: `internal/runtime/config.go`

### 問題 - 僅綁定 loopback 導致區網 IP 無法連線

### 修改 - 三服務與 Vite 預設監聽 0.0.0.0，涵蓋所有 IPv4 介面並保留指定介面設定

- 更新範例設定及實際連線位址、多網卡與防火牆說明

---

## 2026.10.27 00:50 建立 DVN Phase 1 三服務與管理介面骨架

**類型**: 新增 · **檔案**: `internal/runtime/server.go`

### 問題 - 架構已定案，專案尚無可執行服務

### 修改 - 建立 Go 三服務、設定、Port 遞補、健康與版本 API、結構化日誌及優雅關閉

- 建立 React Vite TypeScript 管理頁面與三種外觀，支援 Go 提供打包靜態資源
- 新增建置腳本、架構基準、開發說明與分階段 TODO；影片功能仍待 Phase 2

**相關文件**: Docs/architecture.md

---

## 2026.10.26 04:57 初始接入ChangelogKit開發變更管理工具

**類型**: 新增

### 修改 - cli測試

接入開發管理工具

---
