import { type Component, For, Show, createSignal, onMount } from 'solid-js'
import { useGatewayStore } from '@/stores/gateway'
import { api } from '@/lib/api'
import { Card, Badge, Button, Empty, Skeleton } from '@/components/ui'
import { formatNumber, timeAgo } from '@/lib/format'

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
  quotas?: { organization?: QuotaBucket; user?: QuotaBucket }
  message?: string
}

const Quota: Component = () => {
  const store = useGatewayStore()
  const [rows, setRows] = createSignal<any[]>([])
  const [period, setPeriod] = createSignal('')
  const [loading, setLoading] = createSignal(true)
  // 连接级额度明细：id → 后端 /api/usage/connection/{id} 响应
  const [details, setDetails] = createSignal<Record<string, ConnQuota>>({})

  async function load() {
    setLoading(true)
    try {
      const [r, conns] = await Promise.all([
        api('/api/usage/providers?period=7d'),
        Promise.resolve(store.providers()),
      ])
      setRows(r?.providers ?? [])
      setPeriod(r?.period ?? '')

      // 逐连接拉取真实额度（plan/credits/resetAt）；失败静默留空
      const entries: Record<string, ConnQuota> = {}
      await Promise.all(conns.map(async c => {
        try { entries[c.id] = await api(`/api/usage/connection/${c.id}`) } catch { /* provider 不支持 */ }
      }))
      setDetails(entries)
    } catch { setRows([]) } finally { setLoading(false) }
  }

  onMount(load)

  const Bucket = (props: { label: string; b?: QuotaBucket }) => (
    <Show when={props.b && props.b.total > 0} fallback={null}>
      <div class="min-w-[140px] flex-1">
        <div class="flex items-center justify-between text-[11px] text-faint">
          <span>{props.label}</span>
          <span class="tabular-nums">
            {formatNumber(props.b!.used)} / {formatNumber(props.b!.total)} {props.b!.unit}
          </span>
        </div>
        <div class="mt-1 h-1.5 rounded-full bg-hover overflow-hidden">
          <div
            class={`h-full ${(props.b!.remainingPercentage ?? 100) < 15 ? 'bg-[color:var(--red)]' : 'bg-accent'}`}
            style={{ width: `${Math.min(100, props.b!.remainingPercentage ?? 0)}%` }}
          />
        </div>
        <div class="mt-0.5 text-[10px] text-faint">
          剩 {formatNumber(props.b!.remaining)} · 重置于 {props.b!.resetAt?.slice(0, 10)}
        </div>
      </div>
    </Show>
  )

  return (
    <div class="space-y-5">
      <div class="flex items-center justify-between">
        <div>
          <h1 class="text-xl font-semibold">配额</h1>
          <p class="text-sm text-faint mt-0.5">按提供商查看用量，并展示各连接的真实额度（plan/credits）</p>
        </div>
        <Button variant="secondary" onClick={load}>刷新</Button>
      </div>

      <Show when={!loading()} fallback={<Card class="p-6"><Skeleton class="h-32 w-full" /></Card>}>
        {/* 连接明细（含 qoder 这类真实额度） */}
        <Show when={Object.keys(details()).length > 0}>
          <div class="grid gap-3">
            <For each={store.providers().filter(c => details()[c.id])}>
              {c => {
                const d = () => details()[c.id]
                return (
                  <Card class="p-4">
                    <div class="flex items-center gap-2 flex-wrap">
                      <span class="font-medium text-sm">{c.name || c.provider}</span>
                      <Badge tone="gray">{c.provider}</Badge>
                      <Show when={d().plan}><Badge tone="blue">{d().plan}</Badge></Show>
                    </div>
                    <Show when={d().message} fallback={
                      <div class="mt-3 flex gap-4 flex-wrap">
                        <Bucket label="用户额度" b={d().quotas?.user} />
                        <Bucket label="组织额度" b={d().quotas?.organization} />
                      </div>
                    }>
                      <div class="mt-2 text-xs text-faint">{d().message}</div>
                    </Show>
                  </Card>
                )
              }}
            </For>
          </div>
        </Show>

        {/* 按提供商聚合 */}
        <Show when={rows().length > 0} fallback={
          <Card class="p-6"><Empty message="暂无聚合用量。" /></Card>
        }>
          <div class="grid gap-3">
            <For each={rows()}>
              {r => (
                <Card class="p-4">
                  <div class="flex items-center justify-between gap-4 flex-wrap">
                    <div class="flex items-center gap-2">
                      <span class="font-mono text-sm">{r.provider}</span>
                      <Badge tone={r.activeConnections > 0 ? 'green' : 'gray'}>
                        {r.activeConnections}/{r.connections} 连接
                      </Badge>
                      <Show when={r.overQuota}><Badge tone="red">超额</Badge></Show>
                    </div>
                    <div class="flex items-center gap-6 text-xs text-faint tabular-nums">
                      <span>请求 {formatNumber(r.requests)}</span>
                      <span>输入 {formatNumber(r.promptTokens)}</span>
                      <span>输出 {formatNumber(r.completionTokens)}</span>
                    </div>
                  </div>
                  <Show when={r.quotaLimit}>
                    <div class="mt-2 h-1.5 rounded-full bg-hover overflow-hidden">
                      <div
                        class={`h-full ${r.overQuota ? 'bg-[color:var(--red)]' : 'bg-accent'}`}
                        style={{ width: `${Math.min(100, ((r.quotaUsed ?? 0) / r.quotaLimit) * 100)}%` }}
                      />
                    </div>
                  </Show>
                </Card>
              )}
            </For>
          </div>
        </Show>
      </Show>
    </div>
  )
}

export default Quota
