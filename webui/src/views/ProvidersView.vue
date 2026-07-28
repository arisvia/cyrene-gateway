<script setup lang="ts">
import { ref, computed } from 'vue'
import { useRouter } from 'vue-router'
import { useGatewayStore, type Provider, type RegistryProvider } from '@/stores/gateway'
import { api, apiPost } from '@/lib/api'
import { useToast } from '@/lib/toast'
import { useKeyboardShortcuts } from '@/lib/shortcuts'
import GButton from '@/components/ui/GButton.vue'
import GBadge from '@/components/ui/GBadge.vue'
import GCard from '@/components/ui/GCard.vue'
import GModal from '@/components/ui/GModal.vue'
import GEmpty from '@/components/ui/GEmpty.vue'
import { Plus, Search, Zap, Server, ChevronRight } from 'lucide-vue-next'

const store = useGatewayStore()
const router = useRouter()
const toast = useToast()

// --- Search ---
const search = ref('')
const searchInput = ref<HTMLInputElement | null>(null)

useKeyboardShortcuts({
  onSearch: () => searchInput.value?.focus(),
  onEscape: () => { showAdd.value = false },
})

// --- Category grouping ---
interface CategoryGroup {
  category: string
  label: string
  providers: RegistryProvider[]
}

const categoryLabels: Record<string, string> = {
  apikey: 'API Key Providers',
  oauth: 'OAuth Providers',
  freeTier: 'Free Tier Providers',
  free: 'Free Providers',
  webCookie: 'Web Cookie Providers',
}

const groupedRegistry = computed<CategoryGroup[]>(() => {
  const cats = store.registryCategories
  return cats.map(c => ({
    category: c.category,
    label: categoryLabels[c.category] || c.category,
    providers: store.registryList.filter(p => p.category === c.category),
  }))
})

// --- Connections grouped by provider ---
const connectionsByProvider = computed(() => {
  const map = new Map<string, Provider[]>()
  for (const p of store.providers) {
    const list = map.get(p.provider) || []
    list.push(p)
    map.set(p.provider, list)
  }
  return map
})

// --- Filtered providers (search) ---
function filterProviders(providers: RegistryProvider[]): RegistryProvider[] {
  if (!search.value) return providers
  const q = search.value.toLowerCase()
  return providers.filter(p =>
    p.name.toLowerCase().includes(q) || p.id.toLowerCase().includes(q)
  )
}

// --- Connection status for a provider ---
function connectionStatus(providerId: string): { count: number; active: number } {
  const conns = connectionsByProvider.value.get(providerId) || []
  return { count: conns.length, active: conns.filter(c => c.isActive).length }
}

// --- Contextual card action ---
// Determines the primary quick action for a registry provider card based on
// its auth mode and current connection state (9router-style contextual cards).
type CardAction = 'connected' | 'enable' | 'connect' | 'add-key' | 'detail'

function cardAction(rp: RegistryProvider): CardAction {
  if (connectionStatus(rp.id).count > 0) return 'connected'
  if (rp.noAuth || rp.authType === 'none') return 'enable'
  if (rp.authType === 'oauth' && !(rp.authModes || []).includes('api-key')) return 'connect'
  return 'add-key'
}

const actionLabels: Record<CardAction, string> = {
  connected: 'Manage',
  enable: 'Enable',
  connect: 'Connect',
  'add-key': 'Add Key',
  detail: 'Open',
}

const enablingFree = ref<string | 'all' | null>(null)

async function enableFreeProvider(rp: RegistryProvider) {
  enablingFree.value = rp.id
  try { await store.enableFree([rp.id]) }
  catch (e: any) { toast.error(`Enable failed: ${e.message}`) }
  enablingFree.value = null
}

// Card click: contextual action for unconnected providers, otherwise detail.
function onRegistryCard(rp: RegistryProvider, e: Event) {
  const action = cardAction(rp)
  if (action === 'enable') { enableFreeProvider(rp); return }
  if (action === 'connect') { goDetail(rp.id); return }
  if (action === 'add-key') { openAddFor(rp); return }
  goDetail(rp.id)
}

// --- First-run onboarding ---
const isOnboarding = computed(() => store.providers.length === 0)
const freeNotConnected = computed(() =>
  store.registryList.filter(rp => (rp.noAuth || rp.authType === 'none') && connectionStatus(rp.id).count === 0)
)

