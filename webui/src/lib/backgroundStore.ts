// 本地壁纸存储与配置管理：
// - 大体积图片二进制/Base64：持久化于 IndexedDB ('cyrene_gateway_media' -> 'images')
// - 轻量 UI 参数（开关、虚化度、透明度、毛玻璃等）：持久化于 localStorage ('cyrene_wallpaper_config')

const DB_NAME = 'cyrene_gateway_media'
const STORE_NAME = 'images'
const WALLPAPER_KEY = 'custom_wallpaper'
const CONFIG_KEY = 'cyrene_wallpaper_config'

export interface WallpaperConfig {
  enabled: boolean
  blur: number // 0 ~ 30 px
  opacity: number // 0.1 ~ 1.0
  surfaceAlpha: number // 0.4 ~ 0.95 (面板底色不透明度，对齐 zashboard)
  glassBlur: number // 0 ~ 40 px (面板毛玻璃模糊度，对齐 zashboard)
  sourceType?: 'remote' | 'upload'
  remoteUrl?: string
  thumbnail?: string // 32x18 极微缩 Base64 占位图，用于首屏 0ms 瞬间直出消除白屏闪烁
}

export const DEFAULT_WALLPAPER_CONFIG: WallpaperConfig = {
  enabled: false,
  blur: 0,
  opacity: 1,
  surfaceAlpha: 0.62,
  glassBlur: 20,
}

export function getWallpaperConfig(): WallpaperConfig {
  if (typeof window === 'undefined' || !window.localStorage) {
    return { ...DEFAULT_WALLPAPER_CONFIG }
  }
  try {
    const raw = localStorage.getItem(CONFIG_KEY)
    if (!raw) return { ...DEFAULT_WALLPAPER_CONFIG }
    const parsed = JSON.parse(raw) as Partial<WallpaperConfig>
    return {
      enabled: typeof parsed.enabled === 'boolean' ? parsed.enabled : DEFAULT_WALLPAPER_CONFIG.enabled,
      blur: typeof parsed.blur === 'number' ? parsed.blur : DEFAULT_WALLPAPER_CONFIG.blur,
      opacity: typeof parsed.opacity === 'number' ? parsed.opacity : DEFAULT_WALLPAPER_CONFIG.opacity,
      surfaceAlpha: typeof parsed.surfaceAlpha === 'number' ? parsed.surfaceAlpha : DEFAULT_WALLPAPER_CONFIG.surfaceAlpha,
      glassBlur: typeof parsed.glassBlur === 'number' ? parsed.glassBlur : DEFAULT_WALLPAPER_CONFIG.glassBlur,
      sourceType: parsed.sourceType,
      remoteUrl: parsed.remoteUrl,
      thumbnail: typeof parsed.thumbnail === 'string' ? parsed.thumbnail : undefined,
    }
  } catch {
    return { ...DEFAULT_WALLPAPER_CONFIG }
  }
}

export function saveWallpaperConfig(config: WallpaperConfig): void {
  if (typeof window === 'undefined' || !window.localStorage) return
  try {
    localStorage.setItem(CONFIG_KEY, JSON.stringify(config))
  } catch (e) {
    console.warn('[WallpaperConfig] Failed to save to localStorage:', e)
  }
}

function openDB(): Promise<IDBDatabase> {
  const { promise, resolve, reject } = Promise.withResolvers<IDBDatabase>()
  if (typeof window === 'undefined' || !window.indexedDB) {
    reject(new Error('IndexedDB is not supported'))
    return promise
  }
  const req = indexedDB.open(DB_NAME, 1)
  req.onupgradeneeded = () => {
    const db = req.result
    if (!db.objectStoreNames.contains(STORE_NAME)) {
      db.createObjectStore(STORE_NAME)
    }
  }
  req.onsuccess = () => resolve(req.result)
  req.onerror = () => reject(req.error)
  return promise
}

export async function getStoredWallpaper(): Promise<string | null> {
  try {
    const db = await openDB()
    const { promise, resolve } = Promise.withResolvers<string | null>()
    const tx = db.transaction(STORE_NAME, 'readonly')
    const store = tx.objectStore(STORE_NAME)
    const req = store.get(WALLPAPER_KEY)
    req.onsuccess = () => resolve((req.result as string) || null)
    req.onerror = () => resolve(null)
    return await promise
  } catch (e) {
    console.warn('[IndexedDB] Failed to get wallpaper:', e)
    return null
  }
}

export async function saveStoredWallpaper(dataUrl: string): Promise<void> {
  const db = await openDB()
  const { promise, resolve, reject } = Promise.withResolvers<void>()
  const tx = db.transaction(STORE_NAME, 'readwrite')
  const store = tx.objectStore(STORE_NAME)
  const req = store.put(dataUrl, WALLPAPER_KEY)
  req.onsuccess = () => resolve()
  req.onerror = () => reject(req.error)
  return await promise
}

