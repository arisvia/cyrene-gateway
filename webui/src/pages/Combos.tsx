import { type Component, For, Show, createSignal, createMemo, createResource } from 'solid-js'
import { useGatewayStore } from '@/stores/gateway'
import { api } from '@/lib/api'
import { useToast } from '@/lib/toast'
import type { Combo } from '@/types/domain'
import { Card, Badge, Button, Input, Select, Modal, Field, Empty, PageHeader, IconClose, confirm } from '@/components/ui'

const STRATEGY_LABEL: Record<string, string> = {
  fallback: '故障回退', 'round-robin': '轮询',
}

const Combos: Component = () => {
  const store = useGatewayStore()
  const toast = useToast()
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
    } catch (e: unknown) {
      toast.error(e instanceof Error ? e.message : '保存组合失败')
    } finally { setSaving(false) }
  }

  return (
    <div class="space-y-5 stagger">
      <PageHeader
        title="模型组合"
        subtitle="把多个模型编排为一个入口，按策略自动轮转或回退"
        actions={
          <>
            <Button variant="ghost" onClick={() => refetch()}>刷新模型表</Button>
            <Button variant="primary" onClick={openCreate}>+ 新建组合</Button>
          </>
        }
      />

      <Show when={store.combos().length > 0} fallback={
        <Card class="p-6"><Empty message="还没有组合。新建一个把多个上游模型串成统一入口。" /></Card>
      }>
        <div class="grid gap-3">
          <For each={store.combos()}>
            {c => (
              <Card hover class="p-4">
                <div class="flex items-start justify-between gap-4">
                  <div class="min-w-0 flex-1">
                    <div class="flex items-center gap-2 flex-wrap">
                      <span class="font-medium text-sm">{c.name}</span>
                      <Badge tone="blue">{STRATEGY_LABEL[c.kind] || c.kind}</Badge>
                      <Badge tone="gray">{c.models?.length ?? 0} 个模型</Badge>
                    </div>
                    <div class="mt-1.5 flex flex-wrap gap-1.5">
                      <For each={c.models ?? []}>
                        {m => {
                          const displayName = () => modelNames()[m] || m
                          return (
                            <span class="px-2 py-0.5 rounded-md bg-hover text-[11px] font-sans text-muted hover:text-foreground transition-colors" title={m}>
                              {displayName()}
                            </span>
                          )
                        }}
                      </For>
                    </div>
                  </div>
                  <div class="flex gap-1.5 shrink-0">
                    <Button size="sm" variant="ghost" onClick={() => openEdit(c)}>编辑</Button>
                    <Button
                      size="sm"
                      variant="danger"
                      onClick={async () => {
                        const ok = await confirm({
                          title: '删除模型组合',
                          message: `确定要删除组合「${c.name}」吗？依赖此组合的下游请求将无法正常解析。`,
                          variant: 'danger',
                        })
                        if (ok) {
                          await store.deleteCombo(c.id)
                        }
                      }}
                    >
                      删除
                    </Button>
                  </div>
                </div>
              </Card>
            )}
          </For>
        </div>
      </Show>

      <Modal open={open()} title={editing() ? '编辑组合' : '新建组合'} onClose={() => setOpen(false)}>
        <div class="space-y-4">
          <Field label="组合名称" hint="调用时使用的模型名，例如 fast-coding">
            <Input value={form().name} placeholder="fast-coding" onInput={v => setForm(f => ({ ...f, name: v }))} />
          </Field>
          <Field label="调度策略" hint="fallback：按顺序失败转移；round-robin：轮流">
            <Select
              value={form().kind}
              options={[
                { value: 'fallback', label: '故障回退' },
                { value: 'round-robin', label: '轮询' },
              ]}
              onChange={v => setForm(f => ({ ...f, kind: v }))}
            />
          </Field>

          <Field label="成员模型" hint="按顺序排列，fallback 模式下靠前的优先">
            <div class="flex gap-2">
              <Select
                class="flex-1"
                value={modelPick()}
                options={[
                  { value: '', label: '选择模型…' },
                  ...(available() ?? []).map(m => ({
                    value: m.id,
                    label: m.name !== m.id ? `${m.name} (${m.id})` : m.id,
                  })),
                ]}
                onChange={setModelPick}
              />
              <Button variant="secondary" disabled={!modelPick()} onClick={addModel}>添加</Button>
            </div>
            <div class="mt-2 flex flex-wrap gap-1.5">
              <Show when={form().models.length > 0} fallback={<span class="text-[11px] text-faint">尚未添加模型</span>}>
                <For each={form().models}>
                  {m => {
                    const displayName = () => modelNames()[m] || m
                    return (
                      <span class="inline-flex items-center gap-1.5 px-2 py-1 rounded-control bg-hover text-xs" title={m}>
                        <span class="font-medium text-foreground">{displayName()}</span>
                        <button class="text-faint hover:text-danger ml-0.5 cursor-pointer" onClick={() => removeModel(m)} title="移除"><IconClose size={12} /></button>
                      </span>
                    )
                  }}
                </For>
              </Show>
            </div>
          </Field>

          <div class="flex justify-end gap-2 pt-1">
            <Button variant="ghost" onClick={() => setOpen(false)}>取消</Button>
            <Button variant="primary" loading={saving()} disabled={!canSave()} onClick={submit}>
              {editing() ? '保存' : '创建'}
            </Button>
          </div>
        </div>
      </Modal>
    </div>
  )
}

export default Combos
