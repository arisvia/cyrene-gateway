import { createSignal, createRoot, type Accessor } from 'solid-js'
import {
  getWallpaperConfig,
  saveWallpaperConfig,
  getStoredWallpaper,
  saveStoredWallpaper,
  clearStoredWallpaper,
  cleanupLegacyStorage,
  fetchRemoteImageDataUrl,
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
    root.style.setProperty('--glass-blur', `${config.glassBlur}px`)
    root.style.setProperty('--blur-lg', `${config.glassBlur}px`)
    root.style.setProperty('--blur-md', `${config.glassBlur}px`)
  } else {
    root.classList.remove('custom-background')
    root.style.removeProperty('--app-surface-alpha')
    root.style.removeProperty('--app-glass-blur')
    root.style.removeProperty('--glass-blur')
    root.style.removeProperty('--blur-lg')
    root.style.removeProperty('--blur-md')
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
        let data = await getStoredWallpaper()
        if (!data && cfg.remoteUrl?.trim()) {
          // 容灾与自愈：
          // 若 IndexedDB 缓存被清理（如浏览器隐私清理工具、清理垃圾缓存等），
          // 但用户的远程图片 URL 依然保存在 localStorage 配置中：
          // 1. 立即降级使用 remoteUrl 作为图像源，保证刷新即呈现壁纸，绝不误把 enabled 关闭
          data = cfg.remoteUrl.trim()
          // 2. 在后台异步重新拉取图片并回填至 IndexedDB，恢复离线能力与秒开体验
          fetchRemoteImageDataUrl(data).then(async base64 => {
            if (base64) {
              await saveStoredWallpaper(base64)
              setImageData(base64)
            }
          }).catch(() => {})
        }

        if (data) {
          setImageData(data)
          applyAppearanceVariables(cfg, true)
        } else {
          // 仅在既无本地缓存也无远程 URL 时才重置开关
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
    if (next.enabled && !imageData() && next.remoteUrl?.trim()) {
      setImageData(next.remoteUrl.trim())
      fetchRemoteImageDataUrl(next.remoteUrl.trim()).then(async base64 => {
        if (base64) {
          await saveStoredWallpaper(base64)
          setImageData(base64)
        }
      }).catch(() => {})
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
