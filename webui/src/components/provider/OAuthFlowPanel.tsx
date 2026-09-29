import { type Component, Show } from 'solid-js'
import { useI18n } from '@/i18n'
import { useToast } from '@/lib/toast'
import { copyToClipboard } from '@/lib/clipboard'
import type { useOAuthFlow } from '@/lib/oauthFlow'
import { Alert, Button, IconButton, Input, Textarea, IconCheck, IconClipboard } from '@/components/ui'

interface OAuthFlowPanelProps {
  flow: ReturnType<typeof useOAuthFlow>
}

/**
 * OAuth 授权流程面板：错误重试、令牌导入、设备码/网页流三态。
 *
 * 提供商市场向导与连接详情页共用同一套版式与文案，模态外壳只负责 open/title。
 */
const OAuthFlowPanel: Component<OAuthFlowPanelProps> = props => {
  const { t } = useI18n()
  const toast = useToast()
  const {
    deviceFlow, manualCode, setManualCode, manualExchanging, devicePolling, deviceError,
    copiedCode, setCopiedCode, isImportFlow, importTokenText, setImportTokenText, importingToken,
    startDeviceFlow, cancelDeviceFlow, handleImportToken, handleManualSubmit,
  } = props.flow

  return (
    <div class="space-y-4 py-2">
      {/* 1. 纯错误展示（未处于导入流程时优先接管展示与重试） */}
      <Show when={deviceError() && !isImportFlow()}>
        <div class="space-y-4 py-2">
          <Alert variant="danger" title={t('providerDetail.authProblemTitle')}>
            {deviceError()}
          </Alert>
          <div class="flex justify-end gap-2">
            <Button size="sm" variant="secondary" onClick={cancelDeviceFlow}>
              {t('common.close')}
            </Button>
            <Button size="sm" variant="primary" onClick={() => startDeviceFlow()}>
              {t('common.retry')}
            </Button>
          </div>
        </div>
      </Show>

      {/* 2. 令牌导入流程 (如 Cursor) */}
      <Show when={isImportFlow()}>
        <div class="space-y-3 text-left py-1">
          <p class="text-xs text-faint leading-relaxed">
            {t('providerDetail.tokenImportHint')}
          </p>
          <div class="space-y-1.5">
            <Textarea
              rows={4}
              value={importTokenText()}
              onInput={setImportTokenText}
              placeholder={t('providerDetail.tokenPastePlaceholder')}
              class="px-3 py-2 text-xs font-mono"
            />
          </div>
          <Show when={deviceError()}>
            <Alert variant="danger">
              {deviceError()}
            </Alert>
          </Show>
          <div class="flex justify-end gap-2 pt-2 border-t border-subtle">
            <Button size="sm" variant="secondary" onClick={cancelDeviceFlow}>
              {t('common.cancel')}
            </Button>
            <Button
              size="sm"
              variant="primary"
              loading={importingToken()}
              disabled={!importTokenText().trim()}
              onClick={handleImportToken}
            >
              {t('providerDetail.importAndSave')}
            </Button>
          </div>
        </div>
      </Show>

      {/* 3. 设备码 / 网页流卡片 */}
      <Show when={!isImportFlow() && (deviceFlow() || devicePolling()) && !deviceError()}>
        <Show
          when={deviceFlow()}
          fallback={
            <div class="py-8 flex flex-col items-center justify-center gap-3">
              <span class="w-8 h-8 rounded-full border-2 border-accent border-t-transparent animate-spin" />
              <p class="text-xs text-faint">{t('providerDetail.requestingAuthCode')}</p>
            </div>
          }
        >
          {flow => (
            <div class="space-y-4 text-center">
              <Show
                when={!flow().isManualPaste}
                fallback={
                  <div class="space-y-3 py-1 text-left">
                    <p class="text-xs text-faint leading-relaxed">
                      {t('providers.oauthPasteCallbackHelp')}
                    </p>
                    <div class="space-y-1.5">
                      <Input
                        value={manualCode()}
                        onInput={v => setManualCode(v)}
                        placeholder={t('providers.oauthPasteCallbackPlaceholder')}
                        class="font-mono text-xs"
                      />
                    </div>
                    <div class="flex items-center justify-between gap-2 pt-1">
                      <Button
                        size="sm"
                        variant="secondary"
                        onClick={() => {
                          const url = flow().verificationUriComplete || flow().verificationUri
                          if (url) window.open(url, '_blank')
                        }}
                      >
                        {t('providerDetail.openAuthPage')}
                      </Button>
                      <Button
                        size="sm"
                        variant="primary"
                        loading={manualExchanging()}
                        disabled={!manualCode().trim()}
                        onClick={handleManualSubmit}
                      >
                        {t('providers.confirmAndConnect')}
                      </Button>
                    </div>
                  </div>
                }
              >
                {/* 有 UserCode 时展示验证码；无 UserCode 时展示网页跳转指引 */}
                <Show
                  when={flow().userCode}
                  fallback={
                    <div class="space-y-2 py-1">
                      <div class="text-xs text-faint">{t('providerDetail.browserAuthPrompt')}</div>
                      <div class="flex items-center justify-center gap-2">
                        <Button
                          size="sm"
                          variant="secondary"
                          onClick={() => {
                            const url = flow().verificationUriComplete || flow().verificationUri
                            if (url) window.open(url, '_blank')
                          }}
                        >
                          {t('providerDetail.openAuthPage')}
                        </Button>
                      </div>
                    </div>
                  }
                >
                  <p class="text-xs text-faint leading-relaxed">
                    {t('providerDetail.authPageHint')}
                  </p>
                  {/* 验证码卡片 */}
                  <div class="p-4 rounded-card bg-accent/10 border border-accent/30 space-y-1.5">
                    <div class="text-[11px] text-faint font-medium">{t('providerDetail.authCodeLabel')}</div>
                    <div class="flex items-center justify-center gap-3">
                      <span class="font-mono text-2xl sm:text-3xl font-bold text-accent tracking-widest select-all">
                        {flow().userCode}
                      </span>
                      <IconButton size="sm" variant="ghost"
                        type="button"
                        class="hover:bg-accent/20 text-accent"
                        title={t('providerDetail.copyCodeTitle')}
                        onClick={async () => {
                          if (flow().userCode) {
                            const ok = await copyToClipboard(flow().userCode!)
                            if (ok) {
                              setCopiedCode(true)
                              toast.success(t('providerDetail.codeCopied'))
                              setTimeout(() => setCopiedCode(false), 2000)
                            } else {
                              toast.info(flow().userCode!)
                            }
                          }
                        }}
                      >
                        {copiedCode() ? <IconCheck size={14} class="text-success" /> : <IconClipboard size={14} />}
                      </IconButton>
                    </div>
                  </div>
                </Show>

                {/* 登录 URL 卡片 */}
                <div class="p-3.5 rounded-card bg-hover/80 border border-subtle text-left space-y-2">
                  <div class="text-[11px] text-faint font-medium text-center">{t('providerDetail.loginUrl')}</div>
                  <div class="font-mono text-xs break-all text-foreground select-all bg-bg/70 p-2.5 rounded border border-subtle leading-relaxed">
                    {flow().verificationUriComplete || flow().verificationUri}
                  </div>
                  <div class="flex items-center justify-end gap-2 pt-1">
                    <Button
                      size="sm"
                      variant="secondary"
                      onClick={async () => {
                        const url = flow().verificationUriComplete || flow().verificationUri
                        if (url) {
                          const ok = await copyToClipboard(url)
                          if (ok) {
                            toast.success(t('providerDetail.copyUrlSuccess'))
                          } else {
                            toast.info(url)
                          }
                        }
                      }}
                    >
                      {t('providerDetail.copyUrl')}
                    </Button>
                    <Button
                      size="sm"
                      variant="secondary"
                      onClick={() => {
                        const url = flow().verificationUriComplete || flow().verificationUri
                        if (url) window.open(url, '_blank')
                      }}
                    >
                      {t('providerDetail.openPage')}
                    </Button>
                  </div>
                </div>

                {/* 轮询等待状态 */}
                <div class="flex items-center justify-center gap-2 pt-2 text-xs text-faint">
                  <span class="w-2.5 h-2.5 rounded-full border-2 border-accent border-t-transparent animate-spin" />
                  <span>{t('providerDetail.waitingAuth')}</span>
                </div>
              </Show>

              <div class="pt-2 border-t border-subtle flex justify-end">
                <Button size="sm" variant="secondary" onClick={cancelDeviceFlow}>
                  {t('common.cancel')}
                </Button>
              </div>
            </div>
          )}
        </Show>
      </Show>
    </div>
  )
}

export default OAuthFlowPanel
