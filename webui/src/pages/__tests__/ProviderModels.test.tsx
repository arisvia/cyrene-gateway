import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, cleanup, fireEvent } from '@solidjs/testing-library'
import { createMemoryHistory, MemoryRouter, Route } from '@solidjs/router'
import { t } from '@/i18n'
import { api, apiPost } from '@/lib/api'
import { useGatewayStore } from '@/stores/gateway'
import ProviderDetail from '@/pages/ProviderDetail'

vi.mock('@/lib/api', () => ({
  api: vi.fn(), apiPost: vi.fn(), apiPut: vi.fn(), apiPatch: vi.fn(), apiDelete: vi.fn(), setOnUnauthorized: vi.fn(),
}))
const toastSpy = vi.hoisted(() => ({
  toasts: () => [], success: vi.fn(), error: vi.fn(), warning: vi.fn(), info: vi.fn(), dismiss: vi.fn(), clear: vi.fn(),
}))
vi.mock('@/lib/toast', () => ({ toast: toastSpy, useToast: () => toastSpy }))
const confirmMock = vi.hoisted(() => vi.fn(async () => true))
vi.mock('@/lib/confirm', () => ({ confirm: confirmMock, alert: vi.fn(async () => true) }))

const tick = () => new Promise(r => setTimeout(r, 30))

interface ModelRow { id: string; name?: string; enabled?: boolean }

let registryModels: ModelRow[] = []
let customModels: ModelRow[] = []
let disabledCalls: Array<{ models: string[]; disabled: boolean }> = []
let disableReply: Promise<unknown> = Promise.resolve({ ok: true })
let testReply: unknown = { ok: false, error: 'boom' }

beforeEach(() => {
  registryModels = [
    { id: 'm1', name: 'M One', enabled: true },
    { id: 'm2', name: 'M Two', enabled: false },
  ]
  customModels = [{ id: 'm3', name: 'M Three', enabled: true }]
  disabledCalls = []
  disableReply = Promise.resolve({ ok: true })
  testReply = { ok: false, error: 'boom' }
  confirmMock.mockClear()
  for (const fn of [toastSpy.success, toastSpy.error, toastSpy.info, toastSpy.warning]) fn.mockClear()
  vi.mocked(api).mockImplementation(async (path: string) => {
    if (path === '/api/providers') return [{ id: 'selected', provider: 'github', name: 'Primary', authType: 'api-key', priority: 0, isActive: true }]
    if (path === '/api/registry') return { categories: [{ category: 'oauth', count: 1, providers: [{ id: 'github', name: 'GitHub', category: 'oauth', authType: 'api-key', apiType: 'openai', baseUrl: '', capabilities: ['llm'] }] }], total: 1 }
    if (path === '/api/providers/selected') return { id: 'selected', provider: 'github', name: 'Primary', authType: 'api-key', priority: 0, isActive: true }
    if (path.endsWith('/models')) return { registryModels, customModels, authModes: ['api-key'], hasApiKey: true }
    return null
  })
  vi.mocked(apiPost).mockImplementation(async (path: string, body?: unknown) => {
    if (path === '/api/models/disabled') {
      disabledCalls.push(body as { models: string[]; disabled: boolean })
      return disableReply
    }
    if (path === '/api/models/test') return testReply
    return { ok: true }
  })
})

afterEach(() => cleanup())

async function openModelsTab() {
  await useGatewayStore().loadCore()
  const history = createMemoryHistory()
  history.set({ value: '/providers/selected' })
  const view = render(() => (
    <MemoryRouter history={history}>
      <Route path="/providers/:id" component={ProviderDetail} />
    </MemoryRouter>
  ))
  await tick()
  fireEvent.click(view.getByRole('tab', { name: new RegExp(t('providerDetail.tabs.models')) }))
  await tick()
  return view
}

const buttonByText = (view: ReturnType<typeof render>, label: string) =>
  [...view.getAllByRole('button')].find(b => (b.textContent || '').includes(label))

