<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { api, apiPost } from '@/lib/api'
import GButton from '@/components/ui/GButton.vue'
import GBadge from '@/components/ui/GBadge.vue'
import GCard from '@/components/ui/GCard.vue'
import { Download, Play, Square, RefreshCw } from 'lucide-vue-next'

const status = ref<Record<string, any>>({})
const busy = ref(false)
const msg = ref('')
const authUrl = ref('')
const installing = ref(false)
const installMsg = ref('')

async function load() {
  try { status.value = await api('/api/tunnel/status') } catch { status.value = {} }
}

async function enable() {
  busy.value = true; msg.value = 'Enabling tunnel...'; authUrl.value = ''
  try {
    const res = await apiPost('/api/tunnel/tailscale-enable')
    if (res.error) msg.value = res.error
    else if (res.needsLogin) { msg.value = 'Login required. '; authUrl.value = res.authUrl || '' }
    else if (res.success) msg.value = 'Tunnel active: ' + (res.tunnelUrl || '')
    else msg.value = JSON.stringify(res)
  } catch { msg.value = 'Request failed' }
  busy.value = false
  load()
}

async function disable() {
  busy.value = true; msg.value = 'Disabling...'; authUrl.value = ''
  try { await apiPost('/api/tunnel/tailscale-disable'); msg.value = 'Tunnel disabled.' }
  catch { msg.value = 'Failed to disable' }
  busy.value = false
  load()
}

async function install() {
  installing.value = true; installMsg.value = 'Starting installation...'
  try {
    const res = await fetch('/api/tunnel/tailscale-install', { method: 'POST' })
    const reader = res.body!.getReader()
    const decoder = new TextDecoder()
    let buf = ''
    while (true) {
      const { done, value } = await reader.read()
      if (done) break
      buf += decoder.decode(value, { stream: true })
      const lines = buf.split('\n')
      buf = lines.pop()!
      for (const line of lines) {
        if (line.startsWith('data: ')) {
          try {
            const d = JSON.parse(line.slice(6))
            if (d.message) installMsg.value = d.message
            if (d.error) installMsg.value = 'Error: ' + d.error
            if (d.success) installMsg.value = 'Installation complete!'
          } catch {}
        }
      }
    }
  } catch { installMsg.value = 'Install request failed' }
  installing.value = false
  load()
}

onMounted(load)
</script>

<template>
  <div style="max-width:640px;margin:0 auto">
    <div class="page-header">
      <h1 class="page-title">Tunnel</h1>
      <p class="page-desc">Tailscale inbound tunnel for secure remote access.</p>
    </div>

    <GCard pad>
      <p class="card-section-title">Tailscale Status</p>
      <div v-if="!status.installed" style="text-align:center;padding:24px 0">
        <p style="color:var(--text-muted);margin-bottom:12px">Tailscale is not installed on this system.</p>
        <GButton size="sm" @click="install" :disabled="installing">
          <Download :size="13" />{{ installing ? 'Installing...' : 'Install Tailscale' }}
        </GButton>
        <p v-if="installMsg" style="margin-top:8px;font-size:0.82rem;color:var(--text-faint)">{{ installMsg }}</p>
      </div>
      <template v-else>
        <div class="kv-row"><span class="kv-label">Installed</span><GBadge color="green">Yes</GBadge></div>
        <div class="kv-row"><span class="kv-label">Daemon</span><GBadge :color="status.daemonRunning ? 'green' : 'red'">{{ status.daemonRunning ? 'Running' : 'Stopped' }}</GBadge></div>
        <div class="kv-row"><span class="kv-label">Logged In</span><GBadge :color="status.loggedIn ? 'green' : 'red'">{{ status.loggedIn ? 'Yes' : 'No' }}</GBadge></div>
        <div class="kv-row"><span class="kv-label">Funnel</span><GBadge :color="status.funnelRunning ? 'green' : 'red'">{{ status.funnelRunning ? 'Active' : 'Inactive' }}</GBadge></div>
        <div v-if="status.tunnelUrl" class="kv-row"><span class="kv-label">Tunnel URL</span><span class="kv-value mono">{{ status.tunnelUrl }}</span></div>
        <div v-if="status.binPath" class="kv-row"><span class="kv-label">Binary</span><span class="kv-value mono" style="font-size:0.78rem;color:var(--text-faint)">{{ status.binPath }}</span></div>
        <div style="display:flex;gap:8px;margin-top:16px">
          <GButton size="sm" @click="enable" :disabled="busy"><Play :size="13" />{{ busy ? 'Working...' : 'Enable Funnel' }}</GButton>
          <GButton variant="ghost" size="sm" @click="disable" :disabled="busy"><Square :size="13" />Disable</GButton>
          <GButton variant="ghost" size="sm" @click="load" style="margin-left:auto"><RefreshCw :size="13" />Refresh</GButton>
        </div>
        <div v-if="msg" class="msg-box">
          {{ msg }}
          <a v-if="authUrl" :href="authUrl" target="_blank" style="color:var(--accent);text-decoration:underline">Open login page</a>
        </div>
      </template>
    </GCard>
  </div>
</template>

<style scoped>
.card-section-title { font-size: 11px; font-weight: 600; text-transform: uppercase; letter-spacing: 0.06em; color: var(--text-faint); margin-bottom: 12px; }
.kv-row { display: flex; align-items: center; justify-content: space-between; padding: 9px 0; font-size: 13px; }
.kv-row + .kv-row { border-top: 1px solid var(--row-divider); }
.kv-label { color: var(--text-muted); }
.kv-value { font-family: var(--font-mono); font-size: 0.82rem; }
.msg-box {
  margin-top: 12px; padding: 10px 12px; border-radius: var(--radius-sm);
  background: var(--code-bg); font-size: 0.82rem; color: var(--text-muted);
}
</style>
