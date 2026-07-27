<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { api, apiPost } from '@/lib/api'
import GButton from '@/components/ui/GButton.vue'
import GBadge from '@/components/ui/GBadge.vue'
import GCard from '@/components/ui/GCard.vue'
import { Play, Square, RefreshCw, Download, ShieldAlert, ShieldCheck } from 'lucide-vue-next'

const status = ref<Record<string, any>>({})
const traffic = ref<any[]>([])
const busy = ref(false)
const msg = ref('')
let timer: ReturnType<typeof setInterval> | null = null

async function load() {
  try { status.value = await api('/api/mitm/status') } catch { status.value = {} }
  if (status.value.enabled && status.value.running) {
    try {
      const t = await api('/api/mitm/traffic')
      traffic.value = (t.traffic || []).slice().reverse()
    } catch { traffic.value = [] }
  } else {
    traffic.value = []
  }
}

async function start() {
  busy.value = true; msg.value = 'Starting MITM proxy...'
  try {
    const res = await apiPost('/api/mitm/start')
    msg.value = res.error ? res.error : 'MITM proxy started.'
  } catch { msg.value = 'Failed to start' }
  busy.value = false
  load()
}

async function stop() {
  busy.value = true; msg.value = 'Stopping...'
  try {
    const res = await apiPost('/api/mitm/stop')
    msg.value = res.error ? res.error : 'MITM proxy stopped. DNS entries removed.'
  } catch { msg.value = 'Failed to stop' }
  busy.value = false
  load()
}

async function toggleDns(tool: string, enabled: boolean) {
  msg.value = (enabled ? 'Enabling' : 'Disabling') + ' DNS for ' + tool + '...'
  try {
    const res = await apiPost('/api/mitm/dns', { tool, enabled })
    if (res.error) msg.value = res.error
    else msg.value = 'DNS ' + (enabled ? 'enabled' : 'disabled') + ' for ' + tool + '.'
  } catch { msg.value = 'DNS toggle failed (root/admin required to edit hosts file)' }
  load()
}

function downloadCert() {
  window.location.href = '/api/mitm/cert'
}

const toolLabels: Record<string, string> = {
  antigravity: 'Antigravity',
  copilot: 'GitHub Copilot',
  kiro: 'Kiro',
  cursor: 'Cursor',
}

const toolEntries = computed(() => {
  const tools = (status.value.tools || {}) as Record<string, string[]>
  return Object.entries(tools).map(([tool, hosts]) => ({ tool, hosts }))
})

onMounted(() => {
  load()
  timer = setInterval(load, 5000)
})
onUnmounted(() => { if (timer) clearInterval(timer) })
</script>

<template>
  <div style="max-width:760px;margin:0 auto">
    <div class="page-header">
      <h1 class="page-title">MITM Proxy</h1>
      <p class="page-desc">Local TLS interception to route IDE/CLI tool traffic through the gateway. Local deployments only.</p>
    </div>

    <!-- Disabled state -->
    <GCard v-if="status.enabled === false" pad>
      <div class="disabled-box">
        <ShieldAlert :size="26" style="color:var(--amber)" />
        <p class="disabled-title">MITM is disabled</p>
        <p class="disabled-desc">{{ status.reason || 'Start the gateway with the -mitm flag to enable.' }}</p>
        <div class="code-block">./gateway -host 127.0.0.1 -mitm</div>
        <p class="disabled-note">For safety, MITM only runs when the gateway is bound to localhost. Server deployments (0.0.0.0) cannot enable it.</p>
      </div>
    </GCard>

    <template v-else>
      <!-- Status card -->
      <GCard pad>
        <p class="card-section-title">Proxy Status</p>
        <div class="kv-row">
          <span class="kv-label">State</span>
          <GBadge :color="status.running ? 'green' : 'red'">
            <ShieldCheck v-if="status.running" :size="11" />{{ status.running ? 'Running' : 'Stopped' }}
          </GBadge>
        </div>
        <div class="kv-row"><span class="kv-label">Listen Port</span><span class="kv-value mono">{{ status.port }}</span></div>
        <div class="kv-row">
          <span class="kv-label">Root CA</span>
          <GBadge :color="status.certExists ? 'green' : 'amber'">{{ status.certExists ? 'Generated' : 'Not generated' }}</GBadge>
        </div>
        <div v-if="status.certPath" class="kv-row"><span class="kv-label">CA Path</span><span class="kv-value mono" style="font-size:0.76rem;color:var(--text-faint)">{{ status.certPath }}</span></div>
        <div style="display:flex;gap:8px;margin-top:16px;flex-wrap:wrap">
          <GButton v-if="!status.running" size="sm" @click="start" :disabled="busy"><Play :size="13" />Start Proxy</GButton>
          <GButton v-else variant="ghost" size="sm" @click="stop" :disabled="busy"><Square :size="13" />Stop Proxy</GButton>
          <GButton variant="ghost" size="sm" @click="downloadCert" :disabled="!status.certExists"><Download :size="13" />Download CA</GButton>
          <GButton variant="ghost" size="sm" @click="load" style="margin-left:auto"><RefreshCw :size="13" />Refresh</GButton>
        </div>
        <div v-if="msg" class="msg-box">{{ msg }}</div>
      </GCard>

      <!-- CA install instructions -->
      <GCard pad style="margin-top:16px">
        <p class="card-section-title">Install Root CA</p>
        <p class="hint">Trust the downloaded <span class="mono">cyrene-mitm-rootCA.crt</span> in your OS so intercepted tools accept the proxy certificate.</p>
        <div class="os-grid">
          <div class="os-item"><span class="os-name">macOS</span><code>sudo security add-trusted-cert -d -r trustRoot -k /Library/Keychains/System.keychain cyrene-mitm-rootCA.crt</code></div>
          <div class="os-item"><span class="os-name">Linux</span><code>sudo cp cyrene-mitm-rootCA.crt /usr/local/share/ca-certificates/cyrene.crt &amp;&amp; sudo update-ca-certificates</code></div>
          <div class="os-item"><span class="os-name">Windows</span><code>certutil -addstore -f "ROOT" cyrene-mitm-rootCA.crt</code></div>
        </div>
      </GCard>

      <!-- DNS rules -->
      <GCard pad style="margin-top:16px">
        <p class="card-section-title">Domain Interception (DNS)</p>
        <p class="hint">Toggle which tools have their domains pointed to 127.0.0.1 via the hosts file. Requires root/admin.</p>
        <div v-for="entry in toolEntries" :key="entry.tool" class="dns-row">
          <div class="dns-info">
            <span class="dns-tool">{{ toolLabels[entry.tool] || entry.tool }}</span>
            <span class="dns-hosts mono">{{ entry.hosts.join(', ') }}</span>
          </div>
          <div style="display:flex;align-items:center;gap:8px">
            <GBadge :color="(status.dns || {})[entry.tool] ? 'green' : 'glass'">{{ (status.dns || {})[entry.tool] ? 'Active' : 'Inactive' }}</GBadge>
            <GButton v-if="!(status.dns || {})[entry.tool]" size="sm" @click="toggleDns(entry.tool, true)" :disabled="!status.running">Enable</GButton>
            <GButton v-else variant="ghost" size="sm" @click="toggleDns(entry.tool, false)">Disable</GButton>
          </div>
        </div>
      </GCard>

      <!-- Traffic log -->
      <GCard pad style="margin-top:16px">
        <p class="card-section-title">Traffic Log</p>
        <div v-if="traffic.length === 0" class="empty">No intercepted traffic yet.</div>
        <div v-else class="traffic-table">
          <div class="t-head">
            <span>Time</span><span>Tool</span><span>Host</span><span>Model</span><span>Action</span><span>Status</span><span>Latency</span>
          </div>
          <div v-for="(e, i) in traffic" :key="i" class="t-row">
            <span class="mono">{{ new Date(e.time).toLocaleTimeString() }}</span>
            <span>{{ e.tool || '—' }}</span>
            <span class="mono" style="font-size:0.72rem">{{ e.host }}</span>
            <span class="mono" style="font-size:0.72rem">{{ e.model || '—' }}</span>
            <GBadge :color="e.action === 'intercepted' ? 'violet' : 'glass'">{{ e.action }}</GBadge>
            <span :style="{ color: e.status >= 400 ? 'var(--red)' : 'var(--text-muted)' }">{{ e.status }}</span>
            <span class="mono">{{ e.latencyMs }}ms</span>
          </div>
        </div>
      </GCard>
    </template>
  </div>
