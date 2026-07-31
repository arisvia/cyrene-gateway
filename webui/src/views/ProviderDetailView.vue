<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useGatewayStore, type ProviderModel } from '@/stores/gateway'
import { useToast } from '@/lib/toast'
import { api, apiPost, apiPut, apiDelete } from '@/lib/api'
import GCard from '@/components/ui/GCard.vue'
import GButton from '@/components/ui/GButton.vue'
import GBadge from '@/components/ui/GBadge.vue'
import GModal from '@/components/ui/GModal.vue'
import GSwitch from '@/components/ui/GSwitch.vue'
import GSkeleton from '@/components/ui/GSkeleton.vue'
import { ArrowLeft, Plus, Trash2, Zap, RefreshCw, KeyRound } from 'lucide-vue-next'

const props = defineProps<{ id: string }>()
const router = useRouter()
const store = useGatewayStore()
const toast = useToast()

const tab = ref<'connections' | 'models' | 'settings'>('connections')
const loading = ref(true)
const models = ref<ProviderModel[]>([])
const modelsLoading = ref(false)
const testResult = ref<{ ok: boolean; latencyMs?: number; error?: string } | null>(null)
const testing = ref(false)

// Modals
const showAddKey = ref(false)
const newKey = ref('')
const newKeyName = ref('')
const showAddModel = ref(false)
const newModel = ref('')
const confirmDelete = ref(false)

// Settings tab
const baseUrl = ref('')
const strategy = ref('')
const saving = ref(false)

const conn = computed(() => store.providers.find(p => p.id === props.id))

onMounted(async () => {
  if (!store.providers.length) await store.loadCore()
  loading.value = false
  loadModels()
  if (conn.value) {
    baseUrl.value = conn.value.data?.baseUrl || ''
    strategy.value = conn.value.data?.strategy || ''
  }
})

async function loadModels() {
  modelsLoading.value = true
  try {
    const r = await api(`/api/providers/${props.id}/models`)
    models.value = Array.isArray(r?.models) ? r.models : (Array.isArray(r) ? r : [])
  } catch { models.value = [] }
  modelsLoading.value = false
}

async function test() {
  testing.value = true
  testResult.value = null
  try { testResult.value = await store.testProvider(props.id) } catch (e: any) { testResult.value = { ok: false, error: e.message } }
  testing.value = false
}

async function addKey() {
  try {
    await apiPost(`/api/providers/${props.id}/models`, {})
    await store.addProvider({
      provider: conn.value?.provider || 'custom',
      name: newKeyName.value || undefined,
      authType: 'apikey',
      data: { apiKey: newKey.value },
    })
    showAddKey.value = false
    newKey.value = ''
  } catch (e: any) { toast.error(e.message) }
}

async function addModel() {
  try {
    await apiPost(`/api/providers/${props.id}/models`, { model: newModel.value })
    toast.success(`Model "${newModel.value}" added`)
    showAddModel.value = false
    newModel.value = ''
    loadModels()
  } catch (e: any) { toast.error(e.message) }
}

async function removeModel(name: string) {
  try {
    await apiDelete(`/api/providers/${props.id}/models`, { model: name })
    toast.success(`Model "${name}" removed`)
    loadModels()
  } catch (e: any) { toast.error(e.message) }
}

async function refreshModels() {
  modelsLoading.value = true
  try {
    await apiPost(`/api/providers/${props.id}/refresh-models`)
    toast.success('Model list refreshed from upstream')
    await loadModels()
  } catch (e: any) { toast.error(e.message) }
  modelsLoading.value = false
}

async function saveSettings() {
  saving.value = true
  try {
    await apiPut(`/api/providers/${props.id}`, {
      isActive: conn.value?.isActive ?? true,
      data: { ...conn.value?.data, baseUrl: baseUrl.value || undefined, strategy: strategy.value || undefined },
    })
    toast.success('Provider settings saved')
    await store.loadCore()
  } catch (e: any) { toast.error(e.message) }
  saving.value = false
}

async function doDelete() {
  if (!conn.value) return
  await store.deleteProvider(conn.value)
  router.push('/providers')
}
</script>

