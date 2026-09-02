import { type Component, For, Show, createSignal, createResource, createMemo } from 'solid-js'
import { A } from '@solidjs/router'
import { api, apiPost, apiDelete } from '@/lib/api'
import { Card, Badge, Button, Input, Empty, Skeleton } from '@/components/ui'

interface ToolRow {
  id: string
  name: string
  description?: string
  icon?: string
  color?: string
  configType?: string
  installed: boolean
  hasGateway: boolean
  configPath?: string
  message?: string
}

const CliTools: Component = () => {
  const [busy, setBusy] = createSignal<string | null>(null)
  const [baseUrl, setBaseUrl] = createSignal('http://127.0.0.1:20128/v1')
  const [err, setErr] = createSignal('')

  // registry（静态定义） + all-statuses（探测结果）合并
  const [data, { refetch }] = createResource(async () => {
    const [reg, statuses] = await Promise.all([
      api('/api/cli-tools').catch(() => ({ tools: [] })),
      api('/api/cli-tools/all-statuses').catch(() => ({})),
    ])
    const tools = reg?.tools ?? []
    const st = statuses ?? {}
    return tools.map((t: any): ToolRow => {
      const s = st[t.id] ?? {}
      return {
        ...t,
        installed: !!s.installed,
        hasGateway: !!s.has9Router,
        configPath: s.configPath,
        message: s.message,
      }
    })
  })

  const rows = createMemo<ToolRow[]>(() => data() ?? [])
  const configuredCount = createMemo(() => rows().filter(r => r.hasGateway).length)

  async function apply(id: string) {
    setBusy(id); setErr('')
    try {
      await apiPost('/api/cli-tools/' + id, { baseUrl: baseUrl() })
      await refetch()
    } catch (e: any) { setErr(e?.message || '接入失败') }
    finally { setBusy(null) }
  }

  async function reset(id: string) {
    setBusy(id); setErr('')
    try { await apiDelete('/api/cli-tools/' + id); await refetch() }
    catch (e: any) { setErr(e?.message || '重置失败') }
    finally { setBusy(null) }
  }

  return (
    <div class="space-y-5">
      <div class="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 class="text-xl font-semibold">CLI 工具接入</h1>
          <p class="text-sm text-faint mt-0.5">
            写入网关配置到本地 CLI 工具{rows().length ? ` · ${configuredCount()}/${rows().length} 已接入` : ''}
          </p>
        </div>
        <Button variant="ghost" onClick={() => refetch()}>重新检测</Button>
      </div>

      <Card class="p-4 flex flex-wrap items-center gap-2">
        <span class="text-xs text-muted">网关 Base URL</span>
        <Input class="!w-72" value={baseUrl()} onInput={setBaseUrl} placeholder="http://127.0.0.1:20128/v1" />
        <span class="text-[11px] text-faint">写入工具配置的 baseUrl，后端要求非空</span>
      </Card>

      <Show when={err()}>
        <div class="px-3 py-2 rounded-control text-xs bg-danger/10 text-danger">{err()}</div>
      </Show>

      <Show when={!data.loading} fallback={<Card class="p-6"><Skeleton class="h-40 w-full" /></Card>}>
        <Show when={rows().length > 0} fallback={<Card class="p-6"><Empty message="未检测到 CLI 工具。" /></Card>}>
          <div class="grid sm:grid-cols-2 lg:grid-cols-3 gap-3">
            <For each={rows()}>
              {t => (
                <Card class="p-4 hover:border-accent/50 transition-colors">
                  <div class="flex items-start gap-3">
                    <Show when={t.icon} fallback={
                      <div class="w-9 h-9 rounded-control shrink-0" style={{ background: t.color || 'var(--accent)' }} />
                    }>
                      <img src={t.icon} alt="" class="w-9 h-9 rounded-control object-contain shrink-0" />
                    </Show>
                    <div class="min-w-0 flex-1">
                      <A href={`/cli-tools/${t.id}`} class="font-medium text-sm hover:text-accent">
                        {t.name}
                      </A>
                      <div class="text-xs text-faint mt-0.5 line-clamp-2">{t.description || t.configType || ''}</div>

                      <div class="mt-2 flex items-center gap-1.5 flex-wrap">
                        <Badge tone={t.installed ? 'green' : 'gray'}>{t.installed ? '已安装' : '未安装'}</Badge>
                        <Show when={t.hasGateway}><Badge tone="blue">已接入网关</Badge></Show>
                      </div>

                      <Show when={t.configPath}>
                        <div class="mt-1 text-[10px] text-faint font-mono truncate" title={t.configPath}>{t.configPath}</div>
                      </Show>
                      <Show when={t.message}>
                        <div class="mt-0.5 text-[10px] text-faint">{t.message}</div>
                      </Show>

                      <div class="mt-2 flex items-center gap-1.5">
                        <Show when={t.hasGateway} fallback={
                          <Button size="sm" variant="secondary" loading={busy() === t.id}
                            disabled={!baseUrl().trim() || !t.installed}
                            onClick={() => apply(t.id)}>
                            接入
                          </Button>
                        }>
                          <Button size="sm" variant="ghost" loading={busy() === t.id} onClick={() => reset(t.id)}>
                            重置
                          </Button>
                        </Show>
                      </div>
                    </div>
                  </div>
                </Card>
              )}
            </For>
          </div>
        </Show>
      </Show>
    </div>
  )
}

export default CliTools
