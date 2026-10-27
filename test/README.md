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
  部署驗收結束後依 [保留政策](../Docs/test-retention.md)保留最近 5 組；清理依賴在根目錄以 `npm ci` 安裝。`npm run test:cleanup` 可先唯讀預覽。
- Go 測試仍依 Go 慣例放在各套件旁（例如 `internal/auth/auth_test.go`），由 `go test ./...` 執行；不搬到這裡，否則 Go 工具鏈不會自動發現，也無法測試未匯出的套件細節。

影片背景工作測試包含重新探測、來源變更保護與佇列統計；`internal/diagnostics/` 測試監看取樣及 HTTP 計數。部署驗收使用實際 FFprobe 驗證重新探測、離線後重試與 Linux 監看數據。

`pkg/protocol/` 測試協定相容性、規格內部參照、路由涵蓋與回應欄位一致性。OpenAPI 格式檢查與維護方式見 [API 文件](../Docs/api/README.md)。

Swagger UI 的瀏覽器驗收涵蓋登入沿用、API 分組與搜尋、GET／POST 實際呼叫、版本錯誤、外部連線檢查，以及深／淺色與窄螢幕排版；結果記錄於 [部署驗收](../Docs/deployment-test-results.md)。
