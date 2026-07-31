import { describe, it, expect, vi, beforeEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { useGatewayStore } from '@/stores/gateway'

// Mock the api module
vi.mock('@/lib/api', () => ({
  api: vi.fn(),
  apiPost: vi.fn(),
  apiPut: vi.fn(),
  apiPatch: vi.fn(),
  apiDelete: vi.fn(),
}))

// Mock toast
vi.mock('@/lib/toast', () => ({
  useToast: () => ({ success: vi.fn(), error: vi.fn() }),
}))

import { api, apiPost } from '@/lib/api'

describe('gateway store', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
  })

  it('initializes with empty arrays', () => {
    const store = useGatewayStore()
    expect(store.providers).toEqual([])
    expect(store.combos).toEqual([])
    expect(store.apiKeys).toEqual([])
    expect(store.registryCategories).toEqual([])
  })

  it('loadCore handles null responses gracefully', async () => {
    ;(api as any).mockResolvedValue(null)
    const store = useGatewayStore()
    await store.loadCore()
    // Should not throw, arrays stay empty
    expect(store.providers).toEqual([])
    expect(store.combos).toEqual([])
  })

  it('loadCore populates state from API responses', async () => {
    const mockProviders = [{ id: '1', provider: 'openai', isActive: true }]
    const mockRegistry = { categories: [{ category: 'apikey', count: 1, providers: [] }] }

    ;(api as any)
      .mockResolvedValueOnce({ version: '1.0.0' })  // /api/version
      .mockResolvedValueOnce({})                     // /api/health
      .mockResolvedValueOnce(mockProviders)          // /api/providers
      .mockResolvedValueOnce([])                     // /api/combos
      .mockResolvedValueOnce(mockRegistry)           // /api/registry
      .mockResolvedValueOnce({ endpoints: [] })      // /api/endpoints
      .mockResolvedValueOnce({})                     // /api/models/alias
      .mockResolvedValueOnce([])                     // /api/keys (loadKeys)
      .mockResolvedValueOnce({ proxyPools: [] })     // /api/proxy-pools (loadProxyPools)

    const store = useGatewayStore()
    await store.loadCore()

    expect(store.version).toBe('1.0.0')
    expect(store.providers).toEqual(mockProviders)
    expect(store.registryCategories).toEqual(mockRegistry.categories)
  })

  it('activeConnections counts active providers', async () => {
    ;(api as any).mockResolvedValue(null)
    const store = useGatewayStore()
    store.providers = [
      { id: '1', provider: 'a', authType: 'api-key', priority: 0, isActive: true },
      { id: '2', provider: 'b', authType: 'api-key', priority: 0, isActive: false },
      { id: '3', provider: 'c', authType: 'api-key', priority: 0, isActive: true },
    ]
    expect(store.activeConnections).toBe(2)
  })

  it('registryList flattens and sorts categories', () => {
    ;(api as any).mockResolvedValue(null)
    const store = useGatewayStore()
    store.registryCategories = [
      { category: 'apikey', count: 2, providers: [
        { id: 'z', name: 'Zeta', category: 'apikey', authType: 'api-key' },
        { id: 'a', name: 'Alpha', category: 'apikey', authType: 'api-key' },
      ]},
    ]
    expect(store.registryList[0].name).toBe('Alpha')
    expect(store.registryList[1].name).toBe('Zeta')
  })
})
