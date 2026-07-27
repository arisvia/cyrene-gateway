<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, nextTick } from 'vue'
import { useGatewayStore } from '@/stores/gateway'
import { formatNum } from '@/lib/format'
import GCard from '@/components/ui/GCard.vue'
import { Pause, Play, Trash2, ArrowDown } from 'lucide-vue-next'

const store = useGatewayStore()

interface LogLine {
  id: number
  timestamp: string
  provider: string
  model: string
  status: string
  promptTokens: number
  completionTokens: number
  endpoint: string
}

const logs = ref<LogLine[]>([])
const paused = ref(false)
const autoScroll = ref(true)
const connected = ref(false)
const filterLevel = ref<'all' | 'ok' | 'error'>('all')

let eventSource: EventSource | null = null
let logId = 0
const logContainer = ref<HTMLElement | null>(null)

function connectSSE() {
  eventSource = new EventSource('/api/usage/stream')
  eventSource.addEventListener('connected', () => { connected.value = true })
  eventSource.addEventListener('request', (e: MessageEvent) => {
    if (paused.value) return
    try {
      const data = JSON.parse(e.data)
      const line: LogLine = {
        id: ++logId,
        timestamp: data.timestamp || new Date().toISOString(),
        provider: data.provider || '',
        model: data.model || '',
        status: data.status || 'ok',
        promptTokens: data.promptTokens || 0,
        completionTokens: data.completionTokens || 0,
        endpoint: data.endpoint || '',
      }
      logs.value.push(line)
      // Keep max 500 lines
      if (logs.value.length > 500) logs.value.splice(0, logs.value.length - 500)
      if (autoScroll.value) nextTick(() => scrollToBottom())
    } catch { /* ignore parse errors */ }
  })
  eventSource.onerror = () => { connected.value = false }
}

function scrollToBottom() {
  if (logContainer.value) {
    logContainer.value.scrollTop = logContainer.value.scrollHeight
  }
}

function togglePause() {
  paused.value = !paused.value
}

function clearLogs() {
  logs.value = []
}

function fmtTime(ts: string) {
  const d = new Date(ts)
  return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' })
}

const filteredLogs = ref<LogLine[]>([])
const displayLogs = computed(() => {
  if (filterLevel.value === 'all') return logs.value
  return logs.value.filter(l => l.status === filterLevel.value)
})

onMounted(() => {
  connectSSE()
  // Also load recent logs from API as initial data
  store.loadUsageLogs(50).then(() => {
    const initial = store.usageLogs.map((l: any) => ({
      id: ++logId,
      timestamp: l.timestamp || '',
      provider: l.provider || '',
      model: l.model || '',
      status: l.status || 'ok',
      promptTokens: l.promptTokens || 0,
      completionTokens: l.completionTokens || 0,
      endpoint: l.endpoint || '',
    }))
    logs.value = initial.reverse()
    nextTick(() => scrollToBottom())
  })
})

onUnmounted(() => {
  eventSource?.close()
})
</script>

<template>
  <div>
    <div class="page-header">
      <h1 class="page-title">Console Log</h1>
      <p class="page-desc">Live request stream with real-time SSE events.</p>
    </div>

    <GCard class="section-gap console-card">
      <div class="console-toolbar">
        <div class="toolbar-left">
          <span :class="['conn-dot', connected && 'on']"></span>
          <span class="conn-label">{{ connected ? 'Connected' : 'Disconnected' }}</span>
          <span class="log-count">{{ displayLogs.length }} events</span>
        </div>
        <div class="toolbar-right">
          <select v-model="filterLevel" class="level-select">
            <option value="all">All</option>
            <option value="ok">OK</option>
            <option value="error">Error</option>
          </select>
          <button class="tool-btn" @click="togglePause" :title="paused ? 'Resume' : 'Pause'">
            <Play v-if="paused" :size="13" />
            <Pause v-else :size="13" />
          </button>
          <button class="tool-btn" @click="scrollToBottom" title="Scroll to bottom">
            <ArrowDown :size="13" />
          </button>
          <button class="tool-btn" @click="clearLogs" title="Clear">
            <Trash2 :size="13" />
          </button>
        </div>
      </div>

      <div ref="logContainer" class="log-container">
        <div v-for="line in displayLogs" :key="line.id" :class="['log-line', line.status === 'error' && 'err']">
          <span class="log-time">{{ fmtTime(line.timestamp) }}</span>
          <span :class="['log-status', line.status === 'error' ? 'err' : 'ok']">{{ line.status.toUpperCase() }}</span>
          <span class="log-provider">{{ line.provider || '—' }}</span>
          <span class="log-model">{{ line.model || '—' }}</span>
          <span class="log-tokens">{{ formatNum(line.promptTokens + line.completionTokens) }} tok</span>
          <span class="log-endpoint">{{ line.endpoint }}</span>
        </div>
        <div v-if="displayLogs.length === 0" class="log-empty">
          Waiting for events...
        </div>
      </div>
    </GCard>
  </div>
</template>

<style scoped>
.console-card { padding: 0; overflow: hidden; }
.console-toolbar {
  display: flex; align-items: center; justify-content: space-between;
  padding: 10px 14px; border-bottom: 1px solid var(--glass-border);
}
.toolbar-left { display: flex; align-items: center; gap: 8px; }
.toolbar-right { display: flex; align-items: center; gap: 6px; }
.conn-dot { width: 7px; height: 7px; border-radius: 50%; background: var(--text-faint); transition: background 0.2s; }
.conn-dot.on { background: var(--green); box-shadow: 0 0 6px var(--green); }
.conn-label { font-size: 11px; color: var(--text-muted); font-weight: 550; }
.log-count { font-size: 10px; color: var(--text-faint); font-family: var(--font-mono); }

.level-select {
  padding: 3px 8px; border-radius: 5px; font-size: 11px;
  background: var(--glass); border: 1px solid var(--glass-border);
  color: var(--text-muted); font-family: var(--font); cursor: pointer;
}
.tool-btn {
  width: 26px; height: 26px; border-radius: 5px; display: flex; align-items: center; justify-content: center;
  background: transparent; border: 1px solid var(--glass-border); color: var(--text-muted);
  cursor: pointer; transition: all 0.15s;
}
.tool-btn:hover { color: var(--text); background: var(--glass-hover); }

.log-container {
  height: 480px; overflow-y: auto; padding: 8px 0;
  font-family: var(--font-mono); font-size: 11.5px; line-height: 1.7;
}
.log-line {
  display: flex; align-items: center; gap: 10px; padding: 2px 14px;
  transition: background 0.1s;
}
.log-line:hover { background: var(--glass-hover); }
.log-line.err { background: color-mix(in srgb, var(--red, #ef4444) 5%, transparent); }
.log-time { color: var(--text-faint); font-size: 10.5px; flex-shrink: 0; }
.log-status { font-size: 9px; font-weight: 700; padding: 1px 5px; border-radius: 3px; flex-shrink: 0; }
.log-status.ok { color: var(--green); background: color-mix(in srgb, var(--green) 12%, transparent); }
.log-status.err { color: var(--red, #ef4444); background: color-mix(in srgb, var(--red, #ef4444) 12%, transparent); }
.log-provider { color: var(--accent); font-weight: 550; min-width: 80px; }
.log-model { color: var(--text-muted); flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.log-tokens { color: var(--text-faint); flex-shrink: 0; }
.log-endpoint { color: var(--text-faint); font-size: 10px; flex-shrink: 0; opacity: 0.7; }
.log-empty { padding: 40px; text-align: center; color: var(--text-faint); font-size: 12px; }
</style>
