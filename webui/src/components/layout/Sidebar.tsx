import { createSignal } from 'solid-js'

export function ThemeToggle() {
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
      class="flex h-8 w-8 items-center justify-center rounded-control text-muted hover:text-text hover:bg-hover transition-colors"
      onClick={toggle}
      aria-label="切换主题"
      title="切换主题"
    >
      <span aria-hidden="true">{light() ? '🌙' : '☀️'}</span>
    </button>
  )
}
