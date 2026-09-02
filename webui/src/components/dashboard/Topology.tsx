import { type Component, For, Show, createSignal, createMemo } from 'solid-js'
import { Card, Badge, ProviderAvatar } from '@/components/ui'
import type { Provider, Endpoint } from '@/types/domain'

interface TopologyProps {
  providers: Provider[]
  endpoints?: Endpoint[]
  activeConnections?: number
  liveEvents?: Array<{
    timestamp?: string
    provider?: string
    model?: string
    endpoint?: string
    status?: string
    latencyMs?: number
  }>
}

export const GatewayTopology: Component<TopologyProps> = props => {
  const [selectedNode, setSelectedNode] = createSignal<string | null>(null)

  // 活跃与待命上游分类
  const providerNodes = createMemo(() => {
    const list = props.providers || []
    return list.map(p => {
      // 检查最近是否有实时事件命中该上游
      const recentHit = (props.liveEvents || []).find(e => 
        e.provider === p.provider || e.provider === p.id || (e.model && p.name && e.model.toLowerCase().includes(p.name.toLowerCase()))
      )
      return {
        ...p,
        isHitting: !!recentHit,
        recentStatus: recentHit?.status,
        recentLatency: recentHit?.latencyMs,
      }
    })
  })

  const activeCount = createMemo(() => providerNodes().filter(p => p.isActive).length)

  return (
    <Card class="p-5 overflow-hidden relative border border-subtle/80 bg-card/70 backdrop-blur-xl">
      {/* 顶部标题与状态汇总 */}
      <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-3 mb-6">
        <div class="flex items-center gap-3">
          <div class="w-2.5 h-2.5 rounded-full bg-accent animate-pulse shadow-accent shadow-sm" />
          <div>
            <h3 class="text-sm font-semibold flex items-center gap-2">
              实时路由拓扑与上游连接
              <Badge tone="green" class="text-[10px] px-1.5 py-0">
                {activeCount()} 活跃通道
              </Badge>
            </h3>
            <p class="text-xs text-faint mt-0.5">
              可视化客户端流量分发、网关调度核心与模型上游连接状态
            </p>
          </div>
        </div>

        {/* 状态图例 */}
        <div class="flex items-center gap-3 text-[11px] text-faint">
          <span class="flex items-center gap-1.5">
            <span class="w-2 h-2 rounded-full bg-success animate-pulse" /> 活跃响应中
          </span>
          <span class="flex items-center gap-1.5">
            <span class="w-2 h-2 rounded-full bg-info" /> 就绪备用
          </span>
          <span class="flex items-center gap-1.5">
            <span class="w-2 h-2 rounded-full bg-zinc-600" /> 未激活
          </span>
        </div>
      </div>

      {/* 拓扑交互容器 */}
      <div class="grid grid-cols-1 lg:grid-cols-12 gap-4 items-center relative py-2">
        {/* 1. 客户端入口层 (Left 3 cols) */}
        <div class="lg:col-span-3 space-y-3">
          <div class="text-xs font-semibold text-faint uppercase tracking-wider mb-2 flex items-center gap-1.5">
            <svg class="w-3.5 h-3.5 text-accent" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <rect width="20" height="14" x="2" y="3" rx="2" />
              <line x1="8" x2="16" y1="21" y2="21" />
              <line x1="12" x2="12" y1="17" y2="21" />
            </svg>
            客户端协议端点
          </div>

          <div class="space-y-2">
            <div class="p-3 rounded-xl bg-hover/50 border border-subtle/60 text-xs flex items-center justify-between group hover:border-accent/40 transition-colors">
              <div>
                <div class="font-medium text-foreground">OpenAI 兼容端点</div>
                <div class="text-[11px] text-faint font-mono mt-0.5">/v1/chat/completions</div>
              </div>
              <Badge tone="blue">Ready</Badge>
            </div>

            <div class="p-3 rounded-xl bg-hover/50 border border-subtle/60 text-xs flex items-center justify-between group hover:border-accent/40 transition-colors">
              <div>
                <div class="font-medium text-foreground">Anthropic 协议端点</div>
                <div class="text-[11px] text-faint font-mono mt-0.5">/v1/messages</div>
              </div>
              <Badge tone="blue">Ready</Badge>
            </div>

            <div class="p-3 rounded-xl bg-hover/50 border border-subtle/60 text-xs flex items-center justify-between group hover:border-accent/40 transition-colors">
              <div>
                <div class="font-medium text-foreground">CLI / IDE 专线</div>
                <div class="text-[11px] text-faint font-mono mt-0.5">Claude / Cursor / Codex</div>
              </div>
              <Badge tone="green">Active</Badge>
            </div>
          </div>
        </div>

        {/* 2. 中间流向指示与核心网关枢纽 (Middle 3 cols) */}
        <div class="lg:col-span-3 flex flex-col items-center justify-center p-4 rounded-2xl bg-gradient-to-b from-accent/5 via-accent-2/5 to-transparent border border-accent/20 relative shadow-accent/5 shadow-lg">
          {/* 光晕背景 */}
          <div class="absolute inset-0 bg-accent/5 rounded-2xl blur-xl -z-10 pointer-events-none" />

          <div class="w-12 h-12 rounded-2xl bg-accent/15 border border-accent/30 flex items-center justify-center text-accent mb-3 shadow-accent shadow-sm">
            <svg class="w-6 h-6 animate-pulse" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <path d="m12 3-1.912 5.813a2 2 0 0 1-1.275 1.275L3 12l5.813 1.912a2 2 0 0 1 1.275 1.275L12 21l1.912-5.813a2 2 0 0 1 1.275-1.275L21 12l-5.813-1.912a2 2 0 0 1-1.275-1.275L12 3Z" />
            </svg>
          </div>

          <h4 class="text-xs font-bold text-foreground">Cyrene Core Gateway</h4>
          <p class="text-[10px] text-faint mt-0.5 text-center">智能路由调度与故障回退枢纽</p>

          <div class="mt-3 pt-3 border-t border-subtle/60 w-full flex items-center justify-around text-center">
            <div>
              <div class="text-[10px] text-faint">活跃通道</div>
              <div class="text-sm font-bold text-accent tabular-nums">{activeCount()}</div>
            </div>
            <div class="w-px h-6 bg-subtle/60" />
            <div>
              <div class="text-[10px] text-faint">接入提供商</div>
              <div class="text-sm font-bold text-foreground tabular-nums">{providerNodes().length}</div>
            </div>
          </div>
        </div>

        {/* 3. 右侧上游节点网格 (Right 6 cols) */}
        <div class="lg:col-span-6 space-y-3">
          <div class="text-xs font-semibold text-faint uppercase tracking-wider mb-2 flex items-center justify-between">
            <span class="flex items-center gap-1.5">
              <svg class="w-3.5 h-3.5 text-accent-2" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M12 2v20M17 5H9.5a3.5 3.5 0 0 0 0 7h5a3.5 3.5 0 0 1 0 7H6" />
              </svg>
              已接入模型上游通道 ({providerNodes().length})
            </span>
            <span class="text-[10px] text-faint">点击节点查看状态</span>
          </div>

          <Show
            when={providerNodes().length > 0}
            fallback={<div class="p-6 text-center text-xs text-faint border border-dashed border-subtle rounded-xl">暂无已配置的模型提供商</div>}
          >
            <div class="grid grid-cols-1 sm:grid-cols-2 gap-2.5 max-h-72 overflow-y-auto pr-1">
              <For each={providerNodes()}>
                {p => {
                  const isSelected = () => selectedNode() === p.id
                  return (
                    <div
                      class={`p-3 rounded-xl border transition-all cursor-pointer flex items-center justify-between gap-2.5 ${
                        p.isActive
                          ? p.isHitting
                            ? 'bg-accent/10 border-accent shadow-accent/20 shadow-md ring-1 ring-accent/40'
                            : 'bg-hover/60 border-subtle/80 hover:border-accent/40'
                          : 'bg-hover/20 border-subtle/40 opacity-60 hover:opacity-100'
                      } ${isSelected() ? 'ring-2 ring-accent' : ''}`}
                      onClick={() => setSelectedNode(isSelected() ? null : p.id)}
                    >
                      <div class="flex items-center gap-2.5 min-w-0">
                        <ProviderAvatar provider={p.provider} name={p.name} size="sm" class="shrink-0" />
                        <div class="min-w-0">
                          <div class="text-xs font-semibold truncate text-foreground flex items-center gap-1.5">
                            {p.name || p.provider}
                            <Show when={p.isActive}>
                              <span class="w-1.5 h-1.5 rounded-full bg-success shrink-0 animate-pulse" />
                            </Show>
                          </div>
                          <div class="text-[10px] text-faint font-mono truncate">
                            {p.authType || 'API Key'}
                          </div>
                        </div>
                      </div>

                      <div class="text-right shrink-0">
                        <Show
                          when={p.isActive}
                          fallback={<span class="text-[10px] text-faint">未启用</span>}
                        >
                          <Badge tone={p.isHitting ? 'green' : 'blue'} class="text-[10px]">
                            {p.isHitting ? (p.recentLatency ? `${p.recentLatency}ms` : '响应中') : '就绪'}
                          </Badge>
                        </Show>
                      </div>
                    </div>
                  )
                }}
              </For>
            </div>
          </Show>
        </div>
      </div>
    </Card>
  )
}
