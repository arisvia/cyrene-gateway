<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useGatewayStore, type Provider } from '@/stores/gateway'
import { api, apiPost, apiPut, apiDelete } from '@/lib/api'
import { useToast } from '@/lib/toast'
import GButton from '@/components/ui/GButton.vue'
import GBadge from '@/components/ui/GBadge.vue'
import GCard from '@/components/ui/GCard.vue'
import GSwitch from '@/components/ui/GSwitch.vue'
import GEmpty from '@/components/ui/GEmpty.vue'
import { ArrowLeft, Zap, Save, Plus, Trash2, RotateCcw, Server, Key, ExternalLink } from 'lucide-vue-next'

const route = useRoute()
const router = useRouter()
const store = useGatewayStore()
const toast = useToast()

const providerId = computed(() => route.params.id as string)

// Registry info for this provider
const registryInfo = computed(() => store.registryList.find(p => p.id === providerId.value))

// Connections for this provider
const connections = computed(() => store.providers.filter(p => p.provider === providerId.value))

// --- Connection detail editing ---
const selectedConn = ref<Provider | null>(null)
const editData = ref({ apiKey: '', baseUrl: '' })
const testing = ref(false)
const testResult = ref<any>(null)
const registryModels = ref<any[]>([])
const customModels = ref<any[]>([])
const newModel = ref({ id: '', name: '' })

// OAuth
const oauthStatus = ref<any>(null)

