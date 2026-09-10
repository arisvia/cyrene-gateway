import { type Component, For, Show, createSignal, createEffect, onMount, onCleanup } from 'solid-js'
import { api } from '@/lib/api'
import {
  Card,
  Button,
  Select,
  Toggle,
  Modal,
  PageHeader,
  SegmentedControl,
  ProviderAvatar,
  IconChat,
  IconZap,
  IconSliders,
  IconCode,
  IconClipboard,
  IconCheck,
  IconTrash,
  IconSquare,
  IconPlay,
  IconAlertCircle,
  IconSparkles,
} from '@/components/ui'
import { useToast } from '@/lib/toast'
import { confirm } from '@/lib/confirm'

interface ModelEntry {
  id: string
  object: string
  owned_by: string
  display_name?: string
  context_length?: number
  max_output_tokens?: number
}

interface Metrics {
  ttftMs?: number
  totalMs?: number
  tokens?: number
  speed?: number
}

interface AssistantResponse {
  content: string
  busy: boolean
  error?: string
  servedModel?: string
  metrics?: Metrics
  rawRequest?: unknown
  rawResponse?: unknown
}

interface Turn {
  id: string
  user: string
  a: AssistantResponse
  b?: AssistantResponse
}

const SYSTEM_PRESETS = [
  { label: '默认助手', text: '你是一个由 Cyrene Gateway 驱动的高性能、智能且诚恳的 AI 助手。' },
  { label: '资深工程师', text: '你是一位资深全栈架构师与系统工程师。直接输出高质量、生产级、类型安全的代码与方案，分析利弊，拒绝多余客套。' },
  { label: '创意写作', text: '你是一位极具洞察力与表现力的创意作家。擅长以精妙的比喻和深刻的见解展开阐述。' },
  { label: '中英翻译', text: '你是一个精通中英文的同声传译专家。请直接将输入翻译为自然地道、符合母语习惯的对应语言。' },
]

const QUICK_PROMPTS = [
  '你好！请做一段简短的自我介绍。',
  '用 Go 语言编写一个线程安全且支持 TTL 过期的并发缓存。',
  '简述分布式系统中的 CAP 定理，并在现代架构下给出权衡示例。',
  '分析一下并列计算与并发执行（Concurrency vs Parallelism）的核心差异。',
]

