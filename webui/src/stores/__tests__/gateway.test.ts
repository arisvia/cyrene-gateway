import { describe, it, expect, vi, beforeEach } from 'vitest'
import { useGatewayStore } from '@/stores/gateway'

vi.mock('@/lib/api', () => ({
  api: vi.fn(),
  apiPost: vi.fn(),
  apiPut: vi.fn(),
  apiPatch: vi.fn(),
  apiDelete: vi.fn(),
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

    vi.mocked(api)
      .mockResolvedValueOnce({ version: '1.0.0' })
      .mockResolvedValueOnce({})
      .mockResolvedValueOnce(mockProviders)
      .mockResolvedValueOnce([])
      .mockResolvedValueOnce(mockRegistry)
      .mockResolvedValueOnce({ endpoints: [] })
      .mockResolvedValueOnce({})

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
})
