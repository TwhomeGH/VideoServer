import zhTW from '../lang/zh_TW.json';
import zhCN from '../lang/zh_CN.json';
import en from '../lang/en.json';

/** Messages 是扁平化的翻譯鍵值；鍵使用點號分組，值可含 {name} 插值。 */
export type Messages = Record<string, string>;

/** LocaleDefinition 是語言清單的一筆；新增語言時同時加入 registry。 */
export type LocaleDefinition = {
  code: string;
  label: string;
  messages: Messages;
};

/** BASE_LOCALE 是翻譯的來源語言；缺少鍵時回退到這個語言。 */
export const BASE_LOCALE = 'zh-TW';

/** DEFAULT_LOCALE 是瀏覽器語言無法對應時使用的語言。 */
export const DEFAULT_LOCALE = 'zh-TW';

/** locales 是語言註冊器；新增 ja.json 後在這裡加入一筆即可出現在選單。 */
export const locales: LocaleDefinition[] = [
  { code: 'zh-TW', label: '繁體中文', messages: zhTW },
  { code: 'zh-CN', label: '简体中文', messages: zhCN },
  { code: 'en', label: 'English', messages: en },
];

export const baseMessages = locales.find(locale => locale.code === BASE_LOCALE)!.messages;

/** findLocale 回傳已註冊語言；找不到時回傳 undefined。 */
export function findLocale(code: string): LocaleDefinition | undefined {
  return locales.find(locale => locale.code === code);
}

/** matchLocale 將瀏覽器語言標籤對應到已註冊語言，支援未來新增的語言前綴。 */
export function matchLocale(candidates: readonly string[]): string {
  for (const candidate of candidates) {
    const value = candidate.toLowerCase();
    if (value.startsWith('zh')) {
      if (/hant|tw|hk|mo/.test(value)) return 'zh-TW';
      if (/hans|cn|sg/.test(value)) return 'zh-CN';
    }
    const exact = locales.find(locale => locale.code.toLowerCase() === value);
    if (exact) return exact.code;
    const prefix = locales.find(locale => value.startsWith(`${locale.code.toLowerCase()}-`));
    if (prefix) return prefix.code;
  }
  return DEFAULT_LOCALE;
}
