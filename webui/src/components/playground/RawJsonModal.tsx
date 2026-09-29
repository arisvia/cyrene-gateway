import { type Component, Show } from 'solid-js'
import { useI18n } from '@/i18n'
import { Button, Modal } from '@/components/ui'

export interface RawPayload {
  title: string
  request: unknown
  response: unknown
}

interface RawJsonModalProps {
  data: () => RawPayload | null
  onClose: () => void
  onCopy: (text: string, label: string) => void
}

/** 单轮请求/响应原始报文对照，数据由轮次卡片注入。 */
const RawJsonModal: Component<RawJsonModalProps> = props => {
  const { t } = useI18n()

  return (
  <Show when={props.data()}>
    {data => (
      <Modal
        open={true}
        title={`${t('playground.rawJsonModalTitle')} - ${data().title}`}
        onClose={() => props.onClose()}
      >
        <div class="space-y-4">
          <div class="space-y-1.5">
            <div class="flex flex-wrap items-center justify-between gap-2 text-xs font-medium text-muted">
              <span>{t('playground.requestPayload')}</span>
              <Button
                variant="ghost"
                size="sm"
                class="text-accent"
                onClick={() => props.onCopy(JSON.stringify(data().request, null, 2), t('playground.copyRequestJson'))}
              >
                {t('playground.copyRequestJson')}
              </Button>
            </div>
            <pre class="min-w-0 p-3 bg-code-bg text-text rounded-lg text-xs leading-relaxed font-mono overflow-auto max-h-40 whitespace-pre-wrap break-words [overflow-wrap:anywhere] border border-subtle">
              {JSON.stringify(data().request, null, 2)}
            </pre>
          </div>

          <div class="space-y-1.5">
            <div class="flex flex-wrap items-center justify-between gap-2 text-xs font-medium text-muted">
              <span>{t('playground.responsePayload')}</span>
              <Button
                variant="ghost"
                size="sm"
                class="text-accent"
                onClick={() => props.onCopy(JSON.stringify(data().response, null, 2), t('playground.copyResponseJson'))}
              >
                {t('playground.copyResponseJson')}
              </Button>
            </div>
            <pre class="min-w-0 p-3 bg-code-bg text-text rounded-lg text-xs leading-relaxed font-mono overflow-auto max-h-56 whitespace-pre-wrap break-words [overflow-wrap:anywhere] border border-subtle">
              {JSON.stringify(data().response, null, 2)}
            </pre>
          </div>
        </div>
      </Modal>
    )}
  </Show>
  )
}

export default RawJsonModal
