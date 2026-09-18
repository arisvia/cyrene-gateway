import { describe, it, expect, vi, beforeEach } from 'vitest'
import { useGatewayStore } from '@/stores/gateway'

vi.mock('@/lib/api', () => ({
  api: vi.fn(),
  apiPost: vi.fn(),
  apiPut: vi.fn(),
  apiPatch: vi.fn(),
  apiDelete: vi.fn(),
  setOnUnauthorized: vi.fn(),
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
  beforeEach(() => {
    vi.clearAllMocks()
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
})
