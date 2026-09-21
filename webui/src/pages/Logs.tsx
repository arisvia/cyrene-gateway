import { type Component, For, Show, createSignal, onMount, onCleanup, createMemo } from 'solid-js'
import { api, getStoredSessionToken } from '@/lib/api'
import { Card, Button, Input, Select, PageHeader, StatusPulse } from '@/components/ui'
import { useI18n } from '@/i18n'

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
  const { t } = useI18n()
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
    const token = getStoredSessionToken()
    const streamUrl = token ? `/api/system/logs/stream?token=${encodeURIComponent(token)}` : '/api/system/logs/stream'
    es = new EventSource(streamUrl)
    es.onopen = () => {
      setConnected(true)
    }
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
      case 'ERROR': return 'text-danger font-bold'
      case 'WARN': return 'text-warning font-semibold'
      case 'INFO': return 'text-info'
      case 'DEBUG': return 'text-faint'
      default: return 'text-muted'
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
        title={t('logs.title')}
        badge={<StatusPulse status={connected() ? 'active' : 'idle'} tone={connected() ? 'green' : 'gray'} />}
        subtitle={t('logs.subtitle')}
        actions={
          <>
            <Button
              size="sm"
              variant={autoScroll() ? 'primary' : 'secondary'}
              onClick={() => setAutoScroll(!autoScroll())}
            >
              {autoScroll() ? t('logs.autoScrollOn') : t('logs.autoScrollOff')}
            </Button>
            <Button size="sm" variant="ghost" onClick={clearLogs}>
              {t('logs.clearLogs')}
            </Button>
          </>
        }
      />

      {/* 过滤工具栏 */}
      <Card class="p-3 flex flex-wrap items-center justify-between gap-3 shadow-sm shrink-0">
        <div class="flex flex-wrap items-center gap-3 flex-1">
          <Input
            class="w-full! sm:w-64!"
            placeholder={t('logs.filterPlaceholder')}
            value={query()}
            onInput={setQuery}
          />
          <Select
            value={filterLevel()}
            options={[
              { value: '', label: t('logs.levelAll') },
              { value: 'INFO', label: t('logs.levelInfo') },
              { value: 'WARN', label: t('logs.levelWarn') },
              { value: 'ERROR', label: t('logs.levelError') },
              { value: 'DEBUG', label: t('logs.levelDebug') },
            ]}
            onChange={setFilterLevel}
          />
        </div>
        <div class="text-xs text-faint font-mono">
          {t('logs.logCount', { total: logs().length, matched: filteredLogs().length })}
        </div>
      </Card>

      {/* 实时终端日志视窗 */}
      <div
        ref={scrollContainer}
        class="flex-1 min-h-0 bg-code-bg border border-subtle rounded-2xl p-4 font-mono text-xs overflow-y-auto space-y-1.5 selection:bg-accent/30 shadow-inner"
      >
        <Show
          when={displayedLogs().length > 0}
          fallback={
            <div class="h-full flex items-center justify-center text-faint text-sm">
              {t('logs.noLogs')}
            </div>
          }
        >
          <Show when={filteredLogs().length > RENDER_LIMIT}>
            <div class="text-center py-1 text-[11px] text-faint border-b border-subtle/30 select-none">
              {t('logs.renderedLimit', { limit: RENDER_LIMIT, total: filteredLogs().length })}
            </div>
          </Show>
          <For each={displayedLogs()}>
            {log => (
              <div class="flex items-start gap-2.5 leading-relaxed hover:bg-hover/50 px-1.5 py-0.5 rounded transition-colors break-all">
                <span class="text-faint shrink-0 select-none">{formatTime(log.time)}</span>
                <span class={`px-1.5 py-0.5 rounded text-[10px] shrink-0 uppercase select-none ${levelColor(log.level)}`}>
                  [{log.level}]
                </span>
                <div class="flex-1 min-w-0">
                  <span class="text-foreground">{log.msg}</span>
                  <Show when={log.attrs && Object.keys(log.attrs).length > 0}>
                    <span class="text-muted ml-2">
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
