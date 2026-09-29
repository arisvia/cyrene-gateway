import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, cleanup, fireEvent } from '@solidjs/testing-library'
import { MemoryRouter, Route } from '@solidjs/router'
import { api } from '@/lib/api'
import { t } from '@/i18n'
import { copyToClipboard } from '@/lib/clipboard'
import { usePlaygroundStore } from '@/stores/playground'
import Playground from '@/pages/Playground'

vi.mock('@/lib/api', () => ({
  api: vi.fn(), apiPost: vi.fn(), apiPut: vi.fn(), apiPatch: vi.fn(), apiDelete: vi.fn(), setOnUnauthorized: vi.fn(),
}))
const toastSpy = vi.hoisted(() => ({
  toasts: () => [], success: vi.fn(), error: vi.fn(), warning: vi.fn(), info: vi.fn(), dismiss: vi.fn(), clear: vi.fn(),
}))
vi.mock('@/lib/toast', () => ({ toast: toastSpy, useToast: () => toastSpy }))
vi.mock('@/lib/clipboard', () => ({ copyToClipboard: vi.fn(async () => true) }))

const tick = () => new Promise(r => setTimeout(r, 40))

// Solid 的受控输入要先走原生 setter，事件处理器读到的才是新值
function type(el: HTMLInputElement | HTMLTextAreaElement, value: string) {
  const proto = el instanceof HTMLTextAreaElement ? HTMLTextAreaElement : HTMLInputElement
  Object.getOwnPropertyDescriptor(proto.prototype, 'value')!.set!.call(el, value)
  fireEvent.input(el)
}

async function mount() {
  vi.mocked(api).mockImplementation(async (path: string) => {
    if (path === '/v1/models') {
      return { data: [{ id: 'qd/gemini-2.5-flash', object: 'model', owned_by: 'qd', display_name: 'Gemini 2.5 Flash' }] }
    }
    return null
  })
  const view = render(() => (
    <MemoryRouter>
      <Route path="/" component={Playground} />
    </MemoryRouter>
  ))
  await tick()
  return view
}

// Modal 走 portal，按钮挂在 baseElement 上，必须按整体文档查
const allButtons = (view: ReturnType<typeof render>) =>
  [...view.baseElement.querySelectorAll('button')] as HTMLButtonElement[]

const buttonByText = (view: ReturnType<typeof render>, label: string) =>
  allButtons(view).find(b => (b.textContent || '').includes(label))

beforeEach(() => {
  // 抽屉初始态来自 window.innerWidth >= 1024，显式钉住视口，
  // 不让 jsdom/vitest 的默认值决定用例前提。
  Object.defineProperty(window, 'innerWidth', { value: 1280, configurable: true })
  const store = usePlaygroundStore()
  store.setMode('single')
  store.setSystemPrompt('')
  store.setStream(false)
  store.setMaxTokens(2048)
  store.setTurnsAll([])
  vi.mocked(copyToClipboard).mockClear()
  for (const fn of [toastSpy.success, toastSpy.info]) fn.mockClear()
})

afterEach(() => cleanup())

describe('Playground 参数抽屉', () => {
  // beforeEach 把视口钉在 1280，所以抽屉初始为展开态
  it('参数面板按需展开与收起', async () => {
    const view = await mount()
    expect(view.getByText(t('playground.paramsConfig'))).toBeTruthy()
    expect(view.getByText(t('playground.samplingParams'))).toBeTruthy()
    expect(view.getByText(t('playground.paramMaxTokens'))).toBeTruthy()

    fireEvent.click(buttonByText(view, t('playground.paramPanel'))!)
    await tick()
    expect(view.queryByText(t('playground.paramsConfig'))).toBeNull()

    fireEvent.click(buttonByText(view, t('playground.paramPanel'))!)
    await tick()
    fireEvent.click(view.getByRole('button', { name: t('common.close') }))
    await tick()
    expect(view.queryByText(t('playground.paramsConfig'))).toBeNull()
  })

  it('系统提示直接落到 store，预设按钮写入自身文案', async () => {
    const view = await mount()
    const store = usePlaygroundStore()

    const area = view.getByPlaceholderText(t('playground.systemPromptPlaceholder')) as HTMLTextAreaElement
    type(area, '只回答一行')
    expect(store.systemPrompt()).toBe('只回答一行')

    // 有内容时才出现重置按钮
    fireEvent.click(buttonByText(view, t('playground.systemPromptReset'))!)
    await tick()
    expect(store.systemPrompt()).toBe('')

    // 预设芯片是唯一带 title 的 rounded-full 按钮（页头开关也带 title，必须区分）
    const preset = allButtons(view).find(b => /rounded-full/.test(b.className) && b.getAttribute('title'))
    expect(preset).toBeTruthy()
    fireEvent.click(preset!)
    expect(store.systemPrompt()).toBe(preset!.getAttribute('title'))
  })

  it('最大输出留空或 0 会回到 2048 默认，流式开关写 store', async () => {
    const view = await mount()
    const store = usePlaygroundStore()

    const max = view.getByRole('spinbutton') as HTMLInputElement
    type(max, '0')
    expect(store.maxTokens()).toBe(2048)
    type(max, '4096')
    expect(store.maxTokens()).toBe(4096)

    fireEvent.click(view.getByRole('switch', { name: t('playground.streamLabel') }))
    expect(store.stream()).toBe(true)
  })
})

