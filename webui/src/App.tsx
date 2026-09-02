import { type Component, For, Show, createSignal, onMount } from 'solid-js'
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

  return (
    <HashRouter>
      <ToastHost />
      <div class="min-h-screen flex">
        {/* 桌面侧栏 */}
        <aside class="hidden md:flex flex-col fixed inset-y-0 left-0 w-[220px] border-r border-subtle bg-card backdrop-blur-xl z-40">
          <div class="h-14 flex items-center gap-2 px-4 border-b border-subtle">
            <div class="w-7 h-7 rounded-lg" style={{ background: 'var(--gradient)' }} />
            <div>
              <div class="text-sm font-semibold leading-tight">Cyrene Gateway</div>
              <div class="text-[11px] text-faint leading-tight">v{store.version()}</div>
            </div>
          </div>
          <SidebarNav onNavigate={() => setOpen(false)} />
          <div class="mt-auto p-3 border-t border-subtle flex items-center justify-between">
            <ThemeToggle />
          </div>
        </aside>

        {/* 移动端抽屉 */}
        <Show when={open()}>
          <div class="md:hidden fixed inset-0 z-50">
            <div class="absolute inset-0 bg-black/60" onClick={() => setOpen(false)} />
            <aside class="absolute inset-y-0 left-0 w-[260px] bg-bg-elevated border-r border-subtle flex flex-col">
              <div class="h-14 flex items-center gap-2 px-4 border-b border-subtle">
                <div class="w-7 h-7 rounded-lg" style={{ background: 'var(--gradient)' }} />
                <span class="text-sm font-semibold">Cyrene Gateway</span>
              </div>
              <SidebarNav onNavigate={() => setOpen(false)} />
            </aside>
          </div>
        </Show>

        {/* 主区 */}
        <div class="flex-1 md:ml-[220px] min-w-0 flex flex-col">
          <header class="h-14 sticky top-0 z-30 flex items-center gap-3 px-4 lg:px-8 border-b border-subtle bg-card backdrop-blur-xl">
            <button class="md:hidden p-2 -ml-2" onClick={() => setOpen(true)} aria-label="打开菜单">☰</button>
            <div class="ml-auto flex items-center gap-3 text-sm text-muted">
              <span class="text-faint">{store.activeConnections()} 个活跃连接</span>
              <A href="/settings" class="px-3 py-1.5 rounded-lg border border-subtle hover:border-accent transition-colors">设置</A>
            </div>
          </header>
          <main class="flex-1 max-w-7xl w-full mx-auto px-4 lg:px-10 py-6 lg:py-10">
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
          </main>
        </div>
      </div>
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
                    class="flex items-center px-2 py-1.5 rounded-lg text-sm text-muted hover:text-text hover:bg-glass-hover transition-colors"
                    activeClass="!text-text bg-gradient-soft font-medium"
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
