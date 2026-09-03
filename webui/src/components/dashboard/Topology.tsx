import { type Component, For, Show, createSignal, createMemo, onMount, onCleanup } from 'solid-js'
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
  let containerRef: HTMLDivElement | undefined
  const [zoom, setZoom] = createSignal(1)
  const [pan, setPan] = createSignal({ x: 0, y: 0 })
  const [isDragging, setIsDragging] = createSignal(false)
  const [dragStart, setDragStart] = createSignal({ x: 0, y: 0 })
  const [hoveredNode, setHoveredNode] = createSignal<string | null>(null)

  const providers = createMemo(() => props.providers || [])
  const activeCount = createMemo(() => providers().filter(p => p.isActive).length)

  // 计算节点放射状环形排布坐标 (以中心 0,0 为原点)
  const nodePositions = createMemo(() => {
    const list = providers()
    const count = list.length
    if (count === 0) return []

    // 基础半径自适应
    const radius = Math.min(220, Math.max(140, 120 + count * 10))

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

  // 缩放控制（限制 50% ~ 200%）
  const zoomIn = () => setZoom(z => Math.min(2.0, Number((z + 0.15).toFixed(2))))
  const zoomOut = () => setZoom(z => Math.max(0.5, Number((z - 0.15).toFixed(2))))
  const resetView = () => {
    setZoom(1)
    setPan({ x: 0, y: 0 })
  }

  // 鼠标拖拽平移
  const handleMouseDown = (e: MouseEvent) => {
    if ((e.target as HTMLElement).closest('button')) return
    setIsDragging(true)
    setDragStart({ x: e.clientX - pan().x, y: e.clientY - pan().y })
  }

  const handleMouseMove = (e: MouseEvent) => {
    if (!isDragging()) return
    setPan({ x: e.clientX - dragStart().x, y: e.clientY - dragStart().y })
  }

  const handleMouseUp = () => setIsDragging(false)

  // 滚轮缩放（以视窗绝对中心为原点）
  const handleWheel = (e: WheelEvent) => {
    e.preventDefault()
    const delta = e.deltaY < 0 ? 0.08 : -0.08
    setZoom(z => Math.max(0.5, Math.min(2.0, Number((z + delta).toFixed(2)))))
  }

  onMount(() => {
    window.addEventListener('mouseup', handleMouseUp)
  })

  onCleanup(() => {
    window.removeEventListener('mouseup', handleMouseUp)
  })

  return (
    <Card class="h-[460px] sm:h-[480px] overflow-hidden relative border border-subtle/80 bg-card/60 backdrop-blur-xl animate-fade-in select-none">
      {/* 顶部标题栏与状态指示 */}
      <div class="absolute top-4 left-4 right-4 z-20 flex items-center justify-between pointer-events-none">
        <div class="flex items-center gap-3 bg-bg/85 backdrop-blur-md px-3.5 py-2 rounded-xl border border-subtle shadow-sm pointer-events-auto">
          <div class="w-2.5 h-2.5 rounded-full bg-accent animate-pulse shadow-accent shadow-sm" />
          <div>
            <h3 class="text-xs font-semibold flex items-center gap-2 text-foreground">
              实时路由拓扑
              <Badge tone="green" class="text-[10px] px-1.5 py-0">
                {activeCount()} 活跃通道
              </Badge>
            </h3>
            <p class="text-[11px] text-faint">
              支持无限平移与缩放 · 实时监控流量路由分发
            </p>
          </div>
        </div>

        {/* 状态图例 */}
        <div class="hidden sm:flex items-center gap-3 text-[11px] text-faint bg-bg/85 backdrop-blur-md px-3 py-1.5 rounded-xl border border-subtle pointer-events-auto">
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

      {/* 点阵网格背景 (随平移无缝滚动) */}
      <div
        class="absolute inset-0 pointer-events-none opacity-30 dark:opacity-15"
        style={{
          'background-image': 'radial-gradient(currentColor 1px, transparent 1px)',
          'background-size': '24px 24px',
          'background-position': `${pan().x}px ${pan().y}px`,
        }}
      />

      {/* 画布拖拽与视窗视口 */}
      <div
        ref={containerRef}
        class="w-full h-full relative cursor-grab active:cursor-grabbing overflow-hidden flex items-center justify-center"
        onMouseDown={handleMouseDown}
        onMouseMove={handleMouseMove}
        onWheel={handleWheel}
      >
        {/* 核心缩放图层：严格以 (0,0) 为几何中心对称放大/缩小 */}
        <div
          class="absolute top-1/2 left-1/2 w-0 h-0 transition-transform duration-75 ease-out"
          style={{
            transform: `translate(${pan().x}px, ${pan().y}px) scale(${zoom()})`,
            'transform-origin': '0 0',
          }}
        >
          {/* SVG 曲线连接层 (贝塞尔平滑流向曲线 + 粒子脉冲) */}
          <svg
            class="absolute top-0 left-0 -translate-x-1/2 -translate-y-1/2 w-[1200px] h-[1200px] pointer-events-none overflow-visible -z-10"
            viewBox="-600 -600 1200 1200"
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
            class="absolute top-0 left-0 -translate-x-1/2 -translate-y-1/2 z-20 px-4 py-2.5 rounded-xl bg-bg-elevated/95 backdrop-blur-xl border border-accent/40 shadow-xl shadow-accent/15 flex items-center gap-2.5 justify-center hover:scale-105 transition-transform duration-200 cursor-default"
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
                    left: `${node.x}px`,
                    top: `${node.y}px`,
                  }}
                  onMouseEnter={() => setHoveredNode(node.id)}
                  onMouseLeave={() => setHoveredNode(null)}
                >
                  <div
                    class={`w-44 h-12 px-3 py-2 rounded-xl bg-bg-elevated/95 backdrop-blur-md border shadow-md flex items-center gap-2.5 transition-all duration-200 cursor-pointer ${
                      node.isActive
                        ? node.isHitting
                          ? 'border-accent ring-2 ring-accent/40 shadow-accent/20 scale-105'
                          : 'border-subtle hover:border-accent/50 hover:scale-105'
                        : 'border-subtle/50 opacity-60 hover:opacity-100'
                    }`}
                  >
                    <ProviderAvatar provider={node.provider} name={node.name} size="sm" class="shrink-0" />
                    <div class="min-w-0 flex-1">
                      <div class="text-xs font-semibold text-foreground flex items-center gap-1.5 truncate">
                        <span class="truncate">{node.name || node.provider}</span>
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
      </div>

      {/* 左下角视窗控制器 (缩放/重置) */}
      <div class="absolute bottom-4 left-4 z-20 flex items-center gap-1 bg-bg/85 backdrop-blur-md p-1 rounded-xl border border-subtle shadow-sm">
        <button
          type="button"
          class="w-7 h-7 flex items-center justify-center rounded-lg text-muted hover:text-foreground hover:bg-hover transition-colors text-sm font-bold"
          onClick={zoomIn}
          title="放大 (+)"
        >
          +
        </button>
        <button
          type="button"
          class="w-7 h-7 flex items-center justify-center rounded-lg text-muted hover:text-foreground hover:bg-hover transition-colors text-sm font-bold"
          onClick={zoomOut}
          title="缩小 (−)"
        >
          −
        </button>
        <button
          type="button"
          class="w-7 h-7 flex items-center justify-center rounded-lg text-muted hover:text-foreground hover:bg-hover transition-colors text-xs"
          onClick={resetView}
          title="重置视图"
        >
          <svg class="w-3.5 h-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M3 12a9 9 0 1 0 9-9 9.75 9.75 0 0 0-6.74 2.74L3 8" />
            <path d="M3 3v5h5" />
          </svg>
        </button>
        <span class="text-[10px] text-faint px-1.5 tabular-nums font-mono">
          {Math.round(zoom() * 100)}%
        </span>
      </div>
    </Card>
  )
}
