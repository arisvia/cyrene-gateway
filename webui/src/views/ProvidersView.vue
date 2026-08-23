<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { useGatewayStore, type RegistryProvider, type Provider } from '@/stores/gateway'
import { useToast } from '@/lib/toast'
import { api, apiPost } from '@/lib/api'
import GCard from '@/components/ui/GCard.vue'
import GButton from '@/components/ui/GButton.vue'
import GBadge from '@/components/ui/GBadge.vue'
import GModal from '@/components/ui/GModal.vue'
import GEmpty from '@/components/ui/GEmpty.vue'
import GSwitch from '@/components/ui/GSwitch.vue'
import {
  Plus,
  Search,
  Zap,
  KeyRound,
  Layers,
  ExternalLink,
  RotateCcw,
  Trash2,
  CheckCircle2,
  AlertCircle,
  Clock,
  Sparkles,
  Server,
  Play,
  Copy,
  QrCode
} from 'lucide-vue-next'

const store = useGatewayStore()
const toast = useToast()
const router = useRouter()

// Tabs: connections (default) vs catalog
const activeTab = ref<'connections' | 'catalog'>('connections')
const search = ref('')

// Connection Wizard state
const showWizard = ref(false)
const wizardStep = ref<1 | 2>(1)
const selectedProvider = ref<RegistryProvider | null>(null)
const selectedAuthMode = ref<string>('apikey')

// Form inputs
const formName = ref('')
const formApiKey = ref('')
const formBaseUrl = ref('')

// OAuth & Device Code state
const deviceCodeData = ref<{
  userCode?: string
  verificationUri?: string
  verificationUriComplete?: string
  deviceCode?: string
  codeVerifier?: string
  nonce?: string
  machineId?: string
} | null>(null)
const pollingOAuth = ref(false)
let pollTimer: number | null = null

// Wizard test state
const testingConnection = ref(false)
const testResult = ref<{ ok: boolean; latencyMs?: number; error?: string } | null>(null)
const submitting = ref(false)

// Connection list testing
const connTestResults = ref<Record<string, { ok: boolean; latencyMs?: number; error?: string }>>({})
const testingConns = ref<Record<string, boolean>>({})

onMounted(() => {
  if (!store.registryCategories.length) store.loadCore()
})

const freeProviders = computed(() =>
  store.registryList.filter(p => p.category === 'free' || p.category === 'freeTier')
)

// Brand grouping for catalog
interface BrandGroup { brand: string; region: string; providers: RegistryProvider[] }
function groupByBrand(providers: RegistryProvider[]): BrandGroup[] {
  const groups: BrandGroup[] = []
  const byBrand = new Map<string, RegistryProvider[]>()
  for (const p of providers) {
    if (p.brand) {
      const list = byBrand.get(p.brand) || []
      list.push(p)
      byBrand.set(p.brand, list)
    } else {
      groups.push({ brand: '', region: p.region || '', providers: [p] })
    }
  }
  for (const [brand, list] of byBrand) {
    list.sort((a, b) => (a.region || '').localeCompare(b.region || ''))
    groups.push({ brand, region: list[0].region || '', providers: list })
  }
  return groups
}

const filteredCategories = computed(() => {
  const q = search.value.toLowerCase()
  return store.registryCategories
    .map(cat => ({
      ...cat,
      groups: groupByBrand(cat.providers.filter(p => !q || p.name.toLowerCase().includes(q) || p.id.toLowerCase().includes(q))),
    }))
    .filter(cat => cat.groups.length > 0)
})

const filteredConnections = computed(() => {
  const q = search.value.toLowerCase()
  if (!q) return store.providers
  return store.providers.filter(p =>
    (p.name && p.name.toLowerCase().includes(q)) ||
    p.provider.toLowerCase().includes(q) ||
    p.id.toLowerCase().includes(q)
  )
})

const regionSel = ref<Record<string, number>>({})
function activeInGroup(g: BrandGroup): RegistryProvider {
  return g.providers[regionSel.value[g.brand] || 0] || g.providers[0]
}

function regionLabel(g: BrandGroup, rp: RegistryProvider): string {
  const region = (rp.region || '').toUpperCase()
  const dup = g.providers.some(o => o !== rp && (o.region || '').toUpperCase() === region)
  return dup ? rp.id : (region || rp.id)
}

