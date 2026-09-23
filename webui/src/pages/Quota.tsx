import { type Component, For, Show, batch, createSignal, createMemo, createEffect, onMount, onCleanup } from 'solid-js'
import { useGatewayStore } from '@/stores/gateway'
import { useI18n } from '@/i18n'
import { api } from '@/lib/api'
import { Card, Badge, AuthBadge, Button, Alert, Empty, Skeleton, Toggle, ProviderAvatar, IconSettings, Select, Input, IconChevronLeft, IconChevronRight, IconZap, IconClock, PageHeader } from '@/components/ui'
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
  const [pendingIds, setPendingIds] = createSignal<Set<string>>(new Set())
  const [failedIds, setFailedIds] = createSignal<Set<string>>(new Set())
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

      // 汇总独立更新，不阻塞逐账号额度，也不因失败提前结束本轮刷新。
      const rPromise = api<{ providers?: ProviderUsage[] }>(`/api/usage/providers?period=7d&_t=${cacheBust}`, { signal })
        .then(r => {
          if (!signal.aborted) setRows(r?.providers ?? [])
        })

      // 先同步初始化本轮状态，再展示网格，避免首屏短暂显示无额度接口。
      batch(() => {
        setPendingIds(new Set(conns.map(c => c.id)))
        setFailedIds(new Set<string>())
        setLoading(false)
      })

      const quotaPromises = conns.map(async c => {
        try {
          const res = await api<ConnQuota>(`/api/usage/connection/${c.id}?_t=${cacheBust}`, { signal })
          if (signal.aborted) return
          if (res) {
            setDetails(prev => ({ ...prev, [c.id]: res }))
          }
        } catch {
          if (!signal.aborted) setFailedIds(prev => new Set([...prev, c.id]))
        } finally {
          // 被取消的旧轮次不能删除新轮次的 pending。
          if (!signal.aborted) {
            setPendingIds(prev => {
              const next = new Set(prev)
              next.delete(c.id)
              return next
            })
          }
        }
      })

      await Promise.allSettled([rPromise, ...quotaPromises])
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

  // 按供应商过滤并排序连接（同供应商账号排在一起，高度与指标完全对称对齐）
  const filteredConnections = createMemo(() => {
    const list = store.providers()
    const filter = providerFilter()
    const filtered = filter ? list.filter(c => c.provider === filter) : list
    return filtered.slice().sort((a, b) => {
      // 1. 同供应商账号聚集在一起（如 antigravity, codebuddy-cn, qoder...）
      const pCmp = a.provider.localeCompare(b.provider)
      if (pCmp !== 0) return pCmp
      // 2. 相同供应商按 priority 升序（0 主账号优先，排在同供应商的第一张卡片）
      if (a.priority !== b.priority) return a.priority - b.priority
      // 3. 再次按名称 / email 稳定排序
      const nameA = a.name || a.email || a.id
      const nameB = b.name || b.email || b.id
      return nameA.localeCompare(nameB)
    })
  })

  // 提供商下拉过滤选项
  const providerOptions = createMemo(() => {
    const set = new Set(store.providers().map(c => c.provider))
    return Array.from(set)
  })

  // 单条额度行（紧凑 2 行设计：第 1 行为状态圆点 + 指标名称 + 余量/总量 + 百分比，第 2 行为平滑细进度条 + 倒计时）
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
        const mins = Math.floor((diffMs % (1000 * 60 * 60)) / (1000 * 60))
        if (days > 0) {
          return hours > 0
            ? t('quota.resetInDaysHours', { days, hours })
            : t('quota.resetInDays', { days })
        }
        if (hours > 0) return t('quota.resetInHoursMins', { hours, mins })
        if (mins > 0) return t('quota.resetInMins', { mins })
        return t('quota.resetSoon')
      } catch {
        return ''
      }
    }

    return (
      <div class="py-1 px-2 rounded-lg hover:bg-hover/60 transition-colors text-xs space-y-1">
        <div class="flex items-center justify-between gap-2 min-w-0">
          <div class="flex items-center gap-1.5 min-w-0 flex-1 truncate">
            <span class={`w-1.5 h-1.5 rounded-full shrink-0 ${colorClass().dot}`} />
            <span class="text-[11px] text-foreground/80 font-normal truncate" title={props.name}>
              {props.name}
            </span>
          </div>
          <div class="flex items-center gap-1.5 shrink-0 font-mono tabular-nums text-[11px]">
            <span
              class="text-muted/80"
              title={t('quota.remainingTotalTitle', {
                remaining: formatNumber(props.quota.remaining),
                total: formatNumber(props.quota.total),
                used: props.quota.used != null ? t('quota.usedSuffix', { used: formatNumber(props.quota.used) }) : '',
              })}
            >
              {formatNumber(props.quota.remaining)} / {formatNumber(props.quota.total)}
            </span>
            <span class={`font-medium min-w-7 text-right ${colorClass().text}`}>
              {pct()}%
            </span>
          </div>
        </div>
        <div class="flex items-center gap-2.5 min-w-0">
          <div class="flex-1 min-w-0 h-1 rounded-full bg-control overflow-hidden">
            <div
              class={`h-full rounded-full transition-all duration-300 ${colorClass().bar}`}
              style={{ width: `${Math.min(100, Math.max(0, pct()))}%` }}
            />
          </div>
          <div class="w-16 sm:w-20 shrink-0 text-right">
            <Show when={resetHint()}>
              <span
                class="inline-flex items-center justify-end gap-1 text-[10px] text-faint font-mono whitespace-nowrap"
                title={props.quota.resetAt ? new Date(props.quota.resetAt).toLocaleString() : undefined}
              >
                <IconClock size={10} class="opacity-60 shrink-0" />
                {resetHint()}
              </span>
            </Show>
          </div>
        </div>
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
          <div class="flex flex-wrap items-center justify-between gap-2 px-1 pt-1 pb-0.5 text-xs text-faint">
            <Input
              size="sm"
              placeholder={t('quota.searchPlaceholder')}
              value={search()}
              onInput={v => { setSearch(v); setPage(1); }}
              class="min-w-0 flex-1 basis-40"
            />
            <div class="flex items-center gap-1.5 text-xs shrink-0 font-mono">
              <Button
                size="sm"
                variant="secondary"
                disabled={effectivePage() <= 1}
                onClick={() => setPage(p => Math.max(1, Math.min(p, totalPages()) - 1))}
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
                title={t('quota.nextPage')}
              >
                <IconChevronRight size={12} />
              </Button>
            </div>
          </div>
        </Show>

        <div class="space-y-0.5 max-h-[360px] overflow-y-auto px-1">
          <For each={currentKeys()}>
            {k => {
              const b = () => props.quotasObj[k]
              const label = () => b().displayName || (k === 'user' ? t('quota.bucketUser') : k === 'organization' ? t('quota.bucketOrg') : k)
              return <QuotaItem name={label()} quota={b()} />
            }}
          </For>
          <Show when={currentKeys().length === 0}>
            <Empty message={t('quota.noMatchingMetrics')} class="py-3!" />
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
          <div class="flex items-center gap-2 flex-wrap">

            <Show when={providerOptions().length > 1}>
              <Select
                size="sm"
                class="w-40"
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
          </div>
        }
      />
      <Show
        when={!loading()}
        fallback={
          <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
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
            <Card class="p-12">
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
          <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
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
                      <Card hover class="@container min-w-0 p-4 flex flex-col justify-between shadow-sm transition-all">
                        <div>
                          <div class="flex items-start justify-between gap-2.5 pb-2.5 border-b border-subtle/50">
                            <div class="flex items-center gap-3 min-w-0">
                              <ProviderAvatar
                                provider={conn.provider}
                                name={providerName()}
                                size="md"
                              />
                              <div class="min-w-0">
                                <div class="flex items-center gap-1.5 flex-wrap">
                                  <A
                                    href={`/providers/${conn.id}`}
                                    class="font-semibold text-sm text-foreground hover:text-accent transition-colors truncate"
                                  >
                                    {providerName()}
                                  </A>
                                  <AuthBadge authType={conn.authType} />
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
                                ariaLabel={`${t('common.enabled')}: ${providerName()} · ${conn.name || conn.email || conn.id}`}
                                checked={conn.isActive}
                                onChange={async () => {
                                  await store.toggleProvider(conn)
                                }}
                              />
                            </div>
                          </div>

                          <div class="text-xs text-faint font-medium mt-2 px-1 flex flex-wrap items-center justify-between gap-1">
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
                            <Show when={failedIds().has(conn.id)}>
                              <Alert variant="danger" title={t('common.error')}>
                                <Button size="sm" variant="secondary" onClick={() => { load(true) }}>
                                  {t('common.retry')}
                                </Button>
                              </Alert>
                            </Show>
                            <Show
                              when={!qData() && pendingIds().has(conn.id)}
                              fallback={
                                <Show
                                  when={hasRealQuotas()}
                                  fallback={
                                    <Show when={!failedIds().has(conn.id)}>
                                      <Show
                                        when={qData()?.message}
                                        fallback={
                                          <div class="p-3 text-center text-xs text-faint bg-surface-inset rounded-lg">
                                            {t('quota.noOnlineApiShort')}
                                          </div>
                                        }
                                      >
                                        <div class="p-2.5 text-xs text-faint bg-surface-inset rounded-lg flex flex-col sm:flex-row sm:items-center justify-between gap-2.5">
                                          <span class="min-w-0 flex-1 leading-relaxed wrap-anywhere">
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
                                    </Show>
                                  }
                                >
                                  <QuotaList quotasObj={quotasObj()} />
                                </Show>
                              }
                            >
                              <div class="space-y-2 py-1">
                                <For each={[1, 2, 3]}>
                                  {() => (
                                    <div class="flex min-w-0 items-center gap-2.5 py-1.5 px-2">
                                      <Skeleton class="w-2 h-2 rounded-full shrink-0" />
                                      <Skeleton class="h-3 w-1/4 max-w-32 min-w-0" />
                                      <Skeleton class="h-3 w-1/6 max-w-20 min-w-0" />
                                      <div class="flex-1 min-w-0 h-1.5 rounded-full bg-hover overflow-hidden mx-1.5">
                                        <Skeleton class="h-full w-1/2 rounded-full" />
                                      </div>
                                      <Skeleton class="h-3 w-10 min-w-0" />
                                      <Skeleton class="h-3 w-16 min-w-0" />
                                    </div>
                                  )}
                                </For>
                              </div>
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
