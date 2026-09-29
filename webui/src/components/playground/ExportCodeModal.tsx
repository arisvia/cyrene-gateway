import { type Component, Show, createSignal } from 'solid-js'
import { useI18n } from '@/i18n'
import type { ProtocolOption } from '@/stores/playground'
import type { CodeLang } from '@/lib/playgroundRequest'
import { Badge, Button, IconCheck, IconClipboard, Modal, SegmentedControl } from '@/components/ui'

interface ExportCodeModalProps {
  open: () => boolean
  onClose: () => void
  activeMeta: () => ProtocolOption
  snippet: (lang: CodeLang) => string
  onCopy: (text: string, label: string) => void
}

/** 客户端调用代码导出弹窗：语言选择与复制态只属于这里，不占页面信号。 */
const ExportCodeModal: Component<ExportCodeModalProps> = props => {
  const { t } = useI18n()
  const [lang, setLang] = createSignal<CodeLang>('curl')
  const [copied, setCopied] = createSignal(false)

  return (
  <Modal
    open={props.open()}
    title={t('playground.exportCodeModalTitle')}
    onClose={() => props.onClose()}
  >
    <div class="space-y-4">
      <div class="flex items-center justify-between gap-2 flex-wrap">
        <div class="flex flex-wrap items-center gap-2 min-w-0">
          <SegmentedControl
            value={lang()}
            onChange={l => setLang(l as 'curl' | 'python' | 'node')}
            options={[
              { value: 'curl', label: 'cURL' },
              { value: 'python', label: 'Python' },
              { value: 'node', label: 'Node.js / TS' },
            ]}
            size="sm"
          />
          <Badge tone={props.activeMeta().badgeTone} class="text-xs font-mono px-2 py-0.5 uppercase">
            {props.activeMeta().shortLabel}
          </Badge>
        </div>
        <Button
          variant="primary"
          class="gap-1.5"
          onClick={() => {
            props.onCopy(props.snippet(lang()), t('playground.copyClientCode'))
            setCopied(true)
            setTimeout(() => setCopied(false), 2000)
          }}
        >
          <Show when={copied()} fallback={<IconClipboard size={14} />}>
            <IconCheck size={14} />
          </Show>
          {t('playground.copyCode')}
        </Button>
      </div>

      <div class="relative">
        <pre class="min-w-0 p-4 bg-code-bg text-text rounded-xl text-xs leading-relaxed font-mono overflow-auto max-h-[min(24rem,60dvh)] border border-subtle">
          <code>{props.snippet(lang())}</code>
        </pre>
      </div>
    </div>
  </Modal>
  )
}

export default ExportCodeModal
