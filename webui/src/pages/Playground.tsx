import { type Component, For, Show, createSignal, createEffect, createMemo, onMount, onCleanup } from 'solid-js'
import { reloadModels, type GatewayModel } from '@/lib/models'
import { asArray, dig, num, str } from '@/lib/json'
import { usePlaygroundStore, type AssistantResponse, type Metrics, type ProtocolOption, type ProtocolType, type Turn } from '@/stores/playground'
import {
  Card,
  Input,
  Textarea,
  Button,
  IconButton,
  Empty,
  Select,
  Toggle,
  Slider,
  Modal,
  PageHeader,
  SegmentedControl,
  ProviderAvatar,
  Badge,
  IconChat,
  IconZap,
  IconSparkles,
  IconSliders,
  IconCode,
  IconClipboard,
  IconCheck,
  IconTrash,
  IconSquare,
  IconPlay,
  IconAlertCircle,
  IconClose,
  TabTransition,
} from '@/components/ui'
import { useToast } from '@/lib/toast'
import { copyToClipboard } from '@/lib/clipboard'
import { confirm } from '@/lib/confirm'
import { useGatewayStore } from '@/stores/gateway'
import { buildRequestPayload, generateCodeSnippet, type CodeLang, type SamplingParams } from '@/lib/playgroundRequest'
import { useI18n } from '@/i18n'



