import { type Component, For, Show, createSignal, onMount } from 'solid-js'
import { A } from '@solidjs/router'
import { api, apiPost } from '@/lib/api'
import { Card, Button, Input, Field, Select, Modal, StatusPulse, ProviderAvatar, Alert, PageHeader, IconBulb, IconPlug, IconSparkles } from '@/components/ui'
import { useToast } from '@/lib/toast'

type Cap = 'image' | 'search' | 'tts' | 'stt' | 'embeddings'

interface MediaModelEntry {
  id: string
  name: string
  kind: string
}

interface MediaProvider {
  provider: string
  name: string
  kinds: string[]
  models?: MediaModelEntry[]
  hasConnection: boolean
  activeConnections: number
  primaryConnectionId?: string
}

const CAPS: { id: Cap; label: string; endpoint: string; hint: string; kindKey: string }[] = [
  { id: 'image', label: '图像生成', endpoint: '/v1/images/generations', hint: '输入提示词，调用多模态模型生成高分辨率图像', kindKey: 'image' },
  { id: 'search', label: '联网搜索', endpoint: '/v1/search', hint: '通过 Google Search Grounding 或专业搜索引擎检索网页', kindKey: 'web-search' },
  { id: 'tts', label: '语音合成', endpoint: '/v1/audio/speech', hint: '将输入文本实时渲染为自然拟人语音', kindKey: 'tts' },
  { id: 'stt', label: '语音识别', endpoint: '/v1/audio/transcriptions', hint: '转录音频内容为结构化文本', kindKey: 'stt' },
  { id: 'embeddings', label: '向量嵌入', endpoint: '/v1/embeddings', hint: '文本特征与语义向量化提取', kindKey: 'embedding' },
]

