<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { useGatewayStore, type RegistryProvider } from '@/stores/gateway'
import { useToast } from '@/lib/toast'
import { apiPost } from '@/lib/api'
import GCard from '@/components/ui/GCard.vue'
import GButton from '@/components/ui/GButton.vue'
import GBadge from '@/components/ui/GBadge.vue'
import GModal from '@/components/ui/GModal.vue'
import GEmpty from '@/components/ui/GEmpty.vue'
import { Plus, Search, Zap, KeyRound, Layers } from 'lucide-vue-next'

const store = useGatewayStore()
const toast = useToast()
const router = useRouter()

const search = ref('')
const showAdd = ref(false)
const addTab = ref<'registry' | 'custom'>('registry')
const addSelected = ref<RegistryProvider | null>(null)
const addKey = ref('')
const addName = ref('')
const addBaseUrl = ref('')
const addProvider = ref('')
const submitting = ref(false)
const testResults = ref<Record<string, { ok: boolean; latencyMs?: number }>>({})
const testingAll = ref(false)
const enablingFree = ref(false)

onMounted(() => { if (!store.registryCategories.length) store.loadCore() })

const freeProviders = computed(() => store.registryList.filter(p => p.category === 'free' || p.category === 'freeTier'))

const filteredCategories = computed(() => {
  const q = search.value.toLowerCase()
  return store.registryCategories
    .map(cat => ({
      ...cat,
      providers: cat.providers.filter(p => !q || p.name.toLowerCase().includes(q) || p.id.toLowerCase().includes(q)),
    }))
    .filter(cat => cat.providers.length > 0)
})

function openAddFor(rp: RegistryProvider) {
  addSelected.value = rp
  addKey.value = ''
  showAdd.value = true
}

async function submitAdd() {
  if (submitting.value) return
  submitting.value = true
  try {
    if (addTab.value === 'registry' && addSelected.value) {
      await store.addProvider({
        provider: addSelected.value.id,
        authType: addSelected.value.authType || 'apikey',
        data: { apiKey: addKey.value },
      })
    } else {
      await store.addProvider({
        provider: addProvider.value || 'openai-compatible',
        name: addName.value || undefined,
        authType: 'apikey',
        data: { apiKey: addKey.value, baseUrl: addBaseUrl.value || undefined },
      })
    }
    showAdd.value = false
  } catch (e: any) {
    toast.error(`Failed: ${e.message}`)
  }
  submitting.value = false
}

async function testAll() {
  if (!store.providers.length || testingAll.value) return
  testingAll.value = true
  testResults.value = {}
  const active = store.providers.filter(p => p.isActive)
  const queue = [...active]
  const workers = Array.from({ length: Math.min(10, queue.length) }, async () => {
    while (queue.length) {
      const p = queue.shift()!
      try {
        const r = await store.testProvider(p.id)
        testResults.value[p.id] = { ok: r.ok, latencyMs: r.latencyMs }
      } catch {
        testResults.value[p.id] = { ok: false }
      }
    }
  })
  await Promise.all(workers)
  testingAll.value = false
}

async function enableFreeAll() {
  enablingFree.value = true
  try { await store.enableFree() } catch (e: any) { toast.error(e.message) }
  enablingFree.value = false
}

function connectionsFor(rpId: string) {
  return store.providers.filter(p => p.provider === rpId)
}
</script>

