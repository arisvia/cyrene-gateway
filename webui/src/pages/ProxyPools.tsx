import { type Component, For, Show, createSignal, onMount } from 'solid-js'
import { useGatewayStore } from '@/stores/gateway'
import type { ProxyPool } from '@/types/domain'
import { Card, Badge, Button, Input, Select, Toggle, Modal, Field, Empty } from '@/components/ui'

const ProxyPools: Component = () => {
  const store = useGatewayStore()
  const [open, setOpen] = createSignal(false)
  const [editing, setEditing] = createSignal<ProxyPool | null>(null)
  const [saving, setSaving] = createSignal(false)
  const [form, setForm] = createSignal({ name: '', proxyUrl: '', type: 'http', noProxy: '', strictProxy: true })

  onMount(() => store.loadProxyPools())

  function openCreate() {
    setEditing(null)
    setForm({ name: '', proxyUrl: '', type: 'http', noProxy: '', strictProxy: true })
    setOpen(true)
  }

  function openEdit(p: ProxyPool) {
    setEditing(p)
    setForm({
      name: p.name || '', proxyUrl: p.proxyUrl || '', type: p.type || 'http',
      noProxy: p.noProxy || '', strictProxy: p.strictProxy ?? true,
    })
    setOpen(true)
  }

  async function submit() {
    const f = form()
    if (!f.name.trim() || !f.proxyUrl.trim()) return
    setSaving(true)
    try {
      await store.saveProxyPool({ id: editing()?.id, ...f })
      setOpen(false)
    } catch (e) { console.error('[pools] save failed:', e) }
    finally { setSaving(false) }
  }

  return (
    <div class="space-y-5 stagger">
      <div class="flex items-center justify-between">
        <div>
          <h1 class="text-xl font-semibold">代理池</h1>
          <p class="text-sm text-faint mt-0.5">出站请求的 HTTP 代理轮换</p>
        </div>
        <Button variant="primary" onClick={openCreate}>+ 新建代理池</Button>
      </div>

      <Show when={store.proxyPools().length > 0} fallback={
        <Card class="p-6"><Empty message="尚未配置代理池。" /></Card>
      }>
        <div class="grid gap-3">
          <For each={store.proxyPools()}>
            {p => (
              <Card hover class="p-4">
                <div class="flex items-center justify-between gap-4 flex-wrap">
                  <div class="min-w-0">
                    <div class="flex items-center gap-2">
                      <span class="font-medium text-sm">{p.name}</span>
                      <Badge tone={p.isActive ? 'green' : 'gray'}>{p.isActive ? '启用' : '停用'}</Badge>
                      <Badge tone="blue">{p.type || 'http'}</Badge>
                      <Show when={p.strictProxy}><Badge tone="amber">强制</Badge></Show>
                    </div>
                    <div class="mt-1 text-xs text-faint font-mono truncate">{p.proxyUrl}</div>
                    <Show when={p.noProxy}>
                      <div class="text-[11px] text-faint truncate">排除：{p.noProxy}</div>
                    </Show>
                  </div>
                  <div class="flex items-center gap-1.5 shrink-0">
                    <Button size="sm" variant="ghost" onClick={() => openEdit(p)}>编辑</Button>
                    <Toggle checked={p.isActive} onChange={() => store.toggleProxyPool(p)} />
                    <Button size="sm" variant="danger" onClick={() => store.deleteProxyPool(p.id)}>删除</Button>
                  </div>
                </div>
              </Card>
            )}
          </For>
        </div>
      </Show>

      <Modal open={open()} title={editing() ? '编辑代理池' : '新建代理池'} onClose={() => setOpen(false)}>
        <div class="space-y-4">
          <Field label="名称"><Input value={form().name} onInput={v => setForm(f => ({ ...f, name: v }))} placeholder="my-proxy" /></Field>
          <Field label="代理地址" hint="例如 http://127.0.0.1:7890">
            <Input value={form().proxyUrl} onInput={v => setForm(f => ({ ...f, proxyUrl: v }))} placeholder="http://host:port" />
          </Field>
          <Field label="类型">
            <Select value={form().type} options={[
              { value: 'http', label: 'HTTP' }, { value: 'socks5', label: 'SOCKS5' },
            ]} onChange={v => setForm(f => ({ ...f, type: v }))} />
          </Field>
          <Field label="排除地址" hint="逗号分隔，直连不走代理">
            <Input value={form().noProxy} onInput={v => setForm(f => ({ ...f, noProxy: v }))} placeholder="localhost,127.0.0.1" />
          </Field>
          <Field label="强制代理" hint="开启后该池的连接必须走代理，否则失败">
            <Toggle checked={form().strictProxy} onChange={v => setForm(f => ({ ...f, strictProxy: v }))} />
          </Field>
          <div class="flex justify-end gap-2 pt-1">
            <Button variant="ghost" onClick={() => setOpen(false)}>取消</Button>
            <Button variant="primary" loading={saving()} onClick={submit}>保存</Button>
          </div>
        </div>
      </Modal>
    </div>
  )
}

export default ProxyPools
