import { describe, it, expect, vi, afterEach } from 'vitest'
import { render, cleanup } from '@solidjs/testing-library'
import { MemoryRouter, Route } from '@solidjs/router'
import { api } from '@/lib/api'
import ProxyPools from '@/pages/ProxyPools'
import CliTools from '@/pages/CliTools'
import Skills from '@/pages/Skills'
import Console from '@/pages/Console'
import Media from '@/pages/Media'
import Quota from '@/pages/Quota'
import Tunnel from '@/pages/Tunnel'
import Mitm from '@/pages/Mitm'

vi.mock('@/lib/api', () => ({
  api: vi.fn(), apiPost: vi.fn(), apiPut: vi.fn(), apiPatch: vi.fn(), apiDelete: vi.fn(),
}))
vi.mock('@/lib/toast', () => ({
  useToast: () => ({ success: vi.fn(), error: vi.fn(), info: vi.fn() }),
}))

const tick = () => new Promise(r => setTimeout(r, 40))

function mount(Comp: any) {
  return render(() => (
    <MemoryRouter>
      <Route path="/" component={Comp} />
    </MemoryRouter>
  ))
}

describe('运维与工具页渲染', () => {
  afterEach(() => { cleanup(); vi.clearAllMocks() })

  it('ProxyPools 渲染代理池列表', async () => {
    ;(api as any).mockImplementation((path: string) => {
      if (path === '/api/proxy-pools') {
        return Promise.resolve({ proxyPools: [{ id: 'x1', name: 'home-proxy', proxyUrl: 'http://127.0.0.1:7890', type: 'http', isActive: true }] })
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

  it('CliTools 渲染工具卡片与接入按钮', async () => {
    ;(api as any).mockResolvedValue({
      tools: [
        { id: 'claude', name: 'Claude Code', description: 'Anthropic CLI', configType: 'env', configured: false },
        { id: 'codex', name: 'OpenAI Codex', description: 'Codex CLI', configType: 'json', configured: true },
      ],
    })
    mount(CliTools)
    await tick()
    const text = document.body.textContent || ''
    expect(text).toContain('CLI 工具接入')
    expect(text).toContain('Claude Code')
    expect(text).toContain('接入')
    expect(text).toContain('已接入')
  })

  it('Skills 渲染技能列表与搜索', async () => {
    ;(api as any).mockResolvedValue({
      count: 2,
      skills: [
        { id: 's1', name: 'cyrene-chat', description: 'Chat capability' },
        { id: 's2', name: 'cyrene-search', description: 'Web search' },
      ],
    })
    mount(Skills)
    await tick()
    const text = document.body.textContent || ''
    expect(text).toContain('技能清单')
    expect(text).toContain('cyrene-chat')
    expect(text).toContain('cyrene-search')
  })

  it('Console 渲染模型选择器与输入区', async () => {
    ;(api as any).mockResolvedValue({ data: [{ id: 'anthropic/*' }, { id: 'openai/*' }] })
    mount(Console)
    await tick()
    const text = document.body.textContent || ''
    expect(text).toContain('控制台')
    expect(text).toContain('选择模型')
    expect(text).toContain('发送一条消息开始测试')
  })

  it('Media 渲染能力切换', async () => {
    ;(api as any).mockResolvedValue(null)
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
    ] as any)
    ;(api as any).mockResolvedValue({
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

  it('Tunnel 渲染状态与操作', async () => {
    ;(api as any).mockResolvedValue({
      installed: true, daemonRunning: true, loggedIn: true,
      funnelRunning: false, tunnelUrl: '', platform: 'windows',
    })
    mount(Tunnel)
    await tick()
    const text = document.body.textContent || ''
    expect(text).toContain('内网穿透')
    expect(text).toContain('Tailscale')
    expect(text).toContain('开启 Funnel')
  })

  it('Mitm 在未启用时显示原因说明', async () => {
    ;(api as any).mockResolvedValue({
      enabled: false, running: false,
      reason: 'MITM is disabled. Start the gateway with -mitm to enable (local deployments only).',
    })
    mount(Mitm)
    await tick()
    const text = document.body.textContent || ''
    expect(text).toContain('MITM 调试代理')
    expect(text).toContain('MITM is disabled')
  })
})
