import { describe, it, expect, beforeEach } from 'vitest'
import { usePlaygroundStore, type Turn } from '@/stores/playground'

const turn = (id: string, mode: 'single' | 'compare'): Turn => ({
  id,
  user: `prompt ${id}`,
  mode,
  a: { targetModel: 'openai/gpt-test', content: 'answer', busy: false },
})

describe('playground 会话 store', () => {
  beforeEach(() => {
    const store = usePlaygroundStore()
    store.setTurnsAll([])
  })

  it('是路由外的单例：离开演练场再回来仍持有同一段会话', () => {
    const first = usePlaygroundStore()
    first.setTurns(prev => [...prev, turn('a', 'single')])
    expect(usePlaygroundStore().turns().map(t => t.id)).toEqual(['a'])
  })

  it('会话按模式隔离，切换模式不串流', () => {
    const store = usePlaygroundStore()
    store.setTurnsAll([turn('s1', 'single'), turn('c1', 'compare')])
    store.setMode('single')
    expect(store.turns().map(t => t.id)).toEqual(['s1'])
    store.setMode('compare')
    expect(store.turns().map(t => t.id)).toEqual(['c1'])
  })

  it('清空只作用于当前模式', () => {
    const store = usePlaygroundStore()
    store.setMode('single')
    store.setTurnsAll([turn('s1', 'single'), turn('c1', 'compare')])
    store.clearTurns()
    expect(store.turnsAll()).toEqual([turn('c1', 'compare')])
  })

})
