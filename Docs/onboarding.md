# 接手開發與部署：從這裡開始

這份文件是給第一次接手這個專案的人看的入口。它只回答三件事：**這是什麼**、**你現在該從哪裡開始**、**下一步去哪裡看細節**。其他手冊不會在這裡重複。

**最短答案：**

- 想先看到整套系統跑起來：`npm run testing:up`，開 <http://localhost:18710>。
- 要改程式：`npm --prefix web/admin ci`、`npm run dev`，開 <http://localhost:5173>。
- 要正式部署：走 [C. 正式部署](#c-正式部署單一-video-node)。

## 先選一條路徑

| 你的目標 | 走哪條路 | 完成後會看到 |
| --- | --- | --- |
| 修改程式，最快看到畫面 | [A. 本機開發](#a-本機開發) | Vite 5173，Go API 自動代理 |
| 驗證 Docker 打包、磁碟掛載與完整啟動 | [B. Docker 測試環境](#b-docker-測試環境) | 18710／18720／18730 三服務 |
| 正式部署一個 Video Node | [C. 正式部署](#c-正式部署單一-video-node) | 18610 管理頁面 |
| 先了解系統邊界與資料模型 | [架構與目前範圍](#架構與目前範圍) | 知道哪些功能已實作、哪些還是骨架 |

不確定選哪條時：**只改前端或 Go 邏輯先用 A；要確認「打包成 image 後還能跑」用 B；要對外提供服務才走 C。**

## 前置需求

- Go 1.26 以上（SQLite 驅動需求）、Node.js 22.12 以上、npm。
- 走 Docker 路徑時：Docker Desktop 使用 Linux containers，或 Linux 主機的 Docker Engine + Compose。
- 只有執行 `npm run test:deployment` 時才需要 FFmpeg。
- Windows 專案路徑以 `F:\VideoServer` 為例；指令都在專案根目錄執行，除非特別註明。

## 架構與目前範圍

三個服務共用同一個 Go 後端與 React + Vite 管理介面，但負責不同角色：

| 服務 | 目前狀態 | 預設端口 |
| --- | --- | --- |
| Video Node | 已實作儲存池、影片匯入索引、自動掃描與原始檔播放 | 18610 |
| Central | 骨架，只提供總覽與服務資訊 | 18620 |
| Transcode Worker | 骨架，尚無轉碼派工 | 18630 |

管理資料的重點：

- NodeID、儲存池、影片描述與 Tag 存在 SQLite；影片原檔**不複製**，只在原目錄建立索引。
- 轉碼、Central 索引協作、遠端 Worker 派工、登入授權、封面擷取都還沒實作。
- 目前只適合可信任的本機／區網環境。管理 API 的 Origin 檢查不是登入授權。
- 更完整的模型與限制見 [架構文件](architecture.md)、[TODO](../TODO.md)。

## A. 本機開發

適合只改程式、需要熱更新，不想每次重建 image 的情境。

```sh
npm --prefix web/admin ci
npm run dev
```

完成後：

- Go 會建置並啟動 `bin/video-node.exe`（Linux 無 `.exe`），Vite 啟動在 `http://localhost:5173`。
- Vite 的 `/api` 會代理到本次實際啟動的 Go 端口，日誌會列出實際網址；不要假設一定是 18610。
- 修改前端會熱更新；修改 Go 後需按 Ctrl+C 重新執行 `npm run dev`。
- 設定自動放在 `local/development/config/`，資料放在 `local/development/data/`；第一次啟動會建立缺少的檔案，不覆蓋既有設定。
- 要同時開發三服務時用 `npm run dev:all`；Central 與 Worker 目前只有診斷頁。

日常檢查：

```sh
go test ./...
go vet ./...
npm --prefix web/admin run build
```

詳細設定、端口記憶與 runtime.json 見 [設定管理](configuration.md)。

## B. Docker 測試環境

適合在正式部署前確認 image、Compose、磁碟掛載、健康檢查與靜態頁面都正確。**這也是接手後最建議先跑一次的路徑。**

```sh
npm run testing:up
```

這一個指令會：

1. 建立缺少的 `testing/local` 設定與 `.env`（不覆寫既有資料）。
2. 建置三個 image，寫入建置時間、commit、dirty 與來源摘要。
3. 用 `testing/compose.yaml` 啟動三服務並等待健康檢查。
4. 執行 `npm run check:testing` 比對 image 與目前原始碼。

啟動後：

- Video Node：<http://localhost:18710>（區網用主機 IP，例如 <http://192.168.0.102:18710>）
- Central：<http://localhost:18720>
- Worker：<http://localhost:18730>
- 資料在 `testing/local/` 與 Docker volume `dvn-testing_node-data`，與本機開發、根目錄正式部署分開。

只檢查、不建置也不啟動：

```sh
npm run check:testing
```

它會顯示每個 image 的建置時間、來源摘要、容器使用的 image 與 Git HEAD；來源過期、容器仍用舊 image，或 image 缺少版本資訊時會回傳非 0。加上 `-- --json` 可取得結構化結果。

需要一次性的完整 Docker 驗收（需 FFmpeg，會建立獨立 Compose 專案，不動你的測試資料）時：

```sh
npm run test:deployment
```

它會建置 image、產生兩秒範例影片，驗證匯入、播放傳輸、編輯與重建容器後資料保留。

停止與看日誌：

```sh
docker compose -f testing/compose.yaml --profile all stop
docker compose -f testing/compose.yaml --profile all logs -f
```

只有這次沒改程式、想沿用既有 image 時，才在 `testing/` 目錄使用：

```sh
docker compose --profile all up -d --no-build --pull never --wait --wait-timeout 60
```

完整測試工作區說明與常見操作見 [testing/README.md](../testing/README.md)。

## C. 正式部署（單一 Video Node）

只想先跑起一個可管理、可匯入、可播放的節點時，照 [Docker 上手手冊](deployment.md) 的六個步驟。最小指令順序是：

```sh
# 1. 建立 .env，指定既有的影片目錄（Windows 路徑用 /）
#    DVN_MEDIA_PATH=D:/Videos
#    DVN_NODE_PORT=18610

# 2. 建置本機 image（包含後端與管理網頁）
npm run image -- video-node

# 3. 啟動
docker compose up -d --no-build --pull never --wait --wait-timeout 60
docker compose ps

# 4. 開管理頁
# http://localhost:18610
```

部署後資料位置：

| 內容 | 位置 | 更新 image 後 |
| --- | --- | --- |
| 設定 | `deployments/docker/video-node.json`，容器內 `/app/config.json`（唯讀） | 沿用 |
| SQLite、NodeID、影片描述 | Docker volume `dvn_node-data`，容器內 `/data` | 保留 |
| 影片原檔 | `.env` 指定的主機目錄，容器內 `/media`（唯讀） | 保留 |

**不要**刪除 `dvn_node-data` volume，也不要使用 `docker compose down -v`，否則會失去 NodeID 與索引。更新前先停止服務並備份資料卷與設定。

需要多顆磁碟、H 槽或多個影片目錄時，見 [Docker 掛載與多磁碟影片庫](docker-volumes.md)。
需要同時啟動 Central／Worker、Linux systemd 或發版流程時，見 [進階部署參考](deployment-reference.md)。

## 程式從哪裡改

| 位置 | 內容 |
| --- | --- |
| `apps/` | 三個服務的入口，只呼叫 `internal/runtime` |
| `internal/runtime/` | 啟動、設定載入、端口選擇、HTTP 狀態與靜態頁 |
| `internal/library/` | 儲存池、影片索引、掃描、描述與作品來源 API |
| `internal/atomicfile/` | 原子寫入工具 |
| `pkg/buildinfo/` | 版本、commit、dirty 與 image 建置資訊 |
| `deployments/docker/` | 根目錄 Compose 使用的容器設定 |
| `web/admin/` | React + Vite 管理介面；`src/` 是原始碼，`dist/` 是建置產物 |
| `scripts/` | 開發啟動、image 建置、測試環境檢查與工作區初始化 |
| `Docs/` | 操作手冊；行為改變時同步更新 `ChangeHistory.md` |

修改後的檢查順序：

```sh
go test ./...
go vet ./...
npm --prefix web/admin run build
node --test scripts/*.test.mjs
npm run testing:up        # 有動到 Docker、靜態頁或部署路徑時
```

`data/`、`local/`、`testing/local/`、`media/`、`web/admin/dist/`、SQLite 檔與 `.env` 都是本機產物，不要提交。新增行為或修正既有行為時，請在 [ChangeHistory](ChangeHistory.md) 補一筆，說明問題、修改與驗證方式。

## 版本與建置時間怎麼看

管理頁「服務資訊」現在分開顯示兩個時間：

- **程式建置時間**：Go 執行檔的編譯時間。Docker 層快取命中時可能比 image 舊。
- **鏡像建置時間**：image 的建置時間，附來源摘要前 12 碼。

`npm run check:testing` 會拿 image 標籤中的來源摘要與目前工作目錄比對；這是判斷「image 是否還是舊的」的主要依據，不要只看 `docker image inspect` 的 `Created`，快取重建時它可能不會更新。

直接 `docker compose build` 且沒有提供 `BUILD_TIME`／`SOURCE_DIGEST` 時，image 會標示 unknown。要讓版本資訊完整，使用：

```sh
npm run image -- all       # 只建置三個 image
npm run testing:up         # 建置、啟動並檢查測試環境
```

## 資料在哪裡、哪些不能刪

| 環境 | 設定 | 資料 | 影片 |
| --- | --- | --- | --- |
| 本機開發 | `local/development/config/` | `local/development/data/`（或既有 `data/`） | 自行註冊的主機目錄 |
| Docker 測試 | `testing/local/config/` | volume `dvn-testing_node-data` | `testing/local/media/` 或 `testing/compose.local.yaml` 的額外掛載 |
| 正式部署 | `deployments/docker/`、`.env` | volume `dvn_node-data` | `.env` 指定的主機目錄 |

備份時先停止對應服務，再完整備份設定與資料卷；不要只複製 SQLite 主檔而漏掉 WAL 檔。

## 文件地圖

| 文件 | 什麼時候看 |
| --- | --- |
| [Docker 上手手冊](deployment.md) | 第一次正式部署單一 Video Node、日常開關與卡住時 |
| [testing/README.md](../testing/README.md) | 持續人工測試、版本檢查、測試資料放哪裡 |
| [進階部署參考](deployment-reference.md) | 三服務、Linux 原生執行、systemd、正式發版 |
| [Docker 掛載與多磁碟影片庫](docker-volumes.md) | 加入其他磁碟、多個影片目錄、volume 欄位 |
| [設定管理](configuration.md) | 端口、資料目錄、掃描間隔、runtime.json |
| [架構文件](architecture.md) | 資料模型、服務邊界與後續方向 |
| [部署驗收紀錄](deployment-test-results.md) | 已經實測過哪些 Docker 行為 |
| [ChangeHistory](ChangeHistory.md) | 某個設計為什麼變成現在這樣 |
| [TODO](../TODO.md) | 還沒做的功能與已知缺口 |

## 接手檢查清單

第一次接手時，建議依序完成：

1. 讀完本文件與 [架構文件](architecture.md)。
2. 執行 `npm --prefix web/admin ci`、`npm run dev`，確認 5173 可以開啟並看到 Video Node。
3. 執行 `npm run testing:up`，確認 18710 三服務健康，且 `npm run check:testing` 全部相符。
4. 打開 `#diagnostics`，確認 NodeID、程式建置時間、鏡像建置時間與來源摘要都有值。
5. 執行 `go test ./...`、`go vet ./...`、`npm --prefix web/admin run build`。
6. 確認你知道三種環境的資料位置，以及哪些 volume 不能刪。
7. 讀 [TODO](../TODO.md) 與 [ChangeHistory](ChangeHistory.md) 最近幾筆，了解目前刻意留下的限制。

## 卡住時先看這裡

| 現象 | 先檢查 |
| --- | --- |
| 不知道 image 是不是舊的 | `npm run check:testing`；來源摘要不同就重建 |
| 服務資訊的鏡像建置時間是「未提供」 | 用 `npm run image -- all` 或 `npm run testing:up` 重建 |
| Docker 無法連線 | Docker Desktop 是否啟動、是否使用 Linux containers |
| 端口被占用 | 測試環境改 `testing/.env`，正式部署改根目錄 `.env` |
| 註冊儲存池失敗 | 容器內要填 `/media`，不是主機的 `D:/Videos` |
| 可以匯入但不能播放 | 先用 H.264/AAC MP4，確認瀏覽器支援編碼 |
| 更新後資料不見 | 確認沒有刪除 volume 或使用 `down -v`；比對還原後的資料卷 |
| 區網不能開 | 檢查主機 IP、實際外部端口與防火牆 |
