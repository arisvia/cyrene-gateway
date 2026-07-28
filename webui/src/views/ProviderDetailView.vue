<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useGatewayStore, type Provider } from '@/stores/gateway'
import { api, apiPost, apiPut, apiDelete, apiPatch } from '@/lib/api'
import { useToast } from '@/lib/toast'
import GButton from '@/components/ui/GButton.vue'
import GBadge from '@/components/ui/GBadge.vue'
import GCard from '@/components/ui/GCard.vue'
import GSwitch from '@/components/ui/GSwitch.vue'
import GEmpty from '@/components/ui/GEmpty.vue'
import {
  ArrowLeft, Zap, Save, Plus, Trash2, RotateCcw, Server, Key, ExternalLink,
  ChevronUp, ChevronDown, Copy, Check, FlaskConical, X, Settings2, Brain,
  Download, ArrowUpDown, Link2
} from 'lucide-vue-next'

const route = useRoute()
const router = useRouter()
const store = useGatewayStore()
const toast = useToast()

const providerId = computed(() => route.params.id as string)
const registryInfo = computed(() => store.registryList.find(p => p.id === providerId.value))
const connections = computed(() =>
  [...store.providers.filter(p => p.provider === providerId.value)].sort((a, b) => (a.priority || 0) - (b.priority || 0))
)

// --- Connection selection ---
const selectedConn = ref<Provider | null>(null)
const editData = ref({ apiKey: '', baseUrl: '' })
const testing = ref(false)
const testResult = ref<any>(null)

// --- Models ---
const registryModels = ref<any[]>([])
const customModels = ref<any[]>([])
const newModel = ref({ id: '', name: '' })
const disabledModelIds = ref<string[]>([])
const modelTestResults = ref<Record<string, { ok: boolean; latency?: string; error?: string }>>({})
const testingModelIds = ref<Set<string>>(new Set())
const copiedModel = ref('')

// --- Aliases ---
const modelAliases = ref<Record<string, string>>({})
const aliasEditId = ref('')
const aliasEditValue = ref('')

// --- Settings: strategy + thinking ---
const providerStrategy = ref<string | null>(null)
const providerStickyLimit = ref('1')
const thinkingMode = ref('auto')

// --- Bulk selection ---
const selectedConnectionIds = ref<string[]>([])
const showBulkProxy = ref(false)
const bulkUpdating = ref(false)

// --- OAuth ---
const oauthStatus = ref<any>(null)
const showOAuth = ref(false)
const oauthMode = ref<'authorize' | 'device' | 'import'>('authorize')
const oauthDeviceCode = ref<any>(null)
const oauthToken = ref('')
const oauthPolling = ref(false)
let pollTimer: ReturnType<typeof setInterval> | null = null

// --- Add connection ---
const showAddConn = ref(false)
const newConn = ref({ name: '', authType: 'api-key', priority: 0, data: { apiKey: '', baseUrl: '' } })

// --- Computed helpers ---
const isOAuthOnly = computed(() => {
  const rp = registryInfo.value
  if (!rp) return false
  return rp.authType === 'oauth' && !(rp.authModes || []).includes('api-key')
})
const isNoAuth = computed(() => registryInfo.value?.authType === 'none' || registryInfo.value?.noAuth === true)

const ENDPOINT_SUFFIXES = ['/chat/completions', '/v1/messages', '/messages', '/responses', '/generate']
function stripBase(url?: string): string {
  if (!url) return ''
  let base = url.replace(/\/+$/, '')
  const lower = base.toLowerCase()
  for (const sfx of ENDPOINT_SUFFIXES) {
    if (lower.endsWith(sfx)) { base = base.slice(0, base.length - sfx.length).replace(/\/+$/, ''); break }
  }
  return base
}
const basePlaceholder = computed(() => stripBase(registryInfo.value?.baseUrl) || 'https://api.openai.com/v1')

// Provider alias for model strings
const providerAlias = computed(() => providerId.value)

// Thinking levels available for this provider
const thinkingLevels = computed(() => {
  // Reasoning-capable providers
  const reasoningProviders = ['openrouter', 'anthropic', 'openai', 'gemini', 'deepseek', 'qwen', 'kimi', 'groq', 'xai', 'mistral']
  if (!reasoningProviders.includes(providerId.value)) return null
  return ['auto', 'minimal', 'medium', 'high', 'max']
})

// All models (registry + custom, minus disabled)
const allModels = computed(() => {
  const reg = registryModels.value.map(m => ({ id: m.id, name: m.name, source: 'registry' as const }))
  const cust = customModels.value.map(m => ({ id: m.id, name: m.name || m.id, source: 'custom' as const }))
  return [...cust, ...reg]
})
const enabledModels = computed(() => allModels.value.filter(m => !disabledModelIds.value.includes(m.id)))
const disabledModels = computed(() => allModels.value.filter(m => disabledModelIds.value.includes(m.id)))

// --- Connection actions ---
async function selectConnection(conn: Provider) {
  selectedConn.value = conn
  editData.value = { apiKey: conn.data?.apiKey || '', baseUrl: conn.data?.baseUrl || '' }
  testResult.value = null
  if (conn.authType === 'oauth') {
    try { oauthStatus.value = await api(`/api/oauth/${conn.provider}/status`) } catch { oauthStatus.value = null }
  } else {
    oauthStatus.value = null
  }
}

async function saveCredentials() {
  const conn = selectedConn.value
  if (!conn) return
  const data = { ...(conn.data || {}), apiKey: editData.value.apiKey, baseUrl: editData.value.baseUrl }
  await apiPut(`/api/providers/${conn.id}`, { data })
  toast.success('Credentials saved')
  await store.loadAll()
  const fresh = store.providers.find(x => x.id === conn.id)
  if (fresh) selectedConn.value = fresh
}

async function testConnection() {
  const conn = selectedConn.value
  if (!conn) return
  testing.value = true
  testResult.value = null
  try {
    testResult.value = await apiPost(`/api/providers/${conn.id}/test`)
  } catch (e) { testResult.value = { ok: false, error: String(e) } }
  testing.value = false
  await store.loadAll()
  const fresh = store.providers.find(x => x.id === conn.id)
  if (fresh) selectedConn.value = fresh
}