function openWizard(rp?: RegistryProvider) {
  stopOAuthPoll()
  testResult.value = null
  testingConnection.value = false
  deviceCodeData.value = null
  if (rp) {
    selectedProvider.value = rp
    formName.value = rp.name
    formBaseUrl.value = rp.baseUrl || ''
    selectedAuthMode.value = rp.authModes?.[0] || rp.authType || 'apikey'
    wizardStep.value = 2
  } else {
    selectedProvider.value = null
    formName.value = ''
    formBaseUrl.value = ''
    selectedAuthMode.value = 'apikey'
    wizardStep.value = 1
  }
  formApiKey.value = ''
  showWizard.value = true
}

function selectProviderInWizard(rp: RegistryProvider) {
  stopOAuthPoll()
  selectedProvider.value = rp
  formName.value = rp.name
  formBaseUrl.value = rp.baseUrl || ''
  selectedAuthMode.value = rp.authModes?.[0] || rp.authType || 'apikey'
  testResult.value = null
  deviceCodeData.value = null
  wizardStep.value = 2
}

async function startDeviceCodeFlow() {
  if (!selectedProvider.value) return
  stopOAuthPoll()
  pollingOAuth.value = true
  deviceCodeData.value = null
  try {
    const res = await apiPost(`/api/oauth/${selectedProvider.value.id}/device-code`)
    deviceCodeData.value = res
    if (res.verificationUriComplete || res.verificationUri) {
      window.open(res.verificationUriComplete || res.verificationUri, '_blank')
    }
    // Start polling
    pollTimer = window.setInterval(async () => {
      try {
        const pollRes = await apiPost(`/api/oauth/${selectedProvider.value?.id}/device-code/poll`, {
          deviceCode: deviceCodeData.value?.deviceCode,
          codeVerifier: deviceCodeData.value?.codeVerifier,
          nonce: deviceCodeData.value?.nonce,
          machineId: deviceCodeData.value?.machineId,
        })
        if (pollRes?.status === 'complete' || pollRes?.accessToken || pollRes?.id) {
          stopOAuthPoll()
          toast.success(`${selectedProvider.value?.name} OAuth authorization complete!`)
          showWizard.value = false
          await store.loadCore()
          activeTab.value = 'connections'
        }
      } catch (e) {
        // Continue polling until timeout
      }
    }, 2500)
  } catch (e: any) {
    pollingOAuth.value = false
    toast.error(e.message || 'Failed to start device code flow')
  }
}

function stopOAuthPoll() {
  pollingOAuth.value = false
  if (pollTimer) {
    clearInterval(pollTimer)
    pollTimer = null
  }
}

async function testCurrentInWizard() {
  if (!selectedProvider.value && !formBaseUrl.value) return
  testingConnection.value = true
  testResult.value = null
  try {
    const res = await apiPost('/api/models/test', {
      provider: selectedProvider.value?.id || 'custom',
      apiKey: formApiKey.value,
      baseUrl: formBaseUrl.value,
    })
    testResult.value = { ok: res?.ok ?? true, latencyMs: res?.latencyMs }
    if (res?.ok) {
      toast.success('Connection test succeeded!')
    } else {
      toast.error(res?.error || 'Connection failed')
    }
  } catch (e: any) {
    testResult.value = { ok: false, error: e.message }
    toast.error(e.message || 'Connection test failed')
  } finally {
    testingConnection.value = false
  }
}

async function saveConnection() {
  if (submitting.value) return
  submitting.value = true
  try {
    const provId = selectedProvider.value ? selectedProvider.value.id : 'custom'
    const payload: Record<string, any> = {
      provider: provId,
      name: formName.value || selectedProvider.value?.name || 'Custom Endpoint',
      authType: selectedAuthMode.value,
      priority: 0,
      data: {
        apiKey: formApiKey.value || undefined,
        baseUrl: formBaseUrl.value || undefined,
      },
    }
    await store.addProvider(payload)
    showWizard.value = false
    activeTab.value = 'connections'
  } catch (e: any) {
    toast.error(e.message || 'Failed to save connection')
  } finally {
    submitting.value = false
  }
}

