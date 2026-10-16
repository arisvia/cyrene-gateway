import { type Component, Show, createSignal } from 'solid-js'
import { useI18n } from '@/i18n'
import { IconChevronDown, IconChevronRight, IconSparkles } from '@/components/ui'

interface ThinkingBlockProps {
  reasoning: string
  busy?: boolean
  hasContent?: boolean
}

/** 演练场思考过程折叠卡片：支持实时流式展开与完成后按需折叠 */
const ThinkingBlock: Component<ThinkingBlockProps> = props => {
  const { t } = useI18n()
  const [userToggled, setUserToggled] = createSignal<boolean | null>(null)

  // 默认展开状态：当仍在生成且未吐出最终回答正文时保持展开；正文开始吐出或生成结束时默认折叠
  const isExpanded = () => {
    if (userToggled() !== null) return userToggled()!
    if (props.busy && !props.hasContent) return true
    return false
  }

  return (
    <div class="min-w-0 rounded-xl bg-surface-inset border border-subtle/50 p-2.5 space-y-2 mb-2">
      <button
        type="button"
        class="flex items-center gap-1.5 text-xs text-faint hover:text-text transition-colors select-none cursor-pointer group w-full text-left"
        onClick={() => setUserToggled(!isExpanded())}
        aria-expanded={isExpanded()}
      >
        <Show when={isExpanded()} fallback={<IconChevronRight size={13} class="text-faint group-hover:text-text shrink-0" />}>
          <IconChevronDown size={13} class="text-faint group-hover:text-text shrink-0" />
        </Show>
        <IconSparkles size={13} class={props.busy && !props.hasContent ? 'text-accent animate-pulse shrink-0' : 'text-faint shrink-0'} />
        <span class="font-medium text-text truncate">
          {props.busy && !props.hasContent ? t('playground.thinkingInProgress') : t('playground.thinkingProcess')}
        </span>
        <span class="text-[10px] text-faint/80 font-mono ml-auto shrink-0">
          {isExpanded() ? t('playground.collapseThinking') : t('playground.expandThinking')}
        </span>
      </button>

      <Show when={isExpanded()}>
        <div class="p-2.5 rounded-lg bg-surface/80 border border-subtle/40 text-xs text-muted font-mono leading-relaxed whitespace-pre-wrap break-words [overflow-wrap:anywhere] select-text max-h-72 overflow-y-auto px-2">
          {props.reasoning}
        </div>
      </Show>
    </div>
  )
}

export default ThinkingBlock