</template>

<style scoped>
.card-section-title { font-size: 11px; font-weight: 600; text-transform: uppercase; letter-spacing: 0.06em; color: var(--text-faint); margin-bottom: 12px; }
.kv-row { display: flex; align-items: center; justify-content: space-between; padding: 9px 0; font-size: 13px; }
.kv-row + .kv-row { border-top: 1px solid var(--row-divider); }
.kv-label { color: var(--text-muted); }
.kv-value { font-family: var(--font-mono); font-size: 0.82rem; }
.mono { font-family: var(--font-mono); }
.msg-box { margin-top: 12px; padding: 10px 12px; border-radius: var(--radius-sm); background: var(--code-bg); font-size: 0.82rem; color: var(--text-muted); }
.hint { font-size: 0.82rem; color: var(--text-muted); margin-bottom: 12px; line-height: 1.5; }

.disabled-box { text-align: center; padding: 20px 0; }
.disabled-title { font-size: 15px; font-weight: 600; margin: 12px 0 6px; }
.disabled-desc { font-size: 0.85rem; color: var(--text-muted); margin-bottom: 16px; }
.disabled-note { font-size: 0.76rem; color: var(--text-faint); margin-top: 14px; line-height: 1.5; }
.code-block { display: inline-block; padding: 8px 14px; border-radius: var(--radius-sm); background: var(--code-bg); font-family: var(--font-mono); font-size: 0.8rem; color: var(--text); }

.os-grid { display: flex; flex-direction: column; gap: 10px; }
.os-item { display: flex; flex-direction: column; gap: 4px; }
.os-name { font-size: 11px; font-weight: 600; color: var(--text-muted); }
.os-item code { padding: 8px 10px; border-radius: var(--radius-sm); background: var(--code-bg); font-family: var(--font-mono); font-size: 0.72rem; color: var(--text-muted); overflow-x: auto; white-space: nowrap; }

.dns-row { display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 12px 0; }
.dns-row + .dns-row { border-top: 1px solid var(--row-divider); }
.dns-info { display: flex; flex-direction: column; gap: 3px; min-width: 0; }
.dns-tool { font-size: 13px; font-weight: 550; }
.dns-hosts { font-size: 0.7rem; color: var(--text-faint); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }

.empty { text-align: center; padding: 24px 0; color: var(--text-faint); font-size: 0.85rem; }
.traffic-table { font-size: 0.78rem; overflow-x: auto; }
.t-head, .t-row { display: grid; grid-template-columns: 80px 90px 1fr 1fr 90px 55px 60px; gap: 8px; align-items: center; padding: 7px 0; }
.t-head { font-size: 10px; text-transform: uppercase; letter-spacing: 0.05em; color: var(--text-faint); font-weight: 600; border-bottom: 1px solid var(--row-divider); }
.t-row { border-bottom: 1px solid var(--row-divider); color: var(--text-muted); }
.t-row > span { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
</style>