async function enableAllFree() {
  enablingFree.value = 'all'
  try { await store.enableFree() }
  catch (e: any) { toast.error(`Enable failed: ${e.message}`) }
  enablingFree.value = null
}

// --- Provider logo ---
function logoUrl(providerId: string): string {
  return `/providers/${providerId}.png`
}

function onLogoError(e: Event) {
  const img = e.target as HTMLImageElement
  img.style.display = 'none'
  const sibling = img.nextElementSibling as HTMLElement
  if (sibling) sibling.style.display = 'flex'
}

// --- Test All ---
const testingAll = ref(false)
const testResults = ref<Record<string, { ok: boolean; latency?: string; error?: string }>>({})

async function testAll() {
  testingAll.value = true
  testResults.value = {}
  try {
    const res = await apiPost('/api/providers/test-batch', {})
    if (res?.results) {
      for (const r of res.results) {
        testResults.value[r.id] = { ok: r.ok, latency: r.latency, error: r.error }
      }
      toast.success(`Test complete: ${res.passed} passed, ${res.failed} failed`)
    }
  } catch (e: any) {
    toast.error(`Test batch failed: ${e.message}`)
  }
  testingAll.value = false
  await store.loadAll()
}

// --- Add Provider ---
const showAdd = ref(false)
const addMode = ref<'registry' | 'openai' | 'anthropic'>('registry')
const registryFilter = ref('')
const registrySearch = ref('')
const newProvider = ref({ provider: '', name: '', authType: 'api-key', priority: 0, data: { apiKey: '', baseUrl: '' } })

const filteredRegistry = computed(() => {
  let list = store.registryList
  if (registryFilter.value) list = list.filter(p => p.category === registryFilter.value)
  if (registrySearch.value) {
    const q = registrySearch.value.toLowerCase()
    list = list.filter(p => p.name.toLowerCase().includes(q) || p.id.toLowerCase().includes(q))
  }
  return list
})

function selectRegistry(rp: RegistryProvider) {
  newProvider.value.provider = rp.id
  newProvider.value.authType = rp.authType || 'api-key'
  if (rp.baseUrl) newProvider.value.data.baseUrl = rp.baseUrl
  if (!newProvider.value.name) newProvider.value.name = rp.name
}

// The registry provider currently selected in the Add modal (if any).
const selectedRegistry = computed(() =>
  store.registryList.find(p => p.id === newProvider.value.provider) || null
)

// Auth modes the selected registry provider supports (locks the form fields).
const supportedAuthModes = computed<string[]>(() => {
  const rp = selectedRegistry.value
  if (!rp) return ['api-key', 'oauth', 'none']
  if (rp.authModes && rp.authModes.length) return rp.authModes
  return [rp.authType || 'api-key']
})

const isNoAuthSelected = computed(() => {
  const rp = selectedRegistry.value
  return !!rp && (rp.noAuth || rp.authType === 'none' || newProvider.value.authType === 'none')
})

// Open the Add modal pre-locked to a specific registry provider (contextual
// "Add Key" action from a card).
function openAddFor(rp: RegistryProvider) {
  addMode.value = 'registry'
  selectRegistry(rp)
  showAdd.value = true
}

async function addProvider() {
  if (!newProvider.value.provider) { toast.error('Select a provider first'); return }
  // api-key auth requires a credential; NoAuth needs none.
  if (newProvider.value.authType === 'api-key' && !newProvider.value.data.apiKey) {
    toast.error('An API key is required for this provider')
    return
  }
  await store.addProvider(newProvider.value)
  newProvider.value = { provider: '', name: '', authType: 'api-key', priority: 0, data: { apiKey: '', baseUrl: '' } }
  showAdd.value = false
}

// --- Navigate to detail ---
function goDetail(providerId: string) {
  router.push(`/providers/${providerId}`)
}
</script>

