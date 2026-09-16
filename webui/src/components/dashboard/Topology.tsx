import { type Component, For, Show, createSignal, createMemo, createEffect, onMount, onCleanup } from 'solid-js'
import { A } from '@solidjs/router'
import { Card, Badge, ProviderAvatar, CyreneLogo, IconRotateCcw } from '@/components/ui'
import type { Provider, LiveUsageEvent } from '@/types/domain'
import { useGatewayStore } from '@/stores/gateway'
import { useI18n } from '@/i18n'

interface TopologyProps {
  providers: Provider[]
  activeConnections?: number
  liveEvents?: LiveUsageEvent[]
}

export const GatewayTopology: Component<TopologyProps> = props => {
  const store = useGatewayStore()
  const { t } = useI18n()
  let containerRef: HTMLDivElement | undefined
  const [zoom, setZoom] = createSignal(1)
  const [pan, setPan] = createSignal({ x: 0, y: 0 })
  const [isDragging, setIsDragging] = createSignal(false)
  const [dragStart, setDragStart] = createSignal({ x: 0, y: 0 })
  const [hoveredNode, setHoveredNode] = createSignal<string | null>(null)
  const [activeHit, setActiveHit] = createSignal<{ provider: string; latencyMs?: number; time: number } | null>(null)
  let hitTimer: ReturnType<typeof setTimeout> | undefined

  // 仅在真实到达新请求时触发 3 秒激光脉冲，随后平滑恢复常态
  createEffect(() => {
    const events = props.liveEvents || []
    if (events.length === 0) return
    const latest = events[0]
    if (!latest || !latest.provider) return

    // 仅针对 5 秒内生成的实时事件触发脉冲
    if (latest.timestamp) {
      const age = Date.now() - new Date(latest.timestamp).getTime()
      if (age > 5000) return
    }

    setActiveHit({
      provider: latest.provider,
      latencyMs: latest.latencyMs,
      time: Date.now(),
    })

    clearTimeout(hitTimer)
    hitTimer = setTimeout(() => {
      setActiveHit(null)
    }, 3000)
  })

  onCleanup(() => {
    clearTimeout(hitTimer)
  })

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
    const rx = Math.min(290, Math.max(240, 225 + count * 8))
    const ry = Math.min(165, Math.max(140, 130 + count * 4))

    return grouped.map((g, i) => {
      // 角度均匀分布（从 -90 度/正上方开始顺时针分布）
      const angle = (i / count) * 2 * Math.PI - Math.PI / 2
      const x = Math.round(Math.cos(angle) * rx)
      const y = Math.round(Math.sin(angle) * ry)
      const hit = activeHit()
      const isHitting = hit ? (
        hit.provider === g.provider ||
        g.accounts.some(a => a.id === hit.provider) ||
        (g.name && hit.provider.toLowerCase().includes(g.name.toLowerCase()))
      ) : false

      return {
        ...g,
        x,
        y,
        isHitting,
        recentLatency: isHitting ? hit?.latencyMs : undefined,
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
    <Card class="h-[460px] sm:h-[480px] overflow-hidden relative border border-subtle/80 animate-fade-in select-none">
      {/* 顶部标题栏与状态指示 */}
      <div class="absolute top-4 left-4 right-4 z-20 flex items-center justify-between pointer-events-none">
        <div class="flex items-center gap-3 glass-float px-3.5 py-2 rounded-xl pointer-events-auto">
          <div class="w-2.5 h-2.5 rounded-full bg-accent animate-pulse shadow-accent" />
          <div>
            <h3 class="text-xs font-semibold flex items-center gap-2 text-foreground">
              {t('topology.title')}
              <Badge tone="green" class="text-[10px] px-1.5 py-0">
                {t('topology.activeChannels', { count: activeCount() })}
              </Badge>
            </h3>
            <p class="text-[11px] text-faint">
              {t('topology.subtitle')}
            </p>
          </div>
        </div>

        {/* 状态图例 */}
        <div class="hidden sm:flex items-center gap-3 text-[11px] text-faint glass-float px-3 py-1.5 rounded-xl pointer-events-auto">
          <span class="flex items-center gap-1.5">
            <span class="w-2 h-2 rounded-full bg-accent animate-pulse shadow-accent" /> {t('topology.legendRouting')}
          </span>
          <span class="flex items-center gap-1.5">
            <span class="w-2 h-2 rounded-full bg-success" /> {t('topology.legendActive')}
          </span>
          <span class="flex items-center gap-1.5">
            <span class="w-2 h-2 rounded-full bg-zinc-600" /> {t('topology.legendInactive')}
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
          {/* SVG 曲线连接层 (全向对称贝塞尔平滑流向曲线 + 高能激光能量流) */}
          <svg
            class="absolute top-0 left-0 -translate-x-1/2 -translate-y-1/2 w-[1200px] h-[1200px] pointer-events-none overflow-visible -z-10"
            viewBox="-600 -600 1200 1200"
          >
            <defs>
              {/* 全息虹彩高能激光流渐变 (洋红 -> 紫罗兰 -> 天青 -> 薄荷绿 -> 暖阳金) */}
              <linearGradient id="laserStreamGrad" x1="0%" y1="0%" x2="100%" y2="100%">
                <stop offset="0%" stop-color="#ec4899" />
                <stop offset="22%" stop-color="#818cf8" />
                <stop offset="48%" stop-color="#06b6d4" />
                <stop offset="76%" stop-color="#10b981" />
                <stop offset="100%" stop-color="#fbbf24" />
              </linearGradient>

              {/* 网关中心光粒子喷涌渐变 */}
              <radialGradient id="gatewayBurstGrad" cx="50%" cy="50%" r="50%">
                <stop offset="0%" stop-color="#fbbf24" stop-opacity="0.9" />
                <stop offset="50%" stop-color="#ec4899" stop-opacity="0.5" />
                <stop offset="100%" stop-color="#38bdf8" stop-opacity="0" />
              </radialGradient>

              {/* 激光辉光滤镜 */}
              <filter id="laserGlow" x="-50%" y="-50%" width="200%" height="200%">
                <feGaussianBlur in="SourceGraphic" stdDeviation="3.5" result="blur" />
                <feMerge>
                  <feMergeNode in="blur" />
                  <feMergeNode in="SourceGraphic" />
                </feMerge>
              </filter>
            </defs>

            <For each={nodePositions()}>
              {node => {
                const isHovered = () => hoveredNode() === node.id
                const absX = Math.abs(node.x)
                const absY = Math.abs(node.y)
                let d = ''
                if (absY >= absX) {
                  // 纵向为主（上方/下方节点）：S 弯平滑连接，x 为 0 时添加自然弧度偏移
                  const arcX = node.x === 0 ? 18 : 0
                  const cp1x = Math.round(node.x * 0.25 + arcX)
                  const cp1y = Math.round(node.y * 0.5)
                  const cp2x = Math.round(node.x * 0.85 + (node.x === 0 ? 8 : 0))
                  const cp2y = Math.round(node.y * 0.6)
                  d = `M 0 0 C ${cp1x} ${cp1y}, ${cp2x} ${cp2y}, ${node.x} ${node.y}`
                } else {
                  // 横向为主（左侧/右侧节点）：S 弯平滑连接，y 为 0 时添加自然弧度偏移
                  const arcY = node.y === 0 ? 14 : 0
                  const cp1x = Math.round(node.x * 0.5)
                  const cp1y = Math.round(node.y * 0.25 + arcY)
                  const cp2x = Math.round(node.x * 0.6)
                  const cp2y = Math.round(node.y * 0.85 + (node.y === 0 ? 6 : 0))
                  d = `M 0 0 C ${cp1x} ${cp1y}, ${cp2x} ${cp2y}, ${node.x} ${node.y}`
                }

                return (
                  <g class="transition-opacity duration-200">
                    {/* 底层基础导轨线 */}
                    <path
                      d={d}
                      fill="none"
                      stroke={node.isActive ? 'rgba(45, 212, 191, 0.35)' : 'rgba(150, 150, 150, 0.2)'}
                      stroke-width={node.isActive ? 2 : 1.5}
                      stroke-dasharray={node.isActive ? 'none' : '4 4'}
                    />

                    {/* 激活命中或悬停时的密集激光能量流 (Laser Energy Stream) */}
                    <Show when={node.isHitting || isHovered()}>
                      {/* 1. 外层霓虹漫反射扩散光晕 */}
                      <path
                        d={d}
                        fill="none"
                        stroke="url(#laserStreamGrad)"
                        stroke-width="7"
                        stroke-linecap="round"
                        opacity={node.isHitting ? 0.5 : 0.38}
                        filter="url(#laserGlow)"
                      />
                      {/* 2. 核心高速流动的激光能量段流 (Segmented Laser Stream) */}
                      <path
                        d={d}
                        fill="none"
                        stroke="url(#laserStreamGrad)"
                        stroke-width="3.5"
                        stroke-linecap="round"
                        stroke-dasharray="12 8 22 8 8 8"
                        filter="url(#laserGlow)"
                      >
                        <animate
                          attributeName="stroke-dashoffset"
                          from="0"
                          to="-66"
                          dur="0.85s"
                          repeatCount="indefinite"
                        />
                      </path>
                      {/* 3. 三重错相密集彩色能量粒子流 (全息彩色能谱激光流) */}
                      {/* 领头金阳粒子 */}
                      <circle r="4.5" fill="#fbbf24" filter="url(#laserGlow)">
                        <animateMotion path={d} dur="0.85s" repeatCount="indefinite" begin="0s" />
                      </circle>
                      {/* 中继天青粒子 */}
                      <circle r="4" fill="#38bdf8" filter="url(#laserGlow)">
                        <animateMotion path={d} dur="0.85s" repeatCount="indefinite" begin="-0.28s" />
                      </circle>
                      {/* 尾部霓虹洋红粒子 */}
                      <circle r="3.5" fill="#ec4899" filter="url(#laserGlow)">
                        <animateMotion path={d} dur="0.85s" repeatCount="indefinite" begin="-0.56s" />
                      </circle>
                    </Show>
                  </g>
                )
              }}
            </For>
            {/* 网关核心命中时的光粒子喷涌与共振能量冲击波 */}
            <Show when={activeHit()}>
              <g class="pointer-events-none">
                {/* 1. 双重共振能量扩散冲击波环 */}
                <circle cx="0" cy="0" r="32" fill="none" stroke="url(#laserStreamGrad)" stroke-width="2.5" filter="url(#laserGlow)">
                  <animate attributeName="r" values="28;115" dur="0.85s" repeatCount="indefinite" />
                  <animate attributeName="opacity" values="0.85;0" dur="0.85s" repeatCount="indefinite" />
                  <animate attributeName="stroke-width" values="3.5;0.5" dur="0.85s" repeatCount="indefinite" />
                </circle>
                <circle cx="0" cy="0" r="32" fill="none" stroke="#f59e0b" stroke-width="2" filter="url(#laserGlow)">
                  <animate attributeName="r" values="28;135" dur="0.85s" begin="0.42s" repeatCount="indefinite" />
                  <animate attributeName="opacity" values="0.75;0" dur="0.85s" begin="0.42s" repeatCount="indefinite" />
                  <animate attributeName="stroke-width" values="2.5;0.5" dur="0.85s" begin="0.42s" repeatCount="indefinite" />
                </circle>

                {/* 2. 全向辐射散射的光粒子流簇 (8 向辐射光子) */}
                <For each={[
                  { dx: 65, dy: 0, c: '#fbbf24', delay: '0s' },
                  { dx: 46, dy: 46, c: '#38bdf8', delay: '0.1s' },
                  { dx: 0, dy: 65, c: '#ec4899', delay: '0.2s' },
                  { dx: -46, dy: 46, c: '#10b981', delay: '0.3s' },
                  { dx: -65, dy: 0, c: '#fbbf24', delay: '0.4s' },
                  { dx: -46, dy: -46, c: '#818cf8', delay: '0.5s' },
                  { dx: 0, dy: -65, c: '#06b6d4', delay: '0.6s' },
                  { dx: 46, dy: -46, c: '#f43f5e', delay: '0.7s' },
                ]}>
                  {spark => (
                    <circle cx="0" cy="0" r="3" fill={spark.c} filter="url(#laserGlow)">
                      <animate attributeName="cx" values={`0;${spark.dx * 1.5}`} dur="0.75s" begin={spark.delay} repeatCount="indefinite" />
                      <animate attributeName="cy" values={`0;${spark.dy * 1.5}`} dur="0.75s" begin={spark.delay} repeatCount="indefinite" />
                      <animate attributeName="opacity" values="1;0.8;0" dur="0.75s" begin={spark.delay} repeatCount="indefinite" />
                      <animate attributeName="r" values="3.5;2;0.5" dur="0.75s" begin={spark.delay} repeatCount="indefinite" />
                    </circle>
                  )}
                </For>
              </g>
            </Show>
          </svg>

          {/* 1. 中心枢纽：Cyrene Gateway (命中时激活温暖金橙色光晕) */}
          <div
            class={`absolute top-0 left-0 -translate-x-1/2 -translate-y-1/2 z-20 w-[168px] h-[48px] px-2.5 py-1.5 rounded-xl glass-float transition-all duration-300 cursor-default flex items-center gap-2 ${
              activeHit()
                ? 'border-amber-400 ring-2 ring-amber-400/40 shadow-[0_0_32px_rgba(245,158,11,0.45)] scale-105 animate-gateway-jitter'
                : 'border-accent/40 shadow-lg shadow-accent/15 ring-1 ring-accent/20 hover:scale-105'
            }`}
          >
            <div class="relative shrink-0">
              <CyreneLogo class="w-6 h-6" pulsing={Boolean(activeHit())} />
              <span class={`absolute -bottom-0.5 -right-0.5 w-2 h-2 rounded-full ring-2 ring-bg-elevated ${
                activeHit() ? 'bg-amber-400 animate-ping shadow-[0_0_8px_#f59e0b]' : 'bg-accent animate-pulse'
              }`} />
            </div>
            <div class="min-w-0 flex-1">
              <div class="font-bold text-xs tracking-tight text-foreground truncate flex items-center gap-1">
                <span>Cyrene Gateway</span>
              </div>
              <div class={`text-[10px] font-medium truncate flex items-center gap-1 ${
                activeHit() ? 'text-amber-500 dark:text-amber-400 font-semibold' : 'text-accent/90'
              }`}>
                <span>{activeHit() ? t('topology.routing') : t('topology.core')}</span>
                <span class="text-[9px] text-faint">{t('topology.activeCount', { count: activeCount() })}</span>
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
                  return t('topology.accountsActive', { active: activeN, total: node.accounts.length })
                }
                const acc = activeAcc()
                if (!acc) return t('topology.notConfigured')
                if (!acc.isActive) return t('common.disabled')
                if (acc.email) return acc.email
                if (acc.name && acc.name.trim() !== providerDisplayName() && acc.name.trim().toLowerCase() !== node.provider.toLowerCase()) {
                  return acc.name
                }
                if (acc.data?.credentialHint) return String(acc.data.credentialHint)
                return acc.authType === 'api-key' ? 'API Key' : acc.authType === 'oauth' ? t('topology.oauthAuth') : (acc.authType || t('topology.activeNow'))
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
                    class={`w-[164px] h-[46px] px-2.5 py-1.5 rounded-xl glass-float border shadow-sm flex items-center gap-2 transition-all duration-300 cursor-pointer block no-underline ${
                      node.isActive
                        ? node.isHitting
                          ? 'border-amber-400 ring-2 ring-amber-400/50 shadow-[0_0_26px_rgba(245,158,11,0.38)] scale-105 bg-amber-500/5'
                          : hoveredNode() === node.id
                            ? 'border-accent ring-2 ring-accent/40 shadow-accent/25 scale-105 bg-accent/5'
                            : 'border-subtle hover:border-accent/50 hover:shadow-md hover:-translate-y-0.5'
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
                              ? (node.isHitting ? 'bg-amber-400 animate-pulse shadow-[0_0_8px_#f59e0b]' : 'bg-success')
                              : 'bg-zinc-600'
                          }`} />
                        </div>
                      </div>
                      <div class="text-[10px] text-faint font-mono truncate mt-0.5 flex items-center justify-between gap-1">
                        <span class="truncate">{accountSubtitle()}</span>
                        <Show when={node.isHitting && node.recentLatency}>
                          <span class="text-amber-500 dark:text-amber-400 shrink-0 font-semibold font-mono">{node.recentLatency}ms</span>
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
      <div class="absolute bottom-4 left-4 z-20 flex items-center gap-1 glass-float p-1 rounded-xl">
        <button
          type="button"
          class="w-7 h-7 flex items-center justify-center rounded-lg text-muted hover:text-foreground hover:bg-hover transition-colors text-sm font-bold"
          onClick={zoomIn}
          title={t('topology.zoomIn')}
        >
          +
        </button>
        <button
          type="button"
          class="w-7 h-7 flex items-center justify-center rounded-lg text-muted hover:text-foreground hover:bg-hover transition-colors text-sm font-bold"
          onClick={zoomOut}
          title={t('topology.zoomOut')}
        >
          −
        </button>
        <button
          type="button"
          class="w-7 h-7 flex items-center justify-center rounded-lg text-muted hover:text-foreground hover:bg-hover transition-colors text-xs"
          onClick={resetView}
          title={t('topology.reset')}
        >
          <IconRotateCcw size={14} />
        </button>
        <span class="text-[10px] text-faint px-1.5 tabular-nums font-mono">
          {Math.round(zoom() * 100)}%
        </span>
      </div>
    </Card>
  )
}
