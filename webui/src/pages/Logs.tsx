import { type Component, For, Show, createSignal, onMount, onCleanup, createMemo } from 'solid-js'
import { api } from '@/lib/api'
import { Card, Badge, Button, Input, Select } from '@/components/ui'

interface LogItem {
  time: string
  level: string
  msg: string
  attrs?: Record<string, unknown>
}

const LogsPage: Component = () => {
  const [logs, setLogs] = createSignal<LogItem[]>([])
  const [filterLevel, setFilterLevel] = createSignal('')
  const [query, setQuery] = createSignal('')
  const [autoScroll, setAutoScroll] = createSignal(true)
  const [connected, setConnected] = createSignal(false)
  let scrollContainer: HTMLDivElement | undefined

  let es: EventSource | null = null

  function scrollToBottom() {
    if (autoScroll() && scrollContainer) {
      scrollContainer.scrollTop = scrollContainer.scrollHeight
    }
  }

  onMount(async () => {
    // 1. 先载入最近的历史内存日志
    try {
      const res = await api('/api/system/logs') as { logs?: LogItem[] } | null
      if (res?.logs) {
        setLogs(res.logs)
        scrollToBottom()
      }
    } catch (e: unknown) {
      console.warn('[logs] failed to load initial logs:', e)
    }

    // 2. 建立 SSE 实时流
    const streamUrl = '/api/system/logs/stream'
    es = new EventSource(streamUrl)

    es.addEventListener('connected', () => {
      setConnected(true)
    })

    es.addEventListener('log', e => {
      try {
        const item = JSON.parse(e.data) as LogItem
        setLogs(prev => {
          const next = [...prev, item]
          if (next.length > 2000) return next.slice(-2000)
          return next
        })
        scrollToBottom()
      } catch (err: unknown) {
        console.error('[logs] parse error:', err)
      }
    })

    es.onerror = () => {
      setConnected(false)
    }
  })

  onCleanup(() => {
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
      const fullText = `${l.msg} ${JSON.stringify(l.attrs || {})}`.toLowerCase()
      return fullText.includes(q)
    })
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
    <div class="space-y-4 flex flex-col h-[calc(100vh-140px)]">
      {/* 头部与状态栏 */}
      <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-3 shrink-0">
        <div>
          <h1 class="text-xl font-semibold flex items-center gap-2.5">
            网关系统日志
            <span class={`inline-block w-2.5 h-2.5 rounded-full ${connected() ? 'bg-emerald-500 shadow-emerald-500/50 shadow-sm' : 'bg-zinc-600'}`} />
          </h1>
          <p class="text-sm text-faint mt-0.5">
            实时捕获与推送 Cyrene Gateway 后端请求转发、上游故障重试与轮转事件
          </p>
        </div>

        <div class="flex items-center gap-2">
          <Button
            size="sm"
            variant={autoScroll() ? 'primary' : 'secondary'}
            onClick={() => setAutoScroll(!autoScroll())}
          >
            {autoScroll() ? '自动滚动: 开' : '自动滚动: 关'}
          </Button>
          <Button size="sm" variant="ghost" onClick={() => setLogs([])}>
            清屏
          </Button>
        </div>
      </div>

      {/* 过滤工具栏 */}
      <Card class="p-3 flex flex-wrap items-center justify-between gap-3 shadow-sm shrink-0">
        <div class="flex flex-wrap items-center gap-3 flex-1">
          <Input
            class="!w-64"
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
          共 {logs().length} 条，显示 {filteredLogs().length} 条
        </div>
      </Card>

      {/* 实时终端日志视窗 */}
      <div
        ref={scrollContainer}
        class="flex-1 min-h-0 bg-[#0d1117] border border-subtle rounded-2xl p-4 font-mono text-xs overflow-y-auto space-y-1.5 selection:bg-accent/30 shadow-inner"
      >
        <Show
          when={filteredLogs().length > 0}
          fallback={
            <div class="h-full flex items-center justify-center text-zinc-600 text-sm">
              暂无匹配的系统运行日志…
            </div>
          }
        >
          <For each={filteredLogs()}>
            {log => (
              <div class="flex items-start gap-2.5 leading-relaxed hover:bg-white/[0.02] px-1.5 py-0.5 rounded transition-colors break-all">
                <span class="text-zinc-500 shrink-0 select-none">{formatTime(log.time)}</span>
                <span class={`px-1.5 py-0.2 rounded text-[10px] shrink-0 uppercase select-none ${levelColor(log.level)}`}>
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
