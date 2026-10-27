import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from 'react';
import { initialLocale, setActiveLocale, translateFor } from './core';
import { locales, findLocale, type LocaleDefinition } from './registry';

type I18nContextValue = {
  locale: string;
  setLocale: (code: string) => void;
  t: (key: string, params?: Record<string, string | number>) => string;
  locales: LocaleDefinition[];
};

const I18nContext = createContext<I18nContextValue | null>(null);

/** I18nProvider 管理語言選擇、html lang 與 localStorage；切換時整個介面重新渲染。 */
export function I18nProvider({ children }: { children: ReactNode }) {
  const [locale, setLocaleState] = useState(initialLocale);
  const definition = findLocale(locale) ?? locales[0];

  useEffect(() => {
    setActiveLocale(locale);
    document.documentElement.lang = locale;
    try { localStorage.setItem('dvn-locale', locale); } catch { /* 選擇仍適用本次工作階段。 */ }
  }, [locale]);

  const t = useCallback((key: string, params?: Record<string, string | number>) => {
    const value = translateFor(definition.messages, key, params);
    if (import.meta.env.DEV && definition.messages[key] === undefined) {
      console.warn(`[i18n] missing key "${key}" in ${locale}`);
    }
    return value;
  }, [definition, locale]);

  const setLocale = useCallback((code: string) => {
    if (findLocale(code)) setLocaleState(code);
  }, []);

  const value = useMemo(() => ({ locale, setLocale, t, locales }), [locale, setLocale, t]);
  return <I18nContext.Provider value={value}>{children}</I18nContext.Provider>;
}

/** useI18n 取得目前語言與翻譯函式；必須在 I18nProvider 內使用。 */
export function useI18n(): I18nContextValue {
  const context = useContext(I18nContext);
  if (!context) throw new Error('useI18n must be used inside I18nProvider');
  return context;
}
