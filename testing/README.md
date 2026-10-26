# 固定測試工作區

> 還不確定該用本機開發、Docker 測試或正式部署？先看 [接手開發與部署：從這裡開始](../Docs/onboarding.md)。

測試用的 `compose.yaml` 位於 **`F:\VideoServer\testing`**，與專案根目錄的部署環境分開。以下指令除非標明切換目錄，都在專案根目錄 `F:\VideoServer` 執行。

## 第一次準備

```sh
npm run testing:up
```

這會建立缺少的 `testing/local` 設定與 `.env`、建置三個 image、啟動並執行版本檢查。既有設定與影片不會被覆寫。
只想先建立工作區、不建置 image 時，執行 `npm run setup:test`。

## 每次啟動測試

用一個指令完成建置、啟動與版本檢查：

```sh
npm run testing:up
```

若這次沒有改程式，只想用既有 image 重啟，切換到 `F:\VideoServer\testing`：

```sh
docker compose --profile all up -d --no-build --pull never --wait --wait-timeout 60
docker compose ps
```

這會開啟三服務。只測影片節點時可省略 `--profile all`。

| 服務 | 測試網址 |
| --- | --- |
| Video Node | [開啟影片節點](http://localhost:18710) |
| Central | [開啟 Central](http://localhost:18720) |
| Worker | [開啟 Worker](http://localhost:18730) |

區網用主機 IP 取代 localhost，例如 [區網測試影片庫](http://192.168.0.102:18710)。這些端口與開發模式的 18610／18620／18630 不同，可以並存。
Central 與 Worker 目前只驗證啟動與診斷，尚無索引協作或轉碼派工。

## 確認目前測試的是哪一版

在專案根目錄執行：

```sh
npm run check:testing
```

檢查會比對 image 的建置時間、來源摘要與目前 Git HEAD，並確認執行中的容器是否仍使用舊 image。輸出範例：

```text
video-node  dvn/video-node:local
  image       sha256:09799d90a664…
  建置時間    2026/10/27 03:38:18
  來源摘要    3185700dea0f（相符）
  Commit      8d4f0ee1a512 · dirty
  容器        275b440b9d24 · running
  容器 image  與目前 image 相符
  判斷        可用
```

- 來源摘要相符：image 由目前工作目錄的內容建置，即使 Git 顯示有未提交變更也是這次的內容。
- 來源摘要不同：先執行 `npm run testing:up`，或 `npm run image -- all` 後再 `compose up`。
- 容器 image 仍是舊 image：重新執行 `compose up` 讓 Compose 重建容器。
- 檢查失敗會回傳非 0，適合在啟動前或自動化流程使用；`npm run check:testing -- --json` 可取得結構化結果。
- 直接執行 `docker compose build` 且未提供 DVN_BUILD_TIME / DVN_SOURCE_DIGEST 時，image 不會有版本資訊；請使用 `npm run image -- all` 或 `npm run testing:up`，服務資訊頁才會有「鏡像建置時間」。

「程式建置時間」是 Go 執行檔的編譯時間，「鏡像建置時間」是 image 的建置時間；Docker 層快取命中時兩者可能不同，屬正常現象。

## 測試檔案放哪裡？

若影片在 H:\Video 等其他位置、不想複製進 local/media，請複製 `compose.local.example.yaml` 為 `compose.local.yaml`，填入自己的來源，再於網頁註冊容器內的 target。`compose.local.yaml` 不會提交，`npm run testing:up` 會自動疊加。

```text
testing/
  compose.yaml                  固定的測試啟動檔，納入 Git
  compose.local.example.yaml    額外磁碟掛載範例，納入 Git
  compose.local.yaml            你的額外磁碟掛載，不提交
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

> 若已建立 `compose.local.yaml`，在 `testing/` 目錄手動執行 `docker compose` 時要加上 `-f compose.local.yaml`，否則不會帶入個人掛載。最簡單的方式是回到專案根目錄使用 `npm run testing:up`。

## 修改設定與繼續測試

- 改 `local/config/服務名.json` 後：`docker compose --profile all restart`。
- 改 `.env` 的端口後：重新執行上面的 up 指令。
- 容器內端口維持 18610／18620／18630，外部測試端口由 `.env` 管理。
- 測試 config 的 fallbackPorts 為空，端口記憶自動停用；runtime.json 僅記錄固定端口供診斷。
- 設定中的 dataDirectory 保持 `/data`，webDirectory 保持 `/app/web`。
- 看目前設定：開啟網頁「服務資訊」，可見設定來源與實際資料目錄。

更新程式時，在專案根目錄執行 `npm run testing:up`；它會重建 image、重建容器並執行版本檢查。只想重建 image 時使用 `npm run image -- all`。需備份既有測試資料時先停止節點，再備份完整資料卷與 local/config。

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