<template>
  <div class="page">
    <!-- Header -->
    <header class="page-head">
      <button class="back-btn" @click="router.push('/providers')" aria-label="Back">
        <ArrowLeft :size="15" />
      </button>
      <div class="head-info">
        <h1 class="page-title">{{ conn?.name || conn?.provider || '…' }}</h1>
        <p class="page-desc mono">{{ conn?.provider }} · {{ conn?.authType }}</p>
      </div>
      <div class="head-right">
        <GBadge :color="conn?.isActive ? 'green' : 'gray'">{{ conn?.isActive ? 'Connected' : 'Disabled' }}</GBadge>
        <GButton variant="ghost" size="sm" :loading="testing" @click="test">
          <Zap :size="13" /> Test
        </GButton>
      </div>
    </header>
    <p v-if="testResult" class="test-line" :class="testResult.ok ? 'ok' : 'fail'">
      {{ testResult.ok ? `Reachable — ${testResult.latencyMs || 0}ms` : `Failed: ${testResult.error || 'unreachable'}` }}
    </p>

    <!-- Tabs -->
    <div class="tabs">
      <button class="tab" :class="{ active: tab === 'connections' }" @click="tab = 'connections'">Connections</button>
      <button class="tab" :class="{ active: tab === 'models' }" @click="tab = 'models'">Models</button>
      <button class="tab" :class="{ active: tab === 'settings' }" @click="tab = 'settings'">Settings</button>
    </div>

    <!-- Connections tab -->
    <div v-if="tab === 'connections'" class="tab-panel">
      <div class="panel-actions">
        <GButton size="sm" @click="showAddKey = true"><KeyRound :size="13" /> Add Key</GButton>
        <GButton variant="ghost" size="sm" @click="store.toggleProvider(conn!)" v-if="conn">
          {{ conn.isActive ? 'Disable' : 'Enable' }}
        </GButton>
        <GButton variant="ghost" size="sm" @click="store.resetCooldown(conn!)" v-if="conn">
          <RefreshCw :size="13" /> Reset Cooldown
        </GButton>
      </div>
      <GCard class="conn-info">
        <div class="info-grid">
          <div class="info-item"><span class="info-label">ID</span><code>{{ conn?.id }}</code></div>
          <div class="info-item"><span class="info-label">Priority</span><span>{{ conn?.priority }}</span></div>
          <div class="info-item"><span class="info-label">Auth</span><span>{{ conn?.authType }}</span></div>
          <div class="info-item"><span class="info-label">Email</span><span>{{ conn?.email || '—' }}</span></div>
        </div>
      </GCard>
    </div>

    <!-- Models tab -->
    <div v-if="tab === 'models'" class="tab-panel">
      <div class="panel-actions">
        <GButton size="sm" @click="showAddModel = true"><Plus :size="13" /> Add Custom Model</GButton>
        <GButton variant="ghost" size="sm" :loading="modelsLoading" @click="refreshModels">
          <RefreshCw :size="13" /> Refresh from Upstream
        </GButton>
      </div>
      <GSkeleton v-if="modelsLoading && !models.length" height="120px" />
      <div v-else-if="models.length" class="model-list stagger">
        <div v-for="m in models" :key="m.name" class="model-row">
          <div class="model-info">
            <span class="model-name">{{ m.displayName || m.name }}</span>
            <span v-if="m.contextLength" class="model-meta">{{ Math.round(m.contextLength / 1000) }}K ctx</span>
          </div>
          <div class="model-caps" v-if="m.capabilities?.length">
            <GBadge v-for="c in m.capabilities.slice(0, 3)" :key="c" color="violet">{{ c }}</GBadge>
          </div>
          <button class="icon-btn danger" @click="removeModel(m.name)" title="Remove model" v-if="m.source === 'custom'">
            <Trash2 :size="13" />
          </button>
        </div>
      </div>
      <p v-else class="empty-note">No models listed. Use "Refresh from Upstream" or add custom models.</p>
    </div>

    <!-- Settings tab -->
    <div v-if="tab === 'settings'" class="tab-panel">
      <GCard class="settings-card">
        <label class="field-label">Base URL Override</label>
        <input v-model="baseUrl" class="field" placeholder="Leave empty for default">
        <label class="field-label">Strategy Override</label>
        <input v-model="strategy" class="field" placeholder="e.g. round-robin, sticky">
        <div class="settings-actions">
          <GButton size="sm" :loading="saving" @click="saveSettings">Save</GButton>
        </div>
      </GCard>

      <GCard class="danger-zone">
        <p class="dz-title">Danger Zone</p>
        <p class="dz-desc">Deleting this connection removes all its credentials permanently.</p>
        <GButton v-if="!confirmDelete" variant="danger" size="sm" @click="confirmDelete = true">
          <Trash2 :size="13" /> Delete Provider
        </GButton>
        <div v-else class="dz-confirm">
          <span class="dz-warn">Are you sure?</span>
          <GButton variant="danger" size="sm" @click="doDelete">Yes, delete</GButton>
          <GButton variant="ghost" size="sm" @click="confirmDelete = false">Cancel</GButton>
        </div>
      </GCard>
    </div>

    <!-- Add Key modal -->
    <GModal v-if="showAddKey" title="Add API Key" @close="showAddKey = false">
      <label class="field-label">Key Name (optional)</label>
      <input v-model="newKeyName" class="field" placeholder="work-key">
      <label class="field-label">API Key</label>
      <input v-model="newKey" class="field" type="password" placeholder="sk-…" @keyup.enter="addKey">
      <div class="modal-actions">
        <GButton variant="ghost" @click="showAddKey = false">Cancel</GButton>
        <GButton @click="addKey">Add Key</GButton>
      </div>
    </GModal>

    <!-- Add Model modal -->
    <GModal v-if="showAddModel" title="Add Custom Model" @close="showAddModel = false">
      <label class="field-label">Model ID</label>
      <input v-model="newModel" class="field" placeholder="my-custom-model" @keyup.enter="addModel">
      <div class="modal-actions">
        <GButton variant="ghost" @click="showAddModel = false">Cancel</GButton>
        <GButton @click="addModel">Add Model</GButton>
      </div>
    </GModal>
  </div>
