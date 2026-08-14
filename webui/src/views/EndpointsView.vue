<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useGatewayStore } from '@/stores/gateway'
import { useToast } from '@/lib/toast'
import { api } from '@/lib/api'
import GCard from '@/components/ui/GCard.vue'
import GButton from '@/components/ui/GButton.vue'
import GBadge from '@/components/ui/GBadge.vue'
import {
  Copy, Check, Globe, Wifi, Radio, ShieldAlert, ShieldCheck,
  Zap, Plus, Trash2, KeyRound, Activity, X,
} from 'lucide-vue-next'

const store = useGatewayStore()
const toast = useToast()

const loading = ref(true)
const requireAuth = ref(false)
const pingResults = ref<Record<string, { ms: number; ok: boolean }>>({})
const pinging = ref(false)
const copied = ref('')
const newKeyName = ref('')
const creatingKey = ref(false)
const curlExample = ref('')
// One-time reveal: the raw key only exists in the create response (37A).
const revealedKey = ref<{ id: string; name?: string; key: string } | null>(null)

onMounted(async () => {
  if (!store.endpoints.length) await store.loadCore()
  requireAuth.value = !!(store.health.requireAuth)
  loading.value = false
  buildCurl()
})

function buildCurl() {
  const url = store.endpoints[0]?.url || 'http://localhost:20128'
  const key = revealedKey.value?.key || 'cg-your-api-key'
  const auth = requireAuth.value ? ` \\\n  -H "Authorization: Bearer ${key}"` : ''
  curlExample.value = `curl -X POST ${url}/v1/chat/completions${auth} \\\n  -H "Content-Type: application/json" \\\n  -d '{"model":"openai/gpt-4o","messages":[{"role":"user","content":"Hello"}]}'`
}

async function pingAll() {
  pinging.value = true
  pingResults.value = {}
  for (const ep of store.endpoints) {
    const start = performance.now()
    try {
      const res = await fetch(ep.url + '/api/health', { signal: AbortSignal.timeout(3000) })
      pingResults.value[ep.url] = { ms: Math.round(performance.now() - start), ok: res.ok }
    } catch {
      pingResults.value[ep.url] = { ms: 0, ok: false }
    }
  }
  pinging.value = false
}

function copyText(text: string, label: string) {
  navigator.clipboard.writeText(text).catch(() => {})
  copied.value = text
  toast.success(`${label} copied`)
  setTimeout(() => { copied.value = '' }, 2000)
}

async function createKey() {
  if (creatingKey.value) return
  creatingKey.value = true
  try {
    const k = await store.createKey(newKeyName.value || 'default')
    if (k?.key) revealedKey.value = { id: k.id, name: k.name, key: k.key }
    newKeyName.value = ''
    buildCurl()
  } catch (e: any) {
    toast.error(`Failed: ${e.message}`)
  }
  creatingKey.value = false
}

function dismissRevealedKey() {
  revealedKey.value = null
  buildCurl()
}

async function deleteKey(id: string) {
  await store.deleteKey(id)
  buildCurl()
}

const typeIcon: Record<string, any> = { local: Globe, lan: Wifi, tunnel: Radio }
</script>

