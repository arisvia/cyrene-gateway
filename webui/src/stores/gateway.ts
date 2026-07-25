import { defineStore } from 'pinia'
import { ref } from 'vue'
import { api, apiPost, apiPut, apiDelete } from '@/lib/api'
import { useToast } from '@/lib/toast'

export interface Provider {
  id: string
  provider: string
  name?: string
  authType: string
  priority: number
  isActive: boolean
  data?: Record<string, any>
}

export interface RegistryProvider {
  id: string
  name: string
  category: string
  authType: string
  baseUrl?: string
}

export interface Combo {
  id: string
  name: string
  kind: string
  models: string[]
}

export interface ApiKey {
  id: string
  name?: string
  key: string
  isActive: boolean
  createdAt?: string
}

export interface ProxyPool {
  id: string
  isActive: boolean
  data: { name: string; type: string; proxyUrl: string; noProxy?: string; strictProxy?: boolean }
}

export interface UsageStats {
  totalRequests?: number
  totalPromptTokens?: number
  totalCompletionTokens?: number
  totalCost?: number
  totalRequestsLifetime?: number
  byProvider?: Record<string, { requests: number; promptTokens: number; completionTokens: number }>
}

export const useGatewayStore = defineStore('gateway', () => {
  const version = ref('...')
  const health = ref<Record<string, any>>({})
  const providers = ref<Provider[]>([])
  const aliases = ref<Record<string, string>>({})
  const combos = ref<Combo[]>([])
  const apiKeys = ref<ApiKey[]>([])
  const proxyPools = ref<ProxyPool[]>([])
  const settings = ref<Record<string, any>>({})
  const registryList = ref<RegistryProvider[]>([])
  const registryCategories = ref<{ category: string; count: number }[]>([])
  const usageStats = ref<UsageStats>({})
  const usageChart = ref<{ label: string; tokens: number }[]>([])
  const requestDetails = ref<any[]>([])
  const quotaEntries = ref<any[]>([])

  async function loadAll() {
    try {
      const [v, h, p, a, c, reg] = await Promise.all([
        api('/api/version'), api('/api/health'), api('/api/providers'),
        api('/api/models/alias'), api('/api/combos'), api('/api/registry'),
      ])
      version.value = v.version || 'dev'
      health.value = h
      providers.value = Array.isArray(p) ? p : []
      aliases.value = a || {}
      combos.value = Array.isArray(c) ? c : []
      const all: RegistryProvider[] = []
      ;(reg?.categories || []).forEach((cat: any) => { (cat.providers || []).forEach((rp: any) => all.push(rp)) })
      registryList.value = all.sort((x, y) => x.name.localeCompare(y.name))
      registryCategories.value = (reg?.categories || []).map((c: any) => ({ category: c.category, count: c.count }))
    } catch (e) {
      console.error('load failed:', e)
    }
    loadKeys()
    loadProxies()
  }

  async function loadKeys() {
    try { apiKeys.value = await api('/api/keys') } catch { apiKeys.value = [] }
  }

  async function loadProxies() {
    try {
      const r = await api('/api/proxy-pools')
      proxyPools.value = r?.proxyPools || (Array.isArray(r) ? r : [])
    } catch { proxyPools.value = [] }
  }

  async function loadSettings() {
    try { settings.value = await api('/api/settings') } catch { settings.value = {} }
  }

  async function loadUsage(period: string) {
    try {
      const [stats, chart, details] = await Promise.all([
        api(`/api/usage/stats?period=${period}`),
        api(`/api/usage/chart?period=${period}`),
        api('/api/usage/request-details?page=1&pageSize=15'),
      ])
      usageStats.value = stats || {}
      usageChart.value = Array.isArray(chart) ? chart : []
      requestDetails.value = Array.isArray(details) ? details : (details?.details || [])
    } catch (e) { console.error('usage load failed:', e) }
  }

  async function loadQuota() {
    try {
      const res = await api('/api/quota')
      quotaEntries.value = Array.isArray(res.entries) ? res.entries : []
    } catch { quotaEntries.value = [] }
  }

  // --- Provider actions ---
  const toast = useToast()

  async function addProvider(payload: any) {
    try { await apiPost('/api/providers', payload); toast.success(`Provider "${payload.name || payload.provider}" added`); await loadAll() }
    catch (e: any) { toast.error(`Failed to add provider: ${e.message}`) }
  }
  async function toggleProvider(p: Provider) { await apiPut(`/api/providers/${p.id}`, { isActive: !p.isActive }); await loadAll() }
  async function resetProvider(p: Provider) {
    try { await apiPost(`/api/providers/${p.id}/reset`); toast.success(`Cooldown reset for "${p.name || p.provider}"`); await loadAll() }
    catch (e: any) { toast.error(`Reset failed: ${e.message}`) }
  }
  async function deleteProvider(p: Provider) {
    try { await apiDelete(`/api/providers/${p.id}`); toast.success(`Provider "${p.name || p.provider}" deleted`); await loadAll() }
    catch (e: any) { toast.error(`Delete failed: ${e.message}`) }
  }

  // --- Key actions ---
  async function createKey(name: string) {
    const k = await apiPost('/api/keys', { name })
    toast.success(`Key "${name || k.id?.slice(0, 8)}" generated`)
    await loadKeys()
    return k
  }
  async function deleteKey(k: ApiKey) {
    try { await apiDelete(`/api/keys/${k.id}`); toast.success('Key deleted'); await loadKeys() }
    catch (e: any) { toast.error(`Delete failed: ${e.message}`) }
  }

  // --- Alias actions ---
  async function addAlias(alias: string, target: string) {
    try { await apiPost('/api/models/alias', { alias, target }); toast.success(`Alias "${alias}" → ${target}`); await loadAll() }
    catch (e: any) { toast.error(`Failed to add alias: ${e.message}`) }
  }
  async function deleteAlias(alias: string) {
    try { await apiDelete('/api/models/alias', { alias }); toast.success(`Alias "${alias}" removed`); await loadAll() }
    catch (e: any) { toast.error(`Delete failed: ${e.message}`) }
  }

  // --- Combo actions ---
  async function addCombo(name: string, kind: string, models: string[]) {
    try { await apiPost('/api/combos', { name, kind, models }); toast.success(`Combo "${name}" created`); await loadAll() }
    catch (e: any) { toast.error(`Failed to create combo: ${e.message}`) }
  }
  async function deleteCombo(c: Combo) {
    try { await apiDelete(`/api/combos/${c.id}`); toast.success(`Combo "${c.name}" deleted`); await loadAll() }
    catch (e: any) { toast.error(`Delete failed: ${e.message}`) }
  }

  // --- Proxy actions ---
  async function addProxy(payload: any) {
    try { await apiPost('/api/proxy-pools', payload); toast.success(`Proxy "${payload.name}" added`); await loadProxies() }
    catch (e: any) { toast.error(`Failed to add proxy: ${e.message}`) }
  }
  async function toggleProxy(pp: ProxyPool) { await apiPut(`/api/proxy-pools/${pp.id}`, { isActive: !pp.isActive }); await loadProxies() }
  async function deleteProxy(pp: ProxyPool) {
    try { await apiDelete(`/api/proxy-pools/${pp.id}`); toast.success(`Proxy "${pp.data.name}" deleted`); await loadProxies() }
    catch (e: any) { toast.error(`Delete failed: ${e.message}`) }
  }

  // --- Settings actions ---
  async function saveSettings() {
    try { await apiPut('/api/settings', settings.value); toast.success('Settings saved') }
    catch (e: any) { toast.error(`Failed to save settings: ${e.message}`) }
  }
  async function setPassword(password: string) { return apiPost('/api/auth/password', { password }) }

  return {
    version, health, providers, aliases, combos, apiKeys, proxyPools, settings,
    registryList, registryCategories, usageStats, usageChart, requestDetails, quotaEntries,
    loadAll, loadKeys, loadProxies, loadSettings, loadUsage, loadQuota,
    addProvider, toggleProvider, resetProvider, deleteProvider,
    createKey, deleteKey,
    addAlias, deleteAlias,
    addCombo, deleteCombo,
    addProxy, toggleProxy, deleteProxy,
    saveSettings, setPassword,
  }
})
