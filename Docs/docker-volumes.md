# Docker 掛載與多磁碟影片庫

本文件說明如何把主機上的既有影片目錄提供給 Video Node。掛載不會複製影片；主機與容器只是使用不同路徑讀取同一份內容。基本概念見 [Docker bind mount 官方說明](https://docs.docker.com/engine/storage/bind-mounts/)。

## 先選對 Compose 檔案

| 環境 | 編輯檔案 | 執行指令的位置 | Video Node 預設端口 |
| --- | --- | --- | --- |
| 固定測試 | `testing/compose.local.yaml`（由 example 複製） | 專案根目錄（`npm run testing:up`）或 `testing/` | 18710 |
| 日常部署 | 根目錄 `compose.override.yaml`（由 example 複製） | `F:\VideoServer` | 18610 |

兩組環境的掛載及資料卷各自獨立。個人掛載檔已列入 `.gitignore`，不會修改或提交版控中的 `testing/compose.yaml`、根目錄 `compose.yaml`。

## 目前三種掛載的用途

| 容器內路徑 | 來源 | 用途 |
| --- | --- | --- |
| /data | node-data Docker 資料卷 | SQLite、NodeID、runtime.json 與影片描述，需可寫 |
| /app/config.json | 主機上的 JSON 設定檔 | 啟動設定，唯讀掛載 |
| /media | 主機影片目錄 | 預設影片池目錄，唯讀掛載 |

測試環境的 `/media` 對應 `testing/local/media/`；根目錄部署的 `/media` 則對應根目錄 `.env` 中的 DVN_MEDIA_PATH。
想換掉根目錄部署的預設影片來源，只需修改 DVN_MEDIA_PATH。想同時使用多個影片目錄，則依下方增加掛載。

## 增加 H 槽影片目錄

### 固定測試環境

個人掛載放在 `testing/compose.local.yaml`，不要修改版控中的 `testing/compose.yaml`：

1. 複製 `testing/compose.local.example.yaml` 為 `testing/compose.local.yaml`。
2. 修改 `source`；範例已包含 `H:/Video` → `/storage/h-video`。
3. 回到專案根目錄執行 `npm run testing:up`，它會自動疊加兩個 compose 檔。
4. 若在 `testing/` 目錄手動執行，指令要同時帶 `-f compose.yaml -f compose.local.yaml`。

`compose.local.yaml` 已列入 `.gitignore`，不會提交。

### 日常部署環境

複製根目錄 `compose.override.example.yaml` 為 `compose.override.yaml`，修改 `source`。Compose 在根目錄執行時會自動疊加 `compose.override.yaml`，不需要額外的 `-f`。

```yaml
      - type: bind
        source: H:/Video
        target: /storage/h-video
        read_only: true
        bind:
          create_host_path: false
```

`compose.override.yaml` 已列入 `.gitignore`，不會提交。

| 欄位 | 意思 |
| --- | --- |
| type: bind | 將主機既有目錄提供給容器 |
| source: H:/Video | 主機上的來源，Windows 對應 H:\Video |
| target: /storage/h-video | Video Node 容器內看到的路徑 |
| read_only: true | 這個掛載不允許容器修改影片原檔 |
| create_host_path: false | 來源不存在時報錯，不自動建立空資料夾 |

`H:\Video` 必須先存在。Windows Compose 路徑建議使用 `/`，容器 target 必須使用 Linux 絕對路徑。
`create_host_path` 是 Compose 的 bind 選項，不是 DVN config；詳見 [Compose volumes 官方欄位](https://docs.docker.com/reference/compose-file/services/#volumes)。

## 還有其他磁碟或目錄

在 `testing/compose.local.yaml` 或 `compose.override.yaml` 的 `video-node` → `volumes` 下，每個目錄再追加一組，target 要各自不同。例如 `I:/Movies` 是示意，請換成你的實際目錄：

```yaml
      - type: bind
        source: I:/Movies
        target: /storage/i-movies
        read_only: true
        bind:
          create_host_path: false
```

建議只掛需要管理的影片目錄，不必掛整個磁碟。不要用 /data、/app 或已使用的 target 作為新影片掛載位置，以免遮住資料庫、程式或原本目錄。
目前只需加在 video-node 的 volumes，Central／Worker 不會自動取得這些掛載。

## 套用並註冊儲存池

固定測試環境在專案根目錄執行：

```sh
npm run testing:up
```

它會自動疊加 `testing/compose.local.yaml`，並在啟動後檢查版本。

日常部署在專案根目錄執行：

```sh
docker compose config --quiet
docker compose up -d --no-build --pull never --wait --wait-timeout 60 video-node
```

Compose 會自動疊加 `compose.override.yaml`。修改掛載要執行 up，讓 Compose 重建容器；單純 restart 不會套用新的掛載。不需重建 image，也不需刪除資料卷。
測試環境若只改掛載、不想重新建置 image，可在專案根目錄執行：

```sh
docker compose -f testing/compose.yaml -f testing/compose.local.yaml up -d --no-build --pull never --wait --wait-timeout 60
```

第一條 `config` 指令只檢查 Compose 語法及變數展開，不代表來源目錄或權限一定有效。

容器啟動後，在網頁分別註冊：

| 儲存池名稱範例 | 網頁要填的目錄 | 主機實際來源 |
| --- | --- | --- |
| H 槽影片庫 | /storage/h-video | H:\Video |
| I 槽電影庫 | /storage/i-movies | I:\Movies |

**掛載只是讓容器看得到目錄，每個來源仍須註冊一次儲存池。** 網頁不要填 H:/Video 或 I:/Movies。
容器看不到主機磁碟的 volume GUID；儲存池的身分是容器內 target。主機盤符改變時，更新 Compose 的 source 並維持相同 target；若 target 改名，池會離線。網頁顯示的檔案系統是容器視角（例如 9p、overlay 或 fuse），不一定是主機的 NTFS。
例如主機的 `H:\Video\旅行\第一天.mp4`，在 H 槽影片庫內的相對路徑是 `旅行/第一天.mp4`。註冊後會定期自動掃描新增影片；描述、Tag 與授權可再於影片庫編輯。
避免把同一批檔案以重疊目錄反覆註冊為多個池；目前不同池會建立各自的索引。

## 讀不到時如何確認

確認容器內是否真的有掛載路徑：

```sh
docker compose exec -T video-node ls -ld /storage/h-video
```

若仍失敗，先檢查修改的是測試還是根目錄 Compose、是否已執行 up、source 是否存在及容器讀取權限。服務使用 UID/GID 10001，Linux 主機目錄需讓該身分可讀且可進入。
NAS／網路映射磁碟／OpenList 虛擬磁碟，即使主機能瀏覽，也要額外確認 Docker Desktop 的引擎能掛載；目前 DVN 不會自動把遠端網址轉成目錄，也尚無原生 WebDAV 儲存池。
卸載或移除已註冊池的掛載會讓原有索引顯示離線，不代表影片已刪除。NodeID 與索引保存在 /data 資料卷，請保留原本 node-data 掛載。儲存池可停用；停用後不掃描、不匯入、不播放，但影片索引與描述會保留，影片會標示所屬池已停用。
