import { createSignal, createRoot, type Accessor } from 'solid-js'
import {
  getStoredBackground,
  saveStoredBackground,
  clearStoredBackground,
  type CustomBackgroundConfig,
} from '@/lib/backgroundStore'

export interface BackgroundStore {
  bgConfig: Accessor<CustomBackgroundConfig>
  loaded: Accessor<boolean>
  init: () => Promise<void>
  setBackground: (config: CustomBackgroundConfig) => Promise<void>
  resetBackground: () => Promise<void>
}

function createCustomBgStore(): BackgroundStore {
  const [bgConfig, setBgConfig] = createSignal<CustomBackgroundConfig>({
    type: 'none',
    value: '',
    blur: 0,
    opacity: 1,
  })
  const [loaded, setLoaded] = createSignal(false)

  async function init() {
    try {
      const stored = await getStoredBackground()
      if (stored) {
        setBgConfig(stored)
      }
    } catch (e) {
      console.warn('[bgStore] init failed:', e)
    } finally {
      setLoaded(true)
    }
  }

  async function setBackground(config: CustomBackgroundConfig) {
    setBgConfig(config)
    if (config.type === 'none') {
      await clearStoredBackground()
    } else {
      await saveStoredBackground(config)
    }
  }

  async function resetBackground() {
    setBgConfig({ type: 'none', value: '', blur: 0, opacity: 1 })
    await clearStoredBackground()
  }

  return {
    bgConfig,
    loaded,
    init,
    setBackground,
    resetBackground,
  }
}

let _bgStore: BackgroundStore | null = null

export function useBackgroundStore(): BackgroundStore {
  if (!_bgStore) _bgStore = createRoot(createCustomBgStore)
  return _bgStore
}
