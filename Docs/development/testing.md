# 測試與驗證

測試分成四層；不是每次都要全部跑，但要依改動範圍選對層級。

## 最小檢查

```sh
go test ./... -timeout 120s
go vet ./...
npm run lang:check
npm --prefix web/admin run build
```

前端 build 會先跑 `lang:check`；Go 測試不會檢查前端型別。

## 什麼改動要跑什麼

| 改動 | 最低驗證 |
| --- | --- |
| 只有文案／CSS | `npm run lang:build`、`lang:check`、前端 build、瀏覽器實測 |
| 前端元件／API client | 前端 build；需要時 `npm run test:unit` |
| Go library／store | `go test ./...`；schema 改動要測舊 DB 升級 |
| runtime／config | `go test ./internal/runtime ./pkg/protocol` |
| 掃描／佇列／FFmpeg | `go test ./...`；需要時 `npm run test:deployment` |
| Docker／Compose | `npm run testing:up`；最後看 `check:testing` |
| CI／release | unit、image build、release workflow 的驗證步驟 |

## 四層

1. **Go package 測試**
   - `internal/.../*_test.go`、`pkg/.../*_test.go`。
   - 適合 store、migration、config、auth、HTTP handler、WebDAV client。
   - 指令：`go test ./... -timeout 120s`。

2. **JS 單元測試**
   - `test/unit/*.test.mjs`，只測不依賴 Docker 的腳本／工具。
   - 指令：`npm run test:unit`。

3. **映像／部署整合**
   - `npm run test:images`：實際 image 啟動與 API 行為。
   - `npm run test:deployment`：完整部署驗收（需要的話會使用 FFmpeg）。
   - 這些會建立容器與測試資料；不要把它們當成無副作用的單元測試。

4. **目前 testing stack**
   - `npm run testing:up`：建置、啟動、等待健康檢查，最後跑 `check:testing`。
   - `npm run check:testing`：只讀檢查 image 標籤、來源 digest、容器 image ID。
   - testing 目前可在本機設定 HTTPS；若憑證缺少，`testing:up` 會先預檢失敗，不會退回 HTTP。

## Schema migration 的測試

新增 migration 時，至少要有：

1. 全新資料庫從 0 升級到目前版本。
2. 舊版資料庫升級後，既有影片／池／工作資料仍存在。
3. 新欄位的 default、nullability、query scan 正確。

不要只測「建立新資料庫」；正式站是舊資料庫升級。

## 前端實測

- 桌面（1440／1920）與 390px 都要確認無水平溢出。
- 檢查 loading、empty、error、無權限／401 狀態。
- 有 dialog 時確認 Esc、背景點擊、關閉後捲動位置。
- 修改 i18n 後切換三種語言，確認沒有缺鍵 console 警告。

## 端到端流程

需要確認「image 裡真的能跑」時：

```sh
npm run testing:up
```

接著用 `https://localhost:18710`（若本機已啟用 testing TLS）或 `http://...`（未設定 TLS 時）操作。完成後保留 `check:testing` 的輸出。
