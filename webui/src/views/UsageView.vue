<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { useGatewayStore } from '@/stores/gateway'
import { formatNum } from '@/lib/format'
import GCard from '@/components/ui/GCard.vue'

const store = useGatewayStore()
const period = ref('7d')

function setPeriod(p: string) {
  period.value = p
  store.loadUsage(p)
}

const maxTokens = computed(() => Math.max(...store.usageChart.map(b => b.tokens || 0), 1))
function barHeight(tokens: number) {
  return Math.max(2, Math.round((tokens / maxTokens.value) * 100))
}

onMounted(() => store.loadUsage(period.value))
</script>

<template>
  <div>
    <div class="page-header">
      <h1 class="page-title">Usage</h1>
      <p class="page-desc">Token consumption, costs, and request analytics.</p>
    </div>

    <div class="section-gap">
      <div class="period-tabs">
        <button v-for="p in ['today','7d','30d','60d']" :key="p"
          :class="['period-tab', period === p && 'active']"
          @click="setPeriod(p)">{{ p === 'today' ? 'Today' : p.toUpperCase() }}</button>
      </div>
    </div>

    <div class="stat-grid section-gap">
      <GCard pad class="stat-card">
        <p class="stat-label">Requests</p>
        <p class="stat-value teal">{{ formatNum(store.usageStats.totalRequests) }}</p>
      </GCard>
      <GCard pad class="stat-card">
        <p class="stat-label">Prompt Tokens</p>
        <p class="stat-value">{{ formatNum(store.usageStats.totalPromptTokens) }}</p>
      </GCard>
      <GCard pad class="stat-card">
        <p class="stat-label">Completion Tokens</p>
        <p class="stat-value">{{ formatNum(store.usageStats.totalCompletionTokens) }}</p>
      </GCard>
      <GCard pad class="stat-card">
        <p class="stat-label">Est. Cost</p>
        <p class="stat-value">${{ store.usageStats.totalCost ? store.usageStats.totalCost.toFixed(4) : '0.00' }}</p>
      </GCard>
    </div>

    <GCard pad class="section-gap">
      <p class="card-section-title">Token volume · {{ period === 'today' ? 'hourly' : 'daily' }}</p>
      <div class="chart-area">
        <div v-for="(b, i) in store.usageChart" :key="i" class="chart-bar-wrap">
          <div class="chart-bar" :style="{ height: barHeight(b.tokens) + '%' }"
            :title="b.label + ' — ' + formatNum(b.tokens) + ' tokens'"></div>
          <span class="chart-label">{{ b.label }}</span>
        </div>
      </div>
    </GCard>

    <div class="grid-2 section-gap">
      <GCard pad>
        <p class="card-section-title">By Provider</p>
        <div v-for="(v, k) in store.usageStats.byProvider" :key="k" class="kv-row">
          <span class="mono text-xs">{{ k }}</span>
          <span class="text-xs text-muted">{{ formatNum(v.requests) }} req · <b style="color:var(--text)">{{ formatNum(v.promptTokens + v.completionTokens) }}</b> tok</span>
        </div>
        <p v-if="!store.usageStats.byProvider || Object.keys(store.usageStats.byProvider).length === 0" class="text-xs text-faint">No data in this period.</p>
      </GCard>
      <GCard pad>
        <p class="card-section-title">Recent Requests</p>
        <div v-for="d in store.requestDetails.slice(0, 8)" :key="d.id" class="kv-row">
          <span class="mono text-xs truncate" style="max-width:55%">{{ d.model || d.provider }}</span>
          <span class="text-xs text-muted">{{ d.status || 'ok' }} · {{ formatNum((d.promptTokens || 0) + (d.completionTokens || 0)) }} tok</span>
        </div>
        <p v-if="store.requestDetails.length === 0" class="text-xs text-faint">No requests recorded yet.</p>
      </GCard>
    </div>
  </div>
</template>

<style scoped>
.period-tabs { display: inline-flex; gap: 2px; padding: 3px; border-radius: 8px; background: var(--glass); border: 1px solid var(--glass-border); }
.period-tab {
  padding: 5px 12px; border-radius: 6px; font-size: 11.5px; font-weight: 550;
  color: var(--text-faint); cursor: pointer; border: 1px solid transparent; background: none;
  font-family: var(--font); transition: all 0.15s ease;
}
.period-tab:hover { color: var(--text-muted); }
.period-tab.active { background: var(--glass-hover); color: var(--text); border-color: var(--glass-border); box-shadow: 0 1px 4px rgba(0,0,0,0.12); }
.stat-grid { display: grid; grid-template-columns: repeat(4, 1fr); gap: 12px; }
@media (max-width: 800px) { .stat-grid { grid-template-columns: repeat(2, 1fr); } }
.stat-label { font-size: 11px; color: var(--text-muted); font-weight: 500; }
.stat-value { font-size: 22px; font-weight: 700; letter-spacing: -0.03em; margin-top: 4px; }
.stat-value.teal { background: var(--gradient); -webkit-background-clip: text; -webkit-text-fill-color: transparent; background-clip: text; }
.card-section-title { font-size: 11px; font-weight: 600; text-transform: uppercase; letter-spacing: 0.06em; color: var(--text-faint); margin-bottom: 12px; }
.chart-area { display: flex; align-items: flex-end; gap: 3px; height: 140px; padding-top: 8px; }
.chart-bar-wrap { flex: 1; display: flex; flex-direction: column; align-items: center; gap: 6px; height: 100%; justify-content: flex-end; }
.chart-bar {
  width: 100%; border-radius: 3px 3px 1px 1px; min-height: 2px;
  background: linear-gradient(180deg, var(--chart-a), var(--chart-b));
  transition: all 0.2s ease;
}
.chart-bar-wrap:hover .chart-bar { background: linear-gradient(180deg, var(--chart-a-hover), var(--chart-b-hover)); box-shadow: 0 0 12px var(--ring-soft); }
.chart-label { font-size: 8.5px; color: var(--text-faint); width: 100%; text-align: center; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.kv-row { display: flex; align-items: center; justify-content: space-between; padding: 9px 0; font-size: 13px; }
.kv-row + .kv-row { border-top: 1px solid var(--row-divider); }
</style>
