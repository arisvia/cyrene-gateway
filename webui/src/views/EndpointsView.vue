<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { api, apiPost, apiDelete } from '@/lib/api'
import { useGatewayStore } from '@/stores/gateway'
import { useToast } from '@/lib/toast'
import GCard from '@/components/ui/GCard.vue'
import GButton from '@/components/ui/GButton.vue'
import GBadge from '@/components/ui/GBadge.vue'
import { Copy, Check, Globe, Wifi, Radio, ShieldAlert, ShieldCheck, Zap, Plus, Trash2, KeyRound, MessageSquare, Activity } from 'lucide-vue-next'

const store = useGatewayStore()
const toast = useToast()

interface Endpoint { label: string; url: string; type: string }

const endpoints = ref<Endpoint[]>([])
const requireAuth = ref(false)
const loading = ref(true)
const pingResults = ref<Record<string, { ms: number; ok: boolean }>>({})
const pinging = ref(false)
const copied = ref('')
const newKeyName = ref('')
const creatingKey = ref(false)

async function load() {
  loading.value = true
  try {
    const res = await api('/api/endpoints')
    endpoints.value = res.endpoints || []
    requireAuth.value = res.requireAuth || false
  } catch { toast.error('Failed to load endpoints') }
  loading.value = false
}

async function pingAll() {
  pinging.value = true
  pingResults.value = {}
  for (const ep of endpoints.value) {
    const start = performance.now()
    try {
      const res = await fetch(ep.url + '/api/health', { signal: AbortSignal.timeout(3000) })
      const ms = Math.round(performance.now() - start)
      pingResults.value[ep.url] = { ms, ok: res.ok }
    } catch { pingResults.value[ep.url] = { ms: 0, ok: false } }
  }
  pinging.value = false
}

function copyText(text: string, label: string) {
  navigator.clipboard.writeText(text)
  copied.value = text
  toast.success(`${label} copied`)
  setTimeout(() => { copied.value = '' }, 2000)
}

async function createKey() {
  if (creatingKey.value) return
  creatingKey.value = true
  try { await store.createKey(newKeyName.value || 'default') }
  catch (e: any) { toast.error(`Failed: ${e.message}`) }
  newKeyName.value = ''
  creatingKey.value = false
}

async function deleteKey(id: string) {
  const k = store.apiKeys.find(x => x.id === id)
  if (k) await store.deleteKey(k)
}

const curlExample = ref('')
function buildCurl() {
  const url = endpoints.value[0]?.url || 'http://localhost:20128'
  const key = store.apiKeys[0]?.key || 'sk-...'
  const auth = requireAuth.value ? ` \\\n  -H "Authorization: Bearer ${key}"` : ''
  curlExample.value = `curl -X POST ${url}/v1/chat/completions${auth} \\\n  -H "Content-Type: application/json" \\\n  -d '{"model":"openai/gpt-4o","messages":[{"role":"user","content":"Hello"}]}'`
}

const typeIcon: Record<string, any> = { local: Globe, lan: Wifi, tunnel: Radio }

onMounted(() => { load().then(buildCurl) })
</script>

<template>
  <div class="page">
    <div class="page-header">
      <h1 class="page-title">Endpoint & Key</h1>
      <p class="page-desc">Gateway URLs, API keys, and quick-start examples</p>
    </div>

    <!-- Health status card -->
    <GCard class="health-card">
      <div class="health-row">
        <div class="health-item">
          <Activity :size="14" class="health-icon" />
          <span class="health-label">Status</span>
          <GBadge :color="store.health.status === 'ok' ? 'green' : 'amber'">{{ store.health.status || 'online' }}</GBadge>
        </div>
        <div class="health-item">
          <span class="health-label">Connections</span>
          <span class="health-val mono">{{ store.health.activeConnections || 0 }}</span>
        </div>
        <div class="health-item">
          <span class="health-label">Version</span>
          <span class="health-val mono">v{{ store.version }}</span>
        </div>
        <div class="health-item" v-if="!requireAuth">
          <ShieldAlert :size="14" style="color:var(--amber)" />
          <span class="health-label" style="color:var(--amber)">Auth disabled</span>
        </div>
        <div class="health-item" v-else>
          <ShieldCheck :size="14" style="color:var(--green)" />
          <span class="health-label" style="color:var(--green)">Auth enabled</span>
        </div>
      </div>
    </GCard>

    <!-- Endpoint cards -->
    <div class="section-label">Gateway URLs</div>
    <div class="ep-grid stagger">
      <GCard v-for="ep in endpoints" :key="ep.url" class="ep-card">
        <div class="ep-top">
          <component :is="typeIcon[ep.type] || Globe" :size="15" class="ep-icon" />
          <span class="ep-label">{{ ep.label }}</span>
          <GBadge v-if="pingResults[ep.url]" :variant="pingResults[ep.url].ok ? 'green' : 'red'" class="ep-ping">
            {{ pingResults[ep.url].ok ? pingResults[ep.url].ms + 'ms' : 'unreachable' }}
          </GBadge>
        </div>
        <code class="ep-url">{{ ep.url }}</code>
        <button class="copy-btn" @click="copyText(ep.url, 'URL')">
          <Check v-if="copied === ep.url" :size="13" />
          <Copy v-else :size="13" />
        </button>
      </GCard>
    </div>
    <div class="actions-row">
      <GButton variant="ghost" size="sm" @click="pingAll" :disabled="pinging">
        <Zap :size="13" /> {{ pinging ? 'Pinging…' : 'Ping All' }}
      </GButton>
    </div>

    <!-- API Keys -->
    <div class="section-label">API Keys</div>
    <div class="keys-section">
      <div v-if="store.apiKeys.length === 0" class="empty-keys">
        <KeyRound :size="16" />
        <span>No API keys yet. Create one to authenticate requests.</span>
      </div>
      <div v-else class="keys-list stagger">
        <div v-for="k in store.apiKeys" :key="k.id" class="key-row">
          <code class="key-val">{{ k.key.slice(0, 14) }}…{{ k.key.slice(-4) }}</code>
          <span class="key-name">{{ k.name || k.id.slice(0, 8) }}</span>
          <div class="key-actions">
            <button class="icon-btn" @click="copyText(k.key, 'API Key')" title="Copy">
              <Check v-if="copied === k.key" :size="13" style="color:var(--green)" />
              <Copy v-else :size="13" />
            </button>
            <button class="icon-btn danger" @click="deleteKey(k.id)" title="Delete">
              <Trash2 :size="13" />
            </button>
          </div>
        </div>
      </div>
      <div class="key-create">
        <input v-model="newKeyName" class="input" placeholder="Key name (optional)" @keyup.enter="createKey">
        <GButton size="sm" @click="createKey" :disabled="creatingKey">
          <Plus :size="13" />{{ creatingKey ? 'Creating…' : 'Create Key' }}
        </GButton>
      </div>
    </div>

    <!-- Curl example -->
    <div class="section-label">Quick Start</div>
    <GCard class="curl-card">
      <pre class="curl-code">{{ curlExample }}</pre>
      <button class="copy-btn curl-copy" @click="copyText(curlExample, 'curl command')">
        <Check v-if="copied === curlExample" :size="13" />
        <Copy v-else :size="13" />
      </button>
    </GCard>
  </div>
