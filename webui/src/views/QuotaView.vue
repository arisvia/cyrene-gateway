<script setup lang="ts">
import { onMounted } from 'vue'
import { useGatewayStore } from '@/stores/gateway'
import { formatNum } from '@/lib/format'
import GCard from '@/components/ui/GCard.vue'
import GBadge from '@/components/ui/GBadge.vue'

const store = useGatewayStore()
onMounted(() => store.loadQuota())
</script>

<template>
  <div>
    <div class="page-header">
      <h1 class="page-title">Quota Tracker</h1>
      <p class="page-desc">Per-connection quota limits and usage monitoring.</p>
    </div>

    <GCard pad>
      <p class="card-section-title">Connection Quotas</p>
      <p v-if="store.quotaEntries.length === 0" style="color:var(--text-muted);font-size:13px;padding:12px 0">
        No connections have quota limits configured. Set quotaLimit on a connection to enable tracking.
      </p>
      <table v-else class="table">
        <thead><tr><th>Provider</th><th>Name</th><th>Period</th><th>Used / Limit</th><th>Tokens</th><th>Status</th></tr></thead>
        <tbody>
          <tr v-for="q in store.quotaEntries" :key="q.connectionId">
            <td>{{ q.provider }}</td>
            <td>{{ q.name || q.connectionId.slice(0,8) }}</td>
            <td>{{ q.quotaPeriod }}</td>
            <td>
              <div class="quota-cell">
                <div class="quota-bar">
                  <div class="quota-fill" :class="{ over: q.overQuota }"
                    :style="{ width: Math.min(100, (q.usedRequests / q.quotaLimit) * 100) + '%' }"></div>
                </div>
                <span style="font-size:12px;white-space:nowrap">{{ q.usedRequests }} / {{ q.quotaLimit }}</span>
              </div>
            </td>
            <td style="font-size:12px">{{ formatNum(q.usedPromptTokens + q.usedCompTokens) }}</td>
            <td><GBadge :color="q.overQuota ? 'red' : 'green'">{{ q.overQuota ? 'Over Quota' : 'OK' }}</GBadge></td>
          </tr>
        </tbody>
      </table>
    </GCard>
  </div>
</template>

<style scoped>
.card-section-title { font-size: 11px; font-weight: 600; text-transform: uppercase; letter-spacing: 0.06em; color: var(--text-faint); margin-bottom: 12px; }
.table { width: 100%; border-collapse: collapse; }
.table th {
  padding: 10px 14px; text-align: left; font-size: 10.5px; font-weight: 600;
  text-transform: uppercase; letter-spacing: 0.05em; color: var(--text-faint);
  border-bottom: 1px solid var(--glass-border);
}
.table td { padding: 10px 14px; font-size: 12.5px; border-bottom: 1px solid var(--row-divider); }
.table tr:last-child td { border-bottom: none; }
.table tbody tr { transition: background 0.1s; }
.table tbody tr:hover { background: var(--glass); }
.quota-cell { display: flex; align-items: center; gap: 8px; }
.quota-bar { flex: 1; height: 6px; border-radius: 3px; background: var(--glass); overflow: hidden; min-width: 60px; }
.quota-fill { height: 100%; border-radius: 3px; background: var(--accent); transition: width 0.3s ease; }
.quota-fill.over { background: var(--red); }
</style>
