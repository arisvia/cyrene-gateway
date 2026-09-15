import { type Component, For, Show, createSignal, onMount } from 'solid-js'
import { useGatewayStore } from '@/stores/gateway'
import type { ProxyPool } from '@/types/domain'
import { useToast } from '@/lib/toast'
import { useI18n } from '@/i18n'
import { Card, Badge, Button, Input, Select, Toggle, Modal, Field, Empty, PageHeader, IconGlobe, confirm } from '@/components/ui'

const ProxyPools: Component = () => {
  const store = useGatewayStore()
  const { t } = useI18n()
  const toast = useToast()
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
    } catch (e: unknown) {
      toast.error(t('toast.saveProxyFailed'))
    } finally { setSaving(false) }
  }

  return (
    <div class="space-y-5 stagger">
      <PageHeader
        title={t('proxies.title')}
        subtitle={t('proxies.subtitle')}
        actions={
          <Show when={store.proxyPools().length > 0}>
            <Button variant="primary" size="sm" onClick={openCreate}>+ {t('proxies.newPool')}</Button>
          </Show>
        }
      />
      <Show when={store.proxyPools().length > 0} fallback={
        <Card class="p-12 border-dashed border-subtle">
          <Empty
            icon={<IconGlobe size={24} />}
            title={t('proxies.emptyTitle')}
            description={t('proxies.emptyDesc')}
            action={
              <Button variant="primary" size="sm" onClick={openCreate} class="gap-1.5">
                <span>+ {t('proxies.newPool')}</span>
              </Button>
            }
          />
        </Card>
      }>
        <div class="grid gap-3">
          <For each={store.proxyPools()}>
            {p => (
              <Card hover class="p-4">
                <div class="flex items-center justify-between gap-4 flex-wrap">
                  <div class="min-w-0">
                    <div class="flex items-center gap-2">
                      <span class="font-medium text-sm">{p.name}</span>
                      <Badge tone={p.isActive ? 'green' : 'gray'}>{p.isActive ? t('common.enabled') : t('common.disabled')}</Badge>
                      <Badge tone="blue">{p.type || 'http'}</Badge>
                      <Show when={p.strictProxy}><Badge tone="amber">{t('proxies.strict')}</Badge></Show>
                    </div>
                    <div class="mt-1 text-xs text-faint font-mono truncate">{p.proxyUrl}</div>
                    <Show when={p.noProxy}>
                      <div class="text-[11px] text-faint truncate">{t('proxies.noProxyPrefix')}{p.noProxy}</div>
                    </Show>
                  </div>
                  <div class="flex items-center gap-1.5 shrink-0">
                    <Button size="sm" variant="ghost" onClick={() => openEdit(p)}>{t('common.edit')}</Button>
                    <Toggle checked={p.isActive} onChange={() => store.toggleProxyPool(p)} />
                    <Button
                      size="sm"
                      variant="danger"
                      onClick={async () => {
                        const ok = await confirm({
                          title: t('proxies.deleteConfirmTitle'),
                          message: t('proxies.deleteConfirmMessage', { name: p.name }),
                          variant: 'danger',
                        })
                        if (ok) {
                          await store.deleteProxyPool(p.id)
                        }
                      }}
                    >
                      {t('common.delete')}
                    </Button>
                  </div>
                </div>
              </Card>
            )}
          </For>
        </div>
      </Show>

      <Modal open={open()} title={editing() ? t('proxies.editPool') : t('proxies.newPool')} onClose={() => setOpen(false)}>
        <div class="space-y-4">
          <Field label={t('proxies.name')}><Input value={form().name} onInput={v => setForm(f => ({ ...f, name: v }))} placeholder="my-proxy" /></Field>
          <Field label={t('proxies.proxyUrl')} hint={t('proxies.proxyUrlHint')}>
            <Input value={form().proxyUrl} onInput={v => setForm(f => ({ ...f, proxyUrl: v }))} placeholder="http://host:port" />
          </Field>
          <Field label={t('proxies.type')}>
            <Select value={form().type} options={[
              { value: 'http', label: 'HTTP' }, { value: 'socks5', label: 'SOCKS5' },
            ]} onChange={v => setForm(f => ({ ...f, type: v }))} />
          </Field>
          <Field label={t('proxies.noProxy')} hint={t('proxies.noProxyHint')}>
            <Input value={form().noProxy} onInput={v => setForm(f => ({ ...f, noProxy: v }))} placeholder="localhost,127.0.0.1" />
          </Field>
          <Field label={t('proxies.strictProxy')} hint={t('proxies.strictProxyHint')}>
            <Toggle checked={form().strictProxy} onChange={v => setForm(f => ({ ...f, strictProxy: v }))} />
          </Field>
          <div class="flex justify-end gap-2 pt-1">
            <Button variant="ghost" onClick={() => setOpen(false)}>{t('common.cancel')}</Button>
            <Button variant="primary" loading={saving()} onClick={submit}>{t('common.save')}</Button>
          </div>
        </div>
      </Modal>
    </div>
  )
}

export default ProxyPools
