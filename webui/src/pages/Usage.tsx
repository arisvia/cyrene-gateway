import { type Component, For, Show, createSignal, createMemo, onMount, onCleanup } from 'solid-js'
import { useGatewayStore } from '@/stores/gateway'
import { Card, Badge, Button, Select, Empty, Skeleton } from '@/components/ui'
import { formatNumber as fmtNum, formatCost as fmtCost, timeAgo as fmtTime } from '@/lib/format'

const PERIODS = [
  { value: '24h', label: '最近 24 小时' },
  { value: '7d', label: '最近 7 天' },
  { value: '30d', label: '最近 30 天' },
]

const Usage: Component = () => {
  const store = useGatewayStore()
  const [period, setPeriod] = createSignal('7d')
  const [loading, setLoading] = createSignal(true)
  const [live, setLive] = createSignal(false)
  const [liveEvents, setLiveEvents] = createSignal<any[]>([])
  let es: EventSource | null = null

  async function load() {
    setLoading(true)
    try { await store.loadUsage(period()) } finally { setLoading(false) }
  }

  onMount(() => {
    load()
    store.loadRequestDetails(1, 20)
  })

  onCleanup(() => { es?.close(); es = null })

  function toggleLive() {
    if (live()) {
      es?.close(); es = null; setLive(false); return
    }
    es = new EventSource('/api/usage/stream')
    es.onmessage = ev => {
      try {
        const d = JSON.parse(ev.data)
        setLiveEvents(list => [d, ...list].slice(0, 30))
      } catch { /* 忽略心跳 */ }
    }
    es.onerror = () => { setLive(false); es?.close(); es = null }
    setLive(true)
  }

  const chart = () => store.usageChart()
  const maxTokens = createMemo(() => Math.max(1, ...chart().map(c => c.tokens || 0)))

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
    <div class="space-y-5">
      <div class="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 class="text-xl font-semibold">用量统计</h1>
          <p class="text-sm text-faint mt-0.5">累计 {fmtNum(store.usageStats.totalRequestsLifetime ?? 0)} 次请求</p>
        </div>
        <div class="flex items-center gap-2">
          <Select value={period()} options={PERIODS} onChange={v => { setPeriod(v); load() }} />
          <Button variant={live() ? 'danger' : 'secondary'} onClick={toggleLive}>
            {live() ? '■ 停止实时' : '● 实时事件'}
          </Button>
        </div>
      </div>

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
        <h3 class="text-sm font-semibold mb-4">Token 趋势</h3>
        <Show when={chart().length > 0} fallback={<Empty message="该周期暂无数据。" />}>
          <div class="flex items-end gap-1.5 h-40">
            <For each={chart()}>
              {c => (
                <div class="flex-1 flex flex-col items-center gap-1 group">
                  <div class="relative w-full flex-1 flex items-end">
                    <div
                      class="w-full rounded-t bg-accent/70 group-hover:bg-accent transition-colors"
                      style={{ height: `${Math.max(2, ((c.tokens || 0) / maxTokens()) * 100)}%` }}
                      title={`${c.label}: ${fmtNum(c.tokens || 0)} tokens`}
                    />
                  </div>
                  <span class="text-[10px] text-faint truncate w-full text-center">{c.label}</span>
                </div>
              )}
            </For>
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

      {/* 请求明细 */}
      <Card class="p-5">
        <div class="flex items-center justify-between mb-3">
          <h3 class="text-sm font-semibold">请求明细</h3>
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
                </tr>
              </thead>
              <tbody>
                <For each={store.requestDetails()}>
                  {d => (
                    <tr class="border-b border-subtle/40 last:border-0">
                      <td class="py-1.5 text-faint font-mono">{fmtTime(d.timestamp)}</td>
                      <td class="py-1.5 truncate max-w-[180px]">{d.model || '-'}</td>
                      <td class="py-1.5"><Badge tone={d.status === 'ok' ? 'green' : 'red'}>{d.status || '-'}</Badge></td>
                      <td class="py-1.5 text-right tabular-nums">{fmtNum(d.promptTokens ?? 0)}</td>
                      <td class="py-1.5 text-right tabular-nums">{fmtNum(d.completionTokens ?? 0)}</td>
                      <td class="py-1.5 text-right text-faint tabular-nums">{d.latencyMs ?? '-'}ms</td>
                    </tr>
                  )}
                </For>
              </tbody>
            </table>
          </div>
          {/* 分页 */}
          <div class="flex items-center justify-end gap-2 mt-3">
            <Button
              size="sm" variant="ghost"
              disabled={!store.requestDetailsPagination().hasPrev}
              onClick={() => store.loadRequestDetails(store.requestDetailsPagination().page - 1, 20)}
            >上一页</Button>
            <span class="text-xs text-faint">
              {store.requestDetailsPagination().page} / {Math.max(1, store.requestDetailsPagination().totalPages)}
            </span>
            <Button
              size="sm" variant="ghost"
              disabled={!store.requestDetailsPagination().hasNext}
              onClick={() => store.loadRequestDetails(store.requestDetailsPagination().page + 1, 20)}
            >下一页</Button>
          </div>
        </Show>
      </Card>
    </div>
  )
}

export default Usage
