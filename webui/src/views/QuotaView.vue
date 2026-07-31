<script setup lang="ts">
import { onMounted } from 'vue'
import { useGatewayStore } from '@/stores/gateway'
import { formatNumber } from '@/lib/format'
import GCard from '@/components/ui/GCard.vue'
import GBadge from '@/components/ui/GBadge.vue'
import GEmpty from '@/components/ui/GEmpty.vue'
import GSkeleton from '@/components/ui/GSkeleton.vue'
import { ref } from 'vue'

const store = useGatewayStore()
const loading = ref(true)

onMounted(async () => {
  await store.loadQuota()
  loading.value = false
})
</script>

<template>
  <div class="page">
    <header class="page-head">
      <h1 class="page-title">Quota Tracker</h1>
      <p class="page-desc">Per-connection usage limits and quota consumption</p>
    </header>

    <GSkeleton v-if="loading" height="160px" />
    <GEmpty v-else-if="!store.quotaEntries.length" title="No quota entries" desc="Quota limits appear here once configured on provider connections." />
    <div v-else class="quota-grid stagger">
      <GCard v-for="(q, i) in store.quotaEntries" :key="i" class="quota-card">
        <div class="q-top">
          <span class="q-name">{{ q.name || q.connectionId || q.provider }}</span>
          <GBadge :color="q.overQuota ? 'red' : 'green'">{{ q.overQuota ? 'Over quota' : 'OK' }}</GBadge>
        </div>
        <div class="q-bar-wrap">
          <div class="q-bar">
            <div class="q-fill" :class="{ over: q.overQuota }" :style="{ width: Math.min(100, q.limit ? ((q.used || 0) / q.limit) * 100 : 0) + '%' }" />
          </div>
        </div>
        <p class="q-text">{{ formatNumber(q.used || 0)}} / {{ q.limit ? formatNumber(q.limit) : '∞' }} requests</p>
      </GCard>
    </div>
  </div>
</template>

<style scoped>
.page-head { margin-bottom: 22px; }
.page-title { font-size: 20px; font-weight: 700; letter-spacing: -0.03em; }
.page-desc { font-size: 12.5px; color: var(--text-faint); margin-top: 3px; }
.quota-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(260px, 1fr)); gap: 10px; }
.quota-card { padding: 15px 17px; }
.q-top { display: flex; align-items: center; justify-content: space-between; margin-bottom: 10px; }
.q-name { font-size: 12.5px; font-weight: 600; }
.q-bar-wrap { margin-bottom: 8px; }
.q-bar { height: 6px; border-radius: 3px; background: var(--glass-hover); overflow: hidden; }
.q-fill { height: 100%; border-radius: 3px; background: var(--gradient); transition: width 0.4s var(--ease-out-expo); }
.q-fill.over { background: var(--red); }
.q-text { font-size: 11px; color: var(--text-faint); font-family: var(--font-mono); }
</style>
