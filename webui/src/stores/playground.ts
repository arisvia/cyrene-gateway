import { createEffect, createRoot, createSignal } from 'solid-js'
import { createStore } from 'solid-js/store'
import { dig, num, str } from '@/lib/json'

export interface Metrics {
  ttftMs?: number
  totalMs?: number
  tokens?: number
  speed?: number
}

export type ProtocolType = 'openai' | 'responses' | 'anthropic' | 'gemini'

export type ReasoningEffort = '' | 'none' | 'minimal' | 'low' | 'medium' | 'high' | 'xhigh' | 'max' | 'adaptive'

const VALID_REASONING_EFFORTS = new Set<string>(['', 'none', 'minimal', 'low', 'medium', 'high', 'xhigh', 'max', 'adaptive'])

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
  reasoningEffort?: ReasoningEffort
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
    const rawEffort = str(dig(parsed, 'reasoningEffort'))
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
      reasoningEffort: rawEffort !== null && VALID_REASONING_EFFORTS.has(rawEffort) ? (rawEffort as ReasoningEffort) : undefined,
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
  const [reasoningEffort, setReasoningEffort] = createSignal<ReasoningEffort>(saved.reasoningEffort ?? '')
  const [stream, setStream] = createSignal(saved.stream ?? true)

  // 对话历史：使用 createStore 实现细粒度响应式更新，流式追加时保持已有 Turn 引用不变，避免重建 ThinkingBlock 状态。
  const [turnsState, setTurnsState] = createStore<Turn[]>([])
  const turnsAll = () => turnsState
  const setTurnsAll = (updater: Turn[] | ((prev: Turn[]) => Turn[])) => {
    setTurnsState(typeof updater === 'function' ? updater(turnsState) : updater)
  }
  const setTurns = setTurnsAll
  const turns = () => turnsState.filter(turn => turn.mode === mode())
  const appendTurn = (turn: Turn) => setTurnsState(turnsState.length, turn)
  const updateTurnSlot = (turnId: string, slot: 'a' | 'b', patch: Partial<AssistantResponse>) => {
    const idx = turnsState.findIndex(t => t.id === turnId)
    if (idx >= 0 && turnsState[idx][slot]) {
      setTurnsState(idx, slot, patch)
    }
  }
  const stopBusyTurns = () => {
    setTurnsState(t => t.a.busy, 'a', 'busy', false)
    setTurnsState(t => Boolean(t.b?.busy), 'b', 'busy', false)
  }
  const clearTurns = () => setTurnsState(turnsState.filter(turn => turn.mode !== mode()))

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
      reasoningEffort: reasoningEffort(),
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
    maxTokens, setMaxTokens, reasoningEffort, setReasoningEffort, stream, setStream,
    turnsAll, setTurnsAll, turns, setTurns, appendTurn, updateTurnSlot, stopBusyTurns, clearTurns,
  }
}

let _store: ReturnType<typeof createPlaygroundStore> | null = null

export function usePlaygroundStore() {
  if (!_store) _store = createRoot(createPlaygroundStore)
  return _store
}