<template>
  <div class="page">
    <header class="page-head">
      <div>
        <h1 class="page-title">Endpoint & Key</h1>
        <p class="page-desc">Gateway URLs, API keys, and quick-start examples</p>
      </div>
    </header>

    <!-- Health card -->
    <GCard class="health-card">
      <div class="health-row">
        <div class="health-item">
          <Activity :size="14" class="health-icon" />
          <span class="h-label">Status</span>
          <GBadge :color="store.health.status === 'ok' ? 'green' : 'amber'">{{ store.health.status || 'online' }}</GBadge>
        </div>
        <div class="health-item">
          <span class="h-label">Connections</span>
          <span class="h-val">{{ store.activeConnections }}</span>
        </div>
        <div class="health-item">
          <span class="h-label">Version</span>
          <span class="h-val mono">v{{ store.version }}</span>
        </div>
        <div class="health-item">
          <ShieldCheck v-if="requireAuth" :size="14" style="color:var(--green)" />
          <ShieldAlert v-else :size="14" style="color:var(--amber)" />
          <span class="h-label" :style="{ color: requireAuth ? 'var(--green)' : 'var(--amber)' }">
            {{ requireAuth ? 'Auth enabled' : 'Auth disabled' }}
          </span>
        </div>
      </div>
    </GCard>

    <!-- Gateway URLs -->
    <p class="section-label">Gateway URLs</p>
    <div class="ep-grid stagger">
      <GCard v-for="ep in store.endpoints" :key="ep.url" class="ep-card">
        <div class="ep-top">
          <component :is="typeIcon[ep.type] || Globe" :size="15" class="ep-icon" />
          <span class="ep-label">{{ ep.label }}</span>
          <GBadge v-if="pingResults[ep.url]" :color="pingResults[ep.url].ok ? 'green' : 'red'" class="ep-ping">
            {{ pingResults[ep.url].ok ? pingResults[ep.url].ms + 'ms' : 'unreachable' }}
          </GBadge>
        </div>
        <code class="ep-url">{{ ep.url }}</code>
        <button class="copy-btn" @click="copyText(ep.url, 'URL')" aria-label="Copy URL">
          <Check v-if="copied === ep.url" :size="13" />
          <Copy v-else :size="13" />
        </button>
      </GCard>
    </div>
    <div class="actions-row">
      <GButton variant="ghost" size="sm" :loading="pinging" @click="pingAll">
        <Zap :size="13" /> Ping All
      </GButton>
    </div>

    <!-- API Keys -->
    <p class="section-label">API Keys</p>
    <div class="keys-section">
      <!-- One-time key reveal after creation -->
      <div v-if="revealedKey" class="key-reveal">
        <div class="reveal-head">
          <KeyRound :size="14" />
          <span>Key "{{ revealedKey.name || revealedKey.id.slice(0, 8) }}" created — copy it now, it won't be shown again</span>
          <button class="icon-btn danger" @click="dismissRevealedKey" title="Dismiss" aria-label="Dismiss key reveal">
            <X :size="13" />
          </button>
        </div>
        <div class="reveal-row">
          <code class="key-val reveal-key">{{ revealedKey.key }}</code>
          <button class="icon-btn" @click="copyText(revealedKey.key, 'API Key')" title="Copy key">
            <Check v-if="copied === revealedKey.key" :size="13" style="color:var(--green)" />
            <Copy v-else :size="13" />
          </button>
        </div>
      </div>

      <div v-if="store.apiKeys.length === 0" class="empty-keys">
        <KeyRound :size="15" />
        <span>No API keys yet. Create one to authenticate requests.</span>
      </div>
      <div v-else class="keys-list stagger">
        <div v-for="k in store.apiKeys" :key="k.id" class="key-row">
          <code class="key-val">{{ k.keyHint || '••••' }}</code>
          <span class="key-name">{{ k.name || k.id.slice(0, 8) }}</span>
          <div class="key-actions">
            <button class="icon-btn danger" @click="deleteKey(k.id)" title="Delete key">
              <Trash2 :size="13" />
            </button>
          </div>
        </div>
      </div>
      <div class="key-create">
        <input v-model="newKeyName" class="field" placeholder="Key name (optional)" @keyup.enter="createKey">
        <GButton size="sm" :loading="creatingKey" @click="createKey">
          <Plus :size="13" /> Create Key
        </GButton>
      </div>
    </div>

    <!-- Quick Start -->
    <p class="section-label">Quick Start</p>
    <GCard class="curl-card">
      <pre class="curl-code">{{ curlExample }}</pre>
      <button class="copy-btn curl-copy" @click="copyText(curlExample, 'curl command')" aria-label="Copy curl">
        <Check v-if="copied === curlExample" :size="13" />
        <Copy v-else :size="13" />
      </button>
    </GCard>
  </div>
