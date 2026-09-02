// IndexedDB 存储工具：用于本地持久化大体积背景图（File / Blob / DataURL / 远程 URL）

const DB_NAME = 'cyrene_gateway_ui'
const STORE_NAME = 'ui_config'
const BG_KEY = 'custom_background'

export interface CustomBackgroundConfig {
  type: 'none' | 'url' | 'image'
  value: string // DataURL, ObjectURL 或远程 http/https URL
  blur?: number // 背景虚化度 (px)，默认 0
  opacity?: number // 背景不透明度 (0.1 ~ 1)，默认 1
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

export async function getStoredBackground(): Promise<CustomBackgroundConfig | null> {
  try {
    const db = await openDB()
    const { promise, resolve } = Promise.withResolvers<CustomBackgroundConfig | null>()
    const tx = db.transaction(STORE_NAME, 'readonly')
    const store = tx.objectStore(STORE_NAME)
    const req = store.get(BG_KEY)
    req.onsuccess = () => resolve((req.result as CustomBackgroundConfig) || null)
    req.onerror = () => resolve(null)
    return await promise
  } catch (e) {
    console.warn('[IndexedDB] Failed to get background:', e)
    return null
  }
}

export async function saveStoredBackground(config: CustomBackgroundConfig): Promise<void> {
  const db = await openDB()
  const { promise, resolve, reject } = Promise.withResolvers<void>()
  const tx = db.transaction(STORE_NAME, 'readwrite')
  const store = tx.objectStore(STORE_NAME)
  const req = store.put(config, BG_KEY)
  req.onsuccess = () => resolve()
  req.onerror = () => reject(req.error)
  return await promise
}

export async function clearStoredBackground(): Promise<void> {
  const db = await openDB()
  const { promise, resolve, reject } = Promise.withResolvers<void>()
  const tx = db.transaction(STORE_NAME, 'readwrite')
  const store = tx.objectStore(STORE_NAME)
  const req = store.delete(BG_KEY)
  req.onsuccess = () => resolve()
  req.onerror = () => reject(req.error)
  return await promise
}
