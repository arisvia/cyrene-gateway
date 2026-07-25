<script setup lang="ts">
import { ref, computed } from 'vue'
import { useGatewayStore, type Provider } from '@/stores/gateway'
import { api, apiPost, apiPut, apiDelete } from '@/lib/api'
import GButton from '@/components/ui/GButton.vue'
import GBadge from '@/components/ui/GBadge.vue'
import GCard from '@/components/ui/GCard.vue'
import GSwitch from '@/components/ui/GSwitch.vue'
import GModal from '@/components/ui/GModal.vue'
import GEmpty from '@/components/ui/GEmpty.vue'
import { Plus, Server, Settings2, RotateCcw, Trash2, X, Zap, Save, Check } from 'lucide-vue-next'

const store = useGatewayStore()

// --- Add Provider ---
const showAdd = ref(false)
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

function selectRegistry(rp: any) {
  newProvider.value.provider = rp.id
  newProvider.value.authType = rp.authType || 'api-key'
  if (rp.baseUrl) newProvider.value.data.baseUrl = rp.baseUrl
  if (!newProvider.value.name) newProvider.value.name = rp.name
}

function registryBaseUrl(id: string) {
  return store.registryList.find(p => p.id === id)?.baseUrl || ''
}

async function addProvider() {
  await store.addProvider(newProvider.value)
  newProvider.value = { provider: '', name: '', authType: 'api-key', priority: 0, data: { apiKey: '', baseUrl: '' } }
  showAdd.value = false
}

async function removeProvider(p: Provider) {
  if (!confirm(`Delete provider "${p.name || p.provider}"?`)) return
  await store.deleteProvider(p)
}

// --- Provider Detail ---
const detail = ref<Provider | null>(null)
const detailEdit = ref({ apiKey: '', baseUrl: '' })
const detailRegistryModels = ref<any[]>([])
const detailCustomModels = ref<any[]>([])
const newCustomModel = ref({ id: '', name: '' })
const detailTesting = ref(false)
const detailTestResult = ref<any>(null)

async function openDetail(p: Provider) {
  detail.value = p
  detailEdit.value = { apiKey: p.data?.apiKey || '', baseUrl: p.data?.baseUrl || '' }
  detailTestResult.value = null
  newCustomModel.value = { id: '', name: '' }
  detailRegistryModels.value = []
  detailCustomModels.value = []
  try {
    const res = await api(`/api/providers/${p.id}/models`)
    detailRegistryModels.value = res?.registryModels || []
    detailCustomModels.value = res?.customModels || []
  } catch (e) { console.error('load provider models failed:', e) }
}

async function saveDetail() {
  const p = detail.value
  if (!p) return
  const data = { ...(p.data || {}), apiKey: detailEdit.value.apiKey, baseUrl: detailEdit.value.baseUrl }
  await apiPut(`/api/providers/${p.id}`, { data })
  await store.loadAll()
  const fresh = store.providers.find(x => x.id === p.id)
  if (fresh) detail.value = fresh
}

async function testDetail() {
  const p = detail.value
  if (!p) return
  detailTesting.value = true
  detailTestResult.value = null
  try {
    detailTestResult.value = await apiPost(`/api/providers/${p.id}/test`)
  } catch (e) { detailTestResult.value = { ok: false, error: String(e) } }
  detailTesting.value = false
  await store.loadAll()
  const fresh = store.providers.find(x => x.id === p.id)
  if (fresh) detail.value = fresh
}

async function addCustomModel() {
  const p = detail.value
  if (!p || !newCustomModel.value.id) return
  const res = await apiPost(`/api/providers/${p.id}/models`, newCustomModel.value)
  if (res?.customModels) detailCustomModels.value = res.customModels
  newCustomModel.value = { id: '', name: '' }
}

async function removeCustomModel(m: any) {
  const p = detail.value
  if (!p) return
  const res = await apiDelete(`/api/providers/${p.id}/models`, { id: m.id })
  if (res?.customModels) detailCustomModels.value = res.customModels
}
</script>

