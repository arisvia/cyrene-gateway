import { A } from '@solidjs/router'
import { For, Show, createEffect, createMemo, createSignal, onCleanup, onMount, type JSX } from 'solid-js'
import { Badge, Card, CyreneLogo, IconButton, IconRotateCcw, ProviderAvatar } from '@/components/ui'
import { TopologyGraph } from '@/components/ui/TopologyGraph'
import { useI18n } from '@/i18n'
import { useGatewayStore } from '@/stores/gateway'
import type { LiveUsageEvent, Provider } from '@/types/domain'

interface TopologyProps {
  providers: Provider[]
  activeConnections?: number
  liveEvents?: LiveUsageEvent[]
}

export function GatewayTopology(props: TopologyProps): JSX.Element {
  const store = useGatewayStore()
  const { t } = useI18n()
  let containerRef: HTMLDivElement | undefined
  const [zoom, setZoom] = createSignal(1)
  const [reducedMotion, setReducedMotion] = createSignal(false)
  const [fitZoom, setFitZoom] = createSignal(1)
  const [pan, setPan] = createSignal({ x: 0, y: 0 })
  const [isDragging, setIsDragging] = createSignal(false)
  const [dragStart, setDragStart] = createSignal({ x: 0, y: 0 })
  const [hoveredNode, setHoveredNode] = createSignal<string | null>(null)
  const [activeHit, setActiveHit] = createSignal<{ provider: string; latencyMs?: number; time: number } | null>(null)
  let hitTimer: ReturnType<typeof setTimeout> | undefined

  // 新请求到达后保留 3 秒命中反馈。
  createEffect(() => {
    const events = props.liveEvents || []
    if (events.length === 0) return
    const latest = events[0]
    if (!latest || !latest.provider) return

    // 仅响应近期实时事件，容忍微小时钟偏差。
    if (latest.timestamp) {
      const age = Math.abs(Date.now() - new Date(latest.timestamp).getTime())
      if (age > 30000) return
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

    // 椭圆布局为横向卡片预留水平间距。
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
        (g.name !== '' && hit.provider.toLowerCase().includes(g.name.toLowerCase()))
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

  // 缩放范围为 30% ~ 200%，重置时恢复适配视窗的比例。
  function zoomIn(): void {
    setZoom(z => Math.min(2.0, Number((z + 0.15).toFixed(2))))
  }

  function zoomOut(): void {
    setZoom(z => Math.max(0.3, Number((z - 0.15).toFixed(2))))
  }

  function resetView(): void {
    setZoom(fitZoom())
    setPan({ x: 0, y: 0 })
  }

  // 鼠标拖拽平移
  function handleMouseDown(e: MouseEvent): void {
    if ((e.target as HTMLElement).closest('button')) return
    setIsDragging(true)
    setDragStart({ x: e.clientX - pan().x, y: e.clientY - pan().y })
  }

  function handleMouseMove(e: MouseEvent): void {
    if (!isDragging()) return
    setPan({ x: e.clientX - dragStart().x, y: e.clientY - dragStart().y })
  }

  function handleMouseUp(): void {
    setIsDragging(false)
  }

  // 滚轮缩放（以视窗绝对中心为原点）
  function handleWheel(e: WheelEvent): void {
    e.preventDefault()
    const delta = e.deltaY < 0 ? 0.08 : -0.08
    setZoom(z => Math.max(0.3, Math.min(2.0, Number((z + delta).toFixed(2)))))
  }

  onMount(() => {
    window.addEventListener('mouseup', handleMouseUp)
    const motion = window.matchMedia('(prefers-reduced-motion: reduce)')
    const updateMotion = () => setReducedMotion(motion.matches)
    updateMotion()
    motion.addEventListener('change', updateMotion)
    const observer = new ResizeObserver(entries => {
      const { width, height } = entries[0].contentRect
      const fit = Math.min(1, (width - 32) / 800, (height - 144) / 390)
      setFitZoom(Math.max(0.3, fit))
      setZoom(Math.max(0.3, fit))
      setPan({ x: 0, y: 0 })
    })
    if (containerRef) observer.observe(containerRef)
    onCleanup(() => {
      motion.removeEventListener('change', updateMotion)
      observer.disconnect()
    })
  })

  onCleanup(() => {
    window.removeEventListener('mouseup', handleMouseUp)
  })

  return (
    <Card class="h-[460px] sm:h-[480px] overflow-hidden relative border border-subtle/80 animate-fade-in select-none">
      {/* 顶部标题栏与状态指示 */}
      <div class="absolute top-4 left-4 right-4 z-20 flex items-center justify-between gap-4 pointer-events-none">
        <div class="pointer-events-auto">
          <h3 class="text-xs font-semibold flex flex-wrap items-center gap-2 text-foreground">
            {t('topology.title')}
            <Badge tone="green" class="text-xs! px-1.5 py-0">
              {t('topology.activeChannels', { count: activeCount() })}
            </Badge>
          </h3>
          <p class="text-xs text-faint">
            {t('topology.subtitle')}
          </p>
        </div>

        {/* 状态图例 */}
        <div class="hidden sm:flex shrink-0 items-center gap-3 text-xs text-faint pointer-events-auto">
          <span class="flex items-center gap-1.5">
            <span class="w-2 h-2 rounded-full bg-accent" /> {t('topology.legendRouting')}
          </span>
          <span class="flex items-center gap-1.5">
            <span class="w-2 h-2 rounded-full bg-success" /> {t('topology.legendActive')}
          </span>
          <span class="flex items-center gap-1.5">
            <span class="w-2 h-2 rounded-full bg-faint" /> {t('topology.legendInactive')}
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
          <TopologyGraph nodes={nodePositions()} hoveredNode={hoveredNode()} reducedMotion={reducedMotion()} />

          {/* 中心枢纽 */}
          <div
            class={`absolute top-0 left-0 -translate-x-1/2 -translate-y-1/2 z-20 w-[180px] h-[52px] px-3 py-2 rounded-2xl glass-float transition-all duration-300 cursor-default flex items-center gap-2.5 ${
              activeHit()
                ? 'border-accent ring-2 ring-ring-soft'
                : 'border-accent/30'
            }`}
          >
            <div class="relative shrink-0">
              <CyreneLogo class="w-6 h-6" />
              <span class={`absolute -bottom-0.5 -right-0.5 w-2 h-2 rounded-full ring-2 ring-bg-elevated ${
                activeHit() ? 'bg-accent' : 'bg-success'
              }`} />
            </div>
            <div class="min-w-0 flex-1">
              <div class="font-bold text-xs tracking-tight text-foreground truncate flex items-center gap-1">
                <span>Cyrene Gateway</span>
              </div>
              <div class={`text-xs font-medium flex items-center gap-1 ${
                activeHit() ? 'text-accent font-semibold' : 'text-accent/90'
              }`}>
                <span class="truncate">{activeHit() ? t('topology.routing') : t('topology.core')}</span>
                <span class="shrink-0 text-faint">{t('topology.activeCount', { count: activeCount() })}</span>
              </div>
            </div>
          </div>

          {/* 同品牌上游合并为一个卡片，显示激活账号。 */}
          <For each={nodePositions()}>
            {node => {
              const reg = () => store.registryList().find(r => r.id === node.provider)
              const providerDisplayName = () => reg()?.name || node.provider

              function accountSubtitle(): string {
                if (node.accounts.length > 1) {
                  const activeAccounts = node.accounts.filter(account => account.isActive).length
                  return t('topology.accountsActive', { active: activeAccounts, total: node.accounts.length })
                }
                const account = node.activeAccount || node.accounts[0]
                if (!account) return t('topology.notConfigured')
                if (!account.isActive) return t('common.disabled')
                if (account.email) return account.email
                if (account.name && account.name.trim() !== providerDisplayName() && account.name.trim().toLowerCase() !== node.provider.toLowerCase()) {
                  return account.name
                }
                if (account.data?.credentialHint) return String(account.data.credentialHint)
                if (account.authType === 'api-key') return 'API Key'
                if (account.authType === 'oauth') return t('topology.oauthAuth')
                return account.authType || t('topology.activeNow')
              }

              function nodeStateClass(): string {
                if (!node.isActive) return 'border-subtle/50 opacity-60 hover:opacity-100 focus-visible:opacity-100'
                if (node.isHitting) return 'border-accent ring-2 ring-ring-soft'
                if (hoveredNode() === node.id) return 'border-accent/60'
                return 'border-subtle hover:border-accent/50'
              }

              function statusClass(): string {
                if (!node.isActive) return 'bg-faint'
                if (node.isHitting) return 'bg-accent'
                return 'bg-success'
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
                    onFocus={() => setHoveredNode(node.id)}
                    onBlur={() => setHoveredNode(null)}
                    class={`w-[180px] h-[52px] px-3 py-2 rounded-2xl glass-float border shadow-sm flex items-center gap-2.5 transition-all duration-300 cursor-pointer no-underline ${nodeStateClass()}`}
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
                        <span class="relative flex h-2.5 w-2.5 shrink-0 items-center justify-center">
                          <Show when={node.isActive && node.isHitting && !reducedMotion()}>
                            <span class="animate-pulse absolute inline-flex h-full w-full rounded-full bg-accent opacity-50" />
                          </Show>
                          <span class={`relative inline-flex rounded-full h-2 w-2 ${statusClass()}`} />
                        </span>
                      </div>
                      <div class="text-xs text-faint font-mono truncate mt-0.5 flex items-center justify-between gap-1">
                        <span class="truncate">{accountSubtitle()}</span>
                        <Show when={node.isHitting && node.recentLatency}>
                          <span class="text-accent shrink-0 font-semibold font-mono">{node.recentLatency}ms</span>
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
        <IconButton onClick={zoomIn} title={t('topology.zoomIn')} aria-label={t('topology.zoomIn')}>+</IconButton>
        <IconButton onClick={zoomOut} title={t('topology.zoomOut')} aria-label={t('topology.zoomOut')}>−</IconButton>
        <IconButton onClick={resetView} title={t('topology.reset')} aria-label={t('topology.reset')}>
          <IconRotateCcw size={14} />
        </IconButton>
        <span class="text-[10px] text-faint px-1.5 tabular-nums font-mono">
          {Math.round(zoom() * 100)}%
        </span>
      </div>
    </Card>
  )
}
