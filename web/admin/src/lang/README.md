# 語言資料

- `zh_TW.json`：來源語言，手動維護；UI 文字以繁體中文為準。
- `en.json`：英文，手動維護；必須與 `zh_TW.json` 有相同鍵與插值參數。
- `zh_CN.json`：由 `zh_TW.json` 自動產生，**不要手動修改**。轉換器使用 `opencc-js` 的台灣繁體片語轉換（`twp → cn`），可再以 `zh_CN.overrides.json` 補地區用語。
- `zh_CN.overrides.json`（選填）：`{ "轉換後片段": "修正後片段" }`，在 OpenCC 轉換後套用；只放轉換結果仍不符合用語習慣的片段。

## 指令

```sh
npm run lang:build   # 重新產生 zh_CN.json
npm run lang:check   # 缺鍵、空值、插值參數與 zh_CN 是否過期
```

`lang:check` 也會檢查所有非來源、非產生的 `*.json`；新增語言時會被自動納入缺值檢查，並確認 `registry.ts` 有對應的 import 與 `code`。程式未使用的鍵會列出警告，不會讓檢查失敗。前端 `build` 會先執行這個檢查。

## 新增語言

以日文為例：

1. 建立 `src/lang/ja.json`，鍵與 `zh_TW.json` 相同。
2. 在 `src/i18n/registry.ts` 加入：
   ```ts
   import ja from '../lang/ja.json';
   // locales 陣列中加入
   { code: 'ja', label: '日本語', messages: ja },
   ```
3. 執行 `npm run lang:check`，確認沒有缺鍵或插值參數不一致。

語言選單、瀏覽器語言偵測與 `html lang` 會自動使用 registry；不需要改其他元件。語言檔目前是靜態載入，語言數量很多時再改成動態載入。

## 插值

訊息使用 `{name}` 插值，例如：

```json
"library.filterSummary": "符合 {matching} 部 ／ 影片庫共 {total} 部"
```

所有語言的同名鍵必須使用完全相同的參數集合；`lang:check` 會比對。缺少鍵時執行期回退到 `zh_TW`，開發模式會在 console 警告。

## 範圍

目前只涵蓋管理介面文字。Go 後端回傳的錯誤訊息、OpenAPI 文件內容仍是中文；若要完整多語，後端需要改用錯誤碼或另外的訊息目錄。