const Playground: Component = () => {
  const {
    mode, setMode, protocol, setProtocol, modelA, setModelA, modelB, setModelB,
    systemPrompt, setSystemPrompt, temperature, setTemperature, topP, setTopP,
    maxTokens, setMaxTokens, stream, setStream, turnsAll, setTurnsAll, turns, setTurns,
  } = usePlaygroundStore()
  const { t } = useI18n()
  const toast = useToast()
  const systemPresets = () => [
    { label: t('playground.presets.defaultAssistant.label'), text: t('playground.presets.defaultAssistant.text') },
    { label: t('playground.presets.seniorArchitect.label'), text: t('playground.presets.seniorArchitect.text') },
    { label: t('playground.presets.creativeWriter.label'), text: t('playground.presets.creativeWriter.text') },
    { label: t('playground.presets.translator.label'), text: t('playground.presets.translator.text') },
  ]

  const quickPrompts = () => [
    t('playground.quickPrompts.p1'),
    t('playground.quickPrompts.p2'),
    t('playground.quickPrompts.p3'),
    t('playground.quickPrompts.p4'),
  ]

  const [models, setModels] = createSignal<GatewayModel[]>([])
  const [loadingModels, setLoadingModels] = createSignal(false)



  const protocolOptions = createMemo((): ProtocolOption[] => [
    {
      value: 'openai',
      label: t('playground.protocolOpenAI'),
      endpoint: '/v1/chat/completions',
      badgeTone: 'green',
      shortLabel: 'OpenAI Chat',
    },
    {
      value: 'responses',
      label: t('playground.protocolResponses'),
      endpoint: '/v1/responses',
      badgeTone: 'purple',
      shortLabel: 'Responses',
    },
    {
      value: 'anthropic',
      label: t('playground.protocolAnthropic'),
      endpoint: '/v1/messages',
      badgeTone: 'amber',
      shortLabel: 'Anthropic',
    },
    {
      value: 'gemini',
      label: t('playground.protocolGemini'),
      endpoint: '/v1beta/models/...:generateContent',
      badgeTone: 'blue',
      shortLabel: 'Gemini',
    },
  ])

  const activeProtocolMeta = createMemo(() => {
    const proto = protocol()
    return protocolOptions().find(p => p.value === proto) || protocolOptions()[0]
  })
  // 参数配置

  // 参数抽屉显示状态（桌面端默认展开，移动端默认收起）
  const [showParams, setShowParams] = createSignal(typeof window !== 'undefined' ? window.innerWidth >= 1024 : true)


  const [inputPrompt, setInputPrompt] = createSignal('')
  let chatContainerRef: HTMLDivElement | undefined
  let isNearBottom = true

  function handleScroll() {
    if (!chatContainerRef) return
    const threshold = 120
    const distanceToBottom = chatContainerRef.scrollHeight - chatContainerRef.scrollTop - chatContainerRef.clientHeight
    isNearBottom = distanceToBottom <= threshold
  }

  createEffect(() => {
    // 监听当前模式下的 turns 状态更新并在内部容器中平滑保持对话底部可见（带用户主动向上回溯时的阅读防拉扯保护）
    const currentTurns = turns()
    if (currentTurns.length === 0) return
    requestAnimationFrame(() => {
      if (chatContainerRef && isNearBottom) {
        chatContainerRef.scrollTo({
          top: chatContainerRef.scrollHeight,
          behavior: 'smooth',
        })
      }
    })
  })

  createEffect(() => {
    // 切换单/双模型模式时，重置贴底并滚动到对应模式会话底部
    mode()
    isNearBottom = true
    if (chatContainerRef) {
      requestAnimationFrame(() => {
        if (chatContainerRef) {
          chatContainerRef.scrollTo({
            top: chatContainerRef.scrollHeight,
          })
        }
      })
    }
  })
  // 控制器以便中断流式传输
  let abortControllerA: AbortController | null = null
  let abortControllerB: AbortController | null = null

  // 弹窗状态
  const [codeModalOpen, setCodeModalOpen] = createSignal(false)
  const [codeLang, setCodeLang] = createSignal<'curl' | 'python' | 'node'>('curl')
  const [rawJsonModal, setRawJsonModal] = createSignal<{ title: string; request: unknown; response: unknown } | null>(null)
  const [copiedCode, setCopiedCode] = createSignal(false)

  onMount(async () => {
    await loadModels()
  })


  async function loadModels() {
    setLoadingModels(true)
    const list = await reloadModels()
    setModels(list)
    if (list.length > 0) {
      if (!modelA() || !list.some(m => m.id === modelA())) {
        setModelA(list[0].id)
      }
      if (!modelB() || !list.some(m => m.id === modelB())) {
        setModelB(list[1]?.id || list[0].id)
      }
    }
    setLoadingModels(false)
  }

  const store = useGatewayStore()

  function formatContextLength(tokens?: number): string {
    if (!tokens || tokens <= 0) return ''
    if (tokens >= 1048576) return `${(tokens / 1048576).toFixed(tokens % 1048576 === 0 ? 0 : 1)}M`
    if (tokens >= 1024) return `${Math.round(tokens / 1024)}k`
    return `${tokens}`
  }

  function getProviderBadge(providerId: string): string {
    const reg = store.registryList().find(r => r.id === providerId)
    if (reg?.name) return reg.name
    if (providerId === 'codebuddy-cn' || providerId === 'codebuddy') return 'CodeBuddy'
    if (providerId === 'openrouter') return 'OpenRouter'
    if (providerId === 'antigravity') return 'Antigravity'
    if (providerId === 'opencode') return 'OpenCode'
    if (providerId === 'github-models') return 'GitHub'
    if (providerId === 'deepseek') return 'DeepSeek'
    return providerId.split('-')[0].charAt(0).toUpperCase() + providerId.split('-')[0].slice(1)
  }

  const modelOptions = createMemo(() =>
    models().map(m => {
      const providerId = m.owned_by || m.id.split('/')[0]
      const badge = getProviderBadge(providerId)
      let cleanName = m.display_name?.trim() || ''
      if (!cleanName || cleanName === m.id) {
        cleanName = m.id.includes('/') ? m.id.slice(m.id.indexOf('/') + 1) : m.id
      }
      const ctxStr = formatContextLength(m.context_length)
      const desc = ctxStr ? `${m.id} · ${ctxStr}` : m.id

      return {
        value: m.id,
        badge,
        label: cleanName,
        description: desc,
      }
    })
  )
  function getResponseModelLabel(resp?: AssistantResponse): string {
    if (!resp) return ''
    const opt = modelOptions().find(o => o.value === resp.targetModel)
    if (opt?.label) return opt.label

    const targetEntry = models().find(m => m.id === resp.targetModel)
    if (targetEntry?.display_name && targetEntry.display_name.trim() !== targetEntry.id) {
      return targetEntry.display_name.trim()
    }

    if (resp.servedModel && resp.servedModel !== 'auto' && resp.servedModel !== 'default') {
      const servedEntry = models().find(m => m.id === resp.servedModel || m.id.endsWith('/' + resp.servedModel))
      if (servedEntry?.display_name && servedEntry.display_name.trim() !== servedEntry.id) {
        return servedEntry.display_name.trim()
      }
      return resp.servedModel.includes('/') ? resp.servedModel.slice(resp.servedModel.indexOf('/') + 1) : resp.servedModel
    }

    const fallback = resp.targetModel
    return fallback.includes('/') ? fallback.slice(fallback.indexOf('/') + 1) : fallback
  }

  function getTurnProvider(resp?: AssistantResponse): string {
    if (!resp) return ''
    const modelId = resp.servedModel && resp.servedModel !== 'auto' && resp.servedModel !== 'default'
      ? resp.servedModel
      : resp.targetModel
    const entry = models().find(m => m.id === modelId || m.id === resp.targetModel)
    if (entry?.owned_by === 'cyrene-gateway' || entry?.owned_by === 'cyrene') {
      return 'cyrene'
    }
    if (modelId && modelId.includes('/')) {
      return modelId.split('/')[0]
    }
    if (entry?.owned_by) {
      return entry.owned_by
    }
    return (resp.targetModel || '').split('/')[0]
  }

  function getResponseProvider(resp?: AssistantResponse): string {
    if (!resp) return ''
    const opt = modelOptions().find(o => o.value === resp.targetModel)
    if (opt?.badge) return opt.badge
    return getProviderBadge(getTurnProvider(resp))
  }
  const isBusy = () =>
    turnsAll().some(t => t.a.busy || (t.b && t.b.busy))

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
    if (!await confirm(t('playground.clearConfirm'))) return
    stopAll()
    setTurnsAll(prev => prev.filter(t => t.mode !== mode()))
    toast.info(t('toast.clearHistorySuccess'))
  }

  // 采样参数只在 store 里，请求体构造与客户端示例共用同一份真值。
  function requestParams(): SamplingParams {
    return {
      systemPrompt: systemPrompt(),
      temperature: Number(temperature()),
      topP: Number(topP()),
      maxTokens: Number(maxTokens()),
    }
  }

  function codeSnippet(lang: CodeLang): string {
    return generateCodeSnippet({
      lang,
      proto: protocol(),
      model: modelA() || 'antigravity/gemini-2.5-flash',
      prompt: turns().slice(-1)[0]?.user || t('playground.sampleUserMessage'),
      origin: window.location.origin,
      stream: stream(),
      params: requestParams(),
    })
  }


  // 跨协议提取非流式响应文本
  function extractResponseContent(proto: ProtocolType, data: unknown): string {
    const openAIFallback = () => str(dig(data, 'choices', 0, 'message', 'content')) || str(dig(data, 'choices', 0, 'message', 'reasoning_content')) || ''
    switch (proto) {
      case 'responses': {
        const outText = str(dig(data, 'output_text'))
        if (outText) return outText
        const items = asArray(dig(data, 'output'))
        if (items && items.length > 0) {
          const blocks = asArray(dig(items[0], 'content'))
          if (blocks && blocks.length > 0) {
            return str(dig(blocks[0], 'text')) ?? ''
          }
        }
        return openAIFallback()
      }
      case 'anthropic': {
        const blocks = asArray(dig(data, 'content'))
        if (blocks && blocks.length > 0) {
          return blocks.map(b => str(dig(b, 'text')) ?? '').join('')
        }
        return openAIFallback()
      }
      case 'gemini': {
        const parts = asArray(dig(data, 'candidates', 0, 'content', 'parts'))
        if (parts && parts.length > 0) {
          return parts.map(p => str(dig(p, 'text')) ?? '').join('')
        }
        return openAIFallback()
      }
      case 'openai':
      default:
        return openAIFallback()
    }
  }

  // 跨协议提取 SSE 增量文本
  function extractSSEDelta(proto: ProtocolType, parsed: unknown): string | null {
    if (!parsed) return null
    const openAIFallback = () => str(dig(parsed, 'choices', 0, 'delta', 'content')) ?? str(dig(parsed, 'choices', 0, 'delta', 'reasoning_content'))
    switch (proto) {
      case 'responses': {
        if (dig(parsed, 'type') === 'response.output_text.delta') {
          const delta = str(dig(parsed, 'delta'))
          if (delta !== null) return delta
        }
        return openAIFallback()
      }
      case 'anthropic': {
        if (dig(parsed, 'type') === 'content_block_delta') {
          const delta = str(dig(parsed, 'delta', 'text'))
          if (delta !== null) return delta
        }
        return openAIFallback()
      }
      case 'gemini': {
        const parts = asArray(dig(parsed, 'candidates', 0, 'content', 'parts'))
        if (parts && parts.length > 0) {
          const t = parts.map(p => str(dig(p, 'text')) ?? '').join('')
          return t || null
        }
        return openAIFallback()
      }
      case 'openai':
      default:
        return openAIFallback()
    }
  }

  // 执行单模型请求（支持多协议 Stream 与非 Stream）
  async function executeRequest(
    targetModel: string,
    historyMessages: { role: string; content: string }[],
    onUpdate: (chunk: string, meta?: { servedModel?: string; firstToken?: boolean }) => void,
    signal: AbortSignal
  ): Promise<{
    fullContent: string
    servedModel: string
    protocol: ProtocolType
    metrics: Metrics
    rawRequest: unknown
    rawResponse: unknown
  }> {
    const startTime = performance.now()
    let firstTokenTime: number | null = null
    const activeProto = protocol()
    const reqInfo = buildRequestPayload(activeProto, targetModel, historyMessages, stream(), requestParams())

    const res = await fetch(reqInfo.url, {
      method: 'POST',
      headers: reqInfo.headers,
      body: JSON.stringify(reqInfo.body),
      signal,
    })

    const servedHeader = res.headers.get('x-cyrene-served-model') || ''

    if (!res.ok) {
      const errData: unknown = await res.json().catch(() => ({}))
      const errMsg = str(dig(errData, 'error', 'message')) ?? str(dig(errData, 'error'))
      const msg = errMsg || `HTTP ${res.status} ${res.statusText}`
      throw new Error(msg)
    }

    if (!stream() || !res.body) {
      const data: unknown = await res.json()
      const totalMs = Math.round(performance.now() - startTime)
      const content = extractResponseContent(activeProto, data)
      const reportedTokens =
        num(dig(data, 'usage', 'completion_tokens')) ??
        num(dig(data, 'usage', 'output_tokens')) ??
        num(dig(data, 'usageMetadata', 'candidatesTokenCount'))
      const completionTokens = reportedTokens ?? Math.max(1, Math.round(content.length / 3.5))
      const speed = totalMs > 0 ? Number(((completionTokens / totalMs) * 1000).toFixed(1)) : 0
      const respModel = str(dig(data, 'model')) ?? str(dig(data, 'modelVersion')) ?? ''
      onUpdate(content, { servedModel: servedHeader || respModel || undefined })
      return {
        fullContent: content,
        servedModel: servedHeader || respModel || targetModel,
        protocol: activeProto,
        metrics: {
          ttftMs: totalMs,
          totalMs,
          tokens: completionTokens,
          speed,
        },
        rawRequest: reqInfo.body,
        rawResponse: data,
      }
    }

    // Stream 模式处理
    const reader = res.body.getReader()
    const decoder = new TextDecoder('utf-8')
    let accumulated = ''
    let buffer = ''
    let servedModel = servedHeader
    const rawChunks: unknown[] = []

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
            const parsed: unknown = JSON.parse(dataStr)
            rawChunks.push(parsed)
            if (!servedModel) {
              servedModel = str(dig(parsed, 'model')) ?? str(dig(parsed, 'modelVersion')) ?? ''
            }
            const delta = extractSSEDelta(activeProto, parsed)
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
      protocol: activeProto,
      metrics: {
        ttftMs,
        totalMs,
        tokens: estimatedTokens,
        speed,
      },
      rawRequest: reqInfo.body,
      rawResponse: rawChunks.length > 0 ? rawChunks : { content: accumulated },
    }
  }

  async function handleSend(customPrompt?: string) {
    const text = (customPrompt || inputPrompt()).trim()
    if (!text || isBusy()) return
    isNearBottom = true
    if (!modelA()) {
      toast.error(t('toast.selectValidModel'))
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
      mode: isCompare ? 'compare' : 'single',
      a: { targetModel: modelA(), content: '', busy: true, protocol: protocol() },
      b: isCompare ? { targetModel: modelB(), content: '', busy: true, protocol: protocol() } : undefined,
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
                    ...t.a,
                    content: res.fullContent,
                    busy: false,
                    servedModel: res.servedModel,
                    protocol: res.protocol,
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
        if (err instanceof Error && (err.name === 'AbortError' || err.message.includes('aborted'))) {
          setTurns(prev =>
            prev.map(t => (t.id === turnId ? { ...t, a: { ...t.a, busy: false } } : t))
          )
          return
        }
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
                      ...t.b,
                      content: res.fullContent,
                      busy: false,
                      servedModel: res.servedModel,
                      protocol: res.protocol,
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
        if (err instanceof Error && (err.name === 'AbortError' || err.message.includes('aborted'))) {
          setTurns(prev =>
            prev.map(t => (t.id === turnId && t.b ? { ...t, b: { ...t.b, busy: false } } : t))
          )
          return
        }
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

    await Promise.allSettled([promiseA, promiseB])
  }

  async function copyText(text: string, label = t('playground.copyDefault')) {
    const ok = await copyToClipboard(text)
    if (ok) {
      toast.success(t('toast.copySuccess', { label }))
    } else {
      toast.info(text)
    }
  }

  // 生成代码片段
  function getProtocolBadgeTone(proto?: ProtocolType): 'green' | 'purple' | 'amber' | 'blue' | 'gray' {
    switch (proto) {
      case 'responses':
        return 'purple'
      case 'anthropic':
        return 'amber'
      case 'gemini':
        return 'blue'
      case 'openai':
        return 'green'
      default:
        return 'gray'
    }
  }

  function getProtocolLabel(proto?: ProtocolType): string {
    switch (proto) {
      case 'responses':
        return 'Responses'
      case 'anthropic':
        return 'Messages'
      case 'gemini':
        return 'Gemini'
      case 'openai':
        return 'Chat'
      default:
        return 'Chat'
    }
  }


  return (
    <div class="flex flex-col h-auto min-h-[calc(100dvh-9rem)] lg:h-[calc(100dvh-9rem)] lg:min-h-[36rem] min-w-0 max-w-7xl mx-auto gap-3 stagger">
      <PageHeader
        sticky={false}
        class="shrink-0 py-2.5"
        title={t('playground.title')}
        subtitle={t('playground.subtitle')}
        actions={
          <div class="flex items-center gap-2 flex-wrap">
            <SegmentedControl
              value={mode()}
              onChange={m => setMode(m as 'single' | 'compare')}
              options={[
                { value: 'single', label: t('playground.modeSingle') },
                { value: 'compare', label: t('playground.modeCompare') },
              ]}
              size="sm"
            />
            <Button
              variant="secondary"
              size="sm"
              class="gap-1.5"
              onClick={() => setCodeModalOpen(true)}
              title={t('playground.exportCodeTitle')}
            >
              <IconCode size={14} />
              {t('playground.exportCode')}
            </Button>
            <Button
              variant={showParams() ? 'secondary' : 'ghost'}
              size="sm"
              class="gap-1.5"
              onClick={() => setShowParams(!showParams())}
              title={t('playground.paramPanelTitle')}
            >
              <IconSliders size={14} />
              {t('playground.paramPanel')}
            </Button>
            <Button
              variant="ghost"
              size="sm"
              class="gap-1.5 text-danger hover:bg-danger/10"
              disabled={turns().length === 0}
              onClick={handleClear}
              title={t('playground.clearHistoryTitle')}
            >
              <IconTrash size={14} />
              {t('playground.clearHistory')}
            </Button>
          </div>
        }
      />

      {/* 主工作区布局：铺满视口剩余高度，左右列独立滚动与自适应 */}
      <div class="flex-1 min-h-0 min-w-0 grid grid-cols-1 lg:grid-cols-12 gap-3 items-stretch">
        {/* 对话互动主体区：纵向弹性布局，顶部选择器、中间可滚动对话流、底部固定输入框 */}
        <div class={`${showParams() ? 'lg:col-span-8 xl:col-span-9' : 'lg:col-span-12'} flex flex-col min-h-0 min-w-0 h-full gap-3`}>
          {/* 顶部模型选择栏 */}
          <Card class="p-2.5 min-w-0 shrink-0">
            <TabTransition
              value={mode()}
              order={['single', 'compare']}
              views={{
                single: () => (
                  <div class="flex flex-wrap items-center gap-2 sm:gap-3 min-w-0 py-0.5">
                    <span class="text-xs font-medium text-text shrink-0">
                      {t('playground.selectEvalModel')}
                    </span>
                    <div class="flex-1 basis-48 min-w-0">
                      <Select
                        class="w-full"
                        value={modelA()}
                        options={modelOptions()}
                        onChange={setModelA}
                      />
                    </div>
                    <Show when={loadingModels()}>
                      <span class="text-xs text-faint shrink-0">{t('common.syncing')}</span>
                    </Show>
                  </div>
                ),
                compare: () => (
                  <div class="grid min-w-0 grid-cols-1 md:grid-cols-2 gap-3 py-0.5">
                    <div class="flex flex-wrap items-center gap-2 min-w-0">
                      <span class="px-1.5 py-0.5 rounded text-xs font-semibold bg-info/10 text-info border border-info/25 shrink-0">
                        {t('playground.modelA')}
                      </span>
                      <div class="flex-1 basis-40 min-w-0">
                        <Select
                          class="w-full"
                          value={modelA()}
                          options={modelOptions()}
                          onChange={setModelA}
                        />
                      </div>
                    </div>
                    <div class="flex flex-wrap items-center gap-2 min-w-0">
                      <span class="px-1.5 py-0.5 rounded text-xs font-semibold bg-accent-2/10 text-accent-2 border border-accent-2/25 shrink-0">
                        {t('playground.modelB')}
                      </span>
                      <div class="flex-1 basis-40 min-w-0">
                        <Select
                          class="w-full"
                          value={modelB()}
                          options={modelOptions()}
                          onChange={setModelB}
                        />
                      </div>
                    </div>
                  </div>
                ),
              }}
            />
          </Card>

          {/* 对话消息滚动流（仅在内部滚动，不驱动页面全局滚动） */}
          <div
            ref={chatContainerRef}
            onScroll={handleScroll}
            class="flex-none h-[45dvh] min-h-64 max-h-128 lg:flex-1 lg:h-auto lg:min-h-0 lg:max-h-none min-w-0 overflow-y-auto space-y-3.5 px-1 scroll-smooth flex flex-col"
          >
            <Show
              when={turns().length > 0}
              fallback={
                <Empty
                  class="flex-1 flex justify-center flex-col"
                  icon={<IconChat size={24} />}
                  title={t('playground.startInteraction')}
                  description={mode() === 'compare' ? t('playground.arenaHint') : t('playground.startInteractionHint')}
                  action={
                    <div class="flex flex-wrap items-center justify-center gap-2 max-w-xl mx-auto">
                      <For each={quickPrompts()}>
                        {q => (
                          <Button
                            variant="secondary"
                            size="sm"
                            class="h-auto min-h-8 min-w-0 max-w-full py-1.5 whitespace-normal break-words [overflow-wrap:anywhere] text-left"
                            onClick={() => handleSend(q)}
                          >
                            {q}
                          </Button>
                        )}
                      </For>
                    </div>
                  }
                />
              }
            >
              <For each={turns()}>
                {turn => (
                  <div class="min-w-0 space-y-3">
                    {/* 用户提问气泡 */}
                    <div class="flex min-w-0 justify-end">
                      <div class="min-w-0 max-w-[85%] rounded-2xl bg-accent text-on-accent px-4 py-2.5 text-sm whitespace-pre-wrap break-words [overflow-wrap:anywhere] leading-relaxed">
                        {turn.user}
                      </div>
                    </div>

                    {/* 回答区域 */}
                    <Show
                      when={Boolean(turn.b && turn.mode === 'compare')}
                      fallback={
                        /* 单模型回答卡片 */
                        <Card class="min-w-0 p-4 space-y-3 border-t-2 border-t-accent/60">
                          <div class="flex items-center justify-between border-b border-subtle/50 pb-2 flex-wrap gap-2 shrink-0">
                            <div class="flex flex-wrap items-center gap-2 min-w-0">
                              <ProviderAvatar provider={getTurnProvider(turn.a)} size="sm" />
                              <div class="flex flex-col min-w-0 leading-tight">
                                <span class="text-xs font-semibold text-text truncate max-w-[260px] sm:max-w-[340px]" title={turn.a.servedModel ? `${turn.a.targetModel} (served: ${turn.a.servedModel})` : turn.a.targetModel}>
                                  {getResponseModelLabel(turn.a)}
                                </span>
                                <div class="flex items-center gap-1.5 mt-0.5">
                                  <span class="text-xs text-faint truncate max-w-[180px]">
                                    {getResponseProvider(turn.a)}
                                  </span>
                                  <Badge tone={getProtocolBadgeTone(turn.a.protocol)} class="text-xs font-mono px-1 py-0 uppercase">
                                    {getProtocolLabel(turn.a.protocol)}
                                  </Badge>
                                </div>
                              </div>
                              <Show when={turn.a.busy}>
                                <span class="text-xs text-info animate-pulse flex items-center gap-1 ml-1 shrink-0">
                                  <IconZap size={11} /> {t('playground.generating')}
                                </span>
                              </Show>
                            </div>

                            {/* 性能指标栏 */}
                            <div class="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted font-mono min-w-0">
                              <Show when={turn.a.metrics?.ttftMs !== undefined}>
                                <span title={t('playground.ttftTitle')}>
                                  TTFT: <strong class="text-text">{turn.a.metrics!.ttftMs}ms</strong>
                                </span>
                              </Show>
                              <Show when={turn.a.metrics?.totalMs !== undefined}>
                                <span title={t('playground.totalMsTitle')}>
                                  {t('playground.totalTime')}: <strong class="text-text">{(turn.a.metrics!.totalMs! / 1000).toFixed(2)}s</strong>
                                </span>
                              </Show>
                              <Show when={turn.a.metrics?.speed && turn.a.metrics!.speed > 0}>
                                <span title={t('playground.speedTitle')}>
                                  {t('playground.speed')}: <strong class="text-text">{turn.a.metrics!.speed} t/s</strong>
                                </span>
                              </Show>
                              <Show when={turn.a.rawRequest}>
                                <IconButton
                                  size="sm"
                                  title={t('playground.viewRawJson')}
                                  aria-label={t('playground.viewRawJson')}
                                  onClick={() =>
                                    setRawJsonModal({
                                      title: `${getResponseModelLabel(turn.a)} (${turn.a.targetModel})`,
                                      request: turn.a.rawRequest,
                                      response: turn.a.rawResponse,
                                    })
                                  }
                                >
                                  <IconCode size={14} />
                                </IconButton>
                              </Show>
                              <IconButton
                                size="sm"
                                title={t('playground.copyContent')}
                                aria-label={t('playground.copyContent')}
                                onClick={() => copyText(turn.a.content, t('playground.copyAssistant'))}
                              >
                                <IconClipboard size={14} />
                              </IconButton>
                            </div>
                          </div>

                          <Show
                            when={!turn.a.error}
                            fallback={
                              <div class="p-3 bg-danger/10 border border-danger/25 rounded-xl text-danger text-xs flex items-start gap-2">
                                <IconAlertCircle size={15} class="shrink-0 mt-0.5" />
                                <div class="min-w-0">
                                  <div class="font-semibold">{t('playground.callFailed')}</div>
                                  <div class="font-mono mt-0.5 whitespace-pre-wrap break-words [overflow-wrap:anywhere]">{turn.a.error}</div>
                                </div>
                              </div>
                            }
                          >
                            <div class="min-w-0 text-sm text-text whitespace-pre-wrap break-words [overflow-wrap:anywhere] leading-relaxed select-text font-sans">
                              {turn.a.content || (turn.a.busy ? '...' : t('playground.emptyResponse'))}
                            </div>
                          </Show>
                        </Card>
                      }
                    >
                      {/* 双模型并排横评展示 (Side-by-Side) */}
                      <div class="grid min-w-0 grid-cols-1 md:grid-cols-2 gap-3">
                        {/* 左侧：模型 A */}
                        <Card class="min-w-0 p-3.5 flex flex-col space-y-2.5 border-t-2 border-t-info/60">
                          <div class="flex items-center justify-between border-b border-subtle/40 pb-2 flex-wrap gap-1.5 shrink-0">
                            <div class="flex flex-wrap items-center gap-2 min-w-0">
                              <span class="w-5 h-5 rounded-md text-xs font-bold flex items-center justify-center shrink-0 bg-info/10 text-info border border-info/25">
                                A
                              </span>
                              <ProviderAvatar provider={getTurnProvider(turn.a)} size="sm" />
                              <div class="flex flex-col min-w-0 leading-tight">
                                <span class="text-xs font-semibold text-text truncate max-w-[150px] sm:max-w-[180px]" title={turn.a.servedModel ? `${turn.a.targetModel} (served: ${turn.a.servedModel})` : turn.a.targetModel}>
                                  {getResponseModelLabel(turn.a)}
                                </span>
                                <div class="flex items-center gap-1 mt-0.5">
                                  <span class="text-xs text-faint truncate max-w-[100px]">
                                    {getResponseProvider(turn.a)}
                                  </span>
                                  <Badge tone={getProtocolBadgeTone(turn.a.protocol)} class="text-xs font-mono px-1 py-0 uppercase">
                                    {getProtocolLabel(turn.a.protocol)}
                                  </Badge>
                                </div>
                              </div>
                              <Show when={turn.a.busy}>
                                <span class="text-xs text-info animate-pulse ml-1 shrink-0">{t('playground.generating')}</span>
                              </Show>
                            </div>
                            <div class="flex flex-wrap items-center gap-2 text-xs font-mono text-muted min-w-0">
                              <Show when={turn.a.metrics?.ttftMs !== undefined}>
                                <span>{turn.a.metrics!.ttftMs}ms</span>
                              </Show>
                              <Show when={turn.a.metrics?.speed && turn.a.metrics!.speed > 0}>
                                <span class="text-accent">{turn.a.metrics!.speed} t/s</span>
                              </Show>
                              <Show when={turn.a.rawRequest}>
                                <IconButton
                                  size="sm"
                                  title={t('playground.viewRawJson')}
                                  aria-label={t('playground.viewRawJson')}
                                  onClick={() =>
                                    setRawJsonModal({
                                      title: `${t('playground.modelA')} · ${getResponseModelLabel(turn.a)} (${turn.a.targetModel})`,
                                      request: turn.a.rawRequest,
                                      response: turn.a.rawResponse,
                                    })
                                  }
                                >
                                  <IconCode size={14} />
                                </IconButton>
                              </Show>
                              <IconButton
                                size="sm"
                                title={t('playground.copyModelA')}
                                aria-label={t('playground.copyModelA')}
                                onClick={() => copyText(turn.a.content, t('playground.copyModelA'))}
                              >
                                <IconClipboard size={14} />
                              </IconButton>
                            </div>
                          </div>

                          <Show
                            when={!turn.a.error}
                            fallback={
                              <div class="min-w-0 p-3 bg-danger/10 border border-danger/25 rounded-lg text-danger text-xs font-mono whitespace-pre-wrap break-words [overflow-wrap:anywhere]">
                                {turn.a.error}
                              </div>
                            }
                          >
                            <div class="min-w-0 text-sm text-text whitespace-pre-wrap break-words [overflow-wrap:anywhere] leading-relaxed select-text min-h-[60px]">
                              {turn.a.content || (turn.a.busy ? '...' : t('playground.emptyResponse'))}
                            </div>
                          </Show>
                        </Card>

                        {/* 右侧：模型 B */}
                        <Card class="min-w-0 p-3.5 flex flex-col space-y-2.5 border-t-2 border-t-accent-2/60">
                          <div class="flex items-center justify-between border-b border-subtle/40 pb-2 flex-wrap gap-1.5 shrink-0">
                            <div class="flex flex-wrap items-center gap-2 min-w-0">
                              <span class="w-5 h-5 rounded-md text-xs font-bold flex items-center justify-center shrink-0 bg-accent-2/10 text-accent-2 border border-accent-2/25">
                                B
                              </span>
                              <ProviderAvatar provider={getTurnProvider(turn.b)} size="sm" />
                              <div class="flex flex-col min-w-0 leading-tight">
                                <span class="text-xs font-semibold text-text truncate max-w-[150px] sm:max-w-[180px]" title={turn.b?.servedModel ? `${turn.b?.targetModel} (served: ${turn.b?.servedModel})` : turn.b?.targetModel}>
                                  {getResponseModelLabel(turn.b)}
                                </span>
                                <div class="flex items-center gap-1 mt-0.5">
                                  <span class="text-xs text-faint truncate max-w-[100px]">
                                    {getResponseProvider(turn.b)}
                                  </span>
                                  <Badge tone={getProtocolBadgeTone(turn.b?.protocol)} class="text-xs font-mono px-1 py-0 uppercase">
                                    {getProtocolLabel(turn.b?.protocol)}
                                  </Badge>
                                </div>
                              </div>
                              <Show when={turn.b?.busy}>
                                <span class="text-xs text-info animate-pulse ml-1 shrink-0">{t('playground.generating')}</span>
                              </Show>
                            </div>
                            <div class="flex flex-wrap items-center gap-2 text-xs font-mono text-muted min-w-0">
                              <Show when={turn.b?.metrics?.ttftMs !== undefined}>
                                <span>{turn.b!.metrics!.ttftMs}ms</span>
                              </Show>
                              <Show when={turn.b?.metrics?.speed && turn.b!.metrics!.speed > 0}>
                                <span class="text-accent">{turn.b!.metrics!.speed} t/s</span>
                              </Show>
                              <Show when={turn.b?.rawRequest}>
                                <IconButton
                                  size="sm"
                                  title={t('playground.viewRawJson')}
                                  aria-label={t('playground.viewRawJson')}
                                  onClick={() =>
                                    setRawJsonModal({
                                      title: `${t('playground.modelB')} · ${getResponseModelLabel(turn.b)} (${turn.b?.targetModel})`,
                                      request: turn.b?.rawRequest,
                                      response: turn.b?.rawResponse,
                                    })
                                  }
                                >
                                  <IconCode size={14} />
                                </IconButton>
                              </Show>
                              <IconButton
                                size="sm"
                                title={t('playground.copyModelB')}
                                aria-label={t('playground.copyModelB')}
                                onClick={() => copyText(turn.b?.content || '', t('playground.copyModelB'))}
                              >
                                <IconClipboard size={14} />
                              </IconButton>
                            </div>
                          </div>

                          <Show
                            when={!turn.b?.error}
                            fallback={
                              <div class="min-w-0 p-3 bg-danger/10 border border-danger/25 rounded-lg text-danger text-xs font-mono whitespace-pre-wrap break-words [overflow-wrap:anywhere]">
                                {turn.b?.error}
                              </div>
                            }
                          >
                            <div class="min-w-0 text-sm text-text whitespace-pre-wrap break-words [overflow-wrap:anywhere] leading-relaxed select-text min-h-[60px]">
                              {turn.b?.content || (turn.b?.busy ? '...' : t('playground.emptyResponse'))}
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

          {/* 底部输入框区（作为弹性容器底部固定子节点，彻底杜绝悬浮遮挡内容） */}
          <Card class="min-w-0 p-3 shrink-0 transition-colors duration-200 focus-within:border-accent/50 focus-within:ring-1 focus-within:ring-ring-soft">
            <div class="space-y-2">
              <Textarea
                class="w-full bg-transparent border-0 resize-none px-1 py-1 text-sm text-text placeholder:text-faint min-h-12 max-h-30"
                ariaLabel={mode() === 'compare' ? t('playground.sendComparePlaceholder') : t('playground.sendPlaceholder')}
                placeholder={
                  mode() === 'compare' ? t('playground.sendComparePlaceholder') : t('playground.sendPlaceholder')
                }
                value={inputPrompt()}
                onInput={setInputPrompt}
                onKeyDown={e => {
                  if (e.key === 'Enter' && !e.shiftKey) {
                    e.preventDefault()
                    handleSend()
                  }
                }}
              />
              <div class="flex flex-wrap items-center justify-between gap-3 pt-2 border-t border-subtle/40">
                <div class="text-xs text-muted flex flex-wrap items-center gap-x-3 gap-y-1 min-w-0">
                  <span>{t('playground.enterHint')}</span>
                  <Show when={stream()}>
                    <span class="text-success flex items-center gap-1.5">
                      <span class="w-1.5 h-1.5 shrink-0 rounded-full bg-success" />
                      {t('playground.streamingActive')}
                    </span>
                  </Show>
                </div>
                <div class="flex flex-wrap items-center gap-2 ml-auto">
                  <Show when={isBusy()}>
                    <Button
                      variant="danger"
                      size="sm"
                      class="gap-1"
                      onClick={stopAll}
                    >
                      <IconSquare size={14} />
                      {t('playground.stop')}
                    </Button>
                  </Show>
                  <Button
                    variant="primary"
                    size="sm"
                    class="gap-1.5 px-4"
                    disabled={!inputPrompt().trim() || isBusy()}
                    onClick={() => handleSend()}
                  >
                    <IconPlay size={14} />
                    {t('playground.send')}
                  </Button>
                </div>
              </div>
            </div>
          </Card>
        </div>

        {/* 右侧高级参数面板 (Inspector)：大屏并列内滚动，小屏侧滑抽屉 */}
        <Show when={showParams()}>
          <div
            class="lg:hidden fixed inset-0 z-40 bg-overlay backdrop-blur-sm animate-fade-in"
            onClick={() => setShowParams(false)}
          />
          <div class="fixed inset-y-0 right-0 z-50 w-80 max-w-[85vw] lg:static lg:w-auto lg:z-auto lg:col-span-4 xl:col-span-3 flex flex-col h-dvh lg:h-full min-h-0 min-w-0 animate-slide-up lg:animate-none">
            <Card class="flex flex-col h-full min-h-0 min-w-0 p-4 rounded-none lg:rounded-card border-l lg:border border-subtle/50">
              <div class="flex items-center justify-between border-b border-subtle/50 pb-2 shrink-0">
                <span class="text-xs font-semibold text-text flex items-center gap-1.5">
                  <IconSliders size={14} />
                  {t('playground.paramsConfig')}
                </span>
                <div class="flex items-center gap-2">
                  <span class="text-xs text-faint">{t('playground.autoSaved')}</span>
                  <IconButton
                    size="sm"
                    class="lg:hidden"
                    aria-label={t('common.close')}
                    onClick={() => setShowParams(false)}
                  >
                    <IconClose size="xs" />
                  </IconButton>
                </div>
              </div>
              {/* 参数项独立滚动容器 */}
              <div class="flex-1 min-h-0 overflow-y-auto space-y-4 px-1 pt-2">
              {/* 1. API 协议端点选择卡片 */}
              <div class="p-3 rounded-xl bg-surface-inset border border-subtle/50 space-y-2.5">
                <div class="flex items-center justify-between text-xs">
                  <span class="font-medium text-text flex items-center gap-1.5">
                    <IconZap size={14} class="text-accent" />
                    <span>{t('playground.protocol')}</span>
                  </span>
                  <Badge tone={activeProtocolMeta().badgeTone} class="text-[10px] font-mono px-1.5 py-0 uppercase">
                    POST
                  </Badge>
                </div>
                <Select
                  class="w-full text-xs font-mono"
                  size="sm"
                  value={protocol()}
                  options={protocolOptions()}
                  onChange={v => setProtocol(v as ProtocolType)}
                />
              </div>

              {/* 2. 系统提示词卡片 */}
              <div class="p-3 rounded-xl bg-surface-inset border border-subtle/50 space-y-2.5">
                <div class="flex items-center justify-between text-xs">
                  <span class="font-medium text-text flex items-center gap-1.5">
                    <IconSparkles size={14} class="text-accent" />
                    <span>{t('playground.systemPrompt')}</span>
                  </span>
                  <Show when={systemPrompt()}>
                    <Button
                      variant="ghost"
                      size="sm"
                      class="h-5! min-h-0! px-1.5! py-0! text-[11px]! text-muted hover:text-danger cursor-pointer"
                      onClick={() => setSystemPrompt('')}
                    >
                      {t('playground.systemPromptReset')}
                    </Button>
                  </Show>
                </div>
                <Textarea
                  class="p-2 text-xs min-h-[80px] resize-y"
                  ariaLabel={t('playground.systemPrompt')}
                  placeholder={t('playground.systemPromptPlaceholder')}
                  value={systemPrompt()}
                  onInput={setSystemPrompt}
                />
                <div class="flex flex-wrap items-center gap-1.5 pt-0.5">
                  <For each={systemPresets()}>
                    {preset => {
                      const isActive = () => systemPrompt() === preset.text
                      return (
                        <Button
                          variant={isActive() ? 'primary' : 'secondary'}
                          size="sm"
                          class="h-6! min-h-0! px-2.5! py-0! text-[11px]! rounded-full font-normal transition-all"
                          onClick={() => setSystemPrompt(preset.text)}
                          title={preset.text}
                        >
                          <span class={`w-1.5 h-1.5 rounded-full shrink-0 mr-1.5 ${isActive() ? 'bg-on-accent' : 'bg-muted/70'}`} />
                          <span>{preset.label}</span>
                        </Button>
                      )
                    }}
                  </For>
                </div>
              </div>

              {/* 3. 采样与推理参数卡片 */}
              <div class="p-3 rounded-xl bg-surface-inset border border-subtle/50 space-y-3.5">
                <div class="flex items-center justify-between text-xs">
                  <span class="font-medium text-text flex items-center gap-1.5">
                    <IconSliders size={14} class="text-accent" />
                    <span>{t('playground.samplingParams')}</span>
                  </span>
                </div>

                {/* Temperature (温度) */}
                <div class="space-y-1">
                  <Slider
                    label={t('playground.paramTemperature')}
                    min={0}
                    max={2}
                    step={0.05}
                    value={temperature()}
                    valueDisplay={temperature().toFixed(2)}
                    onChange={setTemperature}
                  />
                  <div class="flex justify-between text-[11px] text-faint">
                    <span>{t('playground.tempPrecise')}</span>
                    <span>{t('playground.tempCreative')}</span>
                  </div>
                </div>

                {/* Top P */}
                <div class="space-y-1">
                  <Slider
                    label={t('playground.paramTopP')}
                    min={0}
                    max={1}
                    step={0.05}
                    value={topP()}
                    valueDisplay={topP().toFixed(2)}
                    onChange={setTopP}
                  />
                </div>

                {/* 最大输出 Tokens */}
                <div class="space-y-1.5">
                  <label class="block text-xs font-medium text-muted leading-none">{t('playground.paramMaxTokens')}</label>
                  <Input
                    type="number"
                    min={64}
                    max={32768}
                    step={256}
                    size="sm"
                    value={maxTokens()}
                    onInput={(v: string) => setMaxTokens(parseInt(v) || 2048)}
                    class="font-mono"
                  />
                </div>

                {/* 流式传输开关 */}
                <div class="flex items-center justify-between gap-3 pt-2.5 border-t border-subtle/40">
                  <div class="min-w-0">
                    <div class="text-xs font-medium text-text">{t('playground.streamLabel')}</div>
                    <div class="text-[11px] text-faint">{t('playground.streamDesc')}</div>
                  </div>
                  <Toggle
                    ariaLabel={t('playground.streamLabel')}
                    checked={stream()}
                    onChange={setStream}
                  />
                </div>
              </div>
              </div>
            </Card>
          </div>
        </Show>
      </div>

      {/* 导出代码弹窗 */}
      <Modal
        open={codeModalOpen()}
        title={t('playground.exportCodeModalTitle')}
        onClose={() => setCodeModalOpen(false)}
      >
        <div class="space-y-4">
          <div class="flex items-center justify-between gap-2 flex-wrap">
            <div class="flex flex-wrap items-center gap-2 min-w-0">
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
              <Badge tone={activeProtocolMeta().badgeTone} class="text-xs font-mono px-2 py-0.5 uppercase">
                {activeProtocolMeta().shortLabel}
              </Badge>
            </div>
            <Button
              variant="primary"
              class="gap-1.5"
              onClick={() => {
                copyText(codeSnippet(codeLang()), t('playground.copyClientCode'))
                setCopiedCode(true)
                setTimeout(() => setCopiedCode(false), 2000)
              }}
            >
              <Show when={copiedCode()} fallback={<IconClipboard size={14} />}>
                <IconCheck size={14} />
              </Show>
              {t('playground.copyCode')}
            </Button>
          </div>

          <div class="relative">
            <pre class="min-w-0 p-4 bg-code-bg text-text rounded-xl text-xs leading-relaxed font-mono overflow-auto max-h-[min(24rem,60dvh)] border border-subtle">
              <code>{codeSnippet(codeLang())}</code>
            </pre>
          </div>
        </div>
      </Modal>

      {/* 原始 JSON 抽屉/弹窗 */}
      <Show when={rawJsonModal()}>
        {data => (
          <Modal
            open={true}
            title={`${t('playground.rawJsonModalTitle')} - ${data().title}`}
            onClose={() => setRawJsonModal(null)}
          >
            <div class="space-y-4">
              <div class="space-y-1.5">
                <div class="flex flex-wrap items-center justify-between gap-2 text-xs font-medium text-muted">
                  <span>{t('playground.requestPayload')}</span>
                  <Button
                    variant="ghost"
                    size="sm"
                    class="text-accent"
                    onClick={() => copyText(JSON.stringify(data().request, null, 2), t('playground.copyRequestJson'))}
                  >
                    {t('playground.copyRequestJson')}
                  </Button>
                </div>
                <pre class="min-w-0 p-3 bg-code-bg text-text rounded-lg text-xs leading-relaxed font-mono overflow-auto max-h-40 whitespace-pre-wrap break-words [overflow-wrap:anywhere] border border-subtle">
                  {JSON.stringify(data().request, null, 2)}
                </pre>
              </div>

              <div class="space-y-1.5">
                <div class="flex flex-wrap items-center justify-between gap-2 text-xs font-medium text-muted">
                  <span>{t('playground.responsePayload')}</span>
                  <Button
                    variant="ghost"
                    size="sm"
                    class="text-accent"
                    onClick={() => copyText(JSON.stringify(data().response, null, 2), t('playground.copyResponseJson'))}
                  >
                    {t('playground.copyResponseJson')}
                  </Button>
                </div>
                <pre class="min-w-0 p-3 bg-code-bg text-text rounded-lg text-xs leading-relaxed font-mono overflow-auto max-h-56 whitespace-pre-wrap break-words [overflow-wrap:anywhere] border border-subtle">
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
