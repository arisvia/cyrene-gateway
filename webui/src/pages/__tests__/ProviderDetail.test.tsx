import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { cleanup, fireEvent, render, screen, within } from '@solidjs/testing-library'
import { createMemoryHistory, MemoryRouter, Route } from '@solidjs/router'
import { t } from '@/i18n'
import { clearToasts, toast } from '@/lib/toast'
import { useGatewayStore, resetGatewayStoreForTesting } from '@/stores/gateway'
import type { Provider, RegistryProvider } from '@/types/domain'
import ProviderDetail from '@/pages/ProviderDetail'
import Providers from '@/pages/Providers'

type Page = 'catalog' | 'detail'
type PollResult = number | 'network' | 'success' | 'pending' | 'body-error'
const json = (body: unknown, status = 200) => new Response(JSON.stringify(body), { status })
const flush = () => vi.advanceTimersByTimeAsync(0)
const dialog = () => within(screen.getByRole('dialog'))

let registry: RegistryProvider
let connections: Provider[]
let pollResults: PollResult[]
let pollBodies: unknown[]
let addedBodies: unknown[]
let addResponse: Promise<Response> | undefined
let flowType: string
let oauthReplies: Map<string, Promise<Response>[]>
let oauthRequests: Array<{ path: string; signal: AbortSignal | null | undefined; body: unknown }>
let providerLoads: number
let modelsPayload: unknown = { registryModels: [], customModels: [], authModes: [] }
let disableBodies: unknown[] = []
let disableReply: Promise<Response> = Promise.resolve(json({ ok: true }))

// Deliberately ignore abort: an already-arriving response must still be fenced off by the page.
function deferOAuth(endpoint: string) {
  let resolve!: (response: Response) => void
  let reject!: (error: Error) => void
  const promise = new Promise<Response>((res, rej) => { resolve = res; reject = rej })
  const path = `/api/oauth/${registry.id}/${endpoint}`
  oauthReplies.set(path, [...(oauthReplies.get(path) ?? []), promise])
  return { resolve, reject }
}

beforeEach(() => {
  vi.useFakeTimers()
  flowType = 'device_code'
  oauthReplies = new Map()
  oauthRequests = []
  providerLoads = 0
  modelsPayload = { registryModels: [], customModels: [], authModes: [] }
  disableBodies = []
  disableReply = Promise.resolve(json({ ok: true }))
  registry = {
    id: 'github', name: 'GitHub', category: 'oauth', authType: 'oauth',
    authModes: ['api-key', 'oauth'], priority: 77, capabilities: ['llm'], apiType: 'openai',
  }
  connections = [
    { id: 'first', provider: 'github', name: 'Primary account', authType: 'oauth', priority: 1, isActive: true },
    { id: 'selected', provider: 'github', name: 'Selected account', authType: 'oauth', priority: 7, isActive: true },
  ]
  pollResults = []
  pollBodies = []
  addedBodies = []
  addResponse = undefined
  vi.spyOn(window, 'open').mockImplementation(() => null)
  vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const path = String(input)
    if (path.startsWith('/api/oauth/')) {
      const endpoint = path.split('?')[0]
      oauthRequests.push({ path: endpoint, signal: init?.signal, body: init?.body ? JSON.parse(String(init.body)) : undefined })
      const reply = oauthReplies.get(endpoint)?.shift()
      if (reply) return reply
    }
    if (path === '/api/providers' && init?.method === 'POST') {
      addedBodies.push(JSON.parse(String(init.body)))
      return addResponse ?? json({ ...connections[0], id: 'added' })
    }
    if (path === '/api/providers') {
      providerLoads++
      return json(connections)
    }
    if (path === '/api/registry') return json({ categories: [{ category: 'oauth', count: 1, providers: [registry] }], total: 1 })
    if (path === '/api/providers/test-credentials') return json({ ok: true, latency: '10ms' })
    if (path === '/api/models/disabled') {
      disableBodies.push(JSON.parse(String(init?.body)))
      return disableReply
    }
    if (path.endsWith('/models')) return json(modelsPayload)
    if (path.startsWith('/api/providers/')) return json(connections.find(c => path === `/api/providers/${c.id}`) ?? connections[0])
    if (path.endsWith('/device-code/poll')) {
      pollBodies.push(JSON.parse(String(init?.body)))
      const result = pollResults.shift() ?? 'pending'
      if (result === 'network') throw new TypeError('Failed to fetch')
      if (typeof result === 'number') return json({ error: 'expired_token' }, result)
      if (result === 'body-error') return json({ error: 'expired_token', pending: false })
      if (result === 'success') {
        connections = [{ id: 'authorized', provider: 'github', name: 'Authorized account', authType: 'oauth', priority: 7, isActive: true }]
        return json({ success: true, connection: connections[0] })
      }
      return json({ pending: true })
    }
    if (path.endsWith('/device-code')) return json({ verificationUri: 'https://github.com/login/device', userCode: 'TEST-CODE', deviceCode: 'device-token', interval: 1, expiresIn: 900 })
    if (path.endsWith('/status') && path.startsWith('/api/oauth/')) return json({ flowType, connections })
    if (path.includes('/authorize?')) return json({ authorizeUrl: 'https://example.com/authorize', state: 'pkce-state' })
    if (path === '/api/auth/status') return json({ requireLogin: false, authenticated: true })
    return json(null)
  }))
})

