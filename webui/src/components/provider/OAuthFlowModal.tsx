import { type Component } from 'solid-js'
import { useI18n } from '@/i18n'
import type { useOAuthFlow } from '@/lib/oauthFlow'
import { Modal } from '@/components/ui'
import OAuthFlowPanel from './OAuthFlowPanel'

interface OAuthFlowModalProps {
  flow: ReturnType<typeof useOAuthFlow>
  displayName: () => string
}

/** 连接详情页的 OAuth 授权弹窗外壳：开合与标题在此，流程内容共用 OAuthFlowPanel。 */
const OAuthFlowModal: Component<OAuthFlowModalProps> = props => {
  const { t } = useI18n()
  const { deviceFlow, devicePolling, isImportFlow, deviceError, cancelDeviceFlow } = props.flow

  return (
    <Modal
      open={!!deviceFlow() || devicePolling() || isImportFlow() || !!deviceError()}
      title={isImportFlow() ? t('providerDetail.importTokenTitle', { name: props.displayName() }) : t('providerDetail.connectTitle', { name: props.displayName() })}
      onClose={cancelDeviceFlow}
    >
      <OAuthFlowPanel flow={props.flow} />
    </Modal>
  )
}

export default OAuthFlowModal
