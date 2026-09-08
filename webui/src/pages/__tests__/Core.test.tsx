import { describe, it, expect, vi, afterEach } from 'vitest'
import type { Component } from 'solid-js'
import { render, cleanup } from '@solidjs/testing-library'
import { MemoryRouter, Route } from '@solidjs/router'
import { useGatewayStore } from '@/stores/gateway'
import Combos from '@/pages/Combos'
import Usage from '@/pages/Usage'
import Settings from '@/pages/Settings'

vi.mock('@/lib/api', () => ({
  api: vi.fn(), apiPost: vi.fn(), apiPut: vi.fn(), apiPatch: vi.fn(), apiDelete: vi.fn(),
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

import { api } from '@/lib/api'

const tick = () => new Promise(r => setTimeout(r, 30))

function mount(Comp: Component) {
  return render(() => (
    <MemoryRouter>
      <Route path="/" component={Comp} />
    </MemoryRouter>
  ))
}

describe('Combos 页', () => {
  afterEach(() => cleanup())

  it('空状态显示引导', async () => {
    vi.mocked(api).mockResolvedValue(null)
    await useGatewayStore().loadCore()
    mount(Combos)
    await tick()
    expect(document.body.textContent).toContain('模型组合')
    expect(document.body.textContent).toContain('还没有组合')
  })

  it('渲染已有组合及其模型与策略', async () => {
    vi.mocked(api).mockImplementation((path: string) => {
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
    vi.mocked(api).mockImplementation((path: string) => {
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
  it('渲染全部设置项含访问控制、响应精确缓存与令牌节省引擎', async () => {
    vi.mocked(api).mockImplementation((path: string) => {
      if (path === '/api/settings') {
        return Promise.resolve({
          requireLogin: false, requireApiKey: true, apiKeyRpm: 5,
          comboStrategy: 'fallback', rtkEnabled: true, cavemanEnabled: true, cavemanLevel: 'lite',
          ponytailEnabled: true, ponytailLevel: 'lite',
          responseCacheEnabled: true, responseCacheTTL: 3600, responseCacheAll: false,
          tokenSaverExclude: ['deepseek'],
        })
      }
      if (path === '/api/cache/stats') {
        return Promise.resolve({
          hits: 42, misses: 8, hitRate: 0.84, entries: 50, maxEntries: 1000, bytesUsed: 102400, tokensSaved: 12500,
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
    expect(text).toContain('响应精确缓存')
    expect(text).toContain('缓存命中率')
    expect(text).toContain('84.0%')
    expect(text).toContain('累计节省 Token')
    expect(text).toContain('12,500')
    expect(text).toContain('令牌节省引擎')
    expect(text).toContain('RTK 压缩')
    expect(text).toContain('Caveman 极简表达')
    expect(text).toContain('Ponytail 极简代码')
    expect(text).toContain('排除提供商名单')
    expect(text).toContain('deepseek')
    expect(text).toContain('全部已保存')
  })

  it('支持在设置页中交互式添加与移除排除提供商并触发保存状态变更', async () => {
    vi.mocked(api).mockImplementation((path: string) => {
      if (path === '/api/settings') {
        return Promise.resolve({
          requireLogin: false, requireApiKey: true, apiKeyRpm: 5,
          rtkEnabled: true,
          tokenSaverExclude: ['deepseek'],
        })
      }
      return Promise.resolve(null)
    })

    await useGatewayStore().loadSettings()
    mount(Settings)
    await tick()

    expect(document.body.textContent).toContain('deepseek')
    expect(document.body.textContent).toContain('全部已保存')

    // 输入并添加 ollama
    const input = document.body.querySelector('input[placeholder*="deepseek"]') as HTMLInputElement
    expect(input).toBeTruthy()
    input.value = 'ollama'
    input.dispatchEvent(new Event('input', { bubbles: true }))
    await tick()

    const addBtn = Array.from(document.body.querySelectorAll('button')).find(b => b.textContent?.trim() === '添加')
    expect(addBtn).toBeTruthy()
    addBtn!.click()
    await tick()

    expect(document.body.textContent).toContain('ollama')
    // 脏检查生效，按钮变为 保存修改
    expect(document.body.textContent).toContain('保存修改')

    // 点击移除 deepseek
    const removeBtns = Array.from(document.body.querySelectorAll('button[title="移除排除"]'))
    expect(removeBtns.length).toBeGreaterThanOrEqual(1)
    const deepseekRemoveBtn = removeBtns[0] as HTMLButtonElement
    deepseekRemoveBtn.click()
    await tick()

    // deepseek chip 已被移除
    const chips = Array.from(document.body.querySelectorAll('span.font-mono > span')).map(el => el.textContent?.trim())
    expect(chips).toContain('ollama')
    expect(chips).not.toContain('deepseek')
  })
})
