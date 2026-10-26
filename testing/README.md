# 固定測試工作區

以後要持續做 Docker 測試，就在 **`F:\VideoServer\testing`** 開啟終端機。這裡的 `compose.yaml` 專門用於測試，與專案根目錄的部署環境分開。

## 第一次準備

先在專案根目錄 `F:\VideoServer` 執行：

```sh
npm run setup:test
npm run image -- video-node
npm run image -- central
npm run image -- transcode-worker
```

初始化只建立缺少的檔案，不覆蓋既有設定或影片。

## 每次啟動測試

切換到 `F:\VideoServer\testing`：

```sh
docker compose --profile all up -d --no-build --pull never --wait --wait-timeout 60
docker compose ps
```

這會開啟三服務。只測影片節點時可省略 `--profile all`。

| 服務 | 測試網址 |
|---|---|
| Video Node | http://localhost:18710 |
| Central | http://localhost:18720 |
| Worker | http://localhost:18730 |

區網用主機 IP 取代 localhost，例如 http://192.168.0.102:18710。這些端口與開發模式的 18610／18620／18630 不同，可以並存。
Central 與 Worker 目前只驗證啟動與診斷，尚無索引協作或轉碼派工。

## 測試檔案放哪裡？

```text
testing/
  compose.yaml                  固定的測試啟動檔，納入 Git
  .env                          本機測試外部端口，自動產生，不提交
  local/
    config/
      video-node.json           Video Node 的實際設定
      central.json              Central 的實際設定
      transcode-worker.json     Worker 的實際設定
    media/                      放你要測試的影片，不提交
```

容器將 `local/media/` 看成 `/media`。把影片放進這裡，網頁「儲存池」註冊 `/media`，匯入時填相對檔名。
註冊後不必每部手動匯入：放入新的影片，預設約 30–60 秒會自動加入影片庫。儲存池頁面可查看掃描統計或按「立即掃描新增影片」；手動掃描同樣等待檔案穩定。作者、Tag 與授權等資訊可在自動匯入後編輯。
測試索引與 NodeID 使用獨立 Docker 資料卷 `dvn-testing_node-data`；不會讀取開發模式的 SQLite，也不會讀取根目錄 Compose 的 `dvn_node-data`。
各服務另有獨立資料卷。不要刪除這些 volume，否則會失去測試節點身分與索引。

## 修改設定與繼續測試

- 改 `local/config/服務名.json` 後：`docker compose --profile all restart`。
- 改 `.env` 的端口後：重新執行上面的 up 指令。
- 容器內端口維持 18610／18620／18630，外部測試端口由 `.env` 管理。
- 測試 config 的 fallbackPorts 為空，端口記憶自動停用；runtime.json 僅記錄固定端口供診斷。
- 設定中的 dataDirectory 保持 `/data`，webDirectory 保持 `/app/web`。
- 看目前設定：開啟網頁「服務資訊」，可見設定來源與實際資料目錄。

更新程式時，在專案根目錄重建對應 image，再回到此目錄執行 up。需備份既有測試資料時先停止節點，再備份完整資料卷與 local/config。

停止所有測試服務，資料保留：

```sh
docker compose --profile all stop
```

查看集中日誌：

```sh
docker compose --profile all logs -f
```

如果不想切換目錄，也可在專案根目錄使用 `docker compose --env-file testing/.env -f testing/compose.yaml --profile all up -d --no-build --pull never --wait --wait-timeout 60`。不要在其他目錄直接執行不帶 `-f` 的命令，以免啟動到另一組環境。

`npm run test:deployment` 仍是另開獨立專案的一次性自動驗收；此資料夾則用於持續的人工測試。兩者不共用資料。