<template>
  <div>
    <div class="page-header">
      <h1 class="page-title">Providers</h1>
      <p class="page-desc">Manage upstream AI provider connections and credentials.</p>
    </div>

    <!-- Toolbar -->
    <div class="toolbar">
      <div class="search-box">
        <Search :size="14" class="search-icon" />
        <input ref="searchInput" v-model="search" class="input search-input" placeholder="Search providers...">
      </div>
      <div class="flex-gap">
        <GBadge>{{ store.providers.length }} connections</GBadge>
        <GBadge color="green">{{ store.providers.filter(p => p.isActive).length }} active</GBadge>
        <GButton size="sm" variant="ghost" @click="testAll" :disabled="testingAll">
          <Zap :size="13" />{{ testingAll ? 'Testing…' : 'Test All' }}
        </GButton>
        <GButton size="sm" @click="showAdd = true"><Plus :size="13" />Add Provider</GButton>
      </div>
    </div>

    <!-- First-run onboarding -->
    <section v-if="isOnboarding" class="onboarding">
      <div class="onboarding-hero">
        <div class="onboarding-icon"><Zap :size="20" /></div>
        <div>
          <h2 class="onboarding-title">Welcome to Cyrene Gateway</h2>
          <p class="onboarding-desc">You have no provider connections yet. Start with free providers — no API keys required — or add your own key below.</p>
        </div>
      </div>
      <div class="onboarding-actions">
        <GButton @click="enableAllFree" :disabled="enablingFree === 'all' || freeNotConnected.length === 0">
          <Zap :size="13" />{{ enablingFree === 'all' ? 'Enabling…' : `Enable ${freeNotConnected.length} free providers` }}
        </GButton>
        <GButton variant="ghost" @click="showAdd = true"><Plus :size="13" />Add an API key provider</GButton>
      </div>
    </section>

    <!-- Custom Connections (already added) -->
    <section v-if="store.providers.length > 0" class="category-section">
      <div class="section-header">
        <h2 class="section-title">Your Connections</h2>
        <GBadge>{{ store.providers.length }}</GBadge>
      </div>
      <div class="card-grid">
        <div v-for="p in store.providers" :key="p.id" class="provider-card" @click="goDetail(p.provider)">
          <div class="card-top">
            <div class="card-logo">
              <img :src="logoUrl(p.provider)" :alt="p.provider" @error="onLogoError" />
              <span class="logo-fallback"><Server :size="16" /></span>
            </div>
            <div class="card-info">
              <span class="card-name">{{ p.name || p.provider }}</span>
              <span class="card-id mono">{{ p.provider }}</span>
            </div>
            <ChevronRight :size="14" class="card-arrow" />
          </div>
          <div class="card-badges">
            <GBadge :color="p.isActive ? 'green' : 'glass'">{{ p.isActive ? 'active' : 'disabled' }}</GBadge>
            <GBadge :color="p.authType === 'oauth' ? 'blue' : p.authType === 'none' ? 'green' : 'violet'">{{ p.authType }}</GBadge>
            <GBadge v-if="testResults[p.id]" :color="testResults[p.id].ok ? 'green' : 'red'">
              {{ testResults[p.id].ok ? 'OK' : 'FAIL' }}
            </GBadge>
          </div>
        </div>
      </div>
    </section>

    <!-- Registry grouped by category -->
    <section v-for="group in groupedRegistry" :key="group.category" class="category-section">
      <div class="section-header">
        <h2 class="section-title">{{ group.label }}</h2>
        <GBadge>{{ filterProviders(group.providers).length }}</GBadge>
      </div>
      <div class="card-grid" v-if="filterProviders(group.providers).length > 0">
        <div v-for="rp in filterProviders(group.providers)" :key="rp.id" class="provider-card" @click="onRegistryCard(rp, $event)">
          <div class="card-top">
            <div class="card-logo">
              <img :src="logoUrl(rp.id)" :alt="rp.name" @error="onLogoError" />
              <span class="logo-fallback"><Server :size="16" /></span>
            </div>
            <div class="card-info">
              <span class="card-name">{{ rp.name }}</span>
              <span class="card-id mono">{{ rp.id }}</span>
            </div>
            <ChevronRight :size="14" class="card-arrow" />
          </div>
          <div class="card-badges">
            <template v-if="connectionStatus(rp.id).count > 0">
              <GBadge color="green">{{ connectionStatus(rp.id).active }} Connected</GBadge>
            </template>
            <GBadge v-else color="glass">Ready</GBadge>
            <GBadge :color="rp.authType === 'oauth' ? 'blue' : rp.authType === 'none' ? 'green' : 'violet'">
              {{ rp.authType === 'api-key' ? 'key' : rp.authType }}
            </GBadge>
            <GBadge v-if="testResults[rp.id]" :color="testResults[rp.id].ok ? 'green' : 'red'">
              {{ testResults[rp.id].ok ? 'OK' : 'FAIL' }}<span v-if="testResults[rp.id].latency"> · {{ testResults[rp.id].latency }}</span>
            </GBadge>
          </div>
          <!-- Contextual quick action -->
          <button
            v-if="cardAction(rp) !== 'connected'"
            class="card-action"
            :disabled="enablingFree === rp.id"
            @click.stop="onRegistryCard(rp, $event)">
            {{ enablingFree === rp.id ? 'Enabling…' : actionLabels[cardAction(rp)] }}
          </button>
          <button v-else class="card-action card-action-manage" @click.stop="goDetail(rp.id)">
            {{ actionLabels.connected }}
          </button>
        </div>
      </div>
      <GEmpty v-else>No providers match your search.</GEmpty>
    </section>

    <GEmpty v-if="store.registryList.length === 0 && store.providers.length === 0">
      No providers available. Check your connection.
    </GEmpty>

    <!-- Add Provider Modal -->
    <GModal v-if="showAdd" title="Add Provider" desc="Select from the registry or configure a custom endpoint." width="560px" @close="showAdd = false">
      <!-- Mode tabs -->
      <div class="add-tabs">
        <button :class="['add-tab', addMode === 'registry' && 'active']" @click="addMode = 'registry'">Registry</button>
        <button :class="['add-tab', addMode === 'openai' && 'active']" @click="addMode = 'openai'; newProvider.data.baseUrl = ''">OpenAI Compatible</button>
        <button :class="['add-tab', addMode === 'anthropic' && 'active']" @click="addMode = 'anthropic'; newProvider.data.baseUrl = ''">Anthropic Compatible</button>
      </div>

      <!-- Registry picker -->
      <template v-if="addMode === 'registry'">
        <div class="filter-row">
          <button :class="['filter-btn', !registryFilter && 'active']" @click="registryFilter = ''">All</button>
          <button v-for="cat in store.registryCategories" :key="cat.category"
            :class="['filter-btn', registryFilter === cat.category && 'active']"
            @click="registryFilter = registryFilter === cat.category ? '' : cat.category">
            {{ cat.category }} ({{ cat.count }})
          </button>
          <input v-model="registrySearch" class="input sm" placeholder="Search...">
        </div>
        <div class="registry-grid">
          <div v-for="rp in filteredRegistry" :key="rp.id"
            :class="['registry-item', newProvider.provider === rp.id && 'selected']"
            @click="selectRegistry(rp)">
            <div class="registry-name">
              <span class="truncate">{{ rp.name }}</span>
              <GBadge :color="rp.authType === 'oauth' ? 'blue' : rp.authType === 'none' ? 'green' : 'glass'">
                {{ rp.authType === 'api-key' ? 'key' : rp.authType }}
              </GBadge>
            </div>
            <p class="registry-id">{{ rp.id }}</p>
          </div>
        </div>
      </template>

      <!-- Custom endpoint -->
      <template v-else>
        <div class="form-group">
          <label class="form-label">Endpoint Name</label>
          <input v-model="newProvider.name" class="input" placeholder="My Custom Endpoint">
        </div>
        <div class="form-group">
          <label class="form-label">Base URL</label>
          <input v-model="newProvider.data.baseUrl" class="input mono"
            :placeholder="addMode === 'openai' ? 'https://api.example.com/v1' : 'https://api.anthropic.com'">
        </div>
      </template>

      <!-- Common fields -->
      <div class="form-grid-2">
        <div class="form-group">
          <label class="form-label">Provider ID</label>
          <input v-model="newProvider.provider" class="input mono" placeholder="e.g. openai" :disabled="addMode !== 'registry'">
        </div>
        <div class="form-group">
          <label class="form-label">Display Name</label>
          <input v-model="newProvider.name" class="input" placeholder="optional" v-if="addMode === 'registry'">
        </div>
      </div>

      <!-- NoAuth: no credentials needed -->
      <div v-if="isNoAuthSelected" class="noauth-note">
        <Zap :size="14" />
        <span>This provider requires no authentication. A connection will be created instantly — no API key needed.</span>
      </div>
      <!-- API key / token field (hidden for NoAuth) -->
      <div class="form-group" v-else>
        <label class="form-label">{{ newProvider.authType === 'oauth' ? 'Access Token' : 'API Key / Access Token' }}</label>
        <input v-model="newProvider.data.apiKey" type="password" class="input mono" placeholder="sk-...">
      </div>

      <div class="form-group" v-if="addMode === 'registry'">
        <label class="form-label">Base URL <span class="text-faint">(auto-filled from registry)</span></label>
        <input v-model="newProvider.data.baseUrl" class="input mono" :placeholder="selectedRegistry?.baseUrl || 'https://api.openai.com/v1'">
      </div>
      <div class="form-grid-2">
        <div class="form-group">
          <label class="form-label">Auth Type</label>
          <select v-model="newProvider.authType" class="input" :disabled="addMode === 'registry' && supportedAuthModes.length <= 1">
            <option v-for="m in (addMode === 'registry' ? supportedAuthModes : ['api-key', 'oauth', 'none'])" :key="m" :value="m">{{ m }}</option>
          </select>
        </div>
        <div class="form-group">
          <label class="form-label">Priority</label>
          <input v-model.number="newProvider.priority" type="number" class="input" placeholder="0">
        </div>
      </div>
      <div class="modal-actions">
        <GButton variant="ghost" @click="showAdd = false">Cancel</GButton>
        <GButton @click="addProvider">Add Connection</GButton>
      </div>
    </GModal>
  </div>