async function selectConnection(conn: Provider) {
  selectedConn.value = conn
  editData.value = { apiKey: conn.data?.apiKey || '', baseUrl: conn.data?.baseUrl || '' }
  boundPoolId.value = currentPoolId(conn)
  testResult.value = null
  registryModels.value = []
  customModels.value = []
  newModel.value = { id: '', name: '' }
  try {
    const res = await api(`/api/providers/${conn.id}/models`)
    registryModels.value = res?.registryModels || []
    customModels.value = res?.customModels || []
  } catch (e) { console.error(e) }
  // Load OAuth status if applicable
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

async function addCustomModel() {
  const conn = selectedConn.value
  if (!conn || !newModel.value.id) return
  const res = await apiPost(`/api/providers/${conn.id}/models`, newModel.value)
  if (res?.customModels) customModels.value = res.customModels
  newModel.value = { id: '', name: '' }
}

async function removeCustomModel(m: any) {
  const conn = selectedConn.value
  if (!conn) return
  const res = await apiDelete(`/api/providers/${conn.id}/models`, { id: m.id })
  if (res?.customModels) customModels.value = res.customModels
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

// --- Add new connection for this provider ---
const showAddConn = ref(false)
const newConn = ref({ name: '', authType: 'api-key', priority: 0, data: { apiKey: '', baseUrl: '' } })

// Whether this provider is OAuth-only (no manual api-key form)
const isOAuthOnly = computed(() => {
  const rp = registryInfo.value
  if (!rp) return false
  return rp.authType === 'oauth' && !(rp.authModes || []).includes('api-key')
})

// Whether this provider is NoAuth (free, no credentials needed)
const isNoAuth = computed(() => registryInfo.value?.authType === 'none' || registryInfo.value?.noAuth === true)

// Strip known endpoint suffixes so the Base URL placeholder shows the API
// base (e.g. .../v1) rather than a full chat-completions path.
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

// --- NoAuth proxy pool binding (9router-style NoAuthProxyCard) ---
const NONE_POOL = '__none__'
const boundPoolId = ref(NONE_POOL)
const savingPool = ref(false)

function currentPoolId(conn: Provider | null): string {
  const id = conn?.data?.providerSpecificData?.proxyPoolId
  return typeof id === 'string' && id ? id : NONE_POOL
}

async function bindProxyPool(poolId: string) {
  const conn = selectedConn.value
  if (!conn) return
  savingPool.value = true
  boundPoolId.value = poolId
  const specific = { ...(conn.data?.providerSpecificData || {}) }
  if (poolId === NONE_POOL) delete specific.proxyPoolId
  else specific.proxyPoolId = poolId
  const data = { ...(conn.data || {}), providerSpecificData: specific }
  try {
    await apiPut(`/api/providers/${conn.id}`, { data })
    toast.success(poolId === NONE_POOL ? 'Proxy pool unbound' : 'Proxy pool bound')
    await store.loadAll()
    const fresh = store.providers.find(x => x.id === conn.id)
    if (fresh) selectedConn.value = fresh
  } catch (e: any) {
    toast.error(`Failed to bind proxy pool: ${e.message}`)
  }
  savingPool.value = false
}

function openAddConn() {
  const rp = registryInfo.value
  if (rp) {
    // Lock auth type to what the registry supports
    newConn.value.authType = rp.authType || 'api-key'
  }
  // Don't pre-fill full endpoint URLs into the base URL field — show as placeholder
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

// --- OAuth connect ---
const showOAuth = ref(false)
const oauthMode = ref<'authorize' | 'device' | 'import'>('authorize')
const oauthDeviceCode = ref<any>(null)
const oauthToken = ref('')
const oauthPolling = ref(false)

// Determine default OAuth mode based on provider flow
function openOAuth() {
  const rp = registryInfo.value
  if (rp?.deviceCodeUrl || rp?.loginUrl || providerId.value === 'qoder') {
    oauthMode.value = 'device'
  } else if (rp?.authorizeUrl) {
    oauthMode.value = 'authorize'
  } else {
    oauthMode.value = 'import'
  }
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
    // Qoder-style flow: open the browser login page immediately
    if (oauthDeviceCode.value?.verificationUriComplete || oauthDeviceCode.value?.verificationUri) {
      const url = oauthDeviceCode.value.verificationUriComplete || oauthDeviceCode.value.verificationUri
      window.open(url, '_blank')
      startAutoPoll()
    }
  } catch (e: any) { toast.error(`Device code failed: ${e.message}`) }
}

let pollTimer: ReturnType<typeof setInterval> | null = null

function stopAutoPoll() {
  if (pollTimer) { clearInterval(pollTimer); pollTimer = null }
  oauthPolling.value = false
}

function startAutoPoll() {
  stopAutoPoll()
  oauthPolling.value = true
  const interval = (oauthDeviceCode.value?.interval || 2) * 1000
  pollTimer = setInterval(async () => {
    try {
      await pollDeviceCode()
    } catch { /* keep polling */ }
  }, interval)
}

function reopenLoginPage() {
  const url = oauthDeviceCode.value?.verificationUriComplete || oauthDeviceCode.value?.verificationUri
  if (url) window.open(url, '_blank')
  startAutoPoll()
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
  } catch (e: any) {
    stopAutoPoll()
    toast.error(`Poll failed: ${e.message}`)
  }
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

function logoUrl(id: string): string {
  return `/providers/${id}.png`
}
function onLogoError(e: Event) {
  const img = e.target as HTMLImageElement
  img.style.display = 'none'
  const sibling = img.nextElementSibling as HTMLElement
  if (sibling) sibling.style.display = 'flex'
}

onMounted(async () => {
  if (store.providers.length === 0 && store.registryList.length === 0) {
    await store.loadAll()
  }
  if (store.proxyPools.length === 0) {
    await store.loadProxies()
  }
  // Auto-select first connection if exists
  if (connections.value.length > 0) {
    selectConnection(connections.value[0])
  }
})

onUnmounted(() => {
  stopAutoPoll()
})
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
          <h2 class="section-title">Connections</h2>
          <div class="flex-gap">
            <GButton v-if="registryInfo?.authType === 'oauth'" size="sm" variant="ghost" @click="openOAuth">
              <Key :size="12" />OAuth
            </GButton>
            <GButton v-if="!isOAuthOnly" size="sm" @click="openAddConn"><Plus :size="12" />Add</GButton>
          </div>
        </div>

        <div v-for="conn in connections" :key="conn.id"
          :class="['conn-item', selectedConn?.id === conn.id && 'selected']"
          @click="selectConnection(conn)">
          <div class="conn-info">
            <span class="conn-name">{{ conn.name || conn.provider }}</span>
            <span class="conn-meta mono">priority {{ conn.priority || 0 }}</span>
          </div>
          <div class="flex-gap shrink-0">
            <GBadge :color="conn.isActive ? 'green' : 'glass'" style="font-size:9px">{{ conn.isActive ? 'active' : 'off' }}</GBadge>
            <GBadge v-if="conn.data?.testStatus === 'error'" color="red" style="font-size:9px">error</GBadge>
          </div>
        </div>
        <GEmpty v-if="connections.length === 0">No connections yet for this provider.</GEmpty>
      </div>

      <!-- Right: Connection detail -->
      <div class="conn-detail" v-if="selectedConn">
        <GCard>
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

          <!-- NoAuth: no credentials required (9router-style card) -->
          <div v-else class="noauth-card">
            <div class="noauth-head">
              <div class="noauth-icon"><Key :size="18" /></div>
              <div class="noauth-text">
                <p class="noauth-title">No authentication required</p>
                <p class="noauth-desc">This provider is ready to use. Optionally route requests through a proxy pool to bypass IP-based limits.</p>
              </div>
            </div>
            <div class="form-group" style="margin-bottom:0">
              <label class="form-label">Proxy Pool</label>
              <select class="input" :value="currentPoolId(selectedConn)" :disabled="savingPool"
                @change="bindProxyPool(($event.target as HTMLSelectElement).value)">
                <option :value="NONE_POOL">None (direct)</option>
                <option v-for="pool in store.proxyPools" :key="pool.id" :value="pool.id">{{ pool.data?.name || pool.id }}</option>
              </select>
              <p class="text-xs text-faint" style="margin-top:6px">
                {{ store.proxyPools.length === 0 ? 'Add a proxy pool in the Proxies page to route this provider through it.' : 'Requests use the gateway proxy pool; binding tags this connection for pool-aware routing.' }}
              </p>
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

          <!-- Models -->
          <div class="form-group" style="margin-top:16px">
            <label class="form-label">Models</label>
            <div class="flex-gap" style="margin-bottom:8px">
              <input v-model="newModel.id" class="input mono" placeholder="custom-model-id" style="flex:1">
              <input v-model="newModel.name" class="input" placeholder="display name" style="flex:1">
              <GButton size="sm" @click="addCustomModel"><Plus :size="13" />Add</GButton>
            </div>
            <div v-if="customModels.length" style="margin-bottom:8px">
              <p class="text-xs text-faint" style="margin-bottom:4px">Custom models</p>
              <div v-for="m in customModels" :key="'c'+m.id" class="model-row">
                <div class="min-w-0">
                  <span class="mono" style="font-size:12px">{{ m.id }}</span>
                  <span v-if="m.name && m.name !== m.id" class="text-xs text-faint"> · {{ m.name }}</span>
                </div>
                <GButton variant="danger-ghost" size="icon" @click="removeCustomModel(m)"><Trash2 :size="13" /></GButton>
              </div>
            </div>
            <details v-if="registryModels.length">
              <summary class="text-xs text-faint" style="cursor:pointer;margin-bottom:4px">Registry models ({{ registryModels.length }})</summary>
              <div class="model-chips">
                <span v-for="m in registryModels" :key="'r'+m.id" class="model-chip" :title="m.name">{{ m.id }}</span>
              </div>
            </details>
            <p v-else class="text-xs text-faint">No registry models — use custom models or wildcard <code class="mono">{{ providerId }}/*</code>.</p>
          </div>
        </GCard>
      </div>

      <!-- No connection selected -->
      <div v-else class="conn-detail">
        <GCard>
          <GEmpty>Select a connection to view details, or add a new one.</GEmpty>
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
          <p class="text-xs text-faint" style="margin-bottom:12px">This provider is free and requires no credentials. A connection will be created automatically.</p>
        </template>
        <template v-else>
          <div class="form-group">
            <label class="form-label">API Key / Access Token</label>
            <input v-model="newConn.data.apiKey" type="password" class="input mono" placeholder="sk-...">
          </div>
        </template>
        <div class="form-group">
          <label class="form-label">Base URL <span class="text-faint">(optional, defaults to registry)</span></label>
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
              <p class="text-xs" style="text-align:center">A login page was opened in a new tab.<br>Complete the login there — this dialog polls automatically.</p>
              <GButton v-if="!oauthPolling" size="sm" variant="ghost" style="margin-top:8px;width:100%"
                @click="reopenLoginPage">
                Reopen Login Page
              </GButton>
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
  display: grid; grid-template-columns: 280px 1fr; gap: 16px; align-items: start;
}
@media (max-width: 768px) {
  .detail-body { grid-template-columns: 1fr; }
}

.conn-list { }
.section-header {
  display: flex; align-items: center; justify-content: space-between; margin-bottom: 10px;
}
.section-title { font-size: 13px; font-weight: 600; }
.conn-item {
  display: flex; align-items: center; justify-content: space-between;
  padding: 10px 12px; border-radius: 8px; cursor: pointer;
  border: 1px solid var(--glass-border); background: var(--glass);
  margin-bottom: 6px; transition: all 0.15s ease;
}
.conn-item:hover { background: var(--glass-hover); }
.conn-item.selected { border-color: var(--ring); background: var(--ring-soft); }
.conn-name { font-size: 12.5px; font-weight: 550; display: block; }
.conn-meta { font-size: 10px; color: var(--text-faint); display: block; margin-top: 2px; }

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

/* NoAuth card (9router-style) */
.noauth-card {
  padding: 16px; border-radius: 12px; margin-bottom: 14px;
  border: 1px solid rgba(74,222,128,.25);
  background: rgba(74,222,128,.05);
}
.noauth-head { display: flex; align-items: flex-start; gap: 12px; margin-bottom: 14px; }
.noauth-icon {
  width: 38px; height: 38px; border-radius: 10px; flex-shrink: 0;
  display: flex; align-items: center; justify-content: center;
  background: rgba(74,222,128,.12); color: var(--green);
}
.noauth-title { font-size: 13px; font-weight: 600; margin-bottom: 3px; }
.noauth-desc { font-size: 11.5px; color: var(--text-muted); line-height: 1.5; }
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
.model-row {
  display: flex; align-items: center; justify-content: space-between;
  padding: 6px 8px; border-bottom: 1px solid var(--row-divider);
}
.model-chips { display: flex; flex-wrap: wrap; gap: 4px; margin-top: 4px; }
.model-chip {
  font-family: var(--font-mono); font-size: 10px; padding: 3px 8px;
  border-radius: 4px; background: var(--glass-hover);
  border: 1px solid var(--glass-border); color: var(--text-muted);
}
.inline-form { margin-top: 16px; }

/* OAuth modal */
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
</style>
