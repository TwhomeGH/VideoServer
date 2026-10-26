# Docker 上手手冊：先跑起一個 Video Node

完成後，你會得到一個可在瀏覽器管理、匯入及播放影片的節點。先只啟動 Video Node；Central 與 Worker 目前仍為骨架，暫時不用一起開。

## 1. 準備

- 啟動 Docker Desktop，使用 Linux containers；Linux 主機使用 Docker Engine 與 Compose。
- 安裝 Node.js 22.12 以上，僅用來執行本機建置入口。
- 準備一個已存在的影片目錄，例如 Windows 的 `D:\Videos`，或 Linux 的 `/srv/videos`。
- 下列指令都在專案根目錄執行。Windows 本專案為 `F:\VideoServer`。

Docker 會自行下載建置工具，不需要在主機安裝 Go，也不用先執行 npm install 或啟動 CI。首次建置需要網路。

## 2. 指定影片目錄

在專案根目錄新增 `.env`。如果已存在，編輯原檔，不要覆蓋其他設定。

Windows 範例，路徑用 `/`：

```dotenv
DVN_MEDIA_PATH=D:/Videos
DVN_NODE_PORT=18610
```

Linux 將第一行改為 `DVN_MEDIA_PATH=/srv/videos`。路徑要改成你的實際目錄，不必把影片複製進專案。

## 3. 建置程式 image

```sh
npm run image -- video-node
```

成功後產生本機 `dvn/video-node:local`，包含後端與管理網頁。這一步只建置，尚未啟動服務，也不會上傳 image。

## 4. 啟動

```sh
docker compose up -d --no-build --pull never --wait --wait-timeout 60
docker compose ps
```

`video-node` 應顯示運作中且健康檢查通過。這一步會建立資料卷保存影片索引與節點身分，並掛載指定的影片目錄。

## 5. 開啟管理網頁

同一台電腦開啟 [http://localhost:18610](http://localhost:18610)。區網其他電腦使用主機 IP，例如 [http://192.168.0.102:18610](http://192.168.0.102:18610)。
若改了 DVN_NODE_PORT，網址也跟著改。Docker 部署不需要開啟 5173，那是前端開發模式的端口。

## 6. 匯入第一部影片

1. 進入「儲存池」，名稱填「我的影片庫」，目錄填 **`/media`**，按「註冊儲存池」。
2. 進入「匯入影片」，選剛建立的儲存池。
3. 填池內相對路徑。例如主機檔案是 `D:\Videos\旅行\第一天.mp4`，這裡填 `旅行/第一天.mp4`。
4. 填寫標題、簡介、Tag 與作品來源等資料，按「匯入影片」。大檔案計算雜湊需要時間，請等待結果。
5. 進入「影片庫」，按「播放影片」或「編輯資料」。

目前直接播放原始檔；建議先以 H.264 視訊、AAC 音訊的 MP4 測試。MKV 等檔案雖可索引，瀏覽器未必能解碼，目前尚無自動轉碼。

## 設定、資料庫、影片在哪裡？

| 內容 | 主機端 | 容器內 | 更新 image 後 |
|---|---|---|---|
| 設定 | `deployments/docker/video-node.json` | `/app/config.json`，唯讀掛載 | 沿用主機檔案 |
| SQLite、NodeID、影片描述 | Docker 資料卷 `dvn_node-data`（預設專案名稱） | `/data` | 保留 |
| 影片原檔 | `.env` 指定的既有目錄 | `/media`，唯讀掛載 | 保留原檔 |
| 程式與網頁 | 本機 image | `/app/service`、`/app/web` | 隨 image 更新 |

一般使用不必改 JSON。要改瀏覽器連線端口，改 `.env` 的 DVN_NODE_PORT 即可；保留容器內端口 18610，才能維持映射與健康檢查一致。
要使用自己的設定檔，可在 `.env` 加入 `DVN_NODE_CONFIG=你的JSON檔案路徑`，檔案必須先存在。設定中的 `dataDirectory` 保持 `/data`，資料才會落在目前的持久資料卷。

## 平常怎麼開、關與更新？

停止服務，資料保留：

```sh
docker compose stop
```

重新開啟：

```sh
docker compose up -d --no-build --pull never --wait --wait-timeout 60
```

修改 JSON 設定後套用：

```sh
docker compose restart video-node
```

修改 `.env` 的端口或掛載路徑後，重新執行上面的 up 指令。
更新程式前先停止服務並備份設定與資料卷；備妥後重新執行步驟 3、4，Compose 會以新 image 重建容器。不要刪除 node-data 資料卷，也不要使用帶 `-v` 的移除指令，否則可能失去節點身分與索引。

## 卡住時先看這裡

| 現象 | 檢查方式 |
|---|---|
| Docker 無法連線 | Docker Desktop 是否已啟動且使用 Linux containers |
| image 找不到 | 先完成步驟 3，確認建置成功 |
| 影片目錄不存在 | 檢查 `.env` 的實際路徑；Windows 改用 `/` |
| 註冊儲存池失敗 | Docker 內填 `/media`，不是 `D:/Videos` |
| 端口被占用 | 修改 `.env` 的 DVN_NODE_PORT，再執行 up |
| 本機可開、區網不能開 | 檢查主機 IP、實際端口與防火牆 |
| 可以匯入但不能播放 | 先用 H.264/AAC MP4，確認瀏覽器支援編碼 |
| 來源離線或變更 | 確認原始檔案仍在原位且未被修改 |

查看日誌：

```sh
docker compose logs --tail 100 video-node
```

目前管理功能尚無登入授權，只適合可信任的本機／區網環境。三服務部署、Linux systemd 與發版安排，見 [進階部署參考](deployment-reference.md)。

## 重跑部署驗收測試

已安裝 Docker、Node.js 與 FFmpeg 時，可執行：

```sh
npm run test:deployment
```

這會建置 image、產生兩秒範例影片，使用獨立 Compose 專案驗證匯入、播放傳輸、編輯與重建容器後資料保留，不掛載你的影片庫。完成後停止測試容器，將報告、設定與影片保留在 `data/deployment-tests/`，測試資料卷也保留供檢查。
重用已建好的 image 可執行 `npm run test:deployment -- --skip-build`。這項測試驗證 HTTP 影片傳輸，不等同實際瀏覽器解碼與播放器操作測試。
