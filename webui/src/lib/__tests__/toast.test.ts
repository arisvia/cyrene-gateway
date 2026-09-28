import { describe, it, expect, beforeEach, vi, afterEach } from 'vitest'
import { createComputed, createRoot } from 'solid-js'
import { toast, clearToasts, dismiss } from '../toast'

describe('toast store', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    clearToasts()
  })

  afterEach(() => {
    clearToasts()
    vi.useRealTimers()
  })

  it('pushes toast items and assigns unique ids', () => {
    const id1 = toast.success('Operation completed')
    const id2 = toast.info('Notice')

    const list = toast.toasts()
    expect(list.length).toBe(2)
    expect(list[0].id).toBe(id1)
    expect(list[0].message).toBe('Operation completed')
    expect(list[0].kind).toBe('success')
    expect(list[1].id).toBe(id2)
  })

  it('deduplicates exact same message of same kind', () => {
    const id1 = toast.error('Connection failed')
    const id2 = toast.error('Connection failed')

    expect(id1).toBe(id2)
    const list = toast.toasts()
    expect(list.length).toBe(1)
    expect(list[0].message).toBe('Connection failed')
  })

  it('subsumes less specific toast when rich contextual message arrives', () => {
    const id1 = toast.error('device code request failed: EOF')
    expect(toast.toasts().length).toBe(1)

    const id2 = toast.error('发起授权失败: device code request failed: EOF')
    const list = toast.toasts()
    expect(list.length).toBe(1)
    expect(list[0].id).toBe(id2)
    expect(list[0].message).toBe('发起授权失败: device code request failed: EOF')
    expect(list[0].id).not.toBe(id1)
  })

  it('ignores less specific toast when rich message already visible', () => {
    const id1 = toast.error('发起授权失败: device code request failed: EOF')
    const id2 = toast.error('device code request failed: EOF')

    expect(id2).toBe(id1)
    const list = toast.toasts()
    expect(list.length).toBe(1)
    expect(list[0].message).toBe('发起授权失败: device code request failed: EOF')
  })

  it('renews a duplicate notification near expiry without adding another item', () => {
    const id = toast.success('Saved')
    vi.advanceTimersByTime(3499)

    expect(toast.success('Saved')).toBe(id)
    vi.advanceTimersByTime(1)
    expect(toast.toasts()).toHaveLength(1)
    vi.advanceTimersByTime(3499)
    expect(toast.toasts()).toHaveLength(0)
  })

  it('updates duplicate options and cancels its previous dismissal timer', () => {
    const id = toast.info('Notice', { title: 'Old title', duration: 1000 })
    vi.advanceTimersByTime(999)
    expect(toast.info('Notice', { title: 'New title', duration: 0 })).toBe(id)
    vi.advanceTimersByTime(10000)
    expect(toast.toasts()).toMatchObject([{ id, title: 'New title', duration: 0 }])

    toast.info('Notice', { duration: 500 })
    vi.advanceTimersByTime(500)
    expect(toast.toasts()).toHaveLength(0)
  })

  it('does not subscribe a notification caller to the toast list', () => {
    let calls = 0
    const dispose = createRoot(dispose => {
      createComputed(() => {
        if (++calls === 1) toast.info('Effect notification')
      })
      return dispose
    })
    try {
      toast.warning('Unrelated notification')
      expect(calls).toBe(1)
    } finally {
      dispose()
    }
  })

  it('auto-dismisses after duration', () => {
    toast.info('Temporary message', { duration: 1000 })
    expect(toast.toasts().length).toBe(1)

    vi.advanceTimersByTime(999)
    expect(toast.toasts().length).toBe(1)

    vi.advanceTimersByTime(1)
    expect(toast.toasts().length).toBe(0)
  })

  it('dismisses toast explicitly', () => {
    const id = toast.warning('Warning')
    expect(toast.toasts().length).toBe(1)

    dismiss(id)
    expect(toast.toasts().length).toBe(0)
  })
})
