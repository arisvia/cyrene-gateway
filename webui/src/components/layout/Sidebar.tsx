import { createSignal } from 'solid-js'
import { useI18n } from '@/i18n'
import { IconSun, IconMoon, IconGlobe } from '@/components/ui'
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
      <IconSun
        size={16}
        class={`absolute transition-all duration-300 ${
          light() ? 'opacity-0 scale-75 rotate-90 pointer-events-none' : 'opacity-100 scale-100 rotate-0'
        }`}
      />

      {/* 月亮图标（亮色模式显示） */}
      <IconMoon
        size={16}
        class={`absolute transition-all duration-300 ${
          light() ? 'opacity-100 scale-100 rotate-0' : 'opacity-0 scale-75 -rotate-90 pointer-events-none'
        }`}
      />
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
      title={locale() === 'zh-CN' ? t('sidebar.switchToEnglish') : t('sidebar.switchToChinese')}
    >
      <IconGlobe size={14} class="shrink-0" />
      <span class="font-mono text-[11px] uppercase tracking-wider">{locale() === 'zh-CN' ? 'EN' : '中'}</span>
    </button>
  )
}