</template>

<style scoped>
.page { max-width: 840px; }
.page-head { margin-bottom: 22px; }
.page-title { font-size: 20px; font-weight: 700; letter-spacing: -0.03em; }
.page-desc { font-size: 12.5px; color: var(--text-faint); margin-top: 3px; }

.health-card { padding: 14px 18px; margin-bottom: 26px; }
.health-row { display: flex; align-items: center; gap: 22px; flex-wrap: wrap; }
.health-item { display: flex; align-items: center; gap: 6px; }
.health-icon { color: var(--accent); }
.h-label { font-size: 12px; color: var(--text-muted); }
.h-val { font-size: 12px; font-weight: 600; }
.mono { font-family: var(--font-mono); }

.section-label {
  font-size: 11px; font-weight: 650; text-transform: uppercase;
  letter-spacing: 0.07em; color: var(--text-faint); margin: 26px 0 10px;
}

.ep-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(260px, 1fr)); gap: 10px; }
.ep-card { position: relative; padding: 14px 16px; }
.ep-top { display: flex; align-items: center; gap: 8px; margin-bottom: 8px; }
.ep-icon { color: var(--accent); opacity: 0.85; }
.ep-label { font-size: 12.5px; font-weight: 560; }
.ep-ping { margin-left: auto; }
.ep-url { font-size: 12px; color: var(--text-muted); word-break: break-all; }

.copy-btn {
  position: absolute; top: 10px; right: 10px;
  width: 26px; height: 26px; border-radius: var(--radius-xs);
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
.key-reveal {
  margin-bottom: 10px; padding: 12px 14px; border-radius: var(--radius-sm);
  background: rgba(45,212,191,0.06); border: 1px solid rgba(45,212,191,0.25);
}
.reveal-head { display: flex; align-items: center; gap: 8px; font-size: 12px; color: var(--text-muted); margin-bottom: 8px; }
.reveal-head span { flex: 1; }
.reveal-row { display: flex; align-items: center; gap: 8px; }
.reveal-key { word-break: break-all; font-size: 12px; }
.key-row {
  display: flex; align-items: center; gap: 12px;
  padding: 10px 14px; border-radius: var(--radius-sm);
  background: var(--glass); border: 1px solid var(--glass-border);
  transition: all 0.15s ease;
}
.key-row:hover { border-color: var(--glass-border-hover); background: var(--glass-hover); }
.key-val { font-size: 12px; }
.key-name { font-size: 11.5px; color: var(--text-faint); margin-left: auto; }
.key-actions { display: flex; gap: 4px; }
.icon-btn {
  width: 26px; height: 26px; border-radius: var(--radius-xs);
  display: flex; align-items: center; justify-content: center;
  background: transparent; border: 1px solid transparent;
  color: var(--text-muted); cursor: pointer; transition: all 0.12s ease;
}
.icon-btn:hover { color: var(--text); background: var(--glass-hover); border-color: var(--glass-border); }
.icon-btn.danger:hover { color: var(--red); background: rgba(248,113,113,0.08); }
.key-create { display: flex; gap: 8px; margin-top: 10px; align-items: center; }

.field {
  flex: 1; max-width: 240px; height: 30px; padding: 0 11px;
  background: var(--code-bg); color: var(--text);
  border: 1px solid var(--glass-border); border-radius: var(--radius-sm);
  font-size: 12.5px; font-family: var(--font); outline: none; transition: all 0.15s ease;
}
.field::placeholder { color: var(--text-faint); }
.field:focus { border-color: var(--ring); box-shadow: 0 0 0 3px var(--ring-soft); }

.curl-card { position: relative; padding: 16px; }
.curl-code {
  font-size: 12px; color: var(--text-muted);
  white-space: pre-wrap; word-break: break-all; margin: 0; line-height: 1.7;
}
.curl-copy { top: 12px; }
</style>
