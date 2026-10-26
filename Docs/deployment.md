# Docker、Linux 與正式發版

## 工作分工

本機原始碼可直接建置三個獨立 image，不依賴 GitHub、CI、image registry 或預先編譯的前端。首次建置仍需網路下載官方基底 image 與 Go/npm 相依。
CI 僅在推送 `vX.Y.Z` 正式版本標籤時檢查與建置。目前不會推送 image：發布目的地、帳號與存取範圍尚待確認。也不會在一般 commit / PR 自動發布。
三服務共用 Dockerfile 的建置層，但各 image 只包含自己的執行檔及已打包管理頁面。容器使用 UID/GID 10001，不包含 Go、Node.js、Vite 開發伺服器。

## 本機建置 image

在專案根目錄執行（需要 Docker Engine + Compose，Windows 使用 Linux containers）：

```sh
npm run image -- video-node
npm run image -- central
npm run image -- transcode-worker
node scripts/test-images.mjs
```

這個 npm 入口只用 Node 標準庫呼叫 Docker，不需要先 npm install。它會注入目前 commit、dirty 狀態，產生 `dvn/video-node:local` 等本機標籤，不推送任何 registry。
不想安裝 Node，也可直接：

```sh
docker build --target video-node -t dvn/video-node:local .
```

直接建置若未提供 VCS_REF / DIRTY，版本資訊會明確顯示 unknown / true；正式版本另提供 VERSION / CHANNEL 建置參數。Dockerfile 採多階段建置，依序打包前端、編譯 Go、組裝執行層，使用方式參見 [Docker 官方說明](https://docs.docker.com/build/building/multi-stage/)。

## Compose 啟動

將 `deployments/docker/.env.example` 複製為專案根目錄 `.env`，設定已存在的影片目錄，例如 Windows 的 `DVN_MEDIA_PATH=D:/Videos` 或 Linux 的 `DVN_MEDIA_PATH=/srv/videos`。預設的 `./media` 需自行先建立；來源不存在時明確報錯，不會悄悄建立空目錄。

```sh
docker compose up -d --no-build --pull never
docker compose logs -f
docker compose stop
```

需要同時啟動三服務時：

```sh
docker compose --profile all up -d --no-build --pull never
```

也可使用 `docker compose build` 完全在本機建置；若未設定 DVN_COMMIT / DVN_DIRTY，識別使用 unknown / true，因此建議優先使用上面的 image 腳本。
Video Node 在 `http://主機IP:18610`，Central 為 18620，Worker 為 18630。容器直接提供打包網頁，無需 5173。
容器內端口固定且不遞補；外部占用時修改 `.env` 的 DVN_NODE_PORT 等，再執行 up。Docker 不會因容器內自行換端口就調整映射。
預設對所有 IPv4 介面綁定；可用 DVN_BIND_ADDRESS 指定單一介面。管理 API 仍只適合可信任網路，目前尚無登入授權。

容器內註冊儲存池要填 `/media`，不是 Windows / Linux 主機路徑。現階段唯讀掛載即可索引與播放；日後轉碼輸出會另外規劃可寫掛載。NAS 必須先在主機掛載，再提供容器讀取權限。
SQLite 與 NodeID 保存在 `node-data` named volume。更新 image 前先停止 Video Node，備份完整資料卷及設定，再重新建置與啟動；不應刪除 volume。資料庫升級後回退舊版 image 前須恢復相容備份。Central、Worker 仍為骨架，不代表索引與轉碼派工已實作。

## Linux 原生執行

可直接在 Linux 建置，或在 Windows 交叉編譯。Linux 範例：

```sh
npm --prefix web/admin ci
npm --prefix web/admin run build
GOOS=linux GOARCH=amd64 node scripts/build.mjs video-node
GOOS=linux GOARCH=amd64 node scripts/build.mjs central
GOOS=linux GOARCH=amd64 node scripts/build.mjs transcode-worker
```

Windows PowerShell 可先設定 `$env:GOOS='linux'` 與 `$env:GOARCH='amd64'` 再執行相同建置入口，完成後另開新的終端機恢復一般 Windows 建置環境。ARM64 改為 arm64；輸出位於 `bin/linux-amd64/` 或 `bin/linux-arm64/`，不會覆寫 Windows 執行檔。
部署主機只需執行檔與打包前端，不需要 Go / Node 執行環境。

Linux 管理員準備以下目錄與權限：

- `/opt/dvn/`：放置三個執行檔（具執行權限）；`/opt/dvn/web/` 放置前端 dist 內容。
- `/etc/dvn/`：複製 `deployments/linux/` 的三份 JSON 設定。
- 建立專用 dvn 使用者／群組，授權讀取影片目錄；建議影片位於 `/srv/videos`。
- 將 `deployments/linux/dvn@.service` 安裝至 `/etc/systemd/system/`。

```sh
sudo systemctl daemon-reload
sudo systemctl enable --now dvn@video-node
sudo journalctl -u dvn@video-node -f
```

其他兩服務使用 dvn@central、dvn@transcode-worker。systemd 會建立各自 `/var/lib/dvn/服務名` 資料目錄，重啟／開機自動運作。範本限制寫入系統目錄與讀取 home，請將池放在可讀的 `/srv` 路徑；日後加入轉碼寫入時再明確開放輸出目錄。
systemd 範本尚需在實際 Linux 主機驗證；Docker Linux 啟動測試不等同原生 systemd 安裝測試。

## 正式發版 CI

`.github/workflows/release.yml` 只監聽 vX.Y.Z 標籤，執行 Go 測試／檢查、前端打包及三個 image 的 linux/amd64 + linux/arm64 建置。版本、commit、channel=release、dirty=false 注入 image。
目前 OCI 產物只留在 runner 的暫存路徑，不上傳、不發布，工作結束後不保留。待確認 registry／命名空間／可見性後，再接上登入與推送，以及 Linux 發行檔封裝；本機建置完全不受這項待決策影響。
