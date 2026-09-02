import { type Component, For, Show, createSignal, createMemo, createResource } from 'solid-js'
import { useGatewayStore } from '@/stores/gateway'
import { api } from '@/lib/api'
import { Card, Badge, Button, Input, Select, Modal, Field, Empty } from '@/components/ui'

const STRATEGY_LABEL: Record<string, string> = {
  fallback: '故障回退', 'round-robin': '轮询',
}

const Combos: Component = () => {
  const store = useGatewayStore()
  const [open, setOpen] = createSignal(false)
  const [editing, setEditing] = createSignal<any>(null)
  const [saving, setSaving] = createSignal(false)
  const [form, setForm] = createSignal({ name: '', kind: 'fallback', models: [] as string[] })
  const [modelPick, setModelPick] = createSignal('')

  // 可用模型来自网关统一模型表
  const [available, { refetch }] = createResource(async () => {
    try {
      const r = await api('/v1/models')
      return (r?.data ?? []).map((m: any) => m.id as string)
    } catch { return [] as string[] }
  })

  const canSave = createMemo(() => form().name.trim().length > 0 && form().models.length > 0)

  function openCreate() {
    setEditing(null)
    setForm({ name: '', kind: 'fallback', models: [] })
    setOpen(true)
  }

  function openEdit(c: any) {
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
    } catch (e) { console.error('[combos] save failed:', e) }
    finally { setSaving(false) }
  }

  return (
    <div class="space-y-5">
      <div class="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 class="text-xl font-semibold">模型组合</h1>
          <p class="text-sm text-faint mt-0.5">
            把多个模型编排为一个入口，按策略自动轮转或回退
          </p>
        </div>
        <div class="flex gap-2">
          <Button variant="ghost" onClick={() => refetch()}>刷新模型表</Button>
          <Button variant="primary" onClick={openCreate}>+ 新建组合</Button>
        </div>
      </div>

      <Show when={store.combos().length > 0} fallback={
        <Card class="p-6"><Empty message="还没有组合。新建一个把多个上游模型串成统一入口。" /></Card>
      }>
        <div class="grid gap-3">
          <For each={store.combos()}>
            {c => (
              <Card class="p-4">
                <div class="flex items-start justify-between gap-4">
                  <div class="min-w-0 flex-1">
                    <div class="flex items-center gap-2 flex-wrap">
                      <span class="font-medium text-sm">{c.name}</span>
                      <Badge tone="blue">{STRATEGY_LABEL[c.kind] || c.kind}</Badge>
                      <Badge tone="gray">{c.models?.length ?? 0} 个模型</Badge>
                    </div>
                    <div class="mt-1.5 flex flex-wrap gap-1.5">
                      <For each={c.models ?? []}>
                        {m => (
                          <span class="px-2 py-0.5 rounded-md bg-hover text-[11px] font-mono text-muted">{m}</span>
                        )}
                      </For>
                    </div>
                  </div>
                  <div class="flex gap-1.5 shrink-0">
                    <Button size="sm" variant="ghost" onClick={() => openEdit(c)}>编辑</Button>
                    <Button size="sm" variant="danger" onClick={() => store.deleteCombo(c.id)}>删除</Button>
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
                options={[{ value: '', label: '选择模型…' }, ...(available() ?? []).map((m: string) => ({ value: m, label: m }))]}
                onChange={setModelPick}
              />
              <Button variant="secondary" disabled={!modelPick()} onClick={addModel}>添加</Button>
            </div>
            <div class="mt-2 flex flex-wrap gap-1.5">
              <Show when={form().models.length > 0} fallback={<span class="text-[11px] text-faint">尚未添加模型</span>}>
                <For each={form().models}>
                  {m => (
                    <span class="inline-flex items-center gap-1.5 px-2 py-1 rounded-control bg-hover text-xs">
                      <span class="font-mono">{m}</span>
                      <button class="text-faint hover:text-danger" onClick={() => removeModel(m)}>×</button>
                    </span>
                  )}
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
