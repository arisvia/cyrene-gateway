import { describe, it, expect, vi, afterEach } from 'vitest'
import { render, cleanup } from '@solidjs/testing-library'
import { MemoryRouter, Route } from '@solidjs/router'
import { useGatewayStore } from '@/stores/gateway'
import Combos from '@/pages/Combos'
import Usage from '@/pages/Usage'
import Settings from '@/pages/Settings'

vi.mock('@/lib/api', () => ({
  api: vi.fn(), apiPost: vi.fn(), apiPut: vi.fn(), apiPatch: vi.fn(), apiDelete: vi.fn(),
}))
vi.mock('@/lib/toast', () => ({
  useToast: () => ({ success: vi.fn(), error: vi.fn(), info: vi.fn() }),
}))

import { api } from '@/lib/api'

const tick = () => new Promise(r => setTimeout(r, 30))

function mount(Comp: any) {
  return render(() => (
    <MemoryRouter>
      <Route path="/" component={Comp} />
    </MemoryRouter>
  ))
}

describe('Combos 页', () => {
  afterEach(() => cleanup())

  it('空状态显示引导', async () => {
    ;(api as any).mockResolvedValue(null)
    await useGatewayStore().loadCore()
    mount(Combos)
    await tick()
    expect(document.body.textContent).toContain('模型组合')
    expect(document.body.textContent).toContain('还没有组合')
  })

  it('渲染已有组合及其模型与策略', async () => {
    ;(api as any).mockImplementation((path: string) => {
      if (path === '/api/combos') {
        return Promise.resolve([
          { id: 'c1', name: 'fast-coding', kind: 'fallback', models: ['anthropic/*', 'openai/*'] },
        ])
      }
      return Promise.resolve({ data: [] })
    })
    await useGatewayStore().loadCore()
    mount(Combos)
    await tick()
    const text = document.body.textContent || ''
    expect(text).toContain('fast-coding')
    expect(text).toContain('故障回退')
    expect(text).toContain('2 个模型')
    expect(text).toContain('anthropic/*')
  })
})

describe('Usage 页', () => {
  afterEach(() => cleanup())

  it('渲染 KPI 与图表', async () => {
    ;(api as any).mockImplementation((path: string) => {
      if (path.includes('/api/usage/stats')) {
        return Promise.resolve({
          totalRequests: 1234, totalPromptTokens: 50000, totalCompletionTokens: 20000,
          totalCost: 1.5, totalRequestsLifetime: 9999,
          byProvider: { anthropic: { requests: 100, promptTokens: 1, completionTokens: 1 } },
        })
      }
      if (path.includes('/api/usage/chart')) {
        return Promise.resolve([
          { label: 'Mon', tokens: 100 }, { label: 'Tue', tokens: 300 },
        ])
      }
      if (path.includes('/api/usage/request-details')) {
        return Promise.resolve({
          details: [{ timestamp: new Date().toISOString(), model: 'claude-x', status: 'ok', promptTokens: 10, completionTokens: 20, latencyMs: 120 }],
          pagination: { page: 1, pageSize: 20, totalItems: 1, totalPages: 1, hasNext: false, hasPrev: false },
        })
      }
      return Promise.resolve(null)
    })
    await useGatewayStore().loadUsage('7d')
    await useGatewayStore().loadRequestDetails(1, 20)
    mount(Usage)
    await tick()
    let text = document.body.textContent || ''
    expect(text).toContain('用量统计')
    expect(text).toContain('总请求')
    expect(text).toContain('估算成本')
    expect(text).toContain('Token 趋势')
    expect(text).toContain('请求明细')
    // 切换到详情 Tab
    const detailsBtn = document.querySelectorAll('button')
    for (const btn of detailsBtn) {
      if (btn.textContent?.includes('请求明细')) {
        btn.click()
        break
      }
    }
    await tick()
    text = document.body.textContent || ''
    expect(text).toContain('claude-x')
  })
})

describe('Settings 页', () => {
  afterEach(() => cleanup())

  it('渲染全部设置项含 apiKeyRpm', async () => {
    ;(api as any).mockImplementation((path: string) => {
      if (path === '/api/settings') {
        return Promise.resolve({
          requireLogin: false, requireApiKey: true, apiKeyRpm: 5,
          comboStrategy: 'fallback', rtkEnabled: false, cavemanEnabled: false, ponytailEnabled: false,
        })
      }
      return Promise.resolve(null)
    })
    await useGatewayStore().loadSettings()
    mount(Settings)
    await tick()
    const text = document.body.textContent || ''
    expect(text).toContain('访问控制')
    expect(text).toContain('要求 API Key')
    expect(text).toContain('API Key 速率限制')
    expect(text).toContain('Token 节省')
    expect(text).toContain('调度策略')
  })
})