async function toggleConn(conn: Provider) {
  await store.toggleProvider(conn)
  if (selectedConn.value?.id === conn.id) {
    const fresh = store.providers.find(x => x.id === conn.id)
    if (fresh) selectedConn.value = fresh
  }
}

async function resetConn(conn: Provider) {
  await store.resetProvider(conn)
  if (selectedConn.value?.id === conn.id) {
    const fresh = store.providers.find(x => x.id === conn.id)
    if (fresh) selectedConn.value = fresh
  }
}

async function deleteConn(conn: Provider) {
  if (!confirm(`Delete connection "${conn.name || conn.provider}"?`)) return
  await store.deleteProvider(conn)
  if (selectedConn.value?.id === conn.id) selectedConn.value = null
}

// --- Priority reorder ---
async function swapPriority(index1: number, index2: number) {
  const conns = connections.value
  if (index1 < 0 || index2 < 0 || index1 >= conns.length || index2 >= conns.length) return
  const c1 = conns[index1], c2 = conns[index2]
  await Promise.all([
    apiPut(`/api/providers/${c1.id}`, { priority: index2 }),
    apiPut(`/api/providers/${c2.id}`, { priority: index1 }),
  ])
  await store.loadAll()
}

// --- Per-connection proxy pool ---
async function updateConnProxy(conn: Provider, poolId: string | null) {
  const specific = { ...(conn.data?.providerSpecificData || {}) }
  if (poolId) specific.proxyPoolId = poolId
  else delete specific.proxyPoolId
  const data = { ...(conn.data || {}), providerSpecificData: specific }
  await apiPut(`/api/providers/${conn.id}`, { data })
  await store.loadAll()
}

// --- Model test ---
async function testModel(modelId: string) {
  if (testingModelIds.value.has(modelId)) return
  testingModelIds.value.add(modelId)
  try {
    const res = await apiPost('/api/models/test', { model: `${providerAlias.value}/${modelId}` })
    modelTestResults.value[modelId] = res
  } catch (e: any) {
    modelTestResults.value[modelId] = { ok: false, error: e.message }
  }
  testingModelIds.value.delete(modelId)
}

// --- Model enable/disable ---
async function disableModel(modelId: string) {
  await apiPost('/api/models/disabled', { providerAlias: providerAlias.value, ids: [modelId] })
  disabledModelIds.value.push(modelId)
}

async function enableModel(modelId: string) {
  await apiDelete(`/api/models/disabled?providerAlias=${encodeURIComponent(providerAlias.value)}&id=${encodeURIComponent(modelId)}`)
  disabledModelIds.value = disabledModelIds.value.filter(id => id !== modelId)
}

async function disableAll() {
  const ids = enabledModels.value.map(m => m.id)
  if (!ids.length) return
  if (!confirm(`Disable all ${ids.length} model(s)?`)) return
  await apiPost('/api/models/disabled', { providerAlias: providerAlias.value, ids })
  disabledModelIds.value = [...new Set([...disabledModelIds.value, ...ids])]
}

async function enableAllModels() {
  await apiDelete(`/api/models/disabled?providerAlias=${encodeURIComponent(providerAlias.value)}`)
  disabledModelIds.value = []
}

// --- Model alias ---
async function fetchAliases() {
  try {
    const res = await api('/api/models/alias')
    modelAliases.value = res?.aliases || {}
  } catch { /* ignore */ }
}

function getAliasForModel(modelId: string): string | undefined {
  const fullModel = `${providerAlias.value}/${modelId}`
  return Object.entries(modelAliases.value).find(([, m]) => m === fullModel)?.[0]
}

async function setAlias(modelId: string, alias: string) {
  if (!alias) return
  const fullModel = `${providerAlias.value}/${modelId}`
  await apiPut('/api/models/alias', { model: fullModel, alias })
  await fetchAliases()
  aliasEditId.value = ''
  toast.success(`Alias "${alias}" set`)
}

async function deleteAlias(alias: string) {
  await apiDelete(`/api/models/alias?alias=${encodeURIComponent(alias)}`)
  await fetchAliases()
}

function copyModel(modelId: string) {
  const fullModel = `${providerAlias.value}/${modelId}`
  navigator.clipboard.writeText(fullModel)
  copiedModel.value = modelId
  setTimeout(() => { copiedModel.value = '' }, 1500)
}

// --- Custom models ---
async function addCustomModel() {
  const conn = selectedConn.value || connections.value[0]
  if (!conn || !newModel.value.id) return
  const res = await apiPost(`/api/providers/${conn.id}/models`, newModel.value)
  if (res?.customModels) customModels.value = res.customModels
  newModel.value = { id: '', name: '' }
}

async function removeCustomModel(m: any) {
  const conn = selectedConn.value || connections.value[0]
  if (!conn) return
  const res = await apiDelete(`/api/providers/${conn.id}/models`, { id: m.id })
  if (res?.customModels) customModels.value = res.customModels
}

