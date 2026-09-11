import { createSignal } from 'solid-js'
import type { Locale, I18nKey } from './types'
import { zhCN } from './zh-CN'
import { enUS } from './en-US'

export * from './types'

const STORAGE_KEY = 'cyrene_locale'

function getInitialLocale(): Locale {
  if (typeof window !== 'undefined' && window.localStorage) {
    try {
      const saved = window.localStorage.getItem(STORAGE_KEY)
      if (saved === 'zh-CN' || saved === 'en-US') {
        return saved
      }
    } catch {
      // ignore
    }
  }
  if (typeof navigator !== 'undefined' && typeof navigator.language === 'string') {
    return navigator.language.toLowerCase().startsWith('zh') ? 'zh-CN' : 'en-US'
  }
  return 'zh-CN'
}
const [currentLocale, setCurrentLocale] = createSignal<Locale>(getInitialLocale())

export function locale(): Locale {
  return currentLocale()
}

export function setLocale(l: Locale) {
  setCurrentLocale(l)
  if (typeof window !== 'undefined' && window.localStorage) {
    window.localStorage.setItem(STORAGE_KEY, l)
  }
  if (typeof document !== 'undefined' && document.documentElement) {
    document.documentElement.lang = l === 'zh-CN' ? 'zh' : 'en'
  }
}

export function toggleLocale() {
  setLocale(currentLocale() === 'zh-CN' ? 'en-US' : 'zh-CN')
}

export function t(path: I18nKey, params?: Record<string, string | number>): string {
  const loc = currentLocale()
  const dict = loc === 'en-US' ? enUS : zhCN
  let val: unknown = dict

  for (const seg of path.split('.')) {
    if (val && typeof val === 'object' && seg in val) {
      val = (val as Record<string, unknown>)[seg]
    } else {
      val = undefined
      break
    }
  }

  // Fallback to zhCN if missing in enUS
  if (typeof val !== 'string' && loc === 'en-US') {
    let fallback: unknown = zhCN
    for (const seg of path.split('.')) {
      if (fallback && typeof fallback === 'object' && seg in fallback) {
        fallback = (fallback as Record<string, unknown>)[seg]
      } else {
        fallback = undefined
        break
      }
    }
    if (typeof fallback === 'string') {
      val = fallback
    }
  }

  if (typeof val !== 'string') {
    return path
  }

  if (!params) return val
  return val.replace(/\{(\w+)\}/g, (_, k) => (k in params ? String(params[k]) : `{${k}}`))
}

export function useI18n() {
  return {
    locale,
    setLocale,
    toggleLocale,
    t,
  }
}
