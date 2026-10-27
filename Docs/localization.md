# 本地化（i18n）

管理介面的文字從 `web/admin/src/lang/` 載入，語言清單註冊在 `web/admin/src/i18n/registry.ts`。切換語言會保存到瀏覽器 `localStorage` 的 `dvn-locale`，並更新 `<html lang>`；瀏覽器語言無法對應時回退到繁體中文。

## 語言檔

| 檔案 | 維護方式 |
| --- | --- |
| `zh_TW.json` | 來源語言，手動維護 |
| `en.json` | 英文，手動維護 |
| `zh_CN.json` | 由 `zh_TW.json` 自動產生，不要手改 |
| `zh_CN.overrides.json` | 選填；轉換後仍不符合用語的片段修正 |

`zh_CN` 使用 `opencc-js` 的台灣繁體片語轉換（`twp → cn`），再套用 `zh_CN.overrides.json`。這是 devDependency，不會打包進瀏覽器。詳細規則見 [`web/admin/src/lang/README.md`](../web/admin/src/lang/README.md)。

## 指令

```sh
npm run lang:build   # 重新產生 zh_CN.json
npm run lang:check   # 缺鍵、空值、插值參數、未定義的 t() 鍵與 zh_CN 過期檢查
```

`lang:check` 會檢查所有非來源、非產生的 `*.json`，所以在 `src/lang/` 新增 `ja.json` 後會自動納入缺值檢查，並確認 `registry.ts` 已有對應的 import 與 `code`。程式未使用的鍵會列出警告。CI 在 `npm --prefix web/admin ci` 後執行同一個檢查；前端 `build` 也會先執行 `lang:check`。

## 新增語言

以日文為例：

1. 建立 `web/admin/src/lang/ja.json`，鍵與 `zh_TW.json` 相同，插值參數也要一致。
2. 在 `web/admin/src/i18n/registry.ts` 加入：
   ```ts
   import ja from '../lang/ja.json';
   // locales 陣列
   { code: 'ja', label: '日本語', messages: ja },
   ```
3. 執行 `npm run lang:check` 與 `npm --prefix web/admin run build`。

語言選單會自動列出 registry 的項目；瀏覽器語言比對也支援新語言的前綴（例如 `ja-JP` 會對應 `ja`）。目前語言檔是靜態載入；語言很多時再改成動態載入。

## 插值與缺值

訊息使用 `{name}` 插值，例如 `{"library.filterSummary": "符合 {matching} 部 ／ 影片庫共 {total} 部"}`。`lang:check` 會檢查所有語言的同名鍵使用相同參數集合，並掃描程式中的 `t('...')` 是否都有對應鍵。缺少鍵時執行期回退到 `zh_TW`；開發模式會在 console 警告。

## 目前範圍

- 已涵蓋管理介面的標題、導覽、表單、按鈕、提示與空狀態。
- Go 後端回傳的錯誤訊息、OpenAPI 文件內容仍是中文；完整多語需要後端錯誤碼或訊息目錄。
- 日期與時間依目前語言格式化，但服務名稱、檔名、路徑等資料原樣顯示。
