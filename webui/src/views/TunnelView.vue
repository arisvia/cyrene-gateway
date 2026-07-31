<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { api, apiPost } from '@/lib/api'
import { useToast } from '@/lib/toast'
import GCard from '@/components/ui/GCard.vue'
import GButton from '@/components/ui/GButton.vue'
import GBadge from '@/components/ui/GBadge.vue'
import { Cable } from 'lucide-vue-next'

const toast = useToast()
const loading = ref(true)
const status = ref<any>({})
const acting = ref(false)

async function load() {
  try { status.value = (await api('/api/tunnel/status')) || {} } catch { status.value = {} }
  loading.value = false
}

async function action(act: string) {
  acting.value = true
  try {
    await apiPost(`/api/tunnel/tailscale-${act}`)
    toast.success(`Tailscale ${act} initiated`)
    setTimeout(load, 1500)
  } catch (e: any) { toast.error(e.message) }
  acting.value = false
}

onMounted(load)
</script>

<template>
  <div class="page">
    <header class="page-head">
      <h1 class="page-title">Tunnel</h1>
      <p class="page-desc">Inbound tunnel management via Tailscale</p>
    </header>

    <GCard class="tunnel-card">
      <div class="tunnel-top">
        <div class="tunnel-icon"><Cable :size="18" /></div>
        <div class="tunnel-info">
          <p class="tunnel-name">Tailscale</p>
          <p class="tunnel-desc">
            <template v-if="status.installed">Installed · {{ status.running ? 'running' : 'stopped' }}</template>
            <template v-else>Not installed</template>
          </p>
        </div>
        <GBadge :color="status.running ? 'green' : 'gray'">{{ status.running ? 'active' : 'inactive' }}</GBadge>
      </div>

      <p v-if="status.dnsName" class="tunnel-url">{{ status.dnsName }}</p>

      <div class="tunnel-actions">
        <GButton v-if="!status.installed" size="sm" :loading="acting" @click="action('install')">Install Tailscale</GButton>
        <template v-else>
          <GButton v-if="!status.running" size="sm" :loading="acting" @click="action('enable')">Enable</GButton>
          <GButton v-else variant="ghost" size="sm" :loading="acting" @click="action('disable')">Disable</GButton>
        </template>
      </div>
    </GCard>
  </div>
</template>

<style scoped>
.page-head { margin-bottom: 22px; }
.page-title { font-size: 20px; font-weight: 700; letter-spacing: -0.03em; }
.page-desc { font-size: 12.5px; color: var(--text-faint); margin-top: 3px; }
.tunnel-card { max-width: 480px; }
.tunnel-top { display: flex; align-items: center; gap: 12px; }
.tunnel-icon {
  width: 40px; height: 40px; border-radius: 11px;
  display: flex; align-items: center; justify-content: center;
  background: var(--gradient-soft); border: 1px solid var(--glass-border); color: var(--accent);
}
.tunnel-info { flex: 1; }
.tunnel-name { font-size: 13.5px; font-weight: 650; }
.tunnel-desc { font-size: 11.5px; color: var(--text-faint); }
.tunnel-url {
  margin-top: 12px; padding: 8px 12px; border-radius: var(--radius-sm);
  background: var(--code-bg); font-family: var(--font-mono); font-size: 12px; color: var(--accent);
}
.tunnel-actions { margin-top: 16px; display: flex; gap: 8px; }
</style>
