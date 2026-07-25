<script setup lang="ts">
import { useGatewayStore } from '@/stores/gateway'
import { formatNum, formatUptime } from '@/lib/format'
import GCard from '@/components/ui/GCard.vue'
import GBadge from '@/components/ui/GBadge.vue'

const store = useGatewayStore()
</script>

<template>
  <div>
    <div class="page-header">
      <h1 class="page-title">Status</h1>
      <p class="page-desc">Gateway health and resource overview.</p>
    </div>

    <div class="grid-2">
      <GCard pad>
        <p class="card-section-title">Core</p>
        <div class="kv-row"><span class="kv-label">Service</span><span class="kv-value">{{ store.health.service || '—' }}</span></div>
        <div class="kv-row"><span class="kv-label">Status</span><GBadge color="green">{{ store.health.status || 'active' }}</GBadge></div>
        <div class="kv-row"><span class="kv-label">Version</span><span class="kv-value">v{{ store.version }}</span></div>
        <div class="kv-row"><span class="kv-label">Database</span><GBadge :color="store.health.db === 'ok' ? 'green' : 'red'">{{ store.health.db || '—' }}</GBadge></div>
        <div class="kv-row"><span class="kv-label">Uptime</span><span class="kv-value">{{ formatUptime(store.health.uptimeSeconds) }}</span></div>
        <div class="kv-row"><span class="kv-label">Server Time</span><span class="kv-value">{{ store.health.time || '—' }}</span></div>
      </GCard>
      <GCard pad>
        <p class="card-section-title">Resources</p>
        <div class="kv-row"><span class="kv-label">Connections</span><span class="kv-value">{{ store.health.connections ?? store.providers.length }} ({{ store.health.activeConnections ?? store.providers.filter(p => p.isActive).length }} active)</span></div>
        <div class="kv-row"><span class="kv-label">API Keys</span><span class="kv-value">{{ store.apiKeys.length }}</span></div>
        <div class="kv-row"><span class="kv-label">Combos</span><span class="kv-value">{{ store.combos.length }}</span></div>
        <div class="kv-row"><span class="kv-label">Aliases</span><span class="kv-value">{{ Object.keys(store.aliases).length }}</span></div>
        <div class="kv-row"><span class="kv-label">Proxy Pools</span><span class="kv-value">{{ store.proxyPools.length }}</span></div>
        <div class="kv-row"><span class="kv-label">Lifetime Requests</span><span class="kv-value">{{ formatNum(store.usageStats.totalRequestsLifetime) }}</span></div>
      </GCard>
    </div>
  </div>
</template>

<style scoped>
.card-section-title { font-size: 11px; font-weight: 600; text-transform: uppercase; letter-spacing: 0.06em; color: var(--text-faint); margin-bottom: 12px; }
.kv-row { display: flex; align-items: center; justify-content: space-between; padding: 9px 0; font-size: 13px; }
.kv-row + .kv-row { border-top: 1px solid var(--row-divider); }
.kv-label { color: var(--text-muted); }
.kv-value { font-family: var(--font-mono); font-size: 11.5px; }
</style>
