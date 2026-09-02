import { type Component, For, Show, createSignal, createResource } from 'solid-js'
import { api } from '@/lib/api'
import { Card, Badge, Button, Empty, Skeleton } from '@/components/ui'

const Tunnel: Component = () => {
  const [status, { refetch }] = createResource(async () => {
    try { return await api('/api/tunnel/status') } catch { return null }
  })
  const [busy, setBusy] = createSignal('')
  const [msg, setMsg] = createSignal('')
  const [logs, setLogs] = createSignal<string[]>([])

  // install 返回 text/event-stream（tunnel.go:23-47），必须流式读取而非 JSON
  async function installStream() {
    setBusy('install'); setLogs([])
    try {
      const res = await fetch('/api/tunnel/tailscale-install', { method: 'POST' })
      if (!res.body) { setMsg('无法读取安装流'); return }
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
          let payload: any = {}
          try { payload = JSON.parse(dataLine.slice(5).trim()) } catch { continue }
          if (payload.message) setLogs(l => [...l, payload.message])
          if (payload.error) { setLogs(l => [...l, '错误：' + payload.error]); setMsg(payload.error) }
          if (payload.success) setMsg('安装完成')
        }
      }
      await refetch()
    } catch (e: any) { setMsg(e?.message || '安装失败') }
    finally { setBusy('') }
  }

  async function act(kind: string, path: string) {
    setBusy(kind); setMsg('')
    try {
      const res = await fetch(path, { method: 'POST' })
      let payload: any = null
      try { payload = await res.json() } catch { /* 非 JSON 响应 */ }
      if (!res.ok) setMsg(payload?.error || ('HTTP ' + res.status))
      else setMsg(payload?.message || payload?.url || '已完成')
      await refetch()
    } catch (e: any) { setMsg(e?.message || '操作失败') }
    finally { setBusy('') }
  }

  const Row = (props: { label: string; value: any; tone?: string }) => (
    <div class="flex items-center justify-between py-2 border-b border-subtle/50 last:border-0">
      <span class="text-sm text-muted">{props.label}</span>
      <Show when={props.tone} fallback={<span class="text-sm font-mono">{String(props.value ?? '-')}</span>}>
        <Badge tone={props.tone as any}>{props.value}</Badge>
      </Show>
    </div>
  )

  return (
    <div class="space-y-5">
      <div>
        <h1 class="text-xl font-semibold">内网穿透</h1>
        <p class="text-sm text-faint mt-0.5">通过 Tailscale 把网关暴露到远程</p>
      </div>

      <Show when={!status.loading} fallback={<Card class="p-6"><Skeleton class="h-48 w-full" /></Card>}>
        <Show when={status()} fallback={<Card class="p-6"><Empty message="无法获取隧道状态。" /></Card>}>
          <div class="grid lg:grid-cols-2 gap-4">
            <Card class="p-5">
              <h3 class="text-sm font-semibold mb-2">状态</h3>
              <Row label="平台" value={status().platform} />
              <Row label="已安装" value={status().installed ? '是' : '否'} tone={status().installed ? 'green' : 'gray'} />
              <Row label="守护进程" value={status().daemonRunning ? '运行中' : '未运行'} tone={status().daemonRunning ? 'green' : 'gray'} />
              <Row label="已登录" value={status().loggedIn ? '是' : '否'} tone={status().loggedIn ? 'green' : 'amber'} />
              <Row label="Funnel" value={status().funnelRunning ? '开启' : '关闭'} tone={status().funnelRunning ? 'green' : 'gray'} />
              <Show when={status().tunnelUrl}>
                <div class="mt-3 pt-3 border-t border-subtle">
                  <div class="text-xs text-faint mb-1">访问地址</div>
                  <code class="text-xs font-mono text-accent break-all">{status().tunnelUrl}</code>
                </div>
              </Show>
            </Card>

            <Card class="p-5 space-y-3">
              <h3 class="text-sm font-semibold">操作</h3>
              <Show when={!status().installed}>
                <Button variant="primary" loading={busy() === 'install'} onClick={installStream}>
                  安装 Tailscale
                </Button>
              </Show>
              <Show when={status().installed}>
                <div class="flex flex-wrap gap-2">
                  <Show when={!status().funnelRunning}>
                    <Button variant="primary" loading={busy() === 'enable'} onClick={() => act('enable', '/api/tunnel/tailscale-enable')}>
                      开启 Funnel
                    </Button>
                  </Show>
                  <Show when={status().funnelRunning}>
                    <Button variant="danger" loading={busy() === 'disable'} onClick={() => act('disable', '/api/tunnel/tailscale-disable')}>
                      关闭 Funnel
                    </Button>
                  </Show>
                </div>
              </Show>
              <Show when={logs().length > 0}>
                <div class="max-h-40 overflow-y-auto space-y-0.5 bg-bg-elevated border border-subtle rounded-control p-2">
                  <For each={logs()}>
                    {(l: string) => <div class="text-[11px] font-mono text-muted">{l}</div>}
                  </For>
                </div>
              </Show>
              <Show when={msg()}>
                <div class="text-xs text-faint px-2 py-1.5 rounded-control bg-hover">{msg()}</div>
              </Show>
              <p class="text-[11px] text-faint pt-2 border-t border-subtle">
                开启 Funnel 会把网关暴露到公网，请确保已启用登录与 API Key 校验。
              </p>
            </Card>
          </div>
        </Show>
      </Show>
    </div>
  )
}

export default Tunnel
