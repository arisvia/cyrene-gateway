import { describe, it, expect, vi } from 'vitest'
import { render, cleanup } from '@solidjs/testing-library'
import App from '@/App'

vi.mock('@/lib/api', () => ({
  api: vi.fn(() => Promise.resolve(null)),
  apiPost: vi.fn(() => Promise.resolve(null)),
  apiPut: vi.fn(() => Promise.resolve(null)),
  apiPatch: vi.fn(() => Promise.resolve(null)),
  apiDelete: vi.fn(() => Promise.resolve(null)),
}))
vi.mock('@/lib/toast', () => ({
  useToast: () => ({ toasts: () => [], success: vi.fn(), error: vi.fn(), info: vi.fn() }),
}))

// 回归：此前侧栏 <A> 位于 <Route> 之外 → 整页抛错白屏
describe('App 根组件（白屏回归）', () => {
  it('应用能完整挂载且侧栏 <A> 不抛路由错误', async () => {
    const errors: string[] = []
    const origError = console.error
    console.error = (...args: any[]) => { errors.push(args.join(' ')); origError(...args) }

    let container: HTMLElement | undefined
    expect(() => { container = render(() => <App />).container }).not.toThrow()
    await new Promise(r => setTimeout(r, 60))

    console.error = origError

    const text = document.body.textContent || ''
    // 未抛 "can be only used inside a Route"
    expect(errors.join(' ')).not.toContain('inside a Route')
    // 侧栏真实渲染
    expect(text).toContain('Cyrene Gateway')
    expect(text).toContain('提供商')
    expect(text).toContain('用量')
    expect(document.body.querySelector('a[href*="settings"]')).toBeTruthy()
    expect(text).toContain('运行中')
    expect(container).toBeTruthy()
    cleanup()
  })
})
