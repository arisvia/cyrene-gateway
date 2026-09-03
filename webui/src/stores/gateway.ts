import { createSignal, createRoot } from 'solid-js'
import { createStore, produce } from 'solid-js/store'
import { api, apiPost, apiPut, apiPatch, apiDelete } from '@/lib/api'
import { useToast } from '@/lib/toast'
import type {
  Provider, RegistryCategory, Combo, ApiKey, ProxyPool, Endpoint,
  UsageStats, RequestDetail, Pagination, ProviderUsage,
} from '@/types/domain'

function createGatewayStore() {
  const toast = useToast()

  // ── state ──
  const [version, setVersion] = createSignal('…')
  const [health, setHealth] = createSignal<Record<string, any>>({})
  const [providers, setProviders] = createSignal<Provider[]>([])
  const [combos, setCombos] = createSignal<Combo[]>([])
  const [apiKeys, setApiKeys] = createSignal<ApiKey[]>([])
  const [proxyPools, setProxyPools] = createSignal<ProxyPool[]>([])
  const [endpoints, setEndpoints] = createSignal<Endpoint[]>([])
  const [registryCategories, setRegistryCategories] = createSignal<RegistryCategory[]>([])
  const [settings, setSettings] = createSignal<Record<string, any>>({})
  const [aliases, setAliases] = createSignal<Record<string, string>>({})

  const [usageStats, setUsageStats] = createStore<UsageStats>({})
  const [usageChart, setUsageChart] = createSignal<{ label: string; tokens: number }[]>([])
  const [requestDetails, setRequestDetails] = createSignal<RequestDetail[]>([])
  const [requestDetailsPagination, setRequestDetailsPagination] = createSignal<Pagination>(
    { page: 1, pageSize: 20, totalItems: 0, totalPages: 0, hasNext: false, hasPrev: false },
  )
  const [providerUsage, setProviderUsage] = createSignal<ProviderUsage[]>([])
  const [usageLogs, setUsageLogs] = createSignal<any[]>([])
  const [quotaEntries, setQuotaEntries] = createSignal<any[]>([])

  const registryList = () =>
    registryCategories().flatMap(c => c.providers).sort((a, b) => a.name.localeCompare(b.name))
  const activeConnections = () => providers().filter(p => p.isActive).length

  // ── loaders ──
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
      setVersion(v?.version || 'dev')
      setHealth(h || {})
      setProviders(Array.isArray(p) ? p : [])
      setCombos(Array.isArray(c) ? c : [])
      setRegistryCategories(Array.isArray(reg?.categories) ? reg.categories : [])
      setEndpoints(Array.isArray(ep?.endpoints) ? ep.endpoints : [])
      setAliases(a || {})
    } catch (e) {
      console.error('[store] loadCore failed:', e)
    }
    loadKeys()
    loadProxyPools()
  }

  async function loadProvidersOnly() {
    try {
      const p = await api('/api/providers')
      setProviders(Array.isArray(p) ? p : [])
    } catch (e) {
      console.error('[store] loadProvidersOnly failed:', e)
    }
  }

  async function loadKeys() {
    try {
      const r = await api('/api/keys')
      setApiKeys(Array.isArray(r) ? r : [])
    } catch { setApiKeys([]) }
  }

  async function loadProxyPools() {
    try {
      const r = await api('/api/proxy-pools')
      setProxyPools(Array.isArray(r?.proxyPools) ? r.proxyPools : (Array.isArray(r) ? r : []))
    } catch { setProxyPools([]) }
  }

  async function loadSettings() {
    try { setSettings((await api('/api/settings')) || {}) } catch { setSettings({}) }
  }

  async function loadUsage(period: string) {
    try {
      const [stats, chart, details] = await Promise.all([
        api(`/api/usage/stats?period=${period}`),
        api(`/api/usage/chart?period=${period}`),
        api('/api/usage/request-details?page=1&pageSize=15'),
      ])
      setUsageStats(stats || {})
      setUsageChart(Array.isArray(chart) ? chart : [])
      setRequestDetails(Array.isArray(details?.details) ? details.details : (Array.isArray(details) ? details : []))
    } catch (e) { console.error('[store] loadUsage failed:', e) }
  }

  async function loadRequestDetails(page = 1, pageSize = 20, filters: Record<string, string> = {}) {
    try {
      const params = new URLSearchParams({ page: String(page), pageSize: String(pageSize) })
      for (const [k, v] of Object.entries(filters)) { if (v) params.set(k, v) }
      const res = await api(`/api/usage/request-details?${params}`)
      setRequestDetails(Array.isArray(res?.details) ? res.details : [])
      setRequestDetailsPagination(res?.pagination || { page, pageSize, totalItems: 0, totalPages: 0, hasNext: false, hasPrev: false })
    } catch (e) { console.error('[store] loadRequestDetails failed:', e) }
  }

  async function loadProviderUsage(period = '7d') {
    try {
      const res = await api(`/api/usage/providers?period=${period}`)
      setProviderUsage(Array.isArray(res?.providers) ? res.providers : [])
    } catch { setProviderUsage([]) }
  }

  async function loadUsageLogs(limit = 50) {
    try {
      const res = await api(`/api/usage/logs?limit=${limit}`)
      setUsageLogs(Array.isArray(res?.logs) ? res.logs : [])
    } catch { setUsageLogs([]) }
  }

  async function loadQuota() {
    try {
      const res = await api('/api/quota')
      setQuotaEntries(Array.isArray(res?.entries) ? res.entries : [])
    } catch { setQuotaEntries([]) }
  }

  // ── provider actions ──
  async function addProvider(payload: Record<string, any>) {
    const res = await apiPost<Provider>('/api/providers', payload)
    toast.success(`Provider "${payload.name || payload.provider}" added`)
    // 以服务端返回的 DTO（含后端生成的 ID/默认值）为准刷新列表，而非本地拼接
    await loadProvidersOnly()
    return res
   }

  async function toggleProvider(p: Provider) {
    const original = p.isActive
    setProviders(list => list.map(x => x.id === p.id ? { ...x, isActive: !original } : x))
    try {
      await apiPut(`/api/providers/${p.id}`, { isActive: !original })
    } catch {
      setProviders(list => list.map(x => x.id === p.id ? { ...x, isActive: original } : x))
      toast.error('Failed to update provider status')
    }
  }

  async function resetCooldown(p: Provider) {
    await apiPost(`/api/providers/${p.id}/reset`)
    toast.success(`Cooldown reset — ${p.name || p.provider}`)
    setProviders(list => list.map(x => x.id === p.id
      ? { ...x, data: { ...x.data, rateLimitedUntil: '', testStatus: 'active' } } : x))
  }

  async function deleteProvider(p: Provider) {
    await apiDelete(`/api/providers/${p.id}`)
    toast.success(`Deleted "${p.name || p.provider}"`)
    setProviders(list => list.filter(item => item.id !== p.id))
  }

  async function enableFree(ids?: string[]): Promise<number> {
    const res = await apiPost('/api/providers/enable-free', ids?.length ? { providers: ids } : {})
    const count = res?.count || 0
    toast.success(count > 0 ? `Enabled ${count} free provider${count === 1 ? '' : 's'}` : 'Free providers already enabled')
    await loadCore()
    return count
  }

  async function testProvider(id: string): Promise<{ ok: boolean; latencyMs?: number; error?: string; code?: number; latency?: string }> {
    return apiPost(`/api/providers/${id}/test`)
  }

  // ── keys ──
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

  // ── combos ──
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

  // ── proxy pools ──
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

  // ── aliases ──
  async function addAlias(alias: string, target: string) {
    await apiPost('/api/models/alias', { alias, target })
    toast.success(`Alias "${alias}" → ${target}`)
    setAliases(a => ({ ...a, [alias]: target }))
  }

  async function deleteAlias(alias: string) {
    await apiDelete('/api/models/alias', { alias })
    toast.success(`Alias "${alias}" removed`)
    setAliases(a => {
      const next = { ...a }
      delete next[alias]
      return next
    })
  }

  // ── settings ──
  async function saveSettings(patch: Record<string, any>) {
    await apiPatch('/api/settings', patch)
    toast.success('Settings saved')
    await loadSettings()
  }

  async function setPassword(password: string) {
    return apiPost('/api/auth/password', { password })
  }

  // ── provider 编辑 / 凭证 ──
  async function updateProvider(id: string, patch: Record<string, any>) {
    const res = await apiPut(`/api/providers/${id}`, patch)
    await loadProvidersOnly()
    return res
  }

  async function testCredentials(payload: Record<string, any>) {
    return apiPost('/api/providers/test-credentials', payload)
  }

  async function testBatch(ids: string[]) {
    return apiPost('/api/providers/test-batch', { ids })
  }

  async function refreshModels(id: string) {
    return apiPost(`/api/providers/${id}/refresh-models`)
  }

  // ── provider 模型（registry + 自定义）──
  async function loadProviderModels(id: string) {
    return api(`/api/providers/${id}/models`)
  }

  async function addProviderModel(id: string, model: { id: string; name?: string }) {
    return apiPost(`/api/providers/${id}/models`, model)
  }

  async function deleteProviderModel(id: string, modelId: string) {
    return apiDelete(`/api/providers/${id}/models`, { id: modelId })
  }

  async function saveProviderModelMeta(id: string, meta: { id: string; displayName?: string; contextLength?: number; maxOutputTokens?: number }) {
    return apiPost(`/api/providers/${id}/models/meta`, meta)
  }

  async function resetProviderModelMeta(id: string, modelId: string) {
    return apiDelete(`/api/providers/${id}/models/meta`, { id: modelId })
  }
  // ── OAuth 流程 ──
  // authorize 后端只注册 GET（用 POST 会 405）；device-code 为 POST
  async function oauthStart(providerId: string, mode: 'authorize' | 'device-code', extra: Record<string, any> = {}) {
    if (mode === 'authorize') {
      const params = new URLSearchParams(extra as Record<string, string>)
      const qs = params.toString()
      return api(`/api/oauth/${providerId}/authorize${qs ? `?${qs}` : ''}`)
    }
    return apiPost(`/api/oauth/${providerId}/${mode}`, extra)
  }

  async function oauthPoll(providerId: string, payload: Record<string, any>) {
    return apiPost(`/api/oauth/${providerId}/device-code/poll`, payload)
  }

  async function oauthImport(providerId: string, payload: Record<string, any>) {
    return apiPost(`/api/oauth/${providerId}/import`, payload)
  }

  async function oauthStatus(providerId: string) {
    return api(`/api/oauth/${providerId}/status`)
  }

  async function oauthRefresh(providerId: string) {
    return apiPost(`/api/oauth/${providerId}/refresh`)
  }

  // ── provider 节点 ──
  async function loadNodes() {
    try {
      const r = await api('/api/provider-nodes')
      return Array.isArray(r) ? r : []
    } catch { return [] }
  }

  async function saveNode(payload: Record<string, any>) {
    if (payload.id) return apiPut(`/api/provider-nodes/${payload.id}`, payload)
    return apiPost('/api/provider-nodes', payload)
  }

  async function deleteNode(id: string) {
    return apiDelete(`/api/provider-nodes/${id}`)
  }

  // ── 模型禁用 / 启用 ──
  async function loadDisabledModels() {
    try { return (await api('/api/models/disabled')) || {} } catch { return {} }
  }

  async function setModelDisabled(model: string, disabled: boolean) {
    if (disabled) return apiPost('/api/models/disabled', { model })
    return apiDelete('/api/models/disabled', { model })
  }
  return {
    version, health, providers, setProviders, combos, apiKeys, proxyPools, endpoints,
    registryCategories, registryList, settings, aliases,
    usageStats, usageChart, requestDetails, requestDetailsPagination,
    providerUsage, usageLogs, quotaEntries, activeConnections,
    loadCore, loadProvidersOnly, loadKeys, loadProxyPools, loadSettings, loadUsage,
    loadRequestDetails, loadProviderUsage, loadUsageLogs, loadQuota,
    addProvider, toggleProvider, resetCooldown, deleteProvider, enableFree, testProvider,
    createKey, deleteKey,
    saveCombo, deleteCombo,
    saveProxyPool, toggleProxyPool, deleteProxyPool,
    addAlias, deleteAlias,
    saveSettings, setPassword,
    updateProvider, testCredentials, testBatch, refreshModels,
    loadProviderModels, addProviderModel, deleteProviderModel,
    saveProviderModelMeta, resetProviderModelMeta,
    oauthStart, oauthPoll, oauthImport, oauthStatus, oauthRefresh,
    loadNodes, saveNode, deleteNode,
    loadDisabledModels, setModelDisabled,
  }
}

// 单例：整个应用共享同一份信号（此前每次调用都新建一份 → 页面读到空数据）
let _store: ReturnType<typeof createGatewayStore> | null = null

export function useGatewayStore() {
  if (!_store) _store = createRoot(createGatewayStore)
  return _store
}
