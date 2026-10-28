# 開發指南索引

這份資料夾給「要改程式」的人。目標是回答三件事：**功能應該改在哪裡**、**資料庫／API／UI 怎麼一起動**、**改完怎麼驗證**。

## 先建立心智模型

```text
Browser (React)
  │  fetch /api/v1/*
  ▼
internal/runtime/server.go
  ├─ auth / config / diagnostics / SSE
  └─ Video Node routes
       ├─ internal/library/http.go       HTTP 契約
       ├─ internal/library/store.go      SQLite schema / migration
       ├─ scanner / jobs / reprobe       掃描與背景工作
       ├─ webdav_client / webdav_http   遠端來源
       └─ internal/media                  FFprobe / FFmpeg
```

- `web/admin/src/`：管理介面。所有文案先進 `lang/zh_TW.json`，再產生 `zh_CN`，英文手動維護。
- `internal/library/`：Video Node 的資料、掃描、工作佇列、WebDAV、封面與分類。
- `internal/runtime/`：程序啟動、設定、TLS、服務註冊與 API 掛載。
- `pkg/protocol/`：API 契約、OpenAPI、protocol 版本。
- `deployments/`、`compose.yaml`、`Dockerfile`：部署。

## 我要改什麼？

| 需求 | 先看 | 通常還要改 |
| --- | --- | --- |
| 新增欄位／資料表 | [`schema-migrations.md`](schema-migrations.md) | query struct、API、OpenAPI、測試 |
| 新增管理 API | `internal/library/http.go` | `pkg/protocol/openapi.json`、前端 `api.ts`、測試 |
| 新增設定開關 | `internal/runtime/config.go`、`configapi.go` | `Settings.tsx`、OpenAPI、override 測試 |
| 改掃描／匯入／佇列 | `scanner.go`、`jobs.go`、`reprobe.go` | `store.go` 查詢、SSE、Monitor、測試 |
| 改 WebDAV／TLS | `webdav_client.go`、`webdav_http.go`、`remote_*.go` | pool schema、API、UI、文件 |
| 只有畫面／文案 | `web/admin/src/` | `zh_TW.json`、`lang:build`、前端 build |
| 改部署行為 | `Dockerfile`、`compose*.yaml`、`scripts/` | deployment docs、`check:testing` |

## 先分清楚三個版本

| 名稱 | 位置 | 用途 |
| --- | --- | --- |
| 程式發行版 | `pkg/buildinfo.Version` | 產品版本，例如 `0.1.0` |
| 資料庫 schema | `internal/library/store.go` | SQLite 結構版本，目前最高 **16** |
| API protocol | `pkg/protocol.Current` | 服務／peer 的 HTTP 協定版本 |

三者互不相干。加資料表只會動 schema；不要把它跟 `buildinfo` 或 protocol 混在一起。

## 開發流程

1. 先判斷是「只改 UI」、「API/資料」還是「schema」。
2. 依 [`change-playbook.md`](change-playbook.md) 的順序改：資料 → 服務 → API → 前端 → 文案。
3. 跑 [`testing.md`](testing.md) 的最小檢查，再跑完整檢查。
4. 更新 `Docs/ChangeHistory.md`；需要操作說明的功能同步 `Docs/` 與 `README.md`。
5. 需要部署驗證時跑 `npm run testing:up`；它會建置、啟動並執行 `check:testing`。

## 文件地圖

- 要跑起來／部署：[`../onboarding.md`](../onboarding.md)
- API 與 OpenAPI：[`../api/README.md`](../api/README.md)
- 架構邊界：[`../architecture.md`](../architecture.md)
- 設定：[`../configuration.md`](../configuration.md)
- 測試：[`testing.md`](testing.md)
- Schema：[`schema-migrations.md`](schema-migrations.md)
- 變更食譜：[`change-playbook.md`](change-playbook.md)