// --- Qoder model import ---
const importingModels = ref(false)
async function importQoderModels() {
  const conn = connections.value.find(c => c.isActive)
  if (!conn) { toast.error('No active connection'); return }
  importingModels.value = true
  try {
    const res = await api(`/api/providers/${conn.id}/models`)
    const models = res?.registryModels || []
    let imported = 0
    for (const m of models) {
      const id = (m.id || '').replace(/^qoder\//, '')
      if (!id) continue
      if (customModels.value.some(cm => cm.id === id)) continue
      const addRes = await apiPost(`/api/providers/${conn.id}/models`, { id, name: m.name || id })
      if (addRes?.customModels) customModels.value = addRes.customModels
      imported++
    }
    toast.success(imported > 0 ? `Imported ${imported} models` : 'All models already exist')
  } catch (e: any) {
    toast.error(`Import failed: ${e.message}`)
  }
  importingModels.value = false
}

// --- Provider strategy ---
async function loadSettings() {
  try {
    const settings = await api('/api/settings')
    const override = settings?.providerStrategies?.[providerId.value]
    providerStrategy.value = override?.fallbackStrategy || null
    providerStickyLimit.value = override?.stickyRoundRobinLimit != null ? String(override.stickyRoundRobinLimit) : '1'
    const thinkCfg = settings?.providerThinking?.[providerId.value]
    thinkingMode.value = thinkCfg?.mode || 'auto'
  } catch { /* ignore */ }
}

async function saveStrategy(strategy: string | null, stickyLimit: string) {
  const settings = await api('/api/settings')
  const current = settings?.providerStrategies || {}
  const updated = { ...current }
  if (!strategy) {
    delete updated[providerId.value]
  } else {
    updated[providerId.value] = { fallbackStrategy: strategy, stickyRoundRobinLimit: Number(stickyLimit) || 1 }
  }
  await apiPatch('/api/settings', { providerStrategies: updated })
  providerStrategy.value = strategy
}

async function saveThinking(mode: string) {
  const settings = await api('/api/settings')
  const current = settings?.providerThinking || {}
  const updated = { ...current }
  if (!mode || mode === 'auto') delete updated[providerId.value]
  else updated[providerId.value] = { mode }
  await apiPatch('/api/settings', { providerThinking: updated })
  thinkingMode.value = mode
}

// --- Bulk proxy ---
const allSelected = computed(() => connections.value.length > 0 && selectedConnectionIds.value.length === connections.value.length)

function toggleSelectAll() {
  if (allSelected.value) selectedConnectionIds.value = []
  else selectedConnectionIds.value = connections.value.map(c => c.id)
}

function toggleSelect(id: string) {
  const idx = selectedConnectionIds.value.indexOf(id)
  if (idx >= 0) selectedConnectionIds.value.splice(idx, 1)
  else selectedConnectionIds.value.push(id)
}

async function applyBulkProxy(poolId: string | null) {
  bulkUpdating.value = true
  try {
    for (const id of selectedConnectionIds.value) {
      const conn = store.providers.find(p => p.id === id)
      if (conn) await updateConnProxy(conn, poolId)
    }
    toast.success('Proxy pool applied')
    showBulkProxy.value = false
    selectedConnectionIds.value = []
  } catch (e: any) {
    toast.error(`Bulk update failed: ${e.message}`)
  }
  bulkUpdating.value = false
}

async function applyOneToOne() {
  const activePools = store.proxyPools.filter(p => p.isActive)
  if (!activePools.length) { toast.error('No active proxy pools'); return }
  bulkUpdating.value = true
  try {
    for (let i = 0; i < connections.value.length; i++) {
      await updateConnProxy(connections.value[i], activePools[i % activePools.length].id)
    }
    toast.success('One-to-one proxy rotation applied')
    showBulkProxy.value = false
  } catch (e: any) {
    toast.error(`Failed: ${e.message}`)
  }
  bulkUpdating.value = false
}

// --- Add connection ---
function openAddConn() {
  const rp = registryInfo.value
  if (rp) newConn.value.authType = rp.authType || 'api-key'
  newConn.value.data.baseUrl = ''
  showAddConn.value = true
}

async function addConnection() {
  await store.addProvider({
    provider: providerId.value,
    name: newConn.value.name,
    authType: newConn.value.authType,
    priority: newConn.value.priority,
    data: newConn.value.data,
  })
  newConn.value = { name: '', authType: 'api-key', priority: 0, data: { apiKey: '', baseUrl: '' } }
  showAddConn.value = false
}

// --- OAuth ---
function openOAuth() {
  const rp = registryInfo.value
  if (rp?.deviceCodeUrl || rp?.loginUrl || providerId.value === 'qoder') oauthMode.value = 'device'
  else if (rp?.authorizeUrl) oauthMode.value = 'authorize'
  else oauthMode.value = 'import'
  oauthDeviceCode.value = null
  showOAuth.value = true
}

async function startOAuthAuthorize() {
  try {
    const res = await api(`/api/oauth/${providerId.value}/authorize`)
    if (res?.url) window.open(res.url, '_blank')
  } catch (e: any) { toast.error(`OAuth authorize failed: ${e.message}`) }
}

async function startDeviceCode() {
  try {
    oauthDeviceCode.value = await apiPost(`/api/oauth/${providerId.value}/device-code`)
    if (oauthDeviceCode.value?.verificationUriComplete || oauthDeviceCode.value?.verificationUri) {
      window.open(oauthDeviceCode.value.verificationUriComplete || oauthDeviceCode.value.verificationUri, '_blank')
      startAutoPoll()
    }
  } catch (e: any) { toast.error(`Device code failed: ${e.message}`) }
}

function stopAutoPoll() { if (pollTimer) { clearInterval(pollTimer); pollTimer = null } oauthPolling.value = false }
function startAutoPoll() {
  stopAutoPoll()
  oauthPolling.value = true
  const interval = (oauthDeviceCode.value?.interval || 2) * 1000
  pollTimer = setInterval(async () => { try { await pollDeviceCode() } catch { /* keep polling */ } }, interval)
}

async function pollDeviceCode() {
  try {
    const res = await apiPost(`/api/oauth/${providerId.value}/device-code/poll`, {
      deviceCode: oauthDeviceCode.value?.deviceCode,
      codeVerifier: oauthDeviceCode.value?.codeVerifier,
      nonce: oauthDeviceCode.value?.nonce,
      machineId: oauthDeviceCode.value?.machineId,
      extraData: oauthDeviceCode.value?.extraData,
    })
    if (res?.success) {
      stopAutoPoll()
      toast.success('OAuth connected!')
      showOAuth.value = false
      await store.loadAll()
    }
  } catch (e: any) { stopAutoPoll(); toast.error(`Poll failed: ${e.message}`) }
}

async function importToken() {
  try {
    await apiPost(`/api/oauth/${providerId.value}/import`, { token: oauthToken.value })
    toast.success('Token imported')
    showOAuth.value = false
    oauthToken.value = ''
    await store.loadAll()
  } catch (e: any) { toast.error(`Import failed: ${e.message}`) }
}

// --- Cooldown timer ---
function cooldownRemaining(conn: Provider): string | null {
  const until = conn.data?.rateLimitedUntil || conn.data?.cooldownUntil
  if (!until) return null
  const diff = new Date(until).getTime() - Date.now()
  if (diff <= 0) return null
  const mins = Math.floor(diff / 60000)
  const secs = Math.floor((diff % 60000) / 1000)
  return mins > 0 ? `${mins}m ${secs}s` : `${secs}s`
}

// --- Masked credential ---
function maskedCredential(conn: Provider): string {
  const key = conn.data?.apiKey || conn.data?.accessToken || ''
  if (!key) return ''
  if (key.length <= 8) return '••••'
  return key.slice(0, 4) + '••••' + key.slice(-4)
}

// --- Logo ---
function logoUrl(id: string): string { return `/providers/${id}.png` }
function onLogoError(e: Event) {
  const img = e.target as HTMLImageElement
  img.style.display = 'none'
  const sibling = img.nextElementSibling as HTMLElement
  if (sibling) sibling.style.display = 'flex'
}

// --- Lifecycle ---
onMounted(async () => {
  if (store.providers.length === 0 && store.registryList.length === 0) await store.loadAll()
  if (store.proxyPools.length === 0) await store.loadProxies()
  await loadSettings()
  await fetchAliases()
  // Load models for first connection
  const conn = connections.value[0]
  if (conn) {
    selectConnection(conn)
    try {
      const res = await api(`/api/providers/${conn.id}/models`)
      registryModels.value = res?.registryModels || []
      customModels.value = res?.customModels || []
    } catch { /* ignore */ }
  }
  // Load disabled models
  try {
    const res = await api(`/api/models/disabled?providerAlias=${encodeURIComponent(providerAlias.value)}`)
    disabledModelIds.value = res?.ids || res?.disabled || []
  } catch { /* ignore */ }
})

onUnmounted(() => { stopAutoPoll() })
</script>

<template>
  <div>
    <!-- Header -->
    <div class="detail-header">
      <GButton variant="ghost" size="icon" @click="router.push('/providers')"><ArrowLeft :size="16" /></GButton>
      <div class="detail-logo">
        <img :src="logoUrl(providerId)" :alt="providerId" @error="onLogoError" />
        <span class="logo-fallback"><Server :size="20" /></span>
      </div>
      <div>
        <h1 class="page-title" style="margin:0">{{ registryInfo?.name || providerId }}</h1>
        <p class="page-desc mono" style="margin:2px 0 0">{{ providerId }}</p>
      </div>
      <div class="flex-gap" style="margin-left:auto">
        <GBadge v-if="registryInfo" :color="registryInfo.authType === 'oauth' ? 'blue' : 'violet'">{{ registryInfo.authType }}</GBadge>
        <GBadge v-if="registryInfo?.category">{{ registryInfo.category }}</GBadge>
      </div>
    </div>

    <div class="detail-body">
      <!-- Left: Connection list -->
      <div class="conn-list">
        <div class="section-header">
          <h2 class="section-title">Connections ({{ connections.length }})</h2>
          <div class="flex-gap">
            <GButton v-if="registryInfo?.authType === 'oauth'" size="sm" variant="ghost" @click="openOAuth">
              <Key :size="12" />OAuth
            </GButton>
            <GButton v-if="!isOAuthOnly" size="sm" @click="openAddConn"><Plus :size="12" />Add</GButton>
          </div>
        </div>

        <!-- Bulk select header -->
        <div v-if="connections.length > 1" class="bulk-header">
          <label class="checkbox-label">
            <input type="checkbox" :checked="allSelected" @change="toggleSelectAll" />
            <span class="text-xs">Select all</span>
          </label>
          <GButton v-if="selectedConnectionIds.length > 0" size="sm" variant="ghost" @click="showBulkProxy = true">
            <Link2 :size="12" />Proxy ({{ selectedConnectionIds.length }})
          </GButton>
        </div>

        <div v-for="(conn, idx) in connections" :key="conn.id"
          :class="['conn-item', selectedConn?.id === conn.id && 'selected']">
          <div class="conn-item-left">
            <input v-if="connections.length > 1" type="checkbox" class="conn-checkbox"
              :checked="selectedConnectionIds.includes(conn.id)" @change="toggleSelect(conn.id)" />
            <!-- Priority arrows -->
            <div class="priority-arrows">
              <button class="arrow-btn" :disabled="idx === 0" @click.stop="swapPriority(idx, idx - 1)"><ChevronUp :size="12" /></button>
              <button class="arrow-btn" :disabled="idx === connections.length - 1" @click.stop="swapPriority(idx, idx + 1)"><ChevronDown :size="12" /></button>
            </div>
            <div class="conn-info" @click="selectConnection(conn)">
              <span class="conn-name">{{ conn.name || conn.provider }}</span>
              <div class="conn-meta-row">
                <span class="conn-meta mono">#{{ conn.priority || 0 }}</span>
                <span v-if="maskedCredential(conn)" class="conn-meta mono">{{ maskedCredential(conn) }}</span>
                <span v-if="conn.authType === 'oauth'" class="conn-meta">OAuth</span>
              </div>
              <div class="conn-meta-row" v-if="conn.data?.lastError">
                <span class="conn-error">{{ conn.data.lastError }}</span>
              </div>
              <div class="conn-meta-row" v-if="cooldownRemaining(conn)">
                <GBadge color="amber" style="font-size:9px">cooldown {{ cooldownRemaining(conn) }}</GBadge>
              </div>
            </div>
          </div>
          <div class="conn-item-right">
            <GBadge :color="conn.isActive ? 'green' : 'glass'" style="font-size:9px">{{ conn.isActive ? 'active' : 'off' }}</GBadge>
            <GBadge v-if="conn.data?.testStatus === 'error'" color="red" style="font-size:9px">error</GBadge>
            <!-- Per-connection proxy pool select -->
            <select v-if="store.proxyPools.length > 0" class="mini-select"
              :value="conn.data?.providerSpecificData?.proxyPoolId || ''"
              @change="updateConnProxy(conn, ($event.target as HTMLSelectElement).value || null)"
              title="Proxy pool">
              <option value="">No proxy</option>
              <option v-for="pool in store.proxyPools" :key="pool.id" :value="pool.id">{{ pool.data?.name || pool.id }}</option>
            </select>
          </div>
        </div>
        <GEmpty v-if="connections.length === 0">No connections yet for this provider.</GEmpty>
      </div>

      <!-- Right: Detail panel -->
      <div class="conn-detail">
        <!-- Strategy & Thinking card -->
        <GCard style="margin-bottom:16px">
          <div class="card-section-title"><Settings2 :size="14" /> Provider Settings</div>
          <div class="settings-grid">
            <div class="form-group">
              <label class="form-label">Fallback Strategy</label>
              <div class="flex-gap">
                <GSwitch :model-value="providerStrategy === 'round-robin'"
                  @update:model-value="(v: boolean) => saveStrategy(v ? 'round-robin' : null, providerStickyLimit)" />
                <span class="text-xs">{{ providerStrategy === 'round-robin' ? 'Round-Robin' : 'Default (fallback)' }}</span>
              </div>
            </div>
            <div v-if="providerStrategy === 'round-robin'" class="form-group">
              <label class="form-label">Sticky Limit</label>
              <input v-model="providerStickyLimit" type="number" min="1" class="input" style="width:80px"
                @change="saveStrategy('round-robin', providerStickyLimit)" />
            </div>
            <div v-if="thinkingLevels" class="form-group">
              <label class="form-label"><Brain :size="12" style="vertical-align:-1px" /> Thinking Level</label>
              <select class="input" :value="thinkingMode" @change="saveThinking(($event.target as HTMLSelectElement).value)">
                <option v-for="lvl in thinkingLevels" :key="lvl" :value="lvl">{{ lvl }}</option>
              </select>
            </div>
          </div>
        </GCard>

        <!-- Connection detail (when selected) -->
        <GCard v-if="selectedConn" style="margin-bottom:16px">
          <div class="detail-toolbar">
            <div class="flex-gap" style="flex-wrap:wrap">
              <GBadge :color="selectedConn.isActive ? 'green' : 'glass'">{{ selectedConn.isActive ? 'active' : 'disabled' }}</GBadge>
              <GBadge v-if="selectedConn.data?.rateLimitedUntil" color="amber">rate-limited</GBadge>
              <GBadge v-if="(selectedConn.data?.backoffLevel || 0) > 0" color="amber">backoff {{ selectedConn.data?.backoffLevel }}</GBadge>
            </div>
            <div class="flex-gap">
              <GSwitch :model-value="selectedConn.isActive" @update:model-value="toggleConn(selectedConn)" />
              <GButton variant="ghost" size="icon" @click="resetConn(selectedConn)" title="Reset cooldown"><RotateCcw :size="14" /></GButton>
              <GButton variant="danger-ghost" size="icon" @click="deleteConn(selectedConn)" title="Delete"><Trash2 :size="14" /></GButton>
            </div>
          </div>

          <p v-if="selectedConn.data?.lastError" class="error-box">{{ selectedConn.data.lastError }}</p>

          <!-- Test -->
          <div class="form-group">
            <label class="form-label">Connection Test</label>
            <div class="flex-gap">
              <GButton size="sm" @click="testConnection" :disabled="testing">
                <Zap :size="13" />{{ testing ? 'Testing…' : 'Test Connection' }}
              </GButton>
              <span v-if="testResult" class="mono text-xs"
                :style="{ color: testResult.ok ? 'var(--green)' : 'var(--red)' }">
                {{ testResult.ok ? 'OK' : 'FAIL' }} · {{ testResult.latency || '' }}<span v-if="testResult.code"> · HTTP {{ testResult.code }}</span>
              </span>
            </div>
          </div>

          <!-- Credentials (hidden for NoAuth) -->
          <template v-if="!isNoAuth">
            <div class="form-group">
              <label class="form-label">API Key / Access Token</label>
              <input v-model="editData.apiKey" type="password" class="input mono" placeholder="sk-...">
            </div>
            <div class="form-group">
              <label class="form-label">
                Base URL
                <span class="label-hint" :title="`Defaults to the registry base (${basePlaceholder}). Only override if you use a custom endpoint or mirror.`">ⓘ</span>
              </label>
              <input v-model="editData.baseUrl" class="input mono" :placeholder="basePlaceholder">
            </div>
            <GButton size="sm" @click="saveCredentials"><Save :size="13" />Save Credentials</GButton>
          </template>

          <!-- NoAuth card -->
          <div v-else class="noauth-card">
            <div class="noauth-head">
              <div class="noauth-icon"><Key :size="18" /></div>
              <div class="noauth-text">
                <p class="noauth-title">No authentication required</p>
                <p class="noauth-desc">This provider is ready to use. Optionally route requests through a proxy pool.</p>
              </div>
            </div>
          </div>

          <!-- OAuth status -->
          <div v-if="oauthStatus" class="form-group" style="margin-top:16px">
            <label class="form-label">OAuth Status</label>
            <div class="flex-gap">
              <GBadge :color="oauthStatus.connected ? 'green' : 'red'">{{ oauthStatus.connected ? 'Connected' : 'Not connected' }}</GBadge>
              <span v-if="oauthStatus.expiresAt" class="text-xs text-faint">Expires: {{ oauthStatus.expiresAt }}</span>
            </div>
          </div>
        </GCard>

        <!-- Models section -->
        <GCard>
          <div class="card-section-title">
            <span>Models ({{ allModels.length }})</span>
            <div class="flex-gap">
              <GButton v-if="disabledModels.length > 0" size="sm" variant="ghost" @click="enableAllModels">Enable All</GButton>
              <GButton v-if="enabledModels.length > 0" size="sm" variant="ghost" @click="disableAll">Disable All</GButton>
              <GButton v-if="providerId === 'qoder' && connections.some(c => c.isActive)" size="sm" variant="ghost"
                @click="importQoderModels" :disabled="importingModels">
                <Download :size="12" />{{ importingModels ? 'Importing…' : 'Import Models' }}
              </GButton>
            </div>
          </div>

          <!-- Add custom model -->
          <div class="flex-gap" style="margin-bottom:12px">
            <input v-model="newModel.id" class="input mono" placeholder="custom-model-id" style="flex:1">
            <input v-model="newModel.name" class="input" placeholder="display name" style="flex:1">
            <GButton size="sm" @click="addCustomModel"><Plus :size="13" />Add</GButton>
          </div>

          <!-- Enabled models -->
          <div class="model-list">
            <div v-for="m in enabledModels" :key="m.id" class="model-row-full">
              <div class="model-row-main">
                <span class="model-status-dot" :class="{
                  'dot-ok': modelTestResults[m.id]?.ok === true,
                  'dot-err': modelTestResults[m.id]?.ok === false
                }"></span>
                <code class="model-id mono">{{ providerAlias }}/{{ m.id }}</code>
                <span v-if="m.name && m.name !== m.id" class="text-xs text-faint">{{ m.name }}</span>
                <GBadge v-if="m.source === 'custom'" color="violet" style="font-size:8px">custom</GBadge>
              </div>
              <div class="model-row-actions">
                <!-- Test button -->
                <button v-if="connections.length > 0 || isNoAuth" class="icon-btn" @click="testModel(m.id)"
                  :disabled="testingModelIds.has(m.id)" :title="testingModelIds.has(m.id) ? 'Testing…' : 'Test model'">
                  <FlaskConical :size="13" :class="{ spinning: testingModelIds.has(m.id) }" />
                </button>
                <!-- Copy -->
                <button class="icon-btn" @click="copyModel(m.id)" title="Copy model string">
                  <Check v-if="copiedModel === m.id" :size="13" style="color:var(--green)" />
                  <Copy v-else :size="13" />
                </button>
                <!-- Alias -->
                <button class="icon-btn" @click="aliasEditId = aliasEditId === m.id ? '' : m.id; aliasEditValue = getAliasForModel(m.id) || ''" title="Set alias">
                  <span class="text-xs" style="font-weight:700">A</span>
                </button>
                <!-- Disable / Delete -->
                <button v-if="m.source === 'custom'" class="icon-btn danger" @click="removeCustomModel(m)" title="Remove custom model">
                  <X :size="13" />
                </button>
                <button v-else class="icon-btn danger" @click="disableModel(m.id)" title="Disable model">
                  <X :size="13" />
                </button>
              </div>
              <!-- Alias inline edit -->
              <div v-if="aliasEditId === m.id" class="alias-edit-row">
                <input v-model="aliasEditValue" class="input mono" placeholder="alias-name" style="flex:1;height:28px;font-size:11px">
                <GButton size="sm" @click="setAlias(m.id, aliasEditValue)">Set</GButton>
                <GButton v-if="getAliasForModel(m.id)" size="sm" variant="ghost" @click="deleteAlias(getAliasForModel(m.id)!); aliasEditId = ''">Del</GButton>
              </div>
              <!-- Test result -->
              <div v-if="modelTestResults[m.id]" class="model-test-result">
                <span :style="{ color: modelTestResults[m.id].ok ? 'var(--green)' : 'var(--red)' }">
                  {{ modelTestResults[m.id].ok ? 'OK' : 'FAIL' }}
                  <template v-if="modelTestResults[m.id].latency"> · {{ modelTestResults[m.id].latency }}</template>
                  <template v-if="modelTestResults[m.id].error"> · {{ modelTestResults[m.id].error }}</template>
                </span>
              </div>
            </div>
          </div>

          <!-- Disabled models -->
          <details v-if="disabledModels.length > 0" class="disabled-models">
            <summary class="text-xs text-faint" style="cursor:pointer">Disabled models ({{ disabledModels.length }})</summary>
            <div v-for="m in disabledModels" :key="'d'+m.id" class="model-row-full disabled">
              <div class="model-row-main">
                <code class="model-id mono">{{ providerAlias }}/{{ m.id }}</code>
              </div>
              <div class="model-row-actions">
                <button class="icon-btn" @click="enableModel(m.id)" title="Enable model"><RotateCcw :size="12" /></button>
              </div>
            </div>
          </details>

          <!-- Registry models (collapsed) -->
          <details v-if="registryModels.length > 0 && customModels.length === 0 && enabledModels.length === 0" class="disabled-models">
            <summary class="text-xs text-faint" style="cursor:pointer">Registry models ({{ registryModels.length }})</summary>
            <div class="model-chips">
              <span v-for="m in registryModels" :key="'r'+m.id" class="model-chip">{{ m.id }}</span>
            </div>
          </details>
          <p v-if="allModels.length === 0" class="text-xs text-faint">No models — use custom models or wildcard <code class="mono">{{ providerId }}/*</code>.</p>
        </GCard>
      </div>
    </div>

    <!-- Add Connection inline -->
    <div v-if="showAddConn" class="inline-form">
      <GCard>
        <h3 style="font-size:13px;font-weight:600;margin-bottom:12px">Add Connection to {{ registryInfo?.name || providerId }}</h3>
        <div class="form-grid-2">
          <div class="form-group">
            <label class="form-label">Display Name</label>
            <input v-model="newConn.name" class="input" placeholder="optional">
          </div>
          <div class="form-group">
            <label class="form-label">Auth Type</label>
            <input :value="newConn.authType" class="input" disabled>
          </div>
        </div>
        <template v-if="isNoAuth">
          <p class="text-xs text-faint" style="margin-bottom:12px">This provider is free and requires no credentials.</p>
        </template>
        <template v-else>
          <div class="form-group">
            <label class="form-label">API Key / Access Token</label>
            <input v-model="newConn.data.apiKey" type="password" class="input mono" placeholder="sk-...">
          </div>
        </template>
        <div class="form-group">
          <label class="form-label">Base URL <span class="text-faint">(optional)</span></label>
          <input v-model="newConn.data.baseUrl" class="input mono" :placeholder="basePlaceholder">
        </div>
        <div class="form-group">
          <label class="form-label">Priority</label>
          <input v-model.number="newConn.priority" type="number" class="input" style="max-width:120px" placeholder="0">
        </div>
        <div class="modal-actions">
          <GButton variant="ghost" size="sm" @click="showAddConn = false">Cancel</GButton>
          <GButton size="sm" @click="addConnection">Add Connection</GButton>
        </div>
      </GCard>
    </div>

    <!-- Bulk Proxy Modal -->
    <div v-if="showBulkProxy" class="modal-overlay" @click.self="showBulkProxy = false">
      <GCard class="oauth-modal">
        <h3 style="font-size:13px;font-weight:600;margin-bottom:12px">Apply Proxy ({{ selectedConnectionIds.length }} connections)</h3>
        <div class="bulk-proxy-list">
          <button class="bulk-proxy-btn" @click="applyOneToOne" :disabled="bulkUpdating">
            <ArrowUpDown :size="14" /> One-to-one (rotate)
          </button>
          <button class="bulk-proxy-btn" @click="applyBulkProxy(null)" :disabled="bulkUpdating">
            <X :size="14" /> None (unbind all)
          </button>
          <button v-for="pool in store.proxyPools" :key="pool.id" class="bulk-proxy-btn"
            @click="applyBulkProxy(pool.id)" :disabled="bulkUpdating || !pool.isActive">
            <Link2 :size="14" /> {{ pool.data?.name || pool.id }}
            <span v-if="!pool.isActive" class="text-xs text-faint">(inactive)</span>
          </button>
        </div>
        <div class="modal-actions" style="margin-top:14px">
          <GButton variant="ghost" size="sm" @click="showBulkProxy = false" :disabled="bulkUpdating">Cancel</GButton>
        </div>
      </GCard>
    </div>

    <!-- OAuth Modal -->
    <div v-if="showOAuth" class="modal-overlay" @click.self="showOAuth = false">
      <GCard class="oauth-modal">
        <h3 style="font-size:13px;font-weight:600;margin-bottom:12px">OAuth Connect — {{ registryInfo?.name || providerId }}</h3>
        <div class="add-tabs" style="margin-bottom:12px">
          <button :class="['add-tab', oauthMode === 'authorize' && 'active']" @click="oauthMode = 'authorize'">Authorize</button>
          <button :class="['add-tab', oauthMode === 'device' && 'active']" @click="oauthMode = 'device'">Device Code</button>
          <button :class="['add-tab', oauthMode === 'import' && 'active']" @click="oauthMode = 'import'">Token Import</button>
        </div>

        <template v-if="oauthMode === 'authorize'">
          <p class="text-xs text-faint" style="margin-bottom:10px">Opens the provider's authorization page in a new tab.</p>
          <GButton size="sm" @click="startOAuthAuthorize"><ExternalLink :size="13" />Open Authorize URL</GButton>
        </template>

        <template v-if="oauthMode === 'device'">
          <GButton size="sm" @click="startDeviceCode" style="margin-bottom:10px">Start Device Login</GButton>
          <div v-if="oauthDeviceCode" class="device-code-box">
            <template v-if="oauthDeviceCode.userCode">
              <p class="mono" style="font-size:16px;letter-spacing:2px;text-align:center">{{ oauthDeviceCode.userCode }}</p>
              <p class="text-xs text-faint" style="text-align:center;margin-top:6px">{{ oauthDeviceCode.verificationUri }}</p>
            </template>
            <template v-else>
              <p class="text-xs" style="text-align:center">A login page was opened in a new tab.<br>Complete the login there.</p>
            </template>
            <GButton size="sm" style="margin-top:10px;width:100%" @click="pollDeviceCode" :disabled="oauthPolling">
              {{ oauthPolling ? 'Waiting for login…' : 'Poll for Token' }}
            </GButton>
          </div>
        </template>

        <template v-if="oauthMode === 'import'">
          <div class="form-group">
            <label class="form-label">Paste Access Token / Refresh Token</label>
            <textarea v-model="oauthToken" class="input textarea mono" rows="3" placeholder="eyJ..."></textarea>
          </div>
          <GButton size="sm" @click="importToken">Import Token</GButton>
        </template>

        <div class="modal-actions" style="margin-top:14px">
          <GButton variant="ghost" size="sm" @click="showOAuth = false">Close</GButton>
        </div>
      </GCard>
    </div>
  </div>
</template>

<style scoped>
.detail-header {
  display: flex; align-items: center; gap: 12px; margin-bottom: 24px;
}
.detail-logo {
  width: 44px; height: 44px; border-radius: 10px; flex-shrink: 0;
  display: flex; align-items: center; justify-content: center;
  background: var(--code-bg); border: 1px solid var(--glass-border);
  overflow: hidden;
}
.detail-logo img { width: 30px; height: 30px; object-fit: contain; }
.logo-fallback { display: none; color: var(--text-muted); align-items: center; justify-content: center; }

.detail-body {
  display: grid; grid-template-columns: 300px 1fr; gap: 16px; align-items: start;
}
@media (max-width: 768px) {
  .detail-body { grid-template-columns: 1fr; }
}

.section-header {
  display: flex; align-items: center; justify-content: space-between; margin-bottom: 10px;
}
.section-title { font-size: 13px; font-weight: 600; }

/* Bulk selection */
.bulk-header {
  display: flex; align-items: center; justify-content: space-between;
  padding: 6px 8px; margin-bottom: 6px; border-radius: 6px;
  background: var(--glass-hover);
}
.checkbox-label { display: flex; align-items: center; gap: 6px; cursor: pointer; }