export async function clearStoredWallpaper(): Promise<void> {
  try {
    const db = await openDB()
    const { promise, resolve, reject } = Promise.withResolvers<void>()
    const tx = db.transaction(STORE_NAME, 'readwrite')
    const store = tx.objectStore(STORE_NAME)
    const req = store.delete(WALLPAPER_KEY)
    req.onsuccess = () => resolve()
    req.onerror = () => reject(req.error)
    await promise
  } catch (e) {
    console.warn('[IndexedDB] Failed to clear wallpaper:', e)
  }
}

// 自动清理之前测试时留下的废弃库 'cyrene_gateway_ui'
export function cleanupLegacyStorage(): void {
  if (typeof window === 'undefined') return
  try {
    if (window.indexedDB && indexedDB.deleteDatabase) {
      indexedDB.deleteDatabase('cyrene_gateway_ui')
    }
  } catch {}
}

/**
 * 远程图片拉取并转为 Base64 Data URL：
 * 采用 no-referrer 策略绕过绝大部分图床/CDN 防盗链限制。
 * 若遇跨域/网络故障返回 null，由上层安全降级使用原始 URL。
 */
export async function fetchRemoteImageDataUrl(url: string): Promise<string | null> {
  if (!url || typeof window === 'undefined') return null
  try {
    const resp = await fetch(url, { referrerPolicy: 'no-referrer' })
    if (!resp.ok) return null
    const blob = await resp.blob()
    if (!blob.type.startsWith('image/')) return null
    const reader = new FileReader()
    const { promise, resolve, reject } = Promise.withResolvers<string>()
    reader.onload = () => resolve(reader.result as string)
    reader.onerror = () => reject(reader.error)
    reader.readAsDataURL(blob)
    return await promise
  } catch {
    return null
  }
}

export interface ProcessedWallpaper {
  dataUrl: string
  thumbnail: string
}

/**
 * 客户端智能降采样与压缩管道：
 * 1. 约束最大尺寸在 2560px（2K 黄金比例），防止超大原始图导致内存崩溃与卡顿；
 * 2. 导出高效 WebP/JPEG（质量 0.85），大幅缩减体积；
 * 3. 抽取 32xH 超微缩占位图（LQIP，~500 字节），存入 localStorage 用于 0ms 首屏秒开。
 */
export async function processAndCompressImage(source: string): Promise<ProcessedWallpaper> {
  if (typeof window === 'undefined' || typeof document === 'undefined') {
    return { dataUrl: source, thumbnail: '' }
  }
  try {
    const img = new Image()
    img.crossOrigin = 'anonymous'
    const { promise, resolve, reject } = Promise.withResolvers<void>()
    img.onload = () => resolve()
    img.onerror = () => reject(new Error('Failed to load image'))
    img.src = source
    await promise

    const MAX_DIM = 2560
    let w = img.naturalWidth || img.width || 1920
    let h = img.naturalHeight || img.height || 1080
    if (w > MAX_DIM || h > MAX_DIM) {
      if (w > h) {
        h = Math.round((h * MAX_DIM) / w)
        w = MAX_DIM
      } else {
        w = Math.round((w * MAX_DIM) / h)
        h = MAX_DIM
      }
    }

    const canvas = document.createElement('canvas')
    canvas.width = w
    canvas.height = h
    const ctx = canvas.getContext('2d')
    if (!ctx) {
      return { dataUrl: source, thumbnail: '' }
    }

    ctx.imageSmoothingEnabled = true
    ctx.imageSmoothingQuality = 'high'
    ctx.drawImage(img, 0, 0, w, h)

    let compData = ''
    try {
      compData = canvas.toDataURL('image/webp', 0.85)
      if (!compData.startsWith('data:image/webp')) {
        compData = canvas.toDataURL('image/jpeg', 0.85)
      }
    } catch {
      compData = source
    }

    // LQIP 微缩图：固定 32px 宽度，高按比例自适应
    let thumbData = ''
    try {
      const tw = 32
      const th = Math.max(16, Math.round((32 * h) / w))
      const thumbCanvas = document.createElement('canvas')
      thumbCanvas.width = tw
      thumbCanvas.height = th
      const thumbCtx = thumbCanvas.getContext('2d')
      if (thumbCtx) {
        thumbCtx.imageSmoothingEnabled = true
        thumbCtx.imageSmoothingQuality = 'medium'
        thumbCtx.drawImage(img, 0, 0, tw, th)
        thumbData = thumbCanvas.toDataURL('image/jpeg', 0.6)
      }
    } catch {}

    return {
      dataUrl: compData || source,
      thumbnail: thumbData,
    }
  } catch (e) {
    console.warn('[processAndCompressImage] Fallback to raw source:', e)
    return { dataUrl: source, thumbnail: '' }
  }
}
