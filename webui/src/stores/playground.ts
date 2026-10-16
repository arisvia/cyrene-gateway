import { createEffect, createRoot, createSignal } from 'solid-js'
import { dig, num, str } from '@/lib/json'

export interface Metrics {
  ttftMs?: number
  totalMs?: number
  tokens?: number
  speed?: number
}

export type ProtocolType = 'openai' | 'responses' | 'anthropic' | 'gemini'

export interface ProtocolOption {
  value: ProtocolType
  label: string
  endpoint: string
  badgeTone: 'green' | 'purple' | 'amber' | 'blue'
  shortLabel: string
}

export interface AssistantResponse {
  targetModel: string
  content: string
  reasoning?: string
  busy: boolean
  error?: string
  servedModel?: string
  protocol?: ProtocolType
  metrics?: Metrics
  rawRequest?: unknown
  rawResponse?: unknown
}

export interface Turn {
  id: string
  user: string
  mode: 'single' | 'compare'
  a: AssistantResponse
  b?: AssistantResponse
}

const CONFIG_KEY = 'cyrene_playground_config'

interface PlaygroundConfig {
  protocol?: ProtocolType
  mode?: 'single' | 'compare'
  modelA?: string
  modelB?: string
  systemPrompt?: string
  temperature?: number
  topP?: number
  maxTokens?: number
  stream?: boolean
}

// 浏览器里存的配置可能是任何形状（版本升级、手工改过），逐项窄化后再用。
function readConfig(): PlaygroundConfig {
  try {
    if (typeof localStorage === 'undefined' || !localStorage) return {}
    const saved = localStorage.getItem(CONFIG_KEY)
    if (!saved) return {}
    const parsed: unknown = JSON.parse(saved)
    const proto = str(dig(parsed, 'protocol'))
    const savedMode = str(dig(parsed, 'mode'))
    const stream = dig(parsed, 'stream')
    return {
      protocol: proto === 'openai' || proto === 'responses' || proto === 'anthropic' || proto === 'gemini' ? proto : undefined,
      mode: savedMode === 'single' || savedMode === 'compare' ? savedMode : undefined,
      modelA: str(dig(parsed, 'modelA')) ?? undefined,
      modelB: str(dig(parsed, 'modelB')) ?? undefined,
      systemPrompt: str(dig(parsed, 'systemPrompt')) ?? undefined,
      temperature: num(dig(parsed, 'temperature')) ?? undefined,
      topP: num(dig(parsed, 'topP')) ?? undefined,
      maxTokens: num(dig(parsed, 'maxTokens')) ?? undefined,
      stream: typeof stream === 'boolean' ? stream : undefined,
    }
  } catch {
    return {}
  }
}

// 会话与参数放在路由之外：离开演练场再回来不该丢掉整段对比结果。
function createPlaygroundStore() {
  const saved = readConfig()
  const [mode, setMode] = createSignal<'single' | 'compare'>(saved.mode ?? 'single')
  const [protocol, setProtocol] = createSignal<ProtocolType>(saved.protocol ?? 'openai')
  const [modelA, setModelA] = createSignal(saved.modelA ?? '')
  const [modelB, setModelB] = createSignal(saved.modelB ?? '')
  const [systemPrompt, setSystemPrompt] = createSignal(saved.systemPrompt ?? '')
  const [temperature, setTemperature] = createSignal(saved.temperature ?? 0.7)
  const [topP, setTopP] = createSignal(saved.topP ?? 0.95)
  const [maxTokens, setMaxTokens] = createSignal(saved.maxTokens ?? 2048)
  const [stream, setStream] = createSignal(saved.stream ?? true)

  // 对话历史：按模式隔离（单模型与双模型对比各自保持独立消息流，切换不串流）
  const [turnsAll, setTurnsAll] = createSignal<Turn[]>([])
  const turns = () => turnsAll().filter(turn => turn.mode === mode())
  const setTurns = (updater: Turn[] | ((prev: Turn[]) => Turn[])) => setTurnsAll(updater)
  const clearTurns = () => setTurnsAll(prev => prev.filter(turn => turn.mode !== mode()))

  createEffect(() => {
    const config: PlaygroundConfig = {
      protocol: protocol(),
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
        localStorage.setItem(CONFIG_KEY, JSON.stringify(config))
      }
    } catch {
      // 隐私模式或配额已满：参数丢失可接受，不打断演练
    }
  })

  return {
    mode, setMode, protocol, setProtocol, modelA, setModelA, modelB, setModelB,
    systemPrompt, setSystemPrompt, temperature, setTemperature, topP, setTopP,
    maxTokens, setMaxTokens, stream, setStream,
    turnsAll, setTurnsAll, turns, setTurns, clearTurns,
  }
}

let _store: ReturnType<typeof createPlaygroundStore> | null = null

export function usePlaygroundStore() {
  if (!_store) _store = createRoot(createPlaygroundStore)
  return _store
}
