<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useGatewayStore } from '@/stores/gateway'
import { formatNumber, formatCost, timeAgo } from '@/lib/format'
import GCard from '@/components/ui/GCard.vue'
import GBadge from '@/components/ui/GBadge.vue'
import GButton from '@/components/ui/GButton.vue'
import GSkeleton from '@/components/ui/GSkeleton.vue'
import { ChevronLeft, ChevronRight } from 'lucide-vue-next'

const store = useGatewayStore()
const tab = ref<'overview' | 'details' | 'limits'>('overview')
const period = ref('7d')
const loading = ref(true)
const page = ref(1)

onMounted(async () => {
  await Promise.all([store.loadUsage(period.value), store.loadProviderUsage(period.value)])
  loading.value = false
})

async function setPeriod(p: string) {
  period.value = p
  loading.value = true
  await store.loadUsage(p)
  await store.loadProviderUsage(p)
  loading.value = false
}

async function goPage(p: number) {
  page.value = p
  await store.loadRequestDetails(p)
}

const maxChart = () => Math.max(...store.usageChart.map(c => c.tokens), 1)
</script>

<template>
  <div class="page">
    <header class="page-head">
      <div>
        <h1 class="page-title">Usage</h1>
        <p class="page-desc">Request analytics, token consumption, and per-provider breakdown</p>
      </div>
      <div class="period-btns">
        <button v-for="p in ['24h', '7d', '30d']" :key="p" class="period-btn" :class="{ active: period === p }" @click="setPeriod(p)">{{ p }}</button>
      </div>
    </header>

    <div class="tabs">
      <button class="tab" :class="{ active: tab === 'overview' }" @click="tab = 'overview'">Overview</button>
      <button class="tab" :class="{ active: tab === 'details' }" @click="tab = 'details'; store.loadRequestDetails(1)">Request Details</button>
      <button class="tab" :class="{ active: tab === 'limits' }" @click="tab = 'limits'">Provider Limits</button>
    </div>

    <GSkeleton v-if="loading" height="180px" />

    <!-- Overview -->
    <template v-else-if="tab === 'overview'">
      <div class="stat-grid stagger">
        <GCard class="stat-card">
          <p class="stat-val">{{ formatNumber(store.usageStats.totalRequests) }}</p>
          <p class="stat-label">Requests</p>
        </GCard>
        <GCard class="stat-card">
          <p class="stat-val">{{ formatNumber(store.usageStats.totalPromptTokens) }}</p>
          <p class="stat-label">Prompt Tokens</p>
        </GCard>
        <GCard class="stat-card">
          <p class="stat-val">{{ formatNumber(store.usageStats.totalCompletionTokens) }}</p>
          <p class="stat-label">Completion Tokens</p>
        </GCard>
        <GCard class="stat-card">
          <p class="stat-val">{{ formatCost(store.usageStats.totalCost) }}</p>
          <p class="stat-label">Est. Cost</p>
        </GCard>
      </div>

      <!-- Chart -->
      <GCard v-if="store.usageChart.length" class="chart-card">
        <div class="chart">
          <div v-for="(bar, i) in store.usageChart" :key="i" class="bar-wrap" :title="`${bar.label}: ${formatNumber(bar.tokens)} tokens`">
            <div class="bar" :style="{ height: Math.max(4, (bar.tokens / maxChart()) * 100) + '%' }" />
            <span class="bar-label">{{ bar.label }}</span>
          </div>
        </div>
      </GCard>
    </template>

    <!-- Request details -->
    <template v-else-if="tab === 'details'">
      <div class="table-wrap">
        <table class="table">
          <thead>
            <tr><th>Time</th><th>Provider</th><th>Model</th><th>Status</th><th>Tokens</th><th>Latency</th></tr>
          </thead>
          <tbody>
            <tr v-for="d in store.requestDetails" :key="d.id">
              <td class="mono">{{ timeAgo(d.timestamp) }}</td>
              <td>{{ d.provider }}</td>
              <td class="mono td-model">{{ d.model }}</td>
              <td><GBadge :color="d.status === 'ok' || d.status === '200' ? 'green' : 'red'">{{ d.status }}</GBadge></td>
              <td class="mono">{{ formatNumber((d.promptTokens || 0) + (d.completionTokens || 0)) }}</td>
              <td class="mono">{{ d.latencyMs ? d.latencyMs + 'ms' : '—' }}</td>
            </tr>
            <tr v-if="!store.requestDetails.length"><td colspan="6" class="empty-cell">No requests recorded yet</td></tr>
          </tbody>
        </table>
      </div>
      <div class="pager" v-if="store.requestDetailsPagination.totalPages > 1">
        <GButton variant="ghost" size="sm" :disabled="!store.requestDetailsPagination.hasPrev" @click="goPage(page - 1)"><ChevronLeft :size="13" /></GButton>
        <span class="page-info">{{ store.requestDetailsPagination.page }} / {{ store.requestDetailsPagination.totalPages }}</span>
        <GButton variant="ghost" size="sm" :disabled="!store.requestDetailsPagination.hasNext" @click="goPage(page + 1)"><ChevronRight :size="13" /></GButton>
      </div>
    </template>

    <!-- Provider limits -->
    <template v-else>
      <div class="limits-list stagger">
        <GCard v-for="pu in store.providerUsage" :key="pu.provider" class="limit-card">
          <div class="limit-top">
            <span class="limit-name">{{ pu.provider }}</span>
            <span class="limit-req">{{ formatNumber(pu.requests) }} req · {{ formatCost(pu.cost) }}</span>
          </div>
          <div v-if="pu.quotaLimit" class="quota-bar-wrap">
            <div class="quota-bar">
              <div class="quota-fill" :class="{ over: pu.overQuota }" :style="{ width: Math.min(100, ((pu.quotaUsed || 0) / pu.quotaLimit) * 100) + '%' }" />
            </div>
            <span class="quota-text">{{ pu.quotaUsed || 0 }} / {{ pu.quotaLimit }}</span>
          </div>
        </GCard>
        <p v-if="!store.providerUsage.length" class="empty-note">No usage data yet.</p>
      </div>
    </template>
  </div>