<template>
  <div class="page">
    <header class="page-head">
      <div>
        <h1 class="page-title">Providers</h1>
        <p class="page-desc">Manage upstream AI provider connections and credentials</p>
      </div>
      <div class="head-actions">
        <GButton variant="ghost" size="sm" :loading="testingAll" :disabled="!store.providers.length" @click="testAll">
          <Zap :size="13" /> Test All
        </GButton>
        <GButton size="sm" @click="showAdd = true; addTab = 'registry'">
          <Plus :size="13" /> Add Provider
        </GButton>
      </div>
    </header>

    <!-- Toolbar -->
    <div class="toolbar">
      <div class="search-wrap">
        <Search :size="14" class="search-icon" />
        <input v-model="search" class="search-input" placeholder="Search providers…">
      </div>
      <span class="conn-count">{{ store.providers.length }} connections · {{ store.activeConnections }} active</span>
    </div>

    <!-- First-run onboarding -->
    <GCard v-if="store.providers.length === 0" class="onboard stagger">
      <div class="onboard-icon"><Zap :size="20" /></div>
      <div class="onboard-text">
        <p class="onboard-title">Welcome to Cyrene Gateway</p>
        <p class="onboard-desc">No provider connections yet. Start with free providers — no API keys required — or add your own key below.</p>
      </div>
      <div class="onboard-actions">
        <GButton size="sm" :loading="enablingFree" @click="enableFreeAll">
          Enable {{ freeProviders.length }} free providers
        </GButton>
        <GButton variant="ghost" size="sm" @click="showAdd = true; addTab = 'registry'">
          <KeyRound :size="13" /> Add an API key provider
        </GButton>
      </div>
    </GCard>

    <!-- Connected providers -->
    <template v-if="store.providers.length > 0">
      <p class="section-label">Your Connections</p>
      <div class="conn-list stagger">
        <GCard v-for="p in store.providers" :key="p.id" class="conn-card" @click="router.push(`/providers/${p.id}`)">
          <div class="conn-left">
            <span class="conn-dot" :class="{ active: p.isActive }" />
            <div>
              <p class="conn-name">{{ p.name || p.provider }}</p>
              <p class="conn-meta">{{ p.provider }} · {{ p.authType }}</p>
            </div>
          </div>
          <div class="conn-right">
            <GBadge v-if="testResults[p.id]" :color="testResults[p.id].ok ? 'green' : 'red'">
              {{ testResults[p.id].ok ? (testResults[p.id].latencyMs || 0) + 'ms' : 'failed' }}
            </GBadge>
            <GBadge :color="p.isActive ? 'green' : 'gray'">{{ p.isActive ? 'active' : 'disabled' }}</GBadge>
          </div>
        </GCard>
      </div>
    </template>

    <!-- Registry by category -->
    <template v-for="cat in filteredCategories" :key="cat.category">
      <p class="section-label">{{ cat.category }} <span class="cat-count">{{ cat.providers.length }}</span></p>
      <div class="reg-grid stagger">
        <GCard v-for="rp in cat.providers.slice(0, search ? 100 : 12)" :key="rp.id" class="reg-card">
          <div class="reg-top">
            <span class="reg-name">{{ rp.name }}</span>
            <GBadge v-if="connectionsFor(rp.id).length" color="teal">{{ connectionsFor(rp.id).length }} conn</GBadge>
          </div>
          <p class="reg-id">{{ rp.id }}</p>
          <GButton
            v-if="rp.category === 'apikey'" size="sm" class="reg-btn"
            @click="openAddFor(rp)"
          >
            <KeyRound :size="12" /> Add Key
          </GButton>
          <GButton
            v-else-if="rp.category === 'free'" size="sm" variant="outline" class="reg-btn"
            :disabled="connectionsFor(rp.id).length > 0"
            @click="store.enableFree([rp.id])"
          >
            {{ connectionsFor(rp.id).length ? 'Enabled' : 'Enable' }}
          </GButton>
          <GButton v-else size="sm" variant="ghost" class="reg-btn" @click="openAddFor(rp)">Connect</GButton>
        </GCard>
      </div>
    </template>

    <!-- Add Provider modal -->
    <GModal v-if="showAdd" :title="addSelected ? `Add ${addSelected.name}` : 'Add Provider'" @close="showAdd = false; addSelected = null">
      <div class="tabs" v-if="!addSelected">
        <button class="tab" :class="{ active: addTab === 'registry' }" @click="addTab = 'registry'">Registry</button>
        <button class="tab" :class="{ active: addTab === 'custom' }" @click="addTab = 'custom'">Custom / OpenAI-compatible</button>
      </div>

      <template v-if="addTab === 'registry' && !addSelected">
        <p class="modal-hint">Pick a provider from the list above, or switch to Custom tab.</p>
      </template>

      <template v-else-if="addTab === 'registry' && addSelected">
        <label class="field-label">API Key</label>
        <input v-model="addKey" class="field" type="password" placeholder="sk-…" @keyup.enter="submitAdd">
      </template>

      <template v-else>
        <label class="field-label">Provider ID</label>
        <input v-model="addProvider" class="field" placeholder="openai-compatible">
        <label class="field-label">Display Name (optional)</label>
        <input v-model="addName" class="field" placeholder="My Provider">
        <label class="field-label">Base URL</label>
        <input v-model="addBaseUrl" class="field" placeholder="https://api.example.com/v1">
        <label class="field-label">API Key</label>
        <input v-model="addKey" class="field" type="password" placeholder="sk-…" @keyup.enter="submitAdd">
      </template>

      <div class="modal-actions">
        <GButton variant="ghost" @click="showAdd = false; addSelected = null">Cancel</GButton>
        <GButton :loading="submitting" @click="submitAdd">Add Provider</GButton>
      </div>
    </GModal>
  </div>
</template>