</template>

<style scoped>
.page { max-width: 820px; }

.health-card { margin-bottom: 24px; padding: 14px 18px; }
.health-row { display: flex; align-items: center; gap: 24px; flex-wrap: wrap; }
.health-item { display: flex; align-items: center; gap: 6px; }
.health-icon { color: var(--accent); }
.health-label { font-size: 12px; color: var(--text-muted); }
.health-val { font-size: 12px; }

.section-label {
  font-size: 11px; font-weight: 600; text-transform: uppercase;
  letter-spacing: 0.06em; color: var(--text-faint); margin: 24px 0 10px;
}

.ep-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(280px, 1fr)); gap: 10px; }
.ep-card { position: relative; padding: 14px 16px; }
.ep-top { display: flex; align-items: center; gap: 8px; margin-bottom: 8px; }
.ep-icon { color: var(--accent); opacity: 0.8; }
.ep-label { font-size: 12.5px; font-weight: 550; }
.ep-ping { margin-left: auto; }
.ep-url { font-size: 12px; font-family: var(--font-mono); color: var(--text-muted); word-break: break-all; }
.copy-btn {
  position: absolute; top: 10px; right: 10px;
  width: 26px; height: 26px; border-radius: var(--radius-sm);
  display: flex; align-items: center; justify-content: center;
  background: var(--glass-hover); border: 1px solid var(--glass-border);
  color: var(--text-muted); cursor: pointer; transition: all 0.15s ease;
}
.copy-btn:hover { color: var(--text); border-color: var(--glass-border-hover); }
.actions-row { margin-top: 12px; }

.keys-section { margin-bottom: 8px; }
.empty-keys {
  display: flex; align-items: center; gap: 10px;
  padding: 14px 16px; border-radius: var(--radius);
  border: 1px dashed var(--glass-border); color: var(--text-faint); font-size: 12.5px;
}
.keys-list { display: flex; flex-direction: column; gap: 6px; }
.key-row {
  display: flex; align-items: center; gap: 12px;
  padding: 10px 14px; border-radius: var(--radius-sm);
  background: var(--glass); border: 1px solid var(--glass-border);
  transition: all 0.15s ease;
}
.key-row:hover { border-color: var(--glass-border-hover); background: var(--glass-hover); }
.key-val { font-size: 12px; font-family: var(--font-mono); }
.key-name { font-size: 11.5px; color: var(--text-faint); margin-left: auto; }
.key-actions { display: flex; gap: 4px; }
.icon-btn {
  width: 26px; height: 26px; border-radius: var(--radius-sm);
  display: flex; align-items: center; justify-content: center;
  background: transparent; border: 1px solid transparent;
  color: var(--text-muted); cursor: pointer; transition: all 0.12s ease;
}
.icon-btn:hover { color: var(--text); background: var(--glass-hover); border-color: var(--glass-border); }
.icon-btn.danger:hover { color: var(--red); background: rgba(248,113,113,0.08); }
.key-create { display: flex; gap: 8px; margin-top: 10px; align-items: center; }
.key-create .input { flex: 1; max-width: 240px; }

.input {
  height: 34px; padding: 0 12px;
  background: var(--code-bg); color: var(--text);
  border: 1px solid var(--glass-border); border-radius: var(--radius-sm);
  font-size: 13px; font-family: var(--font);
  transition: all 0.15s ease; outline: none;
}
.input::placeholder { color: var(--text-faint); }
.input:focus { border-color: var(--ring); box-shadow: 0 0 0 3px var(--ring-soft); }

.curl-card { position: relative; padding: 16px; }
.curl-code {
  font-size: 12px; font-family: var(--font-mono);
  color: var(--text-muted); white-space: pre-wrap; word-break: break-all;
  margin: 0; line-height: 1.6;
}
.curl-copy { top: 12px; }
</style>
