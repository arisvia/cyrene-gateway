import { type Component, Show, createSignal, createResource } from 'solid-js'
import { Badge, Button, ProviderAvatar, Spinner } from '@/components/ui'
import { api } from '@/lib/api'
import { formatNumber as fmtNum, formatCost as fmtCost, timeAgo as fmtTime } from '@/lib/format'
import type { RequestDetail } from '@/types/domain'

interface RequestDetailModalProps {
  item: RequestDetail | null
  onClose: () => void
}

export const RequestDetailModal: Component<RequestDetailModalProps> = props => {
  const [activeTab, setActiveTab] = createSignal<'overview' | 'payload' | 'raw'>('overview')

  // 若 item 存在 id，尝试从后端获取更详细的持久化请求数据
  const [fullDetail] = createResource(
    () => props.item?.id,
    async id => {
      if (!id) return null
      try {
        const r = await api(`/api/usage/request-details/${encodeURIComponent(id)}`)
        return r || null
      } catch {
        return null
      }
    },
  )

  const detail = () => fullDetail() || props.item

  // 清洗并提取对话内容（过滤冗长思考过程 <thinking> ... </thinking>）
  const cleanPayload = () => {
    const d = detail() as any
    if (!d) return null
    let input = d.input || d.prompt || d.messages || d.data?.input || null
    let output = d.output || d.response || d.content || d.data?.output || null

    // 如果为字符串，过滤思考块
    if (typeof output === 'string') {
      output = output.replace(/<thinking>[\s\S]*?<\/thinking>/gi, '').trim()
    } else if (typeof output === 'object' && output?.content) {
      if (typeof output.content === 'string') {
        output.content = output.content.replace(/<thinking>[\s\S]*?<\/thinking>/gi, '').trim()
      }
    }

    return { input, output }
  }

  return (
    <Show when={props.item}>
      <div class="fixed inset-0 z-[80] flex items-center justify-center p-4 sm:p-6 animate-fade-in">
        {/* 背景遮罩 */}
        <div
          class="absolute inset-0 bg-black/60 backdrop-blur-sm"
          onClick={props.onClose}
          aria-hidden="true"
        />

        {/* 弹窗主体卡片 */}
        <div class="relative w-full max-w-2xl max-h-[85vh] bg-bg-elevated border border-subtle rounded-2xl shadow-2xl flex flex-col overflow-hidden animate-slide-up z-10">
          {/* 顶栏：标题、模型与关闭按钮 */}
          <div class="h-16 px-6 border-b border-subtle flex items-center justify-between gap-4 shrink-0 bg-card/40">
            <div class="flex items-center gap-3 min-w-0">
              <ProviderAvatar provider={props.item?.provider || 'default'} name={props.item?.provider} size="sm" class="shrink-0" />
              <div class="min-w-0">
                <div class="text-sm font-bold truncate text-foreground flex items-center gap-2">
                  <span>{props.item?.model || '未知模型'}</span>
                  <Badge tone={props.item?.status === 'ok' ? 'green' : 'red'} class="text-[10px]">
                    {props.item?.status || '未知'}
                  </Badge>
                </div>
                <div class="text-xs text-faint font-mono truncate mt-0.5">
                  ID: {props.item?.id || '-'} · {props.item?.timestamp ? fmtTime(props.item.timestamp) : ''}
                </div>
              </div>
            </div>

            <button
              type="button"
              class="w-8 h-8 rounded-control text-muted hover:text-foreground hover:bg-hover transition-colors flex items-center justify-center"
              onClick={props.onClose}
              title="关闭 (Esc)"
              aria-label="关闭"
            >
              <svg class="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                <line x1="18" y1="6" x2="6" y2="18" />
                <line x1="6" y1="6" x2="18" y2="18" />
              </svg>
            </button>
          </div>

          {/* 分段 Tab 切换器 */}
          <div class="px-6 pt-3 pb-2 border-b border-subtle/60 flex items-center justify-between gap-3 bg-bg/50 shrink-0">
            <div class="inline-flex p-1 rounded-xl bg-card border border-subtle shadow-sm text-xs">
              <button
                type="button"
                class={`px-3 py-1 font-semibold rounded-lg transition-all ${
                  activeTab() === 'overview'
                    ? 'bg-accent text-on-accent shadow-sm'
                    : 'text-muted hover:text-foreground'
                }`}
                onClick={() => setActiveTab('overview')}
              >
                调度概览与指标
              </button>
              <button
                type="button"
                class={`px-3 py-1 font-semibold rounded-lg transition-all ${
                  activeTab() === 'payload'
                    ? 'bg-accent text-on-accent shadow-sm'
                    : 'text-muted hover:text-foreground'
                }`}
                onClick={() => setActiveTab('payload')}
              >
                输入输出摘要
              </button>
              <button
                type="button"
                class={`px-3 py-1 font-semibold rounded-lg transition-all ${
                  activeTab() === 'raw'
                    ? 'bg-accent text-on-accent shadow-sm'
                    : 'text-muted hover:text-foreground'
                }`}
                onClick={() => setActiveTab('raw')}
              >
                元数据 JSON
              </button>
            </div>

            <Show when={fullDetail.loading}>
              <div class="flex items-center gap-1.5 text-xs text-faint">
                <Spinner />
                <span>加载元数据…</span>
              </div>
            </Show>
          </div>

          {/* 内容区 */}
          <div class="flex-1 overflow-y-auto p-6 space-y-5 text-xs">
            {/* 1. 调度概览与 Token KPI */}
            <Show when={activeTab() === 'overview'}>
              <div class="grid grid-cols-2 sm:grid-cols-4 gap-3">
                <div class="p-3.5 rounded-xl bg-card/60 border border-subtle">
                  <div class="text-[11px] text-faint">Prompt Tokens</div>
                  <div class="text-base font-semibold mt-1 tabular-nums text-foreground">
                    {fmtNum(props.item?.promptTokens ?? 0)}
                  </div>
                </div>
                <div class="p-3.5 rounded-xl bg-card/60 border border-subtle">
                  <div class="text-[11px] text-faint">Completion Tokens</div>
                  <div class="text-base font-semibold mt-1 tabular-nums text-foreground">
                    {fmtNum(props.item?.completionTokens ?? 0)}
                  </div>
                </div>
                <div class="p-3.5 rounded-xl bg-card/60 border border-subtle">
                  <div class="text-[11px] text-faint">总耗时</div>
                  <div class="text-base font-semibold mt-1 tabular-nums text-accent">
                    {props.item?.latencyMs ?? '-'} ms
                  </div>
                </div>
                <div class="p-3.5 rounded-xl bg-card/60 border border-subtle">
                  <div class="text-[11px] text-faint">估算成本</div>
                  <div class="text-base font-semibold mt-1 tabular-nums text-foreground">
                    {fmtCost(props.item?.cost ?? 0)}
                  </div>
                </div>
              </div>

              {/* 路由与调度属性列表 */}
              <div class="rounded-xl border border-subtle bg-card/40 divide-y divide-subtle/50">
                <div class="px-4 py-2.5 flex items-center justify-between">
                  <span class="text-faint">请求提供商 (Provider)</span>
                  <span class="font-semibold text-foreground">{props.item?.provider || '-'}</span>
                </div>
                <div class="px-4 py-2.5 flex items-center justify-between">
                  <span class="text-faint">实际目标模型 (Model)</span>
                  <span class="font-mono text-foreground">{props.item?.model || '-'}</span>
                </div>
                <div class="px-4 py-2.5 flex items-center justify-between">
                  <span class="text-faint">网关接收端点 (Endpoint)</span>
                  <span class="font-mono text-faint">{props.item?.endpoint || '/v1/chat/completions'}</span>
                </div>
                <div class="px-4 py-2.5 flex items-center justify-between">
                  <span class="text-faint">请求时间戳 (UTC)</span>
                  <span class="font-mono text-faint">{props.item?.timestamp || '-'}</span>
                </div>
                <Show when={props.item?.connectionId}>
                  <div class="px-4 py-2.5 flex items-center justify-between">
                    <span class="text-faint">连接凭证 ID</span>
                    <span class="font-mono text-faint">{props.item?.connectionId}</span>
                  </div>
                </Show>
              </div>
            </Show>

            {/* 2. 输入输出摘要（智能过滤思考） */}
            <Show when={activeTab() === 'payload'}>
              <div class="space-y-4">
                {/* 客户端输入 */}
                <div class="space-y-1.5">
                  <div class="font-semibold text-foreground flex items-center gap-1.5 text-xs">
                    <svg class="w-3.5 h-3.5 text-accent" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                      <path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z" />
                    </svg>
                    请求输入 (Prompt Payload)
                  </div>
                  <div class="p-3.5 rounded-xl bg-hover/40 border border-subtle font-mono text-[11px] leading-relaxed max-h-52 overflow-y-auto whitespace-pre-wrap break-all select-text">
                    {cleanPayload()?.input
                      ? (typeof cleanPayload()!.input === 'object' ? JSON.stringify(cleanPayload()!.input, null, 2) : String(cleanPayload()!.input))
                      : '（该历史记录未包含完整 Prompt 内容）'}
                  </div>
                </div>

                {/* 模型回复 */}
                <div class="space-y-1.5">
                  <div class="font-semibold text-foreground flex items-center gap-1.5 text-xs">
                    <svg class="w-3.5 h-3.5 text-accent-2" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                      <path d="M12 2v20M17 5H9.5a3.5 3.5 0 0 0 0 7h5a3.5 3.5 0 0 1 0 7H6" />
                    </svg>
                    模型回复 (Response Content · 已自动精简长思考)
                  </div>
                  <div class="p-3.5 rounded-xl bg-hover/40 border border-subtle font-mono text-[11px] leading-relaxed max-h-52 overflow-y-auto whitespace-pre-wrap break-all select-text">
                    {cleanPayload()?.output
                      ? (typeof cleanPayload()!.output === 'object' ? JSON.stringify(cleanPayload()!.output, null, 2) : String(cleanPayload()!.output))
                      : '（该历史记录未包含完整 Response 内容）'}
                  </div>
                </div>
              </div>
            </Show>

            {/* 3. 原始元数据 JSON */}
            <Show when={activeTab() === 'raw'}>
              <div class="p-4 rounded-xl bg-code-bg border border-subtle font-mono text-[11px] leading-relaxed overflow-x-auto max-h-80 select-text">
                <pre>{JSON.stringify(detail(), null, 2)}</pre>
              </div>
            </Show>
          </div>

          {/* 底栏 */}
          <div class="h-14 px-6 border-t border-subtle flex items-center justify-end bg-card/20 shrink-0">
            <Button size="sm" variant="secondary" onClick={props.onClose}>
              关闭
            </Button>
          </div>
        </div>
      </div>
    </Show>
  )
}
