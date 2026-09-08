import { createSignal, createRoot, type Accessor } from 'solid-js'
import {
  getWallpaperConfig,
  saveWallpaperConfig,
  getStoredWallpaper,
  saveStoredWallpaper,
  clearStoredWallpaper,
  cleanupLegacyStorage,
  type WallpaperConfig,
  DEFAULT_WALLPAPER_CONFIG,
} from '@/lib/backgroundStore'

export interface BackgroundStore {
  config: Accessor<WallpaperConfig>
  imageData: Accessor<string>
  hasCustomBg: Accessor<boolean>
  loaded: Accessor<boolean>
  init: () => Promise<void>
  setWallpaper: (dataUrl: string, meta?: { sourceType?: 'remote' | 'upload'; remoteUrl?: string }) => Promise<void>
  updateConfig: (partial: Partial<WallpaperConfig>) => void
  resetWallpaper: () => Promise<void>
}

function applyAppearanceVariables(config: WallpaperConfig, enabled: boolean) {
  if (typeof document === 'undefined') return
  const root = document.documentElement
  if (enabled) {
    root.classList.add('custom-background')
    root.style.setProperty('--app-surface-alpha', `${config.surfaceAlpha}`)
    root.style.setProperty('--app-glass-blur', `${config.glassBlur}px`)
  } else {
    root.classList.remove('custom-background')
    root.style.removeProperty('--app-surface-alpha')
    root.style.removeProperty('--app-glass-blur')
  }
}

function createCustomBgStore(): BackgroundStore {
  const [config, setConfig] = createSignal<WallpaperConfig>(getWallpaperConfig())
  const [imageData, setImageData] = createSignal<string>('')
  const [loaded, setLoaded] = createSignal(false)

  const hasCustomBg = () => config().enabled && !!imageData()

  async function init() {
    try {
      cleanupLegacyStorage()
      const cfg = getWallpaperConfig()
      setConfig(cfg)
      if (cfg.enabled) {
        const data = await getStoredWallpaper()
        if (data) {
          setImageData(data)
          applyAppearanceVariables(cfg, true)
        } else {
          // 无图片数据时重置开关
          const disabledCfg = { ...cfg, enabled: false }
          setConfig(disabledCfg)
          saveWallpaperConfig(disabledCfg)
          applyAppearanceVariables(disabledCfg, false)
        }
      } else {
        applyAppearanceVariables(cfg, false)
      }
    } catch (e) {
      console.warn('[bgStore] init failed:', e)
    } finally {
      setLoaded(true)
    }
  }

  async function setWallpaper(dataUrl: string, meta?: { sourceType?: 'remote' | 'upload'; remoteUrl?: string }) {
    await saveStoredWallpaper(dataUrl)
    setImageData(dataUrl)
    const newConfig: WallpaperConfig = {
      ...config(),
      enabled: true,
      sourceType: meta?.sourceType ?? 'upload',
      remoteUrl: meta?.remoteUrl ?? config().remoteUrl,
    }
    setConfig(newConfig)
    saveWallpaperConfig(newConfig)
    applyAppearanceVariables(newConfig, true)
  }

  function updateConfig(partial: Partial<WallpaperConfig>) {
    const next: WallpaperConfig = {
      ...config(),
      ...partial,
    }
    setConfig(next)
    saveWallpaperConfig(next)
    applyAppearanceVariables(next, next.enabled && !!imageData())
  }

  async function resetWallpaper() {
    await clearStoredWallpaper()
    setImageData('')
    const resetCfg: WallpaperConfig = {
      ...DEFAULT_WALLPAPER_CONFIG,
      remoteUrl: '',
    }
    setConfig(resetCfg)
    saveWallpaperConfig(resetCfg)
    applyAppearanceVariables(resetCfg, false)
  }

  return {
    config,
    imageData,
    hasCustomBg,
    loaded,
    init,
    setWallpaper,
    updateConfig,
    resetWallpaper,
  }
}

let _bgStore: BackgroundStore | null = null

export function useBackgroundStore(): BackgroundStore {
  if (!_bgStore) _bgStore = createRoot(createCustomBgStore)
  return _bgStore
}
