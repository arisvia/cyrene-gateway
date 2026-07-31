<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { api, apiPost } from '@/lib/api'
import { useToast } from '@/lib/toast'
import GCard from '@/components/ui/GCard.vue'
import GButton from '@/components/ui/GButton.vue'
import GBadge from '@/components/ui/GBadge.vue'
import GSwitch from '@/components/ui/GSwitch.vue'
import { ShieldHalf, Download } from 'lucide-vue-next'

const toast = useToast()
const loading = ref(true)
const status = ref<any>({})
const acting = ref(false)
const dnsEnabled = ref(false)

async function load() {
  try {
    status.value = (await api('/api/mitm/status')) || {}
    dnsEnabled.value = !!status.value.dnsEnabled
  } catch { status.value = {} }
  loading.value = false
}

async function toggle() {
  acting.value = true
  try {
    await apiPost(status.value.running ? '/api/mitm/stop' : '/api/mitm/start')
    toast.success(status.value.running ? 'MITM proxy stopped' : 'MITM proxy started')
    setTimeout(load, 800)
  } catch (e: any) { toast.error(e.message) }
  acting.value = false
}

async function toggleDns() {
  try {
    await apiPost('/api/mitm/dns', { enabled: !dnsEnabled.value })
    dnsEnabled.value = !dnsEnabled.value
    toast.success(`DNS interception ${dnsEnabled.value ? 'enabled' : 'disabled'}`)
  } catch (e: any) { toast.error(e.message) }
}

async function downloadCert() {
  try {
    const r = await api('/api/mitm/cert')
    const pem = r?.cert || r?.pem || ''
    const blob = new Blob([pem], { type: 'application/x-x509-ca-cert' })
    const a = document.createElement('a')
    a.href = URL.createObjectURL(blob)
    a.download = 'cyrene-ca.pem'
    a.click()
  } catch (e: any) { toast.error(e.message) }
}

onMounted(load)
</script>

<template>
  <div class="page">
    <header class="page-head">
      <h1 class="page-title">MITM Proxy</h1>
      <p class="page-desc">Local TLS interception proxy for debugging AI client traffic (127.0.0.1 only)</p>
    </header>

    <GCard class="mitm-card">
      <div class="mitm-top">
        <div class="mitm-icon"><ShieldHalf :size="18" /></div>
        <div class="mitm-info">
          <p class="mitm-name">Interception Proxy</p>
          <p class="mitm-desc">{{ status.running ? `Listening on ${status.addr || '127.0.0.1:' + (status.port || 8080)}` : 'Stopped' }}</p>
        </div>
        <GBadge :color="status.running ? 'green' : 'gray'">{{ status.running ? 'running' : 'stopped' }}</GBadge>
      </div>

      <div class="mitm-rows">
        <div class="mitm-row">
          <div>
            <p class="row-label">DNS Interception</p>
            <p class="row-desc">Redirect AI API domains to the local proxy via hosts file</p>
          </div>
          <GSwitch :model-value="dnsEnabled" :disabled="!status.running" @update:model-value="toggleDns" />
        </div>
      </div>

      <div class="mitm-actions">
        <GButton :variant="status.running ? 'ghost' : 'primary'" size="sm" :loading="acting" @click="toggle">
          {{ status.running ? 'Stop Proxy' : 'Start Proxy' }}
        </GButton>
        <GButton variant="ghost" size="sm" @click="downloadCert"><Download :size="13" /> CA Certificate</GButton>
      </div>
    </GCard>
  </div>
</template>

<style scoped>
.page-head { margin-bottom: 22px; }
.page-title { font-size: 20px; font-weight: 700; letter-spacing: -0.03em; }
.page-desc { font-size: 12.5px; color: var(--text-faint); margin-top: 3px; max-width: 520px; }
.mitm-card { max-width: 520px; }
.mitm-top { display: flex; align-items: center; gap: 12px; }
.mitm-icon {
  width: 40px; height: 40px; border-radius: 11px;
  display: flex; align-items: center; justify-content: center;
  background: var(--gradient-soft); border: 1px solid var(--glass-border); color: var(--accent);
}
.mitm-info { flex: 1; }
.mitm-name { font-size: 13.5px; font-weight: 650; }
.mitm-desc { font-size: 11.5px; color: var(--text-faint); font-family: var(--font-mono); }
.mitm-rows { margin-top: 16px; border-top: 1px solid var(--glass-border); }
.mitm-row {
  display: flex; align-items: center; justify-content: space-between; gap: 16px; padding: 13px 0;
}
.row-label { font-size: 12.5px; font-weight: 600; }
.row-desc { font-size: 11px; color: var(--text-faint); margin-top: 2px; }
.mitm-actions { margin-top: 14px; display: flex; gap: 8px; }
</style>