describe('ProviderDetail 模型运维基线（重构前后必须等价）', () => {
  it('计数徽章按 enabled/total 显示，自定义模型计入总数', async () => {
    const view = await openModelsTab()
    expect(view.getByText('2/3')).toBeTruthy()
  })

  it('全部启用只提交被禁用的模型，并立即乐观置为全启用', async () => {
    const view = await openModelsTab()
    let settle!: (v: unknown) => void
    disableReply = new Promise(r => { settle = r })

    fireEvent.click(buttonByText(view, t('providerDetail.enableAllModels'))!)
    await tick()
    expect(disabledCalls).toEqual([{ models: ['github/m2'], disabled: false }])
    expect(view.getByText('3/3')).toBeTruthy()

    settle({ ok: true })
    await tick()
    expect(view.getByText('3/3')).toBeTruthy()
  })

  it('全部禁用需要确认，且只提交当前启用的模型', async () => {
    const view = await openModelsTab()
    fireEvent.click(buttonByText(view, t('providerDetail.disableAllModels'))!)
    await tick()
    expect(confirmMock).toHaveBeenCalledTimes(1)
    expect(disabledCalls).toEqual([{ models: ['github/m1', 'github/m3'], disabled: true }])
    expect(view.getByText('0/3')).toBeTruthy()
  })

  it('确认被拒绝时不提交任何批量请求', async () => {
    confirmMock.mockImplementationOnce(async () => false)
    const view = await openModelsTab()
    fireEvent.click(buttonByText(view, t('providerDetail.disableAllModels'))!)
    await tick()
    expect(disabledCalls).toEqual([])
    expect(view.getByText('2/3')).toBeTruthy()
  })

  it('批量请求失败时回滚到服务端状态', async () => {
    const view = await openModelsTab()
    disableReply = Promise.reject(new Error('upstream down'))
    fireEvent.click(buttonByText(view, t('providerDetail.enableAllModels'))!)
    await tick()
    await tick()
    expect(view.getByText('2/3')).toBeTruthy()
  })

  it('测试全部后出现"禁用失败项"，且只提交失败的那批', async () => {
    const view = await openModelsTab()
    fireEvent.click(buttonByText(view, t('providerDetail.testAllModels'))!)
    await tick()
    await tick()
    const failedButton = buttonByText(view, t('providerDetail.disableFailedModels'))
    // 已停用的模型不算失败项：只有当前启用的两条被测失败
    expect(failedButton!.textContent).toContain('(2)')

    disabledCalls = []
    fireEvent.click(failedButton!)
    await tick()
    expect(disabledCalls).toEqual([{ models: ['github/m1', 'github/m3'], disabled: true }])
  })

  it('单模型开关只提交该模型', async () => {
    const view = await openModelsTab()
    // 模型行的开关带 ariaLabel，页面头部另有一个启用总开关，必须按名称取
    fireEvent.click(view.getByRole('switch', { name: /M One/ }))
    await tick()
    expect(disabledCalls).toEqual([{ models: ['github/m1'], disabled: true }])
    expect(view.getByText('1/3')).toBeTruthy()
  })

  it('搜索框按模型 ID 过滤并显示命中数', async () => {
    const view = await openModelsTab()
    const search = view.getByPlaceholderText(t('providerDetail.modelSearchPlaceholder'))
    // Solid 的受控输入要先走原生 value setter，事件处理器读到的才是新值
    const setValue = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!
    setValue.call(search, 'm1')
    fireEvent.input(search)
    await tick()
    // 搜索只过滤注册表清单；自定义模型区与请求侧不受影响
    expect(view.queryByText(/M Two/)).toBeNull()
    expect(view.getByText(/M One/)).toBeTruthy()
    expect(view.getByText(/M Three/)).toBeTruthy()
    expect(disabledCalls).toEqual([])
  })
})

describe('ProviderDetail 模型提示与撞名边界', () => {
  // 页头还有一个"启用整个连接"的开关，模型行的开关必须按行可访问名取
  const rowSwitches = (view: ReturnType<typeof render>) =>
    view.getAllByRole('switch').filter(s => !/GitHub/.test(s.getAttribute('aria-label') || ''))
  const testButtons = (view: ReturnType<typeof render>) =>
    [...view.baseElement.querySelectorAll(`button[title="${t('providerDetail.testSingleTitle')}"]`)] as HTMLButtonElement[]

  it('单点探测请求异常时只由 api 层弹一次错误，不再补一条', async () => {
    const view = await openModelsTab()
    vi.mocked(apiPost).mockImplementationOnce(async () => { throw new Error('connection reset') })
    fireEvent.click(testButtons(view)[0])
    await tick()
    expect(toastSpy.error).not.toHaveBeenCalled()
    expect(disabledCalls).toEqual([])
  })

  it('上游明确返回失败时才提示模型测试未通过', async () => {
    const view = await openModelsTab()
    testReply = { ok: false, error: 'upstream refused' }
    fireEvent.click(testButtons(view)[0])
    await tick()
    expect(toastSpy.error).toHaveBeenCalledTimes(1)
    expect(toastSpy.error.mock.calls[0][0]).toContain('upstream refused')
  })

  it('一行开关只翻转它自己：另一行的 name 与本行 id 撞名也不牵连', async () => {
    registryModels = [{ id: 'dup', enabled: true }, { id: 'other', name: 'dup', enabled: true }]
    customModels = []
    const view = await openModelsTab()
    expect(view.getByText('2/2')).toBeTruthy()
    fireEvent.click(rowSwitches(view)[0])
    await tick()
    expect(disabledCalls).toEqual([{ models: ['github/dup'], disabled: true }])
    expect(view.getByText('1/2')).toBeTruthy()
  })
})

