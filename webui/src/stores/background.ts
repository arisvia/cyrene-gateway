import { createSignal, createRoot, type Accessor } from 'solid-js'
import {
  getWallpaperConfig,
  saveWallpaperConfig,
  getStoredWallpaper,
  saveStoredWallpaper,
  clearStoredWallpaper,
  cleanupLegacyStorage,
  fetchRemoteImageDataUrl,
  processAndCompressImage,
  type WallpaperConfig,
  DEFAULT_WALLPAPER_CONFIG,
} from '@/lib/backgroundStore'
export interface BackgroundStore {
  config: Accessor<WallpaperConfig>
  imageData: Accessor<string>
  hasCustomBg: Accessor<boolean>
  loaded: Accessor<boolean>
  init: () => Promise<void>
  setWallpaper: (dataUrl: string, meta?: { sourceType?: 'remote' | 'upload'; remoteUrl?: string; thumbnail?: string }) => Promise<void>
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
  const initialCfg = getWallpaperConfig()
  const [config, setConfig] = createSignal<WallpaperConfig>(initialCfg)
  const [imageData, setImageData] = createSignal<string>('')
  const [loaded, setLoaded] = createSignal(false)

  // 同步初始化外观变量：在 store 创建瞬间生效，彻底消除样式跳闪 (Zero-FOUC)
  if (initialCfg.enabled) {
    applyAppearanceVariables(initialCfg, true)
  }

  const hasCustomBg = () => config().enabled && (!!imageData() || !!config().thumbnail)

  async function init() {
    try {
      cleanupLegacyStorage()
      const cfg = getWallpaperConfig()
      setConfig(cfg)
      if (cfg.enabled) {
        applyAppearanceVariables(cfg, true)
        let data = await getStoredWallpaper()
        if (!data && cfg.remoteUrl?.trim()) {
          // 容灾与自愈：
          // 若 IndexedDB 缓存被清理，但用户远程 URL 依然存在：
          // 1. 立即降级使用 remoteUrl 作为图像源
          data = cfg.remoteUrl.trim()
          // 2. 在后台异步重新拉取并智能压缩，回填至 IndexedDB
          fetchRemoteImageDataUrl(data).then(async base64 => {
            if (base64) {
              const proc = await processAndCompressImage(base64)
              await saveStoredWallpaper(proc.dataUrl)
              setImageData(proc.dataUrl)
              if (proc.thumbnail && proc.thumbnail !== config().thumbnail) {
                const updated = { ...config(), thumbnail: proc.thumbnail }
                setConfig(updated)
                saveWallpaperConfig(updated)
              }
            }
          }).catch(() => {})
        }

        if (data) {
          setImageData(data)
          applyAppearanceVariables(cfg, true)
          // 自愈升级：若旧版本用户此前没有生成过 thumbnail，在后台静默生成并写入 localStorage
          if (!cfg.thumbnail) {
            processAndCompressImage(data).then(proc => {
              if (proc.thumbnail) {
                const updated = { ...config(), thumbnail: proc.thumbnail }
                setConfig(updated)
                saveWallpaperConfig(updated)
              }
            }).catch(() => {})
          }
        } else if (!cfg.thumbnail) {
          // 仅在既无本地缓存也无缩略图也无远程 URL 时才重置开关
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

  async function setWallpaper(
    dataUrl: string,
    meta?: { sourceType?: 'remote' | 'upload'; remoteUrl?: string; thumbnail?: string }
  ) {
    let finalData = dataUrl
    let finalThumb = meta?.thumbnail

    // 若未传入预计算的 thumbnail，自动进行智能降采样与压缩
    if (!finalThumb) {
      const proc = await processAndCompressImage(dataUrl)
      finalData = proc.dataUrl
      finalThumb = proc.thumbnail
    }

    await saveStoredWallpaper(finalData)
    setImageData(finalData)
    const newConfig: WallpaperConfig = {
      ...config(),
      enabled: true,
      thumbnail: finalThumb || config().thumbnail,
      sourceType: meta?.sourceType ?? 'upload',
      remoteUrl: meta?.remoteUrl ?? (meta?.sourceType === 'upload' ? undefined : config().remoteUrl),
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
      thumbnail: undefined,
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