const Media: Component = () => {
  const toast = useToast()
  const [active, setActive] = createSignal<Cap>('image')
  const [providers, setProviders] = createSignal<MediaProvider[]>([])
  const [loadingProviders, setLoadingProviders] = createSignal(false)

  // 专属试用弹窗状态
  const [workbenchOpen, setWorkbenchOpen] = createSignal(false)
  const [selectedProvider, setSelectedProvider] = createSignal<MediaProvider | null>(null)
  const [model, setModel] = createSignal('')
  const [text, setText] = createSignal('')
  const [busy, setBusy] = createSignal(false)
  const [result, setResult] = createSignal<unknown>(null)
  const [error, setError] = createSignal('')

  async function loadProviders() {
    setLoadingProviders(true)
    try {
      const cap = CAPS.find(c => c.id === active())!
      const res = (await api(`/api/media-providers?kind=${cap.kindKey}&connectedOnly=true`)) as { providers?: MediaProvider[] }
      setProviders(res?.providers || [])
    } catch {
      // ignore
    } finally {
      setLoadingProviders(false)
    }
  }

  onMount(() => {
    loadProviders()
  })

  function handleTabChange(capId: Cap) {
    setActive(capId)
    loadProviders()
  }

  function openWorkbench(p: MediaProvider) {
    setSelectedProvider(p)
    setError('')
    setResult(null)
    const defaultM = p.models?.[0]?.id || ''
    setModel(defaultM)
    if (active() === 'image') {
      setText('A futuristic cybernetic city at twilight with glowing neon reflections on wet streets, 8k render, masterpiece')
    } else if (active() === 'search') {
      setText('2026 AI coding agents and LLM developments')
    } else {
      setText('Hello, this is a media test from Cyrene Gateway.')
    }
    setWorkbenchOpen(true)
  }

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

  // 提取搜索结果列表
  const searchResults = () => {
    const res = result() as { summary?: string; count?: number; results?: Array<{ title?: string; url?: string; snippet?: string; position?: number }> } | null
    return res || null
  }

  async function runWorkbench() {
    const p = selectedProvider()
    const cap = CAPS.find(c => c.id === active())!
    if (!text().trim() || !p) return
    setBusy(true)
    setError('')
    setResult(null)
    try {
      const body: Record<string, unknown> = {
        provider: p.provider,
        input: text(),
        prompt: text(),
        query: text(),
      }
      if (model()) body.model = model()
      const r = await apiPost(cap.endpoint, body)
      setResult(r)
      toast.success(`${cap.label} 调用完成`)
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : '请求失败'
      setError(msg)
      toast.error(msg)
    } finally {
      setBusy(false)
    }
  }

  return (
    <div class="space-y-5 stagger pb-16">
      <PageHeader
        title="媒体能力与多模态工作台"
        subtitle="多模态与衍生能力中心：图像生成、联网搜索、语音合成/识别与文本向量嵌入"
        actions={
          <A href="/providers?tab=catalog&category=media">
            <Button size="sm" variant="secondary" class="gap-1.5 shrink-0">
              <span>前往提供商市场接入更多渠道 ↗</span>
            </Button>
          </A>
        }
      />
      {/* 顶部能力分类 Tab */}
      <div class="flex flex-wrap items-center gap-1.5 p-1 rounded-card bg-black/[0.05] dark:bg-white/[0.08] border border-black/[0.03] dark:border-white/[0.04] w-fit">
        <For each={CAPS}>
          {c => (
            <button
              class={`px-3 py-1.5 rounded-control text-xs font-medium transition cursor-pointer flex items-center gap-1.5 ${
                active() === c.id
                  ? 'bg-bg-elevated text-foreground shadow-xs font-semibold'
                  : 'text-faint hover:text-foreground'
              }`}
              onClick={() => handleTabChange(c.id)}
            >
              {c.label}
            </button>
          )}
        </For>
      </div>

      <div class="text-xs text-faint flex items-center gap-2">
        <span class="flex items-center gap-1.5"><IconBulb size={14} class="text-accent" /> {CAPS.find(c => c.id === active())?.hint}</span>
        <span class="font-mono text-[11px] px-2 py-0.5 rounded bg-bg-elevated border border-subtle">
          {CAPS.find(c => c.id === active())?.endpoint}
        </span>
      </div>

      {/* 提供商能力卡片网格 */}
      <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
        <For each={providers()}>
          {p => (
            <Card class="p-4 flex flex-col justify-between space-y-3 hover:border-accent/40 transition">
              <div class="space-y-2.5">
                <div class="flex items-center justify-between">
                  <div class="flex items-center gap-2.5 min-w-0">
                    <ProviderAvatar provider={p.provider} name={p.name} size="md" />
                    <div class="min-w-0">
                      <h3 class="text-sm font-semibold text-foreground truncate">{p.name}</h3>
                      <p class="text-[11px] font-mono text-faint truncate">{p.provider}</p>
                    </div>
                  </div>
                  <div class="flex items-center gap-1.5 shrink-0">
                    <StatusPulse status={p.hasConnection ? 'active' : 'idle'} size="xs" />
                    <span class="text-[11px] text-faint">
                      {p.hasConnection ? `${p.activeConnections} 个账号` : '未接入'}
                    </span>
                  </div>
                </div>

                <div class="space-y-1">
                  <div class="text-[11px] text-faint">支持模型 / 端点:</div>
                  <div class="flex flex-wrap gap-1 max-h-16 overflow-y-auto">
                    <Show
                      when={p.models && p.models.length > 0}
                      fallback={
                        <span class="text-[11px] font-mono text-muted bg-bg-elevated px-2 py-0.5 rounded border border-subtle">
                          默认端点驱动
                        </span>
                      }
                    >
                      <For each={p.models}>
                        {m => (
                          <span class="text-[11px] font-mono text-foreground bg-bg-elevated px-2 py-0.5 rounded border border-subtle">
                            {m.name || m.id}
                          </span>
                        )}
                      </For>
                    </Show>
                  </div>
                </div>
              </div>

              <div class="pt-2.5 border-t border-subtle flex items-center justify-between gap-2">
                <span class="text-[11px] text-faint">
                  就绪可调用
                </span>
                <div class="flex items-center gap-2">
                  <Show when={p.primaryConnectionId}>
                    <A href={`/providers/${p.primaryConnectionId}`}>
                      <Button size="sm" variant="secondary" title="管理账号与参数">
                        管理 →
                      </Button>
                    </A>
                  </Show>
                  <Button
                    size="sm"
                    variant="primary"
                    onClick={() => openWorkbench(p)}
                  >
                    试用{CAPS.find(c => c.id === active())?.label}
                  </Button>
                </div>
              </div>
            </Card>
          )}
        </For>
      </div>

      <Show when={providers().length === 0 && !loadingProviders()}>
        <Card class="p-12 text-center space-y-4 border-dashed border-subtle">
          <div class="flex justify-center text-accent/80">
            <IconPlug size={36} />
          </div>
          <div class="space-y-1">
            <h3 class="text-sm font-semibold text-foreground">
              尚未接入支持「{CAPS.find(c => c.id === active())?.label}」的提供商
            </h3>
            <p class="text-xs text-faint max-w-md mx-auto leading-relaxed">
              统一凭证已归入市场管理。前往「提供商市场」绑定 OpenAI、Google Gemini、Antigravity 或专业媒体供应商（如 Stability AI、ElevenLabs、Tavily）的账号凭证后即可在此调试。
            </p>
          </div>
          <div class="pt-2">
            <A href="/providers?tab=catalog&category=media">
              <Button variant="primary" size="sm">
                前往提供商市场选购接入 →
              </Button>
            </A>
          </div>
        </Card>
      </Show>

      {/* 专属试用交互工作台 (Workbench Modal) */}
      <Modal
        open={workbenchOpen()}
        title={`${selectedProvider()?.name || ''} · ${CAPS.find(c => c.id === active())?.label || ''}试用台`}
        onClose={() => setWorkbenchOpen(false)}
      >
        <div class="space-y-4 max-h-[80vh] overflow-y-auto pr-1">
          <Show when={selectedProvider()?.models && selectedProvider()!.models!.length > 0}>
            <Field label="选用模型">
              <Select
                value={model()}
                onChange={setModel}
                options={(selectedProvider()?.models || []).map(m => ({
                  value: m.id,
                  label: m.name || m.id,
                }))}
              />
            </Field>
          </Show>

          <Field label={active() === 'image' ? '生成提示词 (Prompt)' : active() === 'search' ? '搜索关键词 (Query)' : '输入内容 (Input)'}>
            <Input
              value={text()}
              onInput={setText}
              placeholder={active() === 'image' ? '输入英文提示词以获得最佳效果...' : '输入关键词或文本...'}
            />
          </Field>

          <div class="flex justify-end gap-2">
            <Button size="sm" variant="secondary" onClick={() => setWorkbenchOpen(false)}>
              关闭
            </Button>
            <Button size="sm" variant="primary" loading={busy()} disabled={!text().trim()} onClick={runWorkbench}>
              {active() === 'image' ? '开始生成图片' : active() === 'search' ? '执行全网检索' : '执行调用'}
            </Button>
          </div>

          <Show when={error()}>
            <Alert variant="danger">{error()}</Alert>
          </Show>

          {/* 生图结果预览画廊 */}
          <Show when={imageUrls().length > 0}>
            <div class="space-y-2 pt-2 border-t border-subtle">
              <div class="flex items-center justify-between">
                <span class="text-xs font-semibold text-foreground">生成结果预览</span>
                <span class="text-[11px] text-faint">共 {imageUrls().length} 张</span>
              </div>
              <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
                <For each={imageUrls()}>
                  {(url, idx) => (
                    <div class="relative group rounded-card overflow-hidden border border-subtle bg-bg-elevated aspect-square flex items-center justify-center">
                      <img src={url} alt={`Generated result ${idx() + 1}`} class="w-full h-full object-cover transition duration-300 group-hover:scale-105" />
                      <div class="absolute inset-0 bg-black/40 opacity-0 group-hover:opacity-100 transition flex items-center justify-center gap-2">
                        <a href={url} target="_blank" rel="noreferrer" class="px-3 py-1.5 rounded-control text-xs bg-bg/90 text-foreground font-medium shadow hover:bg-bg transition">
                          查看原图
                        </a>
                      </div>
                    </div>
                  )}
                </For>
              </div>
            </div>
          </Show>

          {/* 联网搜索结果卡片流 */}
          <Show when={searchResults() && searchResults()!.results && searchResults()!.results!.length > 0}>
            <div class="space-y-2.5 pt-2 border-t border-subtle">
              <Show when={searchResults()?.summary}>
                <div class="p-3 rounded-card bg-accent/5 border border-accent/20 text-xs text-foreground leading-relaxed">
                  <div class="font-semibold text-accent mb-1 flex items-center gap-1.5">
                    <IconSparkles size={14} />
                    <span>AI 检索总结</span>
                  </div>
                  {searchResults()?.summary}
                </div>
              </Show>

              <div class="text-xs font-semibold text-foreground">检索引文 ({searchResults()?.count || searchResults()?.results?.length} 条)</div>
              <div class="space-y-2">
                <For each={searchResults()?.results}>
                  {item => (
                    <div class="p-3 rounded-card bg-bg-elevated border border-subtle text-xs space-y-1 hover:border-accent/30 transition">
                      <div class="flex items-center justify-between">
                        <a href={item.url} target="_blank" rel="noreferrer" class="font-medium text-accent hover:underline flex items-center gap-1 truncate max-w-[85%]">
                          <span>{item.position}. {item.title || item.url}</span>
                        </a>
                      </div>
                      <p class="text-[11px] text-faint truncate font-mono">{item.url}</p>
                    </div>
                  )}
                </For>
              </div>
            </div>
          </Show>

          {/* 原始响应 JSON 调试折叠面板 */}
          <Show when={result()}>
            <div class="pt-2 border-t border-subtle space-y-1.5">
              <div class="text-[11px] font-semibold text-faint uppercase">原始响应数据 (JSON)</div>
              <pre class="text-[10px] font-mono text-muted whitespace-pre-wrap break-all bg-bg-elevated p-3 rounded-control border border-subtle max-h-60 overflow-y-auto">
                {JSON.stringify(result(), null, 2)}
              </pre>
            </div>
          </Show>
        </div>
      </Modal>
    </div>
  )
}

export default Media
