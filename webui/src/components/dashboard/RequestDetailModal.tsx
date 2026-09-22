import { type Component, Show, createSignal, createResource } from 'solid-js'
import { Portal } from 'solid-js/web'
import { Badge, Button, IconButton, ProviderAvatar, Spinner, SegmentedControl, IconClose, IconMessageSquare, IconSparkles } from '@/components/ui'
import { api } from '@/lib/api'
import { useI18n } from '@/i18n'
import { formatNumber as fmtNum, formatCost as fmtCost, timeAgo as fmtTime } from '@/lib/format'
import { fetchModelDisplayNameMap, resolveModelDisplayName } from '@/lib/models'
import type { RequestDetail } from '@/types/domain'
interface RequestDetailModalProps {
  item: RequestDetail | null
  onClose: () => void
}

export const RequestDetailModal: Component<RequestDetailModalProps> = props => {
  const [activeTab, setActiveTab] = createSignal<'overview' | 'payload' | 'raw'>('overview')
  const [modelNameMap, setModelNameMap] = createSignal<Record<string, string>>({})
  const { t } = useI18n()
  const timeUnits = () => ({
    justNow: t('time.justNow'),
    minutesAgo: (m: number) => t('time.minutesAgo', { m }),
    hoursAgo: (h: number) => t('time.hoursAgo', { h }),
    daysAgo: (d: number) => t('time.daysAgo', { d }),
  })

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
  fetchModelDisplayNameMap().then(setModelNameMap)
  // 清洗并提取对话内容（过滤冗长思考过程 <thinking> ... </thinking>）
  const cleanPayload = () => {
    const raw = detail()
    if (!raw || typeof raw !== 'object') return null
    const d = raw as Record<string, unknown>
    let dataObj = (d.data && typeof d.data === 'object' ? d.data : null) as Record<string, unknown> | null
    if (typeof d.data === 'string') {
      try {
        dataObj = JSON.parse(d.data) as Record<string, unknown>
      } catch { /* ignore */ }
    }
    let input = d.input ?? d.prompt ?? d.messages ?? dataObj?.input ?? dataObj?.prompt ?? dataObj?.messages ?? null
    let output = d.output ?? d.response ?? d.content ?? dataObj?.output ?? dataObj?.response ?? dataObj?.content ?? null

    // 如果为字符串，过滤思考块
    if (typeof output === 'string') {
      output = output.replace(/<thinking>[\s\S]*?<\/thinking>/gi, '').trim()
    } else if (output && typeof output === 'object' && 'content' in output) {
      const content = output.content
      if (typeof content === 'string') {
        output = content.replace(/<thinking>[\s\S]*?<\/thinking>/gi, '').trim()
      }
    }

    return { input, output }
  }

  return (
    <Show when={props.item}>
      <Portal>
        <div class="fixed inset-0 z-[100] flex items-center justify-center p-4 sm:p-6 animate-fade-in">
          {/* 背景遮罩 */}
          <button
            type="button"
            aria-label={t('requestDetail.closeOverlay')}
            class="absolute inset-0 w-full h-full bg-overlay backdrop-blur-sm border-none cursor-default"
            onClick={props.onClose}
          />
        {/* 弹窗主体卡片 */}
        <div class="relative w-full min-w-0 max-w-2xl max-h-[calc(100dvh-2rem)] sm:max-h-[calc(100dvh-3rem)] glass-float rounded-card flex flex-col overflow-hidden animate-slide-up z-10">
          {/* 顶栏：标题、模型与关闭按钮 */}
          <div class="min-h-16 px-4 sm:px-6 py-4 border-b border-subtle flex items-center justify-between gap-3 shrink-0">
            <div class="flex items-center gap-3 min-w-0">
              <ProviderAvatar provider={props.item?.provider || 'default'} name={props.item?.provider} size="sm" class="shrink-0" />
              <div class="min-w-0">
                <div class="text-sm font-semibold text-text flex flex-wrap items-center gap-x-2 gap-y-1 min-w-0">
                  <span class="min-w-0 break-words [overflow-wrap:anywhere]">{resolveModelDisplayName(modelNameMap(), props.item?.model) || t('common.unknownModel')}</span>
                  <Show when={Boolean(props.item?.model && resolveModelDisplayName(modelNameMap(), props.item?.model) !== props.item?.model)}>
                    <span class="text-xs text-muted font-mono font-normal min-w-0 break-words [overflow-wrap:anywhere]">({props.item?.model})</span>
                  </Show>
                  <Badge tone={props.item?.status === 'ok' ? 'green' : 'red'} class="text-xs shrink-0">
                    {props.item?.status || t('common.unknown')}
                  </Badge>
                </div>
                <div class="text-xs text-faint font-mono truncate mt-0.5">
                  ID: {props.item?.id || '-'} · {props.item?.timestamp ? fmtTime(props.item.timestamp, timeUnits()) : ''}
                </div>
              </div>
            </div>

            <IconButton
              size="sm"
              class="shrink-0"
              onClick={props.onClose}
              title={t('requestDetail.closeEsc')}
              aria-label={t('requestDetail.close')}
            >
              <IconClose size={16} />
            </IconButton>
          </div>

          {/* 分段 Tab 切换器 */}
          <div class="px-4 sm:px-6 py-3 border-b border-subtle flex flex-wrap items-center justify-between gap-3 shrink-0">
            <SegmentedControl
              value={activeTab()}
              onChange={setActiveTab}
              options={[
                { value: 'overview', label: t('requestDetail.tabs.overview') },
                { value: 'payload', label: t('requestDetail.tabs.payload') },
                { value: 'raw', label: t('requestDetail.tabs.raw') },
              ]}
            />

            <Show when={fullDetail.loading}>
              <div class="flex items-center gap-1.5 text-xs text-faint">
                <Spinner />
                <span>{t('requestDetail.loadingMeta')}</span>
              </div>
            </Show>
          </div>

          {/* 内容区 */}
          <div class="flex-1 min-h-0 min-w-0 overflow-y-auto px-4 sm:px-6 py-5 space-y-5 text-sm">
            {/* 1. 调度概览与 Token KPI */}
            <Show when={activeTab() === 'overview'}>
              <div class="grid grid-cols-2 sm:grid-cols-4 gap-3">
                <div class="p-3.5 rounded-xl bg-surface-inset border border-subtle">
                  <div class="text-xs text-faint">{t('requestDetail.promptTokens')}</div>
                  <div class="text-base font-semibold mt-1 tabular-nums text-text">
                    {fmtNum(props.item?.promptTokens ?? 0)}
                  </div>
                </div>
                <div class="p-3.5 rounded-xl bg-surface-inset border border-subtle">
                  <div class="text-xs text-faint">{t('requestDetail.completionTokens')}</div>
                  <div class="text-base font-semibold mt-1 tabular-nums text-text">
                    {fmtNum(props.item?.completionTokens ?? 0)}
                  </div>
                </div>
                <div class="p-3.5 rounded-xl bg-surface-inset border border-subtle">
                  <div class="text-xs text-faint">{t('requestDetail.totalLatency')}</div>
                  <div class="text-base font-semibold mt-1 tabular-nums text-accent">
                    {props.item?.latencyMs ?? '-'} ms
                  </div>
                </div>
                <div class="p-3.5 rounded-xl bg-surface-inset border border-subtle">
                  <div class="text-xs text-faint">{t('requestDetail.estimatedCost')}</div>
                  <div class="text-base font-semibold mt-1 tabular-nums text-text">
                    {fmtCost(props.item?.cost ?? 0)}
                  </div>
                </div>
              </div>

              {/* 路由与调度属性列表 */}
              <div class="rounded-xl border border-subtle bg-card/40 divide-y divide-subtle/50">
                <div class="px-4 py-3 flex flex-wrap items-start justify-between gap-x-4 gap-y-1 min-w-0 break-words [overflow-wrap:anywhere]">
                  <span class="text-faint">{t('requestDetail.properties.provider')}</span>
                  <span class="font-semibold text-text">{props.item?.provider || '-'}</span>
                </div>
                <div class="px-4 py-3 flex flex-wrap items-start justify-between gap-x-4 gap-y-1 min-w-0 break-words [overflow-wrap:anywhere]">
                  <span class="text-faint">{t('requestDetail.properties.targetModel')}</span>
                  <span class="font-mono text-text">{props.item?.model || '-'}</span>
                </div>
                <div class="px-4 py-3 flex flex-wrap items-start justify-between gap-x-4 gap-y-1 min-w-0 break-words [overflow-wrap:anywhere]">
                  <span class="text-faint">{t('requestDetail.properties.endpoint')}</span>
                  <span class="font-mono text-faint">{props.item?.endpoint || '/v1/chat/completions'}</span>
                </div>
                <div class="px-4 py-3 flex flex-wrap items-start justify-between gap-x-4 gap-y-1 min-w-0 break-words [overflow-wrap:anywhere]">
                  <span class="text-faint">{t('requestDetail.properties.timestamp')}</span>
                  <span class="font-mono text-faint">{props.item?.timestamp || '-'}</span>
                </div>
                <Show when={props.item?.connectionId}>
                  <div class="px-4 py-3 flex flex-wrap items-start justify-between gap-x-4 gap-y-1 min-w-0 break-words [overflow-wrap:anywhere]">
                    <span class="text-faint">{t('requestDetail.properties.connectionId')}</span>
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
                  <div class="font-semibold text-text flex items-center gap-1.5 text-xs">
                    <IconMessageSquare size={14} class="text-accent" />
                    {t('requestDetail.inputContent')}
                  </div>
                  <div class="min-w-0 p-3.5 rounded-xl bg-code-bg text-text border border-subtle font-mono text-xs leading-relaxed max-h-52 overflow-y-auto whitespace-pre-wrap break-words [overflow-wrap:anywhere] select-text">
                    {cleanPayload()?.input
                      ? (typeof cleanPayload()!.input === 'object' ? JSON.stringify(cleanPayload()!.input, null, 2) : String(cleanPayload()!.input))
                      : t('requestDetail.noPromptContent')}
                  </div>
                </div>

                {/* 模型回复 */}
                <div class="space-y-1.5">
                  <div class="font-semibold text-text flex items-center gap-1.5 text-xs">
                    <IconSparkles size={14} class="text-accent-2" />
                    {t('requestDetail.outputContent')}
                  </div>
                  <div class="min-w-0 p-3.5 rounded-xl bg-code-bg text-text border border-subtle font-mono text-xs leading-relaxed max-h-52 overflow-y-auto whitespace-pre-wrap break-words [overflow-wrap:anywhere] select-text">
                    {cleanPayload()?.output
                      ? (typeof cleanPayload()!.output === 'object' ? JSON.stringify(cleanPayload()!.output, null, 2) : String(cleanPayload()!.output))
                      : t('requestDetail.noResponseContent')}
                  </div>
                </div>
              </div>
            </Show>

            {/* 3. 原始元数据 JSON */}
            <Show when={activeTab() === 'raw'}>
              <div class="min-w-0 p-4 rounded-xl bg-code-bg text-text border border-subtle font-mono text-xs leading-relaxed overflow-auto max-h-80 select-text">
                <pre class="whitespace-pre-wrap break-words [overflow-wrap:anywhere]">{JSON.stringify(detail(), null, 2)}</pre>
              </div>
            </Show>
          </div>

          {/* 底栏 */}
          <div class="min-h-14 px-4 sm:px-6 py-3 border-t border-subtle flex items-center justify-end shrink-0">
            <Button size="sm" variant="secondary" onClick={props.onClose}>
              {t('common.close')}
            </Button>
          </div>
        </div>
        </div>
      </Portal>
    </Show>
  )
}
