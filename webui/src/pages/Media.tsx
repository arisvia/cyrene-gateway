import { type Component, For, Show, createSignal, onMount } from 'solid-js'
import { A } from '@solidjs/router'
import { api, apiPost } from '@/lib/api'
import { Card, Button, Input, Field, Select, Modal, StatusPulse, ProviderAvatar } from '@/components/ui'

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
}

const CAPS: { id: Cap; label: string; endpoint: string; hint: string; kindKey: string }[] = [
  { id: 'image', label: '图像生成', endpoint: '/v1/images/generations', hint: '输入提示词，调用多模态模型生成高分辨率图像', kindKey: 'image' },
  { id: 'search', label: '联网搜索', endpoint: '/v1/search', hint: '通过 Google Search Grounding 或专业搜索引擎检索网页', kindKey: 'web-search' },
  { id: 'tts', label: '语音合成', endpoint: '/v1/audio/speech', hint: '将输入文本实时渲染为自然拟人语音', kindKey: 'tts' },
  { id: 'stt', label: '语音识别', endpoint: '/v1/audio/transcriptions', hint: '转录音频内容为结构化文本', kindKey: 'stt' },
  { id: 'embeddings', label: '向量嵌入', endpoint: '/v1/embeddings', hint: '文本特征与语义向量化提取', kindKey: 'embedding' },
]

