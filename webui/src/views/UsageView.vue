<script setup lang="ts">
import { ref, onMounted, computed, onUnmounted } from 'vue'
import { useGatewayStore } from '@/stores/gateway'
import { formatNum } from '@/lib/format'
import GCard from '@/components/ui/GCard.vue'
import { Radio, ChevronLeft, ChevronRight } from 'lucide-vue-next'

const store = useGatewayStore()
const period = ref('7d')
const activeTab = ref<'overview' | 'details' | 'providers'>('overview')

// SSE real-time
const liveCount = ref(0)
const liveConnected = ref(false)
let eventSource: EventSource | null = null

function connectSSE() {
  eventSource = new EventSource('/api/usage/stream')
  eventSource.addEventListener('connected', () => { liveConnected.value = true })
  eventSource.addEventListener('request', () => { liveCount.value++ })
  eventSource.onerror = () => { liveConnected.value = false }
}

onMounted(() => {
  store.loadUsage(period.value)
  connectSSE()
})

onUnmounted(() => {
  eventSource?.close()
})

function setPeriod(p: string) {
  period.value = p
  store.loadUsage(p)
  if (activeTab.value === 'providers') store.loadProviderUsage(p)
}

function switchTab(tab: 'overview' | 'details' | 'providers') {
  activeTab.value = tab
  if (tab === 'details') store.loadRequestDetails(1, 20)
  if (tab === 'providers') store.loadProviderUsage(period.value)
}

// Request Details tab
const detailPage = ref(1)
const detailFilters = ref({ provider: '', model: '', status: '' })

function loadDetails(page: number) {
  detailPage.value = page
  store.loadRequestDetails(page, 20, detailFilters.value)
}

function applyFilters() {
  loadDetails(1)
}

// Chart
const maxTokens = computed(() => Math.max(...store.usageChart.map(b => b.tokens || 0), 1))
function barHeight(tokens: number) {
  return Math.max(2, Math.round((tokens / maxTokens.value) * 100))
}

function fmtTime(ts?: string) {
  if (!ts) return '—'
  const d = new Date(ts)
  return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' })
}

function fmtDate(ts?: string) {
  if (!ts) return '—'
  const d = new Date(ts)
  return d.toLocaleDateString([], { month: 'short', day: 'numeric' }) + ' ' + d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
}
</script>

