import { type Component, type JSX, For, Show, createSignal, onMount, onCleanup, createEffect, lazy, Suspense } from 'solid-js'
import { HashRouter, Route, A, useLocation } from '@solidjs/router'
import { useGatewayStore } from './stores/gateway'
import { useBackgroundStore } from './stores/background'
import { ThemeToggle, LanguageToggle } from './components/layout/Sidebar'
import { useI18n } from './i18n'
import { ToastHost, ConfirmDialogHost, CyreneLogo, Skeleton, IconHome, IconServer, IconPlayground, IconLayers, IconActivity, IconClock, IconImage, IconGlobe, IconFileText, IconSettings, IconClose, IconMenu, IconLogout } from './components/ui'
import { LoginModal } from './components/layout/LoginModal'
import Home from './pages/Home'
const Providers = lazy(() => import('./pages/Providers'))
const ProviderDetail = lazy(() => import('./pages/ProviderDetail'))
const Combos = lazy(() => import('./pages/Combos'))
const Usage = lazy(() => import('./pages/Usage'))
const Quota = lazy(() => import('./pages/Quota'))
const Media = lazy(() => import('./pages/Media'))
const LogsPage = lazy(() => import('./pages/Logs'))
const ProxyPools = lazy(() => import('./pages/ProxyPools'))
const Settings = lazy(() => import('./pages/Settings'))
const Playground = lazy(() => import('./pages/Playground'))

// 现代 SVG 矢量侧边栏图标
const NavIcons = {
  home: () => <IconHome class="w-4 h-4 shrink-0" />,
  providers: () => <IconServer class="w-4 h-4 shrink-0" />,
  playground: () => <IconPlayground class="w-4 h-4 shrink-0" />,
  combos: () => <IconLayers class="w-4 h-4 shrink-0" />,
  usage: () => <IconActivity class="w-4 h-4 shrink-0" />,
  quota: () => <IconClock class="w-4 h-4 shrink-0" />,
  media: () => <IconImage class="w-4 h-4 shrink-0" />,
  proxy: () => <IconGlobe class="w-4 h-4 shrink-0" />,
  logs: () => <IconFileText class="w-4 h-4 shrink-0" />,
}