afterEach(() => {
  cleanup()
  clearToasts()
  vi.clearAllTimers()
  vi.useRealTimers()
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

async function openPage(page: Page) {
  if (page === 'catalog') connections = []
  await useGatewayStore().loadCore()
  const history = createMemoryHistory()
  history.set({ value: page === 'detail' ? '/providers/selected' : '/providers' })
  const view = render(() => (
    <MemoryRouter history={history}>
      <Route path="/providers" component={Providers} />
      <Route path="/providers/:id" component={ProviderDetail} />
    </MemoryRouter>
  ))
  await flush()
  if (page === 'catalog') {
    fireEvent.click(screen.getByRole('tab', { name: `${t('providers.tabs.catalog')} (1)` }))
    await vi.advanceTimersByTimeAsync(300)
    fireEvent.click(screen.getByRole('button', { name: t('providers.connectConfig') }))
    await flush()
  }
  return view
}

async function beginOAuth(page: Page) {
  fireEvent.click(page === 'catalog'
    ? dialog().getByRole('button', { name: t('providers.startOAuth') })
    : screen.getByRole('button', { name: t('providerDetail.reauthorize') }))
  await flush()
}

async function startOAuth(page: Page) {
  await beginOAuth(page)
  expect(dialog().getByText('TEST-CODE')).toBeTruthy()
}

async function closeOAuth() {
  fireEvent.keyDown(document, { key: 'Escape' })
  await flush()
  expect(screen.queryByRole('dialog')).toBeNull()
}

async function reopenOAuth(page: Page) {
  if (page === 'catalog') {
    fireEvent.click(screen.getByRole('button', { name: t('providers.connectConfig') }))
    await flush()
  }
  await beginOAuth(page)
}

function submitOAuthCredential(mode: 'import' | 'exchange') {
  // OAuth 面板两处共用同一套文案，市场向导与详情页不再有各自的键位
  const placeholder = mode === 'exchange'
    ? t('providerDetail.oauthPasteCallbackPlaceholder')
    : t('providerDetail.tokenPastePlaceholder')
  fireEvent.input(dialog().getByPlaceholderText(placeholder), { target: { value: 'test-credential' } })
  const submit = dialog().getByRole('button', { name: mode === 'exchange'
    ? t('providerDetail.confirmAndConnect')
    : t('providerDetail.importAndSave') }) as HTMLButtonElement
  submit.click()
  return submit
}

async function openAddAccount() {
  fireEvent.click(screen.getByRole('button', { name: t('providerDetail.addAccount') }))
  await flush()
}

async function startAddOAuth() {
  await openAddAccount()
  fireEvent.input(dialog().getByPlaceholderText(t('providerDetail.newAccountPlaceholder', { index: 3 })), { target: { value: 'Canceled draft' } })
  fireEvent.input(dialog().getByRole('spinbutton'), { target: { value: '44' } })
  fireEvent.click(dialog().getByRole('combobox'))
  fireEvent.click(screen.getByRole('option', { name: t('providerDetail.authOauthMode') }))
  fireEvent.click(dialog().getByRole('button', { name: t('providers.startOAuth') }))
  await flush()
}

// Both pages use the real store, API client, controls and dialogs; only HTTP and window.open are replaced.
describe.each<Page>(['catalog', 'detail'])('%s provider connection regressions', page => {
  it.each(['0', ''])('sends API-key priority %j as zero instead of a registry/account fallback', async value => {
    registry.authType = 'api-key'
    await openPage(page)
    if (page === 'detail') await openAddAccount()
    fireEvent.input(dialog().getByRole('spinbutton'), { target: { value } })
    fireEvent.input(dialog().getByPlaceholderText('sk-...'), { target: { value: 'test-api-key' } })
    if (page === 'catalog') {
      fireEvent.click(dialog().getByText(t('providerDetail.testConnection')))
      await flush()
    }
    fireEvent.click(dialog().getByRole('button', { name: t(page === 'catalog' ? 'providers.verifyAndSave' : 'providerDetail.saveAccount') }))
    await flush()
    expect(addedBodies).toEqual([expect.objectContaining({ provider: 'github', authType: 'api-key', priority: 0, data: { apiKey: 'test-api-key' } })])
    expect(screen.queryByRole('dialog')).toBeNull()
  })

  it.each([400, 403, 404])('stops polling and displays the authorization error on terminal HTTP %i', async status => {
    pollResults = [status]
    await openPage(page)
    await startOAuth(page)
    await vi.advanceTimersByTimeAsync(1000)
    expect.soft(dialog().queryByRole('alert')?.textContent).toContain('expired_token')
    expect.soft(dialog().queryByRole('alert')?.textContent).toContain(t('providerDetail.authProblemTitle'))
    expect.soft(screen.getByRole('dialog').querySelector('.animate-spin')).toBeNull()
    await vi.advanceTimersByTimeAsync(5000)
    expect(pollBodies).toHaveLength(1)
  })

  it.each<PollResult>(['network', 408, 429, 500])('retries transient %s failures and completes authorization', async result => {
    pollResults = [result, 'success']
    await openPage(page)
    await startOAuth(page)
    await vi.advanceTimersByTimeAsync(1000)
    expect(pollBodies).toHaveLength(1)
    expect(dialog().queryByRole('alert')).toBeNull()
    expect(screen.getByRole('dialog').querySelector('.animate-spin')).not.toBeNull()
    await vi.advanceTimersByTimeAsync(1000)
    expect(pollBodies).toHaveLength(2)
    expect(screen.queryByRole('dialog')).toBeNull()
    if (page === 'catalog') {
      expect(screen.getByRole('link', { name: t('providers.manageArrow') }).getAttribute('href')).toBe('/providers/authorized')
    } else {
      expect(document.body.textContent).toContain('Authorized account')
    }
    await vi.advanceTimersByTimeAsync(5000)
    expect(pollBodies).toHaveLength(2)
  })
})

describe.each<Page>(['catalog', 'detail'])('%s OAuth lifetime ownership', page => {
  it.each([400, 200])('ignores a canceled pending device poll returning HTTP %i while the next flow completes', async status => {
    await openPage(page)
    const stale = deferOAuth('device-code/poll')
    await startOAuth(page)
    const firstSignal = oauthRequests.at(-1)?.signal
    await vi.advanceTimersByTimeAsync(1000)
    await closeOAuth()
    await reopenOAuth(page)
    const nextSignal = oauthRequests.at(-1)?.signal
    clearToasts()
    const loads = providerLoads
    stale.resolve(json(status === 400 ? { error: 'stale-expired-token' } : { success: true }, status))
    await flush()
    expect.soft(screen.queryByRole('dialog')).not.toBeNull()
    expect.soft(screen.queryByRole('alert')).toBeNull()
    expect.soft(toast.toasts()).toEqual([])
    expect.soft(providerLoads).toBe(loads)
    expect.soft(firstSignal?.aborted).toBe(true)
    expect.soft(nextSignal?.aborted).toBe(false)
    pollResults = ['success']
    await vi.advanceTimersByTimeAsync(1000)
    expect(pollBodies).toHaveLength(1)
    expect(screen.queryByRole('dialog')).toBeNull()
    expect(providerLoads).toBeGreaterThan(loads)
    expect(nextSignal?.aborted).toBe(true)
    await vi.advanceTimersByTimeAsync(5000)
    expect(pollBodies).toHaveLength(1)
  })

  it.each([
    ['device-code', 'close'], ['device-code', 'unmount'],
    ['status', 'close'], ['status', 'unmount'],
    ['authorize', 'close'], ['authorize', 'unmount'],
  ] as const)('does not continue a pending %s request after %s', async (endpoint, action) => {
    if (endpoint === 'authorize') flowType = 'authorization_code_pkce'
    const view = await openPage(page)
    const stale = deferOAuth(endpoint)
    await beginOAuth(page)
    const signal = oauthRequests.at(-1)?.signal
    if (action === 'close') await closeOAuth()
    else view.unmount()
    clearToasts()
    const requests = oauthRequests.length
    const loads = providerLoads
    stale.resolve(json(endpoint === 'status' ? { flowType: 'import_token', connections } : endpoint === 'authorize'
      ? { authorizeUrl: 'https://example.com/stale', state: 'stale' }
      : { verificationUri: 'https://example.com/stale', deviceCode: 'stale', userCode: 'STALE-CODE', interval: 1 }))
    await flush()
    expect.soft(window.open).not.toHaveBeenCalled()
    expect.soft(screen.queryByRole('dialog')).toBeNull()
    expect.soft(toast.toasts()).toEqual([])
    expect.soft(signal?.aborted).toBe(true)
    await vi.advanceTimersByTimeAsync(6000)
    expect(oauthRequests).toHaveLength(requests)
    expect(providerLoads).toBe(loads)
  })

  it.each(['status', 'device-code', 'authorize'])('suppresses a late %s failure after closing', async endpoint => {
    if (endpoint === 'authorize') flowType = 'authorization_code_pkce'
    await openPage(page)
    const stale = deferOAuth(endpoint)
    await beginOAuth(page)
    await closeOAuth()
    clearToasts()
    stale.resolve(json({ error: 'stale-start-error' }, 400))
    await flush()
    expect(screen.queryByRole('dialog')).toBeNull()
    expect(toast.toasts()).toEqual([])
    expect(window.open).not.toHaveBeenCalled()
  })

  it('silences a real abort rejection during startup', async () => {
    await openPage(page)
    const stale = deferOAuth('status')
    await beginOAuth(page)
    await closeOAuth()
    stale.reject(new DOMException('Canceled', 'AbortError'))
    await flush()
    expect(screen.queryByRole('dialog')).toBeNull()
    expect(toast.toasts()).toEqual([])
  })

  it.each([400, 200])('ignores a canceled PKCE status poll returning HTTP %i', async status => {
    flowType = 'authorization_code_pkce'
    await openPage(page)
    await beginOAuth(page)
    const stale = deferOAuth('status')
    await vi.advanceTimersByTimeAsync(2500)
    const firstSignal = oauthRequests.at(-1)?.signal
    await closeOAuth()
    await reopenOAuth(page)
    const nextSignal = oauthRequests.at(-1)?.signal
    clearToasts()
    const loads = providerLoads
    stale.resolve(json(status === 400 ? { error: 'stale-status-error' } : { connections: [{ id: 'stale-account' }] }, status))
    await flush()
    expect.soft(screen.queryByRole('dialog')).not.toBeNull()
    expect.soft(toast.toasts()).toEqual([])
    expect.soft(providerLoads).toBe(loads)
    expect.soft(firstSignal?.aborted).toBe(true)
    expect.soft(nextSignal?.aborted).toBe(false)
    connections = [...connections, { id: 'pkce-account', provider: 'github', name: 'PKCE account', authType: 'oauth', priority: 0, isActive: true }]
    await vi.advanceTimersByTimeAsync(2500)
    expect(screen.queryByRole('dialog')).toBeNull()
    expect(providerLoads).toBeGreaterThan(loads)
    expect(nextSignal?.aborted).toBe(true)
  })

  it.each([
    ['import', 400], ['import', 200], ['exchange', 400], ['exchange', 200],
  ] as const)('does not let a canceled %s HTTP %i reset the next pending submission', async (mode, status) => {
    flowType = mode === 'import' ? 'import_token' : 'authorization_code_pkce'
    if (mode === 'exchange') {
      registry.id = 'antigravity'
      connections = connections.map(c => ({ ...c, provider: 'antigravity' }))
    }
    await openPage(page)
    await beginOAuth(page)
    const stale = deferOAuth(mode)
    const firstSubmit = submitOAuthCredential(mode)
    await flush()
    expect(firstSubmit.disabled).toBe(true)
    const firstRequest = oauthRequests.at(-1)
    expect(firstRequest?.body).toMatchObject({ name: page === 'catalog' ? 'GitHub' : 'Selected account', priority: page === 'catalog' ? 0 : 7 })
    await closeOAuth()
    await reopenOAuth(page)
    const next = deferOAuth(mode)
    const nextSubmit = submitOAuthCredential(mode)
    await flush()
    const nextSignal = oauthRequests.at(-1)?.signal
    expect(nextSubmit.disabled).toBe(true)
    clearToasts()
    const loads = providerLoads
    stale.resolve(json(status === 400 ? { error: 'stale-submit-error' } : { success: true }, status))
    await flush()
    expect.soft(screen.queryByRole('dialog')).not.toBeNull()
    expect.soft(screen.queryByRole('alert')).toBeNull()
    expect.soft(nextSubmit.disabled).toBe(true)
    expect.soft(toast.toasts()).toEqual([])
    expect.soft(providerLoads).toBe(loads)
    expect.soft(firstRequest?.signal?.aborted).toBe(true)
    expect.soft(nextSignal?.aborted).toBe(false)
    next.resolve(json({ success: true }))
    await flush()
    expect(screen.queryByRole('dialog')).toBeNull()
    expect(providerLoads).toBeGreaterThan(loads)
    expect(nextSignal?.aborted).toBe(true)
  })
})

describe('ProviderDetail account flow metadata', () => {
  it('reauthorizes the selected account rather than zero/default metadata', async () => {
    await openPage('detail')
    await startOAuth('detail')
    await vi.advanceTimersByTimeAsync(1000)
    expect(pollBodies).toEqual([expect.objectContaining({ name: 'Selected account', priority: 7 })])
  })

  it('reauthorizes the selected account after canceling an add-account flow', async () => {
    await openPage('detail')
    await startAddOAuth()
    await vi.advanceTimersByTimeAsync(1000)
    expect(pollBodies[0]).toMatchObject({ name: 'Canceled draft', priority: 44 })
    fireEvent.keyDown(document, { key: 'Escape' })
    await flush()
    expect(screen.queryByRole('dialog')).toBeNull()
    await vi.advanceTimersByTimeAsync(2000)
    expect(pollBodies).toHaveLength(1)
    await startOAuth('detail')
    await vi.advanceTimersByTimeAsync(1000)
    expect(pollBodies[1]).toMatchObject({ name: 'Selected account', priority: 7 })
  })

  it.each<PollResult>(['body-error', 400])('retains add-account metadata when retrying %s in the same flow', async result => {
    pollResults = [result, 'success']
    await openPage('detail')
    await startAddOAuth()
    await vi.advanceTimersByTimeAsync(1000)
    expect(dialog().getByRole('alert').textContent).toContain('expired_token')
    fireEvent.click(dialog().getByRole('button', { name: t('common.retry') }))
    await flush()
    await vi.advanceTimersByTimeAsync(1000)
    expect(pollBodies).toEqual([
      expect.objectContaining({ name: 'Canceled draft', priority: 44 }),
      expect.objectContaining({ name: 'Canceled draft', priority: 44 }),
    ])
    expect(screen.queryByRole('dialog')).toBeNull()
  })

  it('disables account submission while the request is pending', async () => {
    let resolveAdd!: (response: Response) => void
    addResponse = new Promise(resolve => { resolveAdd = resolve })
    await openPage('detail')
    await openAddAccount()
    fireEvent.input(dialog().getByPlaceholderText('sk-...'), { target: { value: 'test-api-key' } })
    const submit = dialog().getByRole('button', { name: t('providerDetail.saveAccount') }) as HTMLButtonElement
    fireEvent.click(submit)
    await flush()
    expect.soft(submit.disabled).toBe(true)
    submit.click()
    expect.soft(addedBodies).toHaveLength(1)
    resolveAdd(json({ ...connections[1], id: 'added' }))
    await flush()
    expect(screen.queryByRole('dialog')).toBeNull()
  })
})

describe('ProviderDetail model switch optimistic state', () => {
  async function openModelsTab() {
    modelsPayload = {
      registryModels: [{ id: 'gpt-test', name: 'gpt-test', enabled: true }],
      customModels: [],
      authModes: registry.authModes,
    }
    await useGatewayStore().loadCore()
    const history = createMemoryHistory()
    history.set({ value: '/providers/selected' })
    const view = render(() => (
      <MemoryRouter history={history}>
        <Route path="/providers" component={Providers} />
        <Route path="/providers/:id" component={ProviderDetail} />
      </MemoryRouter>
    ))
    await flush()
    fireEvent.click(view.getByRole('tab', { name: new RegExp(t('providerDetail.tabs.models')) }))
    await flush()
    return view
  }

  it('greys the model out before the batch request settles', async () => {
    let settle!: (response: Response) => void
    disableReply = new Promise<Response>(resolve => { settle = resolve })

    const view = await openModelsTab()
    const toggle = view.getByRole('switch', { name: /gpt-test/ })
    expect(toggle.getAttribute('aria-checked')).toBe('true')

    fireEvent.click(toggle)
    await flush()
    expect(disableBodies).toEqual([{ models: ['github/gpt-test'], disabled: true }])
    expect(view.getByRole('switch', { name: /gpt-test/ }).getAttribute('aria-checked')).toBe('false')

    settle(json({ ok: true }))
    await flush()
    expect(view.getByRole('switch', { name: /gpt-test/ }).getAttribute('aria-checked')).toBe('false')
  })
})

describe('ProviderDetail header group state hydration', () => {
  it('reflects the loaded connection until the provider list hydrates, then shows the tri-state', async () => {
    resetGatewayStoreForTesting()
    const { promise: listPending, resolve: releaseList } = Promise.withResolvers<Response>()
    vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL) => {
      const path = String(input)
      if (path === '/api/providers') return listPending
      if (path === '/api/registry') return json({ categories: [{ category: 'oauth', count: 1, providers: [registry] }], total: 1 })
      if (path.endsWith('/models')) return json(modelsPayload)
      if (path.startsWith('/api/providers/')) return json(connections.find(c => path === `/api/providers/${c.id}`) ?? connections[0])
      if (path === '/api/auth/status') return json({ requireLogin: false, authenticated: true })
      return json(null)
    }))

    const history = createMemoryHistory()
    history.set({ value: '/providers/selected' })
    render(() => (
      <MemoryRouter history={history}>
        <Route path="/providers" component={Providers} />
        <Route path="/providers/:id" component={ProviderDetail} />
      </MemoryRouter>
    ))
    await flush()

    // Detail loaded but store.providers() still pending: header must mirror
    // the loaded connection, not a false "all disabled" group state.
    const headerToggle = () => within(screen.getByRole('banner')).getByRole('switch', { name: new RegExp(`^${t('common.enabled')}: `) })
    expect(headerToggle().getAttribute('aria-checked')).toBe('true')
    expect(screen.getByText(t('common.enabled'))).toBeTruthy()

    // Hydrated pool: selected active, sibling disabled → partial group state.
    releaseList(json([
      { ...connections[0], isActive: false },
      connections[1],
    ]))
    await flush()
    expect(screen.getByText(t('providers.partiallyEnabled', { active: 1, total: 2 }))).toBeTruthy()
    expect(headerToggle().getAttribute('aria-checked')).toBe('false')
  })

  it('toggles the loaded connection when the header switch is clicked before the provider list hydrates', async () => {
    resetGatewayStoreForTesting()
    const { promise: listPending, resolve: releaseList } = Promise.withResolvers<Response>()
    const puts: Array<{ path: string; body: unknown }> = []
    let toggled = false
    vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const path = String(input)
      if (path === '/api/providers') return listPending
      if (path === '/api/registry') return json({ categories: [{ category: 'oauth', count: 1, providers: [registry] }], total: 1 })
      if (path.endsWith('/models')) return json(modelsPayload)
      if (path.startsWith('/api/providers/') && init?.method === 'PUT') {
        puts.push({ path, body: init?.body ? JSON.parse(String(init.body)) : undefined })
        toggled = true
        return json({ ...connections[1], isActive: false })
      }
      if (path === '/api/providers/selected') return json({ ...connections[1], isActive: !toggled })
      if (path.startsWith('/api/providers/')) return json(connections.find(c => path === `/api/providers/${c.id}`) ?? connections[0])
      if (path === '/api/auth/status') return json({ requireLogin: false, authenticated: true })
      return json(null)
    }))

    const history = createMemoryHistory()
    history.set({ value: '/providers/selected' })
    render(() => (
      <MemoryRouter history={history}>
        <Route path="/providers" component={Providers} />
        <Route path="/providers/:id" component={ProviderDetail} />
      </MemoryRouter>
    ))
    await flush()

    const headerToggle = () => within(screen.getByRole('banner')).getByRole('switch', { name: new RegExp(`^${t('common.enabled')}: `) })
    expect(headerToggle().getAttribute('aria-checked')).toBe('true')

    // Provider list still pending: the click must not be a silent no-op — it
    // targets the loaded connection directly.
    fireEvent.click(headerToggle())
    await flush()
    expect(puts).toEqual([{ path: '/api/providers/selected', body: { isActive: false } }])

    releaseList(json([connections[0], { ...connections[1], isActive: false }]))
    await flush()
    expect(headerToggle().getAttribute('aria-checked')).toBe('false')
  })
})

describe('ProviderDetail cooldown badge expiry', () => {
  it('renders the badge only while rateLimitedUntil is in the future', async () => {
    connections = [
      { id: 'first', provider: 'github', name: 'Primary account', authType: 'oauth', priority: 1, isActive: true, data: { rateLimitedUntil: '2999-01-01T00:00:00Z' } },
      { id: 'selected', provider: 'github', name: 'Selected account', authType: 'oauth', priority: 7, isActive: true, data: { rateLimitedUntil: '2000-01-01T00:00:00Z' } },
    ]
    await openPage('detail')
    expect(screen.getAllByText(t('providerDetail.coolingBadge'))).toHaveLength(1)
  })
})
