import { describe, it, expect, beforeEach, vi } from 'vitest'
import {
  getWallpaperConfig,
  saveWallpaperConfig,
  DEFAULT_WALLPAPER_CONFIG,
} from '@/lib/backgroundStore'
import { useBackgroundStore } from '@/stores/background'

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

  it('IndexedDB 被清理但有 remoteUrl 时，init() 自动降级自愈且不重置 enabled 开关', async () => {
    const remoteUrl = 'https://example.com/wallpaper.jpg'
    mockStore['cyrene_wallpaper_config'] = JSON.stringify({
      ...DEFAULT_WALLPAPER_CONFIG,
      enabled: true,
      sourceType: 'remote',
      remoteUrl,
    })

    // 模拟 fetch 失败（离线/CORS）情景，验证即使静默重拉失败，也绝不误关 enabled
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('Network error')))

    const store = useBackgroundStore()
    await store.init()

    // 验证核心自愈契约：enabled 依然开启，imageData 立即回退为 remoteUrl
    expect(store.config().enabled).toBe(true)
    expect(store.imageData()).toBe(remoteUrl)
    expect(store.hasCustomBg()).toBe(true)

    // 验证未覆写 enabled: false 到 localStorage
    const saved = JSON.parse(mockStore['cyrene_wallpaper_config'] || '{}')
    expect(saved.enabled).not.toBe(false)
  })
})
