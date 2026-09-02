import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, cleanup } from '@solidjs/testing-library'
import { MemoryRouter, Route } from '@solidjs/router'
import { useGatewayStore } from '@/stores/gateway'
import Providers from '@/pages/Providers'

vi.mock('@/lib/api', () => ({
  api: vi.fn(), apiPost: vi.fn(), apiPut: vi.fn(), apiPatch: vi.fn(), apiDelete: vi.fn(),
}))
vi.mock('@/lib/toast', () => ({
  useToast: () => ({ success: vi.fn(), error: vi.fn(), info: vi.fn() }),
}))

import { api } from '@/lib/api'

const tick = () => new Promise(r => setTimeout(r, 30))

describe('store 单例（根因 #1 回归测试）', () => {
  it('多次调用返回同一实例', () => {
    expect(useGatewayStore()).toBe(useGatewayStore())
  })

  it('状态在实例间共享', async () => {
    ;(api as any).mockResolvedValue(null)
    const a = useGatewayStore()
    const b = useGatewayStore()
    await a.loadCore()
    expect(b.providers()).toEqual(a.providers())
  })
})

describe('Providers 页面渲染', () => {
  afterEach(() => cleanup())

  it('空状态显示引导与一键启用', async () => {
    ;(api as any).mockResolvedValue(null)
    await useGatewayStore().loadCore()

    render(() => (
      <MemoryRouter>
        <Route path="/" component={Providers} />
      </MemoryRouter>
    ))
    await tick()

    const text = document.body.textContent || ''
    expect(text).toContain('提供商')
    expect(text).toContain('还没有接入任何提供商连接')
    expect(text).toContain('一键启用全部免费渠道')
  })

  it('有数据时渲染卡片、凭证状态与操作', async () => {
    ;(api as any).mockImplementation((path: string) => {
      if (path === '/api/providers') {
        return Promise.resolve([
          { id: 'p1', provider: 'anthropic', name: '主力 Claude', authType: 'api-key', priority: 0, isActive: true, data: { hasApiKey: true, credentialHint: '...key' } },
          { id: 'p2', provider: 'openai', name: '', authType: 'api-key', priority: 5, isActive: false, data: { hasApiKey: false } },
        ])
      }
      return Promise.resolve(null)
    })
    await useGatewayStore().loadCore()

    render(() => (
      <MemoryRouter>
        <Route path="/" component={Providers} />
      </MemoryRouter>
    ))
    await tick()

    const text = document.body.textContent || ''
    expect(text).toContain('主力 Claude')   // 自定义名称
    expect(text).toContain('已配置凭证')     // p1 hasApiKey
    expect(text).toContain('缺凭证')        // p2 无 key
    expect(text).toContain('测试')
    expect(text).toContain('我的连接 (2)')
  })
})
