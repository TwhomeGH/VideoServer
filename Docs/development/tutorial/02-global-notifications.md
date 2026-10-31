# 02：全域通知元件

> [!TIP]
> 這個功能已完成並在 `main` 驗收；本文是實作回顧與模板，不是待辦。
> 目標是理解 Context、hook、計時器 cleanup 與跨頁通知的設計。

> [!WARNING]
> 請在個人分支／worktree 操作。表單驗證錯誤仍應留在表單旁，不要全部改成 toast。

## 目標

建立可重用的全域通知：

- `notify.success()`／`notify.error()`／`notify.info()`
- 成功與一般通知 3 秒消失；錯誤 5 秒
- 可同時顯示多個、可手動關閉
- 卸載時清除所有 timer
- 右下角顯示，手機不溢出

完成後把複製 ID 的成功／失敗接到通知。

## 練習基準

從 Learn 1 完成後的 commit 開始：

```powershell
git worktree add -b tutorial/02-notifications ../VideoServer-tutorial-02 3b9e35b
cd ../VideoServer-tutorial-02
npm --prefix web/admin ci
npm run dev
```

worktree 的通用用法見 [`git-worktree.md`](../git-worktree.md)。

## 會改到的檔案

- `web/admin/src/notifications.tsx`（新增）
- `web/admin/src/main.tsx`
- `web/admin/src/Library.tsx`
- `web/admin/src/style.css`
- `web/admin/src/lang/zh_TW.json`
- `web/admin/src/lang/en.json`

## 1. 建立 Provider 與 hook

`notifications.tsx` 的核心是：

```tsx
const NotificationContext = createContext<Notify | null>(null);

export function NotificationProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<Toast[]>([]);
  const timers = useRef(new Map<number, number>());
  const nextId = useRef(0);

  const dismiss = useCallback((id: number) => {
    const timer = timers.current.get(id);
    if (timer !== undefined) {
      window.clearTimeout(timer);
      timers.current.delete(id);
    }
    setToasts(current => current.filter(item => item.id !== id));
  }, []);

  const push = useCallback((message: string, tone: Tone) => {
    const id = ++nextId.current;
    setToasts(current => [...current, { id, message, tone }]);
    const ttl = tone === 'error' ? 5000 : 3000;
    timers.current.set(id, window.setTimeout(() => dismiss(id), ttl));
  }, [dismiss]);

  useEffect(() => () => {
    for (const timer of timers.current.values()) window.clearTimeout(timer);
    timers.current.clear();
  }, []);

  const notify = useMemo<Notify>(() => ({
    success: message => push(message, 'success'),
    error: message => push(message, 'error'),
    info: message => push(message, 'info'),
  }), [push]);

  return (
    <NotificationContext.Provider value={notify}>
      {children}
      <div className="toast-stack" aria-live="polite">
        {toasts.map(item => (
          <div
            key={item.id}
            className={`toast toast-${item.tone}`}
            role={item.tone === 'error' ? 'alert' : 'status'}
          >
            <span>{item.message}</span>
            <button
              type="button"
              aria-label={t('notifications.dismiss')}
              onClick={() => dismiss(item.id)}
            >
              ×
            </button>
          </div>
        ))}
      </div>
    </NotificationContext.Provider>
  );
}

export function useNotify(): Notify {
  const value = useContext(NotificationContext);
  if (!value) throw new Error('useNotify 必須在 NotificationProvider 內使用');
  return value;
}
```

`push` 的依賴要有 `dismiss`；`useEffect` 的 cleanup 只做計時器清除，不要在那里 setState。

## 2. 接到 App

`main.tsx` 只能有一個 `createRoot`，而且 Provider 要在 `I18nProvider` 內：

```tsx
createRoot(document.getElementById('root')!).render(
  <I18nProvider>
    <NotificationProvider>
      <App />
    </NotificationProvider>
  </I18nProvider>,
);
```

## 3. 接到複製 ID

`Library.tsx`：

```tsx
const notify = useNotify();
```

成功時：

```tsx
notify.success(t('library.copiedId'));
```

失敗時：

```tsx
notify.error(t('library.copyFailed'));
```

原本按鈕文字提示可以保留；toast 是全域回饋。

## 4. 樣式

焦點是不要讓通知被全域 `button` padding 撐大：

```css
.toast { display:flex; align-items:center; gap:12px; padding:12px 14px; }
.toast > span { flex:1; min-width:0; overflow-wrap:anywhere; }
.toast button {
  flex:0 0 auto;
  width:28px;
  height:28px;
  padding:0;
  display:grid;
  place-items:center;
}
.toast-stack {
  position:fixed;
  right:16px;
  bottom:16px;
  display:grid;
  gap:8px;
  width:min(360px,calc(100vw - 32px));
}
```

## 5. 驗證

```sh
npm run lang:build
npm run lang:check
npm run build:admin
```

瀏覽器：

1. 點「複製 ID」，右下角出現「已複製」。
2. toast 約 3 秒消失。
3. 關閉鈕是 28px，文字垂直置中。
4. 讓 clipboard 失敗，確認錯誤 toast 約 5 秒。
5. 連續點多張卡，確認 toast 可堆疊且不蓋掉按鈕狀態。
6. 390px 無水平溢出。

本次實測：toast `360×54`、關閉鈕 `28×28`、`align-items:center`，3.2 秒後消失。

## 常見錯誤

- `NotificationProvider` 沒有 return JSX：TypeScript 會說它不是有效元件。
- `main.tsx` 保留兩個 `createRoot`：畫面可能重複掛載或行為不穩。
- `useNotify` 在 Provider 外呼叫：hook 會拋錯。
- 忘記 cleanup timer：元件卸載後還可能更新 state。
- 只用 `window.alert`：沒有樣式、無法堆疊，也不是一致的 UX。
- 把表單欄位錯誤也丟 toast：使用者會失去欄位旁的上下文。

## 這題學到什麼

- Context 與 custom hook 的邊界。
- 多筆非同步通知的計時器管理。
- 可存取的通知 UI（`role`、`aria-live`）。
- 什麼時候該用全域提示，什麼時候該留在表單內。

## 下一步

Learn 3 會把通知接到一個小型 API + 前端功能，練習查詢參數、OpenAPI 與 API client。
