import { type Component, For, Show, createSignal, createResource } from 'solid-js'
import { api, apiPost } from '@/lib/api'
import { Card, Badge, Button, Empty, Skeleton, confirm } from '@/components/ui'
import { toast } from '@/lib/toast'

interface MitmStatus {
  enabled?: boolean
  running?: boolean
  reason?: string
}

interface MitmTrafficItem {
  id?: string
  tool?: string
  model?: string
  time?: string
  method?: string
  host?: string
  url?: string
  status?: number | string
}

const Mitm: Component = () => {
  const [status, { refetch }] = createResource(async () => {
    try { return await api<MitmStatus>('/api/mitm/status') } catch { return null }
  })
  const [traffic, { refetch: refetchTraffic }] = createResource(async () => {
    try { return (await api<{ traffic?: MitmTrafficItem[] }>('/api/mitm/traffic'))?.traffic ?? [] } catch { return [] }
  })
  const [busy, setBusy] = createSignal('')

  async function act(kind: string, path: string) {
    if (kind === 'stop') {
      const ok = await confirm({
        title: '停止 MITM 调试代理',
        message: '确定要停止本地流量抓包代理吗？运行中的 CLI 抓包将被中断。',
        variant: 'warning',
      })
      if (!ok) return
    }
    setBusy(kind)
    try {
      await apiPost(path)
      await refetch()
      toast.success(kind === 'start' ? 'MITM 代理已启动' : 'MITM 代理已停止')
    } catch (e: unknown) {
      toast.error(e instanceof Error ? e.message : '操作失败')
      console.error(e)
    } finally {
      setBusy('')
    }
  }

  async function handleRefreshTraffic() {
    await refetchTraffic()
    toast.info('流量记录已刷新')
  }

  return (
    <div class="space-y-5 stagger">
      <div class="sticky top-16 z-20 bg-bg/90 backdrop-blur-md pt-1 pb-3 flex items-center justify-between gap-3 border-b border-subtle/50">
        <div>
          <h1 class="text-xl font-semibold">MITM 调试代理</h1>
          <p class="text-sm text-faint mt-0.5">拦截并观察 CLI 工具的 LLM 流量（仅本地）</p>
        </div>
        <Show when={status()?.enabled}>
          <Badge tone={status()?.running ? 'green' : 'gray'}>{status()?.running ? '运行中' : '已停止'}</Badge>
        </Show>
      </div>

      <Show when={!status.loading} fallback={<Card class="p-6"><Skeleton class="h-32 w-full" /></Card>}>
        <Card class="p-5 space-y-3">
          <Show when={status()?.enabled} fallback={
            <div>
              <Empty message={status()?.reason || 'MITM 未启用。'} />
              <p class="text-xs text-faint text-center pb-3">
                以 <code class="font-mono">-mitm</code> 启动网关并绑定 localhost 后可用。
              </p>
            </div>
          }>
            <div class="flex flex-wrap gap-2">
              <Show when={!status()?.running}>
                <Button variant="primary" loading={busy() === 'start'} onClick={() => act('start', '/api/mitm/start')}>启动</Button>
              </Show>
              <Show when={status()?.running}>
                <Button variant="danger" loading={busy() === 'stop'} onClick={() => act('stop', '/api/mitm/stop')}>停止</Button>
              </Show>
              <Button variant="secondary" onClick={handleRefreshTraffic}>刷新流量</Button>
              <a href="/api/mitm/cert" target="_blank" rel="noreferrer">
                <Button variant="ghost">下载证书</Button>
              </a>
            </div>
          </Show>
        </Card>
      </Show>

      <Show when={status()?.running}>
        <Card class="p-5">
          <h3 class="text-sm font-semibold mb-3">流量记录</h3>
          <Show when={(traffic() ?? []).length > 0} fallback={<Empty message="暂无拦截记录。" />}>
            <div class="space-y-1 max-h-96 overflow-y-auto">
              <For each={traffic() ?? []}>
                {e => (
                  <div class="flex items-center gap-2 text-xs py-1.5 border-b border-subtle/40 last:border-0">
                    <Badge tone="gray">{e.method || 'POST'}</Badge>
                    <span class="truncate font-mono flex-1">{e.host || e.url}</span>
                    <Show when={e.model}><span class="text-faint">{e.model}</span></Show>
                    <Badge tone={e.status === 200 ? 'green' : 'red'}>{e.status ?? '-'}</Badge>
                  </div>
                )}
              </For>
            </div>
          </Show>
        </Card>
      </Show>
    </div>
  )
}

export default Mitm
