import { type Component, For, Show, createSignal, createResource, createEffect, onMount, onCleanup } from 'solid-js'
import { A, useParams, useNavigate } from '@solidjs/router'
import { useGatewayStore } from '@/stores/gateway'
import { api, apiPost } from '@/lib/api'
import { useToast } from '@/lib/toast'
import { useI18n } from '@/i18n'
import type { Provider, ProviderModel } from '@/types/domain'
import { Card, Badge, Button, Input, Toggle, Field, Empty, Skeleton, Select, Modal, Alert, PageHeader, SegmentedControl, ProviderAvatar, IconBulb, IconCheck, IconClose, IconLock, IconEdit, IconClipboard, IconZap, confirm } from '@/components/ui'

const ProviderDetail: Component = () => {
  const params = useParams<{ id: string }>()
  const navigate = useNavigate()
  const store = useGatewayStore()
  const toast = useToast()
  const { t } = useI18n()
  const [conn, setConn] = createSignal<Provider | null>(null)
  const [loading, setLoading] = createSignal(true)
  const [notFound, setNotFound] = createSignal(false)
  const [saving, setSaving] = createSignal(false)
  const [testing, setTesting] = createSignal(false)
  const [tab, setTab] = createSignal<'overview' | 'models' | 'chat'>('overview')
  // Chat 测试状态
  const [selectedModel, setSelectedModel] = createSignal('')
  const [prompt, setPrompt] = createSignal('')
  const [chatBusy, setChatBusy] = createSignal(false)
  const [chatHistory, setChatHistory] = createSignal<Array<{ role: string; content: string; servedModel?: string }>>([])
  const [chatErr, setChatErr] = createSignal('')
  const [modelSearch, setModelSearch] = createSignal('')
  const [newCustomModelId, setNewCustomModelId] = createSignal('')
  let chatBoxRef: HTMLDivElement | undefined

  createEffect(() => {
    chatHistory()
    if (chatBoxRef) {
      const timer = setTimeout(() => {
        if (chatBoxRef) chatBoxRef.scrollTop = chatBoxRef.scrollHeight
      }, 50)
      onCleanup(() => clearTimeout(timer))
    }
  })
  async function sendChat() {
    const text = prompt().trim()
    if (!text || chatBusy() || !conn()) return
    setChatErr('')
    const nextHistory = [...chatHistory(), { role: 'user', content: text }]
    setChatHistory(nextHistory)
    setPrompt('')
    setChatBusy(true)
    try {
      // 如果未指定具体模型，使用该提供商第一个可用模型
      const available = activeModels()
      const targetModel = selectedModel() || available[0]?.id || `${conn()!.provider}/default`
      const fullModel = targetModel.includes('/') ? targetModel : `${conn()!.provider}/${targetModel}`
      const res = await fetch('/v1/chat/completions', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          model: fullModel,
          messages: nextHistory,
        }),
      })
      const servedModel = res.headers.get('x-cyrene-served-model') || ''
      if (!res.ok) {
        const errData = await res.json().catch(() => ({}))
        throw new Error(errData?.error?.message || errData?.error || `${res.status} ${res.statusText}`)
      }
      const r = await res.json()
      const reply = r?.choices?.[0]?.message?.content ?? t('providerDetail.noResponse')
      setChatHistory(h => [...h, { role: 'assistant', content: reply, servedModel: servedModel || r?.model }])
    } catch (e: unknown) {
      let errMsg = t('common.failed')
      if (e instanceof Error) {
        errMsg = e.message
      } else if (typeof e === 'object' && e !== null) {
        errMsg = JSON.stringify(e)
      }
      setChatErr(errMsg)
      setChatHistory(h => h.slice(0, -1))
    } finally {
      setChatBusy(false)
    }
  }

  // 编辑字段
  const [name, setName] = createSignal('')
  const [apiKey, setApiKey] = createSignal('')
  const [baseUrl, setBaseUrl] = createSignal('')
  const [priority, setPriority] = createSignal('0')
  const [showAdvanced, setShowAdvanced] = createSignal(false)
  const [customHeadersText, setCustomHeadersText] = createSignal('')
  const [syncingModels, setSyncingModels] = createSignal(false)

  async function handleSyncModels() {
    const p = conn()?.provider
    if (!p) return
    setSyncingModels(true)
    try {
      await apiPost(`/api/providers/${p}/refresh-models`)
      toast.success(t('toast.syncModelsSuccess'))
      await refetchModels()
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : String(e)
      toast.error(t('toast.syncModelsFailed', { error: msg }))
    } finally {
      setSyncingModels(false)
    }
  }

  // 模型元数据编辑状态
  const [editingModel, setEditingModel] = createSignal<ProviderModel | null>(null)
  const [editDisplayName, setEditDisplayName] = createSignal('')
  const [editContextLength, setEditContextLength] = createSignal('')
  const [editMaxOutput, setEditMaxOutput] = createSignal('')
  const [savingMeta, setSavingMeta] = createSignal(false)
  // ── 多账号管理状态 ──
  const accounts = () =>
    store
      .providers()
      .filter(p => p.provider === conn()?.provider)
      .sort((a, b) => (a.priority ?? 50) - (b.priority ?? 50))

  const regInfo = () => store.registryList().find(r => r.id === conn()?.provider)
  const providerDisplayName = () => regInfo()?.name || conn()?.provider || t('providerDetail.details')

  function getAccountDisplayName(acc?: Provider | null, idx?: number): string {
    if (!acc) return ''
    const rawName = acc.name?.trim()
    const pName = regInfo()?.name?.trim()
    const pId = (acc.provider || conn()?.provider)?.trim()

    // 排除与供应商名称或 ID 完全相同的自动默认命名
    const isGeneric = !rawName || rawName === pName || rawName.toLowerCase() === pId?.toLowerCase()
    if (!isGeneric) {
      return rawName
    }

    // 若存在 OAuth 返回的用户邮箱/身份标识，优先展示
    if (acc.email?.trim()) {
      return acc.email.trim()
    }

    // 否则按账号在当前提供商凭据池中的序号兜底（如 账号 1、账号 2）
    const indexNum = (idx !== undefined && idx >= 0)
      ? idx + 1
      : Math.max(1, accounts().findIndex(a => a.id === acc.id) + 1)
    return t('providerDetail.accountIndex', { index: indexNum })
  }
  function switchAccount(targetId: string) {
    if (!targetId || targetId === conn()?.id) return
    navigate(`/providers/${targetId}`)
  }

  const accountAuthOptions = () => {
    const modes = regInfo()?.authModes || [regInfo()?.authType || 'api-key']
    const opts: Array<{ value: string; label: string }> = [
      { value: 'api-key', label: t('providerDetail.authApiKeyPat') },
    ]
    if (modes.includes('oauth') || regInfo()?.category === 'oauth') {
      opts.push({ value: 'oauth', label: t('providerDetail.authOauthMode') })
    }
    return opts
  }

  const [addAccountOpen, setAddAccountOpen] = createSignal(false)
  const [newAccountName, setNewAccountName] = createSignal('')
  const [newAccountAuthType, setNewAccountAuthType] = createSignal('api-key')
  const [newAccountApiKey, setNewAccountApiKey] = createSignal('')
  const [newAccountPriority, setNewAccountPriority] = createSignal('20')
  const [addingAccount, setAddingAccount] = createSignal(false)

  async function handleAddAccountSubmit() {
    const p = conn()?.provider
    if (!p) return
    const aType = newAccountAuthType()
    if (aType === 'api-key' && !newAccountApiKey().trim()) {
      toast.error(t('toast.requireApiKey'))
      return
    }
    setAddingAccount(true)
    try {
      const added = await store.addProvider({
        provider: p,
        authType: aType,
        name: newAccountName().trim() || t('providerDetail.accountIndex', { index: accounts().length + 1 }),
        priority: Number(newAccountPriority()) || 20,
        data: {
          apiKey: newAccountApiKey().trim() || undefined,
          baseUrl: baseUrl() || undefined,
        },
      })
      await store.loadProvidersOnly()
      setAddAccountOpen(false)
      setNewAccountName('')
      setNewAccountApiKey('')
      if (added?.id) {
        navigate(`/providers/${added.id}`)
      }
    } catch (e: unknown) {
      toast.error(t('toast.addAccountFailed', { error: e instanceof Error ? e.message : 'error' }))
    } finally {
      setAddingAccount(false)
    }
  }

  // ── 设备码 OAuth 工作流状态 ──
  const [deviceFlow, setDeviceFlow] = createSignal<{
    verificationUri: string
    verificationUriComplete?: string
    userCode?: string
    deviceCode?: string
    nonce?: string
    codeVerifier?: string
    machineId?: string
    expiresIn?: number
    interval?: number
  } | null>(null)
  const [devicePolling, setDevicePolling] = createSignal(false)
  const [deviceError, setDeviceError] = createSignal('')
  const [copiedCode, setCopiedCode] = createSignal(false)
  const [isImportFlow, setIsImportFlow] = createSignal(false)
  const [importTokenText, setImportTokenText] = createSignal('')
  const [importingToken, setImportingToken] = createSignal(false)
  let pollTimer: number | undefined

  onCleanup(() => {
    clearInterval(pollTimer)
  })

  async function startDeviceFlow() {
    const p = conn()?.provider
    if (!p) return
    setDeviceError('')
    setDevicePolling(true)

    try {
      // 先查询该供应商真实支持的 OAuth 流类型
      const statusRes = (await api(`/api/oauth/${p}/status`)) as {
        flowType?: string
        connections?: Array<{ id: string; expiresAt?: string; updatedAt?: string }>
      }
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
        const authRes = (await api(`/api/oauth/${p}/authorize?redirect_uri=${encodeURIComponent(callbackUri)}`)) as {
          authorizeUrl: string
          state: string
        }
        if (authRes?.authorizeUrl) {
          window.open(authRes.authorizeUrl, '_blank')
          toast.info(t('toast.oauthWindowOpened', { name: providerDisplayName() }))
        }

        clearInterval(pollTimer)
        pollTimer = window.setInterval(async () => {
          try {
            const pollStatus = (await api(`/api/oauth/${p}/status`)) as {
              connections?: Array<{ id: string; expiresAt?: string; updatedAt?: string }>
            }
            const currentConns = pollStatus?.connections || []
            const isFinished = currentConns.some(c => {
              if (!initialConnMap.has(c.id)) return true
              const oldSnap = initialConnMap.get(c.id) || ''
              const curSnap = `${c.expiresAt || ''}:${c.updatedAt || ''}`
              return curSnap !== oldSnap
            })
            if (isFinished) {
              clearInterval(pollTimer)
              pollTimer = undefined
              setDevicePolling(false)
              toast.success(t('toast.oauthSuccess', { name: providerDisplayName() }))
              await store.loadProvidersOnly()
              await load()
              refetchOAuth()
            }
          } catch {
            // 忽略轮询网络偶发错误
          }
        }, 2500)
        return
      }

      // 2. 设备码流（Device Code Flow，如 GitHub, Kimi, Qoder, X.AI 等）
      const raw = (await apiPost(`/api/oauth/${p}/device-code`)) as Record<string, unknown>
      const res = {
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
          })) as { success?: boolean; error?: string; pending?: boolean; connection?: Provider }
          if (pollRes?.success) {
            clearInterval(pollTimer)
            pollTimer = undefined
            setDevicePolling(false)
            setDeviceFlow(null)
            toast.success(t('toast.oauthSuccess', { name: providerDisplayName() }))
            await store.loadProvidersOnly()
            await load()
            refetchOAuth()
          } else if (pollRes?.error && !pollRes?.pending) {
            clearInterval(pollTimer)
            pollTimer = undefined
            setDevicePolling(false)
            setDeviceError(pollRes.error)
          }
        } catch {
          // 忽略轮询网络偶发错误
        } finally {
          inFlight = false
        }
      }, intervalMs)
    } catch (e: unknown) {
      setDevicePolling(false)
      const msg = e instanceof Error ? e.message : String(e)
      setDeviceError(msg)
      toast.error(t('toast.oauthFailed', { error: msg }))
    }
  }

  async function handleImportToken() {
    const p = conn()?.provider
    if (!p) return
    const token = importTokenText().trim()
    if (!token) {
      toast.error(t('toast.tokenRequired'))
      return
    }
    setImportingToken(true)
    try {
      await apiPost(`/api/oauth/${p}/import`, {
        accessToken: token,
        name: name().trim() || undefined,
      })
      toast.success(t('toast.tokenImportSuccess', { name: providerDisplayName() }))
      cancelDeviceFlow()
      await store.loadProvidersOnly()
      await load()
      refetchOAuth()
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : String(e)
      setDeviceError(msg)
      toast.error(t('toast.tokenImportFailed', { error: msg }))
    } finally {
      setImportingToken(false)
    }
  }

  function cancelDeviceFlow() {
    clearInterval(pollTimer)
    pollTimer = undefined
    setDevicePolling(false)
    setDeviceFlow(null)
    setDeviceError('')
    setIsImportFlow(false)
    setImportTokenText('')
  }
  async function load(idToLoad?: string) {
    const target = idToLoad || params.id
    if (!target) return
    setLoading(true)
    setNotFound(false)
    try {
      const r = (await api(`/api/providers/${target}`)) as Provider | null
      if (!r) {
        setNotFound(true)
        return
      }
      setConn(r)
      const pName = store.registryList().find(item => item.id === r.provider)?.name?.trim()
      const isAuto = !r.name || (pName && r.name.trim() === pName) || r.name.trim().toLowerCase() === r.provider?.trim().toLowerCase()
      setName(isAuto ? '' : (r.name || ''))
      setPriority(String(r.priority ?? 0))
      setApiKey('')
      const d = r.data as { baseUrl?: string; providerSpecificData?: Record<string, unknown> } | undefined
      setBaseUrl(d?.baseUrl || '')
      if (d?.providerSpecificData?.customHeaders) {
        setCustomHeadersText(JSON.stringify(d.providerSpecificData.customHeaders, null, 2))
      } else {
        setCustomHeadersText('')
      }
    } catch {
      setNotFound(true)
    } finally {
      setLoading(false)
    }
  }

  // 响应式监听路由 params.id 变更，切换编辑账号时立即热更新数据
  createEffect(() => {
    const currentId = params.id
    if (currentId) {
      load(currentId)
    }
  })

  // 当 registryList 异步就绪后，如当前账号命名为默认供应商名，自动置空输入框以展示占位提示
  createEffect(() => {
    const c = conn()
    const reg = regInfo()
    if (c && reg && c.name && c.name.trim() === reg.name.trim() && name() === c.name) {
      setName('')
    }
  })

  onMount(async () => {
    if (store.providers().length === 0) {
      await store.loadProvidersOnly()
    }
    const hash = window.location.hash
    if (hash.includes('tab=')) {
      const t = hash.split('tab=')[1]?.split('&')[0]
      if (t === 'overview' || t === 'models' || t === 'chat') {
        setTab(t)
      }
    }
  })

  const [modelsData, setModelsData] = createSignal<{
    registryModels?: ProviderModel[]
    customModels?: ProviderModel[]
    isFreeMode?: boolean
    authType?: string
    authModes?: string[]
    defaultHeaders?: Record<string, string>
    hasApiKey?: boolean
  }>({})

  const [models, { refetch: refetchModels }] = createResource(
    () => params.id,
    async id => {
      try {
        const r = await api(`/api/providers/${id}/models`) as {
          registryModels?: ProviderModel[]
          customModels?: ProviderModel[]
          isFreeMode?: boolean
          authType?: string
          authModes?: string[]
          defaultHeaders?: Record<string, string>
          hasApiKey?: boolean
        }
        setModelsData(r)
        return r
      } catch {
        const fallback = { registryModels: [], customModels: [] }
        setModelsData(fallback)
        return fallback
      }
    },
  )

  const activeModels = () =>
    (modelsData().registryModels ?? models()?.registryModels ?? [])
      .concat(modelsData().customModels ?? models()?.customModels ?? [])
      .filter(m => Boolean(m.id) && m.enabled !== false)

  async function toggleModel(modelId: string, currentEnabled: boolean) {
    if (!conn()) return
    const fullModel = `${conn()!.provider}/${modelId}`
    const nextState = !currentEnabled

    // 立即乐观更新本地响应式状态
    setModelsData(prev => {
      const updateList = (list?: ProviderModel[]) =>
        (list ?? []).map(m => (m.id === modelId || m.name === modelId ? { ...m, enabled: nextState } : m))
      return {
        ...prev,
        registryModels: updateList(prev.registryModels),
        customModels: updateList(prev.customModels),
      }
    })

    try {
      await store.setModelDisabled(fullModel, !nextState)
    } catch {
      refetchModels()
    }
  }
  // ── 模型连通性单测与批量运维 ──
  const [testingModels, setTestingModels] = createSignal<Record<string, boolean>>({})
  const [modelTestResults, setModelTestResults] = createSignal<Record<string, { ok: boolean; latency?: string; error?: string }>>({})
  const [testingAll, setTestingAll] = createSignal(false)
  const [testAllProgress, setTestAllProgress] = createSignal<{ current: number; total: number } | null>(null)
  const [showCustomModels, setShowCustomModels] = createSignal(false)
  const customCount = () => (modelsData().customModels ?? models()?.customModels ?? []).length
  createEffect(() => {
    if (customCount() > 0) {
      setShowCustomModels(true)
    }
  })

  const allDisplayModels = () =>
    (modelsData().registryModels ?? models()?.registryModels ?? [])
      .concat(modelsData().customModels ?? models()?.customModels ?? [])
  const enabledModelsCount = () => allDisplayModels().filter(m => m.enabled !== false).length
  const disabledModelsCount = () => allDisplayModels().filter(m => m.enabled === false).length
  async function testSingleModel(modelId: string) {
    if (!conn() || testingModels()[modelId]) return
    const p = conn()!.provider
    const fullModel = modelId.includes('/') ? modelId : `${p}/${modelId}`
    setTestingModels(prev => ({ ...prev, [modelId]: true }))
    try {
      const res = await store.testModel(fullModel, params.id)
      setModelTestResults(prev => ({
        ...prev,
        [modelId]: {
          ok: res.ok,
          latency: res.latency,
          error: res.error,
        },
      }))
      if (res.ok) {
        toast.success(t('toast.modelTestPassed', { id: modelId, latency: res.latency || 'ok' }))
      } else {
        toast.error(t('toast.modelTestFailed', { id: modelId, error: res.error || 'unavailable' }))
      }
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : t('providerDetail.testRequestError')
      setModelTestResults(prev => ({ ...prev, [modelId]: { ok: false, error: msg } }))
      toast.error(t('toast.modelTestFailed', { id: modelId, error: msg }))
    } finally {
      setTestingModels(prev => ({ ...prev, [modelId]: false }))
    }
  }

  async function handleEnableAll() {
    if (!conn()) return
    const p = conn()!.provider
    const toEnable = allDisplayModels().filter(m => m.enabled === false)
    if (toEnable.length === 0) {
      toast.info(t('toast.allModelsEnabled'))
      return
    }
    setModelsData(prev => ({
      ...prev,
      registryModels: (prev.registryModels ?? []).map(m => ({ ...m, enabled: true })),
      customModels: (prev.customModels ?? []).map(m => ({ ...m, enabled: true })),
    }))
    try {
      await Promise.all(toEnable.map(m => store.setModelDisabled(`${p}/${m.id || m.name}`, false)))
      toast.success(t('toast.batchEnableSuccess', { count: toEnable.length }))
    } catch {
      toast.error(t('toast.batchEnableFailed'))
      refetchModels()
    }
  }

  async function handleDisableAll() {
    if (!conn()) return
    const p = conn()!.provider
    const toDisable = allDisplayModels().filter(m => m.enabled !== false)
    if (toDisable.length === 0) {
      toast.info(t('toast.allModelsDisabled'))
      return
    }
    if (!await confirm(t('providerDetail.confirmDisableAll', { count: toDisable.length }))) return
    setModelsData(prev => ({
      ...prev,
      registryModels: (prev.registryModels ?? []).map(m => ({ ...m, enabled: false })),
      customModels: (prev.customModels ?? []).map(m => ({ ...m, enabled: false })),
    }))
    try {
      await Promise.all(toDisable.map(m => store.setModelDisabled(`${p}/${m.id || m.name}`, true)))
      toast.success(t('toast.batchDisableSuccess', { count: toDisable.length }))
    } catch {
      toast.error(t('toast.batchDisableFailed'))
      refetchModels()
    }
  }

  async function handleTestAll() {
    if (!conn() || testingAll()) return
    const p = conn()!.provider
    const seen = new Set<string>()
    const targetList = allDisplayModels().filter(m => {
      const k = m.id || m.name
      if (!k || seen.has(k)) return false
      seen.add(k)
      return true
    })
    if (targetList.length === 0) {
      toast.info(t('toast.noTestableModels'))
      return
    }
    setTestingAll(true)
    setTestAllProgress({ current: 0, total: targetList.length })

    let completed = 0
    let passed = 0
    let failed = 0

    const concurrency = 3
    let index = 0

    async function worker() {
      while (index < targetList.length) {
        const cur = targetList[index++]
        const modelId = cur.id || cur.name || ''
        if (!modelId) continue
        setTestingModels(prev => ({ ...prev, [modelId]: true }))
        try {
          const fullModel = modelId.includes('/') ? modelId : `${p}/${modelId}`
          const res = await store.testModel(fullModel, params.id)
          setModelTestResults(prev => ({
            ...prev,
            [modelId]: { ok: res.ok, latency: res.latency, error: res.error },
          }))
          if (res.ok) passed++
          else failed++
        } catch (e: unknown) {
          failed++
          const msg = e instanceof Error ? e.message : t('providerDetail.testUnexpected')
          setModelTestResults(prev => ({ ...prev, [modelId]: { ok: false, error: msg } }))
        } finally {
          completed++
          setTestingModels(prev => ({ ...prev, [modelId]: false }))
          setTestAllProgress({ current: completed, total: targetList.length })
        }
      }
    }

    const workers = Array.from({ length: Math.min(concurrency, targetList.length) }, () => worker())
    await Promise.all(workers)

    setTestingAll(false)
    setTestAllProgress(null)
    toast.success(t('toast.testAllDone', { passed, failed }))
  }

  const failedModelsList = () => {
    const results = modelTestResults()
    const seen = new Set<string>()
    return allDisplayModels().filter(m => {
      const mid = m.id || m.name
      if (!mid || seen.has(mid)) return false
      seen.add(mid)
      return results[mid] && !results[mid].ok && m.enabled !== false
    })
  }

  async function handleDisableFailed() {
    if (!conn()) return
    const p = conn()!.provider
    const failedList = failedModelsList()
    if (failedList.length === 0) {
      toast.info(t('toast.noFailedModels'))
      return
    }
    if (!await confirm(t('providerDetail.confirmDisableFailed', { count: failedList.length }))) return
    const failedIds = new Set(failedList.map(m => m.id || m.name).filter((id): id is string => Boolean(id)))
    setModelsData(prev => ({
      ...prev,
      registryModels: (prev.registryModels ?? []).map(m => failedIds.has(m.id || m.name || '') ? { ...m, enabled: false } : m),
      customModels: (prev.customModels ?? []).map(m => failedIds.has(m.id || m.name || '') ? { ...m, enabled: false } : m),
    }))
    try {
      await Promise.all(failedList.map(m => store.setModelDisabled(`${p}/${m.id || m.name}`, true)))
      toast.success(t('toast.batchDisableSuccess', { count: failedList.length }))
    } catch {
      toast.error(t('toast.batchDisableFailed'))
      refetchModels()
    }
  }
  const [, { refetch: refetchOAuth }] = createResource(
    () => conn()?.provider,
    async p => {
      if (!p) return null
      try {
        return await api(`/api/oauth/${p}/status`) as {
          flowType?: string
          connections?: Array<{ id: string }>
        } | null
      } catch { return null }
    },
  )

  function startEditModel(m: ProviderModel) {
    setEditingModel(m)
    setEditDisplayName(m.name || m.id || '')
    setEditContextLength(m.contextLength ? String(m.contextLength) : '')
    setEditMaxOutput(m.maxOutputTokens ? String(m.maxOutputTokens) : '')
  }

  async function handleSaveModelMeta() {
    const m = editingModel()
    if (!m || !m.id) return
    setSavingMeta(true)
    try {
      await store.saveProviderModelMeta(params.id, {
        id: m.id,
        displayName: editDisplayName().trim(),
        contextLength: Number(editContextLength()) || 0,
        maxOutputTokens: Number(editMaxOutput()) || 0,
      })
      await refetchModels()
      setEditingModel(null)
    } catch (e) {
      console.error('save meta failed:', e)
    } finally {
      setSavingMeta(false)
    }
  }

  async function handleResetModelMeta() {
    const m = editingModel()
    if (!m || !m.id) return
    setSavingMeta(true)
    try {
      await store.resetProviderModelMeta(params.id, m.id)
      await refetchModels()
      setEditingModel(null)
    } catch (e) {
      console.error('reset meta failed:', e)
    } finally {
      setSavingMeta(false)
    }
  }

  async function save() {
    setSaving(true)
    try {
      const current = conn()
      const patch: Partial<Provider> & { data?: Record<string, unknown> } = { name: name(), priority: Number(priority()) || 0 }
      const currData = (current?.data || {}) as Record<string, unknown>
      const nextData: Record<string, unknown> = { ...currData }
      if (apiKey().trim()) {
        nextData.apiKey = apiKey().trim()
        if (current?.authType === 'none') {
          patch.authType = 'api-key'
        }
      }
      if (baseUrl().trim()) {
        nextData.baseUrl = baseUrl().trim()
      } else {
        delete nextData.baseUrl
      }

      // 处理高级协议自定义请求头 (customHeaders)
      const hStr = customHeadersText().trim()
      if (hStr) {
        try {
          const parsed = JSON.parse(hStr)
          const psData = ((currData.providerSpecificData as Record<string, unknown>) || {})
          nextData.providerSpecificData = { ...psData, customHeaders: parsed }
        } catch {
          // invalid JSON, do not override
        }
      } else if (currData.providerSpecificData) {
        const psData = { ...(currData.providerSpecificData as Record<string, unknown>) }
        delete psData.customHeaders
        nextData.providerSpecificData = psData
      }

      patch.data = nextData
      await store.updateProvider(params.id, patch)
      await load()
      setApiKey('')
    } catch (e: unknown) {
      console.error('[detail] save failed:', e)
    } finally {
      setSaving(false)
    }
  }

  async function runTest() {
    setTesting(true)
    try {
      const r = await store.testProvider(params.id)
      if (r?.ok) {
        toast.success(t('providerDetail.testConnOk', { latency: r.latencyMs ?? 0 }))
      } else {
        toast.error(t('providerDetail.testConnFailed', {
          error: r?.error || t('providerDetail.testUpstreamNoResponse'),
          code: r?.code ? ` (HTTP ${r.code})` : '',
        }))
      }
    } catch (e: unknown) {
      toast.error(e instanceof Error ? e.message : t('providerDetail.testConnRequestFailed'))
    } finally {
      setTesting(false)
    }
  }

  async function addCustomModel(modelId: string, modelName: string) {
    if (!modelId.trim()) return
    try {
      await store.addProviderModel(params.id, { id: modelId.trim(), name: modelName.trim() || modelId.trim() })
      toast.success(t('providerDetail.customModelAdded', { id: modelId.trim() }))
      refetchModels()
    } catch (e: unknown) {
      toast.error(e instanceof Error ? e.message : t('common.addFailed'))
    }
  }

  async function removeCustomModel(modelId: string) {
    const ok = await confirm({
      title: t('providerDetail.deleteModelTitle'),
      message: t('providerDetail.deleteModelMessage', { id: modelId }),
      variant: 'danger',
    })
    if (!ok) return
    try {
      await store.deleteProviderModel(params.id, modelId)
      toast.success(t('providerDetail.modelDeleted', { id: modelId }))
      refetchModels()
    } catch (e: unknown) {
      toast.error(e instanceof Error ? e.message : t('providerDetail.deleteModelFailed'))
    }
  }

  return (
    <div class="space-y-5 stagger">
      <Show when={!loading() && conn()}>
        {c => (
          <PageHeader
            title={
              <div class="flex items-center gap-2.5">
                <A href="/providers" class="text-xs text-faint hover:text-accent inline-flex items-center gap-1 font-normal mr-1">
                  ← {t('providerDetail.back')}
                </A>
                <ProviderAvatar
                  provider={c().provider}
                  name={providerDisplayName()}
                  color={regInfo()?.color}
                  size="sm"
                  class="shrink-0"
                />
                <span>{providerDisplayName()}</span>
              </div>
            }
            badge={
              <div class="flex items-center gap-2 flex-wrap">
                <Badge tone={c().isActive ? 'green' : 'gray'}>{c().isActive ? t('common.enabled') : t('common.disabled')}</Badge>
                <Badge tone="blue">{c().authType === 'api-key' ? 'API Key' : c().authType === 'oauth' ? 'OAuth' : c().authType}</Badge>
                <span class="text-xs text-faint font-mono px-1.5 py-0.5 rounded bg-black/5 dark:bg-white/5 border border-black/10 dark:border-white/10">
                  {c().provider}
                </span>
                <span class="text-xs text-faint">{t('providerDetail.accountCount', { count: accounts().length })}</span>
              </div>
            }
            actions={
              <div class="flex items-center gap-3">
                <Button size="sm" variant="secondary" loading={testing()} onClick={runTest}>{t('providerDetail.testConnection')}</Button>
                <Toggle checked={c().isActive} onChange={() => { store.toggleProvider(c()); load() }} />
              </div>
            }
          >
            {/* 顶栏控制区：Tab 切换栏与快捷操作工具条融合吸顶 */}
            <div class="pt-2.5 border-t border-subtle/40 flex flex-wrap items-center justify-between gap-3">
              <SegmentedControl<'overview' | 'models' | 'chat'>
                value={tab()}
                onChange={setTab}
                options={[
                  { value: 'overview', label: t('providerDetail.tabs.overview') },
                  { value: 'models', label: `${t('providerDetail.tabs.models')} (${(modelsData().registryModels ?? models()?.registryModels ?? []).length})` },
                  { value: 'chat', label: t('providerDetail.tabs.chat') },
                ]}
              />

              {/* 仅在 models 视图下激活的快捷操作工具组 */}
              <Show when={tab() === 'models'}>
                <div class="animate-slide-up flex flex-wrap items-center gap-2 text-xs">
                  <div class="flex items-center gap-2">
                    <Input
                      class="w-40 sm:w-52!"
                      size="sm"
                      value={modelSearch()}
                      onInput={setModelSearch}
                      placeholder={t('providerDetail.modelSearchPlaceholder')}
                    />
                    <Button
                      size="sm"
                      variant="secondary"
                      loading={syncingModels()}
                      onClick={handleSyncModels}
                      title={t('providerDetail.syncModelsTitle')}
                    >
                      {t('providerDetail.syncUpstreamModels')}
                    </Button>
                  </div>
                  <div class="hidden xl:block h-3.5 w-px bg-subtle/60 mx-0.5" />
                  <div class="flex items-center gap-1.5 flex-wrap">
                    {/* 状态联动批量启停控制器（带实时数字反馈与状态色） */}
                    <div class="inline-flex rounded-lg p-0.5 bg-black/5 dark:bg-white/8 border border-subtle text-xs select-none">
                      <button
                        type="button"
                        class={`px-2.5 py-1 rounded-md transition-all flex items-center gap-1 font-medium ${
                          disabledModelsCount() === 0
                            ? 'opacity-40 cursor-not-allowed text-faint'
                            : 'text-foreground hover:text-success hover:bg-success/10 cursor-pointer'
                        }`}
                        disabled={disabledModelsCount() === 0}
                        onClick={handleEnableAll}
                        title={disabledModelsCount() === 0 ? t('toast.allModelsEnabled') : t('providerDetail.tooltipEnableAll')}
                      >
                        <span class={`w-1.5 h-1.5 rounded-full ${disabledModelsCount() === 0 ? 'bg-success' : 'bg-accent'}`} />
                        <span>{t('providerDetail.enableAllModels')}</span>
                        <Show when={disabledModelsCount() > 0}>
                          <span class="font-mono text-[10px] opacity-75">({disabledModelsCount()})</span>
                        </Show>
                      </button>
                      <button
                        type="button"
                        class={`px-2.5 py-1 rounded-md transition-all flex items-center gap-1 font-medium ${
                          enabledModelsCount() === 0
                            ? 'opacity-40 cursor-not-allowed text-faint'
                            : 'text-faint hover:text-danger hover:bg-danger/10 cursor-pointer'
                        }`}
                        disabled={enabledModelsCount() === 0}
                        onClick={handleDisableAll}
                        title={enabledModelsCount() === 0 ? t('toast.allModelsDisabled') : t('providerDetail.tooltipDisableAll')}
                      >
                        <span class={`w-1.5 h-1.5 rounded-full ${enabledModelsCount() === 0 ? 'bg-faint' : 'bg-danger/70'}`} />
                        <span>{t('providerDetail.disableAllModels')}</span>
                        <Show when={enabledModelsCount() > 0 && disabledModelsCount() > 0}>
                          <span class="font-mono text-[10px] opacity-75">({enabledModelsCount()})</span>
                        </Show>
                      </button>
                    </div>
                    <Button
                      size="sm"
                      variant="secondary"
                      loading={testingAll()}
                      onClick={handleTestAll}
                      title={t('providerDetail.tooltipTestAll')}
                    >
                      <IconZap size={12} class="mr-1 inline" />
                      {t('providerDetail.testAllModels')}
                    </Button>
                    <Show when={testAllProgress()}>
                      {prog => (
                        <span class="text-[11px] text-faint font-mono">
                          {t('providerDetail.progress', { current: prog().current, total: prog().total })}
                        </span>
                      )}
                    </Show>
                    <Show when={failedModelsList().length > 0}>
                      <Button
                        size="sm"
                        variant="danger"
                        onClick={handleDisableFailed}
                        title={t('providerDetail.tooltipDisableFailed')}
                      >
                        {t('providerDetail.disableFailedModels')} ({failedModelsList().length})
                      </Button>
                    </Show>
                  </div>
                </div>
              </Show>

              {/* 在 overview 视图下：右侧展示轻量指示 */}
              <Show when={tab() === 'overview'}>
                <div class="animate-fade-in text-xs text-faint hidden sm:flex items-center gap-2">
                  <span class="inline-block w-1.5 h-1.5 rounded-full bg-accent animate-pulse" />
                  <span>{t('providerDetail.accountsAndPool')}</span>
                </div>
              </Show>

              {/* 在 chat 视图下：右侧展示轻量指示 */}
              <Show when={tab() === 'chat'}>
                <div class="animate-fade-in text-xs text-faint hidden sm:flex items-center gap-2">
                  <span class="inline-block w-1.5 h-1.5 rounded-full bg-success animate-pulse" />
                  <span>{t('providerDetail.tabs.chat')}</span>
                </div>
              </Show>
            </div>
          </PageHeader>
        )}
      </Show>

      <Show when={loading()}>
        <Card class="p-6 space-y-3"><Skeleton class="h-6 w-48" /><Skeleton class="h-4 w-full" /><Skeleton class="h-4 w-2/3" /></Card>
      </Show>

      <Show when={notFound()}>
        <Card class="p-6"><Empty message={t('providerDetail.connectionNotFound')} /></Card>
      </Show>

      <Show when={!loading() && conn()}>
        {c => (
          <div class="space-y-5">
            {/* 账号与连接配置 */}
            <Show when={tab() === 'overview'}>
              <div class="grid lg:grid-cols-12 gap-5 items-start animate-fade-in">
                {/* 左侧 (5 cols)：多账号与调度看板 (带独立滚动区，不会被挤出视野) */}
                <div class="lg:col-span-5 space-y-3">
                  <Card class="p-4 space-y-3">
                    <div class="flex items-center justify-between gap-2 pb-2.5 border-b border-subtle">
                      <div>
                        <h3 class="text-sm font-semibold flex items-center gap-2">
                          <span>{t('providerDetail.accountsAndPool')}</span>
                          <Badge tone="blue">{t('providerDetail.accountCount', { count: accounts().length })}</Badge>
                        </h3>
                        <p class="text-[11px] text-faint mt-0.5">
                          {t('providerDetail.schedulingHint')}
                        </p>
                      </div>
                      <Button
                        size="sm"
                        variant="primary"
                        onClick={() => {
                          const nextNum = accounts().length + 1
                          setNewAccountName(t('providerDetail.accountIndex', { index: nextNum }))
                          setNewAccountAuthType('api-key')
                          setNewAccountApiKey('')
                          setNewAccountPriority(String((Number(priority()) || 0) + 10))
                          setAddAccountOpen(true)
                        }}
                      >
                        {t('providerDetail.addAccount')}
                      </Button>
                    </div>

                    {/* 账号列表滚动容器 */}
                    <div class="max-h-[360px] lg:max-h-[calc(100vh-340px)] overflow-y-auto pr-1 space-y-2">
                      <For each={accounts()}>
                        {(acc, idx) => {
                          const isCurrent = () => acc.id === c().id
                          const cooling = () => !!acc.data?.rateLimitedUntil

                          return (
                            <div
                              role="button"
                              tabIndex={0}
                              aria-label={t('providerDetail.selectAccount', { name: getAccountDisplayName(acc, idx()) })}
                              onClick={e => {
                                if ((e.target as HTMLElement).closest('button, [role="switch"], a, input')) return
                                switchAccount(acc.id)
                              }}
                              onKeyDown={e => {
                                if (e.key === 'Enter' || e.key === ' ') {
                                  if ((e.target as HTMLElement).closest('button, [role="switch"], a, input')) return
                                  e.preventDefault()
                                  switchAccount(acc.id)
                                }
                              }}
                              class={`p-3 rounded-control border transition-all cursor-pointer ${
                                isCurrent()
                                  ? 'border-accent/80 bg-accent/10 shadow-xs ring-1 ring-accent/40'
                                  : acc.isActive
                                  ? 'border-subtle bg-bg-elevated/60 hover:bg-hover hover:border-subtle/80'
                                  : 'border-subtle/40 bg-bg-elevated/20 opacity-60 hover:opacity-100'
                              }`}
                            >
                              <div class="flex items-center justify-between gap-2">
                                <div class="flex items-center gap-2 min-w-0">
                                  <span class={`text-[11px] font-mono font-bold px-1.5 py-0.5 rounded border shrink-0 ${
                                    isCurrent() ? 'bg-accent text-white border-accent' : 'bg-bg text-faint border-subtle'
                                  }`}>
                                    #{idx() + 1}
                                  </span>
                                  <span class="text-xs font-semibold text-foreground truncate">
                                    {getAccountDisplayName(acc, idx())}
                                  </span>
                                  <Show when={acc.email && acc.email !== getAccountDisplayName(acc, idx())}>
                                    <span class="text-[10px] text-faint truncate font-mono">
                                      ({acc.email})
                                    </span>
                                  </Show>
                                  <Show when={isCurrent()}>
                                    <span class="text-[10px] px-1.5 py-0.5 rounded bg-accent/15 text-accent border border-accent/35 font-medium shrink-0">
                                      {t('providerDetail.editingBadge')}
                                    </span>
                                  </Show>
                                </div>

                                <div class="flex items-center gap-1.5 shrink-0">
                                  <Toggle
                                    checked={acc.isActive}
                                    onChange={async () => {
                                      await store.toggleProvider(acc)
                                      await store.loadProvidersOnly()
                                      await load()
                                    }}
                                  />
                                  <Show when={accounts().length > 1}>
                                    <button
                                      type="button"
                                      class="text-muted hover:text-danger text-xs p-1 rounded hover:bg-hover transition-colors"
                                      title={t('providerDetail.deleteAccountTitle2')}
                                      onClick={async (e) => {
                                        e.stopPropagation()
                                        const ok = await confirm({
                                          title: t('providerDetail.deleteAccountTitle'),
                                          message: t('providerDetail.deleteAccountMessage', { name: getAccountDisplayName(acc, idx()) }),
                                          variant: 'danger',
                                        })
                                        if (ok) {
                                          await store.deleteProvider(acc)
                                          await store.loadProvidersOnly()
                                          if (isCurrent()) {
                                            const remaining = accounts().filter(a => a.id !== acc.id)
                                            if (remaining.length > 0) {
                                              navigate(`/providers/${remaining[0].id}`)
                                            } else {
                                              navigate('/providers')
                                            }
                                          }
                                        }
                                      }}
                                    >
                                      <IconClose size={12} />
                                    </button>
                                  </Show>
                                </div>
                              </div>

                              <div class="mt-2.5 flex items-center gap-2 flex-wrap text-xs">
                                <span class="inline-flex items-center justify-center h-5 px-2 text-[10px] font-medium rounded-full text-info bg-info/12 border border-info/30">
                                  {acc.authType === 'api-key' || acc.authType === 'apikey' ? 'API Key' : acc.authType === 'oauth' ? 'OAuth' : acc.authType}
                                </span>
                                <span class={`inline-flex items-center h-5 gap-1.5 text-[10px] px-2 rounded-full border font-mono ${
                                  idx() === 0
                                    ? 'bg-accent/15 border-accent/40 text-accent font-semibold'
                                    : 'bg-black/6 dark:bg-white/8 border-black/12 dark:border-white/15 text-muted'
                                }`}>
                                  <span class={`inline-block w-1.5 h-1.5 rounded-full shrink-0 ${idx() === 0 ? 'bg-accent animate-pulse' : 'bg-faint'}`} />
                                  <span class="leading-none">{t('providerDetail.priorityValue', { value: acc.priority })}</span>
                                  <span class="opacity-70 font-sans leading-none">{idx() === 0 ? t('providerDetail.primaryMark') : t('providerDetail.backupMark')}</span>
                                </span>
                                <Show when={cooling()}>
                                  <Badge tone="amber" class="text-[10px] h-5">{t('providerDetail.coolingBadge')}</Badge>
                                </Show>
                                <Show when={acc.data?.credentialHint}>
                                  <span class="truncate max-w-[140px] text-[11px] text-faint font-mono leading-none" title={String(acc.data?.credentialHint)}>
                                    {String(acc.data?.credentialHint)}
                                  </span>
                                </Show>
                              </div>
                            </div>
                          )
                        }}
                      </For>
                    </div>

                    {/* 容灾与 Fallback 调度说明 */}
                    <div class="p-2.5 rounded-lg bg-black/4 dark:bg-white/6 border border-black/10 dark:border-white/12 text-[11px] text-faint leading-relaxed flex items-start gap-1.5">
                      <IconBulb size={14} class="text-accent shrink-0 mt-0.5" />
                      <div><span class="text-foreground font-medium">{t('providerDetail.dispatchMechanismTitle')}</span>{t('providerDetail.dispatchMechanismDesc')}</div>
                    </div>
                  </Card>
                </div>

                {/* 右侧 (7 cols)：当前正在编辑的账号详情与端点配置 (高度受控，内部滚动，底部按钮绝对吸底可见) */}
                <div class="lg:col-span-7">
                  <Card class="p-5 flex flex-col max-h-[calc(100vh-220px)] min-h-[480px]">
                    <div class="flex items-center justify-between pb-3 border-b border-subtle shrink-0">
                      <div>
                        <div class="text-sm font-semibold flex items-center gap-2">
                          <span>{t('providerDetail.editAccountWith', { name: getAccountDisplayName(c()) })}</span>
                          <Badge tone="blue">{c().authType === 'api-key' ? 'API Key' : c().authType === 'oauth' ? 'OAuth' : c().authType}</Badge>
                        </div>
                        <div class="text-xs text-faint mt-0.5 font-mono flex items-center gap-2">
                          <span>{t('providerDetail.nodeId', { id: c().id })}</span>
                          <Show when={c().email && c().email !== getAccountDisplayName(c())}>
                            <span>{t('providerDetail.authEmail', { email: c().email || '' })}</span>
                          </Show>
                        </div>
                      </div>
                      <Show when={regInfo()?.apiKeyUrl}>
                        <a
                          href={regInfo()!.apiKeyUrl!}
                          target="_blank"
                          rel="noreferrer"
                          class="text-xs text-accent hover:underline inline-flex items-center gap-1"
                        >
                          {t('providerDetail.getOfficialKey')}
                        </a>
                      </Show>
                    </div>

                    {/* 表单内容滚动区 */}
                    <div class="flex-1 overflow-y-auto pr-1 py-3 space-y-4">
                      <Field label={t('providerDetail.accountNameLabel2')} hint={t('providerDetail.accountNameHint2')}>
                        <Input
                          value={name()}
                          onInput={setName}
                          placeholder={t('providerDetail.accountNamePlaceholder', { name: getAccountDisplayName(c()) })}
                        />
                      </Field>

                      <Field label={t('providerDetail.priorityLabel')} hint={t('providerDetail.priorityHint')}>
                        <Input type="number" value={priority()} onInput={setPriority} class="w-32!" />
                      </Field>

                      {/* API Key / 凭据输入 (仅非纯 OAuth 模式显示) */}
                      <Show
                        when={
                          c().authType !== 'oauth' ||
                          (modelsData().authModes?.includes('api-key') && c().authType !== 'oauth')
                        }
                        fallback={
                          <div class="p-3.5 rounded-control bg-accent/10 border border-accent/30 space-y-2 text-xs">
                            <div class="flex items-center justify-between">
                              <span class="font-medium text-accent flex items-center gap-1.5">
                                <IconCheck size={14} /> {t('providerDetail.currentIsOauth')}
                              </span>
                              <Button
                                size="sm"
                                variant="secondary"
                                class="text-accent hover:border-accent"
                                onClick={() => {
                                  setDeviceFlow(null)
                                  setDeviceError('')
                                  startDeviceFlow()
                                }}
                              >
                                {t('providerDetail.reauthorize')}
                              </Button>
                            </div>
                            <p class="text-faint leading-relaxed">
                              {t('providerDetail.oauthManagedHint')}
                            </p>
                          </div>
                        }
                      >
                        <Field
                          label={c().provider === 'qoder' ? t('providerDetail.qoderCredLabel') : t('providerDetail.authCredential')}
                          hint={
                            c().provider === 'qoder'
                              ? t('providerDetail.qoderCredHint')
                              : c().data?.hasApiKey
                              ? t('providerDetail.credKeepHint')
                              : t('providerDetail.credEnterHint')
                          }
                        >
                          <Input
                            type="password"
                            value={apiKey()}
                            onInput={setApiKey}
                            placeholder={
                              c().provider === 'qoder'
                                ? 'pt-... (Personal Access Token)'
                                : c().provider === 'opencode'
                                ? t('providerDetail.opencodeKeyPlaceholder')
                                : 'sk-...'
                            }
                          />
                          <Show when={c().data?.hasApiKey}>
                            <div class="text-[11px] text-success mt-1 flex items-center gap-1">
                              <IconCheck size={12} /> <span>{t('providerDetail.keyConfigured')}</span>
                            </div>
                          </Show>
                        </Field>
                      </Show>

                      <Show when={c().provider.startsWith('custom-') || regInfo()?.category === 'custom'}>
                        <Field
                          label={t('providerDetail.baseUrlRequired')}
                          hint={t('providerDetail.baseUrlHint')}
                        >
                          <Input value={baseUrl()} onInput={setBaseUrl} placeholder="https://api.example.com/v1" />
                        </Field>
                      </Show>

                      {/* 高级协议与客户端指纹覆盖 */}
                      <div class="pt-2 border-t border-subtle">
                        <button
                          type="button"
                          class="text-xs text-muted hover:text-foreground flex items-center gap-1.5 py-1 font-medium transition-colors"
                          onClick={() => setShowAdvanced(!showAdvanced())}
                        >
                          <span>{showAdvanced() ? '▼' : '▶'}</span>
                          <span>{t('providerDetail.customHeadersOverride')}</span>
                        </button>

                        <Show when={showAdvanced()}>
                          <div class="mt-3 p-3.5 rounded-control bg-bg-elevated/50 border border-subtle/70 space-y-3 text-xs">
                            <Show when={modelsData().defaultHeaders && Object.keys(modelsData().defaultHeaders!).length > 0}>
                              <div>
                                <div class="text-faint mb-1.5 font-medium">{t('providerDetail.defaultHeaderHint')}</div>
                                <div class="bg-bg/80 p-2 rounded border border-subtle font-mono text-[11px] space-y-1">
                                  <For each={Object.entries(modelsData().defaultHeaders!)}>
                                    {([k, v]) => (
                                      <div class="flex gap-2">
                                        <span class="text-accent">{k}:</span>
                                        <span class="text-foreground truncate">{v}</span>
                                      </div>
                                    )}
                                  </For>
                                </div>
                              </div>
                            </Show>

                            <Field
                              label={t('providerDetail.customHeadersLabel')}
                              hint={t('providerDetail.customHeadersHint')}
                            >
                              <textarea
                                rows={4}
                                value={customHeadersText()}
                                onInput={e => setCustomHeadersText(e.currentTarget.value)}
                                placeholder={'{\n  "User-Agent": "MyClient/2.0",\n  "anthropic-version": "2023-06-01"\n}'}
                                class="w-full rounded-control border border-subtle bg-bg/80 px-3 py-2 text-xs font-mono focus-visible:outline-2 focus-visible:outline-ring"
                              />
                            </Field>
                          </div>
                        </Show>
                      </div>

                      <Show when={c().data?.credentialHint}>
                        <div class="text-xs text-faint">
                          {t('providerDetail.credentialHintLabel')}<span class="font-mono">{String(c().data?.credentialHint ?? '')}</span>
                          <Show when={c().data?.hasAccessToken}> · {t('providerDetail.accessTokenReady')}</Show>
                          <Show when={c().data?.hasRefreshToken}> · {t('providerDetail.refreshTokenReady')}</Show>
                        </div>
                      </Show>
                    </div>

                    {/* 固定吸底的操作栏 */}
                    <div class="flex items-center justify-between pt-3 border-t border-subtle shrink-0">
                      <Button
                        variant="danger"
                        size="sm"
                        onClick={async () => {
                          const ok = await confirm({
                            title: t('providerDetail.deleteAccountTitle'),
                            message: t('providerDetail.deleteAccountSelfMessage', { name: getAccountDisplayName(c()) }),
                            variant: 'danger',
                          })
                          if (ok) {
                            await store.deleteProvider(c())
                            await store.loadProvidersOnly()
                            const remaining = accounts().filter(a => a.id !== c().id)
                            if (remaining.length > 0) {
                              navigate(`/providers/${remaining[0].id}`)
                            } else {
                              navigate('/providers')
                            }
                          }
                        }}
                      >
                        {t('providerDetail.deleteThisAccount')}
                      </Button>
                      <Button variant="primary" size="sm" loading={saving()} onClick={save}>
                        {t('providerDetail.saveCurrentConfig')}
                      </Button>
                    </div>
                  </Card>
                </div>
              </div>
            </Show>

            {/* 模型 */}
            <Show when={tab() === 'models'}>
              <div class="flex flex-col gap-3.5 animate-fade-in">
                <Card class="p-3.5 sm:p-4">
                  <div class="flex items-center justify-between gap-3">
                    <div class="flex items-center gap-2 flex-wrap min-w-0">
                      <h3 class="text-sm font-semibold whitespace-nowrap">{t('providerDetail.customModelsTitle')}</h3>
                      <Badge tone="gray" class="text-[10px] font-mono px-1.5 py-0.2 shrink-0">
                        {customCount()}
                      </Badge>
                      <span class="text-xs text-faint truncate hidden sm:inline">{t('providerDetail.customModelsDesc')}</span>
                    </div>
                    <Button
                      size="sm"
                      variant="ghost"
                      class="text-xs shrink-0"
                      onClick={() => setShowCustomModels(s => !s)}
                    >
                      {showCustomModels() ? t('common.collapse') : (customCount() > 0 ? t('common.expand') : `+ ${t('common.add')}`)}
                    </Button>
                  </div>
                  <Show when={showCustomModels()}>
                    <div class="pt-3 border-t border-subtle/50 mt-2.5 space-y-2.5 animate-fade-in">
                      <div class="flex gap-2">
                        <Input
                          value={newCustomModelId()}
                          onInput={setNewCustomModelId}
                          placeholder={t('providerDetail.customModelPlaceholder')}
                          size="sm"
                          onKeyDown={e => {
                            if (e.key === 'Enter') {
                              e.preventDefault()
                              const val = newCustomModelId().trim()
                              if (val) { addCustomModel(val, ''); setNewCustomModelId('') }
                            }
                          }}
                        />
                        <Button
                          size="sm"
                          variant="secondary"
                          disabled={!newCustomModelId().trim()}
                          onClick={() => {
                            const val = newCustomModelId().trim()
                            if (val) { addCustomModel(val, ''); setNewCustomModelId('') }
                          }}
                        >
                          {t('common.add')}
                        </Button>
                      </div>
                      <div class="flex flex-wrap gap-2">
                        <Show when={customCount() > 0} fallback={
                          <span class="text-xs text-faint">{t('providerDetail.noCustomModels')}</span>
                        }>
                          <For each={modelsData().customModels ?? models()?.customModels ?? []}>
                            {m => (
                              <span class={`inline-flex items-center gap-2 px-2.5 py-1 rounded-control border text-xs transition-colors ${
                                m.enabled !== false ? 'bg-black/6 dark:bg-white/10 border-black/12 dark:border-white/15 text-text' : 'bg-bg-elevated/40 border-black/6 dark:border-white/8 text-faint opacity-60'
                              }`}>
                                <span class={`font-mono ${m.enabled === false ? 'line-through' : ''}`}>{m.name || m.id}</span>
                                <button
                                  type="button"
                                  class="text-faint hover:text-accent text-xs"
                                  title={t('providerDetail.editModelMetaTitle')}
                                  onClick={() => startEditModel(m)}
                                >
                                  <IconEdit size={12} />
                                </button>
                                <Toggle
                                  checked={m.enabled !== false}
                                  onChange={() => toggleModel(m.id || m.name, m.enabled !== false)}
                                />
                                <button class="text-faint hover:text-danger ml-0.5 cursor-pointer" title={t('providerDetail.deleteModelTitle2')} onClick={() => removeCustomModel(m.id || m.name)}><IconClose size={12} /></button>
                              </span>
                            )}
                          </For>
                        </Show>
                      </div>
                    </div>
                  </Show>
                </Card>

                <Card class="p-4 sm:p-4.5 space-y-3">
                  <div class="flex items-center justify-between gap-2 flex-wrap text-xs text-faint">
                    <div class="flex items-center gap-2">
                      <h3 class="text-sm font-semibold text-foreground">{t('providerDetail.availableModels')}</h3>
                      <span>
                        {t('providerDetail.modelsCountSummary', {
                          total: (modelsData().registryModels ?? models()?.registryModels ?? []).length,
                          enabled: (modelsData().registryModels ?? models()?.registryModels ?? []).filter(m => m.enabled !== false).length,
                        })}
                      </span>
                    </div>
                    <Show when={modelSearch().trim()}>
                      <span class="text-accent font-mono">
                        {(() => {
                          const count = (modelsData().registryModels ?? models()?.registryModels ?? []).filter(m => {
                            const q = modelSearch().trim().toLowerCase()
                            return (m.name || '').toLowerCase().includes(q) || (m.id || '').toLowerCase().includes(q)
                          }).length
                          return `${count} / ${(modelsData().registryModels ?? models()?.registryModels ?? []).length}`
                        })()}
                      </span>
                    </Show>
                  </div>
                  <Show
                    when={(modelsData().registryModels ?? models()?.registryModels ?? []).length > 0}
                    fallback={<Empty message={t('providerDetail.noModels')} />}
                  >
                    <div class="grid sm:grid-cols-2 lg:grid-cols-3 gap-2.5">
                        <For each={(modelsData().registryModels ?? models()?.registryModels ?? []).filter(m => {
                          const q = modelSearch().trim().toLowerCase()
                          if (!q) return true
                          return (m.name || '').toLowerCase().includes(q) || (m.id || '').toLowerCase().includes(q)
                        })}>
                          {m => (
                            <div
                              class={`flex flex-col justify-between p-3 rounded-control border transition-all ${
                                m.enabled !== false
                                  ? 'border-subtle bg-bg-elevated/70 hover:border-accent/40 shadow-sm'
                                  : 'border-subtle/30 bg-bg-elevated/25 opacity-55 hover:opacity-80'
                              }`}
                            >
                              <div class="flex items-start justify-between gap-2">
                                <div class="min-w-0 flex-1 pr-1">
                                  <div class="flex items-center gap-1.5 flex-wrap">
                                    <span class={`text-xs truncate font-medium ${m.enabled !== false ? 'text-foreground' : 'text-faint line-through'}`}>
                                      {m.name || m.id}
                                    </span>
                                    <Show when={m.isFree}>
                                      <span class="px-1.5 py-0.5 rounded text-[10px] font-medium bg-success/12 text-success border border-success/30 shrink-0">
                                        {t('providerDetail.freeBadge')}
                                      </span>
                                    </Show>
                                    <Show when={m.enabled === false}>
                                      <span class="px-1.5 py-0.5 rounded text-[10px] font-medium bg-black/8 dark:bg-white/10 text-muted border border-black/12 dark:border-white/15 shrink-0">
                                        {t('providerDetail.chipDisabled')}
                                      </span>
                                    </Show>
                                    <Show when={m.hasOverride}>
                                      <span class="px-1.5 py-0.5 rounded text-[10px] font-medium bg-accent/15 text-accent border border-accent/30 shrink-0" title={t('providerDetail.customMetaChipTitle')}>
                                        {t('providerDetail.chipCustom')}
                                      </span>
                                    </Show>
                                  </div>
                                  <div class="text-[11px] text-faint font-mono truncate mt-0.5">{m.id || m.name}</div>
                                </div>
                                <div class="shrink-0 flex items-center gap-1">
                                  <button
                                    type="button"
                                    class={`p-1 text-xs rounded transition-colors ${testingModels()[m.id || m.name || ''] ? 'text-accent animate-spin cursor-wait' : 'text-faint hover:text-accent cursor-pointer'}`}
                                    title={t('providerDetail.testSingleTitle')}
                                    disabled={testingModels()[m.id || m.name || '']}
                                    onClick={() => testSingleModel(m.id || m.name || '')}
                                  >
                                    <IconZap size={12} />
                                  </button>
                                  <Show
                                    when={m.canEdit !== false}
                                    fallback={
                                      <span
                                        class="text-faint px-1 text-[11px] cursor-not-allowed opacity-50"
                                        title={t('providerDetail.lockedMetaTitle')}
                                      >
                                        <IconLock size={12} />
                                      </span>
                                    }
                                  >
                                    <button
                                      type="button"
                                      class="text-faint hover:text-accent p-1 text-xs rounded transition-colors"
                                      title={t('providerDetail.editModelMetaLongTitle')}
                                      onClick={() => startEditModel(m)}
                                    >
                                      <IconEdit size={12} />
                                    </button>
                                  </Show>
                                  <div title={m.enabled !== false ? t('providerDetail.toggleOffHint') : t('providerDetail.toggleOnHint')}>
                                    <Toggle
                                      checked={m.enabled !== false}
                                      onChange={() => toggleModel(m.id || m.name, m.enabled !== false)}
                                    />
                                  </div>
                                </div>
                              </div>

                              {/* 元数据 Badge 栏 */}
                              <div class="mt-2 pt-2 border-t border-subtle/40 flex items-center gap-1.5 flex-wrap text-[10px] text-faint">
                                <Show when={modelTestResults()[m.id || m.name || '']}>
                                  {res => (
                                    <Show
                                      when={res().ok}
                                      fallback={
                                        <span
                                          class="inline-flex items-center gap-1 px-1.5 py-0.5 rounded text-[10px] font-medium bg-danger/12 text-danger border border-danger/30 cursor-help"
                                          title={res().error || t('providerDetail.testNotPassed')}
                                        >
                                          <IconClose size={10} class="shrink-0" />
                                          <span>{res().error ? (res().error!.length > 12 ? res().error!.slice(0, 10) + '…' : res().error) : t('common.failed')}</span>
                                        </span>
                                      }
                                    >
                                      <span
                                        class="inline-flex items-center gap-1 px-1.5 py-0.5 rounded text-[10px] font-medium bg-success/12 text-success border border-success/30 font-mono"
                                        title={t('providerDetail.testPassedLatency', { latency: res().latency || t('providerDetail.normal') })}
                                      >
                                        <IconCheck size={10} class="shrink-0" />
                                        <span>{res().latency || t('providerDetail.connected')}</span>
                                      </span>
                                    </Show>
                                  )}
                                </Show>
                                <Show when={m.contextLength && m.contextLength > 0}>
                                  <span class="bg-hover px-1.5 py-0.5 rounded font-mono">
                                    {t('providerDetail.contextLength', { length: m.contextLength! >= 1024 ? `${Math.round(m.contextLength! / 1024)}k` : m.contextLength! })}
                                  </span>
                                </Show>
                                <Show when={m.maxOutputTokens && m.maxOutputTokens > 0}>
                                  <span class="bg-hover px-1.5 py-0.5 rounded font-mono">
                                    {t('providerDetail.outputLength', { length: m.maxOutputTokens! >= 1024 ? `${Math.round(m.maxOutputTokens! / 1024)}k` : m.maxOutputTokens! })}
                                  </span>
                                </Show>
                                <Show when={!m.contextLength && !m.maxOutputTokens}>
                                  <span class="italic text-[10px] opacity-60">{t('providerDetail.noContext')}</span>
                                </Show>
                              </div>
                            </div>
                          )}
                        </For>
                      </div>
                  </Show>
                </Card>
              </div>
            </Show>
            {/* 会话测试 */}
            <Show when={tab() === 'chat'}>
              <div class="space-y-4 animate-fade-in">
                <Card class="p-4 flex flex-wrap items-center justify-between gap-3">
                  <div class="flex items-center gap-3 flex-1 min-w-[240px]">
                    <span class="text-xs text-faint shrink-0">{t('providerDetail.chatModel')}</span>
                    <Select
                      class="flex-1 min-w-[200px]"
                      value={selectedModel()}
                      onChange={setSelectedModel}
                      options={[
                        { value: '', label: t('providerDetail.chatDefaultModel') },
                        ...activeModels().map(m => ({
                          value: m.id || '',
                          label: m.name ? `${m.name} (${m.id})` : m.id || '',
                        })),
                      ]}
                    />
                  </div>
                  <div class="flex items-center gap-3">
                    <Show when={selectedModel()}>
                      <Badge tone="blue">{selectedModel()}</Badge>
                    </Show>
                    <A
                      href="/playground"
                      class="text-xs text-accent hover:underline flex items-center gap-1"
                      title={t('providerDetail.playgroundLinkTitle')}
                    >
                      {t('providerDetail.chatGoPlayground')}
                    </A>
                    <Button size="sm" variant="ghost" onClick={() => setChatHistory([])}>{t('providerDetail.chatClear')}</Button>
                  </div>
                </Card>

                {/* 独立可滚动对话气泡区：固定高度并在新消息到来时自动置底，绝不让整个页面下拉 */}
                <Card class="p-0 overflow-hidden">
                  <div
                    ref={chatBoxRef}
                    class="p-5 h-[420px] max-h-[calc(100vh-360px)] overflow-y-auto space-y-3 scroll-smooth"
                  >
                  <Show
                    when={chatHistory().length > 0}
                    fallback={<Empty message={t('providerDetail.chatEmpty', { name: providerDisplayName() })} />}
                  >
                    <For each={chatHistory()}>
                      {msg => (
                        <div class={`flex flex-col ${msg.role === 'user' ? 'items-end' : 'items-start'}`}>
                          <div class={`max-w-[80%] px-4 py-2.5 rounded-2xl text-sm whitespace-pre-wrap leading-relaxed shadow-xs ${
                            msg.role === 'user'
                              ? 'bg-accent text-on-accent'
                              : 'bg-hover text-foreground border border-subtle'
                          }`}>
                            {msg.content}
                          </div>
                          <Show when={msg.role === 'assistant' && msg.servedModel}>
                            <span class="mt-1 px-1.5 py-0.5 text-[10px] text-faint font-mono">
                              {t('providerDetail.chatServedBy', { model: msg.servedModel! })}
                            </span>
                          </Show>
                        </div>
                      )}
                    </For>
                    <Show when={chatBusy()}>
                      <div class="flex justify-start">
                        <div class="px-4 py-2.5 rounded-2xl bg-hover text-sm text-faint flex items-center gap-2">
                          <span class="inline-block w-2 h-2 rounded-full bg-accent animate-pulse" />
                          {t('providerDetail.chatThinking')}
                        </div>
                      </div>
                    </Show>
                  </Show>
                  </div>
                </Card>

                <Show when={chatErr()}>
                  <Alert variant="danger" closable onClose={() => setChatErr('')}>
                    {chatErr()}
                  </Alert>
                </Show>

                <Card class="p-3 flex gap-2">
                  <Input
                    value={prompt()}
                    placeholder={t('providerDetail.chatPlaceholder')}
                    onInput={setPrompt}
                    disabled={chatBusy()}
                    onKeyDown={e => {
                      if (e.key === 'Enter' && !e.shiftKey) {
                        e.preventDefault()
                        sendChat()
                      }
                    }}
                  />
                  <Button variant="primary" loading={chatBusy()} disabled={!prompt().trim()} onClick={sendChat}>
                    {t('playground.send')}
                  </Button>
                </Card>
              </div>
            </Show>
          </div>
        )}
      </Show>

      {/* 模型元数据编辑弹窗 */}
      <Modal
        open={Boolean(editingModel())}
        title={t('providerDetail.editModelMeta', { id: editingModel()?.id || '' })}
        onClose={() => setEditingModel(null)}
      >
        <div class="space-y-4">
          <Field label={t('providerDetail.modelIdLabel')} hint={t('providerDetail.modelIdHint')}>
            <Input value={editingModel()?.id || ''} disabled class="bg-hover font-mono opacity-80" />
          </Field>
          <Field label={t('providerDetail.displayNameLabel')} hint={t('providerDetail.displayNameHint')}>
            <Input value={editDisplayName()} onInput={setEditDisplayName} placeholder={t('providerDetail.displayNamePlaceholder')} />
          </Field>
          <div class="grid grid-cols-2 gap-3">
            <Field label={t('providerDetail.contextLengthLabel')} hint={t('providerDetail.contextLengthHint')}>
              <Input
                type="number"
                value={editContextLength()}
                onInput={setEditContextLength}
                placeholder="128000"
              />
            </Field>
            <Field label={t('providerDetail.maxOutputLabel')} hint={t('providerDetail.maxOutputHint')}>
              <Input
                type="number"
                value={editMaxOutput()}
                onInput={setEditMaxOutput}
                placeholder="8192"
              />
            </Field>
          </div>
          <div class="flex items-center justify-between pt-3 border-t border-subtle">
            <div>
              <Show when={editingModel()?.hasOverride}>
                <Button
                  size="sm"
                  variant="danger"
                  loading={savingMeta()}
                  onClick={handleResetModelMeta}
                >
                  {t('providerDetail.restoreOfficialDefault')}
                </Button>
              </Show>
            </div>
            <div class="flex items-center gap-2">
              <Button size="sm" variant="secondary" onClick={() => setEditingModel(null)}>
                {t('common.cancel')}
              </Button>
              <Button size="sm" variant="primary" loading={savingMeta()} onClick={handleSaveModelMeta}>
                {t('providerDetail.saveMetadata')}
              </Button>
            </div>
          </div>
        </div>
      </Modal>

      {/* 添加新账号 / 凭证弹窗 */}
      <Modal
        open={addAccountOpen()}
        title={t('providerDetail.addAccountTitle', { name: providerDisplayName() })}
        onClose={() => setAddAccountOpen(false)}
      >
        <div class="space-y-4">
          <div class="p-3 rounded-control bg-hover border border-subtle text-xs text-faint leading-relaxed">
            {t('providerDetail.addAccountHint')}
          </div>
          <Field label={t('providerDetail.accountNameLabel')} hint={t('providerDetail.accountNameHint')}>
            <Input
              value={newAccountName()}
              onInput={setNewAccountName}
              placeholder={t('providerDetail.newAccountPlaceholder', { index: accounts().length + 1 })}
            />
          </Field>
          <Field label={t('providerDetail.authMethodLabel')} hint={t('providerDetail.authMethodSelectHint')}>
            <Select
              value={newAccountAuthType()}
              options={accountAuthOptions()}
              onChange={setNewAccountAuthType}
            />
          </Field>
          <Show when={newAccountAuthType() === 'api-key'}>
            <Field
              label={conn()?.provider === 'qoder' ? t('providerDetail.qoderCredLabel') : t('providerDetail.authCredential')}
              hint={t('providerDetail.credSecureHint')}
            >
              <div class="space-y-1.5">
                <Input
                  type="password"
                  value={newAccountApiKey()}
                  onInput={setNewAccountApiKey}
                  placeholder={
                    conn()?.provider === 'qoder'
                      ? 'pt-... (Personal Access Token)'
                      : conn()?.provider === 'opencode'
                      ? t('providerDetail.opencodeKeyPlaceholder')
                      : 'sk-...'
                  }
                />
                <Show when={regInfo()?.apiKeyUrl}>
                  <div class="flex justify-end">
                    <a
                      href={regInfo()!.apiKeyUrl!}
                      target="_blank"
                      rel="noreferrer"
                      class="text-xs text-accent hover:underline inline-flex items-center gap-1"
                    >
                      {t('providerDetail.getKeyConsole')}
                    </a>
                  </div>
                </Show>
              </div>
            </Field>
          </Show>
          <Show when={newAccountAuthType() === 'oauth'}>
            <div class="p-3 rounded-control bg-accent/10 border border-accent/30 text-xs text-accent space-y-1">
              <div class="font-medium flex items-center gap-1.5"><IconCheck size={14} /> {t('providerDetail.oauthModeTitle')}</div>
              <p class="text-faint">{t('providerDetail.oauthModeDesc')}</p>
            </div>
          </Show>
          <Field label={t('providerDetail.priorityLabel')} hint={t('providerDetail.newAccountPriorityHint')}>
            <Input
              type="number"
              value={newAccountPriority()}
              onInput={setNewAccountPriority}
              class="w-32!"
            />
          </Field>
          <div class="flex items-center justify-end gap-2 pt-3 border-t border-subtle">
            <Button size="sm" variant="secondary" onClick={() => setAddAccountOpen(false)}>
              {t('common.cancel')}
            </Button>
            <Button size="sm" variant="primary" loading={addingAccount()} onClick={handleAddAccountSubmit}>
              {t('providerDetail.saveAccount')}
            </Button>
          </div>
        </div>
      </Modal>
      {/* 统一设备码 OAuth 授权弹窗 */}
      <Modal
        open={!!deviceFlow() || devicePolling() || isImportFlow() || !!deviceError()}
        title={isImportFlow() ? t('providerDetail.importTokenTitle', { name: providerDisplayName() }) : t('providerDetail.connectTitle', { name: providerDisplayName() })}
        onClose={cancelDeviceFlow}
      >
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
                <Button size="sm" variant="primary" onClick={startDeviceFlow}>
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
                <textarea
                  rows={4}
                  value={importTokenText()}
                  onInput={e => setImportTokenText(e.currentTarget.value)}
                  placeholder={t('providerDetail.tokenPastePlaceholder')}
                  class="w-full rounded-control border border-subtle bg-bg/80 px-3 py-2 text-xs font-mono focus-visible:outline-2 focus-visible:outline-ring"
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
                        <button
                          type="button"
                          class="p-1.5 rounded hover:bg-accent/20 text-accent transition-colors cursor-pointer"
                          title={t('providerDetail.copyCodeTitle')}
                          onClick={() => {
                            if (flow().userCode) {
                              navigator.clipboard.writeText(flow().userCode!)
                              setCopiedCode(true)
                              toast.success(t('providerDetail.codeCopied'))
                              setTimeout(() => setCopiedCode(false), 2000)
                            }
                          }}
                        >
                          {copiedCode() ? <IconCheck size={14} class="text-success" /> : <IconClipboard size={14} />}
                        </button>
                      </div>
                    </div>
                  </Show>

                  {/* 登录 URL 卡片 */}
                  <div class="p-3.5 rounded-card bg-hover/80 border border-subtle text-left space-y-2">
                    <div class="text-[11px] text-faint font-medium text-center">Login URL</div>
                    <div class="font-mono text-xs break-all text-foreground select-all bg-bg/70 p-2.5 rounded border border-subtle leading-relaxed">
                      {flow().verificationUriComplete || flow().verificationUri}
                    </div>
                    <div class="flex items-center justify-end gap-2 pt-1">
                      <Button
                        size="sm"
                        variant="secondary"
                        onClick={() => {
                          const url = flow().verificationUriComplete || flow().verificationUri
                          if (url) {
                            navigator.clipboard.writeText(url)
                            toast.success(t('providerDetail.copyUrlSuccess'))
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
      </Modal>
    </div>
  )
}

export default ProviderDetail
