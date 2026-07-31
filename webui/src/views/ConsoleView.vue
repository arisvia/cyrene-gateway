<script setup lang="ts">
import { ref, onMounted, onBeforeUnmount, nextTick } from 'vue'

interface LogLine { time: string; level: string; msg: string; raw: string }

const lines = ref<LogLine[]>([])
const connected = ref(false)
const autoScroll = ref(true)
const logBox = ref<HTMLElement | null>(null)
let es: EventSource | null = null

onMounted(() => {
  es = new EventSource('/api/usage/stream')
  es.onopen = () => { connected.value = true }
  es.onmessage = (ev) => {
    try {
      const d = JSON.parse(ev.data)
      lines.value.push({
        time: d.time || d.timestamp || new Date().toISOString(),
        level: d.level || 'info',
        msg: d.msg || d.message || ev.data,
        raw: ev.data,
      })
      if (lines.value.length > 500) lines.value = lines.value.slice(-400)
      if (autoScroll.value) nextTick(() => { if (logBox.value) logBox.value.scrollTop = logBox.value.scrollHeight })
    } catch {
      lines.value.push({ time: new Date().toISOString(), level: 'info', msg: ev.data, raw: ev.data })
    }
  }
  es.onerror = () => { connected.value = false }
})

onBeforeUnmount(() => { es?.close() })

function clear() { lines.value = [] }

function levelColor(l: string) {
  if (l === 'ERROR' || l === 'error') return 'var(--red)'
  if (l === 'WARN' || l === 'warn') return 'var(--amber)'
  return 'var(--text-faint)'
}
</script>

<template>
  <div class="page">
    <header class="page-head">
      <div>
        <h1 class="page-title">Console Log</h1>
        <p class="page-desc">
          Live gateway event stream
          <span class="conn-badge" :class="{ ok: connected }">{{ connected ? 'connected' : 'disconnected' }}</span>
        </p>
      </div>
      <div class="head-actions">
        <label class="autoscroll"><input type="checkbox" v-model="autoScroll"> Auto-scroll</label>
        <button class="clear-btn" @click="clear">Clear</button>
      </div>
    </header>

    <div class="log-box" ref="logBox">
      <p v-if="!lines.length" class="log-empty">Waiting for events…</p>
      <div v-for="(l, i) in lines" :key="i" class="log-line">
        <span class="log-time">{{ l.time.slice(11, 19) }}</span>
        <span class="log-level" :style="{ color: levelColor(l.level) }">{{ l.level.toUpperCase().padEnd(5) }}</span>
        <span class="log-msg">{{ l.msg }}</span>
      </div>
    </div>
  </div>
</template>

<style scoped>
.page-head { display: flex; align-items: flex-start; justify-content: space-between; gap: 16px; margin-bottom: 16px; flex-wrap: wrap; }
.page-title { font-size: 20px; font-weight: 700; letter-spacing: -0.03em; }
.page-desc { font-size: 12.5px; color: var(--text-faint); margin-top: 3px; display: flex; align-items: center; gap: 8px; }
.conn-badge {
  font-size: 10px; font-weight: 600; padding: 2px 8px; border-radius: 99px;
  background: rgba(248,113,113,0.1); color: var(--red); border: 1px solid rgba(248,113,113,0.2);
}
.conn-badge.ok { background: rgba(52,211,153,0.1); color: var(--green); border-color: rgba(52,211,153,0.2); }
.head-actions { display: flex; align-items: center; gap: 12px; }
.autoscroll { font-size: 11.5px; color: var(--text-muted); display: flex; align-items: center; gap: 5px; cursor: pointer; }
.clear-btn {
  padding: 5px 12px; border-radius: var(--radius-sm);
  background: var(--glass); border: 1px solid var(--glass-border);
  color: var(--text-muted); font-size: 11.5px; cursor: pointer; transition: all 0.15s ease;
}
.clear-btn:hover { color: var(--text); border-color: var(--glass-border-hover); }

.log-box {
  background: var(--code-bg); border: 1px solid var(--glass-border);
  border-radius: var(--radius); padding: 14px 16px;
  height: calc(100vh - 220px); min-height: 300px; overflow-y: auto;
  font-family: var(--font-mono); font-size: 11.5px; line-height: 1.8;
}
.log-empty { color: var(--text-faint); }
.log-line { display: flex; gap: 10px; white-space: pre-wrap; word-break: break-all; }
.log-time { color: var(--text-faint); flex-shrink: 0; }
.log-level { flex-shrink: 0; font-weight: 600; }
.log-msg { color: var(--text-muted); }
</style>
