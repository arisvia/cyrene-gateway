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
import Console from './pages/Console'
import ProxyPools from './pages/ProxyPools'
import Tunnel from './pages/Tunnel'
import Mitm from './pages/Mitm'
import Skills from './pages/Skills'
import Settings from './pages/Settings'

const NAV = [
  {
    group: '接入',
    items: [
      { href: '/', label: '首页', end: true },
      { href: '/providers', label: '提供商' },
      { href: '/combos', label: '组合' },
      { href: '/usage', label: '用量' },
      { href: '/quota', label: '配额' },
    ],
  },
  {
    group: '系统',
    items: [
      { href: '/media', label: '媒体' },
      { href: '/proxy-pools', label: '代理池' },
      { href: '/cli-tools', label: 'CLI 工具' },
      { href: '/console', label: '控制台' },
      { href: '/tunnel', label: '隧道' },
      { href: '/mitm', label: 'MITM' },
      { href: '/skills', label: '技能' },
    ],
  },
]

const App: Component = () => {
  const store = useGatewayStore()
  const [open, setOpen] = createSignal(false)
  onMount(() => store.loadCore())

  // 布局作为 root 传入 Router：这样侧栏/头部里的 <A> 处于路由上下文内，
  const Layout: Component<{ children?: JSX.Element }> = props => (
    <div class="min-h-screen">
      <ToastHost />

      {/* 桌面侧栏 */}
      <aside class="hidden md:flex flex-col fixed inset-y-0 left-0 w-[var(--sidebar-w)] glass-panel border-y-0 border-l-0 z-40">
        <div class="h-14 flex items-center gap-2.5 px-4 border-b border-subtle">
          <div class="w-7 h-7 rounded-control gradient-brand shadow-accent" />
          <div class="min-w-0">
            <div class="text-sm font-semibold leading-tight truncate">Cyrene Gateway</div>
            <div class="text-[11px] text-faint leading-tight">v{store.version()}</div>
          </div>
        </div>
        <SidebarNav onNavigate={() => setOpen(false)} />
        <div class="p-3 border-t border-subtle flex items-center justify-between">
          <ThemeToggle />
          <A href="/settings" class="text-xs text-muted hover:text-text transition-colors">
            设置 →
          </A>
        </div>
      </aside>

      {/* 移动端抽屉 */}
      <Show when={open()}>
        <div class="md:hidden fixed inset-0 z-50">
          <div class="absolute inset-0 bg-black/60 backdrop-blur-sm animate-fade-in" onClick={() => setOpen(false)} aria-hidden="true" />
          <aside class="absolute inset-y-0 left-0 w-[260px] bg-bg-elevated border-r border-subtle flex flex-col animate-slide-up">
            <div class="h-14 flex items-center gap-2.5 px-4 border-b border-subtle">
              <div class="w-7 h-7 rounded-control gradient-brand" />
              <span class="text-sm font-semibold flex-1">Cyrene Gateway</span>
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
      <div class="flex flex-col md:pl-[var(--sidebar-w)] min-h-screen">
        <header class="h-14 sticky top-0 z-30 flex items-center gap-3 px-4 lg:px-8 border-b border-subtle bg-card backdrop-blur-xl">
          <button
            type="button"
            class="md:hidden flex h-8 w-8 items-center justify-center rounded-control text-muted hover:text-text hover:bg-hover"
            onClick={() => setOpen(true)}
            aria-label="打开菜单"
          >
            ☰
          </button>
          <div class="ml-auto flex items-center gap-3 text-sm text-muted">
            <span class="hidden sm:inline text-faint">{store.activeConnections()} 个活跃连接</span>
          </div>
        </header>
        <main class="flex-1 max-w-7xl w-full mx-auto px-4 lg:px-10 py-6 lg:py-10">
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
      <Route path="/console" component={Console} />
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
    <nav class="flex-1 overflow-y-auto px-3 py-4 space-y-6">
      <For each={NAV}>
        {group => (
          <div>
            <div class="px-2 pb-2 text-[11px] uppercase tracking-wider text-faint">{group.group}</div>
            <div class="space-y-0.5">
              <For each={group.items}>
                {item => (
                  <A
                    href={item.href}
                    end={item.end}
                    onClick={props.onNavigate}
                    class="flex items-center px-2.5 py-1.5 rounded-control text-sm text-muted hover:text-text hover:bg-hover transition-colors"
                    activeClass="!text-text gradient-soft ring-1 ring-subtle font-medium"
                  >
                    {item.label}
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
