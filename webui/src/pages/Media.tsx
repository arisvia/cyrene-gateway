import { type Component, For, Show, createSignal } from 'solid-js'
import { apiPost } from '@/lib/api'
import { Card, Badge, Button, Input, Field, Empty } from '@/components/ui'

type Cap = 'image' | 'tts' | 'stt' | 'embeddings' | 'search'

const CAPS: { id: Cap; label: string; endpoint: string; hint: string }[] = [
  { id: 'image', label: '图像生成', endpoint: '/v1/images/generations', hint: '根据提示词生成图片' },
  { id: 'tts', label: '语音合成', endpoint: '/v1/audio/speech', hint: '文本转语音' },
  { id: 'stt', label: '语音识别', endpoint: '/v1/audio/transcriptions', hint: '音频转文本' },
  { id: 'embeddings', label: '向量嵌入', endpoint: '/v1/embeddings', hint: '文本向量化' },
  { id: 'search', label: '联网搜索', endpoint: '/v1/search', hint: '网络检索' },
]

const Media: Component = () => {
  const [active, setActive] = createSignal<Cap>('image')
  const [text, setText] = createSignal('')
  const [model, setModel] = createSignal('')
  const [busy, setBusy] = createSignal(false)
  const [result, setResult] = createSignal<unknown>(null)
  const [error, setError] = createSignal('')

  async function run() {
    const cap = CAPS.find(c => c.id === active())!
    if (!text().trim()) return
    setBusy(true); setError(''); setResult(null)
    try {
      const body: Record<string, unknown> = { input: text(), prompt: text() }
      if (model()) body.model = model()
      const r = await apiPost(cap.endpoint, body)
      setResult(r)
    } catch (e: unknown) { setError(e instanceof Error ? e.message : '请求失败') }
    finally { setBusy(false) }
  }

  return (
    <div class="space-y-5 stagger">
      <div>
        <h1 class="text-xl font-semibold">媒体能力</h1>
        <p class="text-sm text-faint mt-0.5">测试图像、语音、嵌入与搜索等非对话能力</p>
      </div>

      <div class="flex flex-wrap gap-1.5">
        <For each={CAPS}>
          {c => (
            <button
              class={`px-3 py-1.5 rounded-control text-sm border transition-colors ${active() === c.id
                ? 'border-[color:var(--accent)] text-text bg-accent/10'
                : 'border-subtle text-muted hover:text-text'}`}
              onClick={() => { setActive(c.id); setResult(null); setError('') }}
            >
              {c.label}
            </button>
          )}
        </For>
      </div>

      <Card class="p-5 space-y-4">
        <div class="flex items-center gap-2">
          <span class="text-sm font-medium">{CAPS.find(c => c.id === active())!.label}</span>
          <code class="text-[11px] font-mono text-faint">{CAPS.find(c => c.id === active())!.endpoint}</code>
        </div>
        <p class="text-xs text-faint">{CAPS.find(c => c.id === active())!.hint}</p>

        <Field label="模型" hint="留空使用默认">
          <Input value={model()} onInput={setModel} placeholder="可选" class="!w-64" />
        </Field>
        <Field label="输入内容">
          <Input value={text()} onInput={setText} placeholder="输入提示词或文本…" />
        </Field>

        <div class="flex justify-end">
          <Button variant="primary" loading={busy()} disabled={!text().trim()} onClick={run}>执行</Button>
        </div>
      </Card>

      <Show when={error()}>
        <div class="px-3 py-2 rounded-control text-xs bg-danger/10 text-danger">{error()}</div>
      </Show>

      <Show when={result()}>
        <Card class="p-5">
          <h3 class="text-sm font-semibold mb-2">响应</h3>
          <pre class="text-[11px] font-mono text-muted whitespace-pre-wrap break-all bg-bg-elevated p-3 rounded-control border border-subtle max-h-96 overflow-y-auto">
            {JSON.stringify(result(), null, 2)}
          </pre>
        </Card>
      </Show>
    </div>
  )
}

export default Media
