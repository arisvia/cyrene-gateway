export type Locale = 'zh-CN' | 'en-US'

export interface I18nContext {
  locale: () => Locale
  setLocale: (locale: Locale) => void
  t: (path: string, params?: Record<string, string | number>) => string
}
