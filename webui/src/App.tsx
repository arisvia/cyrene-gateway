import { type Component, type JSX, For, Show, createSignal, onMount, onCleanup, createEffect, lazy, Suspense } from 'solid-js'
import { HashRouter, Route, A, useLocation } from '@solidjs/router'
import { useGatewayStore } from './stores/gateway'
import { useBackgroundStore } from './stores/background'
import { ThemeToggle, LanguageToggle } from './components/layout/Sidebar'
import { useI18n } from './i18n'
import { ToastHost, ConfirmDialogHost, CyreneLogo, Skeleton, Button, IconButton, BackToTop, IconHome, IconServer, IconPlayground, IconLayers, IconActivity, IconClock, IconImage, IconGlobe, IconFileText, IconSettings, IconClose, IconMenu, IconLogout } from './components/ui'
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
    const pageTitle = () => {
      const titles = {
        providers: 'nav.providers', playground: 'nav.playground', combos: 'nav.combos',
        usage: 'nav.usage', quota: 'nav.quota', media: 'nav.media',
        'proxy-pools': 'nav.proxies', logs: 'nav.logs', settings: 'nav.settings',
      } as const
      const route = location.pathname.split('/')[1] as keyof typeof titles
      return t(titles[route] ?? 'nav.home')
    }
    let drawer: HTMLElement | undefined

    createEffect(() => {
      location.pathname
      window.scrollTo(0, 0)
      setOpen(false)
      document.documentElement.setAttribute('data-scrolled', 'false')
    })

    createEffect(() => {
      if (!open()) return
      const previousFocus = document.activeElement as HTMLElement | null
      const previousOverflow = document.body.style.overflow
      document.body.style.overflow = 'hidden'
      queueMicrotask(() => drawer?.querySelector<HTMLElement>('button, a[href]')?.focus())
      const onKey = (event: KeyboardEvent) => {
        if (event.key === 'Escape') setOpen(false)
        if (event.key !== 'Tab') return
        const controls = drawer?.querySelectorAll<HTMLElement>('a[href], button:not(:disabled)')
        if (!controls?.length) return
        const first = controls[0]
        const last = controls[controls.length - 1]
        if (event.shiftKey && document.activeElement === first) {
          event.preventDefault()
          last.focus()
        } else if (!event.shiftKey && document.activeElement === last) {
          event.preventDefault()
          first.focus()
        }
      }
      const desktop = window.matchMedia('(min-width: 768px)')
      const onResize = () => { if (desktop.matches) setOpen(false) }
      desktop.addEventListener('change', onResize)
      document.addEventListener('keydown', onKey)
      onCleanup(() => {
        document.body.style.overflow = previousOverflow
        document.removeEventListener('keydown', onKey)
        desktop.removeEventListener('change', onResize)
        previousFocus?.focus()
      })
    })

    onMount(() => {
      let ticking = false
      const onScroll = () => {
        if (!ticking) {
          window.requestAnimationFrame(() => {
            const isS = window.scrollY > 8
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
            {/* 1. LQIP 极速微缩占位层：0ms 同步直出，平滑模糊铺满，消灭加载跳白 */}
            <Show when={bgStore.config().thumbnail}>
              <img
                src={bgStore.config().thumbnail}
                alt=""
                aria-hidden="true"
                class="absolute inset-0 w-full h-full object-cover transition-opacity duration-700 ease-out"
                style={{
                  filter: `blur(${Math.max(16, (bgStore.config().blur ?? 0) + 12)}px)`,
                  opacity: (bgStore.config().opacity ?? 1) * 0.95,
                  transform: 'scale(1.08)',
                }}
              />
            </Show>
            {/* 2. 高清全量壁纸层：从 IndexedDB 异步就绪后 500ms 丝滑淡入无缝替换 */}
            <Show when={bgStore.imageData()}>
              <img
                src={bgStore.imageData()}
                alt=""
                referrerpolicy="no-referrer"
                class="absolute inset-0 w-full h-full object-cover transition-all duration-500 ease-out"
                style={{
                  filter: bgStore.config().blur ? `blur(${bgStore.config().blur}px)` : undefined,
                  opacity: bgStore.config().opacity ?? 1,
                  transform: bgStore.config().blur ? 'scale(1.05)' : undefined,
                }}
              />
            </Show>
          </div>
        </Show>
        <div class="app-ambient" aria-hidden="true" />
        <ToastHost />
        <ConfirmDialogHost />
        {/* 桌面侧栏 */}
        <aside class="hidden md:flex flex-col fixed inset-y-0 left-0 w-(--sidebar-w) z-40 app-sidebar">
          <div class="h-19 flex items-center gap-3 px-5 shrink-0">
            <CyreneLogo class="w-8 h-8 shrink-0" />
            <div class="min-w-0">
              <div class="text-sm font-bold leading-tight truncate text-foreground">Cyrene Gateway</div>
            </div>
          </div>
          <SidebarNav onNavigate={() => setOpen(false)} />
          <div class="h-14 px-4 border-t border-glass-border flex items-center justify-between">
            <div class="flex items-center gap-1">
              <ThemeToggle />
              <LanguageToggle />
            </div>
            <A
              href="/settings"
              class="flex h-9 w-9 items-center justify-center rounded-control text-muted hover:text-foreground hover:bg-hover transition-colors"
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
          <Button
            variant="ghost"
            aria-label={t('nav.closeMenuOverlay')}
            tabindex={-1}
            class="absolute inset-0 w-full h-full! rounded-none! bg-overlay! backdrop-blur-sm animate-fade-in border-none! active:scale-100!"
            onClick={() => setOpen(false)}
          />
          <aside ref={drawer} role="dialog" aria-modal="true" aria-label={t('nav.groupAccess')} class="absolute inset-y-0 left-0 w-68 max-w-[85vw] app-sidebar flex flex-col animate-slide-up">
            <div class="h-19 flex items-center gap-3 px-5 shrink-0">
              <CyreneLogo class="w-8 h-8 shrink-0" />
              <span class="text-sm font-semibold flex-1">Cyrene Gateway</span>
              <IconButton
                size="lg"
                onClick={() => setOpen(false)}
                aria-label={t('nav.closeMenu')}
              >
                <IconClose size={18} />
              </IconButton>
            </div>
            <SidebarNav onNavigate={() => setOpen(false)} />
            <div class="h-14 px-4 border-t border-glass-border flex items-center justify-between shrink-0">
              <div class="flex items-center gap-1">
                <ThemeToggle />
                <LanguageToggle />
              </div>
              <A
                href="/settings"
                onClick={() => setOpen(false)}
                class="flex h-9 w-9 items-center justify-center rounded-control text-muted hover:text-foreground hover:bg-hover transition-colors"
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
      <div class="flex flex-col md:pl-(--sidebar-w) min-h-dvh relative z-10">
        <header class="sticky top-0 z-30 px-4 lg:px-8 pt-3 pb-2 pointer-events-none">
          <div class="max-w-7xl mx-auto h-13 flex items-center justify-between gap-3 px-3 sm:px-4 rounded-2xl glass-sticky pointer-events-auto">
            <IconButton
              size="lg"
              class="md:hidden"
              onClick={() => setOpen(true)}
              aria-label={t('nav.openMenu')}
              aria-expanded={open()}
            >
              <IconMenu size={18} />
            </IconButton>
            <div class="min-w-0 flex items-center gap-2 text-xs text-muted">
              <span class="truncate font-medium">{pageTitle()}</span>
              <span class="text-faint hidden lg:inline" aria-hidden="true">/</span>
              <span class="text-faint hidden lg:inline">{t('nav.tagline')}</span>
            </div>
            <div class="ml-auto flex items-center gap-2 shrink-0">
              <div class="flex items-center gap-2 text-xs">
                <span class="w-1.5 h-1.5 rounded-full bg-success" />
                <span class="font-medium text-text tabular-nums">{store.activeConnections()}</span>
                <span class="text-muted hidden sm:inline">{t('common.activeConns')}</span>
              </div>
              <Show when={store.requireLogin() && store.authenticated()}>
                <IconButton
                  onClick={() => store.logout()}
                  title={t('login.logout')}
                  aria-label={t('login.logout')}
                  class="hover:text-danger"
                >
                  <IconLogout size={15} />
                </IconButton>
              </Show>
            </div>
          </div>
        </header>
        <main class="flex-1 min-w-0 max-w-7xl w-full mx-auto px-4 lg:px-8 py-6 lg:py-8">
          <Suspense fallback={
            <div class="space-y-5" role="status" aria-label={t('common.loading')} aria-busy="true">
              <Skeleton class="h-7 w-40" />
              <Skeleton class="h-48 w-full" />
            </div>
          }>
            {props.children}
          </Suspense>
        </main>
        <BackToTop />
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
                    class="nav-link"
                    activeClass="nav-active"
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
