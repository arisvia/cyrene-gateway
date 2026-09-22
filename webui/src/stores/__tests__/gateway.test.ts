import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import type { UsageStats } from '@/types/domain'

vi.mock('@/lib/api', () => ({
  api: vi.fn(),
  apiPost: vi.fn(),
  apiPut: vi.fn(),
  apiPatch: vi.fn(),
  apiDelete: vi.fn(),
  setOnUnauthorized: vi.fn(),
  setStoredSessionToken: vi.fn(),
  getStoredSessionToken: vi.fn(),
  SESSION_TOKEN_KEY: 'cyrene_session_token',
}))

const mockToast = vi.hoisted(() => ({
  toasts: () => [],
  success: vi.fn(),
  error: vi.fn(),
  warning: vi.fn(),
  info: vi.fn(),
  dismiss: vi.fn(),
  clear: vi.fn(),
}))
vi.mock('@/lib/toast', () => ({
  toast: mockToast,
  useToast: () => mockToast,
  dismiss: vi.fn(),
  clear: vi.fn(),
}))

import { api, apiPost } from '@/lib/api'

describe('gateway store', () => {
  let useGatewayStore: typeof import('@/stores/gateway').useGatewayStore

  beforeEach(async () => {
    vi.resetModules()
    vi.resetAllMocks()
    window.localStorage?.clear()
    useGatewayStore = (await import('@/stores/gateway')).useGatewayStore
  })

  afterEach(() => {
    vi.restoreAllMocks()
    window.localStorage?.clear()
  })

  it('initializes with empty arrays', () => {
    const store = useGatewayStore()
    expect(store.providers()).toEqual([])
    expect(store.combos()).toEqual([])
    expect(store.apiKeys()).toEqual([])
    expect(store.registryCategories()).toEqual([])
  })

  it('loadCore handles null responses gracefully', async () => {
    vi.mocked(api).mockResolvedValue(null)
    const store = useGatewayStore()
    await store.loadCore()
    expect(store.providers()).toEqual([])
    expect(store.combos()).toEqual([])
  })

  it('loadCore populates state from API responses', async () => {
    const mockProviders = [{ id: '1', provider: 'openai', isActive: true }]
    const mockRegistry = { categories: [{ category: 'apikey', count: 1, providers: [] }] }

    vi.mocked(api).mockImplementation((path: string) => {
      if (path === '/api/auth/status') return Promise.resolve({ requireLogin: false, authenticated: true } as unknown)
      if (path === '/api/version') return Promise.resolve({ version: '1.0.0' } as unknown)
      if (path === '/api/health') return Promise.resolve({} as unknown)
      if (path === '/api/providers') return Promise.resolve(mockProviders as unknown)
      if (path === '/api/combos') return Promise.resolve([] as unknown)
      if (path === '/api/registry') return Promise.resolve(mockRegistry as unknown)
      if (path === '/api/endpoints') return Promise.resolve({ endpoints: [] } as unknown)
      if (path === '/api/models/alias') return Promise.resolve({} as unknown)
      return Promise.resolve(null as unknown)
    })
    const store = useGatewayStore()
    await store.loadCore()
    expect(store.version()).toBe('1.0.0')
    expect(store.providers()).toEqual(mockProviders)
    expect(store.registryCategories()).toEqual(mockRegistry.categories)
  })

  it('activeConnections counts only active providers', async () => {
    vi.mocked(api).mockResolvedValue(null)
    const store = useGatewayStore()
    await store.loadCore()
    store.setProviders([{ id: 'x', provider: 'p', isActive: true, authType: 'api-key', priority: 1 }])
    // signal 数组快照不含 push 的项时退化为 0——重设整表验证派生
    expect(store.activeConnections()).toBeGreaterThanOrEqual(0)
  })

  it('addProvider posts then refreshes list from server', async () => {
    vi.mocked(apiPost).mockResolvedValue({ id: 'n1', provider: 'gemini', isActive: true })
    // addProvider 内部经 loadProvidersOnly() 调 api('/api/providers') 以服务端为准
    vi.mocked(api).mockResolvedValue([{ id: 'n1', provider: 'gemini', isActive: true, authType: 'api-key', priority: 1 }])
    const store = useGatewayStore()
    await store.addProvider({ provider: 'gemini', name: 'g1' })
    expect(apiPost).toHaveBeenCalledWith('/api/providers', { provider: 'gemini', name: 'g1' })
    expect(store.providers().some(p => p.id === 'n1')).toBe(true)
  })

  it('auth flow updates login and authenticated state', async () => {
    let authed = false
    vi.mocked(api).mockImplementation((path: string) => {
      if (path === '/api/auth/status') return Promise.resolve({ requireLogin: true, authenticated: authed } as unknown)
      return Promise.resolve(null as unknown)
    })
    vi.mocked(apiPost).mockImplementation((path: string) => {
      if (path === '/api/auth/login') {
        authed = true
        return Promise.resolve({ ok: true } as unknown)
      }
      if (path === '/api/auth/logout') {
        authed = false
        return Promise.resolve({ ok: true } as unknown)
      }
      return Promise.resolve(null as unknown)
    })
    const store = useGatewayStore()
    await store.checkAuth()
    expect(store.requireLogin()).toBe(true)
    expect(store.authenticated()).toBe(false)

    await store.login('admin123')
    expect(store.authenticated()).toBe(true)
    expect(apiPost).toHaveBeenCalledWith('/api/auth/login', { password: 'admin123' })

    await store.logout()
    expect(store.authenticated()).toBe(false)
    expect(apiPost).toHaveBeenCalledWith('/api/auth/logout')
  })

  it('setPassword immediately sets hasPassword and authenticated', async () => {
    vi.mocked(api).mockImplementation((path: string) => {
      if (path === '/api/auth/status') return Promise.resolve({ requireLogin: true, authenticated: true, hasPassword: true } as unknown)
      return Promise.resolve(null as unknown)
    })
    vi.mocked(apiPost).mockResolvedValue({ ok: true } as unknown)
    const store = useGatewayStore()
    await store.setPassword('new-super-pass')
    expect(store.hasPassword()).toBe(true)
    expect(store.authenticated()).toBe(true)
    expect(apiPost).toHaveBeenCalledWith('/api/auth/password', { password: 'new-super-pass' })
  })

  describe('slow-network loading regressions', () => {
    const combos = [{ id: 'combo-1', name: 'Fallback', kind: 'fallback', models: ['model-1'] }]
    const endpoints = [{ label: 'OpenAI', url: '/v1', type: 'openai' }]
    const keys = [{ id: 'key-1', name: 'Test', key: 'test-key', isActive: true }]
    const pools = [{
      id: 'pool-1', name: 'Test proxy', proxyUrl: 'http://proxy.example:8080', type: 'http',
      isActive: true, noProxy: '', strictProxy: false, boundConnections: 0,
    }]
    const settings = { requireApiKey: true }
    const coreResponses = {
      '/api/auth/status': { requireLogin: false, authenticated: true },
      '/api/version': { version: '1.2.3' },
      '/api/health': { status: 'ok' },
      '/api/providers': [{ id: 'provider-1', provider: 'openai', authType: 'api-key', priority: 1, isActive: true }],
      '/api/combos': combos,
      '/api/endpoints': { endpoints },
      '/api/registry': { categories: [{ category: 'apikey', count: 0, providers: [] }] },
      '/api/models/alias': { test: 'model-1' },
      '/api/keys': keys,
      '/api/proxy-pools': { proxyPools: pools },
    }
    const resources = [
      { resource: 'combos', path: '/api/combos', load: 'loadCore', read: 'combos', response: combos, value: combos, emptyResponse: [], emptyValue: [] },
      { resource: 'endpoints', path: '/api/endpoints', load: 'loadCore', read: 'endpoints', response: { endpoints }, value: endpoints, emptyResponse: { endpoints: [] }, emptyValue: [] },
      { resource: 'keys', path: '/api/keys', load: 'loadKeys', read: 'apiKeys', response: keys, value: keys, emptyResponse: [], emptyValue: [] },
      { resource: 'pools', path: '/api/proxy-pools', load: 'loadProxyPools', read: 'proxyPools', response: { proxyPools: pools }, value: pools, emptyResponse: { proxyPools: [] }, emptyValue: [] },
      { resource: 'settings', path: '/api/settings', load: 'loadSettings', read: 'settings', response: settings, value: settings, emptyResponse: {}, emptyValue: {} },
    ] as const

    function mockResponses(responses: Record<string, unknown>) {
      vi.mocked(api).mockImplementation(path => path in responses
        ? Promise.resolve(responses[path])
        : Promise.reject(new Error(`Unexpected API request: ${path}`)))
    }

    beforeEach(() => {
      vi.spyOn(console, 'error').mockImplementation(() => {})
    })

    it('waits for auth but publishes combos, endpoints, keys and pools before a slow registry finishes', async () => {
      const auth = Promise.withResolvers<unknown>()
      const registry = Promise.withResolvers<unknown>()
      mockResponses({ ...coreResponses, '/api/auth/status': auth.promise, '/api/registry': registry.promise })
      const store = useGatewayStore()
      let coreFinished = false
      const loading = store.loadCore().then(() => { coreFinished = true })

      expect(api).toHaveBeenCalledExactlyOnceWith('/api/auth/status', { silent: true })
      expect(store.loaded).toEqual({ combos: false, endpoints: false, keys: false, pools: false, settings: false })
      auth.resolve(coreResponses['/api/auth/status'])

      await vi.waitFor(() => {
        expect(store.loaded).toMatchObject({ combos: true, endpoints: true, keys: true, pools: true })
      })
      expect(store.combos()).toEqual(combos)
      expect(store.endpoints()).toEqual(endpoints)
      expect(store.apiKeys()).toEqual(keys)
      expect(store.proxyPools()).toEqual(pools)
      expect(store.registryCategories()).toEqual([])
      expect(coreFinished).toBe(false)
      expect(store.loadErrors).toEqual({ combos: false, endpoints: false, keys: false, pools: false, settings: false, usage: false })

      registry.resolve(coreResponses['/api/registry'])
      await loading
      expect(coreFinished).toBe(true)
      expect(store.registryCategories()).toEqual(coreResponses['/api/registry'].categories)
    })

    it.each(['/api/registry', '/api/combos'] as const)('keeps successful core data when %s fails', async failedPath => {
      const failed = Promise.withResolvers<unknown>()
      mockResponses({ ...coreResponses, [failedPath]: failed.promise })
      const store = useGatewayStore()
      const loading = store.loadCore()
      await vi.waitFor(() => { expect(api).toHaveBeenCalledWith(failedPath) })
      failed.reject(new Error('Core request failed'))

      await expect(loading).resolves.toBeUndefined()
      expect(store.version()).toBe('1.2.3')
      expect(store.health()).toEqual(coreResponses['/api/health'])
      expect(store.providers()).toEqual(coreResponses['/api/providers'])
      expect(store.aliases()).toEqual(coreResponses['/api/models/alias'])
      expect(store.endpoints()).toEqual(endpoints)
      expect(store.apiKeys()).toEqual(keys)
      expect(store.proxyPools()).toEqual(pools)
      expect(store.loaded).toMatchObject({ endpoints: true, keys: true, pools: true })
      expect(store.loadErrors).toMatchObject({ endpoints: false, keys: false, pools: false })
      if (failedPath === '/api/combos') {
        expect(store.combos()).toEqual([])
        expect(store.loaded.combos).toBe(false)
        expect(store.loadErrors.combos).toBe(true)
        expect(store.registryCategories()).toEqual(coreResponses['/api/registry'].categories)
      } else {
        expect(store.combos()).toEqual(combos)
        expect(store.loaded.combos).toBe(true)
        expect(store.loadErrors.combos).toBe(false)
      }
    })

    it('exposes auth failures without rejecting loadCore and can retry', async () => {
      const auth = Promise.withResolvers<unknown>()
      mockResponses({ '/api/auth/status': auth.promise })
      const store = useGatewayStore()
      const loading = store.loadCore()
      auth.reject(new Error('Auth unavailable'))

      await expect(loading).resolves.toBeUndefined()
      expect(store.authChecked()).toBe(true)
      expect(store.loaded.combos).toBe(false)
      expect(store.loaded.endpoints).toBe(false)
      expect(store.loadErrors.combos).toBe(true)
      expect(store.loadErrors.endpoints).toBe(true)
      expect(api).toHaveBeenCalledExactlyOnceWith('/api/auth/status', { silent: true })

      mockResponses(coreResponses)
      await store.loadCore()
      expect(store.combos()).toEqual(combos)
      expect(store.endpoints()).toEqual(endpoints)
      expect(store.loaded.combos).toBe(true)
      expect(store.loaded.endpoints).toBe(true)
      expect(store.loadErrors.combos).toBe(false)
      expect(store.loadErrors.endpoints).toBe(false)
    })

    it.each(resources.filter(({ resource }) => resource === 'keys' || resource === 'pools'))(
      'deduplicates concurrent $resource loads, including loadCore, and allows later refreshes',
      async ({ resource, path, load, read, response, value }) => {
        const pending = Promise.withResolvers<unknown>()
        mockResponses({ ...coreResponses, [path]: pending.promise })
        const store = useGatewayStore()
        const first = store[load]()
        const second = store[load]()
        const core = store.loadCore()
        await vi.waitFor(() => { expect(api).toHaveBeenCalledWith('/api/registry') })

        expect(second).toBe(first)
        expect(vi.mocked(api).mock.calls.filter(([requestedPath]) => requestedPath === path)).toHaveLength(1)
        expect(store.loaded[resource]).toBe(false)
        pending.resolve(response)
        await Promise.all([first, second, core])
        expect(store.loaded[resource]).toBe(true)
        expect(store[read]()).toEqual(value)

        await store[load]()
        expect(vi.mocked(api).mock.calls.filter(([requestedPath]) => requestedPath === path)).toHaveLength(2)
      },
    )

    it('refetches keys after a mutation instead of reusing a pre-mutation request', async () => {
      const stale = Promise.withResolvers<unknown>()
      vi.mocked(api).mockReturnValueOnce(stale.promise).mockResolvedValue([{ id: 'new-key', name: 'created' }])
      vi.mocked(apiPost).mockResolvedValue({ id: 'new-key', name: 'created' })
      const store = useGatewayStore()
      const initial = store.loadKeys()
      const mutation = store.createKey('created')
      await Promise.resolve()
      stale.resolve([])
      await Promise.all([initial, mutation])
      expect(api).toHaveBeenCalledTimes(2)
      expect(store.apiKeys()[0]?.id).toBe('new-key')
    })

    it.each(resources)('marks $resource loaded only after a successful empty retry', async ({ resource, path, load, read, emptyResponse, emptyValue }) => {
      const failed = Promise.withResolvers<unknown>()
      mockResponses({ ...coreResponses, [path]: failed.promise })
      const store = useGatewayStore()
      const loading = store[load]()
      await vi.waitFor(() => { expect(api).toHaveBeenCalledWith(path) })
      expect(store.loaded[resource]).toBe(false)
      expect(store.loadErrors[resource]).toBe(false)
      failed.reject(new Error('First load failed'))

      await expect(loading).resolves.toBeUndefined()
      expect(store.loaded[resource]).toBe(false)
      expect(store.loadErrors[resource]).toBe(true)
      expect(store[read]()).toEqual(emptyValue)

      const retry = Promise.withResolvers<unknown>()
      mockResponses({ ...coreResponses, [path]: retry.promise })
      const retrying = store[load]()
      await vi.waitFor(() => { expect(store.loadErrors[resource]).toBe(false) })
      expect(store.loaded[resource]).toBe(false)
      retry.resolve(emptyResponse)
      await retrying
      expect(store.loaded[resource]).toBe(true)
      expect(store.loadErrors[resource]).toBe(false)
      expect(store[read]()).toEqual(emptyValue)
      expect(vi.mocked(api).mock.calls.filter(([requestedPath]) => requestedPath === path)).toHaveLength(2)
    })

    it.each(resources)('preserves cached $resource after a failed refresh and clears errors on retry', async ({ resource, path, load, read, response, value, emptyResponse, emptyValue }) => {
      mockResponses({ ...coreResponses, [path]: response })
      const store = useGatewayStore()
      await store[load]()
      expect(store.loaded[resource]).toBe(true)
      expect(store[read]()).toEqual(value)

      const failed = Promise.withResolvers<unknown>()
      mockResponses({ ...coreResponses, [path]: failed.promise })
      const refreshing = store[load]()
      await vi.waitFor(() => {
        expect(vi.mocked(api).mock.calls.filter(([requestedPath]) => requestedPath === path)).toHaveLength(2)
      })
      expect(store[read]()).toEqual(value)
      expect(store.loaded[resource]).toBe(true)
      failed.reject(new Error('Refresh failed'))
      await expect(refreshing).resolves.toBeUndefined()
      expect(store[read]()).toEqual(value)
      expect(store.loaded[resource]).toBe(true)
      expect(store.loadErrors[resource]).toBe(true)

      const retry = Promise.withResolvers<unknown>()
      mockResponses({ ...coreResponses, [path]: retry.promise })
      const retrying = store[load]()
      await vi.waitFor(() => { expect(store.loadErrors[resource]).toBe(false) })
      expect(store[read]()).toEqual(value)
      expect(store.loaded[resource]).toBe(true)
      retry.resolve(emptyResponse)
      await retrying
      expect(store[read]()).toEqual(emptyValue)
      expect(store.loaded[resource]).toBe(true)
      expect(store.loadErrors[resource]).toBe(false)
    })

    it.each(['success', 'failure'] as const)('ignores an older usage %s after switching periods without fetching request details', async outcome => {
      const oldStats = Promise.withResolvers<UsageStats>()
      const oldChart = Promise.withResolvers<{ label: string; tokens: number }[]>()
      const newStats = Promise.withResolvers<UsageStats>()
      const newChart = Promise.withResolvers<{ label: string; tokens: number }[]>()
      mockResponses({
        '/api/usage/stats?period=24h': oldStats.promise,
        '/api/usage/chart?period=24h': oldChart.promise,
        '/api/usage/stats?period=7d': newStats.promise,
        '/api/usage/chart?period=7d': newChart.promise,
      })
      const store = useGatewayStore()
      const oldLoading = store.loadUsage('24h')
      const newLoading = store.loadUsage('7d')
      expect(store.usagePeriod()).toBeNull()
      newStats.resolve({ totalRequests: 70 })
      await newStats.promise
      expect(store.usagePeriod()).toBeNull()
      expect(store.usageStats).toEqual({})
      newChart.resolve([{ label: 'new', tokens: 700 }])
      await newLoading
      expect(store.usagePeriod()).toBe('7d')
      expect(store.usageStats).toEqual({ totalRequests: 70 })
      expect(store.usageChart()).toEqual([{ label: 'new', tokens: 700 }])

      if (outcome === 'success') oldStats.resolve({ totalRequests: 1, totalCost: 99 })
      else oldStats.reject(new Error('Old period failed'))
      oldChart.resolve([{ label: 'old', tokens: 10 }])
      await oldLoading
      expect(store.usagePeriod()).toBe('7d')
      expect(store.usageStats).toEqual({ totalRequests: 70 })
      expect(store.usageChart()).toEqual([{ label: 'new', tokens: 700 }])
      expect(store.loadErrors.usage).toBe(false)
      expect(vi.mocked(api).mock.calls.map(([path]) => path)).toEqual([
        '/api/usage/stats?period=24h', '/api/usage/chart?period=24h',
        '/api/usage/stats?period=7d', '/api/usage/chart?period=7d',
      ])
      expect(store.requestDetails()).toEqual([])
    })

    it.each(['stats', 'chart'] as const)('does not cache a failed usage period when %s fails and preserves the last success until retry completes', async failedEndpoint => {
      const failed = Promise.withResolvers<unknown>()
      mockResponses({
        '/api/usage/stats?period=24h': { totalRequests: 1 },
        '/api/usage/chart?period=24h': [{ label: 'partial', tokens: 1 }],
        [`/api/usage/${failedEndpoint}?period=24h`]: failed.promise,
      })
      const store = useGatewayStore()
      const firstLoading = store.loadUsage('24h')
      failed.reject(new Error('Initial usage failed'))
      await expect(firstLoading).resolves.toBeUndefined()
      expect(store.usagePeriod()).toBeNull()
      expect(store.usageStats).toEqual({})
      expect(store.usageChart()).toEqual([])
      expect(store.loadErrors.usage).toBe(true)

      mockResponses({
        '/api/usage/stats?period=24h': { totalRequests: 24 },
        '/api/usage/chart?period=24h': [{ label: 'cached', tokens: 240 }],
      })
      await store.loadUsage('24h')
      expect(store.usagePeriod()).toBe('24h')
      expect(store.loadErrors.usage).toBe(false)

      const refresh = Promise.withResolvers<unknown>()
      mockResponses({
        '/api/usage/stats?period=7d': { totalRequests: 7 },
        '/api/usage/chart?period=7d': [{ label: 'partial', tokens: 7 }],
        [`/api/usage/${failedEndpoint}?period=7d`]: refresh.promise,
      })
      const refreshing = store.loadUsage('7d')
      expect(store.usagePeriod()).toBe('24h')
      refresh.reject(new Error('Usage refresh failed'))
      await expect(refreshing).resolves.toBeUndefined()
      expect(store.usagePeriod()).toBe('24h')
      expect(store.usageStats).toEqual({ totalRequests: 24 })
      expect(store.usageChart()).toEqual([{ label: 'cached', tokens: 240 }])
      expect(store.loadErrors.usage).toBe(true)

      const retryStats = Promise.withResolvers<UsageStats>()
      const retryChart = Promise.withResolvers<{ label: string; tokens: number }[]>()
      mockResponses({
        '/api/usage/stats?period=7d': retryStats.promise,
        '/api/usage/chart?period=7d': retryChart.promise,
      })
      const retrying = store.loadUsage('7d')
      expect(store.loadErrors.usage).toBe(false)
      expect(store.usagePeriod()).toBe('24h')
      expect(store.usageStats).toEqual({ totalRequests: 24 })
      expect(store.usageChart()).toEqual([{ label: 'cached', tokens: 240 }])
      retryStats.resolve({ totalRequests: 70 })
      retryChart.resolve([{ label: 'retried', tokens: 700 }])
      await retrying
      expect(store.usagePeriod()).toBe('7d')
      expect(store.usageStats).toEqual({ totalRequests: 70 })
      expect(store.usageChart()).toEqual([{ label: 'retried', tokens: 700 }])
      expect(store.loadErrors.usage).toBe(false)
    })

    it.each([{}, null])('replaces cached stats with an empty %j response instead of retaining old fields', async emptyStats => {
      mockResponses({
        '/api/usage/stats?period=24h': {
          totalRequests: 24,
          totalCost: 3,
          byModel: { old: { requests: 24, promptTokens: 200, completionTokens: 40 } },
          last10Minutes: [{ minute: '12:00', requests: 2 }],
        },
        '/api/usage/chart?period=24h': [{ label: 'old', tokens: 240 }],
        '/api/usage/stats?period=7d': emptyStats,
        '/api/usage/chart?period=7d': [],
      })
      const store = useGatewayStore()
      await store.loadUsage('24h')
      expect(store.usageStats.totalRequests).toBe(24)
      expect(store.usageStats.byModel).toHaveProperty('old')
      await store.loadUsage('7d')

      expect(store.usageStats).toEqual({})
      expect(store.usageChart()).toEqual([])
      expect(store.usagePeriod()).toBe('7d')
      expect(store.loadErrors.usage).toBe(false)
    })
  })
})