const Playground: Component = () => {
  const toast = useToast()

  // 模式：单模型 (single) / 双模型对比 (compare)
  const [mode, setMode] = createSignal<'single' | 'compare'>('single')
  const [models, setModels] = createSignal<ModelEntry[]>([])
  const [loadingModels, setLoadingModels] = createSignal(false)

  // 模型选择
  const [modelA, setModelA] = createSignal('')
  const [modelB, setModelB] = createSignal('')

  // 参数配置
  const [systemPrompt, setSystemPrompt] = createSignal('')
  const [temperature, setTemperature] = createSignal(0.7)
  const [topP, setTopP] = createSignal(0.95)
  const [maxTokens, setMaxTokens] = createSignal(2048)
  const [stream, setStream] = createSignal(true)

  // 参数抽屉显示状态
  const [showParams, setShowParams] = createSignal(true)

  // 对话历史
  const [turns, setTurns] = createSignal<Turn[]>([])
  const [inputPrompt, setInputPrompt] = createSignal('')

  // 控制器以便中断流式传输
  let abortControllerA: AbortController | null = null
  let abortControllerB: AbortController | null = null

  // 弹窗状态
  const [codeModalOpen, setCodeModalOpen] = createSignal(false)
  const [codeLang, setCodeLang] = createSignal<'curl' | 'python' | 'node'>('curl')
  const [rawJsonModal, setRawJsonModal] = createSignal<{ title: string; request: unknown; response: unknown } | null>(null)
  const [copiedCode, setCopiedCode] = createSignal(false)

  // 加载本地持久化配置
  onMount(async () => {
    try {
      if (typeof localStorage !== 'undefined' && localStorage) {
        const saved = localStorage.getItem('cyrene_playground_config')
        if (saved) {
          const parsed = JSON.parse(saved)
          if (parsed.mode) setMode(parsed.mode)
          if (parsed.modelA) setModelA(parsed.modelA)
          if (parsed.modelB) setModelB(parsed.modelB)
          if (parsed.systemPrompt !== undefined) setSystemPrompt(parsed.systemPrompt)
          if (typeof parsed.temperature === 'number') setTemperature(parsed.temperature)
          if (typeof parsed.topP === 'number') setTopP(parsed.topP)
          if (typeof parsed.maxTokens === 'number') setMaxTokens(parsed.maxTokens)
          if (typeof parsed.stream === 'boolean') setStream(parsed.stream)
        }
      }
    } catch {
      // ignore
    }

    await loadModels()
  })

  // 持久化设置
  createEffect(() => {
    const config = {
      mode: mode(),
      modelA: modelA(),
      modelB: modelB(),
      systemPrompt: systemPrompt(),
      temperature: temperature(),
      topP: topP(),
      maxTokens: maxTokens(),
      stream: stream(),
    }
    try {
      if (typeof localStorage !== 'undefined' && localStorage) {
        localStorage.setItem('cyrene_playground_config', JSON.stringify(config))
      }
    } catch {
      // ignore
    }
  })

  async function loadModels() {
    setLoadingModels(true)
    try {
      const res = (await api('/v1/models')) as { data?: ModelEntry[] }
      const list = res?.data || []
      setModels(list)
      if (list.length > 0) {
        if (!modelA() || !list.some(m => m.id === modelA())) {
          setModelA(list[0].id)
        }
        if (!modelB() || !list.some(m => m.id === modelB())) {
          setModelB(list[1]?.id || list[0].id)
        }
      }
    } catch (e: unknown) {
      toast.error(`加载模型列表失败: ${e instanceof Error ? e.message : String(e)}`)
    } finally {
      setLoadingModels(false)
    }
  }

  const modelOptions = () =>
    models().map(m => {
      const provider = m.owned_by || m.id.split('/')[0]
      const name = m.display_name ? `${m.display_name} (${m.id})` : m.id
      return {
        value: m.id,
        label: `[${provider}] ${name}`,
      }
    })

  const isBusy = () =>
    turns().some(t => t.a.busy || (t.b && t.b.busy))

  function stopAll() {
    if (abortControllerA) {
      abortControllerA.abort()
      abortControllerA = null
    }
    if (abortControllerB) {
      abortControllerB.abort()
      abortControllerB = null
    }
    setTurns(prev =>
      prev.map(t => ({
        ...t,
        a: { ...t.a, busy: false },
        b: t.b ? { ...t.b, busy: false } : undefined,
      }))
    )
  }

  onCleanup(() => {
    stopAll()
  })

  async function handleClear() {
    if (turns().length === 0) return
    if (!await confirm('确定要清空当前的演练场对话历史吗？')) return
    stopAll()
    setTurns([])
    toast.info('对话历史已清空')
  }

  // 执行单模型请求（支持 Stream 与非 Stream）
  async function executeRequest(
    targetModel: string,
    historyMessages: { role: string; content: string }[],
    onUpdate: (chunk: string, meta?: { servedModel?: string; firstToken?: boolean }) => void,
    signal: AbortSignal
  ): Promise<{
    fullContent: string
    servedModel: string
    metrics: Metrics
    rawRequest: unknown
    rawResponse: unknown
  }> {
    const startTime = performance.now()
    let firstTokenTime: number | null = null

    const reqMessages = systemPrompt().trim()
      ? [{ role: 'system', content: systemPrompt().trim() }, ...historyMessages]
      : historyMessages

    const reqBody: Record<string, unknown> = {
      model: targetModel,
      messages: reqMessages,
      temperature: Number(temperature()),
      top_p: Number(topP()),
      max_tokens: Number(maxTokens()),
      stream: stream(),
    }

    const res = await fetch('/v1/chat/completions', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(reqBody),
      signal,
    })

    const servedHeader = res.headers.get('x-cyrene-served-model') || ''

    if (!res.ok) {
      const errData = await res.json().catch(() => ({}))
      const msg = errData?.error?.message || errData?.error || `HTTP ${res.status} ${res.statusText}`
      throw new Error(msg)
    }

    if (!stream() || !res.body) {
      const data = await res.json()
      const totalMs = Math.round(performance.now() - startTime)
      const content = data?.choices?.[0]?.message?.content ?? ''
      const completionTokens = data?.usage?.completion_tokens || Math.max(1, Math.round(content.length / 3.5))
      const speed = totalMs > 0 ? Number(((completionTokens / totalMs) * 1000).toFixed(1)) : 0
      onUpdate(content, { servedModel: servedHeader || data?.model })
      return {
        fullContent: content,
        servedModel: servedHeader || data?.model || targetModel,
        metrics: {
          ttftMs: totalMs,
          totalMs,
          tokens: completionTokens,
          speed,
        },
        rawRequest: reqBody,
        rawResponse: data,
      }
    }

    // Stream 模式处理
    const reader = res.body.getReader()
    const decoder = new TextDecoder('utf-8')
    let accumulated = ''
    let buffer = ''
    let servedModel = servedHeader
    const rawChunks: string[] = []

    try {
      while (true) {
        const { done, value } = await reader.read()
        if (done) break
        const text = decoder.decode(value, { stream: true })
        buffer += text

        const lines = buffer.split('\n')
        buffer = lines.pop() || ''

        for (const line of lines) {
          const trimmed = line.trim()
          if (!trimmed || !trimmed.startsWith('data:')) continue
          const dataStr = trimmed.slice(5).trim()
          if (dataStr === '[DONE]') continue

          try {
            const parsed = JSON.parse(dataStr)
            rawChunks.push(parsed)
            if (parsed.model && !servedModel) servedModel = parsed.model
            const delta = parsed?.choices?.[0]?.delta?.content
            if (delta) {
              if (firstTokenTime === null) {
                firstTokenTime = performance.now()
                onUpdate(delta, { servedModel, firstToken: true })
              } else {
                onUpdate(delta, { servedModel })
              }
              accumulated += delta
            }
          } catch {
            // ignore non-json chunk
          }
        }
      }
    } catch (e: unknown) {
      if (signal.aborted) {
        // 用户主动停止，保留已有输出
      } else {
        throw e
      }
    } finally {
      reader.releaseLock()
    }

    const totalMs = Math.round(performance.now() - startTime)
    const ttftMs = firstTokenTime !== null ? Math.round(firstTokenTime - startTime) : totalMs
    const estimatedTokens = Math.max(1, Math.round(accumulated.length / 3.5))
    const genDuration = totalMs - (ttftMs || 0)
    const speed = genDuration > 0 ? Number(((estimatedTokens / genDuration) * 1000).toFixed(1)) : 0

    return {
      fullContent: accumulated,
      servedModel: servedModel || targetModel,
      metrics: {
        ttftMs,
        totalMs,
        tokens: estimatedTokens,
        speed,
      },
      rawRequest: reqBody,
      rawResponse: rawChunks.length > 0 ? rawChunks : { content: accumulated },
    }
  }

  async function handleSend(customPrompt?: string) {
    const text = (customPrompt || inputPrompt()).trim()
    if (!text || isBusy()) return

    if (!modelA()) {
      toast.error('请选择有效的模型')
      return
    }

    setInputPrompt('')
    const isCompare = mode() === 'compare' && Boolean(modelB())
    const turnId = `turn-${Date.now()}`

    // 组装历史消息序列
    const historyA: { role: string; content: string }[] = []
    for (const t of turns()) {
      historyA.push({ role: 'user', content: t.user })
      if (t.a.content) historyA.push({ role: 'assistant', content: t.a.content })
    }
    historyA.push({ role: 'user', content: text })

    const newTurn: Turn = {
      id: turnId,
      user: text,
      a: { content: '', busy: true },
      b: isCompare ? { content: '', busy: true } : undefined,
    }

    setTurns(prev => [...prev, newTurn])

    // 启动 Model A
    abortControllerA = new AbortController()
    const promiseA = executeRequest(
      modelA(),
      historyA,
      (chunk, meta) => {
        setTurns(prev =>
          prev.map(t => {
            if (t.id !== turnId) return t
            return {
              ...t,
              a: {
                ...t.a,
                content: t.a.content + chunk,
                servedModel: meta?.servedModel || t.a.servedModel,
              },
            }
          })
        )
      },
      abortControllerA.signal
    )
      .then(res => {
        setTurns(prev =>
          prev.map(t =>
            t.id === turnId
              ? {
                  ...t,
                  a: {
                    content: res.fullContent,
                    busy: false,
                    servedModel: res.servedModel,
                    metrics: res.metrics,
                    rawRequest: res.rawRequest,
                    rawResponse: res.rawResponse,
                  },
                }
              : t
          )
        )
      })
      .catch(err => {
        const msg = err instanceof Error ? err.message : String(err)
        setTurns(prev =>
          prev.map(t =>
            t.id === turnId
              ? { ...t, a: { ...t.a, busy: false, error: msg } }
              : t
          )
        )
      })

    // 启动 Model B（如果在双模型对比模式）
    let promiseB = Promise.resolve()
    if (isCompare) {
      abortControllerB = new AbortController()
      const historyB: { role: string; content: string }[] = []
      for (const t of turns()) {
        historyB.push({ role: 'user', content: t.user })
        if (t.b?.content) historyB.push({ role: 'assistant', content: t.b.content })
      }
      historyB.push({ role: 'user', content: text })

      promiseB = executeRequest(
        modelB(),
        historyB,
        (chunk, meta) => {
          setTurns(prev =>
            prev.map(t => {
              if (t.id !== turnId || !t.b) return t
              return {
                ...t,
                b: {
                  ...t.b,
                  content: t.b.content + chunk,
                  servedModel: meta?.servedModel || t.b.servedModel,
                },
              }
            })
          )
        },
        abortControllerB.signal
      )
        .then(res => {
          setTurns(prev =>
            prev.map(t =>
              t.id === turnId && t.b
                ? {
                    ...t,
                    b: {
                      content: res.fullContent,
                      busy: false,
                      servedModel: res.servedModel,
                      metrics: res.metrics,
                      rawRequest: res.rawRequest,
                      rawResponse: res.rawResponse,
                    },
                  }
                : t
            )
          )
        })
        .catch(err => {
          const msg = err instanceof Error ? err.message : String(err)
          setTurns(prev =>
            prev.map(t =>
              t.id === turnId && t.b
                ? { ...t, b: { ...t.b, busy: false, error: msg } }
                : t
            )
          )
        })
    }

    await Promise.all([promiseA, promiseB])
  }

  function copyText(text: string, label = '内容') {
    navigator.clipboard?.writeText(text)
    toast.success(`已复制 ${label}`)
  }

  // 生成代码片段
  function generateCode(lang: 'curl' | 'python' | 'node'): string {
    const origin = window.location.origin
    const model = modelA() || 'antigravity/gemini-2.5-flash'
    const messages = systemPrompt().trim()
      ? [{ role: 'system', content: systemPrompt().trim() }, { role: 'user', content: turns().slice(-1)[0]?.user || '你好' }]
      : [{ role: 'user', content: turns().slice(-1)[0]?.user || '你好' }]

    if (lang === 'curl') {
      const body = {
        model,
        messages,
        temperature: temperature(),
        stream: stream(),
      }
      return `curl -X POST "${origin}/v1/chat/completions" \\
  -H "Content-Type: application/json" \\
  -d '${JSON.stringify(body, null, 2)}'`
    }

    if (lang === 'python') {
      return `from openai import OpenAI

client = OpenAI(
    base_url="${origin}/v1",
    api_key="cyrene-local"  # 网关本地调用或填写在网关配置的 API Key
)

response = client.chat.completions.create(
    model="${model}",
    messages=${JSON.stringify(messages, null, 4)},
    temperature=${temperature()},
    stream=${stream() ? 'True' : 'False'}
)

${stream() ? 'for chunk in response:\n    if chunk.choices[0].delta.content:\n        print(chunk.choices[0].delta.content, end="", flush=True)' : 'print(response.choices[0].message.content)'}
`
    }

    return `import OpenAI from "openai";

const client = new OpenAI({
  baseURL: "${origin}/v1",
  apiKey: "cyrene-local",
});

async function main() {
  const response = await client.chat.completions.create({
    model: "${model}",
    messages: ${JSON.stringify(messages, null, 4)},
    temperature: ${temperature()},
    stream: ${stream() ? 'true' : 'false'},
  });

  ${stream() ? 'for await (const chunk of response) {\n    process.stdout.write(chunk.choices[0]?.delta?.content || "");\n  }' : 'console.log(response.choices[0]?.message?.content);'}
}

main();
`
  }

  return (
    <div class="space-y-4 max-w-7xl mx-auto pb-10">
      <PageHeader
        title="平台演练场 (Playground)"
        subtitle="跨供应商模型测试、双模型竞技 (Side-by-Side) 横向比对与参数调优"
        badge={
          <span class="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs font-medium bg-accent/10 text-accent">
            <IconSparkles size={13} />
            实时竞技
          </span>
        }
        actions={
          <div class="flex items-center gap-2 flex-wrap">
            <SegmentedControl
              value={mode()}
              onChange={m => setMode(m as 'single' | 'compare')}
              options={[
                { value: 'single', label: '单模型测试' },
                { value: 'compare', label: '双模型对比 (Side-by-Side)' },
              ]}
              size="sm"
            />
            <Button
              variant="secondary"
              size="sm"
              class="gap-1.5"
              onClick={() => setCodeModalOpen(true)}
              title="一键导出为 cURL / Python / Node.js 代码"
            >
              <IconCode size={14} />
              导出代码
            </Button>
            <Button
              variant={showParams() ? 'secondary' : 'ghost'}
              size="sm"
              class="gap-1.5"
              onClick={() => setShowParams(!showParams())}
              title="切换显示右侧高级参数配置"
            >
              <IconSliders size={14} />
              参数面板
            </Button>
            <Button
              variant="ghost"
              size="sm"
              class="gap-1.5 text-danger hover:bg-danger/10"
              disabled={turns().length === 0}
              onClick={handleClear}
              title="清空当前所有会话历史"
            >
              <IconTrash size={14} />
              清空
            </Button>
          </div>
        }
      />

      {/* 主工作区布局 */}
      <div class="grid grid-cols-1 lg:grid-cols-12 gap-4 items-start">
        {/* 对话互动主体区 */}
        <div class={`${showParams() ? 'lg:col-span-8 xl:col-span-9' : 'lg:col-span-12'} space-y-4 transition-all duration-300`}>
          {/* 顶部模型选择栏 */}
          <Card class="p-3 bg-card/60 backdrop-blur-md">
            <Show
              when={mode() === 'compare'}
              fallback={
                <div class="flex items-center gap-3">
                  <span class="text-xs font-medium text-foreground shrink-0 flex items-center gap-1.5">
                    <ProviderAvatar provider={modelA().split('/')[0]} size="sm" />
                    选择评测模型:
                  </span>
                  <div class="flex-1 min-w-[200px]">
                    <Select
                      value={modelA()}
                      options={modelOptions()}
                      onChange={setModelA}
                    />
                  </div>
                  <Show when={loadingModels()}>
                    <span class="text-xs text-faint">同步中...</span>
                  </Show>
                </div>
              }
            >
              <div class="grid grid-cols-1 md:grid-cols-2 gap-3">
                <div class="flex items-center gap-2">
                  <span class="px-1.5 py-0.5 rounded text-[11px] font-bold bg-blue-500/15 text-blue-400 border border-blue-500/30 shrink-0">
                    模型 A
                  </span>
                  <div class="flex-1 min-w-0">
                    <Select
                      value={modelA()}
                      options={modelOptions()}
                      onChange={setModelA}
                    />
                  </div>
                </div>
                <div class="flex items-center gap-2">
                  <span class="px-1.5 py-0.5 rounded text-[11px] font-bold bg-purple-500/15 text-purple-400 border border-purple-500/30 shrink-0">
                    模型 B
                  </span>
                  <div class="flex-1 min-w-0">
                    <Select
                      value={modelB()}
                      options={modelOptions()}
                      onChange={setModelB}
                    />
                  </div>
                </div>
              </div>
            </Show>
          </Card>

          {/* 对话/横评主画布 */}
          <div class="space-y-4 min-h-[420px]">
            <Show
              when={turns().length > 0}
              fallback={
                <Card class="p-12 text-center space-y-6 border-dashed border-subtle">
                  <div class="w-12 h-12 rounded-2xl bg-accent/10 text-accent flex items-center justify-center mx-auto shadow-glass">
                    <IconChat size={24} />
                  </div>
                  <div class="space-y-1">
                    <h3 class="text-base font-semibold text-foreground">开始你的模型交互演练</h3>
                    <p class="text-xs text-faint max-w-md mx-auto">
                      {mode() === 'compare'
                        ? '双模型竞技已激活：输入一个提示词，同时向两个模型并行派发，直观对比首字延迟 (TTFT)、生成耗时及文本质量。'
                        : '直接在下方输入提问，或点击下方快捷问题卡片快速启动测试。'}
                    </p>
                  </div>
                  <div class="flex flex-wrap items-center justify-center gap-2 max-w-xl mx-auto">
                    <For each={QUICK_PROMPTS}>
                      {q => (
                        <button
                          type="button"
                          class="px-3 py-1.5 text-xs text-muted hover:text-foreground bg-card hover:bg-hover border border-subtle rounded-lg transition-all text-left cursor-pointer"
                          onClick={() => handleSend(q)}
                        >
                          {q}
                        </button>
                      )}
                    </For>
                  </div>
                </Card>
              }
            >
              <For each={turns()}>
                {turn => (
                  <div class="space-y-3">
                    {/* 用户提问气泡 */}
                    <div class="flex justify-end">
                      <div class="max-w-[85%] rounded-2xl bg-accent text-accent-foreground px-4 py-2.5 text-sm shadow-sm whitespace-pre-wrap">
                        {turn.user}
                      </div>
                    </div>

                    {/* 回答区域 */}
                    <Show
                      when={mode() === 'compare'}
                      fallback={
                        /* 单模型回答卡片 */
                        <Card class="p-4 space-y-3 bg-card/70 backdrop-blur-sm">
                          <div class="flex items-center justify-between border-b border-subtle/50 pb-2 flex-wrap gap-2">
                            <div class="flex items-center gap-2">
                              <ProviderAvatar provider={turn.a.servedModel?.split('/')[0] || modelA().split('/')[0]} size="sm" />
                              <span class="text-xs font-mono font-medium text-foreground">
                                {turn.a.servedModel || modelA()}
                              </span>
                              <Show when={turn.a.busy}>
                                <span class="text-[11px] text-accent animate-pulse flex items-center gap-1">
                                  <IconZap size={11} /> 生成中...
                                </span>
                              </Show>
                            </div>

                            {/* 性能指标栏 */}
                            <div class="flex items-center gap-2 text-[11px] text-faint font-mono">
                              <Show when={turn.a.metrics?.ttftMs !== undefined}>
                                <span title="首字到达延迟 (TTFT)">
                                  TTFT: <strong class="text-foreground">{turn.a.metrics!.ttftMs}ms</strong>
                                </span>
                              </Show>
                              <Show when={turn.a.metrics?.totalMs !== undefined}>
                                <span title="总响应耗时">
                                  总计: <strong class="text-foreground">{(turn.a.metrics!.totalMs! / 1000).toFixed(2)}s</strong>
                                </span>
                              </Show>
                              <Show when={turn.a.metrics?.speed && turn.a.metrics!.speed > 0}>
                                <span title="生成速度">
                                  速度: <strong class="text-foreground">{turn.a.metrics!.speed} t/s</strong>
                                </span>
                              </Show>
                              <Show when={turn.a.rawRequest}>
                                <button
                                  type="button"
                                  class="hover:text-accent p-1 transition-colors cursor-pointer"
                                  title="查看原始请求与响应报文"
                                  onClick={() =>
                                    setRawJsonModal({
                                      title: turn.a.servedModel || modelA(),
                                      request: turn.a.rawRequest,
                                      response: turn.a.rawResponse,
                                    })
                                  }
                                >
                                  <IconCode size={13} />
                                </button>
                              </Show>
                              <button
                                type="button"
                                class="hover:text-accent p-1 transition-colors cursor-pointer"
                                title="复制回复内容"
                                onClick={() => copyText(turn.a.content, '助手回复')}
                              >
                                <IconClipboard size={13} />
                              </button>
                            </div>
                          </div>

                          <Show
                            when={!turn.a.error}
                            fallback={
                              <div class="p-3 bg-rose-500/10 border border-rose-500/20 rounded-xl text-rose-400 text-xs flex items-start gap-2">
                                <IconAlertCircle size={15} class="shrink-0 mt-0.5" />
                                <div>
                                  <div class="font-semibold">调用失败</div>
                                  <div class="font-mono mt-0.5">{turn.a.error}</div>
                                </div>
                              </div>
                            }
                          >
                            <div class="text-sm text-foreground/90 whitespace-pre-wrap leading-relaxed select-text font-sans">
                              {turn.a.content || (turn.a.busy ? '...' : '(空返回)')}
                            </div>
                          </Show>
                        </Card>
                      }
                    >
                      {/* 双模型并排横评展示 (Side-by-Side) */}
                      <div class="grid grid-cols-1 md:grid-cols-2 gap-3">
                        {/* 左侧：模型 A */}
                        <Card class="p-3.5 space-y-2.5 bg-card/70 backdrop-blur-sm border-t-2 border-t-blue-500/50">
                          <div class="flex items-center justify-between border-b border-subtle/40 pb-2 flex-wrap gap-1.5">
                            <div class="flex items-center gap-1.5 min-w-0">
                              <span class="px-1 py-0.2 rounded text-[10px] font-bold bg-blue-500/15 text-blue-400 shrink-0">
                                A
                              </span>
                              <span class="text-xs font-mono font-medium text-foreground truncate max-w-[140px]" title={turn.a.servedModel || modelA()}>
                                {turn.a.servedModel || modelA()}
                              </span>
                              <Show when={turn.a.busy}>
                                <span class="text-[10px] text-accent animate-pulse">生成中</span>
                              </Show>
                            </div>
                            <div class="flex items-center gap-1 text-[10px] font-mono text-faint">
                              <Show when={turn.a.metrics?.ttftMs !== undefined}>
                                <span>{turn.a.metrics!.ttftMs}ms</span>
                              </Show>
                              <Show when={turn.a.metrics?.speed && turn.a.metrics!.speed > 0}>
                                <span class="text-accent">{turn.a.metrics!.speed} t/s</span>
                              </Show>
                              <Show when={turn.a.rawRequest}>
                                <button
                                  type="button"
                                  class="hover:text-accent p-1 cursor-pointer"
                                  onClick={() =>
                                    setRawJsonModal({
                                      title: `模型 A (${turn.a.servedModel || modelA()})`,
                                      request: turn.a.rawRequest,
                                      response: turn.a.rawResponse,
                                    })
                                  }
                                >
                                  <IconCode size={12} />
                                </button>
                              </Show>
                              <button
                                type="button"
                                class="hover:text-accent p-1 cursor-pointer"
                                onClick={() => copyText(turn.a.content, '模型 A 回复')}
                              >
                                <IconClipboard size={12} />
                              </button>
                            </div>
                          </div>

                          <Show
                            when={!turn.a.error}
                            fallback={
                              <div class="p-2 bg-rose-500/10 border border-rose-500/20 rounded-lg text-rose-400 text-xs font-mono">
                                {turn.a.error}
                              </div>
                            }
                          >
                            <div class="text-xs sm:text-sm text-foreground/90 whitespace-pre-wrap leading-relaxed select-text min-h-[60px]">
                              {turn.a.content || (turn.a.busy ? '...' : '(空返回)')}
                            </div>
                          </Show>
                        </Card>

                        {/* 右侧：模型 B */}
                        <Card class="p-3.5 space-y-2.5 bg-card/70 backdrop-blur-sm border-t-2 border-t-purple-500/50">
                          <div class="flex items-center justify-between border-b border-subtle/40 pb-2 flex-wrap gap-1.5">
                            <div class="flex items-center gap-1.5 min-w-0">
                              <span class="px-1 py-0.2 rounded text-[10px] font-bold bg-purple-500/15 text-purple-400 shrink-0">
                                B
                              </span>
                              <span class="text-xs font-mono font-medium text-foreground truncate max-w-[140px]" title={turn.b?.servedModel || modelB()}>
                                {turn.b?.servedModel || modelB()}
                              </span>
                              <Show when={turn.b?.busy}>
                                <span class="text-[10px] text-accent animate-pulse">生成中</span>
                              </Show>
                            </div>
                            <div class="flex items-center gap-1 text-[10px] font-mono text-faint">
                              <Show when={turn.b?.metrics?.ttftMs !== undefined}>
                                <span>{turn.b!.metrics!.ttftMs}ms</span>
                              </Show>
                              <Show when={turn.b?.metrics?.speed && turn.b!.metrics!.speed > 0}>
                                <span class="text-accent">{turn.b!.metrics!.speed} t/s</span>
                              </Show>
                              <Show when={turn.b?.rawRequest}>
                                <button
                                  type="button"
                                  class="hover:text-accent p-1 cursor-pointer"
                                  onClick={() =>
                                    setRawJsonModal({
                                      title: `模型 B (${turn.b?.servedModel || modelB()})`,
                                      request: turn.b?.rawRequest,
                                      response: turn.b?.rawResponse,
                                    })
                                  }
                                >
                                  <IconCode size={12} />
                                </button>
                              </Show>
                              <button
                                type="button"
                                class="hover:text-accent p-1 cursor-pointer"
                                onClick={() => copyText(turn.b?.content || '', '模型 B 回复')}
                              >
                                <IconClipboard size={12} />
                              </button>
                            </div>
                          </div>

                          <Show
                            when={!turn.b?.error}
                            fallback={
                              <div class="p-2 bg-rose-500/10 border border-rose-500/20 rounded-lg text-rose-400 text-xs font-mono">
                                {turn.b?.error}
                              </div>
                            }
                          >
                            <div class="text-xs sm:text-sm text-foreground/90 whitespace-pre-wrap leading-relaxed select-text min-h-[60px]">
                              {turn.b?.content || (turn.b?.busy ? '...' : '(空返回)')}
                            </div>
                          </Show>
                        </Card>
                      </div>
                    </Show>
                  </div>
                )}
              </For>
            </Show>
          </div>

          {/* 底部输入框区 */}
          <Card class="p-3 bg-card/80 backdrop-blur-md sticky bottom-4 z-10 shadow-glass">
            <div class="space-y-2">
              <textarea
                class="w-full bg-transparent border-0 resize-none text-sm text-foreground placeholder:text-faint focus:outline-none min-h-[70px] max-h-[220px]"
                placeholder={
                  mode() === 'compare'
                    ? '输入提示词，同时向模型 A 和模型 B 发起竞技对比... (Enter 发送，Shift+Enter 换行)'
                    : '输入提示词测试该模型... (Enter 发送，Shift+Enter 换行)'
                }
                value={inputPrompt()}
                onInput={e => setInputPrompt(e.currentTarget.value)}
                onKeyDown={e => {
                  if (e.key === 'Enter' && !e.shiftKey) {
                    e.preventDefault()
                    handleSend()
                  }
                }}
              />
              <div class="flex items-center justify-between pt-1 border-t border-subtle/40">
                <div class="text-[11px] text-faint flex items-center gap-2">
                  <span>Enter 发送 · Shift+Enter 换行</span>
                  <Show when={stream()}>
                    <span class="text-accent flex items-center gap-0.5">
                      <span class="w-1.5 h-1.5 rounded-full bg-accent animate-ping" />
                      流式渲染
                    </span>
                  </Show>
                </div>
                <div class="flex items-center gap-2">
                  <Show when={isBusy()}>
                    <Button
                      variant="danger"
                      size="sm"
                      class="gap-1"
                      onClick={stopAll}
                    >
                      <IconSquare size={13} />
                      停止生成
                    </Button>
                  </Show>
                  <Button
                    variant="primary"
                    size="sm"
                    class="gap-1.5 px-4"
                    disabled={!inputPrompt().trim() || isBusy()}
                    onClick={() => handleSend()}
                  >
                    <IconPlay size={13} />
                    发送
                  </Button>
                </div>
              </div>
            </div>
          </Card>
        </div>

        {/* 右侧高级参数面板 (Inspector) */}
        <Show when={showParams()}>
          <div class="lg:col-span-4 xl:col-span-3 space-y-4">
            <Card class="p-4 space-y-4 bg-card/60 backdrop-blur-md">
              <div class="flex items-center justify-between border-b border-subtle/50 pb-2">
                <span class="text-xs font-semibold text-foreground flex items-center gap-1.5">
                  <IconSliders size={14} />
                  推理参数配置
                </span>
                <span class="text-[11px] text-faint">自动保存</span>
              </div>

              {/* 系统提示词 (System Prompt) */}
              <div class="space-y-1.5">
                <div class="flex items-center justify-between">
                  <label class="text-xs font-medium text-muted">系统提示词 (System Prompt)</label>
                  <Show when={systemPrompt()}>
                    <button
                      type="button"
                      class="text-[11px] text-faint hover:text-danger cursor-pointer"
                      onClick={() => setSystemPrompt('')}
                    >
                      重置
                    </button>
                  </Show>
                </div>
                <textarea
                  class="w-full bg-input border border-subtle rounded-control p-2 text-xs text-foreground placeholder:text-faint focus:outline-none focus:border-accent min-h-[90px] resize-y"
                  placeholder="设定模型的角色、输出格式或行为准则..."
                  value={systemPrompt()}
                  onInput={e => setSystemPrompt(e.currentTarget.value)}
                />
                <div class="flex flex-wrap gap-1 pt-1">
                  <For each={SYSTEM_PRESETS}>
                    {preset => (
                      <button
                        type="button"
                        class="px-2 py-0.5 text-[10px] rounded bg-hover text-muted hover:text-foreground hover:bg-accent/10 transition-colors cursor-pointer"
                        onClick={() => setSystemPrompt(preset.text)}
                      >
                        {preset.label}
                      </button>
                    )}
                  </For>
                </div>
              </div>

              {/* Temperature (温度) */}
              <div class="space-y-1">
                <div class="flex items-center justify-between text-xs">
                  <span class="text-muted font-medium">Temperature</span>
                  <span class="font-mono text-accent">{temperature().toFixed(2)}</span>
                </div>
                <input
                  type="range"
                  min="0"
                  max="2"
                  step="0.05"
                  value={temperature()}
                  onInput={e => setTemperature(parseFloat(e.currentTarget.value))}
                  class="w-full accent-accent cursor-pointer"
                />
                <div class="flex justify-between text-[10px] text-faint">
                  <span>精确 / 严谨 (0.0)</span>
                  <span>发散 / 创意 (2.0)</span>
                </div>
              </div>

              {/* Top P */}
              <div class="space-y-1">
                <div class="flex items-center justify-between text-xs">
                  <span class="text-muted font-medium">Top P</span>
                  <span class="font-mono text-accent">{topP().toFixed(2)}</span>
                </div>
                <input
                  type="range"
                  min="0"
                  max="1"
                  step="0.05"
                  value={topP()}
                  onInput={e => setTopP(parseFloat(e.currentTarget.value))}
                  class="w-full accent-accent cursor-pointer"
                />
              </div>

              {/* 最大输出 Tokens */}
              <div class="space-y-1">
                <div class="flex items-center justify-between text-xs">
                  <span class="text-muted font-medium">Max Tokens</span>
                  <span class="font-mono text-foreground">{maxTokens()}</span>
                </div>
                <input
                  type="number"
                  min="64"
                  max="32768"
                  step="256"
                  value={maxTokens()}
                  onInput={e => setMaxTokens(parseInt(e.currentTarget.value) || 2048)}
                  class="w-full bg-input border border-subtle rounded-control px-2.5 py-1 text-xs font-mono text-foreground focus:outline-none focus:border-accent"
                />
              </div>

              {/* 流式传输开关 */}
              <div class="flex items-center justify-between pt-2 border-t border-subtle/40">
                <div>
                  <div class="text-xs font-medium text-foreground">流式传输 (Stream)</div>
                  <div class="text-[10px] text-faint">逐字流式打字机渲染</div>
                </div>
                <Toggle
                  checked={stream()}
                  onChange={setStream}
                />
              </div>
            </Card>
          </div>
        </Show>
      </div>

      {/* 导出代码弹窗 */}
      <Modal
        open={codeModalOpen()}
        title="导出 API 客户端调用代码"
        onClose={() => setCodeModalOpen(false)}
      >
        <div class="space-y-4">
          <div class="flex items-center justify-between">
            <SegmentedControl
              value={codeLang()}
              onChange={l => setCodeLang(l as 'curl' | 'python' | 'node')}
              options={[
                { value: 'curl', label: 'cURL' },
                { value: 'python', label: 'Python' },
                { value: 'node', label: 'Node.js / TS' },
              ]}
              size="sm"
            />
            <Button
              variant="primary"
              size="sm"
              class="gap-1.5"
              onClick={() => {
                copyText(generateCode(codeLang()), '客户端代码')
                setCopiedCode(true)
                setTimeout(() => setCopiedCode(false), 2000)
              }}
            >
              <Show when={copiedCode()} fallback={<IconClipboard size={14} />}>
                <IconCheck size={14} />
              </Show>
              复制代码
            </Button>
          </div>

          <div class="relative">
            <pre class="p-4 bg-slate-950 text-slate-100 rounded-xl text-xs font-mono overflow-x-auto max-h-[360px] border border-slate-800">
              <code>{generateCode(codeLang())}</code>
            </pre>
          </div>
        </div>
      </Modal>

      {/* 原始 JSON 抽屉/弹窗 */}
      <Show when={rawJsonModal()}>
        {data => (
          <Modal
            open={true}
            title={`原始报文解析 - ${data().title}`}
            onClose={() => setRawJsonModal(null)}
          >
            <div class="space-y-4">
              <div class="space-y-1.5">
                <div class="flex items-center justify-between text-xs font-medium text-muted">
                  <span>客户端发送请求 (Request Payload)</span>
                  <button
                    type="button"
                    class="text-accent hover:underline cursor-pointer"
                    onClick={() => copyText(JSON.stringify(data().request, null, 2), '请求 JSON')}
                  >
                    复制请求 JSON
                  </button>
                </div>
                <pre class="p-3 bg-slate-950 text-slate-100 rounded-lg text-xs font-mono overflow-x-auto max-h-[160px] border border-slate-800">
                  {JSON.stringify(data().request, null, 2)}
                </pre>
              </div>

              <div class="space-y-1.5">
                <div class="flex items-center justify-between text-xs font-medium text-muted">
                  <span>服务端原始返回 (Response Payload)</span>
                  <button
                    type="button"
                    class="text-accent hover:underline cursor-pointer"
                    onClick={() => copyText(JSON.stringify(data().response, null, 2), '响应 JSON')}
                  >
                    复制响应 JSON
                  </button>
                </div>
                <pre class="p-3 bg-slate-950 text-slate-100 rounded-lg text-xs font-mono overflow-x-auto max-h-[220px] border border-slate-800">
                  {JSON.stringify(data().response, null, 2)}
                </pre>
              </div>
            </div>
          </Modal>
        )}
      </Show>
    </div>
  )
}

export default Playground
