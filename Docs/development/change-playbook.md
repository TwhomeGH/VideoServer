# 功能變更食譜

這頁把「一個需求通常要改哪些檔案」列成固定順序。原則是：**資料先穩定，再接 API，最後做 UI 與文案**。

## 1. 只改前端畫面

- `web/admin/src/` 的元件／CSS。
- 文案先改 `web/admin/src/lang/zh_TW.json`；需要英文再改 `en.json`。
- 執行 `npm run lang:build` 重新產生 `zh_CN.json`，再跑 `lang:check`。
- 測試：`npm --prefix web/admin run build`、`lang:check`；有互動時用瀏覽器實測 390px 與桌面寬度。

## 2. 新增／修改 API（不改資料表）

- 後端路由：`internal/library/http.go`（Video Node）或 `internal/runtime/`（跨服務）。
- 授權：管理 API 預設被 auth middleware 保護；公開端點只能明列。
- 契約：`pkg/protocol/openapi.json` 的路徑、schema、required。
- 前端：`web/admin/src/api.ts` 或頁面元件。
- 測試：handler／store 測試；至少 `go test ./...`。

## 3. 新增資料欄位（schema）

先依 [`schema-migrations.md`](schema-migrations.md) 追加 migration，再處理：

1. `internal/library/store.go`：`if version > N`、`if version < N+1`、struct／query scan。
2. `internal/library/videos.go` 或對應檔案：`SELECT` 欄位清單、INSERT 參數。
3. `internal/library/http.go`：API 回應。
4. `pkg/protocol/openapi.json`。
5. `web/admin/src/` 型別與畫面。
6. migration 測試與功能測試。
7. `Docs/ChangeHistory.md`。

## 4. 新增設定

- `internal/runtime/config.go`：`Config`、`Override`、`Apply`、`Empty`、`BuildOverride`、`Validate`。
- `internal/runtime/configapi.go`：`configInput`、必填檢查、`editableConfigFields`。
- `web/admin/src/Settings.tsx`：表單欄位與驗證。
- `pkg/protocol/openapi.json` 的 `Config`。
- 文件：`Docs/configuration.md`；若需要重啟要明寫。
- 測試：`internal/runtime/config_test.go` 的 override／update。

## 5. 掃描、匯入、背景工作

- `internal/library/scanner.go`：掃描與 reconciliation。
- `internal/library/jobs.go`：佇列、工作狀態、hash 限速。
- `internal/library/reprobe.go`：技術資訊與自動補齊。
- `internal/library/cover_queue.go`：封面 worker。
- 事件／UI：SSE 的 `jobs`、`scan`、`config`；`Monitor.tsx`、`ImportJobs.tsx`。
- 失敗路徑要能回報，不要只寫 log；UI 要能重試或看到錯誤。

## 6. WebDAV／遠端來源

- `internal/library/webdav_client.go`：PROPFIND、路徑、ETag。
- `internal/library/webdav_http.go`：TLS／CA／HTTP client。
- `internal/library/remote_import.go`、`remote_probe.go`、`remote_verify.go`：加入書庫、FFprobe、SHA-256。
- `internal/library/http.go`：Range 代理與遠端 API。
- `internal/library/store.go`：pool 的 provider／TLS／CA 欄位。
- 前端：`Library.tsx`、`Discovered.tsx`、`CategoryManager.tsx`（若相關）。
- 注意：不要把帳密寫進 log、URL 或 OpenAPI 範例；自訂 CA 與 skip-verify 互斥。

## 7. 前端與後端的分界

| 類別 | 主要位置 | 不該放哪裡 |
| --- | --- | --- |
| SQL／schema | `internal/library/store.go` | React |
| HTTP 契約 | `internal/library/http.go` + OpenAPI | 只改前端型別 |
| 業務規則 | `internal/library/*.go` | JS 元件 |
| 顯示／互動 | `web/admin/src` | Go handler |
| 設定欄位 | runtime config + Settings | 只寫 config 檔手動改 |

需要兩邊一起改的訊號：新增欄位、API 欄位、設定、篩選、事件、錯誤碼。

## 8. 收尾檢查

- `go test ./... -timeout 120s`
- `go vet ./...`
- `npm run lang:build && npm run lang:check`
- `npm --prefix web/admin run build`
- `npm run test:unit`（有腳本／工具變更時）
- `npm run check:testing`（需要確認目前 image）
- `npm run testing:up`（需要端到端與容器驗證時）
- 更新 `Docs/ChangeHistory.md`；功能文件與 `README.md` 需要時同步。

## 9. 常見錯誤

- 只改 React 型別，忘了 OpenAPI `required`。
- 只改初始 `CREATE TABLE`，沒有寫 migration，舊資料庫升級失敗。
- 改了 zh_TW 沒有 `lang:build`，`lang:check` 會抓到 `zh_CN` 過期。
- 新 API 沒掛 auth middleware，或把敏感欄位放進回應。
- 背景工作失敗只寫 log，UI 永遠停在進行中。
- 測試只跑全新資料庫，沒有測舊資料庫升級。
