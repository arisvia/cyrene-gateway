<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { api } from '@/lib/api'
import { useGatewayStore } from '@/stores/gateway'
import { useToast } from '@/lib/toast'
import GCard from '@/components/ui/GCard.vue'
import GButton from '@/components/ui/GButton.vue'
import GBadge from '@/components/ui/GBadge.vue'
import { Copy, Check, Globe, Wifi, Radio, ShieldAlert, ShieldCheck, Zap } from 'lucide-vue-next'

const store = useGatewayStore()
const toast = useToast()

interface Endpoint {
  label: string
  url: string
  type: string
}

const endpoints = ref<Endpoint[]>([])
const requireAuth = ref(false)
const loading = ref(true)
const pingResults = ref<Record<string, { ms: number; ok: boolean }>>({})
const pinging = ref(false)
const copied = ref('')

async function load() {
  loading.value = true
  try {
    const res = await api('/api/endpoints')
    endpoints.value = res.endpoints || []
    requireAuth.value = res.requireAuth || false
  } catch (e) {
    toast.error('Failed to load endpoints')
  }
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
    } catch {
      pingResults.value[ep.url] = { ms: 0, ok: false }
    }
  }
  pinging.value = false
}

function copyText(text: string, label: string) {
  navigator.clipboard.writeText(text)
  copied.value = text
  toast.success(`${label} copied`)
  setTimeout(() => { copied.value = '' }, 2000)
}

const curlExample = ref('')
function buildCurl() {
  const url = endpoints.value[0]?.url || 'http://localhost:20128'
  const key = store.apiKeys[0]?.key || 'sk-...'
  const auth = requireAuth.value ? ` \\\n  -H "Authorization: Bearer ${key}"` : ''
  curlExample.value = `curl -X POST ${url}/v1/chat/completions${auth} \\\n  -H "Content-Type: application/json" \\\n  -d '{"model":"openai/gpt-4o","messages":[{"role":"user","content":"Hello"}]}'`
}

const typeIcon: Record<string, any> = { local: Globe, lan: Wifi, tunnel: Radio }

onMounted(() => { load(); buildCurl() })
</script>

<template>
  <div class="page">
    <div class="page-header">
      <h1 class="page-title">Endpoints & Keys</h1>
      <p class="page-sub">Gateway URLs, quick-copy API keys, and connection examples</p>
    </div>

    <!-- Auth warning -->
    <div v-if="!requireAuth" class="auth-warning">
      <ShieldAlert :size="16" />
      <span>Authentication is <strong>disabled</strong>. Anyone with network access can use your gateway. Consider enabling API key requirement in Settings.</span>
    </div>
    <div v-else class="auth-ok">
      <ShieldCheck :size="16" />
      <span>API key authentication is <strong>enabled</strong>.</span>
    </div>

    <!-- Endpoint cards -->
    <div class="section-label">Gateway URLs</div>
    <div class="ep-grid">
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

    <!-- API Keys quick copy -->
    <div class="section-label">API Keys</div>
    <div v-if="store.apiKeys.length === 0" class="empty-note">No API keys yet. Create one in the API Keys page.</div>
    <div v-else class="keys-list">
      <div v-for="k in store.apiKeys" :key="k.id" class="key-row">
        <code class="key-val">{{ k.key.slice(0, 12) }}…{{ k.key.slice(-4) }}</code>
        <span class="key-name">{{ k.name || k.id.slice(0, 8) }}</span>
        <button class="copy-btn" @click="copyText(k.key, 'API Key')">
          <Check v-if="copied === k.key" :size="13" />
          <Copy v-else :size="13" />
        </button>
      </div>
    </div>

    <!-- Curl example -->
    <div class="section-label">Quick Start (curl)</div>
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
.page { max-width: 800px; }
.page-header { margin-bottom: 24px; }
.page-title { font-size: 20px; font-weight: 700; letter-spacing: -0.03em; }
.page-sub { font-size: 13px; color: var(--text-muted); margin-top: 4px; }

.auth-warning {
  display: flex; align-items: center; gap: 10px;
  background: rgba(251,191,36,0.08); border: 1px solid rgba(251,191,36,0.25);
  border-radius: var(--radius-sm); padding: 10px 14px;
  font-size: 12.5px; color: var(--yellow); margin-bottom: 20px;
}
.auth-ok {
  display: flex; align-items: center; gap: 10px;
  background: rgba(52,211,153,0.08); border: 1px solid rgba(52,211,153,0.25);
  border-radius: var(--radius-sm); padding: 10px 14px;
  font-size: 12.5px; color: var(--green); margin-bottom: 20px;
}

.section-label {
  font-size: 11px; font-weight: 600; text-transform: uppercase;
  letter-spacing: 0.06em; color: var(--text-faint); margin: 20px 0 10px;
}

.ep-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(280px, 1fr)); gap: 10px; }
.ep-card { position: relative; padding: 14px 16px; }
.ep-top { display: flex; align-items: center; gap: 8px; margin-bottom: 8px; }
.ep-icon { color: var(--accent); opacity: 0.8; }
.ep-label { font-size: 12.5px; font-weight: 550; }
.ep-ping { margin-left: auto; }
.ep-url {
  font-size: 12px; font-family: var(--font-mono);
  color: var(--text-muted); word-break: break-all;
}
.copy-btn {
  position: absolute; top: 10px; right: 10px;
  width: 26px; height: 26px; border-radius: var(--radius-sm);
  display: flex; align-items: center; justify-content: center;
  background: var(--glass-hover); border: 1px solid var(--glass-border);
  color: var(--text-muted); cursor: pointer; transition: all 0.15s ease;
}
.copy-btn:hover { color: var(--text); border-color: var(--glass-border-hover); }

.actions-row { margin-top: 12px; }

.empty-note { font-size: 12.5px; color: var(--text-faint); padding: 8px 0; }
.keys-list { display: flex; flex-direction: column; gap: 6px; }
.key-row {
  display: flex; align-items: center; gap: 12px;
  padding: 8px 12px; border-radius: var(--radius-sm);
  background: var(--glass); border: 1px solid var(--glass-border);
  position: relative;
}
.key-val { font-size: 12px; font-family: var(--font-mono); }
.key-name { font-size: 11.5px; color: var(--text-faint); margin-left: auto; margin-right: 30px; }
.key-row .copy-btn { position: absolute; top: 50%; right: 8px; transform: translateY(-50%); }

.curl-card { position: relative; padding: 16px; }
.curl-code {
  font-size: 12px; font-family: var(--font-mono);
  color: var(--text-muted); white-space: pre-wrap; word-break: break-all;
  margin: 0; line-height: 1.6;
}
.curl-copy { top: 12px; }
</style>
