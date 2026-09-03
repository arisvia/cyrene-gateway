import { type Component, For, Show, createSignal, createMemo, onMount } from 'solid-js'
import { useGatewayStore } from '@/stores/gateway'
import { api } from '@/lib/api'
import { Card, Badge, Button, Empty, Skeleton, Toggle, ProviderAvatar } from '@/components/ui'
import { formatNumber } from '@/lib/format'
import { A } from '@solidjs/router'

interface QuotaBucket {
  used: number
  total: number
  remaining: number
  remainingPercentage: number
  resetAt: string
  unit: string
}

interface ConnQuota {
  plan?: string
  quotas?: Record<string, QuotaBucket>
  message?: string
}

const Quota: Component = () => {
  const store = useGatewayStore()
  const [rows, setRows] = createSignal<any[]>([])
  const [loading, setLoading] = createSignal(true)
  const [refreshing, setRefreshing] = createSignal(false)
  const [details, setDetails] = createSignal<Record<string, ConnQuota>>({})
  const [providerFilter, setProviderFilter] = createSignal('')
  const [autoRefresh, setAutoRefresh] = createSignal(false)

  async function load() {
    try {
      const [r, conns] = await Promise.all([
        api('/api/usage/providers?period=7d'),
        Promise.resolve(store.providers()),
      ])
      setRows(r?.providers ?? [])

      // 逐连接拉取真实额度（plan/credits/resetAt）；失败静默留空
      const entries: Record<string, ConnQuota> = {}
      await Promise.all(conns.map(async c => {
        try {
          const res = await api(`/api/usage/connection/${c.id}`)
          if (res) entries[c.id] = res
        } catch {
          // provider 不支持
        }
      }))
      setDetails(entries)
    } catch {
      setRows([])
    } finally {
      setLoading(false)
      setRefreshing(false)
    }
  }

  onMount(() => {
    load()
    const interval = setInterval(() => {
      if (autoRefresh()) {
        load()
      }
    }, 60000)
    return () => clearInterval(interval)
  })

  // 按供应商过滤连接
  const filteredConnections = createMemo(() => {
    const list = store.providers()
    const filter = providerFilter()
    if (!filter) return list
    return list.filter(c => c.provider === filter)
  })

  // 提供商下拉过滤选项
  const providerOptions = createMemo(() => {
    const set = new Set(store.providers().map(c => c.provider))
    return Array.from(set)
  })

  // 单条额度行（对标 2 列卡片样式：健康圆点 + 额度名称 + 比例 + 细进度条 + 百分比 + 倒计时）
  const QuotaItem = (props: {
    name: string
    quota: QuotaBucket
  }) => {
    const pct = () => Math.round(props.quota.remainingPercentage ?? (
      props.quota.total > 0 ? (props.quota.remaining / props.quota.total) * 100 : 0
    ))
    const isExhausted = () => props.quota.remaining <= 0 && props.quota.total > 0
    const isLow = () => pct() < 20

    const resetHint = () => {
      if (!props.quota.resetAt) return ''
      try {
        const diffMs = new Date(props.quota.resetAt).getTime() - Date.now()
        if (diffMs <= 0) return '即将重置'
        const days = Math.floor(diffMs / (1000 * 60 * 60 * 24))
        const hours = Math.floor((diffMs % (1000 * 60 * 60 * 24)) / (1000 * 60 * 60))
        if (days > 0) return `in ${days}d ${hours}h`
        const mins = Math.floor((diffMs % (1000 * 60 * 60)) / (1000 * 60))
        return `in ${hours}h ${mins}m`
      } catch {
        return props.quota.resetAt.slice(0, 10)
      }
    }

    return (
      <div class="flex items-center gap-2.5 py-1.5 px-2 rounded-lg hover:bg-hover/60 transition-colors text-xs">
        <span
          class={`w-2 h-2 rounded-full shrink-0 ${
            isExhausted() ? 'bg-red-500 shadow-xs shadow-red-500/50' : 'bg-emerald-500 shadow-xs shadow-emerald-500/50'
          }`}
        />
        <span class="w-24 sm:w-28 font-medium text-foreground truncate shrink-0">
          {props.name}
        </span>
        <span class="w-20 text-right tabular-nums text-faint text-[11px] shrink-0">
          {formatNumber(props.quota.used)} / {formatNumber(props.quota.total)}
        </span>
        <div class="flex-1 min-w-[60px] h-1.5 rounded-full bg-hover overflow-hidden mx-1">
          <div
            class={`h-full rounded-full transition-all ${
              isExhausted() ? 'bg-red-500/60' : isLow() ? 'bg-amber-400' : 'bg-emerald-500'
            }`}
            style={{ width: `${Math.min(100, pct())}%` }}
          />
        </div>
        <span
          class={`w-10 text-right font-mono text-[11px] font-medium shrink-0 ${
            isExhausted() ? 'text-danger' : isLow() ? 'text-amber-400' : 'text-emerald-500'
          }`}
        >
          {pct()}%
        </span>
        <span class="w-20 text-right text-[11px] text-faint truncate shrink-0 font-mono">
          {resetHint()}
        </span>
      </div>
    )
  }

  return (
    <div class="space-y-5 stagger">
      {/* 顶部工具栏 (吸顶固定) */}
      <div class="sticky top-16 z-20 bg-bg/90 backdrop-blur-md pt-1 pb-3 flex flex-col sm:flex-row sm:items-center justify-between gap-4 border-b border-subtle/50">
        <div>
          <h1 class="text-xl font-semibold text-foreground">配额中心</h1>
          <p class="text-sm text-faint mt-0.5">
            按账号与节点双列实时呈现官方真实余量 (Credits / Quota) 与自动轮换状态
          </p>
        </div>

        <div class="flex items-center gap-2.5 flex-wrap">
          <Show when={providerOptions().length > 1}>
            <select
              value={providerFilter()}
              onChange={e => setProviderFilter(e.currentTarget.value)}
              class="text-xs px-2.5 py-1.5 rounded-control bg-card border border-subtle text-foreground focus:outline-none focus:border-accent"
            >
              <option value="">全部供应商 ({store.providers().length})</option>
              <For each={providerOptions()}>
                {p => <option value={p}>{p}</option>}
              </For>
            </select>
          </Show>

          <button
            type="button"
            class={`text-xs px-2.5 py-1.5 rounded-control border transition-all flex items-center gap-1.5 cursor-pointer ${
              autoRefresh()
                ? 'bg-amber-500/15 border-amber-500/40 text-amber-400 font-medium'
                : 'bg-hover border-subtle text-faint hover:text-foreground'
            }`}
            onClick={() => setAutoRefresh(!autoRefresh())}
          >
            <span>自动刷新</span>
            <span class="text-[10px] opacity-75">{autoRefresh() ? '(开启 60s)' : '(关闭)'}</span>
          </button>

          <Button
            size="sm"
            variant="secondary"
            loading={refreshing()}
            onClick={() => { setRefreshing(true); load(); }}
          >
            刷新数据
          </Button>
        </div>
      </div>

      <Show when={!loading()} fallback={<Card class="p-6"><Skeleton class="h-32 w-full" /></Card>}>
        {/* 一行两个的账号配额卡片网格 (2-Column Grid) */}
        <Show
          when={filteredConnections().length > 0}
          fallback={<Card class="p-8"><Empty message="当前暂未接入任何提供商连接。" /></Card>}
        >
          <div class="grid grid-cols-1 lg:grid-cols-2 gap-4">
            <For each={filteredConnections()}>
              {conn => {
                const qData = () => details()[conn.id]
                const quotasObj = () => qData()?.quotas || {}
                const quotaKeys = () => Object.keys(quotasObj())
                const hasRealQuotas = () => quotaKeys().length > 0
                const aggRow = () => rows().find(r => r.provider === conn.provider)

                return (
                  <Card hover class="p-4 flex flex-col justify-between border-subtle/80 bg-bg-elevated/70 shadow-sm transition-all hover:border-accent/40">
                    <div>
                      <div class="flex items-start justify-between gap-2.5 pb-2.5 border-b border-subtle/50">
                        <div class="flex items-center gap-3 min-w-0">
                          <ProviderAvatar
                            provider={conn.provider}
                            name={conn.name || conn.provider}
                            size="md"
                          />
                          <div class="min-w-0">
                            <div class="flex items-center gap-2 flex-wrap">
                              <A
                                href={`/providers/${conn.id}`}
                                class="font-semibold text-sm text-foreground hover:text-accent transition-colors truncate"
                              >
                                {conn.name || conn.provider}
                              </A>
                              <Badge tone="gray" class="text-[10px] uppercase font-mono px-1.5 py-0">
                                {conn.provider}
                              </Badge>
                              <Show when={qData()?.plan}>
                                <Badge tone="blue" class="text-[10px] px-1.5 py-0">
                                  {qData()!.plan}
                                </Badge>
                              </Show>
                            </div>
                            <div class="text-xs text-faint font-mono truncate mt-0.5">
                              {conn.email || (conn.data?.credentialHint ? String(conn.data.credentialHint) : `${conn.id.slice(0, 8)}...`)}
                            </div>
                          </div>
                        </div>

                        <div class="flex items-center gap-2 shrink-0">
                          <A href={`/providers/${conn.id}`} title="编辑管理此账号">
                            <Button size="sm" variant="ghost" class="!p-1.5 text-faint hover:text-foreground">
                              ⚙
                            </Button>
                          </A>
                          <Toggle
                            checked={conn.isActive}
                            onChange={async () => {
                              await store.toggleProvider(conn)
                            }}
                          />
                        </div>
                      </div>

                      <div class="text-[11px] text-faint font-medium mt-2 px-1 flex items-center justify-between">
                        <span>
                          {hasRealQuotas() ? `${quotaKeys().length} 项配额指标` : '网关网内用量统计'}
                        </span>
                        <Show when={aggRow()}>
                          <span class="text-[10px] font-mono tabular-nums">
                            请求 {formatNumber(aggRow()!.requests)} · 标记 {formatNumber((aggRow()!.promptTokens || 0) + (aggRow()!.completionTokens || 0))}
                          </span>
                        </Show>
                      </div>

                      <div class="mt-2 space-y-1">
                        <Show
                          when={hasRealQuotas()}
                          fallback={
                            <Show
                              when={qData()?.message}
                              fallback={
                                <div class="p-3 text-center text-xs text-faint bg-bg/50 rounded-lg border border-subtle">
                                  该供应商暂无官方在线配额接口，调度以网关路由与 Fallback 限流探测为准。
                                </div>
                              }
                            >
                              <div class="p-2.5 text-xs text-faint bg-bg/50 rounded-lg border border-subtle">
                                {qData()!.message}
                              </div>
                            </Show>
                          }
                        >
                          <For each={quotaKeys()}>
                            {k => {
                              const b = quotasObj()[k]
                              const label = k === 'user' ? '用户个人额度' : k === 'organization' ? '组织共享包' : k
                              return <QuotaItem name={label} quota={b} />
                            }}
                          </For>
                        </Show>
                      </div>
                    </div>
                  </Card>
                )
              }}
            </For>
          </div>
        </Show>
      </Show>
    </div>
  )
}

export default Quota
