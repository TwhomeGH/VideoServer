# OpenList／WebDAV 儲存池

OpenList（AList fork）可以把多種後端（本機、SMB、雲端）整理成同一個檔案樹。DVN 目前以 **WebDAV** 讀取它，不直接把 OpenList 的 SMB 後端當成 DVN 磁碟。

## OpenList 端

1. 確認 OpenList 已啟動，Web UI 通常是 `http://<host>:5244`。
2. WebDAV 端點是 `http://<host>:5244/dav`。
3. 建立一個給 DVN 使用的使用者，或使用現有 guest；權限只需讀取要索引的目錄。
4. 若要讓 DVN 只看某個目錄，記下它的路徑，例如 `Video5`。

先用 curl 驗證：

```sh
curl -u USER:PASS -X PROPFIND "http://HOST:5244/dav/" -H "Depth: 1"
curl -I -u USER:PASS "http://HOST:5244/dav/Video5/example.mp4"
curl -D - -o NUL -u USER:PASS -H "Range: bytes=0-1023" \
  "http://HOST:5244/dav/Video5/example.mp4"
```

`PROPFIND` 應回 207；影片 `HEAD` 應有 `Content-Length` 與 `Accept-Ranges: bytes`，Range 應回 206。

## DVN 端

在「儲存池 → 新增儲存池」選 **WebDAV（僅索引）**，填：

- WebDAV URL：`http://HOST:5244/dav`
- 子目錄：例如 `Video5`（留空代表整個 `/dav`）
- WebDAV 使用者／密碼

儲存後 DVN 會用 `PROPFIND` 遞迴列出支援的影片，寫入「找到的影片」，**不 hash、不建立 import job**。遠端池固定為 index-only，掃描間隔至少 5 分鐘。

「找到的影片」的 WebDAV 項目可按「播放」，由 DVN 代理 WebDAV Range 請求；瀏覽器只看到 DVN 的 `/api/v1/discovered/content`，不會取得 OpenList 帳密。支援 `Range` 與 `HEAD`，可直接回 206。

遠端項目可按「加入書庫」建立 library 參考，不下載副本；加入後會排入遠端 FFprobe，取得片長／解析度／編碼。DVN 會嘗試從 OpenList 的 `hash_info` 取得 SHA-256；有合法值就標記已驗證，否則標記未驗證。未驗證影片可手動按「驗證 SHA-256」，DVN 會建立背景 verify job，以 16 MiB HTTP Range 分塊讀取，邊讀邊計算，不落地保存，並套用 `hashLimitMiBps`。開始前後會用 size、ETag、Last-Modified 檢查來源是否變更；UI 顯示進度並可取消。

目前遠端池限制：

- 加入書庫是遠端參考，不下載副本；OpenList 離線時影片不可用。
- OpenList 不一定提供 SHA-256；沒有時需手動背景驗證，驗證會讀取完整檔案。
- 取消只在目前分塊結束後生效（最多一個 16 MiB 分塊）。
- 遠端 FFprobe 由 Video Node 直接讀取 WebDAV；憑證只放在 FFprobe 參數，不寫入日誌。
- 不啟用 OS watcher，完全依賴輪詢。
- Basic Auth；正式環境請用 HTTPS。
- 憑證存在 `<dataDirectory>/webdav-secrets.json`（0600），不會出現在 pool API 回應。
- 只支援 WebDAV；OpenList 的 SMB 後端仍由 OpenList 自己掛載，DVN 不直接連 SMB。
- 播放是即時代理，不落地快取；流量經過 Video Node。

## Docker 與網路

Video Node 容器必須能連到 OpenList。若 OpenList 在 Docker 主機上，容器可用 `host.docker.internal`；若在區網其他機器，直接用 LAN IP。不要期待 Windows 的 WebDAV 磁碟一定可以 bind 給 Docker Desktop；原生 WebDAV provider 就是為了避開這個問題。

## 後續

- 遠端檔案身分、etag／mtime 與 rename 策略。
- 可設定的遠端掃描間隔與 TLS 憑證政策。
