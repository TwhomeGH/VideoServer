# 01：複製影片 ID（純前端）

## 目標

在每張影片卡的操作區加一個「複製 ID」按鈕。點擊後把完整 `video.id` 寫入剪貼簿，
按鈕暫時顯示「已複製」；失敗顯示「複製失敗」。

這題只改前端與翻譯檔，不動 API、SQLite、Dockerfile。

## 前置

```sh
npm run dev
```

用終端機印出的 `[dev] 管理密碼` 登入 `http://localhost:5173`。

> dev 的資料庫和 Docker testing 是分開的。若 dev 沒有影片，可先完成程式與 build，
> 再用 `npm run testing:up` 在 testing image 驗收。

## 會改到的檔案

- `web/admin/src/Library.tsx`
- `web/admin/src/lang/zh_TW.json`
- `web/admin/src/lang/en.json`

`zh_CN.json` 是產生檔，不要手改。

## 1. 加翻譯 key

在 `zh_TW.json` 與 `en.json` 加入：

```json
"library.copyId": "複製 ID",
"library.copiedId": "已複製",
"library.copyFailed": "複製失敗"
```

英文對應 `Copy ID`、`Copied`、`Copy failed`。

## 2. 在 Library 元件加兩個 state

`Library.tsx` 的 `Library()` 內、其他 `useState` 附近：

```tsx
/** 目前顯示「已複製」提示的影片 ID。 */
const [copiedId, setCopiedId] = useState('');

/** 目前顯示「複製失敗」提示的影片 ID。 */
const [copyFailedId, setCopyFailedId] = useState('');
```

不要放進 `data.videos.map()`，那是每張卡各自的 render，不是元件 state。

## 3. 加複製函式

放在 `refresh()` 後面、其他函式之前：

```tsx
async function copyVideoId(video: Video) {
  try {
    await navigator.clipboard.writeText(video.id);
    setCopiedId(video.id);
    setCopyFailedId('');
    window.setTimeout(() => {
      setCopiedId(current => current === video.id ? '' : current);
    }, 2000);
  } catch {
    setCopiedId('');
    setCopyFailedId(video.id);
    window.setTimeout(() => {
      setCopyFailedId(current => current === video.id ? '' : current);
    }, 2000);
  }
}
```

重點：

- `navigator.clipboard` 需要安全來源；`localhost` 可以，區網 HTTP 可能被瀏覽器拒絕。
- `setTimeout` 只清掉同一張卡的提示，不會把別張卡剛複製的狀態蓋掉。
- 成功與失敗分開 state，避免「失敗時仍顯示已複製」。

## 4. 加按鈕

在影片卡操作區、`編輯資料` 按鈕後面：

```tsx
{/* 複製完整影片 ID，方便支援與排查 */}
<button type="button" onClick={() => void copyVideoId(video)}>
  {copyFailedId === video.id
    ? t('library.copyFailed')
    : copiedId === video.id
      ? t('library.copiedId')
      : t('library.copyId')}
</button>
```

JSX 註解要用 `{/* ... */}`；直接寫 `//` 會變成畫面文字。

## 5. 驗證

```sh
npm run lang:build
npm run lang:check
npm run build:admin
```

瀏覽器檢查：

1. 每張卡都有「複製 ID」。
2. 點擊後貼到記事本，內容是完整 `video.id`。
3. 按鈕顯示「已複製」，約 2 秒恢復。
4. 點另一張卡，確認 ID 不會沿用上一張。
5. 切 English／简体中文，文字同步。
6. 390px 沒有水平溢出。

需要 image 驗收時：

```sh
npm run testing:up
```

開 `https://<主機>:18710`，用 `testing/.env` 的 `DVN_ADMIN_PASSWORD` 登入，
到影片庫重測一次。dev 與 testing 的影片資料不共用。

## 常見錯誤

- 只啟動 `npm --prefix web/admin run dev`：只有 Vite，`/api` 會 500；要從根目錄 `npm run dev`。
- state 放在 map 裡：每個 render 都會重建，提示不穩定。
- JSX 用 `//` 當註解：會變成文字。
- 改 `zh_TW.json` 沒跑 `lang:build`：`lang:check` 會說 `zh_CN` 過期。
- 只測成功：`navigator.clipboard` 在非 secure context 會 reject，要確認失敗提示。

## 這題學到什麼

- React 元件 state 的位置與更新。
- 非同步瀏覽器 API 的成功／失敗處理。
- i18n 來源與產生檔的差別。
- `npm run build:admin` 的驗證角色。

## 下一步

目前只有按鈕文字回饋；下一題會把提示抽成全域通知，讓其他操作也能重用。