<template>
  <div>
    <div class="page-header">
      <div class="page-header-row">
        <div>
          <h1 class="page-title">Usage</h1>
          <p class="page-desc">Token consumption, costs, and request analytics.</p>
        </div>
        <div class="live-indicator" :class="{ connected: liveConnected }">
          <Radio :size="13" />
          <span>{{ liveConnected ? 'Live' : 'Offline' }}</span>
          <span v-if="liveCount > 0" class="live-count">+{{ liveCount }}</span>
        </div>
      </div>
    </div>

    <!-- Tabs -->
    <div class="tab-bar section-gap">
      <button :class="['tab-btn', activeTab === 'overview' && 'active']" @click="switchTab('overview')">Overview</button>
      <button :class="['tab-btn', activeTab === 'details' && 'active']" @click="switchTab('details')">Request Details</button>
      <button :class="['tab-btn', activeTab === 'providers' && 'active']" @click="switchTab('providers')">Provider Limits</button>
    </div>

    <!-- Overview Tab -->
    <template v-if="activeTab === 'overview'">
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
          <div v-for="d in store.requestDetails.slice(0, 8)" :key="d.id || d.timestamp" class="kv-row">
            <span class="mono text-xs truncate" style="max-width:55%">{{ d.model || d.provider }}</span>
            <span class="text-xs text-muted">{{ d.status || 'ok' }} · {{ formatNum((d.promptTokens || 0) + (d.completionTokens || 0)) }} tok</span>
          </div>
          <p v-if="store.requestDetails.length === 0" class="text-xs text-faint">No requests recorded yet.</p>
        </GCard>
      </div>
    </template>

    <!-- Request Details Tab -->
    <template v-if="activeTab === 'details'">
      <GCard pad class="section-gap">
        <div class="filter-row">
          <input v-model="detailFilters.provider" placeholder="Provider" class="filter-input" @keyup.enter="applyFilters" />
          <input v-model="detailFilters.model" placeholder="Model" class="filter-input" @keyup.enter="applyFilters" />
          <select v-model="detailFilters.status" class="filter-input" @change="applyFilters">
            <option value="">All Status</option>
            <option value="ok">OK</option>
            <option value="error">Error</option>
          </select>
          <button class="btn-sm" @click="applyFilters">Filter</button>
        </div>

        <div class="table-wrap">
          <table class="detail-table">
            <thead>
              <tr>
                <th>Time</th>
                <th>Provider</th>
                <th>Model</th>
                <th>Tokens</th>
                <th>Status</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="d in store.requestDetails" :key="d.id || d.timestamp">
                <td class="mono text-xs">{{ fmtDate(d.timestamp) }}</td>
                <td class="text-xs">{{ d.provider || '—' }}</td>
                <td class="mono text-xs truncate" style="max-width:180px">{{ d.model || '—' }}</td>
                <td class="text-xs">{{ formatNum((d.promptTokens || 0) + (d.completionTokens || 0)) }}</td>
                <td><span :class="['status-badge', d.status === 'error' ? 'err' : 'ok']">{{ d.status || 'ok' }}</span></td>
              </tr>
            </tbody>
          </table>
          <p v-if="store.requestDetails.length === 0" class="text-xs text-faint" style="padding:16px 0">No request details found.</p>
        </div>

        <div class="pagination-row">
          <button class="page-btn" :disabled="!store.requestDetailsPagination.hasPrev" @click="loadDetails(detailPage - 1)">
            <ChevronLeft :size="14" /> Prev
          </button>
          <span class="page-info">{{ store.requestDetailsPagination.page }} / {{ store.requestDetailsPagination.totalPages || 1 }}</span>
          <button class="page-btn" :disabled="!store.requestDetailsPagination.hasNext" @click="loadDetails(detailPage + 1)">
            Next <ChevronRight :size="14" />
          </button>
        </div>
      </GCard>
    </template>

    <!-- Provider Limits Tab -->
    <template v-if="activeTab === 'providers'">
      <div class="section-gap">
        <div class="period-tabs">
          <button v-for="p in ['today','7d','30d','60d']" :key="p"
            :class="['period-tab', period === p && 'active']"
            @click="setPeriod(p)">{{ p === 'today' ? 'Today' : p.toUpperCase() }}</button>
        </div>
      </div>

      <div class="provider-cards section-gap">
        <GCard v-for="p in store.providerUsage" :key="p.provider" pad class="provider-card">
          <div class="provider-card-header">
            <span class="provider-name">{{ p.provider }}</span>
            <span v-if="p.overQuota" class="status-badge err">Over Quota</span>
            <span v-else class="status-badge ok">{{ p.activeConnections }}/{{ p.connections }} active</span>
          </div>
          <div class="provider-stats">
            <div class="pstat"><span class="pstat-val">{{ formatNum(p.requests) }}</span><span class="pstat-label">requests</span></div>
            <div class="pstat"><span class="pstat-val">{{ formatNum(p.promptTokens + p.completionTokens) }}</span><span class="pstat-label">tokens</span></div>
            <div class="pstat"><span class="pstat-val">${{ p.cost.toFixed(4) }}</span><span class="pstat-label">cost</span></div>
          </div>
          <div v-if="p.quotaLimit" class="quota-bar-wrap">
            <div class="quota-bar">
              <div class="quota-fill" :class="{ over: p.overQuota }" :style="{ width: Math.min(100, (p.quotaUsed || 0) / p.quotaLimit * 100) + '%' }"></div>
            </div>
            <span class="quota-text">{{ p.quotaUsed || 0 }} / {{ p.quotaLimit }}</span>
          </div>
        </GCard>
        <p v-if="store.providerUsage.length === 0" class="text-xs text-faint">No provider data available.</p>
      </div>
    </template>
  </div>
</template>

<style scoped>
.page-header-row { display: flex; align-items: flex-start; justify-content: space-between; }
.live-indicator {
  display: flex; align-items: center; gap: 5px; padding: 4px 10px;
  border-radius: 20px; font-size: 11px; font-weight: 550;
  background: var(--glass); border: 1px solid var(--glass-border);
  color: var(--text-faint); transition: all 0.2s ease;
}
.live-indicator.connected { color: var(--green); border-color: color-mix(in srgb, var(--green) 30%, transparent); }
.live-count { font-family: var(--font-mono); font-size: 10px; opacity: 0.8; }

