import { describe, it, expect, beforeEach, vi } from 'vitest'
import { useThemeStore, THEME_COLOR_OPTIONS } from '../theme'

describe('ThemeStore', () => {
  const mockStorage: Record<string, string> = {}

  beforeEach(() => {
    for (const k of Object.keys(mockStorage)) {
      delete mockStorage[k]
    }
    vi.stubGlobal('localStorage', {
      getItem: (k: string) => mockStorage[k] ?? null,
      setItem: (k: string, v: string) => {
        mockStorage[k] = v
      },
      removeItem: (k: string) => {
        delete mockStorage[k]
      },
    })
    document.documentElement.className = ''
    delete document.documentElement.dataset.themeColor
  })

  it('provides 4 spectral color options extracted from Cyrene logo', () => {
    expect(THEME_COLOR_OPTIONS).toHaveLength(4)
    expect(THEME_COLOR_OPTIONS.map(o => o.id)).toEqual(['indigo', 'cyan', 'mint', 'violet'])
  })

  it('initializes with default indigo color and dark mode', () => {
    const theme = useThemeStore()
    expect(theme.color()).toBe('indigo')
    expect(theme.mode()).toBe('dark')
  })

  it('updates color and sets data-theme-color attribute and localStorage', () => {
    const theme = useThemeStore()
    theme.setColor('cyan')
    expect(theme.color()).toBe('cyan')
    expect(document.documentElement.dataset.themeColor).toBe('cyan')
    expect(mockStorage['cyrene-theme-color']).toBe('cyan')

    theme.setColor('mint')
    expect(theme.color()).toBe('mint')
    expect(document.documentElement.dataset.themeColor).toBe('mint')

    theme.setColor('violet')
    expect(theme.color()).toBe('violet')
    expect(document.documentElement.dataset.themeColor).toBe('violet')

    // Reset back to default
    theme.setColor('indigo')
    expect(theme.color()).toBe('indigo')
  })

  it('toggles mode and syncs light class and localStorage', () => {
    const theme = useThemeStore()
    expect(theme.mode()).toBe('dark')

    theme.toggleMode()
    expect(theme.mode()).toBe('light')
    expect(document.documentElement.classList.contains('light')).toBe(true)
    expect(mockStorage['cyrene-theme']).toBe('light')

    theme.toggleMode()
    expect(theme.mode()).toBe('dark')
    expect(document.documentElement.classList.contains('light')).toBe(false)
    expect(mockStorage['cyrene-theme']).toBe('dark')
  })

  it('invokes startViewTransition when available with event origin and sets CSS vars', async () => {
    const theme = useThemeStore()
    let transitionCallback: (() => void) | null = null
    let finishResolve: (() => void) | null = null
    const finishedPromise = new Promise<void>(res => {
      finishResolve = res
    })

    const mockStartViewTransition = vi.fn().mockImplementation((cb: () => void) => {
      transitionCallback = cb
      return {
        ready: Promise.resolve(),
        finished: finishedPromise,
      }
    })
    const doc = document as unknown as {
      startViewTransition: typeof mockStartViewTransition
    }
    doc.startViewTransition = mockStartViewTransition

    theme.toggleMode({ clientX: 100, clientY: 200 } as MouseEvent)
    expect(mockStartViewTransition).toHaveBeenCalled()
    expect(document.documentElement.style.getPropertyValue('--theme-switch-x')).toBe('100px')
    expect(document.documentElement.style.getPropertyValue('--theme-switch-y')).toBe('200px')

    if (transitionCallback) {
      ;(transitionCallback as () => void)()
    }
    expect(theme.mode()).toBe('light')

    // Simulate transition finish
    if (finishResolve) {
      ;(finishResolve as () => void)()
    }
    await finishedPromise
    await Promise.resolve()

    expect(document.documentElement.style.getPropertyValue('--theme-switch-x')).toBe('')
    delete (document as unknown as { startViewTransition?: unknown }).startViewTransition
  })
})