/* Connection items */
.conn-item {
  display: flex; align-items: flex-start; justify-content: space-between;
  padding: 10px 12px; border-radius: 8px;
  border: 1px solid var(--glass-border); background: var(--glass);
  margin-bottom: 6px; transition: all 0.15s ease;
}
.conn-item:hover { background: var(--glass-hover); }
.conn-item.selected { border-color: var(--ring); background: var(--ring-soft); }
.conn-item-left { display: flex; align-items: flex-start; gap: 6px; flex: 1; min-width: 0; }
.conn-item-right { display: flex; align-items: center; gap: 4px; flex-shrink: 0; }
.conn-checkbox { margin-top: 3px; }
.conn-info { cursor: pointer; min-width: 0; }
.conn-name { font-size: 12.5px; font-weight: 550; display: block; }
.conn-meta-row { display: flex; align-items: center; gap: 6px; margin-top: 2px; }
.conn-meta { font-size: 10px; color: var(--text-faint); }
.conn-error { font-size: 10px; color: var(--red); word-break: break-word; }

/* Priority arrows */
.priority-arrows { display: flex; flex-direction: column; gap: 0; }
.arrow-btn {
  background: none; border: none; cursor: pointer; padding: 1px;
  color: var(--text-muted); border-radius: 3px; line-height: 1;
}
.arrow-btn:hover:not(:disabled) { color: var(--text); background: var(--glass-hover); }
.arrow-btn:disabled { opacity: 0.3; cursor: not-allowed; }

