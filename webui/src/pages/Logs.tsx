import { type Component, For, Show, createSignal, onMount, onCleanup, createMemo } from 'solid-js'
import { api } from '@/lib/api'
import { Card, Button, Input, Select, PageHeader, StatusPulse, Empty } from '@/components/ui'
import { useI18n } from '@/i18n'

interface LogItem {
  time: string
  level: string
  msg: string
  attrs?: Record<string, unknown>
  searchIndex?: string
  // Derived once at ingest: formatting 300 rows with Intl on every flush was the
  // dominant per-frame cost, and the attrs join re-ran on every re-render.
  timeText?: string
  attrsText?: string
}

function formatLogTime(iso: string): string {
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return iso
  return d.toLocaleTimeString('zh-CN', { hour12: false }) + '.' + String(d.getMilliseconds()).padStart(3, '0')
}

function formatAttrs(attrs: Record<string, unknown> | undefined): string {
  if (!attrs) return ''
  const parts: string[] = []
  for (const [k, v] of Object.entries(attrs)) {
    parts.push(`${k}=${typeof v === 'object' && v !== null ? JSON.stringify(v) : v}`)
  }
  return parts.join(' ')
}

function processLogItem(item: LogItem): LogItem {
  const attrsText = formatAttrs(item.attrs)
  return {
    ...item,
    timeText: formatLogTime(item.time),
    attrsText,
    searchIndex: `${item.msg} ${attrsText}`.toLowerCase(),
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
  // Set while a programmatic scroll is in flight so the scroll handler does not
  // mistake it for the user scrolling away from the tail.
  let selfScroll = false

  function atBottom(): boolean {
    if (!scrollContainer) return true
    const { scrollTop, scrollHeight, clientHeight } = scrollContainer
    return scrollHeight - scrollTop - clientHeight < 24
  }

  function scrollToBottom() {
    if (!scrollContainer) return
    selfScroll = true
    scrollContainer.scrollTop = scrollContainer.scrollHeight
    // Reset on the next frame, after the scroll event has fired.
    requestAnimationFrame(() => { selfScroll = false })
  }

  // Following the tail is a property of the scroll position, not a mode the user
  // must remember to toggle. Scrolling up pauses it, returning to the bottom
  // resumes it.
  function onScroll() {
    if (selfScroll) return
    const bottom = atBottom()
    if (bottom !== autoScroll()) setAutoScroll(bottom)
  }

  function flushBuffer() {
    flushTimer = null
    if (pendingBuffer.length === 0) return
    const incoming = pendingBuffer
    pendingBuffer = []
    const stick = autoScroll()
    setLogs(prev => {
      const next = prev.concat(incoming)
      if (next.length > 2000) return next.slice(-2000)
      return next
    })
    if (stick) scrollToBottom()
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
    // Subscribe before loading history: any line emitted between the history read
    // and the stream opening used to be lost entirely. Buffered lines are merged
    // in below and de-duplicated by timestamp+message.
    es = new EventSource('/api/system/logs/stream', { withCredentials: true })
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

    // 1. 载入最近的历史内存日志
    try {
      const res = await api('/api/system/logs') as { logs?: LogItem[] } | null
      if (disposed) return
      const history = (res?.logs || []).map(processLogItem)
      if (history.length > 0) {
        setLogs(prev => {
          const seen = new Set(history.map(l => `${l.time}|${l.level}|${l.msg}`))
          const extra = prev.filter(l => !seen.has(`${l.time}|${l.level}|${l.msg}`))
          return history.concat(extra)
        })
      }
      scrollToBottom()
    } catch (e: unknown) {
      if (disposed) return
      console.warn('[logs] failed to load initial logs:', e)
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

  return (
    <div class="space-y-4 flex flex-col min-h-[calc(100dvh-140px)] lg:h-[calc(100dvh-140px)] stagger">
      <PageHeader
        title={t('logs.title')}
        badge={<StatusPulse status={connected() ? 'active' : 'idle'} tone={connected() ? 'green' : 'gray'} />}
        subtitle={t('logs.subtitle')}
        actions={
          <>
            <Button
              size="sm"
              variant={autoScroll() ? 'primary' : 'secondary'}
              onClick={() => {
                const next = !autoScroll()
                setAutoScroll(next)
                if (next) scrollToBottom()
              }}
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
        <div class="flex flex-wrap items-center gap-3 flex-1 min-w-0">
          <Input
            class="min-w-0 w-full! sm:w-64!"
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
        onScroll={onScroll}
        class="flex-1 min-h-48 max-h-[70dvh] lg:min-h-0 lg:max-h-none bg-surface-inset border border-subtle rounded-2xl p-3 sm:p-4 font-mono text-xs overflow-y-auto space-y-1.5 selection:bg-accent/30"
      >
        <Show
          when={displayedLogs().length > 0}
          fallback={
            <Empty message={t('logs.noLogs')} class="h-full flex items-center justify-center" />
          }
        >
          <Show when={filteredLogs().length > RENDER_LIMIT}>
            <div class="text-center py-2 text-xs text-faint border-b border-subtle/30 select-none">
              {t('logs.renderedLimit', { limit: RENDER_LIMIT, total: filteredLogs().length })}
            </div>
          </Show>
          <For each={displayedLogs()}>
            {log => (
              <div class="grid grid-cols-[auto_1fr] sm:grid-cols-[auto_auto_minmax(0,1fr)] items-start gap-x-2.5 gap-y-1 leading-relaxed hover:bg-hover/50 px-1.5 py-1 rounded transition-colors wrap-anywhere">
                <span class="text-faint shrink-0 select-none">{log.timeText}</span>
                <span class={`px-1.5 py-0.5 rounded text-xs shrink-0 uppercase select-none ${levelColor(log.level)}`}>
                  [{log.level}]
                </span>
                <div class="min-w-0 col-span-2 sm:col-span-1">
                  <span class="text-foreground">{log.msg}</span>
                  <Show when={log.attrsText}>
                    <span class="text-muted ml-2">{log.attrsText}</span>
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