async function testSingleConnection(conn: Provider) {
  testingConns.value[conn.id] = true
  try {
    const res = await store.testProvider(conn.id)
    connTestResults.value[conn.id] = res
    if (res.ok) {
      toast.success(`${conn.name || conn.provider} is healthy (${res.latencyMs || 0}ms)`)
    } else {
      toast.error(`${conn.name || conn.provider} failed: ${res.error || 'error'}`)
    }
  } catch (e: any) {
    connTestResults.value[conn.id] = { ok: false, error: e.message }
    toast.error(e.message)
  } finally {
    testingConns.value[conn.id] = false
  }
}
</script>

<template>
  <div class="providers-view">
    <!-- Header with Tab Navigation -->
    <div class="header">
      <div class="header-left">
        <h2>Providers & Connections</h2>
        <div class="tab-pills">
          <button
            class="tab-btn"
            :class="{ active: activeTab === 'connections' }"
            @click="activeTab = 'connections'"
          >
            <Server :size="15" />
            <span>Active Connections ({{ store.providers.length }})</span>
          </button>
          <button
            class="tab-btn"
            :class="{ active: activeTab === 'catalog' }"
            @click="activeTab = 'catalog'"
          >
            <Layers :size="15" />
            <span>Provider Catalog</span>
          </button>
        </div>
      </div>
      <div class="header-actions">
        <GButton variant="primary" @click="openWizard()">
          <Plus :size="16" />
          <span>Add Connection</span>
        </GButton>
      </div>
    </div>

    <!-- Search Bar -->
    <div class="search-bar">
      <Search :size="16" class="search-icon" />
      <input
        v-model="search"
        type="text"
        :placeholder="activeTab === 'connections' ? 'Search your connected accounts...' : 'Search 30+ AI provider catalog...'"
      />
    </div>

    <!-- TAB 1: Connections View -->
    <div v-if="activeTab === 'connections'" class="tab-content">
      <div v-if="filteredConnections.length > 0" class="conn-grid">
        <GCard v-for="conn in filteredConnections" :key="conn.id" class="conn-card">
          <div class="conn-header">
            <div class="conn-title-wrap">
              <div class="conn-avatar">{{ (conn.name || conn.provider).slice(0, 2).toUpperCase() }}</div>
              <div>
                <h3 class="conn-name">{{ conn.name || conn.provider }}</h3>
                <div class="conn-meta">
                  <span class="conn-provider-badge">{{ conn.provider }}</span>
                  <span class="conn-auth-badge">{{ conn.authType?.toUpperCase() || 'APIKEY' }}</span>
                  <span v-if="conn.data?.credentialHint" class="conn-hint">{{ conn.data.credentialHint }}</span>
                </div>
              </div>
            </div>
            <div class="conn-switch">
              <GSwitch
                :model-value="conn.isActive"
                @update:model-value="store.toggleProvider(conn)"
              />
            </div>
          </div>

          <div class="conn-body">
            <div class="conn-stat-row">
              <span class="stat-label">Status</span>
              <span v-if="conn.data?.rateLimitedUntil" class="status-badge rate-limited">
                <Clock :size="12" /> Rate Limited
              </span>
              <span v-else-if="conn.isActive" class="status-badge active">
                <CheckCircle2 :size="12" /> Ready
              </span>
              <span v-else class="status-badge disabled">Disabled</span>
            </div>

            <div v-if="conn.data?.baseUrl" class="conn-stat-row">
              <span class="stat-label">Base URL</span>
              <span class="stat-value truncate" :title="conn.data.baseUrl">{{ conn.data.baseUrl }}</span>
            </div>

            <div v-if="connTestResults[conn.id]" class="test-inline-result">
              <span v-if="connTestResults[conn.id].ok" class="test-ok">
                ✓ Online ({{ connTestResults[conn.id].latencyMs || 0 }}ms)
              </span>
              <span v-else class="test-fail">
                ✕ {{ connTestResults[conn.id].error || 'Failed' }}
              </span>
            </div>
          </div>

          <div class="conn-footer">
            <button class="action-btn" :disabled="testingConns[conn.id]" @click="testSingleConnection(conn)">
              <Play :size="13" />
              <span>{{ testingConns[conn.id] ? 'Testing…' : 'Test' }}</span>
            </button>
            <button
              v-if="conn.data?.rateLimitedUntil"
              class="action-btn"
              @click="store.resetCooldown(conn)"
            >
              <RotateCcw :size="13" />
              <span>Reset</span>
            </button>
            <button class="action-btn" @click="router.push(`/providers/${conn.id}`)">
              <Layers :size="13" />
              <span>Models</span>
            </button>
            <button class="action-btn danger" @click="store.deleteProvider(conn)">
              <Trash2 :size="13" />
            </button>
          </div>
        </GCard>
      </div>

      <GEmpty
        v-else
        title="No connections configured"
        description="Add your first AI provider connection or explore the catalog to get started."
      >
        <GButton variant="primary" @click="openWizard()">
          <Plus :size="16" />
          <span>Add Connection</span>
        </GButton>
      </GEmpty>
    </div>

    <!-- TAB 2: Catalog View -->
    <div v-else-if="activeTab === 'catalog'" class="tab-content">
      <div v-for="cat in filteredCategories" :key="cat.category" class="catalog-section">
        <h3 class="category-heading">{{ cat.category.toUpperCase() }}</h3>
        <div class="catalog-grid">
          <GCard
            v-for="g in cat.groups"
            :key="g.brand || g.providers[0].id"
            class="catalog-card"
            @click="openWizard(activeInGroup(g))"
          >
            <div class="catalog-card-header">
              <div class="catalog-icon">{{ activeInGroup(g).name.slice(0, 2).toUpperCase() }}</div>
              <div class="catalog-title">
                <h4>{{ activeInGroup(g).name }}</h4>
                <span class="catalog-id">{{ activeInGroup(g).id }}</span>
              </div>
            </div>

            <div v-if="g.providers.length > 1" class="region-pills" @click.stop>
              <button
                v-for="(rp, idx) in g.providers"
                :key="rp.id"
                class="region-btn"
                :class="{ active: (regionSel[g.brand] || 0) === idx }"
                @click="regionSel[g.brand] = idx"
              >
                {{ regionLabel(g, rp) }}
              </button>
            </div>

            <div class="catalog-card-footer">
              <span class="auth-pill">{{ activeInGroup(g).authModes?.join(' / ') || activeInGroup(g).authType || 'API Key' }}</span>
              <GButton size="sm" variant="outline" @click.stop="openWizard(activeInGroup(g))">
                Connect
              </GButton>
            </div>
          </GCard>
        </div>
      </div>
    </div>

    <!-- Three-Step Connection Wizard Modal -->
    <GModal :open="showWizard" @close="showWizard = false; stopOAuthPoll()">
      <div class="wizard-modal">
        <!-- Wizard Step 1: Select Provider -->
        <div v-if="wizardStep === 1">
          <h3 class="modal-title">Select Provider to Connect</h3>
          <p class="modal-desc">Pick an official vendor or add a custom OpenAI-compatible endpoint.</p>
          <div class="wizard-catalog-list">
            <div
              v-for="rp in store.registryList"
              :key="rp.id"
              class="wizard-provider-item"
              @click="selectProviderInWizard(rp)"
            >
              <div class="item-avatar">{{ rp.name.slice(0, 2).toUpperCase() }}</div>
              <div class="item-info">
                <span class="item-name">{{ rp.name }}</span>
                <span class="item-meta">{{ rp.category }} · {{ rp.authModes?.join(' / ') || rp.authType }}</span>
              </div>
              <GButton size="sm" variant="outline">Select</GButton>
            </div>
          </div>
        </div>

        <!-- Wizard Step 2: Configure & Test -->
        <div v-else-if="wizardStep === 2">
          <h3 class="modal-title">Configure {{ selectedProvider ? selectedProvider.name : 'Custom Provider' }}</h3>
          <p class="modal-desc">Select channel, input credentials and verify connectivity.</p>

          <div class="form-group">
            <label>Connection Name</label>
            <input v-model="formName" type="text" placeholder="e.g. Primary Account" />
          </div>

          <!-- Dual-Auth Selector: API Key vs OAuth / Device Flow -->
          <div v-if="selectedProvider?.authModes && selectedProvider.authModes.length > 1" class="form-group">
            <label>Authentication Method</label>
            <div class="auth-mode-selector">
              <button
                v-for="mode in selectedProvider.authModes"
                :key="mode"
                class="mode-pill"
                :class="{ active: selectedAuthMode === mode }"
                @click="selectedAuthMode = mode; stopOAuthPoll()"
              >
                {{ mode === 'oauth' ? 'OAuth Login' : (mode === 'apikey' ? 'API Key / Token' : mode.toUpperCase()) }}
              </button>
            </div>
          </div>

          <!-- Mode 1: API Key / PAT Form -->
          <div v-if="selectedAuthMode === 'apikey' || !selectedProvider">
            <div class="form-group">
              <label>API Key / Personal Access Token</label>
              <input v-model="formApiKey" type="password" placeholder="sk-... or pt-..." />
            </div>

            <div class="form-group">
              <label>Base URL (Optional override)</label>
              <input v-model="formBaseUrl" type="text" :placeholder="selectedProvider?.baseUrl || 'https://api.openai.com/v1'" />
            </div>

            <div v-if="testResult" class="wizard-test-result" :class="{ ok: testResult.ok, error: !testResult.ok }">
              <span v-if="testResult.ok">✓ Connection verified! Latency: {{ testResult.latencyMs || 0 }}ms</span>
              <span v-else>✕ Verification failed: {{ testResult.error || 'Invalid credentials or host unreachable' }}</span>
            </div>

            <div class="wizard-actions">
              <GButton variant="outline" @click="wizardStep = 1">Back</GButton>
              <div class="right-actions">
                <GButton
                  variant="outline"
                  :disabled="testingConnection || (!formApiKey && !formBaseUrl)"
                  @click="testCurrentInWizard"
                >
                  {{ testingConnection ? 'Testing…' : 'Test Connection' }}
                </GButton>
                <GButton variant="primary" :disabled="submitting" @click="saveConnection">
                  {{ submitting ? 'Saving…' : 'Save Connection' }}
                </GButton>
              </div>
            </div>
          </div>

          <!-- Mode 2: OAuth / Device Code Flow -->
          <div v-else-if="selectedAuthMode === 'oauth'" class="oauth-flow-section">
            <div v-if="!pollingOAuth" class="oauth-start-wrap">
              <p class="oauth-hint">Click below to open the vendor login page and link your account.</p>
              <GButton variant="primary" @click="startDeviceCodeFlow">
                <ExternalLink :size="15" />
                <span>Start OAuth Login</span>
              </GButton>
            </div>

            <div v-else class="oauth-polling-wrap">
              <div class="oauth-spinner"></div>
              <h4>Waiting for authorization in browser…</h4>
              <div v-if="deviceCodeData?.userCode" class="user-code-box">
                <span>Code:</span> <strong>{{ deviceCodeData.userCode }}</strong>
              </div>
              <p class="oauth-subhint">Please confirm the code in your opened browser window.</p>
              <GButton size="sm" variant="outline" @click="stopOAuthPoll">Cancel</GButton>
            </div>

            <div class="wizard-actions" style="margin-top: 1.5rem;">
              <GButton variant="outline" @click="wizardStep = 1; stopOAuthPoll()">Back</GButton>
            </div>
          </div>
        </div>
      </div>
    </GModal>
  </div>
