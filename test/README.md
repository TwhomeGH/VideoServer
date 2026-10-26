# 測試

測試依性質分開：

- `test/unit/`：Node 內建 test runner 的單元測試，對應 `scripts/` 的純函式與工作區初始化。
  ```sh
  npm run test:unit
  ```
- `test/integration/`：會建置 image、啟動容器的流程驗證。
  ```sh
  npm run test:images
  npm run test:deployment
  ```
- Go 測試仍依 Go 慣例放在各套件旁（例如 `internal/auth/auth_test.go`），由 `go test ./...` 執行；不搬到這裡，否則 Go 工具鏈不會自動發現，也無法測試未匯出的套件細節。