<style scoped>
.page-head { display: flex; align-items: flex-start; justify-content: space-between; gap: 16px; margin-bottom: 18px; flex-wrap: wrap; }
.page-title { font-size: 20px; font-weight: 700; letter-spacing: -0.03em; }
.page-desc { font-size: 12.5px; color: var(--text-faint); margin-top: 3px; }
.head-actions { display: flex; gap: 8px; }

.toolbar { display: flex; align-items: center; gap: 14px; margin-bottom: 20px; flex-wrap: wrap; }
.search-wrap { position: relative; flex: 1; max-width: 300px; }
.search-icon { position: absolute; left: 11px; top: 50%; transform: translateY(-50%); color: var(--text-faint); }
.search-input {
  width: 100%; height: 32px; padding: 0 12px 0 32px;
  background: var(--glass); border: 1px solid var(--glass-border);
  border-radius: var(--radius-sm); color: var(--text);
  font-size: 12.5px; outline: none; transition: all 0.15s ease;
}
.search-input:focus { border-color: var(--ring); box-shadow: 0 0 0 3px var(--ring-soft); }
.conn-count { font-size: 11.5px; color: var(--text-faint); }

.onboard { display: flex; align-items: center; gap: 16px; padding: 20px; margin-bottom: 24px; flex-wrap: wrap; }
.onboard-icon {
  width: 44px; height: 44px; border-radius: 12px; flex-shrink: 0;
  display: flex; align-items: center; justify-content: center;
  background: var(--gradient); color: var(--on-accent); box-shadow: var(--shadow-accent);
}
.onboard-text { flex: 1; min-width: 200px; }
.onboard-title { font-size: 14px; font-weight: 650; }
.onboard-desc { font-size: 12.5px; color: var(--text-muted); margin-top: 2px; }
.onboard-actions { display: flex; gap: 8px; flex-wrap: wrap; }

.section-label {
  font-size: 11px; font-weight: 650; text-transform: uppercase;
  letter-spacing: 0.07em; color: var(--text-faint); margin: 24px 0 10px;
  display: flex; align-items: center; gap: 8px;
}
.cat-count {
  font-size: 10px; background: var(--glass-hover); border: 1px solid var(--glass-border);
  padding: 1px 6px; border-radius: 99px;
}

.conn-list { display: flex; flex-direction: column; gap: 6px; }
.conn-card { display: flex; align-items: center; justify-content: space-between; padding: 12px 16px; cursor: pointer; }
.conn-left { display: flex; align-items: center; gap: 10px; }
.conn-dot { width: 8px; height: 8px; border-radius: 50%; background: var(--text-faint); }
.conn-dot.active { background: var(--green); box-shadow: 0 0 6px var(--green); }
.conn-name { font-size: 13px; font-weight: 600; }
.conn-meta { font-size: 11px; color: var(--text-faint); font-family: var(--font-mono); }
.conn-right { display: flex; gap: 6px; align-items: center; }

.reg-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(200px, 1fr)); gap: 8px; }
.reg-card { padding: 13px 15px; display: flex; flex-direction: column; gap: 6px; }
.reg-top { display: flex; align-items: center; justify-content: space-between; gap: 6px; }
.reg-name { font-size: 12.5px; font-weight: 600; }
.reg-id { font-size: 10.5px; color: var(--text-faint); font-family: var(--font-mono); }
.reg-btn { margin-top: auto; align-self: flex-start; }

.tabs { display: flex; gap: 4px; margin-bottom: 16px; }
.tab {
  padding: 6px 12px; border-radius: var(--radius-sm); border: 1px solid var(--glass-border);
  background: transparent; color: var(--text-muted); font-size: 12px; font-weight: 550;
  cursor: pointer; transition: all 0.15s ease;
}
.tab.active { background: var(--gradient-soft); color: var(--text); border-color: rgba(45,212,191,0.2); }

.modal-hint { font-size: 12.5px; color: var(--text-faint); }
.field-label { display: block; font-size: 11.5px; font-weight: 550; color: var(--text-muted); margin: 12px 0 5px; }
.field {
  width: 100%; height: 34px; padding: 0 12px;
  background: var(--code-bg); color: var(--text);
  border: 1px solid var(--glass-border); border-radius: var(--radius-sm);
  font-size: 13px; outline: none; transition: all 0.15s ease;
}
.field::placeholder { color: var(--text-faint); }
.field:focus { border-color: var(--ring); box-shadow: 0 0 0 3px var(--ring-soft); }
.modal-actions { display: flex; justify-content: flex-end; gap: 8px; margin-top: 20px; }
</style>