<template>
  <div>
    <div class="page-header">
      <h1 class="page-title">Providers</h1>
      <p class="page-desc">Manage upstream AI provider connections and credentials.</p>
    </div>

    <div class="flex-between section-gap">
      <div class="flex-gap">
        <GBadge>{{ store.providers.length }} connections</GBadge>
        <GBadge color="green">{{ store.providers.filter(p => p.isActive).length }} active</GBadge>
      </div>
      <GButton size="sm" @click="showAdd = true"><Plus :size="13" />Add Provider</GButton>
    </div>

    <GCard>
      <div v-for="p in store.providers" :key="p.id" class="list-row">
        <div class="flex-gap min-w-0">
          <div class="row-icon"><Server :size="15" /></div>
          <div class="min-w-0">
            <div class="flex-gap">
              <span class="truncate prov-name">{{ p.name || p.provider }}</span>
              <GBadge><span class="mono">{{ p.provider }}</span></GBadge>
              <GBadge :color="p.authType === 'oauth' ? 'blue' : p.authType === 'none' ? 'green' : 'violet'">{{ p.authType }}</GBadge>
            </div>
            <p class="text-xs text-faint mt-sm mono truncate">priority {{ p.priority || 0 }} · {{ p.data?.baseUrl || registryBaseUrl(p.provider) || 'default endpoint' }}</p>
          </div>
        </div>
        <div class="flex-gap shrink-0">
          <GSwitch :model-value="p.isActive" @update:model-value="store.toggleProvider(p)" />
          <GButton variant="ghost" size="icon" @click="openDetail(p)" title="Details & models"><Settings2 :size="14" /></GButton>
          <GButton variant="ghost" size="icon" @click="store.resetProvider(p)" title="Reset cooldown"><RotateCcw :size="14" /></GButton>
          <GButton variant="danger-ghost" size="icon" @click="removeProvider(p)" title="Delete"><Trash2 :size="14" /></GButton>
        </div>
      </div>
      <GEmpty v-if="store.providers.length === 0">No provider connections yet. Add one to start routing.</GEmpty>
    </GCard>

    <!-- Add Provider Modal -->
    <GModal v-if="showAdd" title="Add Provider" desc="Select from the registry or configure a custom endpoint." width="540px" @close="showAdd = false">
      <div class="filter-row">
        <button :class="['filter-btn', !registryFilter && 'active']" @click="registryFilter = ''">All</button>
        <button v-for="cat in store.registryCategories" :key="cat.category"
          :class="['filter-btn', registryFilter === cat.category && 'active']"
          @click="registryFilter = registryFilter === cat.category ? '' : cat.category">
          {{ cat.category }} ({{ cat.count }})
        </button>
        <input v-model="registrySearch" class="input sm" placeholder="Search providers...">
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
      <div class="form-grid-2">
        <div class="form-group">
          <label class="form-label">Provider ID</label>
          <input v-model="newProvider.provider" class="input mono" placeholder="e.g. openai">
        </div>
        <div class="form-group">
          <label class="form-label">Display Name</label>
          <input v-model="newProvider.name" class="input" placeholder="optional">
        </div>
      </div>
      <div class="form-group">
        <label class="form-label">API Key / Access Token</label>
        <input v-model="newProvider.data.apiKey" type="password" class="input mono" placeholder="sk-...">
      </div>
      <div class="form-group">
        <label class="form-label">Base URL <span class="text-faint">(auto-filled from registry)</span></label>
        <input v-model="newProvider.data.baseUrl" class="input mono" placeholder="https://api.openai.com/v1">
      </div>
      <div class="form-grid-2">
        <div class="form-group">
          <label class="form-label">Auth Type</label>
          <select v-model="newProvider.authType" class="input">
            <option value="api-key">api-key</option>
            <option value="oauth">oauth</option>
            <option value="none">none</option>
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

    <!-- Provider Detail Modal -->
    <GModal v-if="detail" :title="detail.name || detail.provider" width="600px" @close="detail = null">
      <p class="dialog-sub mono">{{ detail.provider }} · priority {{ detail.priority || 0 }}</p>

      <div class="flex-gap" style="flex-wrap:wrap;margin:8px 0">
        <GBadge :color="detail.isActive ? 'green' : 'glass'">{{ detail.isActive ? 'active' : 'disabled' }}</GBadge>
        <GBadge v-if="detail.data?.testStatus === 'error'" color="red">test error</GBadge>
        <GBadge v-if="detail.data?.rateLimitedUntil" color="amber">rate-limited</GBadge>
        <GBadge v-if="(detail.data?.backoffLevel || 0) > 0" color="amber">backoff {{ detail.data?.backoffLevel }}</GBadge>
      </div>
      <p v-if="detail.data?.lastError" class="error-box">{{ detail.data.lastError }}</p>

      <div class="form-group" style="margin-top:10px">
        <label class="form-label">Connection Test</label>
        <div class="flex-gap">
          <GButton size="sm" @click="testDetail" :disabled="detailTesting">
            <Zap :size="13" />{{ detailTesting ? 'Testing…' : 'Test Connection' }}
          </GButton>
          <span v-if="detailTestResult" class="mono text-xs"
            :style="{ color: detailTestResult.ok ? 'var(--green)' : 'var(--red)' }">
            {{ detailTestResult.ok ? 'OK' : 'FAIL' }} · {{ detailTestResult.latency || '' }}<span v-if="detailTestResult.code"> · HTTP {{ detailTestResult.code }}</span>
          </span>
        </div>
      </div>

      <div class="form-group">
        <label class="form-label">API Key / Access Token</label>
        <input v-model="detailEdit.apiKey" type="password" class="input mono" placeholder="sk-...">
      </div>
      <div class="form-group">
        <label class="form-label">Base URL</label>
        <input v-model="detailEdit.baseUrl" class="input mono" placeholder="https://api.openai.com/v1">
      </div>
      <div class="modal-actions" style="margin-bottom:12px">
        <GButton size="sm" @click="saveDetail"><Save :size="13" />Save Credentials</GButton>
      </div>

      <div class="form-group">
        <label class="form-label">Models</label>
        <div class="flex-gap" style="margin-bottom:8px">
          <input v-model="newCustomModel.id" class="input mono" placeholder="custom-model-id" style="flex:1">
          <input v-model="newCustomModel.name" class="input" placeholder="display name (optional)" style="flex:1">
          <GButton size="sm" @click="addCustomModel"><Plus :size="13" />Add</GButton>
        </div>
        <div v-if="detailCustomModels.length" style="margin-bottom:8px">
          <p class="text-xs text-faint" style="margin-bottom:4px">Custom models</p>
          <div v-for="m in detailCustomModels" :key="'c'+m.id" class="model-row">
            <div class="min-w-0">
              <span class="mono" style="font-size:12px">{{ m.id }}</span>
              <span v-if="m.name && m.name !== m.id" class="text-xs text-faint"> · {{ m.name }}</span>
            </div>
            <GButton variant="danger-ghost" size="icon" @click="removeCustomModel(m)"><Trash2 :size="13" /></GButton>
          </div>
        </div>
        <details v-if="detailRegistryModels.length">
          <summary class="text-xs text-faint" style="cursor:pointer;margin-bottom:4px">Registry models ({{ detailRegistryModels.length }})</summary>
          <div class="model-chips">
            <span v-for="m in detailRegistryModels" :key="'r'+m.id" class="model-chip" :title="m.name">{{ m.id }}</span>
          </div>
        </details>
        <p v-else class="text-xs text-faint">No registry models defined — use custom models or wildcard <code class="mono">{{ detail.provider }}/*</code>.</p>
      </div>
    </GModal>
  </div>
</template>

<style scoped>
.list-row {
  display: flex; align-items: center; justify-content: space-between;
  padding: 14px 16px; gap: 12px;
  border-bottom: 1px solid var(--row-divider);
  transition: background 0.12s ease;
}
.list-row:last-child { border-bottom: none; }
.list-row:hover { background: var(--glass); }
.row-icon {
  width: 34px; height: 34px; border-radius: 8px; flex-shrink: 0;
  display: flex; align-items: center; justify-content: center;
  background: var(--glass-hover); border: 1px solid var(--glass-border);
  color: var(--text-muted);
}
.prov-name { font-size: 13.5px; font-weight: 550; }
.dialog-sub { font-size: 11px; color: var(--text-faint); }
.error-box {
  font-size: 11.5px; font-family: var(--font-mono); color: var(--red);
  word-break: break-word; background: rgba(255,80,80,.08);
  padding: 6px 8px; border-radius: 6px;
}
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
</style>