/* Mini select for proxy pool */
.mini-select {
  font-size: 9px; padding: 2px 4px; border-radius: 4px;
  border: 1px solid var(--glass-border); background: var(--code-bg);
  color: var(--text-muted); max-width: 80px; cursor: pointer;
}

/* Cards */
.card-section-title {
  display: flex; align-items: center; justify-content: space-between;
  font-size: 13px; font-weight: 600; margin-bottom: 12px; gap: 8px;
}
.settings-grid { display: flex; flex-wrap: wrap; gap: 16px; }

.detail-toolbar {
  display: flex; align-items: center; justify-content: space-between; margin-bottom: 12px;
}
.error-box {
  font-size: 11.5px; font-family: var(--font-mono); color: var(--red);
  word-break: break-word; background: rgba(255,80,80,.08);
  padding: 6px 8px; border-radius: 6px; margin-bottom: 12px;
}
.form-group { margin-bottom: 14px; }
.form-label { display: block; font-size: 11.5px; font-weight: 550; color: var(--text-muted); margin-bottom: 6px; }
.label-hint { color: var(--text-faint); cursor: help; font-size: 11px; }

/* NoAuth card */
.noauth-card {
  padding: 16px; border-radius: 12px; margin-bottom: 14px;
  border: 1px solid rgba(74,222,128,.25); background: rgba(74,222,128,.05);
}
.noauth-head { display: flex; align-items: flex-start; gap: 12px; }
.noauth-icon {
  width: 38px; height: 38px; border-radius: 10px; flex-shrink: 0;
  display: flex; align-items: center; justify-content: center;
  background: rgba(74,222,128,.12); color: var(--green);
}
.noauth-title { font-size: 13px; font-weight: 600; margin-bottom: 3px; }
.noauth-desc { font-size: 11.5px; color: var(--text-muted); line-height: 1.5; }

