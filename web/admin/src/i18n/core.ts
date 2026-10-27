import { baseMessages, findLocale, matchLocale, DEFAULT_LOCALE, type Messages } from './registry';

let activeMessages: Messages = findLocale(initialLocale())?.messages ?? baseMessages;

/** interpolate 展開 {name}；缺少參數時保留原樣，避免靜默吃掉除錯線索。 */
export function interpolate(value: string, params?: Record<string, string | number>): string {
  return value.replace(/\{([a-zA-Z0-9_]+)\}/g, (match, name: string) =>
    params && Object.hasOwn(params, name) ? String(params[name]) : match);
}

/** translate 供非 React 模組使用；React 內請優先用 useI18n 的 t。 */
export function translate(key: string, params?: Record<string, string | number>): string {
  return interpolate(activeMessages[key] ?? baseMessages[key] ?? key, params);
}

/** translateFor 以指定語言翻譯，缺少鍵時回退到來源語言。 */
export function translateFor(messages: Messages, key: string, params?: Record<string, string | number>): string {
  return interpolate(messages[key] ?? baseMessages[key] ?? key, params);
}

/** setActiveLocale 更新非 React 模組使用的語言。 */
export function setActiveLocale(code: string): void {
  activeMessages = findLocale(code)?.messages ?? baseMessages;
}

/** initialLocale 依已保存選擇、瀏覽器語言與預設語言決定初始值。 */
export function initialLocale(): string {
  try {
    const stored = localStorage.getItem('dvn-locale');
    if (stored && findLocale(stored)) return stored;
  } catch { /* 無儲存權限時仍可由瀏覽器語言決定。 */ }
  const candidates = typeof navigator !== 'undefined'
    ? (navigator.languages?.length ? navigator.languages : [navigator.language])
    : [DEFAULT_LOCALE];
  return matchLocale(candidates);
}
