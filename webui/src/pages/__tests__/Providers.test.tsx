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
    vi.mocked(api).mockResolvedValue(null)
    const a = useGatewayStore()
    const b = useGatewayStore()
    await a.loadCore()
    expect(b.providers()).toEqual(a.providers())
  })
})

describe('Providers 页面渲染', () => {
  afterEach(() => cleanup())

  it('空状态显示引导与一键启用', async () => {
    vi.mocked(api).mockResolvedValue(null)
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

  it('有数据时渲染卡片、凭证状态与能力标签', async () => {
    vi.mocked(api).mockImplementation((path: string) => {
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
    expect(text).toContain('全部启用')
    expect(text).toContain('全部停用')
    expect(text).toContain('管理 →')
    expect(text).toContain('我的连接 (2)')
    expect(text).toContain('LLM 对话')      // 默认能力徽章
  })

  it('纯媒体提供商（无 llm 能力）不展示在我的连接（LLM 对话列表）中', async () => {
    vi.mocked(api).mockImplementation((path: string) => {
      if (path === '/api/providers') {
        return Promise.resolve([
          { id: 'p1', provider: 'elevenlabs', name: 'ElevenLabs Voice', authType: 'api-key', priority: 0, isActive: true, data: { hasApiKey: true } },
        ])
      }
      if (path === '/api/registry') {
        return Promise.resolve({
          categories: [
            {
              category: 'media',
              count: 1,
              providers: [
                { id: 'elevenlabs', name: 'ElevenLabs', capabilities: ['tts'] },
              ],
            },
          ],
          total: 1,
        })
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
    // 我的连接过滤掉了没有 llm 能力的 elevenlabs，应展示空状态
    expect(text).toContain('还没有接入任何提供商连接')
  })
})