</template>

<style scoped>
.toolbar {
  display: flex; align-items: center; justify-content: space-between;
  gap: 12px; margin-bottom: 20px; flex-wrap: wrap;
}
.search-box {
  position: relative; flex: 1; max-width: 320px; min-width: 180px;
}
.search-icon {
  position: absolute; left: 10px; top: 50%; transform: translateY(-50%);
  color: var(--text-faint); pointer-events: none;
}
.search-input { padding-left: 32px !important; }

.category-section { margin-bottom: 28px; }
.section-header {
  display: flex; align-items: center; gap: 8px; margin-bottom: 12px;
}
.section-title { font-size: 14px; font-weight: 600; color: var(--text); }

.card-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(240px, 1fr));
  gap: 12px;
}
.provider-card {
  padding: 14px 16px; border-radius: 12px; cursor: pointer;
  border: 1px solid var(--glass-border); background: var(--glass);
  transition: all 0.18s ease;
}
.provider-card:hover {
  border-color: var(--glass-border-hover); background: var(--glass-hover);
  transform: translateY(-1px); box-shadow: 0 4px 16px rgba(0,0,0,.08);
}
.card-top { display: flex; align-items: center; gap: 10px; margin-bottom: 10px; }
.card-logo {
  width: 36px; height: 36px; border-radius: 8px; flex-shrink: 0;
  display: flex; align-items: center; justify-content: center;
  background: var(--code-bg); border: 1px solid var(--glass-border);
  overflow: hidden;
}
.card-logo img { width: 24px; height: 24px; object-fit: contain; }
.logo-fallback { display: none; color: var(--text-muted); align-items: center; justify-content: center; }
.card-info { flex: 1; min-width: 0; }
.card-name { display: block; font-size: 13px; font-weight: 550; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.card-id { display: block; font-size: 10px; color: var(--text-faint); margin-top: 2px; }
.card-arrow { color: var(--text-faint); flex-shrink: 0; }
.card-badges { display: flex; gap: 4px; flex-wrap: wrap; }

/* Contextual quick action */
.card-action {
  margin-top: 10px; width: 100%; height: 28px;
  font-size: 11.5px; font-weight: 550; font-family: var(--font);
  border-radius: var(--radius-sm); cursor: pointer;
  background: var(--gradient); color: var(--on-accent); border: none;
  transition: all 0.18s var(--ease-spring);
}
.card-action:hover:not(:disabled) { filter: brightness(1.1); }
.card-action:active:not(:disabled) { transform: scale(0.97); }
.card-action:disabled { opacity: 0.6; cursor: not-allowed; }
.card-action-manage {
  background: var(--glass-hover); color: var(--text-muted);
  border: 1px solid var(--glass-border);
}
.card-action-manage:hover { color: var(--text); background: var(--glass-hover); filter: none; }

/* First-run onboarding */
.onboarding {
  margin-bottom: 24px; padding: 20px;
  border-radius: 14px; border: 1px solid var(--glass-border);
  background: var(--glass);
  background-image: linear-gradient(135deg, var(--ring-soft), transparent 60%);
}
.onboarding-hero { display: flex; align-items: flex-start; gap: 14px; margin-bottom: 16px; }
.onboarding-icon {
  width: 42px; height: 42px; border-radius: 11px; flex-shrink: 0;
  display: flex; align-items: center; justify-content: center;
  background: var(--gradient); color: var(--on-accent);
  box-shadow: var(--shadow-accent);
}
.onboarding-title { font-size: 15px; font-weight: 650; margin-bottom: 4px; }
.onboarding-desc { font-size: 12.5px; color: var(--text-muted); line-height: 1.5; max-width: 560px; }
.onboarding-actions { display: flex; gap: 8px; flex-wrap: wrap; }

/* NoAuth note in Add modal */
.noauth-note {
  display: flex; align-items: flex-start; gap: 8px;
  padding: 10px 12px; margin-bottom: 14px;
  border-radius: var(--radius-sm);
  background: rgba(74,222,128,.08); border: 1px solid rgba(74,222,128,.25);
  color: var(--green); font-size: 12px; line-height: 1.5;
}
.noauth-note svg { flex-shrink: 0; margin-top: 1px; }

/* Add modal */
.add-tabs { display: flex; gap: 4px; margin-bottom: 14px; }
.add-tab {
  font-size: 11px; padding: 5px 12px; border-radius: var(--radius-sm);
  border: 1px solid var(--glass-border); background: var(--glass);
  color: var(--text-muted); cursor: pointer; font-family: var(--font);
  transition: all 0.15s ease;
}
.add-tab:hover { background: var(--glass-hover); color: var(--text); }
.add-tab.active { background: var(--gradient); color: var(--on-accent); border-color: transparent; }

.filter-row { display: flex; gap: 6px; margin-bottom: 8px; flex-wrap: wrap; align-items: center; }
.filter-btn {
  font-size: 10px; padding: 3px 8px; border-radius: var(--radius-sm);
  border: 1px solid var(--glass-border); background: var(--glass);
  color: var(--text-muted); cursor: pointer; font-family: var(--font);
  transition: all 0.15s ease;
}
.filter-btn:hover { background: var(--glass-hover); color: var(--text); }
.filter-btn.active { background: var(--gradient); color: var(--on-accent); border-color: transparent; }
.registry-grid { display: grid; grid-template-columns: repeat(3, 1fr); gap: 8px; max-height: 200px; overflow-y: auto; padding-right: 4px; margin-bottom: 18px; }
.registry-item {
  padding: 10px 11px; border-radius: 8px; cursor: pointer;
  border: 1px solid var(--glass-border); background: var(--glass);
  transition: all 0.15s ease;
}
.registry-item:hover { border-color: var(--glass-border-hover); background: var(--glass-hover); }
.registry-item.selected { border-color: var(--ring); background: var(--ring-soft); box-shadow: 0 0 12px var(--ring-soft); }
.registry-name { font-size: 11.5px; font-weight: 550; display: flex; align-items: center; justify-content: space-between; gap: 4px; }
.registry-id { font-size: 9.5px; color: var(--text-faint); font-family: var(--font-mono); margin-top: 3px; }
.form-group { margin-bottom: 14px; }
.form-label { display: block; font-size: 11.5px; font-weight: 550; color: var(--text-muted); margin-bottom: 6px; }
.form-grid-2 { display: grid; grid-template-columns: 1fr 1fr; gap: 12px; }
.input {
  width: 100%; height: 34px; padding: 0 12px;
  background: var(--code-bg); color: var(--text);
  border: 1px solid var(--glass-border); border-radius: var(--radius-sm);
  font-size: 13px; font-family: var(--font);
  transition: all 0.15s ease; outline: none;
}
.input.sm { flex: 1; min-width: 120px; font-size: 11px; padding: 3px 8px; height: auto; }
.input.mono { font-family: var(--font-mono); font-size: 12px; }
.input::placeholder { color: var(--text-faint); }
.input:focus { border-color: var(--ring); box-shadow: 0 0 0 3px var(--ring-soft); }
select.input {
  appearance: none; -webkit-appearance: none;
  background-image: var(--select-arrow);
  background-repeat: no-repeat; background-position: right 10px center;
  background-size: 14px; padding-right: 34px; cursor: pointer;
}
select.input option { background-color: var(--bg-elevated); color: var(--text); }
.modal-actions { display: flex; justify-content: flex-end; gap: 8px; margin-top: 6px; }
</style>
