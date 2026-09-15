import { createSignal, Show, type Component } from 'solid-js'
import { useGatewayStore } from '@/stores/gateway'
import { useI18n } from '@/i18n'
import { CyreneLogo, Input, Button, Alert, IconLock } from '@/components/ui'
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
          <Alert variant="danger" class="w-full mb-4 animate-shake text-left">
            {error()}
          </Alert>
        </Show>

        {/* 表单 */}
        <form onSubmit={handleSubmit} class="w-full space-y-4">
          <div class="relative">
            <Input
              type="password"
              class="w-full h-11 pr-10"
              placeholder={t('login.passwordPlaceholder')}
              value={password()}
              onInput={setPassword}
              disabled={loading()}
            />
            <div class="absolute right-3.5 top-1/2 -translate-y-1/2 text-muted pointer-events-none">
              <IconLock size={16} />
            </div>
          </div>

          <Button
            type="submit"
            variant="primary"
            loading={loading()}
            disabled={!password().trim()}
            class="w-full h-11 justify-center text-sm shadow-lg shadow-accent/20"
          >
            {loading() ? t('login.loggingIn') : t('login.submit')}
          </Button>
        </form>
      </div>
    </div>
  )
}
