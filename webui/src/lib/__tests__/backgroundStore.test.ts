import { describe, it, expect, beforeEach, vi } from 'vitest'
import {
  getWallpaperConfig,
  saveWallpaperConfig,
  DEFAULT_WALLPAPER_CONFIG,
} from '@/lib/backgroundStore'

describe('backgroundStore - 容灾与自愈测试', () => {
  let mockStore: Record<string, string> = {}

  beforeEach(() => {
    mockStore = {}
    const mockStorage = {
      getItem: vi.fn((key: string) => mockStore[key] ?? null),
      setItem: vi.fn((key: string, val: string) => { mockStore[key] = val }),
      removeItem: vi.fn((key: string) => { delete mockStore[key] }),
      clear: vi.fn(() => { mockStore = {} }),
    }
    vi.stubGlobal('localStorage', mockStorage)
  })

  it('初始状态返回默认配置', () => {
    const cfg = getWallpaperConfig()
    expect(cfg.enabled).toBe(false)
    expect(cfg.surfaceAlpha).toBe(DEFAULT_WALLPAPER_CONFIG.surfaceAlpha)
  })

  it('保存与读取远程图片配置正常', () => {
    saveWallpaperConfig({
      ...DEFAULT_WALLPAPER_CONFIG,
      enabled: true,
      sourceType: 'remote',
      remoteUrl: 'https://example.com/wallpaper.jpg',
    })

    const cfg = getWallpaperConfig()
    expect(cfg.enabled).toBe(true)
    expect(cfg.sourceType).toBe('remote')
    expect(cfg.remoteUrl).toBe('https://example.com/wallpaper.jpg')
  })
})
