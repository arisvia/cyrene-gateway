import { ref, type App } from 'vue'

export interface LocaleInfo {
  code: string
  label: string
  flag: string
}

export const LOCALES: LocaleInfo[] = [
  { code: 'en', label: 'English', flag: '🇺🇸' },
  { code: 'zh-CN', label: '简体中文', flag: '🇨🇳' },
  { code: 'ja', label: '日本語', flag: '🇯🇵' },
  { code: 'ko', label: '한국어', flag: '🇰🇷' },
  { code: 'ru', label: 'Русский', flag: '🇷🇺' },
  { code: 'es', label: 'Español', flag: '🇪🇸' },
  { code: 'fr', label: 'Français', flag: '🇫🇷' },
  { code: 'de', label: 'Deutsch', flag: '🇩🇪' },
  { code: 'pt-BR', label: 'Português (BR)', flag: '🇧🇷' },
  { code: 'tr', label: 'Türkçe', flag: '🇹🇷' },
]

const STORAGE_KEY = 'cyrene-locale'

let translationMap: Record<string, string> = {}
const locale = ref(getStoredLocale())
let observer: MutationObserver | null = null
const changeCallbacks: Array<() => void> = []

function getStoredLocale(): string {
  try {
    const stored = localStorage.getItem(STORAGE_KEY)
    if (stored && LOCALES.some(l => l.code === stored)) return stored
  } catch { /* ignore */ }
  // Detect browser language
  const lang = navigator.language
  if (lang.startsWith('zh')) return 'zh-CN'
  if (lang.startsWith('ja')) return 'ja'
  if (lang.startsWith('ko')) return 'ko'
  if (lang.startsWith('ru')) return 'ru'
  if (lang.startsWith('es')) return 'es'
  if (lang.startsWith('fr')) return 'fr'
  if (lang.startsWith('de')) return 'de'
  if (lang.startsWith('pt')) return 'pt-BR'
  if (lang.startsWith('tr')) return 'tr'
  return 'en'
}

async function loadTranslations(code: string) {
  if (code === 'en') {
    translationMap = {}
    return
  }
  try {
    const res = await fetch(`/i18n/literals/${code}.json`)
    if (res.ok) translationMap = await res.json()
    else translationMap = {}
  } catch {
    translationMap = {}
  }
}

export function t(text: string): string {
  if (!text || locale.value === 'en') return text
  const trimmed = text.trim()
  if (!trimmed) return text
  return translationMap[trimmed] || text
}

export function getLocale(): string {
  return locale.value
}

export async function setLocale(code: string) {
  locale.value = code
  try { localStorage.setItem(STORAGE_KEY, code) } catch { /* ignore */ }
  await loadTranslations(code)
  processElement(document.body)
  changeCallbacks.forEach(cb => cb())
}

export function onLocaleChange(cb: () => void): () => void {
  changeCallbacks.push(cb)
  return () => {
    const idx = changeCallbacks.indexOf(cb)
    if (idx >= 0) changeCallbacks.splice(idx, 1)
  }
}

// --- DOM text node processing ---

const SKIP_TAGS = new Set(['SCRIPT', 'STYLE', 'CODE', 'PRE', 'TEXTAREA'])

function processTextNode(node: Text) {
  if (!node.nodeValue || !node.nodeValue.trim()) return
  const parent = node.parentElement
  if (!parent) return
  if (SKIP_TAGS.has(parent.tagName)) return
  // Skip elements with data-i18n-skip
  let el: HTMLElement | null = parent
  while (el) {
    if (el.hasAttribute('data-i18n-skip')) return
    el = el.parentElement
  }
  // Store original
  const anyNode = node as any
  if (!anyNode._orig) anyNode._orig = node.nodeValue
  const translated = t(anyNode._orig)
  if (translated !== node.nodeValue) node.nodeValue = translated
}

function processElement(root: Element | null) {
  if (!root) return
  const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT)
  const nodes: Text[] = []
  let n: Node | null
  while ((n = walker.nextNode())) nodes.push(n as Text)
  nodes.forEach(processTextNode)
}

function startObserver() {
  if (observer) return
  observer = new MutationObserver(mutations => {
    for (const m of mutations) {
      for (const node of m.addedNodes) {
        if (node.nodeType === Node.ELEMENT_NODE) processElement(node as Element)
        else if (node.nodeType === Node.TEXT_NODE) processTextNode(node as Text)
      }
    }
  })
  observer.observe(document.body, { childList: true, subtree: true })
}

// --- Vue plugin ---

export const i18nPlugin = {
  async install(app: App) {
    await loadTranslations(locale.value)
    app.config.globalProperties.$t = t
    app.config.globalProperties.$locale = locale
    // Process DOM after mount
    const origMount = app.mount
    app.mount = function (...args: any[]) {
      const result = origMount.apply(this, args as any)
      processElement(document.body)
      startObserver()
      return result
    }
  },
}

export { locale }
