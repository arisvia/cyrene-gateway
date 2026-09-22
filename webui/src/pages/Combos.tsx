import { type Component, For, Show, createSignal, createMemo, createResource } from 'solid-js'
import { A } from '@solidjs/router'
import { useGatewayStore } from '@/stores/gateway'
import { api } from '@/lib/api'
import { useI18n } from '@/i18n'
import type { Combo } from '@/types/domain'
import { Card, Badge, Button, IconButton, Input, Select, Modal, Field, Empty, PageHeader, IconClose, IconLayers, IconExternalLink, Skeleton, LoadState, confirm } from '@/components/ui'

const Combos: Component = () => {
  const store = useGatewayStore()
  const { t } = useI18n()
  const strategyLabel = (): Record<string, string> => ({
    fallback: t('combos.fallbackStrategy'),
    'round-robin': t('combos.roundRobinStrategy'),
  })
  const [open, setOpen] = createSignal(false)
  const [editing, setEditing] = createSignal<Combo | null>(null)
  const [saving, setSaving] = createSignal(false)
  const [form, setForm] = createSignal<{ name: string; kind: string; models: string[] }>({ name: '', kind: 'fallback', models: [] })
  const [modelPick, setModelPick] = createSignal('')

  // 可用模型来自网关统一模型表
  // 可用模型来自网关统一模型表（含 display_name 与 id）
  const [available, { refetch }] = createResource(async () => {
    try {
      const r = await api('/v1/models') as { data?: Array<{ id: string; display_name?: string }> } | null
      return (r?.data ?? []).map(m => ({
        id: m.id,
        name: m.display_name || m.id,
      }))
    } catch { return [] as Array<{ id: string; name: string }> }
  })

  // 模型 ID 到友好展示名称的映射字典
  const modelNames = createMemo(() => {
    const map: Record<string, string> = {}
    for (const m of available() ?? []) {
      map[m.id] = m.name
    }
    return map
  })

  const canSave = createMemo(() => form().name.trim().length > 0 && form().models.length > 0)

  function openCreate() {
    setEditing(null)
    setForm({ name: '', kind: 'fallback', models: [] })
    setOpen(true)
  }

  function openEdit(c: Combo) {
    setEditing(c)
    setForm({ name: c.name || '', kind: c.kind || 'fallback', models: c.models || [] })
    setOpen(true)
  }

  function addModel() {
    const m = modelPick()
    if (!m) return
    setForm(f => (f.models.includes(m) ? f : { ...f, models: [...f.models, m] }))
    setModelPick('')
  }

  function removeModel(m: string) {
    setForm(f => ({ ...f, models: f.models.filter(x => x !== m) }))
  }

  async function submit() {
    if (!canSave()) return
    setSaving(true)
    try {
      await store.saveCombo({
        id: editing()?.id,
        name: form().name.trim(),
        kind: form().kind,
        models: form().models,
      })
      setOpen(false)
    } catch {
      // Error is automatically surfaced by api.ts
    } finally { setSaving(false) }
  }

  return (
    <div class="space-y-5 stagger">
      <PageHeader
        title={t('combos.title')}
        subtitle={t('combos.subtitle')}
        actions={
          <div class="flex items-center gap-2 flex-wrap">
            <Button variant="secondary" size="sm" onClick={() => refetch()}>{t('combos.refreshModels')}</Button>
            <Show when={store.combos().length > 0}>
              <Button variant="primary" size="sm" onClick={openCreate}>+ {t('combos.newCombo')}</Button>
            </Show>
          </div>
        }
      />

      <LoadState ready={store.loaded.combos} error={store.loadErrors.combos} onRetry={() => store.loadCore()} fallback={
        <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
          <For each={[1, 2, 3, 4]}>
            {() => (
              <Card class="p-4 space-y-3">
                <Skeleton class="h-5 w-32" />
                <Skeleton class="h-3 w-full" />
                <Skeleton class="h-3 w-24" />
              </Card>
            )}
          </For>
        </div>
      }>
      <Show when={store.combos().length > 0} fallback={
        <Card class="p-12 border-dashed border-subtle">
          <Empty
            icon={<IconLayers size={24} />}
            title={t('combos.emptyTitle')}
            description={t('combos.emptyDesc')}
            action={
              <>
                <Button variant="primary" size="sm" onClick={openCreate} class="gap-1.5">
                  <span>+ {t('combos.newCombo')}</span>
                </Button>
                <A href="/providers?tab=catalog">
                  <Button variant="secondary" size="sm" class="gap-1.5">
                    <span>{t('combos.browseMarket')}</span>
                    <IconExternalLink size={13} />
                  </Button>
                </A>
              </>
            }
          />
        </Card>
      }>
        <div class="grid gap-3">
          <For each={store.combos()}>
            {c => (
              <Card hover class="p-4">
                <div class="flex flex-col sm:flex-row items-start justify-between gap-4">
                  <div class="min-w-0 flex-1">
                    <div class="flex items-center gap-2 flex-wrap">
                      <span class="min-w-0 font-medium text-sm wrap-anywhere">{c.name}</span>
                      <Badge tone="blue">{strategyLabel()[c.kind] || c.kind}</Badge>
                      <Badge tone="gray">{t('combos.modelCount', { count: c.models?.length ?? 0 })}</Badge>
                    </div>
                    <div class="mt-1.5 flex flex-wrap gap-1.5">
                      <For each={c.models ?? []}>
                        {m => {
                          const displayName = () => modelNames()[m] || m
                          const hasDifferentName = () => displayName() !== m
                          return (
                            <span class="inline-flex flex-wrap max-w-full min-w-0 items-center gap-x-1 gap-y-0.5 px-2 py-1 rounded-md bg-surface-inset text-xs font-sans text-muted hover:text-foreground transition-colors wrap-anywhere" title={m}>
                              <span class="font-medium">{displayName()}</span>
                              <Show when={hasDifferentName()}>
                                <span class="min-w-0 text-xs text-faint font-mono opacity-80 wrap-anywhere">({m})</span>
                              </Show>
                            </span>
                          )
                        }}
                      </For>
                    </div>
                  </div>
                  <div class="flex flex-wrap gap-1.5 shrink-0">
                    <Button size="sm" variant="ghost" onClick={() => openEdit(c)}>{t('common.edit')}</Button>
                    <Button
                      size="sm"
                      variant="danger"
                      onClick={async () => {
                        const ok = await confirm({
                          title: t('combos.deleteConfirmTitle'),
                          message: t('combos.deleteConfirmMessage', { name: c.name }),
                          variant: 'danger',
                        })
                        if (ok) {
                          await store.deleteCombo(c.id)
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
      </LoadState>

      <Modal open={open()} title={editing() ? t('combos.editCombo') : t('combos.newCombo')} onClose={() => setOpen(false)}>
        <div class="space-y-4">
          <Field label={t('combos.name')} hint={t('combos.nameHint')}>
            <Input value={form().name} placeholder="fast-coding" onInput={v => setForm(f => ({ ...f, name: v }))} />
          </Field>
          <Field label={t('combos.strategy')} hint={t('combos.strategyHint')}>
            <Select
              value={form().kind}
              options={[
                { value: 'fallback', label: t('combos.fallbackStrategy') },
                { value: 'round-robin', label: t('combos.roundRobinStrategy') },
              ]}
              onChange={v => setForm(f => ({ ...f, kind: v }))}
            />
          </Field>

          <Field label={t('combos.memberModels')} hint={t('combos.memberModelsHint')}>
            <div class="flex gap-2">
              <Select
                class="flex-1 min-w-0"
                value={modelPick()}
                options={[
                  { value: '', label: t('combos.selectModel') },
                  ...(available() ?? []).map(m => ({
                    value: m.id,
                    label: m.name !== m.id ? `${m.name} (${m.id})` : m.id,
                  })),
                ]}
                onChange={setModelPick}
              />
              <Button variant="secondary" disabled={!modelPick()} onClick={addModel}>{t('combos.add')}</Button>
            </div>
            <div class="mt-2 flex flex-wrap gap-1.5">
              <Show when={form().models.length > 0} fallback={<span class="text-[11px] text-faint">{t('combos.noModelsAdded')}</span>}>
                <For each={form().models}>
                  {m => {
                    const displayName = () => modelNames()[m] || m
                    const hasDifferentName = () => displayName() !== m
                    return (
                      <span class="inline-flex flex-wrap max-w-full min-w-0 items-center gap-1 px-2 py-1 rounded-control bg-surface-inset text-xs wrap-anywhere" title={m}>
                        <span class="min-w-0 font-medium text-foreground">{displayName()}</span>
                        <Show when={hasDifferentName()}>
                          <span class="min-w-0 text-xs text-faint font-mono opacity-80 wrap-anywhere">({m})</span>
                        </Show>
                        <IconButton size="sm" variant="ghost" class="text-faint hover:text-danger ml-0.5" onClick={() => removeModel(m)} title={t('combos.remove')} aria-label={t('combos.remove')}><IconClose size={12} /></IconButton>
                      </span>
                    )
                  }}
                </For>
              </Show>
            </div>
          </Field>

          <div class="flex justify-end gap-2 pt-1">
            <Button variant="ghost" onClick={() => setOpen(false)}>{t('common.cancel')}</Button>
            <Button variant="primary" loading={saving()} disabled={!canSave()} onClick={submit}>
              {editing() ? t('common.save') : t('common.create')}
            </Button>
          </div>
        </div>
      </Modal>
    </div>
  )
}

export default Combos
