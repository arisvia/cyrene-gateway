import { createSignal } from 'solid-js'
import { useI18n } from '@/i18n'
export function ThemeToggle() {
  const { t } = useI18n()
  const [light, setLight] = createSignal(document.documentElement.classList.contains('light'))
  const toggle = () => {
    const next = !light()
    setLight(next)
    document.documentElement.classList.toggle('light', next)
    localStorage.setItem('cyrene-theme', next ? 'light' : 'dark')
  }
  return (
    <button
      type="button"
      class="relative flex h-8 w-8 items-center justify-center rounded-control text-muted hover:text-text hover:bg-hover transition-colors overflow-hidden"
      onClick={toggle}
      aria-label={t('sidebar.themeToggle')}
      title={light() ? t('sidebar.switchToDark') : t('sidebar.switchToLight')}
    >
      {/* 太阳图标（暗色模式显示） */}
      <svg
        class={`w-4 h-4 absolute transition-all duration-300 ${
          light() ? 'opacity-0 scale-75 rotate-90 pointer-events-none' : 'opacity-100 scale-100 rotate-0'
        }`}
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        stroke-width="2"
        stroke-linecap="round"
        stroke-linejoin="round"
      >
        <circle cx="12" cy="12" r="4" />
        <path d="M12 2v2" />
        <path d="M12 20v2" />
        <path d="m4.93 4.93 1.41 1.41" />
        <path d="m17.66 17.66 1.41 1.41" />
        <path d="M2 12h2" />
        <path d="M20 12h2" />
        <path d="m6.34 17.66-1.41 1.41" />
        <path d="m19.07 4.93-1.41 1.41" />
      </svg>

      {/* 月亮图标（亮色模式显示） */}
      <svg
        class={`w-4 h-4 absolute transition-all duration-300 ${
          light() ? 'opacity-100 scale-100 rotate-0' : 'opacity-0 scale-75 -rotate-90 pointer-events-none'
        }`}
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        stroke-width="2"
        stroke-linecap="round"
        stroke-linejoin="round"
      >
        <path d="M12 3a6 6 0 0 0 9 9 9 9 0 1 1-9-9Z" />
      </svg>
    </button>
  )
}
export function LanguageToggle() {
  const { locale, toggleLocale, t } = useI18n()
  return (
    <button
      type="button"
      class="flex h-8 px-2 items-center justify-center gap-1 rounded-control text-xs font-medium text-muted hover:text-foreground hover:bg-hover transition-colors cursor-pointer"
      onClick={toggleLocale}
      aria-label={t('sidebar.languageToggle')}
      title={locale() === 'zh-CN' ? 'Switch to English' : t('sidebar.switchToChinese')}
    >
      <svg
        class="w-3.5 h-3.5 shrink-0"
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        stroke-width="2"
        stroke-linecap="round"
        stroke-linejoin="round"
      >
        <circle cx="12" cy="12" r="10" />
        <line x1="2" y1="12" x2="22" y2="12" />
        <path d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z" />
      </svg>
      <span class="font-mono text-[11px] uppercase tracking-wider">{locale() === 'zh-CN' ? 'EN' : '中'}</span>
    </button>
  )
}
