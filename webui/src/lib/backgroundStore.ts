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
}

export const DEFAULT_WALLPAPER_CONFIG: WallpaperConfig = {
  enabled: false,
  blur: 0,
  opacity: 1,
  surfaceAlpha: 0.78,
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