.tab-bar { display: flex; gap: 2px; padding: 3px; border-radius: 8px; background: var(--glass); border: 1px solid var(--glass-border); width: fit-content; }
.tab-btn {
  padding: 6px 14px; border-radius: 6px; font-size: 12px; font-weight: 550;
  color: var(--text-faint); cursor: pointer; border: 1px solid transparent; background: none;
  font-family: var(--font); transition: all 0.15s ease;
}
.tab-btn:hover { color: var(--text-muted); }
.tab-btn.active { background: var(--glass-hover); color: var(--text); border-color: var(--glass-border); box-shadow: 0 1px 4px rgba(0,0,0,0.12); }

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

/* Request Details */
.filter-row { display: flex; gap: 8px; margin-bottom: 14px; flex-wrap: wrap; }
.filter-input {
  padding: 6px 10px; border-radius: 6px; font-size: 12px;
  background: var(--input-bg, var(--glass)); border: 1px solid var(--glass-border);
  color: var(--text); font-family: var(--font); min-width: 120px;
}
.filter-input:focus { outline: none; border-color: var(--accent); }
.btn-sm {
  padding: 6px 12px; border-radius: 6px; font-size: 12px; font-weight: 550;
  background: var(--accent); color: #fff; border: none; cursor: pointer;
  font-family: var(--font); transition: opacity 0.15s;
}
.btn-sm:hover { opacity: 0.85; }

.table-wrap { overflow-x: auto; }
.detail-table { width: 100%; border-collapse: collapse; font-size: 12px; }
.detail-table th {
  text-align: left; padding: 8px 10px; font-size: 10px; font-weight: 600;
  text-transform: uppercase; letter-spacing: 0.05em; color: var(--text-faint);
  border-bottom: 1px solid var(--glass-border);
}
.detail-table td { padding: 8px 10px; border-bottom: 1px solid var(--row-divider); color: var(--text-muted); }
.detail-table tr:hover td { background: var(--glass-hover); }

.status-badge {
  display: inline-block; padding: 2px 7px; border-radius: 4px; font-size: 10px; font-weight: 600;
}
.status-badge.ok { background: color-mix(in srgb, var(--green) 15%, transparent); color: var(--green); }
.status-badge.err { background: color-mix(in srgb, var(--red, #ef4444) 15%, transparent); color: var(--red, #ef4444); }

.pagination-row { display: flex; align-items: center; justify-content: center; gap: 12px; margin-top: 14px; }
.page-btn {
  display: flex; align-items: center; gap: 4px; padding: 5px 10px;
  border-radius: 6px; font-size: 11px; font-weight: 550; cursor: pointer;
  background: var(--glass); border: 1px solid var(--glass-border); color: var(--text-muted);
  font-family: var(--font); transition: all 0.15s;
}
.page-btn:hover:not(:disabled) { color: var(--text); border-color: var(--accent); }
.page-btn:disabled { opacity: 0.4; cursor: not-allowed; }
.page-info { font-size: 11px; color: var(--text-faint); font-family: var(--font-mono); }

/* Provider Limits */
.provider-cards { display: grid; grid-template-columns: repeat(auto-fill, minmax(280px, 1fr)); gap: 12px; }
.provider-card-header { display: flex; align-items: center; justify-content: space-between; margin-bottom: 12px; }
.provider-name { font-size: 13px; font-weight: 600; }
.provider-stats { display: flex; gap: 16px; margin-bottom: 10px; }
.pstat { display: flex; flex-direction: column; gap: 2px; }
.pstat-val { font-size: 14px; font-weight: 650; font-family: var(--font-mono); }
.pstat-label { font-size: 9.5px; color: var(--text-faint); text-transform: uppercase; letter-spacing: 0.04em; }
.quota-bar-wrap { display: flex; align-items: center; gap: 8px; }
.quota-bar { flex: 1; height: 6px; border-radius: 3px; background: var(--glass-hover); overflow: hidden; }
.quota-fill { height: 100%; border-radius: 3px; background: var(--accent); transition: width 0.3s ease; }
.quota-fill.over { background: var(--red, #ef4444); }
.quota-text { font-size: 10px; color: var(--text-faint); font-family: var(--font-mono); white-space: nowrap; }
</style>