const App: Component = () => {
  const store = useGatewayStore()
  const bgStore = useBackgroundStore()
  const { t } = useI18n()
  const [open, setOpen] = createSignal(false)
  onMount(() => {
    store.loadCore()
    bgStore.init()
  })

  // 布局作为 root 传入 Router：这样侧栏/头部里的 <A> 处于路由上下文内
  const Layout: Component<{ children?: JSX.Element }> = props => {
    const location = useLocation()
    const [scrolled, setScrolled] = createSignal(false)

    createEffect(() => {
      // 路由切换时平滑滚回顶部并重置吸顶加深状态
      location.pathname
      window.scrollTo(0, 0)
      setScrolled(false)
      document.documentElement.setAttribute('data-scrolled', 'false')
    })

    onMount(() => {
      let ticking = false
      const onScroll = () => {
        if (!ticking) {
          window.requestAnimationFrame(() => {
            const isS = window.scrollY > 8
            setScrolled(isS)
            document.documentElement.setAttribute('data-scrolled', isS ? 'true' : 'false')
            ticking = false
          })
          ticking = true
        }
      }
      window.addEventListener('scroll', onScroll, { passive: true })
      onScroll()
      onCleanup(() => {
        window.removeEventListener('scroll', onScroll)
        document.documentElement.removeAttribute('data-scrolled')
      })
    })

    return (
      <div class={`min-h-screen text-text relative selection:bg-accent/25 app-root-shell ${bgStore.hasCustomBg() ? '' : 'bg-bg'}`}>
        <Show when={bgStore.hasCustomBg()}>
          <div class="fixed inset-0 pointer-events-none z-0 overflow-hidden bg-bg">
            <img
              src={bgStore.imageData()}
              alt=""
              referrerpolicy="no-referrer"
              class="w-full h-full object-cover transition-all duration-300 ease-out"
              style={{
                filter: bgStore.config().blur ? `blur(${bgStore.config().blur}px)` : undefined,
                opacity: bgStore.config().opacity ?? 1,
                transform: bgStore.config().blur ? 'scale(1.05)' : undefined,
              }}
            />
            {/* 仅在暗色模式下提供适度暗色微调，亮色模式保持壁纸原画通透鲜亮 */}
            <div class="absolute inset-0 bg-transparent dark:bg-black/25 pointer-events-none" />
          </div>
        </Show>
        {/* 2026 现代极光光晕背景 (Ambient Gradient Glows) */}
        <div class="fixed top-[-10%] left-[20%] w-125 h-125 bg-accent/5 rounded-full blur-[140px] pointer-events-none z-0" />
        <div class="fixed bottom-[-10%] right-[10%] w-150 h-150 bg-accent-2/5 rounded-full blur-[160px] pointer-events-none z-0" />
        <ToastHost />
        <ConfirmDialogHost />
        {/* 桌面侧栏 */}
        <aside class="hidden md:flex flex-col fixed inset-y-0 left-0 w-(--sidebar-w) z-40 glass-panel border-r border-glass-border shadow-glass">
          <div class="h-16 flex items-center gap-3 px-5 border-b border-subtle box-border">
            <CyreneLogo class="w-8 h-8 shrink-0" />
            <div class="min-w-0">
              <div class="text-sm font-bold leading-tight truncate text-foreground">Cyrene Gateway</div>
            </div>
          </div>
          <SidebarNav onNavigate={() => setOpen(false)} />
          <div class="h-14 px-4 border-t border-glass-border flex items-center justify-between bg-card/40">
            <div class="flex items-center gap-1">
              <ThemeToggle />
              <LanguageToggle />
            </div>
            <A
              href="/settings"
              class="flex h-8 w-8 items-center justify-center rounded-control text-muted hover:text-foreground hover:bg-hover transition-colors"
              title={t('nav.settings')}
              aria-label={t('nav.settings')}
            >
              <IconSettings size={16} />
            </A>
          </div>
        </aside>

      {/* 移动端抽屉 */}
      <Show when={open()}>
        <div class="md:hidden fixed inset-0 z-50">
          <button
            type="button"
            aria-label={t('nav.closeMenuOverlay')}
            class="absolute inset-0 w-full h-full bg-black/60 backdrop-blur-sm animate-fade-in border-none cursor-default"
            onClick={() => setOpen(false)}
          />
          <aside class="absolute inset-y-0 left-0 w-65 glass-panel border-r border-glass-border flex flex-col animate-slide-up shadow-xl">
            <div class="h-16 flex items-center gap-3 px-5 border-b border-subtle">
              <CyreneLogo class="w-8 h-8 shrink-0" />
              <span class="text-sm font-bold flex-1">Cyrene Gateway</span>
              <button
                type="button"
                class="flex h-8 w-8 items-center justify-center rounded-control text-faint hover:text-text hover:bg-hover"
                onClick={() => setOpen(false)}
                aria-label={t('nav.closeMenu')}
              >
                <IconClose size={16} />
              </button>
            </div>
            <SidebarNav onNavigate={() => setOpen(false)} />
            <div class="h-14 px-4 border-t border-glass-border flex items-center justify-between bg-card/40 shrink-0">
              <div class="flex items-center gap-1">
                <ThemeToggle />
                <LanguageToggle />
              </div>
              <A
                href="/settings"
                onClick={() => setOpen(false)}
                class="flex h-8 w-8 items-center justify-center rounded-control text-muted hover:text-foreground hover:bg-hover transition-colors"
                title={t('nav.settings')}
                aria-label={t('nav.settings')}
              >
                <IconSettings size={16} />
              </A>
            </div>
          </aside>
        </div>
      </Show>

      {/* 主区 */}
      <div class="flex flex-col md:pl-(--sidebar-w) min-h-screen relative z-10">
        <header class={`sticky top-0 z-30 px-4 lg:px-10 transition-all duration-300 pointer-events-none ${scrolled() ? 'pt-2 pb-2' : 'pt-3.5 pb-2'}`}>
          <div class="h-14 flex items-center justify-between gap-3 px-4.5 rounded-2xl border border-glass-border glass-sticky shadow-glass transition-all duration-300 pointer-events-auto">
            <button
              type="button"
              class="md:hidden flex h-9 w-9 items-center justify-center rounded-xl text-muted hover:text-text hover:bg-hover border border-subtle shrink-0"
              onClick={() => setOpen(true)}
              aria-label={t('nav.openMenu')}
            >
              <IconMenu size={16} />
            </button>
            <div class="flex items-center gap-2 text-xs font-medium text-muted truncate">
              <span class="inline-block w-1.5 h-1.5 rounded-full bg-accent animate-pulse" />
              <span class="text-foreground font-semibold">Cyrene Gateway</span>
              <span class="text-faint hidden sm:inline">/</span>
              <span class="text-faint hidden sm:inline">Unified LLM Gateway</span>
            </div>
            <div class="ml-auto flex items-center gap-3">
              <div class="flex items-center gap-2 px-3 py-1.5 rounded-full bg-card/60 border border-glass-border shadow-sm text-xs backdrop-blur-md">
                <span class="w-2 h-2 rounded-full bg-success animate-pulse" />
                <span class="font-medium text-foreground">{store.activeConnections()}</span>
                <span class="text-faint">{t('common.activeConns')}</span>
              </div>
              <Show when={store.requireLogin() && store.authenticated()}>
                <button
                  type="button"
                  class="flex items-center gap-1.5 px-3 py-1.5 rounded-full bg-card/60 border border-glass-border shadow-sm text-xs text-muted hover:text-danger hover:border-danger/40 transition-all cursor-pointer backdrop-blur-md"
                  onClick={() => store.logout()}
                  title={t('login.logout')}
                >
                  <IconLogout size={14} />
                  <span class="hidden sm:inline">{t('login.logout')}</span>
                </button>
              </Show>
            </div>
          </div>
        </header>
        <main class="flex-1 max-w-7xl w-full mx-auto px-4 lg:px-10 py-6 lg:py-8 animate-fade-in">
          <Suspense fallback={
            <div class="space-y-5" role="status" aria-label={t('common.loading')} aria-busy="true">
              <Skeleton class="h-7 w-40" />
              <Skeleton class="h-48 w-full" />
            </div>
          }>
            {props.children}
          </Suspense>
        </main>
      </div>

      {/* 登录认证拦截弹窗 */}
      <Show when={store.authChecked() && store.requireLogin() && !store.authenticated()}>
        <LoginModal />
      </Show>
    </div>
    )
  }

  return (
    <HashRouter root={Layout}>
      <Route path="/" component={Home} />
      <Route path="/providers" component={Providers} />
      <Route path="/providers/:id" component={ProviderDetail} />
      <Route path="/playground" component={Playground} />
      <Route path="/combos" component={Combos} />
      <Route path="/usage" component={Usage} />
      <Route path="/quota" component={Quota} />
      <Route path="/media" component={Media} />
      <Route path="/proxy-pools" component={ProxyPools} />
      <Route path="/logs" component={LogsPage} />
      <Route path="/settings" component={Settings} />
      <Route path="*" component={Home} />
    </HashRouter>
  )
}

