import { defineStore } from 'pinia'
import { ref } from 'vue'
import { api, apiPost, apiPut, apiDelete } from '@/lib/api'

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
  async function addProvider(payload: any) { await apiPost('/api/providers', payload); await loadAll() }
  async function toggleProvider(p: Provider) { await apiPut(`/api/providers/${p.id}`, { isActive: !p.isActive }); await loadAll() }
  async function resetProvider(p: Provider) { await apiPost(`/api/providers/${p.id}/reset`); await loadAll() }
  async function deleteProvider(p: Provider) { await apiDelete(`/api/providers/${p.id}`); await loadAll() }

  // --- Key actions ---
  async function createKey(name: string) { const k = await apiPost('/api/keys', { name }); await loadKeys(); return k }
  async function deleteKey(k: ApiKey) { await apiDelete(`/api/keys/${k.id}`); await loadKeys() }

  // --- Alias actions ---
  async function addAlias(alias: string, target: string) { await apiPost('/api/models/alias', { alias, target }); await loadAll() }
  async function deleteAlias(alias: string) { await apiDelete('/api/models/alias', { alias }); await loadAll() }

  // --- Combo actions ---
  async function addCombo(name: string, kind: string, models: string[]) { await apiPost('/api/combos', { name, kind, models }); await loadAll() }
  async function deleteCombo(c: Combo) { await apiDelete(`/api/combos/${c.id}`); await loadAll() }

  // --- Proxy actions ---
  async function addProxy(payload: any) { await apiPost('/api/proxy-pools', payload); await loadProxies() }
  async function toggleProxy(pp: ProxyPool) { await apiPut(`/api/proxy-pools/${pp.id}`, { isActive: !pp.isActive }); await loadProxies() }
  async function deleteProxy(pp: ProxyPool) { await apiDelete(`/api/proxy-pools/${pp.id}`); await loadProxies() }

  // --- Settings actions ---
  async function saveSettings() { await apiPut('/api/settings', settings.value) }
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