/* Model list */
.model-list { display: flex; flex-direction: column; gap: 2px; }
.model-row-full {
  padding: 6px 8px; border-radius: 6px; border: 1px solid transparent;
  transition: all 0.12s ease;
}
.model-row-full:hover { background: var(--glass-hover); border-color: var(--glass-border); }
.model-row-full.disabled { opacity: 0.5; }
.model-row-main { display: flex; align-items: center; gap: 8px; min-width: 0; }
.model-row-actions { display: flex; align-items: center; gap: 2px; margin-left: auto; flex-shrink: 0; }
.model-id { font-size: 11.5px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.model-status-dot {
  width: 7px; height: 7px; border-radius: 50%; flex-shrink: 0;
  background: var(--text-faint); opacity: 0.4;
}
.model-status-dot.dot-ok { background: var(--green); opacity: 1; }
.model-status-dot.dot-err { background: var(--red); opacity: 1; }
.model-test-result { font-size: 10px; font-family: var(--font-mono); margin-top: 3px; padding-left: 15px; }

/* Icon buttons */
.icon-btn {
  background: none; border: none; cursor: pointer; padding: 3px;
  color: var(--text-muted); border-radius: 4px; line-height: 1;
  display: flex; align-items: center; justify-content: center;
  transition: all 0.12s ease;
}
.icon-btn:hover { color: var(--text); background: var(--glass-hover); }
.icon-btn.danger:hover { color: var(--red); background: rgba(255,80,80,.08); }
.icon-btn:disabled { opacity: 0.4; cursor: not-allowed; }

/* Alias edit */
.alias-edit-row { display: flex; align-items: center; gap: 6px; margin-top: 6px; padding-left: 15px; }

/* Disabled models section */
.disabled-models { margin-top: 12px; }

/* Model chips */
.model-chips { display: flex; flex-wrap: wrap; gap: 4px; margin-top: 4px; }
.model-chip {
  font-family: var(--font-mono); font-size: 10px; padding: 3px 8px;
  border-radius: 4px; background: var(--glass-hover);
  border: 1px solid var(--glass-border); color: var(--text-muted);
}

/* Inputs */
.form-grid-2 { display: grid; grid-template-columns: 1fr 1fr; gap: 12px; }
.input {
  width: 100%; height: 34px; padding: 0 12px;
  background: var(--code-bg); color: var(--text);
  border: 1px solid var(--glass-border); border-radius: var(--radius-sm);
  font-size: 13px; font-family: var(--font);
  transition: all 0.15s ease; outline: none;
}
.input.mono { font-family: var(--font-mono); font-size: 12px; }
.input.textarea { height: auto; padding: 8px 12px; resize: vertical; }
.input::placeholder { color: var(--text-faint); }
.input:focus { border-color: var(--ring); box-shadow: 0 0 0 3px var(--ring-soft); }
select.input {
  appearance: none; -webkit-appearance: none;
  background-image: var(--select-arrow);
  background-repeat: no-repeat; background-position: right 10px center;
  background-size: 14px; padding-right: 34px; cursor: pointer;
}
select.input option { background-color: var(--bg-elevated); color: var(--text); }
.modal-actions { display: flex; justify-content: flex-end; gap: 8px; }
.inline-form { margin-top: 16px; }

/* Bulk proxy */
.bulk-proxy-list { display: flex; flex-direction: column; gap: 4px; }
.bulk-proxy-btn {
  display: flex; align-items: center; gap: 8px; width: 100%;
  padding: 8px 12px; border-radius: 8px; border: none;
  background: var(--glass); color: var(--text); font-size: 12.5px;
  cursor: pointer; font-family: var(--font); transition: all 0.12s ease;
  text-align: left;
}
.bulk-proxy-btn:hover:not(:disabled) { background: var(--glass-hover); }
.bulk-proxy-btn:disabled { opacity: 0.5; cursor: not-allowed; }

/* Modals */
.modal-overlay {
  position: fixed; inset: 0; background: rgba(0,0,0,.5);
  display: flex; align-items: center; justify-content: center; z-index: 100;
}
.oauth-modal { width: 420px; max-width: 90vw; padding: 20px; }
.add-tabs { display: flex; gap: 4px; }
.add-tab {
  font-size: 11px; padding: 5px 12px; border-radius: var(--radius-sm);
  border: 1px solid var(--glass-border); background: var(--glass);
  color: var(--text-muted); cursor: pointer; font-family: var(--font);
  transition: all 0.15s ease;
}
.add-tab:hover { background: var(--glass-hover); color: var(--text); }
.add-tab.active { background: var(--gradient); color: var(--on-accent); border-color: transparent; }
.device-code-box {
  padding: 14px; border-radius: 8px; background: var(--code-bg);
  border: 1px solid var(--glass-border); margin-top: 8px;
}

/* Spinning animation for testing */
.spinning { animation: spin 1s linear infinite; }
@keyframes spin { from { transform: rotate(0deg); } to { transform: rotate(360deg); } }
</style>
