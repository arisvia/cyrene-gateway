import { type Component, Show } from 'solid-js'
import { useI18n } from '@/i18n'
import { protocolBadgeTone, protocolLabel } from '@/lib/playgroundRequest'
import type { AssistantResponse } from '@/stores/playground'
import { Badge, Card, IconButton, IconClipboard, IconCode, ProviderAvatar } from '@/components/ui'
import ThinkingBlock from './ThinkingBlock'

interface ResponseCardProps {
  response: AssistantResponse
  /** 槽位标记，横评卡片左上角的字母角标。 */
  tag: string
  tone: 'info' | 'accent2'
  /** 模型展示名依赖页面持有的模型表，由调用方解析后传入。 */
  displayName: string
  providerName: string
  /** 头像按提供商 id 取图标，与展示用的名称是两件事。 */
  providerId: string
  copyLabel: string
  onViewRaw: () => void
  onCopy: () => void
}

const TONES = {
  info: { card: 'border-t-info/60', chip: 'bg-info/10 text-info border-info/25' },
  accent2: { card: 'border-t-accent-2/60', chip: 'bg-accent-2/10 text-accent-2 border-accent-2/25' },
} as const

/** 横评模式下的单张回答卡：A 与 B 共用同一版式，只差槽位色调与文案键。 */
const ResponseCard: Component<ResponseCardProps> = props => {
  const { t } = useI18n()
  const resp = () => props.response
  const tone = () => TONES[props.tone]
  const ttft = () => props.response.metrics?.ttftMs
  const speed = () => props.response.metrics?.speed

  return (
    <Card class={`min-w-0 p-3.5 flex flex-col space-y-2.5 border-t-2 ${tone().card}`}>
      <div class="flex items-center justify-between border-b border-subtle/40 pb-2 flex-wrap gap-1.5 shrink-0">
        <div class="flex flex-wrap items-center gap-2 min-w-0">
          <span class={`w-5 h-5 rounded-md text-xs font-bold flex items-center justify-center shrink-0 border ${tone().chip}`}>
            {props.tag}
          </span>
          <ProviderAvatar provider={props.providerId} size="sm" />
          <div class="flex flex-col min-w-0 leading-tight">
            <span
              class="text-xs font-semibold text-text truncate max-w-[150px] sm:max-w-[180px]"
              title={resp().servedModel ? `${resp().targetModel} (served: ${resp().servedModel})` : resp().targetModel}
            >
              {props.displayName}
            </span>
            <div class="flex items-center gap-1 mt-0.5">
              <span class="text-xs text-faint truncate max-w-[100px]">
                {props.providerName}
              </span>
              <Badge tone={protocolBadgeTone(resp().protocol)} class="text-xs font-mono px-1 py-0 uppercase">
                {protocolLabel(resp().protocol)}
              </Badge>
            </div>
          </div>
          <Show when={resp().busy}>
            <span class="text-xs text-info animate-pulse ml-1 shrink-0">{t('playground.generating')}</span>
          </Show>
        </div>
        <div class="flex flex-wrap items-center gap-2 text-xs font-mono text-muted min-w-0">
          <Show when={ttft() !== undefined}>
            <span>{ttft()}ms</span>
          </Show>
          <Show when={speed() && speed()! > 0}>
            <span class="text-accent">{speed()} t/s</span>
          </Show>
          <Show when={resp().rawRequest}>
            <IconButton
              size="sm"
              title={t('playground.viewRawJson')}
              aria-label={t('playground.viewRawJson')}
              onClick={props.onViewRaw}
            >
              <IconCode size={14} />
            </IconButton>
          </Show>
          <IconButton
            size="sm"
            title={props.copyLabel}
            aria-label={props.copyLabel}
            onClick={props.onCopy}
          >
            <IconClipboard size={14} />
          </IconButton>
        </div>
      </div>

      <Show
        when={!resp().error}
        fallback={
          <div class="min-w-0 p-3 bg-danger/10 border border-danger/25 rounded-lg text-danger text-xs font-mono whitespace-pre-wrap break-words [overflow-wrap:anywhere]">
            {resp().error}
          </div>
        }
      >
        <Show when={resp().reasoning}>
          <ThinkingBlock
            reasoning={resp().reasoning!}
            busy={resp().busy}
            hasContent={Boolean(resp().content)}
          />
        </Show>
        <div class="min-w-0 text-sm text-text whitespace-pre-wrap break-words [overflow-wrap:anywhere] leading-relaxed select-text min-h-[60px]">
          {resp().content || (resp().busy ? '...' : (resp().reasoning ? '' : t('playground.emptyResponse')))}
        </div>
      </Show>
    </Card>
  )
}

export default ResponseCard
