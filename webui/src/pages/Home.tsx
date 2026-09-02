import { type Component, For, Show, createResource } from 'solid-js'
import { A } from '@solidjs/router'
import { useGatewayStore } from '@/stores/gateway'
import { Card, Badge, Empty } from '@/components/ui'
import { api } from '@/lib/api'

const Home: Component = () => {
  const store = useGatewayStore()
  const [skills] = createResource(async () => {
    try {
      const r = await api('/api/skills')
      return Array.isArray(r) ? r : []
    } catch { return [] }
  })

  const copy = (text: string) => navigator.clipboard?.writeText(text)

  return (
    <div class="space-y-6 stagger">
      <Card class="p-5 flex items-center gap-6 flex-wrap">
        <div class="flex items-center gap-2">
          <span class="h-2.5 w-2.5 rounded-full bg-[color:var(--green)] animate-pulse" />
          <span class="font-medium">运行中</span>
        </div>
        <span class="text-sm text-muted">v{store.version()}</span>
        <span class="text-sm text-muted">{store.activeConnections()} 个活跃连接</span>
        <span class="text-sm text-faint ml-auto">SQLite WAL · 单二进制</span>
      </Card>

      <div class="grid md:grid-cols-2 gap-4">
        <Card class="p-5">
          <div class="text-sm font-medium mb-3">端点</div>
          <div class="space-y-2">
            <For each={store.endpoints()}>
              {ep => (
                <button
                  class="w-full flex items-center justify-between gap-3 rounded-lg border border-subtle px-3 py-2 text-left hover:border-accent transition-colors"
                  onClick={() => copy(ep.url)}
                  title="点击复制"
                >
                  <span class="text-sm">{ep.label}</span>
                  <code class="text-xs text-faint truncate max-w-[60%]">{ep.url}</code>
                </button>
              )}
            </For>
            <Show when={store.endpoints().length === 0}>
              <Empty message="暂无端点" />
            </Show>
          </div>
        </Card>

        <Card class="p-5">
          <div class="text-sm font-medium mb-3 flex items-center justify-between">
            快速接入
            <A href="/cli-tools" class="text-xs text-muted hover:text-text">全部工具 →</A>
          </div>
          <div class="grid grid-cols-2 gap-2">
            <A href="/cli-tools" class="rounded-lg border border-subtle px-3 py-2 text-sm hover:border-accent transition-colors">Claude Code</A>
            <A href="/cli-tools" class="rounded-lg border border-subtle px-3 py-2 text-sm hover:border-accent transition-colors">Codex</A>
            <A href="/cli-tools" class="rounded-lg border border-subtle px-3 py-2 text-sm hover:border-accent transition-colors">OpenCode</A>
            <A href="/cli-tools" class="rounded-lg border border-subtle px-3 py-2 text-sm hover:border-accent transition-colors">Cline</A>
          </div>
        </Card>
      </div>

      <Card class="p-5">
        <div class="text-sm font-medium mb-3">技能</div>
        <div class="grid sm:grid-cols-2 lg:grid-cols-3 gap-3">
          <For each={skills()}>
            {s => (
              <div class="rounded-lg border border-subtle p-3">
                <div class="text-sm font-medium flex items-center gap-2">
                  {s.name}
                  <Badge tone="blue">cyrene</Badge>
                </div>
                <div class="text-xs text-faint mt-1 line-clamp-2">{s.description}</div>
              </div>
            )}
          </For>
        </div>
        <Show when={!skills()?.length}>
          <Empty message="暂无技能" />
        </Show>
      </Card>
    </div>
  )
}

export default Home