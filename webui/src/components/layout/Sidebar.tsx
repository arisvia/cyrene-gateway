import { createSignal, Show } from 'solid-js'

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
      title={light() ? '切换到暗色主题' : '切换到亮色主题'}
    >
      <Show
        when={light()}
        fallback={
          <svg class="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
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
        }
      >
        <svg class="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <path d="M12 3a6 6 0 0 0 9 9 9 9 0 1 1-9-9Z" />
        </svg>
      </Show>
    </button>
  )
}