function SidebarNav(props: { onNavigate?: () => void }) {
  const { t } = useI18n()
  const navGroups = () => [
    {
      group: t('nav.groupAccess'),
      items: [
        { href: '/', label: t('nav.home'), end: true, icon: NavIcons.home },
        { href: '/providers', label: t('nav.providers'), icon: NavIcons.providers },
        { href: '/playground', label: t('nav.playground'), icon: NavIcons.playground },
        { href: '/combos', label: t('nav.combos'), icon: NavIcons.combos },
        { href: '/usage', label: t('nav.usage'), icon: NavIcons.usage },
        { href: '/quota', label: t('nav.quota'), icon: NavIcons.quota },
      ],
    },
    {
      group: t('nav.groupSystem'),
      items: [
        { href: '/media', label: t('nav.media'), icon: NavIcons.media },
        { href: '/proxy-pools', label: t('nav.proxies'), icon: NavIcons.proxy },
        { href: '/logs', label: t('nav.logs'), icon: NavIcons.logs },
      ],
    },
  ]

  return (
    <nav class="flex-1 overflow-y-auto px-3.5 py-4 space-y-6">
      <For each={navGroups()}>
        {group => (
          <div>
            <div class="px-3 pb-2 text-[11px] font-semibold uppercase tracking-wider text-faint">{group.group}</div>
            <div class="space-y-1">
              <For each={group.items}>
                {item => (
                  <A
                    href={item.href}
                    end={item.end}
                    onClick={props.onNavigate}
                    class="flex items-center gap-2.5 px-3 py-2 rounded-xl text-sm font-medium text-muted hover:text-text hover:bg-hover transition-all border border-transparent"
                    activeClass="text-foreground! bg-accent/15! border-accent/30! text-accent shadow-sm font-semibold"
                  >
                    <item.icon />
                    <span>{item.label}</span>
                  </A>
                )}
              </For>
            </div>
          </div>
        )}
      </For>
    </nav>
  )
}

export default App
