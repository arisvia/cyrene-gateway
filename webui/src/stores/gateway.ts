import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { api, apiPost, apiPut, apiPatch, apiDelete } from '@/lib/api'
import { useToast } from '@/lib/toast'

/* ─── Domain types ─── */

export interface Provider {
  id: string
  provider: string
  name?: string
  authType: string
  email?: string
  priority: number
  isActive: boolean
  data?: Record<string, any>
  createdAt?: string
  updatedAt?: string
}

export interface RegistryProvider {
  id: string
  name: string
  category: string
  authType: string
  authModes?: string[]
  baseUrl?: string
  noAuth?: boolean
  hasFree?: boolean
  deviceCodeUrl?: string
  loginUrl?: string
  authorizeUrl?: string
  headers?: Record<string, string>
  models?: string[]
  apiKeyUrl?: string
  brand?: string
  region?: string
  authHint?: string
}

export interface RegistryCategory {
  category: string
  count: number
  providers: RegistryProvider[]
}

export interface Combo {
  id: string
  name: string
  kind: string
  models: string[]
  createdAt?: string
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

export interface Endpoint {
  label: string
  url: string
  type: string
}

export interface ProviderModel {
  name: string
  enabled?: boolean
  alias?: string
  contextLength?: number
  maxOutputTokens?: number
  capabilities?: string[]
  modalities?: string[]
  displayName?: string
  source?: string
}

export interface UsageStats {
  totalRequests?: number
  totalPromptTokens?: number
  totalCompletionTokens?: number
  totalCost?: number
  totalRequestsLifetime?: number
  byProvider?: Record<string, { requests: number; promptTokens: number; completionTokens: number }>
}

export interface RequestDetail {
  id?: string
  timestamp?: string
  provider?: string
  model?: string
  status?: string
  promptTokens?: number
  completionTokens?: number
  cost?: number
  latencyMs?: number
  connectionId?: string
  endpoint?: string
}

export interface Pagination {
  page: number
  pageSize: number
  totalItems: number
  totalPages: number
  hasNext: boolean
  hasPrev: boolean
}

export interface ProviderUsage {
  provider: string
  requests: number
  promptTokens: number
  completionTokens: number
  cost: number
  connections: number
  activeConnections: number
  quotaLimit?: number
  quotaUsed?: number
  overQuota?: boolean
}

export interface CLITool {
  id: string
  name: string
  description?: string
  icon?: string
  configType?: string
  configured?: boolean
}

export interface Skill {
  id: string
  name: string
  description?: string
  content?: string
}

/* ─── Store ─── */

export const useGatewayStore = defineStore('gateway', () => {
  const toast = useToast()

  // Core state
  const version = ref('…')
  const health = ref<Record<string, any>>({})
  const providers = ref<Provider[]>([])
  const combos = ref<Combo[]>([])
  const apiKeys = ref<ApiKey[]>([])
  const proxyPools = ref<ProxyPool[]>([])
  const endpoints = ref<Endpoint[]>([])
  const registryCategories = ref<RegistryCategory[]>([])
  const settings = ref<Record<string, any>>({})
  const aliases = ref<Record<string, string>>({})

  // Usage state
  const usageStats = ref<UsageStats>({})
  const usageChart = ref<{ label: string; tokens: number }[]>([])
  const requestDetails = ref<RequestDetail[]>([])
  const requestDetailsPagination = ref<Pagination>({ page: 1, pageSize: 20, totalItems: 0, totalPages: 0, hasNext: false, hasPrev: false })
  const providerUsage = ref<ProviderUsage[]>([])
  const usageLogs = ref<any[]>([])
  const quotaEntries = ref<any[]>([])

  // Derived
  const registryList = computed(() =>
    registryCategories.value.flatMap(c => c.providers).sort((a, b) => a.name.localeCompare(b.name))
  )
  const activeConnections = computed(() => providers.value.filter(p => p.isActive).length)

  /* ─── Loaders ─── */

  async function loadCore() {
    try {
      const [v, h, p, c, reg, ep, a] = await Promise.all([
        api('/api/version'),
        api('/api/health'),
        api('/api/providers'),
        api('/api/combos'),
        api('/api/registry'),
        api('/api/endpoints'),
        api('/api/models/alias'),
      ])
      version.value = v?.version || 'dev'
      health.value = h || {}
      providers.value = Array.isArray(p) ? p : []
      combos.value = Array.isArray(c) ? c : []
      registryCategories.value = Array.isArray(reg?.categories) ? reg.categories : []
      endpoints.value = Array.isArray(ep?.endpoints) ? ep.endpoints : []
      aliases.value = a || {}
    } catch (e) {
      console.error('[store] loadCore failed:', e)
    }
    loadKeys()
    loadProxyPools()
  }

  async function loadKeys() {
    try {
      const r = await api('/api/keys')
      apiKeys.value = Array.isArray(r) ? r : []
    } catch { apiKeys.value = [] }
  }

  async function loadProxyPools() {
    try {
      const r = await api('/api/proxy-pools')
      proxyPools.value = Array.isArray(r?.proxyPools) ? r.proxyPools : (Array.isArray(r) ? r : [])
    } catch { proxyPools.value = [] }
  }

  async function loadSettings() {
    try { settings.value = (await api('/api/settings')) || {} } catch { settings.value = {} }
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
      requestDetails.value = Array.isArray(details?.details) ? details.details : (Array.isArray(details) ? details : [])
    } catch (e) { console.error('[store] loadUsage failed:', e) }
  }

  async function loadRequestDetails(page = 1, pageSize = 20, filters: Record<string, string> = {}) {
    try {
      const params = new URLSearchParams({ page: String(page), pageSize: String(pageSize) })
      for (const [k, v] of Object.entries(filters)) { if (v) params.set(k, v) }
      const res = await api(`/api/usage/request-details?${params}`)
      requestDetails.value = Array.isArray(res?.details) ? res.details : []
      requestDetailsPagination.value = res?.pagination || { page, pageSize, totalItems: 0, totalPages: 0, hasNext: false, hasPrev: false }
    } catch (e) { console.error('[store] loadRequestDetails failed:', e) }
  }

  async function loadProviderUsage(period = '7d') {
    try {
      const res = await api(`/api/usage/providers?period=${period}`)
      providerUsage.value = Array.isArray(res?.providers) ? res.providers : []
    } catch { providerUsage.value = [] }
  }

  async function loadUsageLogs(limit = 50) {
    try {
      const res = await api(`/api/usage/logs?limit=${limit}`)
      usageLogs.value = Array.isArray(res?.logs) ? res.logs : []
    } catch { usageLogs.value = [] }
  }

  async function loadQuota() {
    try {
      const res = await api('/api/quota')
      quotaEntries.value = Array.isArray(res?.entries) ? res.entries : []
    } catch { quotaEntries.value = [] }
  }

  /* ─── Provider actions ─── */

  async function addProvider(payload: Record<string, any>) {
    const res = await apiPost<Provider>('/api/providers', payload)
    toast.success(`Provider "${payload.name || payload.provider}" added`)
    if (res?.id) {
      providers.value.push(res)
    } else {
      await loadProvidersOnly()
    }
  }

  async function loadProvidersOnly() {
    try {
      const p = await api('/api/providers')
      providers.value = Array.isArray(p) ? p : []
    } catch (e) {
      console.error('[store] loadProvidersOnly failed:', e)
    }
  }

  async function toggleProvider(p: Provider) {
    const originalState = p.isActive
    p.isActive = !p.isActive
    try {
      await apiPut(`/api/providers/${p.id}`, { isActive: p.isActive })
    } catch (e) {
      p.isActive = originalState
      toast.error('Failed to update provider status')
    }
  }

  async function resetCooldown(p: Provider) {
    await apiPost(`/api/providers/${p.id}/reset`)
    toast.success(`Cooldown reset — ${p.name || p.provider}`)
    if (p.data) {
      p.data.rateLimitedUntil = ''
      p.data.testStatus = 'ok'
    }
  }

  async function deleteProvider(p: Provider) {
    await apiDelete(`/api/providers/${p.id}`)
    toast.success(`Deleted "${p.name || p.provider}"`)
    providers.value = providers.value.filter(item => item.id !== p.id)
  }

  async function enableFree(ids?: string[]): Promise<number> {
    const res = await apiPost('/api/providers/enable-free', ids?.length ? { providers: ids } : {})
    const count = res?.count || 0
    toast.success(count > 0 ? `Enabled ${count} free provider${count === 1 ? '' : 's'}` : 'Free providers already enabled')
    await loadCore()
    return count
  }

  async function testProvider(id: string): Promise<{ ok: boolean; latencyMs?: number; error?: string }> {
    return apiPost(`/api/providers/${id}/test`)
  }

  /* ─── Key actions ─── */

  async function createKey(name: string) {
    const k = await apiPost('/api/keys', { name })
    toast.success(`Key "${name || k?.id?.slice(0, 8)}" created`)
    await loadKeys()
    return k
  }

  async function deleteKey(id: string) {
    await apiDelete(`/api/keys/${id}`)
    toast.success('Key deleted')
    await loadKeys()
  }

  /* ─── Combo actions ─── */

  async function saveCombo(payload: { id?: string; name: string; kind: string; models: string[] }) {
    if (payload.id) {
      await apiPut(`/api/combos/${payload.id}`, payload)
    } else {
      await apiPost('/api/combos', payload)
    }
    toast.success(`Combo "${payload.name}" saved`)
    await loadCore()
  }

  async function deleteCombo(id: string) {
    await apiDelete(`/api/combos/${id}`)
    toast.success('Combo deleted')
    await loadCore()
  }

  /* ─── Proxy pool actions ─── */

  async function saveProxyPool(payload: { id?: string; name: string; type: string; proxyUrl: string; noProxy?: string; strictProxy?: boolean }) {
    if (payload.id) {
      await apiPut(`/api/proxy-pools/${payload.id}`, payload)
    } else {
      await apiPost('/api/proxy-pools', payload)
    }
    toast.success(`Proxy pool "${payload.name}" saved`)
    await loadProxyPools()
  }

  async function toggleProxyPool(pp: ProxyPool) {
    await apiPut(`/api/proxy-pools/${pp.id}`, { isActive: !pp.isActive })
    await loadProxyPools()
  }

  async function deleteProxyPool(id: string) {
    await apiDelete(`/api/proxy-pools/${id}`)
    toast.success('Proxy pool deleted')
    await loadProxyPools()
  }

  /* ─── Alias actions ─── */

  async function addAlias(alias: string, target: string) {
    await apiPost('/api/models/alias', { alias, target })
    toast.success(`Alias "${alias}" → ${target}`)
    aliases.value = { ...aliases.value, [alias]: target }
  }

  async function deleteAlias(alias: string) {
    await apiDelete('/api/models/alias', { alias })
    toast.success(`Alias "${alias}" removed`)
    const next = { ...aliases.value }
    delete next[alias]
    aliases.value = next
  }

  /* ─── Settings actions ─── */

  async function saveSettings(patch: Record<string, any>) {
    await apiPatch('/api/settings', patch)
    toast.success('Settings saved')
    await loadSettings()
  }

  async function setPassword(password: string) {
    return apiPost('/api/auth/password', { password })
  }

  return {
    // State
    version, health, providers, combos, apiKeys, proxyPools, endpoints,
    registryCategories, registryList, settings, aliases,
    usageStats, usageChart, requestDetails, requestDetailsPagination,
    providerUsage, usageLogs, quotaEntries, activeConnections,
    // Loaders
    loadCore, loadKeys, loadProxyPools, loadSettings, loadUsage,
    loadRequestDetails, loadProviderUsage, loadUsageLogs, loadQuota,
    // Actions
    addProvider, toggleProvider, resetCooldown, deleteProvider, enableFree, testProvider,
    createKey, deleteKey,
    saveCombo, deleteCombo,
    saveProxyPool, toggleProxyPool, deleteProxyPool,
    addAlias, deleteAlias,
    saveSettings, setPassword,
  }
})