</template>

<style scoped>
.page-head { display: flex; align-items: flex-start; justify-content: space-between; gap: 16px; margin-bottom: 16px; flex-wrap: wrap; }
.page-title { font-size: 20px; font-weight: 700; letter-spacing: -0.03em; }
.page-desc { font-size: 12.5px; color: var(--text-faint); margin-top: 3px; }
.period-btns { display: flex; gap: 3px; background: var(--glass); border: 1px solid var(--glass-border); border-radius: var(--radius-sm); padding: 3px; }
.period-btn {
  padding: 4px 10px; border: none; border-radius: var(--radius-xs);
  background: transparent; color: var(--text-muted); font-size: 11.5px; font-weight: 560;
  cursor: pointer; transition: all 0.15s ease;
}
.period-btn.active { background: var(--gradient-soft); color: var(--text); }

.tabs { display: flex; gap: 4px; margin-bottom: 18px; border-bottom: 1px solid var(--glass-border); }
.tab {
  padding: 8px 14px; border: none; background: transparent;
  color: var(--text-muted); font-size: 12.5px; font-weight: 560;
  cursor: pointer; border-bottom: 2px solid transparent; margin-bottom: -1px; transition: all 0.15s ease;
}
.tab:hover { color: var(--text); }
.tab.active { color: var(--accent); border-bottom-color: var(--accent); }

.stat-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(160px, 1fr)); gap: 10px; margin-bottom: 16px; }
.stat-card { padding: 16px; text-align: center; }
.stat-val { font-size: 20px; font-weight: 700; letter-spacing: -0.02em; background: var(--gradient); -webkit-background-clip: text; -webkit-text-fill-color: transparent; background-clip: text; }
.stat-label { font-size: 11px; color: var(--text-faint); margin-top: 3px; }

.chart-card { padding: 18px; }
.chart { display: flex; align-items: flex-end; gap: 4px; height: 120px; }
.bar-wrap { flex: 1; display: flex; flex-direction: column; align-items: center; height: 100%; justify-content: flex-end; gap: 4px; }
.bar { width: 100%; max-width: 32px; border-radius: 4px 4px 0 0; background: var(--gradient); opacity: 0.8; min-height: 4px; transition: height 0.3s var(--ease-out-expo); }
.bar-label { font-size: 9px; color: var(--text-faint); white-space: nowrap; }

.table-wrap { overflow-x: auto; }
.table { width: 100%; border-collapse: collapse; font-size: 12px; }
.table th {
  text-align: left; padding: 8px 12px; font-size: 10.5px; text-transform: uppercase;
  letter-spacing: 0.06em; color: var(--text-faint); border-bottom: 1px solid var(--glass-border);
}
.table td { padding: 9px 12px; border-bottom: 1px solid var(--glass-border); }
.table tr:hover td { background: var(--glass-hover); }
.mono { font-family: var(--font-mono); font-size: 11px; }
.td-model { max-width: 200px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.empty-cell { text-align: center; color: var(--text-faint); padding: 24px !important; }
.pager { display: flex; align-items: center; gap: 10px; margin-top: 12px; }
.page-info { font-size: 11.5px; color: var(--text-faint); }

.limits-list { display: flex; flex-direction: column; gap: 8px; max-width: 600px; }
.limit-card { padding: 13px 16px; }
.limit-top { display: flex; justify-content: space-between; align-items: center; margin-bottom: 8px; }
.limit-name { font-size: 12.5px; font-weight: 600; }
.limit-req { font-size: 11px; color: var(--text-faint); }
.quota-bar-wrap { display: flex; align-items: center; gap: 10px; }
.quota-bar { flex: 1; height: 6px; border-radius: 3px; background: var(--glass-hover); overflow: hidden; }
.quota-fill { height: 100%; border-radius: 3px; background: var(--gradient); transition: width 0.3s ease; }
.quota-fill.over { background: var(--red); }
.quota-text { font-size: 10.5px; color: var(--text-faint); font-family: var(--font-mono); white-space: nowrap; }
.empty-note { font-size: 12.5px; color: var(--text-faint); }
</style>
