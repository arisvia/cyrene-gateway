import { type Component, For, Show, createSignal, createResource } from 'solid-js'
import { api } from '@/lib/api'
import type { BadgeTone, TunnelStatus } from '@/types/domain'
import { Card, Badge, Button, Empty, Skeleton, confirm } from '@/components/ui'
import { useToast } from '@/lib/toast'

const Tunnel: Component = () => {
  const toast = useToast()
  const [status, { refetch }] = createResource(async () => {
    try { return await api('/api/tunnel/status') as TunnelStatus | null } catch { return null }
  })
  const [busy, setBusy] = createSignal('')
  const [logs, setLogs] = createSignal<string[]>([])

  // install 返回 text/event-stream（tunnel.go:23-47），必须流式读取而非 JSON
  async function installStream() {
    setBusy('install'); setLogs([])
    try {
      const res = await fetch('/api/tunnel/tailscale-install', { method: 'POST' })
      if (!res.body) {
        const err = '无法读取安装流'
        toast.error(err)
        return
      }
      const reader = res.body.getReader()
      const decoder = new TextDecoder()
      let buf = ''
      for (;;) {
        const { done, value } = await reader.read()
        if (done) break
        buf += decoder.decode(value, { stream: true })
        const parts = buf.split('\n\n')
        buf = parts.pop() ?? ''
        for (const chunk of parts) {
          const dataLine = chunk.split('\n').find(l => l.startsWith('data:'))
          if (!dataLine) continue
          let payload: { message?: string; error?: string; success?: boolean } = {}
          try { payload = JSON.parse(dataLine.slice(5).trim()) } catch { continue }
          if (payload.message) setLogs(l => [...l, payload.message!])
          if (payload.error) {
            setLogs(l => [...l, '错误：' + payload.error])
            toast.error(`Tailscale 安装失败: ${payload.error}`)
          }
          if (payload.success) {
            toast.success('Tailscale 安装完成')
          }
        }
      }
      await refetch()
    } catch (e: unknown) {
      const err = e instanceof Error ? e.message : '安装失败'
      toast.error(err)
    } finally { setBusy('') }
  }

  async function act(kind: string, path: string) {
    if (kind === 'disable') {
      const ok = await confirm({
        title: '关闭 Funnel 远程访问',
        message: '确定要关闭 Tailscale Funnel 吗？公网访问地址将立即失效。',
        variant: 'warning',
      })
      if (!ok) return
    }
    setBusy(kind)
    try {
      const res = await fetch(path, { method: 'POST' })
      let payload: { error?: string; message?: string; url?: string } | null = null
      try { payload = await res.json() } catch { /* 非 JSON 响应 */ }
      if (!res.ok) {
        const err = payload?.error || ('HTTP ' + res.status)
        toast.error(err)
      } else {
        const succ = payload?.message || (payload?.url ? `Funnel 已开启: ${payload.url}` : '操作已完成')
        toast.success(succ)
      }
      await refetch()
    } catch (e: unknown) {
      const err = e instanceof Error ? e.message : '操作失败'
      toast.error(err)
    } finally { setBusy('') }
  }

  const Row = (props: { label: string; value: string | number | boolean | undefined | null; tone?: BadgeTone }) => (
    <div class="flex items-center justify-between py-2 border-b border-subtle/50 last:border-0">
      <span class="text-sm text-muted">{props.label}</span>
      <Show when={props.tone} fallback={<span class="text-sm font-mono">{String(props.value ?? '-')}</span>}>
        <Badge tone={props.tone!}>{String(props.value)}</Badge>
      </Show>
    </div>
  )

  return (
    <div class="space-y-5 stagger">
      <div class="sticky top-16 z-20 bg-bg/90 backdrop-blur-md pt-1 pb-3 flex items-center justify-between gap-3 border-b border-subtle/50">
        <div>
          <h1 class="text-xl font-semibold">内网穿透</h1>
          <p class="text-sm text-faint mt-0.5">通过 Tailscale 把网关暴露到远程</p>
        </div>
      </div>

      <Show when={!status.loading} fallback={<Card class="p-6"><Skeleton class="h-48 w-full" /></Card>}>
        <Show when={status()} fallback={<Card class="p-6"><Empty message="无法获取隧道状态。" /></Card>}>
          {st => (
            <div class="grid lg:grid-cols-2 gap-4">
              <Card class="p-5">
                <h3 class="text-sm font-semibold mb-2">状态</h3>
                <Row label="平台" value={st().platform} />
                <Row label="已安装" value={st().installed ? '是' : '否'} tone={st().installed ? 'green' : 'gray'} />
                <Row label="守护进程" value={st().daemonRunning ? '运行中' : '未运行'} tone={st().daemonRunning ? 'green' : 'gray'} />
                <Row label="已登录" value={st().loggedIn ? '是' : '否'} tone={st().loggedIn ? 'green' : 'amber'} />
                <Row label="Funnel" value={st().funnelRunning ? '开启' : '关闭'} tone={st().funnelRunning ? 'green' : 'gray'} />
                <Show when={st().tunnelUrl}>
                  <div class="mt-3 pt-3 border-t border-subtle">
                    <div class="text-xs text-faint mb-1">访问地址</div>
                    <code class="text-xs font-mono text-accent break-all">{st().tunnelUrl}</code>
                  </div>
                </Show>
              </Card>

              <Card class="p-5 space-y-3">
                <h3 class="text-sm font-semibold">操作</h3>
                <Show when={!st().installed}>
                  <Button variant="primary" loading={busy() === 'install'} onClick={installStream}>
                    安装 Tailscale
                  </Button>
                </Show>
                <Show when={st().installed}>
                  <div class="flex flex-wrap gap-2">
                    <Show when={!st().funnelRunning}>
                      <Button variant="primary" loading={busy() === 'enable'} onClick={() => act('enable', '/api/tunnel/tailscale-enable')}>
                        开启 Funnel
                      </Button>
                    </Show>
                    <Show when={st().funnelRunning}>
                      <Button variant="danger" loading={busy() === 'disable'} onClick={() => act('disable', '/api/tunnel/tailscale-disable')}>
                        关闭 Funnel
                      </Button>
                    </Show>
                  </div>
                </Show>
                <Show when={logs().length > 0}>
                  <div class="max-h-40 overflow-y-auto space-y-0.5 bg-bg-elevated border border-subtle rounded-control p-2">
                    <For each={logs()}>
                      {l => <div class="text-xs font-mono text-faint leading-relaxed">{l}</div>}
                    </For>
                  </div>
                </Show>
              </Card>
            </div>
          )}
        </Show>
      </Show>
    </div>
  )
}

export default Tunnel
