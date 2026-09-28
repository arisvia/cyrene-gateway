import { describe, it, expect, vi, beforeEach } from 'vitest'
import { api } from '@/lib/api'
import { fetchModels, reloadModels, fetchModelDisplayNameMap } from '@/lib/models'

vi.mock('@/lib/api', () => ({ api: vi.fn() }))

const table = {
  data: [
    { id: 'antigravity/gemini-2.5-flash', object: 'model', owned_by: 'antigravity', display_name: 'Gemini 2.5 Flash' },
    { id: 'opencode/big-pickle', object: 'model', owned_by: 'opencode' },
  ],
}

function callCount(): number {
  return vi.mocked(api).mock.calls.length
}

describe('lib/models 统一模型表入口', () => {
  beforeEach(async () => {
    vi.mocked(api).mockReset()
    vi.mocked(api).mockResolvedValue(table)
    // 同步模块级缓存，避免用例之间互相污染
    await reloadModels()
  })

  it('读取复用缓存，不重复请求', async () => {
    const before = callCount()
    const first = await fetchModels()
    const second = await fetchModels()
    expect(callCount()).toBe(before)
    expect(second).toEqual(first)
    expect(first.map(m => m.id)).toEqual(['antigravity/gemini-2.5-flash', 'opencode/big-pickle'])
  })

  it('reloadModels 强制刷新并回填缓存', async () => {
    const before = callCount()
    const refreshed = await reloadModels()
    expect(callCount()).toBe(before + 1)
    expect(refreshed).toHaveLength(2)
    expect(await fetchModels()).toEqual(refreshed)
  })

  it('展示名映射缺失名称时回退原始 ID', async () => {
    const map = await fetchModelDisplayNameMap()
    expect(map['antigravity/gemini-2.5-flash']).toBe('Gemini 2.5 Flash')
    expect(map['opencode/big-pickle']).toBe('opencode/big-pickle')
  })

  it('上游失败返回空结果，且失败不驻留缓存', async () => {
    vi.mocked(api).mockRejectedValueOnce(new Error('boom'))
    expect(await reloadModels()).toEqual([])
    // 失败没有把空表钉进缓存：本次读取重新发起请求并成功
    const map = await fetchModelDisplayNameMap()
    expect(Object.keys(map)).toHaveLength(2)
  })

  it('上游返回非预期结构时按空表处理', async () => {
    vi.mocked(api).mockResolvedValue({ unexpected: true } as never)
    expect(await reloadModels()).toEqual([])
  })
})
