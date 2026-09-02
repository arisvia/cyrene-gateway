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
      class="text-sm text-muted hover:text-text transition-colors"
      onClick={toggle}
      aria-label="切换主题"
      title="切换主题"
    >
      {light() ? '🌙' : '☀️'}
    </button>
  )
}
