import { createSignal, onCleanup } from 'solid-js'
import { api, apiPost, ApiError } from './api'
import { t } from '@/i18n'
import { useToast } from './toast'
import type { Provider } from '@/types/domain'

export interface DeviceFlowState {
  verificationUri: string
  verificationUriComplete?: string
  userCode?: string
  deviceCode?: string
  nonce?: string
  codeVerifier?: string
  machineId?: string
  extraData?: Record<string, unknown>
  expiresIn?: number
  interval?: number
  state?: string
  isManualPaste?: boolean
}

export interface OAuthFlowDeps {
  /** 当前账号所属供应商 ID；为空表示尚未解析出连接。 */
  providerId: () => string | undefined
  /** 当前选中账号，重试时沿用其优先级与名称。 */
  account: () => Provider | null
  /** 令牌导入时回填的账号名兜底。 */
  accountName: () => string
  /** 提示文案里的供应商显示名。 */
  displayName: () => string
  /** 授权成功后刷新账号列表与详情。 */
  onAuthorized: () => Promise<unknown> | void
}

/**
 * 设备码 / 网页授权码 / 令牌导入三条 OAuth 生命周期的状态与轮询。
 * 中止句柄与定时器归本钩子持有，组件卸载时统一清理。
 */
export function useOAuthFlow(deps: OAuthFlowDeps) {
  const toast = useToast()
  const [deviceFlow, setDeviceFlow] = createSignal<DeviceFlowState | null>(null)
  const [manualCode, setManualCode] = createSignal('')
  const [manualExchanging, setManualExchanging] = createSignal(false)
  const [devicePolling, setDevicePolling] = createSignal(false)
  const [deviceError, setDeviceError] = createSignal('')
  const [copiedCode, setCopiedCode] = createSignal(false)
  const [isImportFlow, setIsImportFlow] = createSignal(false)
  const [importTokenText, setImportTokenText] = createSignal('')
  const [importingToken, setImportingToken] = createSignal(false)
  const [deviceFlowMeta, setDeviceFlowMeta] = createSignal<{ priority: number; name: string } | null>(null)
  let pollTimer: number | undefined
  let oauthController: AbortController | undefined

  function abortDeviceFlow() {
    oauthController?.abort()
    oauthController = undefined
    clearInterval(pollTimer)
    pollTimer = undefined
  }

  onCleanup(abortDeviceFlow)

  async function startDeviceFlow(targetPriority?: number, targetName?: string) {
    const p = deps.providerId()
    if (!p) return
    const prio = targetPriority ?? deviceFlowMeta()?.priority ?? deps.account()?.priority ?? 0
    const accName = targetName ?? deviceFlowMeta()?.name ?? deps.account()?.name ?? ''
    // Capture retry metadata before resetting; only this lifetime may update the flow.
    cancelDeviceFlow()
    oauthController = new AbortController()
    const { signal } = oauthController
    setDeviceFlowMeta({ priority: prio, name: accName })
    setDeviceError('')
    setDevicePolling(true)
    try {
      // 先查询该供应商真实支持的 OAuth 流类型
      const statusRes = (await api(`/api/oauth/${p}/status`, { signal, silent: true })) as {
        flowType?: string
        connections?: Array<{ id: string; expiresAt?: string; updatedAt?: string }>
      }
      if (signal.aborted) return
      const flowType = statusRes?.flowType
      const initialConnMap = new Map((statusRes?.connections || []).map(c => [c.id, `${c.expiresAt || ''}:${c.updatedAt || ''}`]))

      // 1. 令牌导入流 (如 Cursor 等)
      if (flowType === 'import_token') {
        setDevicePolling(false)
        setIsImportFlow(true)
        return
      }

      // 2. 网页授权码 / PKCE 流（如 Antigravity, Claude 等）
      if (flowType === 'authorization_code_pkce' || flowType === 'authorization_code') {
        const callbackUri = `${window.location.origin}/api/oauth/${p}/callback`
        const authRes = (await api(`/api/oauth/${p}/authorize?redirect_uri=${encodeURIComponent(callbackUri)}&priority=${prio}&name=${encodeURIComponent(accName)}`, { signal, silent: true })) as {
          authorizeUrl: string
          state: string
        }
        if (signal.aborted) return
        if (authRes?.authorizeUrl) {
          window.open(authRes.authorizeUrl, '_blank')
          toast.info(t('toast.oauthWindowOpened', { name: deps.displayName() }))
          if (p === 'antigravity') {
            setDeviceFlow({
              verificationUri: authRes.authorizeUrl,
              state: authRes.state,
              isManualPaste: true,
            })
          }
        }
        clearInterval(pollTimer)
        pollTimer = window.setInterval(async () => {
          try {
            const pollStatus = (await api(`/api/oauth/${p}/status`, { signal, silent: true })) as {
              connections?: Array<{ id: string; expiresAt?: string; updatedAt?: string }>
            }
            if (signal.aborted) return
            const currentConns = pollStatus?.connections || []
            const isFinished = currentConns.some(c => {
              if (!initialConnMap.has(c.id)) return true
              const oldSnap = initialConnMap.get(c.id) || ''
              const curSnap = `${c.expiresAt || ''}:${c.updatedAt || ''}`
              return curSnap !== oldSnap
            })
            if (isFinished) {
              cancelDeviceFlow()
              toast.success(t('toast.oauthSuccess', { name: deps.displayName() }))
              await deps.onAuthorized()
            }
          } catch {
            // 忽略轮询偶发网络错误与被中止的请求
          }
        }, 2500)
        return
      }

      // 3. 设备码流（Device Code Flow，如 GitHub, Kimi, Qoder, X.AI 等）
      const raw = (await apiPost(`/api/oauth/${p}/device-code`, undefined, { signal, silent: true })) as Record<string, unknown>
      if (signal.aborted) return
      const res: DeviceFlowState = {
        verificationUri: ((raw.verificationUri || raw.verification_uri || '') as string),
        verificationUriComplete: ((raw.verificationUriComplete || raw.verification_uri_complete || '') as string),
        userCode: (raw.userCode || raw.user_code) as string | undefined,
        deviceCode: (raw.deviceCode || raw.device_code) as string | undefined,
        nonce: raw.nonce as string | undefined,
        codeVerifier: (raw.codeVerifier || raw.code_verifier) as string | undefined,
        machineId: (raw.machineId || raw.machine_id) as string | undefined,
        extraData: (raw.extraData || raw.extra_data) as Record<string, unknown> | undefined,
        expiresIn: (raw.expiresIn || raw.expires_in) as number | undefined,
        interval: (raw.interval || 5) as number,
      }
      setDeviceFlow(res)
      const targetUrl = res.verificationUriComplete || res.verificationUri
      if (targetUrl) {
        window.open(targetUrl, '_blank')
      }

      clearInterval(pollTimer)
      let inFlight = false
      const intervalMs = res.interval ? res.interval * 1000 : 2500
      pollTimer = window.setInterval(async () => {
        if (inFlight) return
        inFlight = true
        try {
          const pollRes = (await apiPost(`/api/oauth/${p}/device-code/poll`, {
            deviceCode: res.deviceCode,
            nonce: res.nonce,
            codeVerifier: res.codeVerifier,
            machineId: res.machineId,
            extraData: res.extraData,
            name: accName || undefined,
            priority: prio,
          }, { signal, silent: true })) as { success?: boolean; error?: string; pending?: boolean; connection?: Provider }
          if (signal.aborted) return
          if (pollRes?.success) {
            cancelDeviceFlow()
            toast.success(t('toast.oauthSuccess', { name: deps.displayName() }))
            await deps.onAuthorized()
          } else if (pollRes?.error && !pollRes?.pending) {
            clearInterval(pollTimer)
            pollTimer = undefined
            setDevicePolling(false)
            setDeviceError(pollRes.error)
          }
        } catch (e: unknown) {
          if (signal.aborted) return
          if (e instanceof ApiError && e.status >= 400 && e.status < 500 && e.status !== 408 && e.status !== 429) {
            clearInterval(pollTimer)
            pollTimer = undefined
            setDevicePolling(false)
            setDeviceError(e.message)
          }
        } finally {
          inFlight = false
        }
      }, intervalMs)
    } catch (e: unknown) {
      if (signal.aborted) return
      setDevicePolling(false)
      const msg = e instanceof Error ? e.message : String(e)
      setDeviceError(msg)
      toast.error(t('toast.oauthFailed', { error: msg }))
    }
  }

  async function handleImportToken() {
    const p = deps.providerId()
    if (!p) return
    const token = importTokenText().trim()
    if (!token) {
      toast.error(t('toast.tokenRequired'))
      return
    }
    const signal = oauthController?.signal
    setImportingToken(true)
    try {
      await apiPost(`/api/oauth/${p}/import`, {
        accessToken: token,
        name: deviceFlowMeta()?.name || deps.accountName().trim() || undefined,
        priority: deviceFlowMeta()?.priority ?? 0,
      }, { signal, silent: true })
      if (signal?.aborted) return
      toast.success(t('toast.tokenImportSuccess', { name: deps.displayName() }))
      cancelDeviceFlow()
      await deps.onAuthorized()
    } catch (e: unknown) {
      if (signal?.aborted) return
      const msg = e instanceof Error ? e.message : String(e)
      setDeviceError(msg)
      toast.error(t('toast.tokenImportFailed', { error: msg }))
    } finally {
      if (!signal?.aborted) setImportingToken(false)
    }
  }

  /** 关闭错误提示条：只清文案，不动轮询生命周期。 */
  function clearDeviceError() {
    setDeviceError('')
  }

  function cancelDeviceFlow() {
    abortDeviceFlow()
    setDevicePolling(false)
    setDeviceFlow(null)
    setDeviceFlowMeta(null)
    setDeviceError('')
    setIsImportFlow(false)
    setImportTokenText('')
    setManualCode('')
    setManualExchanging(false)
    setImportingToken(false)
  }

  async function handleManualSubmit() {
    const raw = manualCode().trim()
    const p = deps.providerId()
    if (!raw || !p) return
    const signal = oauthController?.signal
    setManualExchanging(true)
    setDeviceError('')
    try {
      let code = raw
      let state = deviceFlow()?.state || ''
      if (raw.includes('?') || raw.includes('&')) {
        try {
          const url = new URL(raw.startsWith('http') ? raw : `http://dummy/?${raw}`)
          const c = url.searchParams.get('code')
          const s = url.searchParams.get('state')
          if (c) code = c
          if (s) state = s
        } catch { /* 粘贴内容不是 URL，按原始 code 处理 */ }
      }
      await apiPost(`/api/oauth/${p}/exchange`, {
        code,
        state,
        redirectUri: p === 'antigravity' ? 'http://127.0.0.1:51121/oauth-callback' : undefined,
        name: deviceFlowMeta()?.name || undefined,
        priority: deviceFlowMeta()?.priority ?? 0,
      }, { signal, silent: true })
      if (signal?.aborted) return
      toast.success(t('toast.oauthSuccess', { name: deps.displayName() }))
      cancelDeviceFlow()
      await deps.onAuthorized()
    } catch (e: unknown) {
      if (signal?.aborted) return
      const msg = e instanceof Error ? e.message : String(e)
      setDeviceError(msg)
      toast.error(msg)
    } finally {
      if (!signal?.aborted) setManualExchanging(false)
    }
  }

  return {
    deviceFlow,
    manualCode,
    setManualCode,
    manualExchanging,
    devicePolling,
    deviceError, clearDeviceError,
    copiedCode,
    setCopiedCode,
    isImportFlow,
    importTokenText,
    setImportTokenText,
    importingToken,
    startDeviceFlow,
    cancelDeviceFlow,
    handleImportToken,
    handleManualSubmit,
  }
}
