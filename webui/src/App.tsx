import { type Component, type JSX, For, Show, createSignal, onMount } from 'solid-js'
import { HashRouter, Route, A } from '@solidjs/router'
import { useGatewayStore } from './stores/gateway'
import { ThemeToggle } from './components/layout/Sidebar'
import { ToastHost } from './components/ui'

import Home from './pages/Home'
import Providers from './pages/Providers'
import ProviderDetail from './pages/ProviderDetail'
import Combos from './pages/Combos'
import Usage from './pages/Usage'
import Quota from './pages/Quota'
import Media from './pages/Media'
import CliTools from './pages/CliTools'
import CliToolDetail from './pages/CliToolDetail'
import LogsPage from './pages/Logs'
import ProxyPools from './pages/ProxyPools'
import Tunnel from './pages/Tunnel'
import Mitm from './pages/Mitm'
import Skills from './pages/Skills'
import Settings from './pages/Settings'

// 现代 SVG 矢量侧边栏图标
const NavIcons = {
  home: () => (
    <svg class="w-4 h-4 shrink-0" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
      <path d="m3 9 9-7 9 7v11a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z" />
      <polyline points="9 22 9 12 15 12 15 22" />
    </svg>
  ),
  providers: () => (
    <svg class="w-4 h-4 shrink-0" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
      <rect x="2" y="2" width="20" height="8" rx="2" ry="2" />
      <rect x="2" y="14" width="20" height="8" rx="2" ry="2" />
      <line x1="6" y1="6" x2="6.01" y2="6" />
      <line x1="6" y1="18" x2="6.01" y2="18" />
    </svg>
  ),
  combos: () => (
    <svg class="w-4 h-4 shrink-0" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
      <polygon points="12 2 2 7 12 12 22 7 12 2" />
      <polyline points="2 17 12 22 22 17" />
      <polyline points="2 12 12 17 22 12" />
    </svg>
  ),
  usage: () => (
    <svg class="w-4 h-4 shrink-0" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
      <path d="M3 3v18h18" />
      <path d="m19 9-5 5-4-4-3 3" />
    </svg>
  ),
  quota: () => (
    <svg class="w-4 h-4 shrink-0" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
      <circle cx="12" cy="12" r="10" />
      <path d="M12 6v6l4 2" />
    </svg>
  ),
  media: () => (
    <svg class="w-4 h-4 shrink-0" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
      <rect width="18" height="18" x="3" y="3" rx="2" ry="2" />
      <circle cx="9" cy="9" r="2" />
      <path d="m21 15-3.086-3.086a2 2 0 0 0-2.828 0L6 21" />
    </svg>
  ),
  proxy: () => (
    <svg class="w-4 h-4 shrink-0" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
      <circle cx="12" cy="12" r="10" />
      <path d="M12 2a14.5 14.5 0 0 0 0 20 14.5 14.5 0 0 0 0-20" />
      <path d="M2 12h20" />
    </svg>
  ),
  cli: () => (
    <svg class="w-4 h-4 shrink-0" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
      <polyline points="4 17 10 11 4 5" />
      <line x1="12" y1="19" x2="20" y2="19" />
    </svg>
  ),
  logs: () => (
    <svg class="w-4 h-4 shrink-0" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
      <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z" />
      <polyline points="14 2 14 8 20 8" />
      <line x1="16" y1="13" x2="8" y2="13" />
      <line x1="16" y1="17" x2="8" y2="17" />
      <polyline points="10 9 9 9 8 9" />
    </svg>
  ),
  tunnel: () => (
    <svg class="w-4 h-4 shrink-0" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
      <path d="M4 14a2 2 0 0 1 2-2h12a2 2 0 0 1 2 2v2a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2z" />
      <path d="M6 12V8a6 6 0 0 1 12 0v4" />
    </svg>
  ),
  mitm: () => (
    <svg class="w-4 h-4 shrink-0" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
      <path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10" />
    </svg>
  ),
  skills: () => (
    <svg class="w-4 h-4 shrink-0" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
      <path d="M12 2v4" />
      <path d="m4.93 4.93 2.83 2.83" />
      <path d="M2 12h4" />
      <path d="m4.93 19.07 2.83-2.83" />
      <path d="M12 18v4" />
      <path d="m16.24 16.24 2.83 2.83" />
      <path d="M18 12h4" />
      <path d="m16.24 7.76 2.83-2.83" />
    </svg>
  ),
}

const NAV = [
  {
    group: '接入',
    items: [
      { href: '/', label: '首页', end: true, icon: NavIcons.home },
      { href: '/providers', label: '提供商', icon: NavIcons.providers },
      { href: '/combos', label: '组合', icon: NavIcons.combos },
      { href: '/usage', label: '用量', icon: NavIcons.usage },
      { href: '/quota', label: '配额', icon: NavIcons.quota },
    ],
  },
  {
    group: '系统',
    items: [
      { href: '/media', label: '媒体', icon: NavIcons.media },
      { href: '/logs', label: '日志', icon: NavIcons.logs },
      { href: '/cli-tools', label: 'CLI 工具', icon: NavIcons.cli },
      { href: '/mitm', label: 'MITM', icon: NavIcons.mitm },
      { href: '/skills', label: '技能', icon: NavIcons.skills },
    ],
  },
]

