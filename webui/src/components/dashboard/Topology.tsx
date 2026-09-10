import { type Component, For, Show, createSignal, createMemo, onMount, onCleanup } from 'solid-js'
import { A } from '@solidjs/router'
import { Card, Badge, ProviderAvatar } from '@/components/ui'
import type { Provider, LiveUsageEvent } from '@/types/domain'
import { useGatewayStore } from '@/stores/gateway'

interface TopologyProps {
  providers: Provider[]
  activeConnections?: number
  liveEvents?: LiveUsageEvent[]
}

export const GatewayTopology: Component<TopologyProps> = props => {
  const store = useGatewayStore()
  let containerRef: HTMLDivElement | undefined
  const [zoom, setZoom] = createSignal(1)
  const [pan, setPan] = createSignal({ x: 0, y: 0 })
  const [isDragging, setIsDragging] = createSignal(false)
  const [dragStart, setDragStart] = createSignal({ x: 0, y: 0 })
  const [hoveredNode, setHoveredNode] = createSignal<string | null>(null)

  const providers = createMemo(() => props.providers || [])
  const activeCount = createMemo(() => providers().filter(p => p.isActive).length)

  // 计算节点放射状环形排布坐标 (以中心 0,0 为原点)
  // 按 provider 唯一归属分组：同品牌合并为一个卡片，右侧动态显示当前激活账号/总账号数
  const nodePositions = createMemo(() => {
    const list = providers()
    if (list.length === 0) return []

    // 聚合同一 provider 的连接
    const map = new Map<string, {
      id: string
      provider: string
      name: string
      accounts: Provider[]
      activeAccount?: Provider
      isActive: boolean
      isHitting: boolean
      recentLatency?: number
    }>()

    for (const p of list) {
      const key = p.provider
      const existing = map.get(key)
      if (!existing) {
        map.set(key, {
          id: p.id,
          provider: p.provider,
          name: p.name || p.provider,
          accounts: [p],
          activeAccount: p.isActive ? p : undefined,
          isActive: p.isActive,
          isHitting: false,
        })
      } else {
        existing.accounts.push(p)
        // 优先采纳激活的账号
        if (p.isActive) {
          existing.isActive = true
          if (!existing.activeAccount) {
            existing.activeAccount = p
          }
        }
      }
    }

    const grouped = Array.from(map.values())
    const count = grouped.length
    if (count === 0) return []

    // 宽屏椭圆自适应布局：彻底消除“横向卡片间距过窄（仅14px）、纵向过长（142px）”的几何失真
    // 卡片为横向长方形 (约 204px 宽 × 54px 高)，水平方向补偿半宽之和以保持等距视觉留白
    const rx = Math.min(340, Math.max(280, 260 + count * 10))
    const ry = Math.min(185, Math.max(160, 145 + count * 5))

    return grouped.map((g, i) => {
      // 角度均匀分布（从 -90 度/正上方开始顺时针分布）
      const angle = (i / count) * 2 * Math.PI - Math.PI / 2
      const x = Math.round(Math.cos(angle) * rx)
      const y = Math.round(Math.sin(angle) * ry)
      const recentHit = (props.liveEvents || []).find(e =>
        e.provider === g.provider ||
        g.accounts.some(a => a.id === e.provider) ||
        (e.model && g.name && e.model.toLowerCase().includes(g.name.toLowerCase()))
      )

      return {
        ...g,
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
    <Card class="h-[460px] sm:h-[480px] overflow-hidden relative border border-subtle/80 bg-card/60 backdrop-blur-lg animate-fade-in select-none">
      {/* 顶部标题栏与状态指示 */}
      <div class="absolute top-4 left-4 right-4 z-20 flex items-center justify-between pointer-events-none">
        <div class="flex items-center gap-3 bg-bg/85 backdrop-blur-md px-3.5 py-2 rounded-xl border border-subtle shadow-sm pointer-events-auto">
          <div class="w-2.5 h-2.5 rounded-full bg-accent animate-pulse shadow-accent" />
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
                const cpx = Math.round(node.x * 0.48)
                const cpy = Math.round(node.y * 0.48)
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

          {/* 1. 中心枢纽：Cyrene Gateway */}
          <div
            class="absolute top-0 left-0 -translate-x-1/2 -translate-y-1/2 z-20 w-[208px] h-[58px] px-3.5 py-2.5 rounded-2xl bg-bg-elevated/95 backdrop-blur-xl border border-accent/40 shadow-xl shadow-accent/15 flex items-center gap-3 hover:scale-105 transition-all duration-300 cursor-default ring-1 ring-accent/20"
          >
            <div class="relative shrink-0">
              <img src="/icon.png" alt="Cyrene" class="w-7 h-7 rounded-xl object-contain shadow-md shadow-accent/20" />
              <span class="absolute -bottom-0.5 -right-0.5 w-2.5 h-2.5 rounded-full bg-accent ring-2 ring-bg-elevated animate-pulse" />
            </div>
            <div class="min-w-0 flex-1">
              <div class="font-bold text-xs tracking-tight text-foreground truncate flex items-center gap-1.5">
                <span>Cyrene Gateway</span>
              </div>
              <div class="text-[11px] text-accent/90 font-medium truncate flex items-center gap-1 mt-0.5">
                <span>核心调度枢纽</span>
                <span class="text-[10px] text-faint">· {activeCount()} 活跃通道</span>
              </div>
            </div>
          </div>

          {/* 2. 周围辐射排布的模型上游卡片 (同品牌合并，右侧动态显示激活账号) */}
          <For each={nodePositions()}>
            {node => {
              const reg = () => store.registryList().find(r => r.id === node.provider)
              const providerDisplayName = () => reg()?.name || node.provider
              const activeAcc = () => node.activeAccount || node.accounts[0]
              const accountSubtitle = () => {
                if (node.accounts.length > 1) {
                  const activeN = node.accounts.filter(a => a.isActive).length
                  return `${activeN}/${node.accounts.length} 账号活跃`
                }
                const acc = activeAcc()
                if (!acc) return '未配置'
                if (!acc.isActive) return '已停用'
                if (acc.email) return acc.email
                if (acc.name && acc.name.trim() !== providerDisplayName() && acc.name.trim().toLowerCase() !== node.provider.toLowerCase()) {
                  return acc.name
                }
                if (acc.data?.credentialHint) return String(acc.data.credentialHint)
                return acc.authType === 'api-key' ? 'API Key' : acc.authType === 'oauth' ? 'OAuth 授权' : (acc.authType || '活跃中')
              }

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
                  <A
                    href={`/providers/${node.id}`}
                    class={`w-[204px] h-[54px] px-3 py-2 rounded-xl bg-bg-elevated/95 backdrop-blur-md border shadow-md flex items-center gap-2.5 transition-all duration-200 cursor-pointer block no-underline ${
                      node.isActive
                        ? node.isHitting
                          ? 'border-accent ring-2 ring-accent/40 shadow-accent/25 scale-105'
                          : 'border-subtle hover:border-accent/50 hover:shadow-lg hover:-translate-y-0.5'
                        : 'border-subtle/50 opacity-60 hover:opacity-100'
                    }`}
                  >
                    <ProviderAvatar
                      provider={node.provider}
                      name={providerDisplayName()}
                      color={reg()?.color}
                      size="sm"
                      class="shrink-0"
                    />
                    <div class="min-w-0 flex-1">
                      <div class="text-xs font-semibold text-foreground flex items-center justify-between gap-1">
                        <span class="truncate">{providerDisplayName()}</span>
                        <div class="flex items-center gap-1.5 shrink-0">
                          <Show when={node.accounts.length > 1}>
                            <Badge tone="blue" class="text-[9px] px-1 py-0 font-mono">
                              {node.accounts.length}
                            </Badge>
                          </Show>
                          <span class={`w-2 h-2 rounded-full shrink-0 ${
                            node.isActive
                              ? (node.isHitting ? 'bg-accent animate-pulse shadow-accent' : 'bg-success')
                              : 'bg-zinc-600'
                          }`} />
                        </div>
                      </div>
                      <div class="text-[10px] text-faint font-mono truncate mt-0.5 flex items-center justify-between gap-1">
                        <span class="truncate">{accountSubtitle()}</span>
                        <Show when={node.isHitting && node.recentLatency}>
                          <span class="text-accent shrink-0 font-semibold">{node.recentLatency}ms</span>
                        </Show>
                      </div>
                    </div>
                  </A>
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
