import { describe, it, expect, vi, afterEach } from 'vitest'
import type { Component } from 'solid-js'
import { render, cleanup } from '@solidjs/testing-library'
import { MemoryRouter, Route } from '@solidjs/router'
import { api } from '@/lib/api'
import ProxyPools from '@/pages/ProxyPools'
import Media from '@/pages/Media'
import Quota from '@/pages/Quota'
import Playground from '@/pages/Playground'

vi.mock('@/lib/api', () => ({
  api: vi.fn(), apiPost: vi.fn(), apiPut: vi.fn(), apiPatch: vi.fn(), apiDelete: vi.fn(), setOnUnauthorized: vi.fn(),
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

const tick = () => new Promise(r => setTimeout(r, 40))

function mount(Comp: Component) {
  return render(() => (
    <MemoryRouter>
      <Route path="/" component={Comp} />
    </MemoryRouter>
  ))
}

describe('运维与工具页渲染', () => {
  afterEach(() => { cleanup(); vi.clearAllMocks() })

  it('ProxyPools 渲染代理池列表', async () => {
    vi.mocked(api).mockImplementation((path: string) => {
      if (path === '/api/proxy-pools') {
        return Promise.resolve({
          proxyPools: [
            {
              id: 'x1',
              isActive: true,
              data: { name: 'home-proxy', proxyUrl: 'http://127.0.0.1:7890', type: 'http', strictProxy: true },
            },
          ],
        })
      }
      return Promise.resolve(null)
    })
    mount(ProxyPools)
    await tick()
    const text = document.body.textContent || ''
    expect(text).toContain('代理池')
    expect(text).toContain('home-proxy')
    expect(text).toContain('http://127.0.0.1:7890')
  })
  it('Media 渲染能力切换', async () => {
    vi.mocked(api).mockResolvedValue(null)
    mount(Media)
    await tick()
    const text = document.body.textContent || ''
    expect(text).toContain('媒体能力')
    expect(text).toContain('图像生成')
    expect(text).toContain('语音合成')
    expect(text).toContain('向量嵌入')
  })

  it('Quota 渲染提供商配额行', async () => {
    const { useGatewayStore } = await import('@/stores/gateway')
    const store = useGatewayStore()
    store.setProviders([
      { id: 'conn-1', provider: 'anthropic', name: 'Anthropic Main', isActive: true, priority: 1, authType: 'api-key' },
    ])
    vi.mocked(api).mockResolvedValue({
      period: '7d',
      providers: [{ provider: 'anthropic', requests: 10, promptTokens: 100, completionTokens: 50, cost: 0.1, connections: 2, activeConnections: 1 }],
      quotas: {
        user: { used: 10, total: 100, remaining: 90, remainingPercentage: 90, resetAt: '2026-10-01T00:00:00Z', unit: 'USD' }
      }
    })
    mount(Quota)
    await tick()
    const text = document.body.textContent || ''
    expect(text).toContain('配额')
    expect(text).toContain('anthropic')
  })

  it('Playground 渲染演练场页面与模式切换', async () => {
    vi.mocked(api).mockImplementation((path: string) => {
      if (path === '/v1/models') {
        return Promise.resolve({
          data: [
            { id: 'antigravity/gemini-2.5-flash', object: 'model', owned_by: 'antigravity', display_name: 'Gemini 2.5 Flash' },
            { id: 'opencode/big-pickle', object: 'model', owned_by: 'opencode', display_name: 'Big Pickle' },
          ]
        })
      }
      return Promise.resolve(null)
    })
    mount(Playground)
    await tick()
    const text = document.body.textContent || ''
    expect(text).toContain('演练场')
    expect(text).toContain('单模型测试')
    expect(text).toContain('双模型对比')
    expect(text).toContain('导出代码')
    expect(text).toContain('参数面板')
  })
})