const Media: Component = () => {
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

  // 快捷接入配置弹窗状态
  const [wizardOpen, setWizardOpen] = createSignal(false)
  const [wizardProvider, setWizardProvider] = createSignal<MediaProvider | null>(null)
  const [wizardForm, setWizardForm] = createSignal({
    name: '',
    apiKey: '',
    baseUrl: '',
  })
  const [testingCreds, setTestingCreds] = createSignal(false)
  const [testedCreds, setTestedCreds] = createSignal<{ ok: boolean; msg: string } | null>(null)
  const [savingCreds, setSavingCreds] = createSignal(false)
  const [saveError, setSaveError] = createSignal('')

  function openConfigWizard(p: MediaProvider) {
    setWizardProvider(p)
    setWizardForm({
      name: `${p.name} 账号`,
      apiKey: '',
      baseUrl: '',
    })
    setTestedCreds(null)
    setSaveError('')
    setTestingCreds(false)
    setWizardOpen(true)
  }

  async function handleTestCreds() {
    const p = wizardProvider()
    if (!p || !wizardForm().apiKey.trim()) return
    setTestingCreds(true)
    setTestedCreds(null)
    try {
      const res = await apiPost('/api/providers/test-credentials', {
        provider: p.provider,
        apiKey: wizardForm().apiKey.trim(),
        baseUrl: wizardForm().baseUrl.trim() || undefined,
      }) as { ok?: boolean; error?: string; latency?: string }
      if (res.ok) {
        setTestedCreds({ ok: true, msg: `连接成功 (${res.latency || '正常'})` })
      } else {
        setTestedCreds({ ok: false, msg: res.error || '凭证校验未通过' })
      }
    } catch (e: unknown) {
      setTestedCreds({ ok: false, msg: e instanceof Error ? e.message : '网络或服务异常' })
    } finally {
      setTestingCreds(false)
    }
  }

  async function handleSaveCreds() {
    const p = wizardProvider()
    if (!p || !wizardForm().apiKey.trim()) return
    setSavingCreds(true)
    setSaveError('')
    try {
      await apiPost('/api/providers', {
        provider: p.provider,
        name: wizardForm().name.trim() || `${p.name} 账号`,
        authType: 'api-key',
        data: {
          apiKey: wizardForm().apiKey.trim(),
          baseUrl: wizardForm().baseUrl.trim() || undefined,
        },
      })
      setWizardOpen(false)
      await loadProviders()
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : '保存失败'
      if (msg.includes('already exists') || msg.includes('409') || msg.includes('connection for this provider already exists')) {
        setSaveError('该提供商已存在账号，单提供商当前仅支持配置一个活跃连接；如需更新请至“提供商”页面')
      } else {
        setSaveError(msg)
      }
    } finally {
      setSavingCreds(false)
    }
  }

  async function loadProviders() {
    setLoadingProviders(true)
    try {
      const cap = CAPS.find(c => c.id === active())!
      const res = (await api(`/api/media-providers?kind=${cap.kindKey}`)) as { providers?: MediaProvider[] }
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
    const defaultM = p.models?.[0]?.id || (p.provider === 'antigravity' ? 'gemini-3.1-flash-image' : '')
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
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : '请求失败')
    } finally {
      setBusy(false)
    }
  }

  return (
    <div class="space-y-5 stagger pb-16">
      <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-3 border-b border-subtle/50 pb-3">
        <div>
          <h1 class="text-xl font-semibold">媒体能力与多模态工作台</h1>
          <p class="text-sm text-faint mt-0.5">多模态与衍生能力中心：图像生成、联网搜索、语音合成/识别与文本向量嵌入</p>
        </div>
        <A href="/providers?tab=catalog&category=media">
          <Button size="sm" variant="secondary" class="gap-1.5 shrink-0">
            <span>前往提供商市场接入更多渠道 ↗</span>
          </Button>
        </A>
      </div>
      {/* 顶部能力分类 Tab */}
      <div class="flex flex-wrap gap-1.5 p-1 rounded-card bg-hover/40 border border-subtle/50 w-fit">
        <For each={CAPS}>
          {c => (
            <button
              class={`px-3 py-1.5 rounded-control text-xs font-medium transition cursor-pointer flex items-center gap-1.5 ${
                active() === c.id
                  ? 'bg-card text-foreground shadow-sm border border-subtle'
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
        <span>💡 {CAPS.find(c => c.id === active())?.hint}</span>
        <span class="font-mono text-[11px] px-2 py-0.5 rounded bg-bg-elevated border border-subtle">
          {CAPS.find(c => c.id === active())?.endpoint}
        </span>
      </div>

      {/* 提供商能力卡片网格 */}
      <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
        <For each={providers()}>
          {p => (
            <Card class="p-5 flex flex-col justify-between space-y-4 hover:border-accent/40 transition">
              <div class="space-y-2.5">
                <div class="flex items-center justify-between">
                  <div class="flex items-center gap-2.5">
                    <ProviderAvatar provider={p.provider} name={p.name} size="md" />
                    <div>
                      <h3 class="text-sm font-semibold text-foreground">{p.name}</h3>
                      <p class="text-[11px] font-mono text-faint">{p.provider}</p>
                    </div>
                  </div>
                  <div class="flex items-center gap-1.5">
                    <StatusPulse status={p.hasConnection ? 'active' : 'idle'} size="xs" />
                    <span class="text-[11px] text-faint">
                      {p.hasConnection ? `${p.activeConnections} 个活跃账号` : '未配置账号'}
                    </span>
                  </div>
                </div>

                <div class="space-y-1">
                  <div class="text-[11px] text-faint">支持模型 / 端点:</div>
                  <div class="flex flex-wrap gap-1">
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

              <div class="pt-2 border-t border-subtle flex items-center justify-between">
                <span class="text-[11px] text-faint">
                  {p.hasConnection ? '就绪可调用' : '未配置凭据'}
                </span>
                <div class="flex items-center gap-1.5">
                  <Show when={!p.hasConnection}>
                    <Button
                      size="sm"
                      variant="primary"
                      onClick={() => openConfigWizard(p)}
                      title="快捷配置此提供商凭证"
                    >
                      配置凭证 →
                    </Button>
                  </Show>
                  <Show when={p.hasConnection}>
                    <Button
                      size="sm"
                      variant="secondary"
                      onClick={() => openConfigWizard(p)}
                      title="添加或更换账号凭证"
                    >
                      + 账号
                    </Button>
                    <Button
                      size="sm"
                      variant="primary"
                      onClick={() => openWorkbench(p)}
                    >
                      试用{CAPS.find(c => c.id === active())?.label}
                    </Button>
                  </Show>
                </div>
              </div>
            </Card>
          )}
        </For>
      </div>

      <Show when={providers().length === 0 && !loadingProviders()}>
        <Card class="p-8 text-center space-y-2">
          <p class="text-sm text-faint">暂无支持该能力的提供商定义</p>
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
            <div class="px-3 py-2 rounded-control text-xs bg-danger/10 text-danger border border-danger/20">
              {error()}
            </div>
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
                    <span>✨ AI 检索总结</span>
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

      {/* 原地配置 API 凭据向导 Modal */}
      <Modal
        open={wizardOpen()}
        title={`配置 ${wizardProvider()?.name || ''} 访问凭据`}
        onClose={() => setWizardOpen(false)}
      >
        <Show when={wizardProvider()}>
          {p => (
            <div class="space-y-4">
              <div class="p-3 rounded-xl bg-hover text-xs space-y-1 text-faint border border-subtle">
                <div class="flex items-center justify-between">
                  <span>提供商标识：<strong class="font-mono text-foreground">{p().provider}</strong></span>
                  <span>支持能力：<strong class="text-foreground">{p().kinds.join(', ')}</strong></span>
                </div>
                <div class="text-[11px] text-muted">
                  凭据将加密保存在服务端账号池中，与对话提供商采用统一连接调度架构。
                </div>
              </div>

              <Field label="连接显示名称" hint="便于区分多账号，如：主力 1 号">
                <Input
                  value={wizardForm().name}
                  placeholder={`例如：我的 ${p().name}`}
                  onInput={v => setWizardForm(f => ({ ...f, name: v }))}
                />
              </Field>

              <Field label="API Key / 访问令牌" hint="该服务的官方 API 密钥">
                <div class="flex items-center gap-2">
                  <div class="flex-1">
                    <Input
                      type="password"
                      value={wizardForm().apiKey}
                      placeholder="sk-... / 密钥文本"
                      onInput={v => {
                        setWizardForm(f => ({ ...f, apiKey: v }))
                        setTestedCreds(null)
                      }}
                    />
                  </div>
                  <Button
                    size="md"
                    variant="secondary"
                    loading={testingCreds()}
                    disabled={!wizardForm().apiKey.trim()}
                    onClick={handleTestCreds}
                  >
                    测试连接
                  </Button>
                </div>
              </Field>

              <Show when={testedCreds()}>
                {res => (
                  <div class={`text-xs px-3 py-2 rounded-control flex items-center justify-between ${
                    res().ok ? 'bg-success/10 text-success border border-success/20' : 'bg-danger/10 text-danger border border-danger/20'
                  }`}>
                    <span>{res().ok ? `✓ ${res().msg}` : `✕ ${res().msg}`}</span>
                    <span class="text-[11px] opacity-75">{res().ok ? '凭据有效，允许保存' : '请核对密钥'}</span>
                  </div>
                )}
              </Show>

              <Show when={saveError()}>
                <div class="text-xs px-3 py-2 rounded-control bg-danger/10 text-danger border border-danger/20 flex items-center justify-between">
                  <span>✕ {saveError()}</span>
                </div>
              </Show>

              <Field label="自定义 Base URL (可选)" hint="私有部署或自定义中转代理时填写，留空走官方直连">
                <Input
                  value={wizardForm().baseUrl}
                  placeholder="https://..."
                  onInput={v => setWizardForm(f => ({ ...f, baseUrl: v }))}
                />
              </Field>

              <div class="pt-3 border-t border-subtle flex items-center justify-end gap-2">
                <Button variant="secondary" onClick={() => setWizardOpen(false)}>
                  取消
                </Button>
                <Button
                  variant="primary"
                  loading={savingCreds()}
                  disabled={!wizardForm().apiKey.trim() || !testedCreds()?.ok}
                  onClick={handleSaveCreds}
                >
                  保存并启用
                </Button>
              </div>
            </div>
          )}
        </Show>
      </Modal>
    </div>
  )
}

export default Media
