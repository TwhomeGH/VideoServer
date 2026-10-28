# 資料庫 schema 與 migration

Video Node 使用 SQLite。**不要直接修改正式資料庫**；所有結構變更都寫在 `internal/library/store.go` 的 migration 流程。

## `if version > N` 是什麼

```go
if version > 16 {
    return fmt.Errorf("資料庫版本 %d 高於目前支援的版本 16", version)
}
```

這是**降級保護**：如果資料庫已經被新版程式升級到 17，舊程式不應該繼續寫入，因為它不認識新欄位／新表。看到這個錯誤時，應升級程式，而不是刪資料庫。

目前最高是 **16**。新增 migration 時，這個數字要一起變成 17。

## `if version < N` 是什麼

```go
if version < 17 {
    // 新增欄位或資料表
    if _, err := tx.Exec("INSERT INTO schema_version VALUES(17)"); err != nil {
        return err
    }
}
```

這是**向前升級**：新資料庫從版本 0 開始，會依序跑過所有 `version < N` 區塊；舊資料庫只會跑缺少的版本。

規則：

1. **只在最後追加**新的 `if version < N+1`，不要改舊區塊。
2. 同時把上方 `version > N` 的保護數字加一。
3. 新區塊最後要 `INSERT INTO schema_version VALUES(N+1)`。
4. 加法欄位可以用 `columnExists(tx, table, column)` 避免測試或中斷重跑時 duplicate column。
5. 新表用 `CREATE TABLE IF NOT EXISTS`。
6. 需要搬資料時，在同一個 transaction 內完成，失敗要 rollback。
7. 不要只改 `CREATE TABLE` 的初始 schema；舊資料庫不會重跑那段。**migration 才是升級路徑。**
8. 不要修改已發布的 migration 內容；已升級的資料庫不會重跑它。

## 新增欄位的最小範例

```go
if version < 17 {
    exists, err := columnExists(tx, "videos", "example_flag")
    if err != nil {
        return err
    }
    if !exists {
        if _, err := tx.Exec("ALTER TABLE videos ADD COLUMN example_flag INTEGER NOT NULL DEFAULT 0"); err != nil {
            return err
        }
    }
    if _, err := tx.Exec("INSERT INTO schema_version VALUES(17)"); err != nil {
        return err
    }
}
```

同時要改：

- `videos.go` 的 `videoColumns`、`scanVideo`、INSERT 參數。
- `store.go` 的 `Video` struct／JSON。
- 若是查詢條件，改 `query.go`。
- 若是 API 欄位，改 `pkg/protocol/openapi.json` 與前端型別。
- 補一個從舊版 migration 的測試；至少跑 `go test ./internal/library`。

## 哪些情況需要 migration？

| 變更 | 需要？ |
| --- | --- |
| 新欄位、新表、索引、constraint | 要 |
| 只改顯示文字、CSS、React state | 不用 |
| 只改 API 回應格式但資料庫不變 | 不用，但 OpenAPI 要改 |
| 只改掃描邏輯、不新增持久資料 | 通常不用 |
| 把資料搬到新欄位 | 要，含 backfill |

## 測試資料庫版本

```sh
go test ./... -timeout 120s
```

`internal/library` 的 migration 測試會建立舊版資料庫再升級。新增 schema 時應補一個「舊資料庫有既有資料 → 升級後資料仍在」的案例；不要只測全新資料庫。

## 三個版本不要混用

- **schema**：資料表結構，錯誤訊息會寫「資料庫版本」。
- **buildinfo.Version**：產品發行版。
- **protocol.Current**：HTTP／peer 協定。

新增欄位不一定要升產品版；但一定要升 schema，並保留降級保護。
