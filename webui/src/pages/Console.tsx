import { type Component, For, Show, createSignal, createResource } from 'solid-js'
import { api, apiPost } from '@/lib/api'
import { Card, Badge, Button, Input, Select, Empty } from '@/components/ui'

interface Turn { role: string; content: string }

const Console: Component = () => {
  const [models] = createResource(async () => {
    try {
      const res = await api('/v1/models') as { data?: Array<{ id: string }> } | null
      return res?.data ?? []
    } catch { return [] }
  })
  const [model, setModel] = createSignal('')
  const [prompt, setPrompt] = createSignal('')
  const [busy, setBusy] = createSignal(false)
  const [history, setHistory] = createSignal<Turn[]>([])
  const [err, setErr] = createSignal('')

  async function send() {
    const text = prompt().trim()
    if (!text || busy()) return
    setErr('')
    setHistory(h => [...h, { role: 'user', content: text }])
    setPrompt('')
    setBusy(true)
    try {
      const targetModel = model() || models()?.[0]?.id || ''
      const r = await apiPost('/v1/chat/completions', {
        model: targetModel,
        messages: [...history(), { role: 'user', content: text }],
      }) as { choices?: Array<{ message?: { content?: string } }> } | null
      const reply = r?.choices?.[0]?.message?.content ?? '(无返回)'
      setHistory(h => [...h, { role: 'assistant', content: reply }])
    } catch (e: unknown) {
      setErr(e instanceof Error ? e.message : '请求失败')
      setHistory(h => h.slice(0, -1))
    } finally { setBusy(false) }
  }

  return (
    <div class="space-y-5">
      <div>
        <h1 class="text-xl font-semibold">控制台</h1>
        <p class="text-sm text-faint mt-0.5">直接对网关发起 chat/completions 请求，验证链路</p>
      </div>

      <Card class="p-4 flex flex-wrap items-center gap-2">
        <Select
          class="min-w-[220px]"
          value={model()}
          options={[{ value: '', label: '选择模型…' }, ...(models() ?? []).map(m => ({ value: m.id, label: m.id }))]}
          onChange={setModel}
        />
        <Show when={model()}>
          <Badge tone="blue">{model()}</Badge>
        </Show>
        <Button size="sm" variant="ghost" onClick={() => setHistory([])}>清空</Button>
      </Card>

      <Card class="p-5 min-h-[280px] max-h-[480px] overflow-y-auto space-y-3">
        <Show when={history().length > 0} fallback={<Empty message="发送一条消息开始测试。" />}>
          <For each={history()}>
            {t => (
              <div class={`flex ${t.role === 'user' ? 'justify-end' : 'justify-start'}`}>
                <div class={`max-w-[78%] px-3.5 py-2 rounded-2xl text-sm whitespace-pre-wrap ${t.role === 'user'
                  ? 'bg-accent text-white'
                  : 'bg-hover text-text'}`}>
                  {t.content}
                </div>
              </div>
            )}
          </For>
          <Show when={busy()}>
            <div class="flex justify-start">
              <div class="px-3.5 py-2 rounded-2xl bg-hover text-sm text-faint">思考中…</div>
            </div>
          </Show>
        </Show>
      </Card>

      <Show when={err()}>
        <div class="px-3 py-2 rounded-control text-xs bg-danger/10 text-danger">{err()}</div>
      </Show>

      <Card class="p-3 flex gap-2">
        <Input
          value={prompt()}
          placeholder="输入消息，回车发送…"
          onInput={setPrompt}
          disabled={busy()}
        />
        <Button variant="primary" loading={busy()} disabled={!prompt().trim()} onClick={send}>发送</Button>
      </Card>
    </div>
  )
}

export default Console
