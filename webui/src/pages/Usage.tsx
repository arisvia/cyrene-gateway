import { type Component, For, Show, Switch, Match, createSignal, createMemo, onMount, onCleanup } from 'solid-js'
import { useGatewayStore } from '@/stores/gateway'
import { useI18n } from '@/i18n'
import { Card, Badge, Button, IconButton, Select, Empty, Skeleton, LoadState, StatusPulse, PageHeader, SegmentedControl, TabTransition, IconEye } from '@/components/ui'
import { ProviderAvatar } from '@/components/ui/ProviderIcon'
import { GatewayTopology } from '@/components/dashboard/Topology'
import { RequestDetailModal } from '@/components/dashboard/RequestDetailModal'
import { formatNumber as fmtNum, formatCost as fmtCost, timeAgo as fmtTime, maskKey } from '@/lib/format'
import { getStoredSessionToken } from '@/lib/api'
import { fetchModelDisplayNameMap, resolveModelDisplayName } from '@/lib/models'
import type { RequestDetail, LiveUsageEvent, UsageDimensionItem } from '@/types/domain'

const Usage: Component = () => {
  const { t } = useI18n()
  const periods = () => [
    { value: '24h', label: t('usage.periods.p24h') },
    { value: '7d', label: t('usage.periods.p7d') },
    { value: '30d', label: t('usage.periods.p30d') },
  ]
  const store = useGatewayStore()
  const [subTab, setSubTab] = createSignal<'overview' | 'details'>('overview')
  const [selectedDetail, setSelectedDetail] = createSignal<RequestDetail | null>(null)
  const [period, setPeriod] = createSignal('7d')
  const [hoveredPoint, setHoveredPoint] = createSignal<{ label: string; tokens: number } | null>(null)
  const loading = () => store.usagePeriod() !== period()
  const [live, setLive] = createSignal(false)
  const [realtimeEvents, setRealtimeEvents] = createSignal<LiveUsageEvent[]>([])
  const [liveEvents, setLiveEvents] = createSignal<LiveUsageEvent[]>([])
  const [modelNameMap, setModelNameMap] = createSignal<Record<string, string>>({})
  let es: EventSource | null = null
  const timeUnits = () => ({
    justNow: t('time.justNow'),
    minutesAgo: (m: number) => t('time.minutesAgo', { m }),
    hoursAgo: (h: number) => t('time.hoursAgo', { h }),
    daysAgo: (d: number) => t('time.daysAgo', { d }),
  })

  function load() {
    setHoveredPoint(null)
    return store.loadUsage(period())
  }
  onMount(() => {
    load()
    store.loadRequestDetails(1, 10).then(() => {
      if (liveEvents().length === 0 && store.requestDetails().length > 0) {
        setLiveEvents(store.requestDetails().slice(0, 10).map(rd => ({
          timestamp: rd.timestamp,
          provider: rd.provider,
          model: rd.model,
          status: rd.status,
          promptTokens: rd.promptTokens,
          completionTokens: rd.completionTokens,
          latencyMs: rd.latencyMs,
          endpoint: rd.endpoint,
        })))
      }
    })
    fetchModelDisplayNameMap().then(setModelNameMap)
    toggleLive()
  })
  onCleanup(() => { es?.close(); es = null })

  function toggleLive() {
    if (live()) {
      es?.close()
      es = null
      setLive(false)
      return
    }

    try {
      const token = getStoredSessionToken()
      const streamUrl = token ? `/api/usage/stream?token=${encodeURIComponent(token)}` : '/api/usage/stream'
      es = new EventSource(streamUrl)
      const handleData = (ev: MessageEvent) => {
        try {
          const d = JSON.parse(ev.data)
          if (d && (d.model || d.provider || d.endpoint)) {
            setRealtimeEvents([d])
            setLiveEvents(list => [d, ...list].slice(0, 30))
          }
        } catch { /* 忽略心跳与解析错误 */ }
      }

      es.onmessage = handleData
      es.addEventListener('request', handleData as EventListener)
      es.addEventListener('routing', handleData as EventListener)
      es.addEventListener('connected', () => {
        setLive(true)
      })
      es.onopen = () => {
        setLive(true)
      }
      es.onerror = () => {
        // SSE 断开或重试
        if (es?.readyState === EventSource.CLOSED) {
          setLive(false)
          es?.close()
          es = null
        }
      }
      setLive(true)
    } catch (e: unknown) {
      console.error('[usage] failed to start live events stream:', e)
      setLive(false)
    }
  }

  const chart = () => store.usageChart()
  const maxTokens = createMemo(() => Math.max(1, ...chart().map(c => c.tokens || 0)))

  // 动态计算 X 轴刻度步长，防止 30 天 / 60 天大量标签挤压重叠
  const labelInterval = createMemo(() => {
    const len = chart().length
    if (len <= 8) return 1      // 24h / 7d：全量展示
    if (len <= 15) return 2     // 14d：每隔 1 天展示
    if (len <= 31) return 5     // 30d：每隔 5 天展示（首尾必显）
    return 10                   // 60d+：每隔 10 天展示
  })
  const kpis = createMemo(() => [
    { label: t('usage.totalRequests'), value: fmtNum(store.usageStats.totalRequests ?? 0) },
    { label: t('usage.promptTokens'), value: fmtNum(store.usageStats.totalPromptTokens ?? 0) },
    { label: t('usage.completionTokens'), value: fmtNum(store.usageStats.totalCompletionTokens ?? 0) },
    { label: t('usage.estimatedCost'), value: fmtCost(store.usageStats.totalCost ?? 0) },
  ])

  type BreakdownDimension = 'provider' | 'model' | 'key' | 'endpoint'
  type BreakdownMetric = 'requests' | 'totalTokens' | 'promptTokens' | 'completionTokens' | 'cost'

  const [dimension, setDimension] = createSignal<BreakdownDimension>('provider')
  const [metric, setMetric] = createSignal<BreakdownMetric>('requests')

  const breakdownItems = createMemo(() => {
    const stats = store.usageStats
    let raw: Record<string, UsageDimensionItem> | undefined
    const dim = dimension()
    if (dim === 'provider') raw = stats.byProvider
    else if (dim === 'model') raw = stats.byModel
    else if (dim === 'key') raw = stats.byKey
    else if (dim === 'endpoint') raw = stats.byEndpoint

    if (!raw) return []

    const m = metric()
    const items = Object.entries(raw).map(([key, v]) => {
      let displayName = key
      if (dim === 'model') {
        const parts = key.split('|')
        displayName = resolveModelDisplayName(modelNameMap(), parts[0]) || parts[0]
      } else if (dim === 'key' && key === 'default') {
        displayName = t('usage.defaultKeyLabel')
      }

      const totalTokens = (v.promptTokens || 0) + (v.completionTokens || 0)
      let val = 0
      if (m === 'requests') val = v.requests || 0
      else if (m === 'totalTokens') val = totalTokens
      else if (m === 'promptTokens') val = v.promptTokens || 0
      else if (m === 'completionTokens') val = v.completionTokens || 0
      else if (m === 'cost') val = v.cost || 0

      return {
        key,
        displayName,
        requests: v.requests || 0,
        promptTokens: v.promptTokens || 0,
        completionTokens: v.completionTokens || 0,
        totalTokens,
        cost: v.cost || 0,
        metricVal: val,
      }
    })

    return items.sort((a, b) => b.metricVal - a.metricVal)
  })

  const maxMetricVal = createMemo(() => Math.max(1, ...breakdownItems().map(i => i.metricVal)))

  function formatMetricValue(val: number, m: BreakdownMetric): string {
    if (m === 'cost') return fmtCost(val)
    if (m === 'requests') return `${fmtNum(val)}`
    return fmtNum(val)
  }

  return (
    <div class="space-y-5 stagger">
      <PageHeader
        title={t('usage.title')}
        subtitle={t('usage.subtitle', { count: fmtNum(store.usageStats.totalRequestsLifetime ?? 0) })}
        actions={
          <div class="flex items-center gap-2.5 flex-wrap">
            <SegmentedControl
              value={subTab()}
              onChange={setSubTab}
              options={[
                { value: 'overview', label: t('usage.overview') },
                { value: 'details', label: `${t('usage.requestDetails')} (${store.requestDetailsPagination().totalItems || 0})` },
              ]}
            />
            <Button variant={live() ? 'danger' : 'secondary'} size="sm" onClick={toggleLive} class="flex items-center gap-2">
              <StatusPulse
                status={live() ? 'active' : 'paused'}
                tone={live() ? 'red' : 'accent'}
                size="sm"
              />
              <span>{live() ? t('usage.stopLive') : t('usage.liveEvents')}</span>
            </Button>
          </div>
        }
      />

      <TabTransition
        value={subTab()}
        order={['overview', 'details']}
      >
        {tab => (
          <Switch>
            <Match when={tab === 'overview'}>
              <div class="space-y-5">
                <GatewayTopology
                  providers={store.providers()}
                  activeConnections={store.activeConnections()}
                  liveEvents={realtimeEvents()}
                />
                {/* 概览视图：KPI、Token 趋势、按提供商与实时事件 */}
        {/* KPI */}
        <LoadState ready={!loading()} error={store.loadErrors.usage} onRetry={load} fallback={
          <div class="grid sm:grid-cols-2 lg:grid-cols-4 gap-3">
            <For each={[0, 1, 2, 3]}>{() => <Card class="p-4"><Skeleton class="h-8 w-24" /></Card>}</For>
          </div>
        }>
          <div class="grid sm:grid-cols-2 lg:grid-cols-4 gap-3">
            <For each={kpis()}>
              {k => (
                <Card class="p-4">
                  <div class="text-xs text-faint">{k.label}</div>
                  <div class="text-2xl font-semibold mt-1 tabular-nums">{k.value}</div>
                </Card>
              )}
            </For>
          </div>
        </LoadState>
        {/* 图表 */}
        <Card class="p-5">
          <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-3 mb-4">
            <div class="min-w-0 flex-1">
              <div class="flex flex-col items-start justify-center gap-1 h-14 lg:flex-row lg:items-center lg:justify-start lg:gap-2 lg:h-6">
                <h3 class="text-sm font-semibold whitespace-nowrap">{t('usage.tokenTrend')}</h3>
                <span class={`block min-w-0 max-w-full h-6 transition-opacity duration-150 ${hoveredPoint() ? 'opacity-100' : 'opacity-0 pointer-events-none'}`}>
                  <Badge tone="blue" class="max-w-full text-xs font-mono px-2 py-0.5 whitespace-nowrap">
                    <span class="truncate">{hoveredPoint()?.label ?? ''} · {fmtNum(hoveredPoint()?.tokens ?? 0)} Tokens</span>
                  </Badge>
                </span>
              </div>
              <p class="text-xs text-faint mt-0.5 wrap-anywhere">
                {t('usage.chartAggregateHint', { peak: fmtNum(maxTokens()) })}
              </p>
            </div>
            <Select class="w-full sm:w-40 min-w-0 shrink-0" size="sm" value={period()} options={periods()} onChange={v => { setPeriod(v); load() }} align="right" />
          </div>

          <Show when={!loading()} fallback={<Show when={!store.loadErrors.usage}><Skeleton class="h-48 w-full" /></Show>}>
          <Show when={chart().length > 0} fallback={<Empty message={t('usage.noDataForPeriod')} />}>
            <div class="relative h-48 w-full flex flex-col justify-end pt-6 pb-1">
              {/* 背景水平参考基准线 */}
              <div class="absolute inset-x-0 top-6 bottom-7 flex flex-col justify-between pointer-events-none opacity-40">
                <div class="border-b border-dashed border-subtle w-full flex justify-end">
                  <span class="text-[9px] text-faint font-mono -mt-3.5 mr-1">{fmtNum(maxTokens())}</span>
                </div>
                <div class="border-b border-dashed border-subtle/50 w-full flex justify-end">
                  <span class="text-[9px] text-faint font-mono -mt-3.5 mr-1">{fmtNum(Math.round(maxTokens() / 2))}</span>
                </div>
                <div class="border-b border-subtle w-full" />
              </div>

              {/* 柱状主体：固定高度，flex-col 撑满，绝对防止子级高度崩塌 */}
              <div
                class="relative z-10 h-36 w-full flex items-stretch gap-1 sm:gap-1.5"
                onMouseLeave={() => setHoveredPoint(null)}
              >
                <For each={chart()}>
                  {(c, i) => {
                    const hasTokens = () => (c.tokens || 0) > 0
                    const heightPct = () => hasTokens() ? Math.max(5, Math.round(((c.tokens || 0) / maxTokens()) * 100)) : 0
                    const isVisible = () => i() === 0 || i() % labelInterval() === 0 || i() === chart().length - 1
                    const isHovered = () => hoveredPoint()?.label === c.label

                    return (
                      <div
                        class="flex-1 h-full flex flex-col items-center justify-end min-w-0 group cursor-pointer"
                        onMouseEnter={() => setHoveredPoint(c)}
                      >
                        {/* 柱状高度轨道 */}
                        <div class="relative w-full flex-1 flex items-end justify-center px-0.5">
                          <Show
                            when={hasTokens()}
                            fallback={
                              <div
                                class={`w-full max-w-[20px] h-[3px] rounded-full transition-colors ${
                                  isHovered() ? 'bg-accent' : 'bg-subtle/80 group-hover:bg-subtle'
                                }`}
                                title={`${c.label}: 0 tokens`}
                              />
                            }
                          >
                            <div
                              class={`w-full max-w-[22px] rounded-t transition-all duration-200 ${
                                isHovered()
                                  ? 'bg-accent shadow-sm shadow-accent/40 brightness-110'
                                  : 'bg-accent/75 group-hover:bg-accent'
                              }`}
                              style={{ height: `${heightPct()}%` }}
                              title={`${c.label}: ${fmtNum(c.tokens || 0)} tokens`}
                            />
                          </Show>
                        </div>

                        {/* X 轴日期标注：间距稀疏自适应，不展示文本的柱脚展示微圆点对齐 */}
                        <div class="h-6 w-full flex items-center justify-center pt-1.5 shrink-0">
                          <Show
                            when={isVisible()}
                            fallback={
                              <span
                                class={`w-1 h-1 rounded-full transition-colors ${
                                  isHovered() ? 'bg-accent scale-150' : 'bg-subtle/50 group-hover:bg-faint'
                                }`}
                              />
                            }
                          >
                            <span
                              class={`text-[10px] font-mono transition-colors truncate text-center ${
                                isHovered() ? 'text-accent font-semibold' : 'text-faint group-hover:text-foreground'
                              }`}
                            >
                              {c.label}
                            </span>
                          </Show>
                        </div>
                      </div>
                    )
                  }}
                </For>
              </div>
            </div>
          </Show>
          </Show>
        </Card>

        <div class="grid lg:grid-cols-2 gap-4">
          {/* 多维用量分析 */}
          <Card class="p-5 flex flex-col min-h-[380px]">
            <div class="flex items-center justify-between gap-3 mb-3">
              <div class="flex items-center gap-2 min-w-0">
                <h3 class="text-sm font-semibold truncate">{t('usage.breakdownTitle')}</h3>
                <span class="text-xs text-faint">({breakdownItems().length})</span>
              </div>
              <Select
                value={metric()}
                onChange={v => setMetric(v as BreakdownMetric)}
                size="sm"
                class="w-32 text-xs shrink-0"
                options={[
                  { value: 'requests', label: t('usage.metrics.requests') },
                  { value: 'totalTokens', label: t('usage.metrics.totalTokens') },
                  { value: 'promptTokens', label: t('usage.metrics.promptTokens') },
                  { value: 'completionTokens', label: t('usage.metrics.completionTokens') },
                  { value: 'cost', label: t('usage.metrics.cost') },
                ]}
              />
            </div>

            <div class="mb-3.5">
              <SegmentedControl
                value={dimension()}
                onChange={setDimension}
                size="sm"
                class="w-full"
                options={[
                  { value: 'provider', label: t('usage.dimensions.provider') },
                  { value: 'model', label: t('usage.dimensions.model') },
                  { value: 'key', label: t('usage.dimensions.key') },
                  { value: 'endpoint', label: t('usage.dimensions.endpoint') },
                ]}
              />
            </div>

            <Show when={!loading()} fallback={<Show when={!store.loadErrors.usage}><Skeleton class="h-48 w-full" /></Show>}>
              <Show when={breakdownItems().length > 0} fallback={<Empty message={t('common.noData')} />}>
                <div class="space-y-1.5 flex-1 overflow-y-auto max-h-[320px] px-0.5">
                  <For each={breakdownItems()}>
                    {item => (
                      <div
                        class="group flex items-center gap-3 text-xs py-2 px-2.5 rounded-lg hover:bg-surface/70 transition-colors"
                        title={`${item.displayName}\n${t('usage.metrics.requests')}: ${fmtNum(item.requests)}\n${t('usage.metrics.promptTokens')}: ${fmtNum(item.promptTokens)}\n${t('usage.metrics.completionTokens')}: ${fmtNum(item.completionTokens)}\n${t('usage.metrics.cost')}: ${fmtCost(item.cost)}`}
                      >
                        <div class="w-36 shrink-0 truncate flex items-center gap-2">
                          <Show when={dimension() === 'provider'}>
                            <ProviderAvatar provider={item.key} size="sm" class="w-5 h-5 rounded-md shrink-0" />
                          </Show>
                          <span class="truncate font-medium text-[11px]" title={item.displayName}>
                            {dimension() === 'key' ? (item.key === 'default' ? t('usage.defaultKeyLabel') : maskKey(item.key)) : item.displayName}
                          </span>
                        </div>

                        <div class="flex-1 min-w-0 h-1.5 rounded-full bg-control/70 overflow-hidden">
                          <div
                            class="h-full bg-accent transition-all duration-300 rounded-full"
                            style={{ width: `${Math.max(2, (item.metricVal / maxMetricVal()) * 100)}%` }}
                          />
                        </div>

                        <span class="w-20 shrink-0 text-right font-mono text-[11px] text-faint group-hover:text-foreground tabular-nums transition-colors">
                          {formatMetricValue(item.metricVal, metric())}
                        </span>
                      </div>
                    )}
                  </For>
                </div>
              </Show>
            </Show>
          </Card>

          {/* 实时事件 */}
          <Card class="p-5 flex flex-col min-h-[380px]">
            <div class="flex items-center justify-between mb-4">
              <div class="flex items-center gap-2">
                <h3 class="text-sm font-semibold">{t('usage.liveEvents')}</h3>
                <Show when={liveEvents().length > 0}>
                  <span class="text-xs text-faint">({liveEvents().length})</span>
                </Show>
              </div>
              <Show when={live()} fallback={<Badge tone="gray">{t('common.paused')}</Badge>}>
                <Badge tone="green">{t('usage.connecting')}</Badge>
              </Show>
            </div>

            <Show when={liveEvents().length > 0} fallback={<Empty message={live() ? t('usage.waitingEvents') : t('usage.clickLiveToListen')} />}>
              <div class="space-y-2 flex-1 max-h-[360px] overflow-y-auto px-1">
                <For each={liveEvents()}>
                  {e => (
                    <div class="flex flex-col gap-1.5 py-2 px-2.5 rounded-lg border border-subtle/50 bg-surface/30 hover:bg-surface/70 transition-colors">
                      <div class="flex items-center gap-2 text-xs min-w-0">
                        <span class="text-faint font-mono text-[10px] shrink-0">{fmtTime(e.timestamp, timeUnits())}</span>
                        <div class="truncate flex items-baseline gap-1" title={e.model || e.endpoint || '-'}>
                          <span class="font-medium text-foreground text-[11px]">{resolveModelDisplayName(modelNameMap(), e.model) || e.endpoint || '-'}</span>
                          <Show when={Boolean(e.model && resolveModelDisplayName(modelNameMap(), e.model) !== e.model)}>
                            <span class="text-[9px] text-faint font-mono truncate">({e.model})</span>
                          </Show>
                        </div>
                        <Badge
                          tone={e.status === 'ok' ? 'green' : e.status === 'routing' ? 'blue' : e.status === 'canceled' ? 'amber' : 'red'}
                          class="shrink-0 text-[10px] px-1.5 py-0"
                        >
                          {e.status || '-'}
                        </Badge>
                        <Show when={e.latencyMs != null}>
                          <span class="ml-auto font-mono text-[10px] text-faint shrink-0">{e.latencyMs}ms</span>
                        </Show>
                      </div>
                      <div class="flex items-center justify-between text-[10px] text-faint">
                        <span class="font-mono truncate max-w-[220px]" title={e.endpoint}>{e.endpoint || '/v1/chat/completions'}</span>
                        <Show when={(e.promptTokens || 0) > 0 || (e.completionTokens || 0) > 0}>
                          <div class="flex items-center gap-2 font-mono shrink-0">
                            <span>in: <span class="text-foreground">{fmtNum(e.promptTokens ?? 0)}</span></span>
                            <span>out: <span class="text-foreground">{fmtNum(e.completionTokens ?? 0)}</span></span>
                          </div>
                        </Show>
                      </div>
                    </div>
                  )}
                </For>
              </div>
            </Show>
          </Card>
        </div>
              </div>
            </Match>
            <Match when={tab === 'details'}>
              <Card class="p-5">
          <div class="flex flex-wrap items-center justify-between gap-2 mb-4">
            <h3 class="text-sm font-semibold">{t('usage.requestDetailsLog')}</h3>
            <span class="text-xs text-faint">{t('usage.totalRecords', { count: store.requestDetailsPagination().totalItems })}</span>
          </div>
          <Show when={store.requestDetails().length > 0} fallback={<Empty message={t('usage.noRecords')} />}>
            <div class="overflow-x-auto max-h-[max(12rem,calc(100dvh-320px))] overflow-y-auto px-1">
              <table class="w-full text-xs [&_th]:px-3 [&_td]:px-3 [&_th]:whitespace-nowrap [&_td]:align-middle">
                <thead>
                  <tr class="text-faint text-left border-b border-subtle">
                    <th class="pb-2.5 font-medium">{t('usage.tableHeaders.time')}</th>
                    <th class="pb-2.5 font-medium">{t('usage.tableHeaders.model')}</th>
                    <th class="pb-2.5 font-medium">{t('usage.tableHeaders.status')}</th>
                    <th class="pb-2.5 font-medium text-right">{t('usage.tableHeaders.prompt')}</th>
                    <th class="pb-2.5 font-medium text-right">{t('usage.tableHeaders.completion')}</th>
                    <th class="pb-2.5 font-medium text-right">{t('usage.tableHeaders.latency')}</th>
                    <th class="pb-2.5 font-medium text-right">{t('usage.tableHeaders.details')}</th>
                  </tr>
                </thead>
                <tbody>
                  <For each={store.requestDetails()}>
                    {d => (
                      <tr class="border-b border-subtle/40 last:border-0 hover:bg-hover/30 transition-colors">
                        <td class="py-2.5 text-faint font-mono">{fmtTime(d.timestamp, timeUnits())}</td>
                        <td class="py-2.5 max-w-[260px]">
                          <div class="flex flex-col min-w-0" title={d.model || '-'}>
                            <span class="font-medium text-foreground truncate">{resolveModelDisplayName(modelNameMap(), d.model) || d.model || '-'}</span>
                            <Show when={Boolean(d.model && resolveModelDisplayName(modelNameMap(), d.model) !== d.model)}>
                              <span class="text-[10px] text-faint font-mono truncate">{d.model}</span>
                            </Show>
                          </div>
                        </td>
                        <td class="py-2.5"><Badge tone={d.status === 'ok' ? 'green' : 'red'}>{d.status || '-'}</Badge></td>
                        <td class="py-2.5 text-right tabular-nums">{fmtNum(d.promptTokens ?? 0)}</td>
                        <td class="py-2.5 text-right tabular-nums">{fmtNum(d.completionTokens ?? 0)}</td>
                        <td class="py-2.5 text-right text-faint tabular-nums">{d.latencyMs ?? '-'}ms</td>
                        <td class="py-2.5 text-right">
                          <IconButton
                            size="sm"
                            variant="ghost"
                            type="button"
                            onClick={() => setSelectedDetail(d)}
                            title={t('usage.viewDetailTitle')}
                            aria-label={t('usage.viewDetailTitle')}
                          >
                            <IconEye size={14} />
                          </IconButton>
                        </td>
                      </tr>
                    )}
                  </For>
                </tbody>
              </table>
            </div>
            {/* 分页 */}
            <div class="flex flex-wrap items-center justify-between gap-3 mt-4 pt-3 border-t border-subtle/50">
              <span class="text-xs text-faint">
                {t('usage.totalRecords', { count: store.requestDetailsPagination().totalItems })}
              </span>
              <div class="flex items-center gap-2">
                <Button
                  size="sm" variant="ghost"
                  disabled={!store.requestDetailsPagination().hasPrev}
                  onClick={() => store.loadRequestDetails(store.requestDetailsPagination().page - 1, 10)}
                >{t('common.prevPage')}</Button>
                <span class="text-xs text-faint font-mono">
                  {store.requestDetailsPagination().page} / {Math.max(1, store.requestDetailsPagination().totalPages)}
                </span>
                <Button
                  size="sm" variant="ghost"
                  disabled={!store.requestDetailsPagination().hasNext}
                  onClick={() => store.loadRequestDetails(store.requestDetailsPagination().page + 1, 10)}
                >{t('common.nextPage')}</Button>
              </div>
            </div>
          </Show>
              </Card>
            </Match>
          </Switch>
        )}
      </TabTransition>

      {/* 请求明细详情弹窗 */}
      <RequestDetailModal
        item={selectedDetail()}
        onClose={() => setSelectedDetail(null)}
      />
    </div>
  )
}

export default Usage