describe('Playground 导出代码弹窗', () => {
  it('三种语言各自成文，复制走 clipboard 并提示', async () => {
    const view = await mount()
    fireEvent.click(buttonByText(view, t('playground.exportCode'))!)
    await tick()
    expect(view.baseElement.textContent).toContain('-X POST')

    fireEvent.click(buttonByText(view, 'Python')!)
    await tick()
    expect(view.baseElement.textContent).toContain('from openai import OpenAI')

    fireEvent.click(buttonByText(view, 'Node.js')!)
    await tick()
    expect(view.baseElement.textContent).toContain('import OpenAI from "openai"')

    fireEvent.click(buttonByText(view, t('playground.copyCode'))!)
    await tick()
    expect(copyToClipboard).toHaveBeenCalledTimes(1)
    expect(vi.mocked(copyToClipboard).mock.calls[0][0]).toContain('import OpenAI from "openai"')
    expect(toastSpy.success).toHaveBeenCalledTimes(1)
  })
})

describe('Playground 轮次回答区（拆卡前先钉住）', () => {
  const singleTurn = (over: Record<string, unknown> = {}) => ({
    id: 't1',
    user: '提问正文',
    mode: 'single' as const,
    a: {
      targetModel: 'qd/gemini-2.5-flash',
      content: '回答正文',
      busy: false,
      servedModel: 'qd/gemini-2.5-flash',
      protocol: 'openai' as const,
      metrics: { ttftMs: 123, totalMs: 1200, speed: 42 },
      ...over,
    },
  })

  it('单模型卡片渲染模型名、协议徽章与三项指标', async () => {
    usePlaygroundStore().setTurnsAll([singleTurn()])
    const view = await mount()
    await tick()
    const text = view.baseElement.textContent || ''
    expect(text).toContain('提问正文')
    expect(text).toContain('Gemini 2.5 Flash')
    expect(text).toContain('TTFT')
    expect(text).toContain('123ms')
    expect(text).toContain('42 t/s')
    expect(text).toContain(t('playground.totalTime'))
  })

  it('调用失败时整块换成错误提示，不再显示正文', async () => {
    usePlaygroundStore().setTurnsAll([singleTurn({ content: '', error: 'upstream 503' })])
    const view = await mount()
    await tick()
    const text = view.baseElement.textContent || ''
    expect(text).toContain(t('playground.callFailed'))
    expect(text).toContain('upstream 503')
    expect(text).not.toContain('回答正文')
  })

  it('既无正文又不在生成中时给出空返回占位', async () => {
    usePlaygroundStore().setTurnsAll([singleTurn({ content: '' })])
    const view = await mount()
    await tick()
    expect(view.baseElement.textContent).toContain(t('playground.emptyResponse'))
  })

  it('对比模式一次渲染 A 与 B 两张卡，各自带自己的模型与状态', async () => {
    const store = usePlaygroundStore()
    store.setMode('compare')
    store.setTurnsAll([{
      id: 'c1',
      user: '对比提问',
      mode: 'compare',
      a: { targetModel: 'qd/model-a', content: 'A 的回答', busy: false, protocol: 'openai', metrics: { ttftMs: 11 }, rawRequest: { marker: 'A-REQ' } },
      b: { targetModel: 'qd/model-b', content: 'B 的回答', busy: true, protocol: 'anthropic', metrics: { ttftMs: 22 }, rawRequest: { marker: 'B-REQ' } },
    }])
    const view = await mount()
    await tick()
    const text = view.baseElement.textContent || ''
    expect(text).toContain('A 的回答')
    expect(text).toContain('B 的回答')
    expect(text).toContain('model-a')
    expect(text).toContain('model-b')
    // B 仍在生成中，A 已完成
    expect(text).toContain(t('playground.generating'))
    // 两张卡各自带一个原始报文入口与一个按槽位命名的复制按钮
    expect(view.getAllByTitle(t('playground.viewRawJson')).length).toBe(2)
    expect(view.getByTitle(t('playground.copyModelA'))).toBeTruthy()
    expect(view.getByTitle(t('playground.copyModelB'))).toBeTruthy()
  })

  it('单模型卡的复制按钮写出该卡正文', async () => {
    usePlaygroundStore().setTurnsAll([singleTurn()])
    const view = await mount()
    await tick()
    fireEvent.click(view.getByTitle(t('playground.copyContent')))
    await tick()
    expect(copyToClipboard).toHaveBeenCalledWith('回答正文')
  })
})

describe('Playground 原始报文弹窗', () => {
  it('按轮次打开请求/响应 JSON，标题带上模型名', async () => {
    const store = usePlaygroundStore()
    store.setTurnsAll([{
      id: 't1',
      user: 'hello',
      mode: 'single',
      a: {
        targetModel: 'qd/gemini-2.5-flash',
        content: 'ok',
        busy: false,
        servedModel: 'qd/gemini-2.5-flash',
        rawRequest: { marker: 'REQ-42' },
        rawResponse: { marker: 'RESP-43' },
      },
    }])
    const view = await mount()
    await tick()

    fireEvent.click(view.getByRole('button', { name: t('playground.viewRawJson') }))
    await tick()
    expect(view.baseElement.textContent).toContain('REQ-42')
    expect(view.baseElement.textContent).toContain('RESP-43')

    fireEvent.click(buttonByText(view, t('playground.copyRequestJson'))!)
    await tick()
    expect(vi.mocked(copyToClipboard).mock.calls[0][0]).toContain('REQ-42')
  })
})
