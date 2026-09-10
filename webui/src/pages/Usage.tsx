import { type Component, For, Show, createSignal, createMemo, onMount, onCleanup } from 'solid-js'
import { useGatewayStore } from '@/stores/gateway'
import { Card, Badge, Button, Select, Empty, Skeleton, StatusPulse, PageHeader, SegmentedControl } from '@/components/ui'
import { GatewayTopology } from '@/components/dashboard/Topology'
import { RequestDetailModal } from '@/components/dashboard/RequestDetailModal'
import { formatNumber as fmtNum, formatCost as fmtCost, timeAgo as fmtTime } from '@/lib/format'
import type { RequestDetail, LiveUsageEvent } from '@/types/domain'
const PERIODS = [
  { value: '24h', label: '最近 24 小时' },
  { value: '7d', label: '最近 7 天' },
  { value: '30d', label: '最近 30 天' },
]

const Usage: Component = () => {
  const store = useGatewayStore()
  const [subTab, setSubTab] = createSignal<'overview' | 'details'>('overview')
  const [selectedDetail, setSelectedDetail] = createSignal<RequestDetail | null>(null)
  const [period, setPeriod] = createSignal('7d')
  const [hoveredPoint, setHoveredPoint] = createSignal<{ label: string; tokens: number } | null>(null)
  const [loading, setLoading] = createSignal(true)
  const [live, setLive] = createSignal(false)
  const [liveEvents, setLiveEvents] = createSignal<LiveUsageEvent[]>([])
  let es: EventSource | null = null

  async function load() {
    setLoading(true)
    try { await store.loadUsage(period()) } finally { setLoading(false) }
  }
  onMount(() => {
    load()
    store.loadRequestDetails(1, 10)
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

      es = new EventSource('/api/usage/stream')
      const handleData = (ev: MessageEvent) => {
        try {
          const d = JSON.parse(ev.data)
          if (d && (d.model || d.provider || d.endpoint)) {
            setLiveEvents(list => [d, ...list].slice(0, 30))
          }
        } catch { /* 忽略心跳与解析错误 */ }
      }

      es.onmessage = handleData
      es.addEventListener('request', handleData as EventListener)
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
    { label: '总请求', value: fmtNum(store.usageStats.totalRequests ?? 0) },
    { label: '输入 Token', value: fmtNum(store.usageStats.totalPromptTokens ?? 0) },
    { label: '输出 Token', value: fmtNum(store.usageStats.totalCompletionTokens ?? 0) },
    { label: '估算成本', value: fmtCost(store.usageStats.totalCost ?? 0) },
  ])

  const byProvider = createMemo(() =>
    Object.entries(store.usageStats.byProvider ?? {})
      .map(([k, v]) => ({ provider: k, ...v }))
      .sort((a, b) => b.requests - a.requests),
  )

  return (
    <div class="space-y-5 stagger">
      <PageHeader
        title="用量统计"
        subtitle={`累计 ${fmtNum(store.usageStats.totalRequestsLifetime ?? 0)} 次请求 · 实时监控流量路由分发`}
        actions={
          <>
            <SegmentedControl
              value={subTab()}
              onChange={setSubTab}
              options={[
                { value: 'overview', label: '概览' },
                { value: 'details', label: `请求明细 (${store.requestDetailsPagination().totalItems || 0})` },
              ]}
            />

            <Button variant={live() ? 'danger' : 'secondary'} size="sm" onClick={toggleLive} class="flex items-center gap-2">
              <StatusPulse
                status={live() ? 'active' : 'paused'}
                tone={live() ? 'red' : 'accent'}
                size="sm"
              />
              <span>{live() ? '停止实时' : '实时事件'}</span>
            </Button>
          </>
        }
      />

      {/* 拓扑图 (9router 风格网关拓扑) */}
      <Show when={subTab() === 'overview'}>
        <GatewayTopology
          providers={store.providers()}
          activeConnections={store.activeConnections()}
          liveEvents={liveEvents()}
        />
      </Show>
      {/* 概览视图：KPI、Token 趋势、按提供商与实时事件 */}
      <Show when={subTab() === 'overview'}>
        {/* KPI */}
        <Show when={!loading()} fallback={
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
        </Show>
        {/* 图表 */}
        <Card class="p-5">
          <div class="flex items-center justify-between gap-3 mb-4">
            <div>
              <h3 class="text-sm font-semibold flex items-center gap-2">
                <span>Token 趋势</span>
                <Show when={hoveredPoint()}>
                  {pt => (
                    <Badge tone="blue" class="text-[11px] font-mono px-2 py-0.5">
                      {pt().label} · {fmtNum(pt().tokens)} Tokens
                    </Badge>
                  )}
                </Show>
              </h3>
              <p class="text-xs text-faint mt-0.5">
                <Show when={hoveredPoint()} fallback={`按时间段聚合的 Prompt 与 Completion 吞吐 · 峰值 ${fmtNum(maxTokens())} Tokens`}>
                  悬浮各柱状节点查看详细用量吞吐
                </Show>
              </p>
            </div>
            <Select value={period()} options={PERIODS} onChange={v => { setPeriod(v); load() }} align="right" />
          </div>

          <Show when={chart().length > 0} fallback={<Empty message="该周期暂无数据。" />}>
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
              <div class="relative z-10 h-36 w-full flex items-stretch gap-1 sm:gap-1.5">
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
                        onMouseLeave={() => setHoveredPoint(null)}
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
        </Card>

        <div class="grid lg:grid-cols-2 gap-4">
          {/* 按提供商 */}
          <Card class="p-5">
            <h3 class="text-sm font-semibold mb-3">按提供商</h3>
            <Show when={byProvider().length > 0} fallback={<Empty message="暂无数据" />}>
              <div class="space-y-2">
                <For each={byProvider()}>
                  {p => (
                    <div class="flex items-center gap-3 text-sm">
                      <span class="w-28 truncate font-mono text-xs">{p.provider}</span>
                      <div class="flex-1 h-1.5 rounded-full bg-hover overflow-hidden">
                        <div
                          class="h-full bg-accent"
                          style={{ width: `${(p.requests / Math.max(1, byProvider()[0].requests)) * 100}%` }}
                        />
                      </div>
                      <span class="w-16 text-right text-xs text-faint tabular-nums">{fmtNum(p.requests)}</span>
                    </div>
                  )}
                </For>
              </div>
            </Show>
          </Card>

          {/* 实时事件 */}
          <Card class="p-5">
            <div class="flex items-center justify-between mb-3">
              <h3 class="text-sm font-semibold">实时事件</h3>
              <Show when={live()}><Badge tone="green">连接中</Badge></Show>
            </div>
            <Show when={liveEvents().length > 0} fallback={<Empty message={live() ? '等待事件…' : '点击右上角「实时事件」开始监听'} />}>
              <div class="space-y-1 max-h-64 overflow-y-auto">
                <For each={liveEvents()}>
                  {e => (
                    <div class="flex items-center gap-2 text-xs py-1 border-b border-subtle/50 last:border-0">
                      <span class="text-faint font-mono">{fmtTime(e.timestamp)}</span>
                      <span class="truncate">{e.model || e.endpoint || '-'}</span>
                      <Badge tone={e.status === 'ok' ? 'green' : 'red'}>{e.status || '-'}</Badge>
                      <Show when={e.latencyMs}><span class="ml-auto text-faint">{e.latencyMs}ms</span></Show>
                    </div>
                  )}
                </For>
              </div>
            </Show>
          </Card>
        </div>
      </Show>

      {/* 请求明细视图 */}
      <Show when={subTab() === 'details'}>
        <Card class="p-5">
          <div class="flex items-center justify-between mb-3">
            <h3 class="text-sm font-semibold">请求明细日志</h3>
            <span class="text-xs text-faint">共 {store.requestDetailsPagination().totalItems} 条</span>
          </div>
          <Show when={store.requestDetails().length > 0} fallback={<Empty message="暂无请求记录。" />}>
            <div class="overflow-x-auto">
              <table class="w-full text-xs">
                <thead>
                  <tr class="text-faint text-left border-b border-subtle">
                    <th class="pb-2 font-medium">时间</th>
                    <th class="pb-2 font-medium">模型</th>
                    <th class="pb-2 font-medium">状态</th>
                    <th class="pb-2 font-medium text-right">输入</th>
                    <th class="pb-2 font-medium text-right">输出</th>
                    <th class="pb-2 font-medium text-right">耗时</th>
                    <th class="pb-2 font-medium text-right">详情</th>
                  </tr>
                </thead>
                <tbody>
                  <For each={store.requestDetails()}>
                    {d => (
                      <tr class="border-b border-subtle/40 last:border-0 hover:bg-hover/30 transition-colors">
                        <td class="py-1.5 text-faint font-mono">{fmtTime(d.timestamp)}</td>
                        <td class="py-1.5 truncate max-w-[200px] font-medium">{d.model || '-'}</td>
                        <td class="py-1.5"><Badge tone={d.status === 'ok' ? 'green' : 'red'}>{d.status || '-'}</Badge></td>
                        <td class="py-1.5 text-right tabular-nums">{fmtNum(d.promptTokens ?? 0)}</td>
                        <td class="py-1.5 text-right tabular-nums">{fmtNum(d.completionTokens ?? 0)}</td>
                        <td class="py-1.5 text-right text-faint tabular-nums">{d.latencyMs ?? '-'}ms</td>
                        <td class="py-1.5 text-right">
                          <button
                            type="button"
                            class="inline-flex items-center justify-center w-7 h-7 rounded-lg text-muted hover:text-foreground hover:bg-hover transition-colors"
                            onClick={() => setSelectedDetail(d)}
                            title="查看请求明细"
                            aria-label="查看请求明细"
                          >
                            <svg class="w-3.5 h-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                              <path d="M2 12s3-7 10-7 10 7 10 7-3 7-10 7-10-7-10-7Z" />
                              <circle cx="12" cy="12" r="3" />
                            </svg>
                          </button>
                        </td>
                      </tr>
                    )}
                  </For>
                </tbody>
              </table>
            </div>
            {/* 分页 */}
            <div class="flex items-center justify-end gap-2 mt-4 pt-3 border-t border-subtle/50">
              <Button
                size="sm" variant="ghost"
                disabled={!store.requestDetailsPagination().hasPrev}
                onClick={() => store.loadRequestDetails(store.requestDetailsPagination().page - 1, 10)}
              >上一页</Button>
              <span class="text-xs text-faint">
                {store.requestDetailsPagination().page} / {Math.max(1, store.requestDetailsPagination().totalPages)}
              </span>
              <Button
                size="sm" variant="ghost"
                disabled={!store.requestDetailsPagination().hasNext}
                onClick={() => store.loadRequestDetails(store.requestDetailsPagination().page + 1, 10)}
              >下一页</Button>
            </div>
          </Show>
        </Card>
      </Show>

      {/* 请求明细详情弹窗 */}
      <RequestDetailModal
        item={selectedDetail()}
        onClose={() => setSelectedDetail(null)}
      />
    </div>
  )
}

export default Usage
