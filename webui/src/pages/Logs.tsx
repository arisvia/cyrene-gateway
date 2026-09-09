import { type Component, For, Show, createSignal, onMount, onCleanup, createMemo } from 'solid-js'
import { api } from '@/lib/api'
import { Card, Button, Input, Select, PageHeader } from '@/components/ui'

interface LogItem {
  time: string
  level: string
  msg: string
  attrs?: Record<string, unknown>
  searchIndex?: string
}

function processLogItem(item: LogItem): LogItem {
  return {
    ...item,
    searchIndex: `${item.msg} ${item.attrs ? JSON.stringify(item.attrs) : ''}`.toLowerCase(),
  }
}

const LogsPage: Component = () => {
  const [logs, setLogs] = createSignal<LogItem[]>([])
  const [filterLevel, setFilterLevel] = createSignal('')
  const [query, setQuery] = createSignal('')
  const [autoScroll, setAutoScroll] = createSignal(true)
  const [connected, setConnected] = createSignal(false)
  let scrollContainer: HTMLDivElement | undefined

  let es: EventSource | null = null
  let pendingBuffer: LogItem[] = []
  let flushTimer: number | null = null

  function scrollToBottom() {
    if (autoScroll() && scrollContainer) {
      scrollContainer.scrollTop = scrollContainer.scrollHeight
    }
  }

  function flushBuffer() {
    flushTimer = null
    if (pendingBuffer.length === 0) return
    const incoming = pendingBuffer
    pendingBuffer = []
    setLogs(prev => {
      const next = prev.concat(incoming)
      if (next.length > 2000) return next.slice(-2000)
      return next
    })
    scrollToBottom()
  }

  function scheduleFlush() {
    if (flushTimer === null) {
      flushTimer = window.setTimeout(flushBuffer, 100)
    }
  }

  function clearLogs() {
    pendingBuffer = []
    if (flushTimer !== null) {
      clearTimeout(flushTimer)
      flushTimer = null
    }
    setLogs([])
  }

  let disposed = false
  onMount(async () => {
    // 1. 先载入最近的历史内存日志
    try {
      const res = await api('/api/system/logs') as { logs?: LogItem[] } | null
      if (disposed) return
      if (res?.logs) {
        setLogs(res.logs.map(processLogItem))
        scrollToBottom()
      }
    } catch (e: unknown) {
      if (disposed) return
      console.warn('[logs] failed to load initial logs:', e)
    }

    if (disposed) return
    // 2. 建立 SSE 实时流
    const streamUrl = '/api/system/logs/stream'
    es = new EventSource(streamUrl)
    es.addEventListener('connected', () => {
      setConnected(true)
    })

    es.addEventListener('log', e => {
      try {
        const item = processLogItem(JSON.parse(e.data) as LogItem)
        pendingBuffer.push(item)
        scheduleFlush()
      } catch (err: unknown) {
        console.error('[logs] parse error:', err)
      }
    })

    es.onerror = () => {
      setConnected(false)
    }
  })

  onCleanup(() => {
    disposed = true
    pendingBuffer = []
    if (flushTimer !== null) {
      clearTimeout(flushTimer)
      flushTimer = null
    }
    if (es) {
      es.close()
      es = null
    }
  })

  const filteredLogs = createMemo(() => {
    const q = query().toLowerCase().trim()
    const lvl = filterLevel()
    return logs().filter(l => {
      if (lvl && l.level !== lvl) return false
      if (!q) return true
      return (l.searchIndex || l.msg.toLowerCase()).includes(q)
    })
  })

  const RENDER_LIMIT = 300
  const displayedLogs = createMemo(() => {
    const list = filteredLogs()
    if (list.length <= RENDER_LIMIT) return list
    return list.slice(-RENDER_LIMIT)
  })

  const levelColor = (level: string) => {
    switch (level.toUpperCase()) {
      case 'ERROR': return 'text-red-400 font-bold'
      case 'WARN': return 'text-yellow-400 font-semibold'
      case 'INFO': return 'text-cyan-400'
      case 'DEBUG': return 'text-zinc-500'
      default: return 'text-zinc-400'
    }
  }

  const formatTime = (iso: string) => {
    try {
      const d = new Date(iso)
      return d.toLocaleTimeString('zh-CN', { hour12: false }) + '.' + String(d.getMilliseconds()).padStart(3, '0')
    } catch {
      return iso
    }
  }

  return (
    <div class="space-y-4 flex flex-col h-[calc(100vh-140px)] stagger">
      <PageHeader
        title="网关系统日志"
        badge={<span class={`inline-block w-2.5 h-2.5 rounded-full ${connected() ? 'bg-emerald-500 shadow-emerald-500/50 shadow-sm animate-pulse' : 'bg-zinc-600'}`} />}
        subtitle="实时捕获与推送 Cyrene Gateway 后端请求转发、上游故障重试与轮转事件"
        actions={
          <>
            <Button
              size="sm"
              variant={autoScroll() ? 'primary' : 'secondary'}
              onClick={() => setAutoScroll(!autoScroll())}
            >
              {autoScroll() ? '自动滚动: 开' : '自动滚动: 关'}
            </Button>
            <Button size="sm" variant="ghost" onClick={clearLogs}>
              清屏
            </Button>
          </>
        }
      />

      {/* 过滤工具栏 */}
      <Card class="p-3 flex flex-wrap items-center justify-between gap-3 shadow-sm shrink-0">
        <div class="flex flex-wrap items-center gap-3 flex-1">
          <Input
            class="!w-full sm:!w-64"
            placeholder="过滤日志内容 / 参数 / 路径…"
            value={query()}
            onInput={setQuery}
          />
          <Select
            value={filterLevel()}
            options={[
              { value: '', label: '全部日志级别' },
              { value: 'INFO', label: 'INFO (正常)' },
              { value: 'WARN', label: 'WARN (告警/重试)' },
              { value: 'ERROR', label: 'ERROR (错误)' },
              { value: 'DEBUG', label: 'DEBUG (调试)' },
            ]}
            onChange={setFilterLevel}
          />
        </div>
        <div class="text-xs text-faint font-mono">
          共 {logs().length} 条，匹配 {filteredLogs().length} 条
          <Show when={filteredLogs().length > RENDER_LIMIT}>
            （显示最新 {displayedLogs().length} 条）
          </Show>
        </div>
      </Card>

      {/* 实时终端日志视窗 */}
      <div
        ref={scrollContainer}
        class="flex-1 min-h-0 bg-[#0d1117] border border-subtle rounded-2xl p-4 font-mono text-xs overflow-y-auto space-y-1.5 selection:bg-accent/30 shadow-inner"
      >
        <Show
          when={displayedLogs().length > 0}
          fallback={
            <div class="h-full flex items-center justify-center text-zinc-600 text-sm">
              暂无匹配的系统运行日志…
            </div>
          }
        >
          <Show when={filteredLogs().length > RENDER_LIMIT}>
            <div class="text-center py-1 text-[11px] text-zinc-500 border-b border-subtle/30 select-none">
              为保障流畅渲染，已仅展示最新 {RENDER_LIMIT} 条匹配日志（共 {filteredLogs().length} 条）
            </div>
          </Show>
          <For each={displayedLogs()}>
            {log => (
              <div class="flex items-start gap-2.5 leading-relaxed hover:bg-white/2 px-1.5 py-0.5 rounded transition-colors break-all">
                <span class="text-zinc-500 shrink-0 select-none">{formatTime(log.time)}</span>
                <span class={`px-1.5 py-0.5 rounded text-[10px] shrink-0 uppercase select-none ${levelColor(log.level)}`}>
                  [{log.level}]
                </span>
                <div class="flex-1 min-w-0">
                  <span class="text-zinc-200">{log.msg}</span>
                  <Show when={log.attrs && Object.keys(log.attrs).length > 0}>
                    <span class="text-zinc-400 ml-2">
                      {Object.entries(log.attrs!).map(([k, v]) => `${k}=${typeof v === 'object' ? JSON.stringify(v) : v}`).join(' ')}
                    </span>
                  </Show>
                </div>
              </div>
            )}
          </For>
        </Show>
      </div>
    </div>
  )
}

export default LogsPage