</template>

<style scoped>
.page-head { display: flex; align-items: center; gap: 12px; margin-bottom: 6px; }
.back-btn {
  width: 32px; height: 32px; border-radius: var(--radius-sm);
  display: flex; align-items: center; justify-content: center;
  background: var(--glass); border: 1px solid var(--glass-border);
  color: var(--text-muted); cursor: pointer; transition: all 0.15s ease;
}
.back-btn:hover { color: var(--text); border-color: var(--glass-border-hover); }
.head-info { flex: 1; }
.page-title { font-size: 18px; font-weight: 700; letter-spacing: -0.02em; }
.page-desc { font-size: 11.5px; color: var(--text-faint); }
.mono { font-family: var(--font-mono); }
.head-right { display: flex; align-items: center; gap: 8px; }
.test-line { font-size: 12px; margin-bottom: 10px; }
.test-line.ok { color: var(--green); }
.test-line.fail { color: var(--red); }

.tabs { display: flex; gap: 4px; margin: 16px 0; border-bottom: 1px solid var(--glass-border); padding-bottom: 0; }
.tab {
  padding: 8px 14px; border: none; background: transparent;
  color: var(--text-muted); font-size: 12.5px; font-weight: 560;
  cursor: pointer; border-bottom: 2px solid transparent;
  margin-bottom: -1px; transition: all 0.15s ease;
}
.tab:hover { color: var(--text); }
.tab.active { color: var(--accent); border-bottom-color: var(--accent); }

.tab-panel { animation: fadeIn 0.2s ease; }
.panel-actions { display: flex; gap: 8px; margin-bottom: 14px; flex-wrap: wrap; }

.conn-info { padding: 16px; }
.info-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(180px, 1fr)); gap: 12px; }
.info-item { display: flex; flex-direction: column; gap: 3px; }
.info-label { font-size: 10.5px; text-transform: uppercase; letter-spacing: 0.06em; color: var(--text-faint); font-weight: 600; }
.info-item code { font-size: 11px; color: var(--text-muted); word-break: break-all; }
.info-item span:last-child { font-size: 12.5px; }

.model-list { display: flex; flex-direction: column; gap: 4px; }
.model-row {
  display: flex; align-items: center; gap: 10px;
  padding: 9px 14px; border-radius: var(--radius-sm);
  background: var(--glass); border: 1px solid var(--glass-border);
  transition: all 0.15s ease;
}
.model-row:hover { border-color: var(--glass-border-hover); }
.model-info { display: flex; align-items: baseline; gap: 8px; flex: 1; min-width: 0; }
.model-name { font-size: 12.5px; font-weight: 550; font-family: var(--font-mono); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.model-meta { font-size: 10.5px; color: var(--text-faint); white-space: nowrap; }
.model-caps { display: flex; gap: 4px; }
.empty-note { font-size: 12.5px; color: var(--text-faint); padding: 20px 0; }

.settings-card { max-width: 480px; }
.field-label { display: block; font-size: 11.5px; font-weight: 550; color: var(--text-muted); margin: 12px 0 5px; }
.field-label:first-child { margin-top: 0; }
.field {
  width: 100%; height: 34px; padding: 0 12px;
  background: var(--code-bg); color: var(--text);
  border: 1px solid var(--glass-border); border-radius: var(--radius-sm);
  font-size: 13px; outline: none; transition: all 0.15s ease;
}
.field::placeholder { color: var(--text-faint); }
.field:focus { border-color: var(--ring); box-shadow: 0 0 0 3px var(--ring-soft); }
.settings-actions { margin-top: 16px; }

.danger-zone { max-width: 480px; margin-top: 14px; border-color: rgba(248,113,113,0.2); }
.dz-title { font-size: 13px; font-weight: 650; color: var(--red); }
.dz-desc { font-size: 12px; color: var(--text-faint); margin: 4px 0 12px; }
.dz-confirm { display: flex; align-items: center; gap: 8px; }
.dz-warn { font-size: 12px; color: var(--red); font-weight: 600; }

.icon-btn {
  width: 26px; height: 26px; border-radius: var(--radius-xs);
  display: flex; align-items: center; justify-content: center;
  background: transparent; border: 1px solid transparent;
  color: var(--text-muted); cursor: pointer; transition: all 0.12s ease; flex-shrink: 0;
}
.icon-btn.danger:hover { color: var(--red); background: rgba(248,113,113,0.08); }
.modal-actions { display: flex; justify-content: flex-end; gap: 8px; margin-top: 20px; }
</style>