const App: Component = () => {
  const store = useGatewayStore()
  const [open, setOpen] = createSignal(false)
  onMount(() => store.loadCore())

  // 布局作为 root 传入 Router：这样侧栏/头部里的 <A> 处于路由上下文内
  const Layout: Component<{ children?: JSX.Element }> = props => (
    <div class="min-h-screen bg-bg text-text">
      <ToastHost />

      {/* 桌面侧栏 */}
      <aside class="hidden md:flex flex-col fixed inset-y-0 left-0 w-(--sidebar-w) glass-panel border-y-0 border-l-0 z-40 bg-card/60 backdrop-blur-xl">
        <div class="h-16 flex items-center gap-3 px-5 border-b border-subtle">
          <img src="/icon.png" alt="Cyrene Gateway" class="w-8 h-8 rounded-xl object-contain shadow-accent shrink-0" />
          <div class="min-w-0">
            <div class="text-sm font-bold leading-tight truncate text-foreground">Cyrene Gateway</div>
          </div>
        </div>
        <SidebarNav onNavigate={() => setOpen(false)} />
        <div class="p-3.5 border-t border-subtle flex items-center justify-between bg-card/30">
          <ThemeToggle />
          <A
            href="/settings"
            class="text-xs font-medium text-muted hover:text-accent transition-colors flex items-center gap-1 py-1 px-2 rounded-lg hover:bg-hover"
          >
            <span>系统设置</span>
            <span aria-hidden="true">→</span>
          </A>
        </div>
      </aside>

      {/* 移动端抽屉 */}
      <Show when={open()}>
        <div class="md:hidden fixed inset-0 z-50">
          <div class="absolute inset-0 bg-black/60 backdrop-blur-sm animate-fade-in" onClick={() => setOpen(false)} aria-hidden="true" />
          <aside class="absolute inset-y-0 left-0 w-65 bg-bg-elevated border-r border-subtle flex flex-col animate-slide-up shadow-2xl">
            <div class="h-16 flex items-center gap-3 px-5 border-b border-subtle">
              <img src="/icon.png" alt="Cyrene Gateway" class="w-8 h-8 rounded-xl object-contain shadow-accent shrink-0" />
              <span class="text-sm font-bold flex-1">Cyrene Gateway</span>
              <button
                type="button"
                class="flex h-8 w-8 items-center justify-center rounded-control text-faint hover:text-text hover:bg-hover"
                onClick={() => setOpen(false)}
                aria-label="关闭菜单"
              >
                ×
              </button>
            </div>
            <SidebarNav onNavigate={() => setOpen(false)} />
          </aside>
        </div>
      </Show>

      {/* 主区 */}
      <div class="flex flex-col md:pl-(--sidebar-w) min-h-screen">
        <header class="h-16 sticky top-0 z-30 flex items-center justify-between gap-3 px-4 lg:px-10 border-b border-subtle bg-bg/80 backdrop-blur-xl">
          <button
            type="button"
            class="md:hidden flex h-9 w-9 items-center justify-center rounded-xl text-muted hover:text-text hover:bg-hover border border-subtle"
            onClick={() => setOpen(true)}
            aria-label="打开菜单"
          >
            ☰
          </button>
          <div class="ml-auto flex items-center gap-3">
            <div class="flex items-center gap-2 px-3 py-1.5 rounded-full bg-card border border-subtle shadow-sm text-xs">
              <span class="w-2 h-2 rounded-full bg-success animate-pulse" />
              <span class="font-medium text-foreground">{store.activeConnections()}</span>
              <span class="text-faint">活跃连接</span>
            </div>
          </div>
        </header>
        <main class="flex-1 max-w-7xl w-full mx-auto px-4 lg:px-10 py-6 lg:py-8">
          {props.children}
        </main>
      </div>
    </div>
  )

  return (
    <HashRouter root={Layout}>
      <Route path="/" component={Home} />
      <Route path="/providers" component={Providers} />
      <Route path="/providers/:id" component={ProviderDetail} />
      <Route path="/combos" component={Combos} />
      <Route path="/usage" component={Usage} />
      <Route path="/quota" component={Quota} />
      <Route path="/media" component={Media} />
      <Route path="/proxy-pools" component={ProxyPools} />
      <Route path="/cli-tools" component={CliTools} />
      <Route path="/cli-tools/:id" component={CliToolDetail} />
      <Route path="/logs" component={LogsPage} />
      <Route path="/tunnel" component={Tunnel} />
      <Route path="/mitm" component={Mitm} />
      <Route path="/skills" component={Skills} />
      <Route path="/settings" component={Settings} />
      <Route path="*" component={Home} />
    </HashRouter>
  )
}

function SidebarNav(props: { onNavigate?: () => void }) {
  return (
    <nav class="flex-1 overflow-y-auto px-3.5 py-4 space-y-6">
      <For each={NAV}>
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
                    activeClass="!text-foreground !bg-accent/15 !border-accent/30 text-accent shadow-sm font-semibold"
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
