import { type Component, For, Show, createSignal, createMemo } from 'solid-js'
import { Card, Badge, ProviderAvatar } from '@/components/ui'
import type { Provider } from '@/types/domain'

interface TopologyProps {
  providers: Provider[]
  endpoints?: any[]
  activeConnections?: number
  liveEvents?: Array<{
    timestamp?: string
    provider?: string
    model?: string
    status?: string
    latencyMs?: number
  }>
}

export const GatewayTopology: Component<TopologyProps> = props => {
  const [hoveredNode, setHoveredNode] = createSignal<string | null>(null)

  const providers = createMemo(() => props.providers || [])
  const activeCount = createMemo(() => providers().filter(p => p.isActive).length)

  // 计算节点放射状环形排布坐标 (以中心 0,0 为原点)
  const nodePositions = createMemo(() => {
    const list = providers()
    const count = list.length
    if (count === 0) return []

    // 适中半径，确保在卡片视窗内完全展示且不溢出
    const radius = Math.min(180, Math.max(140, 120 + count * 8))

    return list.map((p, i) => {
      // 角度均匀分布（从 -90 度/正上方开始顺时针分布）
      const angle = (i / count) * 2 * Math.PI - Math.PI / 2
      const x = Math.round(Math.cos(angle) * radius)
      const y = Math.round(Math.sin(angle) * radius)

      const recentHit = (props.liveEvents || []).find(e =>
        e.provider === p.provider || e.provider === p.id || (e.model && p.name && e.model.toLowerCase().includes(p.name.toLowerCase()))
      )

      return {
        ...p,
        x,
        y,
        isHitting: !!recentHit,
        recentLatency: recentHit?.latencyMs,
      }
    })
  })

  return (
    <Card class="h-[460px] sm:h-[480px] overflow-hidden relative border border-subtle/80 bg-card/60 backdrop-blur-xl animate-fade-in select-none flex items-center justify-center">
      {/* 顶部标题栏与状态指示 */}
      <div class="absolute top-4 left-4 right-4 z-20 flex items-center justify-between pointer-events-none">
        <div class="flex items-center gap-3 bg-bg/80 backdrop-blur-md px-3.5 py-2 rounded-xl border border-subtle shadow-sm pointer-events-auto">
          <div class="w-2.5 h-2.5 rounded-full bg-accent animate-pulse shadow-accent shadow-sm" />
          <div>
            <h3 class="text-xs font-semibold flex items-center gap-2 text-foreground">
              实时路由拓扑
              <Badge tone="green" class="text-[10px] px-1.5 py-0">
                {activeCount()} 活跃通道
              </Badge>
            </h3>
            <p class="text-[11px] text-faint">
              实时可视化客户端调度核心与模型上游连接状态
            </p>
          </div>
        </div>

        {/* 状态图例 */}
        <div class="hidden sm:flex items-center gap-3 text-[11px] text-faint bg-bg/80 backdrop-blur-md px-3 py-1.5 rounded-xl border border-subtle pointer-events-auto">
          <span class="flex items-center gap-1.5">
            <span class="w-2 h-2 rounded-full bg-accent animate-pulse shadow-accent" /> 调度中
          </span>
          <span class="flex items-center gap-1.5">
            <span class="w-2 h-2 rounded-full bg-success" /> 活跃
          </span>
          <span class="flex items-center gap-1.5">
            <span class="w-2 h-2 rounded-full bg-zinc-600" /> 未激活
          </span>
        </div>
      </div>

      {/* 点阵网格背景 */}
      <div
        class="absolute inset-0 pointer-events-none opacity-30 dark:opacity-15"
        style={{
          'background-image': 'radial-gradient(currentColor 1px, transparent 1px)',
          'background-size': '24px 24px',
        }}
      />

      {/* 核心画布容器：完全居中，无任何溢出或滚动条 */}
      <div class="relative w-full h-full flex items-center justify-center">
        {/* SVG 曲线连接层 (贝塞尔平滑流向曲线 + 粒子脉冲) */}
        <svg
          class="absolute top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2 w-[800px] h-[800px] pointer-events-none overflow-visible -z-10"
          viewBox="-400 -400 800 800"
        >
          <defs>
            <linearGradient id="activeLineGrad" x1="0%" y1="0%" x2="100%" y2="100%">
              <stop offset="0%" stop-color="var(--accent)" stop-opacity="0.8" />
              <stop offset="100%" stop-color="var(--accent-2)" stop-opacity="0.8" />
            </linearGradient>
          </defs>

          <For each={nodePositions()}>
            {node => {
              const isHovered = () => hoveredNode() === node.id
              const cpx = node.x * 0.45
              const cpy = node.y * 0.45
              const d = `M 0 0 Q ${cpx} ${cpy} ${node.x} ${node.y}`

              return (
                <g class="transition-opacity duration-200">
                  {/* 底层连接线 */}
                  <path
                    d={d}
                    fill="none"
                    stroke={node.isHitting ? 'url(#activeLineGrad)' : (node.isActive ? 'rgba(45, 212, 191, 0.3)' : 'rgba(150, 150, 150, 0.15)')}
                    stroke-width={node.isHitting ? 3 : (node.isActive ? 2 : 1.2)}
                    stroke-dasharray={node.isActive ? 'none' : '4 4'}
                  />

                  {/* 命中时的光斑脉冲粒子 */}
                  <Show when={node.isHitting || isHovered()}>
                    <circle r="4" fill="var(--accent)">
                      <animateMotion path={d} dur="1.2s" repeatCount="indefinite" />
                    </circle>
                  </Show>
                </g>
              )
            }}
          </For>
        </svg>

        {/* 1. 中心枢纽：Cyrene Gateway (简约精致胶囊牌) */}
        <div
          class="absolute top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2 z-20 px-4 py-2.5 rounded-xl bg-bg-elevated/95 backdrop-blur-xl border border-accent/40 shadow-xl shadow-accent/15 flex items-center gap-2.5 justify-center hover:scale-105 transition-transform duration-200 cursor-default"
        >
          <img src="/icon.png" alt="Cyrene" class="w-5 h-5 rounded-lg object-contain shadow-accent shadow-sm shrink-0" />
          <div class="font-bold text-xs tracking-tight text-foreground whitespace-nowrap">
            Cyrene Gateway
          </div>
        </div>

        {/* 2. 周围辐射排布的模型上游卡片 (9router 风格节点胶囊) */}
        <For each={nodePositions()}>
          {node => {
            return (
              <div
                class="absolute -translate-x-1/2 -translate-y-1/2 z-10 transition-all duration-200 group"
                style={{
                  left: `calc(50% + ${node.x}px)`,
                  top: `calc(50% + ${node.y}px)`,
                }}
                onMouseEnter={() => setHoveredNode(node.id)}
                onMouseLeave={() => setHoveredNode(null)}
              >
                <div
                  class={`px-3 py-2 rounded-xl bg-bg-elevated/95 backdrop-blur-md border shadow-md flex items-center gap-2.5 whitespace-nowrap transition-all duration-200 cursor-pointer ${
                    node.isActive
                      ? node.isHitting
                        ? 'border-accent ring-2 ring-accent/40 shadow-accent/20 scale-105'
                        : 'border-subtle hover:border-accent/50 hover:scale-105'
                      : 'border-subtle/50 opacity-60 hover:opacity-100'
                  }`}
                >
                  <ProviderAvatar provider={node.provider} name={node.name} size="sm" class="shrink-0" />
                  <div class="min-w-0">
                    <div class="text-xs font-semibold text-foreground flex items-center gap-1.5">
                      {node.name || node.provider}
                      <Show when={node.isActive}>
                        <span class="w-1.5 h-1.5 rounded-full bg-success shrink-0" />
                      </Show>
                    </div>
                    <div class="text-[10px] text-faint font-mono truncate">
                      {node.authType || 'API Key'}
                    </div>
                  </div>

                  <Show when={node.isHitting}>
                    <Badge tone="green" class="text-[9px] px-1 py-0 ml-1 shrink-0 animate-pulse">
                      {node.recentLatency ? `${node.recentLatency}ms` : '响应中'}
                    </Badge>
                  </Show>
                </div>
              </div>
            )
          }}
        </For>
      </div>
    </Card>
  )
}
