import { createSignal, Show, type Component } from 'solid-js'
import { useGatewayStore } from '@/stores/gateway'
import { useI18n } from '@/i18n'
import { CyreneLogo } from '@/components/ui'
import { ThemeToggle, LanguageToggle } from './Sidebar'

export const LoginModal: Component = () => {
  const store = useGatewayStore()
  const { t } = useI18n()
  const [password, setPassword] = createSignal('')
  const [loading, setLoading] = createSignal(false)
  const [error, setError] = createSignal('')

  const handleSubmit = async (e?: Event) => {
    e?.preventDefault()
    if (!password().trim() || loading()) return
    setLoading(true)
    setError('')
    try {
      await store.login(password().trim())
      setPassword('')
    } catch (err: unknown) {
      if (err instanceof Error) {
        if (err.message.includes('429') || err.message.includes('too many failed attempts')) {
          setError(t('login.tooManyAttempts'))
        } else {
          setError(t('login.invalidPassword'))
        }
      } else {
        setError(t('login.invalidPassword'))
      }
    } finally {
      setLoading(false)
    }
  }

  return (
    <div class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-bg/70 backdrop-blur-xl animate-fade-in">
      {/* 顶部工具栏：主题与语言切换 */}
      <div class="absolute top-4 right-4 flex items-center gap-2 z-10">
        <LanguageToggle />
        <ThemeToggle />
      </div>

      {/* 登录卡片 */}
      <div class="relative w-full max-w-md rounded-3xl border border-glass-border glass-panel shadow-2xl p-8 sm:p-10 flex flex-col items-center text-center animate-slide-up">
        {/* 顶部装饰光斑 */}
        <div class="absolute -top-12 inset-x-12 h-24 bg-accent/20 blur-3xl rounded-full pointer-events-none" />

        {/* 品牌标识 */}
        <div class="w-14 h-14 rounded-2xl border border-glass-border glass-sticky shadow-glass flex items-center justify-center mb-5 shrink-0">
          <CyreneLogo class="w-8 h-8 text-accent" />
        </div>

        <h1 class="text-xl font-bold text-foreground tracking-tight mb-2">
          {t('login.title')}
        </h1>
        <p class="text-xs text-muted leading-relaxed mb-6 max-w-xs">
          {t('login.subtitle')}
        </p>

        {/* 错误提示 */}
        <Show when={error()}>
          <div class="w-full mb-4 px-3.5 py-2.5 rounded-xl bg-danger/10 border border-danger/25 text-danger text-xs flex items-center gap-2 text-left animate-shake">
            <svg class="w-4 h-4 shrink-0" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <circle cx="12" cy="12" r="10" />
              <line x1="12" y1="8" x2="12" y2="12" />
              <line x1="12" y1="16" x2="12.01" y2="16" />
            </svg>
            <span class="flex-1">{error()}</span>
          </div>
        </Show>

        {/* 表单 */}
        <form onSubmit={handleSubmit} class="w-full space-y-4">
          <div class="relative">
            <input
              type="password"
              class="w-full h-11 px-4 pr-10 rounded-xl bg-card/60 border border-glass-border text-foreground placeholder:text-muted/60 text-sm focus:outline-none focus:ring-2 focus:ring-accent/40 transition-all"
              placeholder={t('login.passwordPlaceholder')}
              value={password()}
              onInput={e => setPassword(e.currentTarget.value)}
              disabled={loading()}
              autofocus
            />
            <div class="absolute right-3.5 top-1/2 -translate-y-1/2 text-muted pointer-events-none">
              <svg class="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                <rect x="3" y="11" width="18" height="11" rx="2" ry="2" />
                <path d="M7 11V7a5 5 0 0 1 10 0v4" />
              </svg>
            </div>
          </div>

          <button
            type="submit"
            disabled={loading() || !password().trim()}
            class="w-full h-11 rounded-xl bg-accent text-accent-fg font-medium text-sm flex items-center justify-center gap-2 hover:brightness-110 active:brightness-95 disabled:opacity-50 disabled:pointer-events-none shadow-lg shadow-accent/20 transition-all cursor-pointer"
          >
            <Show when={loading()} fallback={<span>{t('login.submit')}</span>}>
              <svg class="w-4 h-4 animate-spin" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <circle cx="12" cy="12" r="10" stroke-opacity="0.25" />
                <path d="M12 2a10 10 0 0 1 10 10" />
              </svg>
              <span>{t('login.loggingIn')}</span>
            </Show>
          </button>
        </form>
      </div>
    </div>
  )
}