</template>

<style scoped>
.providers-view {
  max-width: 1200px;
  margin: 0 auto;
  padding: 1.5rem;
}

.header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 1.5rem;
  flex-wrap: wrap;
  gap: 1rem;
}

.header-left {
  display: flex;
  align-items: center;
  gap: 1.5rem;
}

.header-left h2 {
  font-size: 1.35rem;
  font-weight: 700;
  margin: 0;
}

.tab-pills {
  display: flex;
  background: var(--g-surface-elevated, #1a1a24);
  padding: 3px;
  border-radius: 8px;
  border: 1px solid var(--g-border, #2e2e3f);
}

.tab-btn {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 6px 14px;
  font-size: 0.85rem;
  font-weight: 500;
  color: var(--g-text-secondary, #94a3b8);
  border: none;
  background: transparent;
  border-radius: 6px;
  cursor: pointer;
  transition: all 0.15s ease;
}

.tab-btn.active {
  background: var(--g-surface, #27273a);
  color: #fff;
  font-weight: 600;
  box-shadow: 0 1px 3px rgba(0,0,0,0.2);
}

.search-bar {
  position: relative;
  margin-bottom: 1.5rem;
}

.search-icon {
  position: absolute;
  left: 12px;
  top: 50%;
  transform: translateY(-50%);
  color: var(--g-text-muted, #64748b);
}

.search-bar input {
  width: 100%;
  padding: 10px 12px 10px 36px;
  background: var(--g-surface, #14141e);
  border: 1px solid var(--g-border, #2e2e3f);
  border-radius: 8px;
  color: var(--g-text, #f8fafc);
  font-size: 0.9rem;
  box-sizing: border-box;
}

.conn-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
  gap: 1rem;
}

.conn-card {
  padding: 1.2rem;
  display: flex;
  flex-direction: column;
  justify-content: space-between;
}

.conn-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  margin-bottom: 1rem;
}

.conn-title-wrap {
  display: flex;
  gap: 12px;
  align-items: center;
}

.conn-avatar {
  width: 36px;
  height: 36px;
  background: var(--g-primary, #6366f1);
  color: #fff;
  border-radius: 8px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-weight: 700;
  font-size: 0.85rem;
}

.conn-name {
  font-size: 1rem;
  font-weight: 600;
  margin: 0 0 4px 0;
}

.conn-meta {
  display: flex;
  gap: 6px;
  align-items: center;
}

.conn-provider-badge {
  font-size: 0.75rem;
  background: var(--g-surface-elevated, #242436);
  padding: 2px 6px;
  border-radius: 4px;
  color: var(--g-text-secondary, #94a3b8);
}

.conn-auth-badge {
  font-size: 0.7rem;
  background: rgba(99, 102, 241, 0.15);
  color: #818cf8;
  padding: 2px 6px;
  border-radius: 4px;
  font-weight: 600;
}

.conn-hint {
  font-size: 0.75rem;
  font-family: monospace;
  color: var(--g-text-muted, #64748b);
}

.conn-body {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin-bottom: 1.2rem;
  font-size: 0.85rem;
}

.conn-stat-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.stat-label {
  color: var(--g-text-muted, #64748b);
}

.status-badge {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: 0.75rem;
  font-weight: 600;
  padding: 2px 8px;
  border-radius: 9999px;
}

.status-badge.active {
  background: rgba(34, 197, 94, 0.15);
  color: #4ade80;
}

.status-badge.rate-limited {
  background: rgba(239, 68, 68, 0.15);
  color: #f87171;
}

.status-badge.disabled {
  background: rgba(148, 163, 184, 0.15);
  color: #94a3b8;
}

.test-inline-result {
  font-size: 0.8rem;
  margin-top: 4px;
}

.test-ok {
  color: #4ade80;
}

.test-fail {
  color: #f87171;
}

.conn-footer {
  display: flex;
  gap: 6px;
  border-top: 1px solid var(--g-border, #2e2e3f);
  padding-top: 10px;
}

.action-btn {
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 5px 10px;
  font-size: 0.8rem;
  background: var(--g-surface-elevated, #242436);
  border: 1px solid var(--g-border, #2e2e3f);
  color: var(--g-text, #f8fafc);
  border-radius: 6px;
  cursor: pointer;
  transition: all 0.15s ease;
}

.action-btn:hover {
  background: var(--g-primary, #6366f1);
  color: #fff;
}

.action-btn.danger:hover {
  background: #ef4444;
  color: #fff;
}

.catalog-section {
  margin-bottom: 2rem;
}

.category-heading {
  font-size: 0.95rem;
  font-weight: 700;
  letter-spacing: 0.05em;
  color: var(--g-text-secondary, #94a3b8);
  margin-bottom: 1rem;
}

.catalog-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(240px, 1fr));
  gap: 1rem;
}

.catalog-card {
  padding: 1rem;
  cursor: pointer;
  transition: transform 0.15s ease, border-color 0.15s ease;
}

.catalog-card:hover {
  transform: translateY(-2px);
  border-color: var(--g-primary, #6366f1);
}

.catalog-card-header {
  display: flex;
  gap: 10px;
  align-items: center;
  margin-bottom: 10px;
}

.catalog-icon {
  width: 32px;
  height: 32px;
  background: var(--g-surface-elevated, #242436);
  border-radius: 6px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-weight: 700;
  font-size: 0.75rem;
  color: var(--g-primary, #6366f1);
}

.catalog-title h4 {
  font-size: 0.95rem;
  margin: 0;
}

.catalog-id {
  font-size: 0.75rem;
  color: var(--g-text-muted, #64748b);
}

.catalog-card-footer {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-top: 10px;
}

.auth-pill {
  font-size: 0.75rem;
  color: var(--g-text-secondary, #94a3b8);
}

/* Wizard Modal */
.wizard-modal {
  padding: 1rem;
  min-width: 420px;
}

.modal-title {
  font-size: 1.15rem;
  font-weight: 700;
  margin: 0 0 6px 0;
}

.modal-desc {
  font-size: 0.85rem;
  color: var(--g-text-secondary, #94a3b8);
  margin: 0 0 1.2rem 0;
}

.wizard-catalog-list {
  display: flex;
  flex-direction: column;
  gap: 6px;
  max-height: 360px;
  overflow-y: auto;
}

.wizard-provider-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 8px 12px;
  border-radius: 6px;
  background: var(--g-surface, #14141e);
  cursor: pointer;
}

.wizard-provider-item:hover {
  background: var(--g-surface-elevated, #242436);
}

.form-group {
  margin-bottom: 1rem;
}

.form-group label {
  display: block;
  font-size: 0.85rem;
  font-weight: 600;
  margin-bottom: 6px;
}

.form-group input {
  width: 100%;
  padding: 8px 12px;
  background: var(--g-surface, #14141e);
  border: 1px solid var(--g-border, #2e2e3f);
  border-radius: 6px;
  color: #fff;
  box-sizing: border-box;
}

.wizard-test-result {
  padding: 8px 12px;
  border-radius: 6px;
  font-size: 0.85rem;
  margin-bottom: 1rem;
}

.wizard-test-result.ok {
  background: rgba(34, 197, 94, 0.15);
  color: #4ade80;
}

.wizard-test-result.error {
  background: rgba(239, 68, 68, 0.15);
  color: #f87171;
}

.wizard-actions {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-top: 1.5rem;
}

.right-actions {
  display: flex;
  gap: 8px;
}

.auth-mode-selector {
  display: flex;
  gap: 6px;
}

.mode-pill {
  padding: 6px 12px;
  font-size: 0.8rem;
  border-radius: 6px;
  background: var(--g-surface, #14141e);
  border: 1px solid var(--g-border, #2e2e3f);
  color: var(--g-text-secondary, #94a3b8);
  cursor: pointer;
  transition: all 0.15s ease;
}

.mode-pill.active {
  background: var(--g-primary, #6366f1);
  color: #fff;
  font-weight: 600;
  border-color: var(--g-primary, #6366f1);
}

.oauth-flow-section {
  padding: 1rem 0;
  text-align: center;
}

.oauth-start-wrap {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 12px;
}

.oauth-hint {
  font-size: 0.85rem;
  color: var(--g-text-secondary, #94a3b8);
  margin: 0;
}

.oauth-polling-wrap {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 10px;
  padding: 1rem;
  background: var(--g-surface, #14141e);
  border-radius: 8px;
}

.oauth-spinner {
  width: 28px;
  height: 28px;
  border: 3px solid rgba(99, 102, 241, 0.2);
  border-top-color: var(--g-primary, #6366f1);
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
}

@keyframes spin {
  to { transform: rotate(360deg); }
}

.user-code-box {
  background: var(--g-surface-elevated, #242436);
  padding: 6px 14px;
  border-radius: 6px;
  font-size: 1.1rem;
  letter-spacing: 0.1em;
  color: var(--g-primary, #6366f1);
}

.oauth-subhint {
  font-size: 0.8rem;
  color: var(--g-text-muted, #64748b);
  margin: 0;
}
</style>
