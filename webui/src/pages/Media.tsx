import { type Component, For, Show, createSignal, onMount } from 'solid-js'
import { api, apiPost } from '@/lib/api'
import { Card, Button, Input, Field, Select } from '@/components/ui'

type Cap = 'image' | 'tts' | 'stt' | 'embeddings' | 'search'

interface ModelOption {
  id: string
  displayName?: string
  ownedBy?: string
}

const CAPS: { id: Cap; label: string; endpoint: string; hint: string }[] = [
  { id: 'image', label: '图像生成', endpoint: '/v1/images/generations', hint: '根据提示词生成高质量图片' },
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
  const [imageModels, setImageModels] = createSignal<ModelOption[]>([])
  const [loadingModels, setLoadingModels] = createSignal(false)

  // 提取响应中的图片 URL 或 base64
  const imageUrls = () => {
    const res = result() as Record<string, unknown> | null
    if (!res) return []
    const urls: string[] = []
    if (Array.isArray(res.data)) {
      for (const item of res.data) {
        if (typeof item === 'object' && item !== null) {
          const d = item as Record<string, unknown>
          if (typeof d.url === 'string') urls.push(d.url)
          else if (typeof d.b64_json === 'string') urls.push(`data:image/png;base64,${d.b64_json}`)
        }
      }
    }
    return urls
  }

  async function loadModels() {
    setLoadingModels(true)
    try {
      // 查询标记为 image-generation 的专有模型
      const res = (await api('/v1/models?type=image')) as { data?: ModelOption[] }
      if (res?.data && res.data.length > 0) {
        setImageModels(res.data)
        if (!model()) setModel(res.data[0].id)
      } else {
        // 若当前未配置生图连接，提供标准已知候选
        setImageModels([
          { id: 'gemini-3.1-flash-image', displayName: 'Gemini 3.1 Flash Image (Antigravity)' },
          { id: 'dall-e-3', displayName: 'DALL-E 3 (OpenAI)' },
          { id: 'flux-pro', displayName: 'FLUX Pro (Black Forest Labs)' },
        ])
        if (!model()) setModel('gemini-3.1-flash-image')
      }
    } catch {
      // 忽略拉取错误
    } finally {
      setLoadingModels(false)
    }
  }

  onMount(() => {
    loadModels()
  })

  async function run() {
    const cap = CAPS.find(c => c.id === active())!
    if (!text().trim()) return
    setBusy(true)
    setError('')
    setResult(null)
    try {
      const body: Record<string, unknown> = { input: text(), prompt: text() }
      if (model()) body.model = model()
      const r = await apiPost(cap.endpoint, body)
      setResult(r)
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : '请求失败')
    } finally {
      setBusy(false)
    }
  }

  return (
    <div class="space-y-5 stagger">
      <div class="sticky top-16 z-20 bg-bg/90 backdrop-blur-md pt-1 pb-3 flex items-center justify-between border-b border-subtle/50">
        <div>
          <h1 class="text-xl font-semibold">媒体能力</h1>
          <p class="text-sm text-faint mt-0.5">测试图像、语音、嵌入与搜索等非对话能力</p>
        </div>
      </div>

      <div class="flex flex-wrap gap-1.5">
        <For each={CAPS}>
          {c => (
            <button
              class={`px-3 py-1.5 rounded-control text-sm border transition-colors ${
                active() === c.id
                  ? 'border-[color:var(--accent)] text-text bg-accent/10'
                  : 'border-subtle text-muted hover:text-text'
              }`}
              onClick={() => {
                setActive(c.id)
                setResult(null)
                setError('')
              }}
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

        <Show when={active() === 'image'}>
          <Field label="图像生成模型" hint={loadingModels() ? '加载可用模型中...' : '已关联当前启用的生图连接'}>
            <Select
              value={model()}
              onChange={setModel}
              options={imageModels().map(m => ({
                value: m.id,
                label: m.displayName || m.id,
              }))}
              class="max-w-md"
            />
          </Field>
        </Show>

        <Show when={active() !== 'image'}>
          <Field label="模型" hint="留空使用提供商默认模型">
            <Input value={model()} onInput={setModel} placeholder="例如：tts-1、whisper-1" class="!w-64" />
          </Field>
        </Show>

        <Field label="提示词 / 输入内容">
          <Input
            value={text()}
            onInput={setText}
            placeholder={active() === 'image' ? '例如：A hyper-realistic sunset over cyberpunk neon city, 8k resolution' : '输入提示词或文本…'}
          />
        </Field>

        <div class="flex justify-end">
          <Button variant="primary" loading={busy()} disabled={!text().trim()} onClick={run}>
            {active() === 'image' ? '开始生成图片' : '执行'}
          </Button>
        </div>
      </Card>

      <Show when={error()}>
        <div class="px-3 py-2 rounded-control text-xs bg-danger/10 text-danger">{error()}</div>
      </Show>

      {/* 图片画廊预览 */}
      <Show when={imageUrls().length > 0}>
        <Card class="p-5 space-y-3">
          <div class="flex items-center justify-between">
            <h3 class="text-sm font-semibold">生成结果预览</h3>
            <span class="text-xs text-faint">共 {imageUrls().length} 张</span>
          </div>
          <div class="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-3 gap-4 pt-1">
            <For each={imageUrls()}>
              {url => (
                <div class="group relative rounded-card overflow-hidden border border-subtle bg-bg/80 aspect-square flex items-center justify-center">
                  <img src={url} alt="Generated" class="w-full h-full object-cover transition-transform duration-300 group-hover:scale-105" />
                  <div class="absolute inset-0 bg-black/40 opacity-0 group-hover:opacity-100 transition-opacity flex items-center justify-center gap-2">
                    <a
                      href={url}
                      target="_blank"
                      rel="noopener noreferrer"
                      class="px-3 py-1.5 text-xs bg-white/20 backdrop-blur hover:bg-white/30 text-white rounded font-medium"
                    >
                      查看原图
                    </a>
                  </div>
                </div>
              )}
            </For>
          </div>
        </Card>
      </Show>

      {/* JSON 调试响应面板 */}
      <Show when={result()}>
        <Card class="p-5">
          <h3 class="text-sm font-semibold mb-2">原始响应 (JSON)</h3>
          <pre class="text-[11px] font-mono text-muted whitespace-pre-wrap break-all bg-bg-elevated p-3 rounded-control border border-subtle max-h-96 overflow-y-auto">
            {JSON.stringify(result(), null, 2)}
          </pre>
        </Card>
      </Show>
    </div>
  )
}
export default Media
