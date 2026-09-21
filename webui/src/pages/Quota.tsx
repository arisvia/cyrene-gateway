import { type Component, For, Show, createSignal, createMemo, createEffect, onMount, onCleanup } from 'solid-js'
import { useGatewayStore } from '@/stores/gateway'
import { useI18n } from '@/i18n'
import { api } from '@/lib/api'
import { Card, Badge, Button, Empty, Skeleton, Toggle, ProviderAvatar, IconSettings, Select, Input, IconChevronLeft, IconChevronRight, IconZap, PageHeader } from '@/components/ui'
import { formatNumber } from '@/lib/format'
import { A } from '@solidjs/router'
import type { ProviderUsage } from '@/types/domain'

interface QuotaBucket {
  used: number
  total: number
  remaining: number
  remainingPercentage: number
  resetAt: string
  resetSoon?: boolean
  unit: string
  displayName?: string
}

interface ConnQuota {
  plan?: string
  quotas?: Record<string, QuotaBucket>
  message?: string
}

const Quota: Component = () => {
  const { t } = useI18n()
  const store = useGatewayStore()
  const REFRESH_INTERVAL_SECS = 60
  const [rows, setRows] = createSignal<ProviderUsage[]>([])
  const [loading, setLoading] = createSignal(true)
  const [refreshing, setRefreshing] = createSignal(false)
  const [details, setDetails] = createSignal<Record<string, ConnQuota>>({})
  const [providerFilter, setProviderFilter] = createSignal('')
  const [autoRefresh, setAutoRefresh] = createSignal(false)
  const [countdown, setCountdown] = createSignal(REFRESH_INTERVAL_SECS)
  let activeAbortController: AbortController | null = null

  async function load(isManual = false) {
    if (isManual) {
      setRefreshing(true)
      setCountdown(REFRESH_INTERVAL_SECS)
    }
    activeAbortController?.abort()
    const controller = new AbortController()
    activeAbortController = controller
    const { signal } = controller

    try {
      // 手动刷新或初次加载时拉取最新的账号连接列表
      if (isManual || store.providers().length === 0) {
        await store.loadProvidersOnly()
      }
      if (signal.aborted) return

      const conns = store.providers()
      const cacheBust = Date.now()

      const rPromise = api<{ providers?: ProviderUsage[] }>(`/api/usage/providers?period=7d&_t=${cacheBust}`, { signal })

      // 先让卡片网格渲染出来，真实额度逐个连接异步流式填充并等待全部就绪
      setLoading(false)

      const quotaPromises = conns.map(async c => {
        try {
          const res = await api<ConnQuota>(`/api/usage/connection/${c.id}?_t=${cacheBust}`, { signal })
          if (signal.aborted) return
          if (res) {
            setDetails(prev => ({ ...prev, [c.id]: res }))
          }
        } catch {
          // provider 不支持或请求取消
        }
      })

      const [r] = await Promise.all([
        rPromise,
        Promise.allSettled(quotaPromises),
      ])
      if (signal.aborted) return

      setRows(r?.providers ?? [])
    } catch {
      if (signal.aborted) return
      setRows([])
    } finally {
      if (!signal.aborted) {
        setLoading(false)
        setRefreshing(false)
      }
    }
  }

  onMount(() => {
    load()
  })
  onCleanup(() => {
    activeAbortController?.abort()
  })


  createEffect(() => {
    if (!autoRefresh()) {
      setCountdown(REFRESH_INTERVAL_SECS)
      return
    }
    setCountdown(REFRESH_INTERVAL_SECS)
    const timer = setInterval(() => {
      setCountdown(prev => {
        if (prev <= 1) {
          load(false)
          return REFRESH_INTERVAL_SECS
        }
        return prev - 1
      })
    }, 1000)
    onCleanup(() => clearInterval(timer))
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
    const pct = () => {
      if (props.quota.remainingPercentage != null) {
        return Math.round(props.quota.remainingPercentage)
      }
      return props.quota.total > 0 ? Math.round((props.quota.remaining / props.quota.total) * 100) : 0
    }
    const isExhausted = () => props.quota.remaining <= 0 && props.quota.total > 0

    // 梯度配额健康色彩：>=80% 翠绿充足，50%~79% 信息蓝，20%~49% 暖橙适中，<20% 警戒红，0% 或耗尽暗红警报
    // 全面收敛为语义 Token（danger / warning / info / success），亮暗主题自适应
    const colorClass = () => {
      if (isExhausted() || pct() <= 0) return { dot: 'bg-danger/80 shadow-xs shadow-danger/50', bar: 'bg-danger/80', text: 'text-danger font-semibold' }
      if (pct() < 20) return { dot: 'bg-danger shadow-xs shadow-danger/50', bar: 'bg-danger', text: 'text-danger' }
      if (pct() < 50) return { dot: 'bg-warning shadow-xs shadow-warning/50', bar: 'bg-warning', text: 'text-warning' }
      if (pct() < 80) return { dot: 'bg-info shadow-xs shadow-info/50', bar: 'bg-info', text: 'text-info' }
      return { dot: 'bg-success shadow-xs shadow-success/50', bar: 'bg-success', text: 'text-success' }
    }

    const resetHint = () => {
      if (props.quota.resetSoon) return t('quota.resetSoon')
      const raw = props.quota.resetAt
      if (!raw) return ''
      try {
        const timeMs = new Date(raw).getTime()
        if (isNaN(timeMs)) return ''
        const diffMs = timeMs - Date.now()
        if (diffMs <= 0) return t('quota.resetSoon')
        const days = Math.floor(diffMs / (1000 * 60 * 60 * 24))
        const hours = Math.floor((diffMs % (1000 * 60 * 60 * 24)) / (1000 * 60 * 60))
        if (days > 0) return t('quota.resetInDaysHours', { days, hours })
        const mins = Math.floor((diffMs % (1000 * 60 * 60)) / (1000 * 60))
        return t('quota.resetInHoursMins', { hours, mins })
      } catch {
        return ''
      }
    }

    return (
      <div class="flex items-center gap-2.5 py-1.5 px-2 rounded-lg hover:bg-hover/60 transition-colors text-xs">
        <span class={`w-2 h-2 rounded-full shrink-0 ${colorClass().dot}`} />
        <span class="w-32 sm:w-44 font-medium text-foreground truncate shrink-0" title={props.name}>
          {props.name}
        </span>
        <span
          class="w-20 text-right tabular-nums text-faint text-[11px] shrink-0 font-mono"
          title={t('quota.remainingTotalTitle', {
            remaining: formatNumber(props.quota.remaining),
            total: formatNumber(props.quota.total),
            used: props.quota.used != null ? t('quota.usedSuffix', { used: formatNumber(props.quota.used) }) : '',
          })}
        >
          {formatNumber(props.quota.remaining)} / {formatNumber(props.quota.total)}
        </span>
        <div class="flex-1 min-w-[70px] h-1.5 rounded-full bg-hover overflow-hidden mx-1.5">
          <div
            class={`h-full rounded-full transition-all duration-300 ${colorClass().bar}`}
            style={{ width: `${Math.min(100, Math.max(0, pct()))}%` }}
          />
        </div>
        <span class={`w-11 text-right font-mono text-[11px] font-medium shrink-0 tabular-nums ${colorClass().text}`}>
          {pct()}%
        </span>
        <span class="w-20 text-right text-[11px] text-faint truncate shrink-0 font-mono">
          {resetHint()}
        </span>
      </div>
    )
  }

  // 账号配额列表组件：支持模型数过多时自动提供分页/展开与滚动控制
  const QuotaList = (props: { quotasObj: Record<string, QuotaBucket> }) => {
    const allKeys = () => Object.keys(props.quotasObj).sort((a, b) => a.localeCompare(b))
    const [search, setSearch] = createSignal('')
    const [page, setPage] = createSignal(1)
    const pageSize = 8

    const filteredKeys = createMemo(() => {
      const q = search().trim().toLowerCase()
      if (!q) return allKeys()
      return allKeys().filter(k => k.toLowerCase().includes(q))
    })

    const totalPages = createMemo(() => Math.ceil(filteredKeys().length / pageSize) || 1)
    const effectivePage = createMemo(() => Math.min(Math.max(1, page()), totalPages()))
    const currentKeys = createMemo(() => {
      if (allKeys().length <= 8) return filteredKeys()
      const start = (effectivePage() - 1) * pageSize
      return filteredKeys().slice(start, start + pageSize)
    })

    return (
      <div class="space-y-1.5">
        <Show when={allKeys().length > 8}>
          <div class="flex items-center justify-between gap-2 px-1 pt-1 pb-0.5 text-xs text-faint">
            <Input
              size="sm"
              placeholder={t('quota.searchPlaceholder')}
              value={search()}
              onInput={v => { setSearch(v); setPage(1); }}
              class="w-40 sm:w-48 text-[11px]! py-0.5! h-7!"
            />
            <div class="flex items-center gap-1.5 text-[11px] shrink-0 font-mono">
              <Button
                size="sm"
                variant="secondary"
                disabled={effectivePage() <= 1}
                onClick={() => setPage(p => Math.max(1, Math.min(p, totalPages()) - 1))}
                class="h-6! px-1.5! min-w-0!"
                title={t('quota.prevPage')}
              >
                <IconChevronLeft size={12} />
              </Button>
              <span class="px-1">{effectivePage()} / {totalPages()}</span>
              <Button
                size="sm"
                variant="secondary"
                disabled={effectivePage() >= totalPages()}
                onClick={() => setPage(p => Math.min(totalPages(), Math.max(p, 1) + 1))}
                class="h-6! px-1.5! min-w-0!"
                title={t('quota.nextPage')}
              >
                <IconChevronRight size={12} />
              </Button>
            </div>
          </div>
        </Show>

        <div class="space-y-0.5 max-h-[360px] overflow-y-auto px-0.5">
          <For each={currentKeys()}>
            {k => {
              const b = props.quotasObj[k]
              const label = b.displayName || (k === 'user' ? t('quota.bucketUser') : k === 'organization' ? t('quota.bucketOrg') : k)
              return <QuotaItem name={label} quota={b} />
            }}
          </For>
          <Show when={currentKeys().length === 0}>
            <div class="p-3 text-center text-xs text-faint">{t('quota.noMatchingMetrics')}</div>
          </Show>
        </div>
      </div>
    )
  }

  return (
    <div class="space-y-5 stagger">
      <PageHeader
        title={t('quota.title')}
        subtitle={t('quota.subtitle')}
        actions={
          <>
            <Show when={providerOptions().length > 1}>
              <Select
                size="sm"
                class="w-44"
                value={providerFilter()}
                onChange={setProviderFilter}
                options={[
                  { value: '', label: t('quota.allProviders', { count: store.providers().length }) },
                  ...providerOptions().map(p => ({ value: p, label: p })),
                ]}
              />
            </Show>

            <Button
              size="sm"
              variant="secondary"
              class={`gap-1.5 transition-colors ${autoRefresh() ? 'border border-warning/40 text-warning font-medium' : 'text-muted'}`}
              onClick={() => setAutoRefresh(!autoRefresh())}
            >
              <span class={`w-1.5 h-1.5 rounded-full ${autoRefresh() ? 'bg-warning animate-pulse' : 'bg-muted/40'}`} />
              <span>{t('quota.autoRefresh')}</span>
              <span class="opacity-70 font-mono">
                {autoRefresh() ? t('quota.autoRefreshOn', { seconds: countdown() }) : t('quota.autoRefreshOff')}
              </span>
            </Button>

            <Button
              size="sm"
              variant="secondary"
              loading={refreshing()}
              onClick={() => { load(true) }}
            >
              {t('quota.refreshData')}
            </Button>
          </>
        }
      />

      <Show
        when={!loading()}
        fallback={
          <div class="grid grid-cols-1 lg:grid-cols-2 gap-4">
            <For each={[1, 2, 3, 4]}>
              {() => (
                <Card class="p-4 space-y-4">
                  <div class="flex items-center justify-between gap-3 pb-2.5 border-b border-subtle/40">
                    <div class="flex items-center gap-3">
                      <Skeleton class="w-10 h-10 rounded-xl" />
                      <div class="space-y-1.5">
                        <Skeleton class="h-4 w-32" />
                        <Skeleton class="h-3 w-20" />
                      </div>
                    </div>
                    <Skeleton class="h-6 w-12 rounded-full" />
                  </div>
                  <div class="space-y-3 pt-1">
                    <div class="space-y-1.5">
                      <div class="flex justify-between">
                        <Skeleton class="h-3 w-24" />
                        <Skeleton class="h-3 w-16" />
                      </div>
                      <Skeleton class="h-1.5 w-full rounded-full" />
                    </div>
                    <div class="space-y-1.5">
                      <div class="flex justify-between">
                        <Skeleton class="h-3 w-28" />
                        <Skeleton class="h-3 w-12" />
                      </div>
                      <Skeleton class="h-1.5 w-full rounded-full" />
                    </div>
                  </div>
                </Card>
              )}
            </For>
          </div>
        }
      >
        {/* 一行两个的账号配额卡片网格 (2-Column Grid) */}
        <Show
          when={filteredConnections().length > 0}
          fallback={
            <Card class="p-12 border-dashed border-subtle">
              <Empty
                icon={<IconZap size={24} />}
                title={t('quota.noConnectionsShort')}
                description={t('quota.emptyDesc')}
                action={
                  <A href="/providers">
                    <Button variant="primary" size="sm">
                      {t('quota.connectProviderAction')}
                    </Button>
                  </A>
                }
              />
            </Card>
          }
        >
          <div class="grid grid-cols-1 lg:grid-cols-2 gap-4">
            <For each={filteredConnections()}>
              {conn => {
                const qData = () => details()[conn.id]
                const quotasObj = () => qData()?.quotas || {}
                const quotaKeys = () => Object.keys(quotasObj())
                const hasRealQuotas = () => quotaKeys().length > 0
                const reg = () => store.registryList().find(r => r.id === conn.provider)
                const providerName = () => reg()?.name || conn.provider
                const aggRow = () => rows().find(r => r.provider === conn.provider)
                return (
                  <Card hover class="p-4 flex flex-col justify-between shadow-sm transition-all">
                    <div>
                      <div class="flex items-start justify-between gap-2.5 pb-2.5 border-b border-subtle/50">
                        <div class="flex items-center gap-3 min-w-0">
                          <ProviderAvatar
                            provider={conn.provider}
                            name={providerName()}
                            size="md"
                          />
                          <div class="min-w-0">
                            <div class="flex items-center gap-2 flex-wrap">
                              <A
                                href={`/providers/${conn.id}`}
                                class="font-semibold text-sm text-foreground hover:text-accent transition-colors truncate"
                              >
                                {providerName()}
                              </A>
                              <Show when={qData()?.plan}>
                                <Badge tone="blue" class="text-[10px] px-1.5 py-0">
                                  {qData()!.plan}
                                </Badge>
                              </Show>
                            </div>
                            <div class="text-xs text-faint font-mono truncate mt-0.5">
                              {(() => {
                                const custom = conn.name && conn.name !== reg()?.name && conn.name.toLowerCase() !== conn.provider.toLowerCase() && conn.name !== conn.email ? conn.name : ''
                                const idHint = conn.email || (conn.data?.credentialHint ? String(conn.data.credentialHint) : `${conn.id.slice(0, 8)}...`)
                                return custom ? `${custom} · ${idHint}` : idHint
                              })()}
                            </div>
                          </div>
                        </div>

                        <div class="flex items-center gap-2 shrink-0">
                          <A href={`/providers/${conn.id}`} title={t('quota.editAccountTitle')}>
                            <Button size="sm" variant="ghost" class="p-1.5! text-faint hover:text-foreground">
                              <IconSettings size={14} />
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
                          {hasRealQuotas() ? t('quota.metricsCountSuffix', { count: quotaKeys().length }) : t('quota.inNetworkUsage')}
                        </span>
                        <Show when={aggRow()}>
                          <span class="text-[10px] font-mono tabular-nums">
                            {t('quota.requestMark', { requests: formatNumber(aggRow()!.requests), tokens: formatNumber((aggRow()!.promptTokens || 0) + (aggRow()!.completionTokens || 0)) })}
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
                                  {t('quota.noOnlineApiShort')}
                                </div>
                              }
                            >
                              <div class="p-2.5 text-xs text-faint bg-bg/50 rounded-lg border border-subtle flex flex-col sm:flex-row sm:items-center justify-between gap-2.5">
                                <span class="min-w-0 flex-1 leading-relaxed">
                                  {qData()!.message?.startsWith('Usage API not implemented')
                                    ? t('quota.usageApiUnavailable')
                                    : conn.provider === 'opencode' && (qData()!.plan === 'OpenCode Zen' || qData()!.message?.includes('OpenCode Zen'))
                                      ? t('quota.opencodeZenNotice')
                                      : qData()!.message}
                                </span>
                                <span class="text-[10px] text-faint font-mono shrink-0 whitespace-nowrap self-end sm:self-center px-1.5 py-0.5 rounded bg-hover/50">
                                  {t('quota.adaptiveThrottle')}
                                </span>
                              </div>
                            </Show>
                          }
                        >
                          <QuotaList quotasObj={quotasObj()} />
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
