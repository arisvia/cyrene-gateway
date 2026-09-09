import { type Component, For, Show, createSignal, createResource, createEffect, onMount, onCleanup } from 'solid-js'
import { A, useParams, useNavigate } from '@solidjs/router'
import { useGatewayStore } from '@/stores/gateway'
import { api, apiPost } from '@/lib/api'
import { useToast } from '@/lib/toast'
import type { Provider, ProviderModel } from '@/types/domain'
import { Card, Badge, Button, Input, Toggle, Field, Empty, Skeleton, Select, Modal, Alert, PageHeader, IconBulb, IconCheck, IconClose, IconLock, IconEdit, IconClipboard, confirm } from '@/components/ui'

const ProviderDetail: Component = () => {
  const params = useParams<{ id: string }>()
  const navigate = useNavigate()
  const store = useGatewayStore()
  const toast = useToast()
  const [conn, setConn] = createSignal<Provider | null>(null)
  const [loading, setLoading] = createSignal(true)
  const [notFound, setNotFound] = createSignal(false)
  const [saving, setSaving] = createSignal(false)
  const [testing, setTesting] = createSignal(false)
  const [testResult, setTestResult] = createSignal<{ ok: boolean; msg: string } | null>(null)
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
      const reply = r?.choices?.[0]?.message?.content ?? '(无返回)'
      setChatHistory(h => [...h, { role: 'assistant', content: reply, servedModel: servedModel || r?.model }])
    } catch (e: unknown) {
      let errMsg = '请求失败'
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
      toast.success('已成功从官方上游同步最新模型列表')
      await refetchModels()
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : String(e)
      toast.error(`同步模型失败: ${msg}`)
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

  function switchAccount(targetId: string) {
    if (!targetId || targetId === conn()?.id) return
    navigate(`/providers/${targetId}`)
  }

  const accountAuthOptions = () => {
    const p = conn()?.provider
    if (p === 'opencode') {
      return [
        { value: 'none', label: '免密模式（激活 Zen / Big-Pickle 等免密模型）' },
        { value: 'api-key', label: '商业授权模式（输入 Go / Zen 套餐 API Key）' },
      ]
    }
    const modes = regInfo()?.authModes || [regInfo()?.authType || 'api-key']
    const opts: Array<{ value: string; label: string }> = [
      { value: 'api-key', label: 'API Key / 访问凭据 (PAT)' },
    ]
    if (modes.includes('oauth') || regInfo()?.category === 'oauth') {
      opts.push({ value: 'oauth', label: 'OAuth 授权模式 (支持设备码一键登录)' })
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
    if (aType === 'api-key' && !newAccountApiKey().trim() && p !== 'opencode') {
      toast.error('请填写 API Key')
      return
    }
    setAddingAccount(true)
    try {
      const added = await store.addProvider({
        provider: p,
        authType: aType,
        name: newAccountName().trim() || `${p} 备用账号`,
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
      toast.error(e instanceof Error ? e.message : '添加账号失败')
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
          toast.info(`已在新窗口打开 ${conn()?.name || p} 授权页面，完成授权后网关将自动绑定。`)
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
              toast.success('OAuth 网页授权成功！已绑定并刷新账号凭据。')
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
            toast.success('OAuth 授权成功！已绑定并刷新账号凭据。')
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
      toast.error(`发起授权失败: ${msg}`)
    }
  }

  async function handleImportToken() {
    const p = conn()?.provider
    if (!p) return
    const token = importTokenText().trim()
    if (!token) {
      toast.error('请输入访问令牌 (Access Token)')
      return
    }
    setImportingToken(true)
    try {
      await apiPost(`/api/oauth/${p}/import`, {
        accessToken: token,
        name: name().trim() || undefined,
      })
      toast.success('令牌导入成功！已更新连接凭据。')
      cancelDeviceFlow()
      await store.loadProvidersOnly()
      await load()
      refetchOAuth()
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : String(e)
      setDeviceError(msg)
      toast.error(`导入失败: ${msg}`)
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
      setName(r.name || '')
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
    setTestResult(null)
    try {
      const r = await store.testProvider(params.id)
      setTestResult({ ok: !!r?.ok, msg: r?.ok ? `连通 · ${r.latencyMs}ms` : `${r?.error || '失败'}${r?.code ? `（HTTP ${r.code}）` : ''}` })
    } catch (e: unknown) {
      setTestResult({ ok: false, msg: e instanceof Error ? e.message : '请求失败' })
    } finally {
      setTesting(false)
    }
  }

  async function addCustomModel(modelId: string, modelName: string) {
    if (!modelId.trim()) return
    try {
      await store.addProviderModel(params.id, { id: modelId.trim(), name: modelName.trim() || modelId.trim() })
      toast.success(`已添加自定义模型「${modelId.trim()}」`)
      refetchModels()
    } catch (e: unknown) {
      toast.error(e instanceof Error ? e.message : '添加模型失败')
    }
  }

  async function removeCustomModel(modelId: string) {
    const ok = await confirm({
      title: '删除模型',
      message: `确定要删除自定义模型「${modelId}」吗？`,
      variant: 'danger',
    })
    if (!ok) return
    try {
      await store.deleteProviderModel(params.id, modelId)
      toast.success(`已删除模型「${modelId}」`)
      refetchModels()
    } catch (e: unknown) {
      toast.error(e instanceof Error ? e.message : '删除模型失败')
    }
  }

  return (
    <div class="space-y-5 stagger">
      <Show when={!loading() && conn()}>
        {c => (
          <PageHeader
            title={
              <div class="flex items-center gap-2.5">
                <A href="/providers" class="text-xs text-faint hover:text-accent inline-flex items-center gap-1 font-normal">
                  ← 返回
                </A>
                <span>{c().name || c().provider}</span>
              </div>
            }
            badge={
              <div class="flex items-center gap-2 flex-wrap">
                <Badge tone={c().isActive ? 'green' : 'gray'}>{c().isActive ? '启用' : '停用'}</Badge>
                <Badge tone="blue">{c().authType}</Badge>
                <span class="text-xs text-faint font-mono">{c().id.slice(0, 12)}…</span>
              </div>
            }
            actions={
              <div class="flex items-center gap-3">
                <Button size="sm" variant="secondary" loading={testing()} onClick={runTest}>测试连接</Button>
                <Toggle checked={c().isActive} onChange={() => { store.toggleProvider(c()); load() }} />
              </div>
            }
          >
            <Show when={testResult()}>
              <div class={`text-xs px-3 py-1.5 rounded-control inline-block ${testResult()!.ok
                ? 'bg-success/10 text-success'
                : 'bg-danger/10 text-danger'}`}>
                {testResult()!.msg}
              </div>
            </Show>
          </PageHeader>
        )}
      </Show>

      <Show when={loading()}>
        <Card class="p-6 space-y-3"><Skeleton class="h-6 w-48" /><Skeleton class="h-4 w-full" /><Skeleton class="h-4 w-2/3" /></Card>
      </Show>

      <Show when={notFound()}>
        <Card class="p-6"><Empty message="连接不存在或已被删除。" /></Card>
      </Show>

      <Show when={!loading() && conn()}>
        {c => (
          <div class="space-y-5">
            {/* Tab */}
            <div class="flex gap-1 border-b border-subtle">
              <For each={[
                { id: 'overview' as const, label: '账号与连接' },
                { id: 'models' as const, label: '可用模型' },
                { id: 'chat' as const, label: '会话测试' },
              ]}>
                {t => (
                  <button
                    type="button"
                    class={`px-3.5 py-2 text-sm font-medium border-b-2 transition-colors ${tab() === t.id
                      ? 'border-[color:var(--accent)] text-foreground font-semibold'
                      : 'border-transparent text-faint hover:text-muted'}`}
                    onClick={() => setTab(t.id)}
                  >
                    {t.label}
                  </button>
                )}
              </For>
            </div>

            {/* 账号与连接配置 */}
            <Show when={tab() === 'overview'}>
              <div class="grid lg:grid-cols-12 gap-5 items-start">
                {/* 左侧 (5 cols)：多账号与调度看板 (带独立滚动区，不会被挤出视野) */}
                <div class="lg:col-span-5 space-y-3">
                  <Card class="p-4 space-y-3">
                    <div class="flex items-center justify-between gap-2 pb-2.5 border-b border-subtle">
                      <div>
                        <h3 class="text-sm font-semibold flex items-center gap-2">
                          <span>账号与凭据池</span>
                          <Badge tone="blue">{accounts().length} 个</Badge>
                        </h3>
                        <p class="text-[11px] text-faint mt-0.5">
                          优先级升序调度 · 限流时自动 Fallback
                        </p>
                      </div>
                      <Button
                        size="sm"
                        variant="primary"
                        onClick={() => {
                          setNewAccountName(`${conn()?.name || conn()?.provider} 备用账号`)
                          setNewAccountAuthType(conn()?.provider === 'opencode' ? 'none' : 'api-key')
                          setNewAccountApiKey('')
                          setNewAccountPriority(String((Number(priority()) || 0) + 10))
                          setAddAccountOpen(true)
                        }}
                      >
                        + 加账号
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
                              aria-label={`选择账号 ${acc.name || acc.provider}`}
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
                                    {acc.name || acc.provider}
                                  </span>
                                  <Show when={isCurrent()}>
                                    <span class="text-[10px] px-1.5 py-0.5 rounded bg-accent/20 text-accent font-medium shrink-0">
                                      编辑中
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
                                      title="删除此账号"
                                      onClick={async (e) => {
                                        e.stopPropagation()
                                        const ok = await confirm({
                                          title: '删除账号',
                                          message: `确定要删除账号「${acc.name || acc.provider}」吗？`,
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
                                <span class="inline-flex items-center justify-center h-5 px-2 text-[10px] font-medium rounded-full text-info bg-info/10 border border-info/20">
                                  {acc.authType === 'api-key' || acc.authType === 'apikey' ? 'API Key' : acc.authType === 'oauth' ? 'OAuth' : acc.authType}
                                </span>
                                <span class={`inline-flex items-center h-5 gap-1.5 text-[10px] px-2 rounded-full border font-mono ${
                                  idx() === 0
                                    ? 'bg-accent/15 border-accent/40 text-accent font-semibold'
                                    : 'bg-hover/80 border-subtle text-muted'
                                }`}>
                                  <span class={`inline-block w-1.5 h-1.5 rounded-full shrink-0 ${idx() === 0 ? 'bg-accent animate-pulse' : 'bg-faint'}`} />
                                  <span class="leading-none">优先级 {acc.priority}</span>
                                  <span class="opacity-70 font-sans leading-none">{idx() === 0 ? '(主)' : '(备用)'}</span>
                                </span>
                                <Show when={cooling()}>
                                  <Badge tone="amber" class="text-[10px] h-5">限流冷却中</Badge>
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
                    <div class="p-2.5 rounded bg-hover/70 border border-subtle/60 text-[11px] text-faint leading-relaxed flex items-start gap-1.5">
                      <IconBulb size={14} class="text-accent shrink-0 mt-0.5" />
                      <div><span class="text-foreground font-medium">调度机制：</span>主账号遇到 429 或配额耗尽时，网关自动 Fallback 转移至备用账号；同优先级多账号自动负载均衡分摊并发。</div>
                    </div>
                  </Card>
                </div>

                {/* 右侧 (7 cols)：当前正在编辑的账号详情与端点配置 (高度受控，内部滚动，底部按钮绝对吸底可见) */}
                <div class="lg:col-span-7">
                  <Card class="p-5 flex flex-col max-h-[calc(100vh-220px)] min-h-[480px]">
                    <div class="flex items-center justify-between pb-3 border-b border-subtle shrink-0">
                      <div>
                        <div class="text-sm font-semibold flex items-center gap-2">
                          <span>编辑账号：{c().name || c().provider}</span>
                          <Badge tone="blue">{c().authType === 'api-key' ? 'API Key' : c().authType === 'oauth' ? 'OAuth' : c().authType}</Badge>
                        </div>
                        <div class="text-xs text-faint mt-0.5 font-mono">节点 ID: {c().id}</div>
                      </div>
                      <Show when={regInfo()?.apiKeyUrl}>
                        <a
                          href={regInfo()!.apiKeyUrl!}
                          target="_blank"
                          rel="noreferrer"
                          class="text-xs text-accent hover:underline inline-flex items-center gap-1"
                        >
                          获取官方密钥 ↗
                        </a>
                      </Show>
                    </div>

                    {/* 表单内容滚动区 */}
                    <div class="flex-1 overflow-y-auto pr-1 py-3 space-y-4">
                      {/* OpenCode 专用模式提示 */}
                      <Show when={c().provider === 'opencode'}>
                        <div class="p-3 rounded-control bg-accent/10 border border-accent/25 text-xs text-text space-y-1">
                          <div class="font-medium text-accent">OpenCode 免密与商业授权说明</div>
                          <p class="text-faint leading-relaxed">
                            {c().authType === 'none'
                              ? '当前处于免密模式，可免鉴权直接调用 Zen 系列与 Big-Pickle 等官方免密模型；填入 Go/Zen 套餐 API Key 后保存即可解锁全量进阶商业模型。'
                              : '当前已配置 API Key 商业凭据，网关将使用该凭据鉴权并请求全量商业模型。'}
                          </p>
                        </div>
                      </Show>

                      <Field label="账号显示名称" hint="自定义名称，便于在调度日志和控制台中辨识">
                        <Input value={name()} onInput={setName} placeholder="例如：主账号、备用 PAT、Team B" />
                      </Field>

                      <Field label="调度优先级" hint="数值越小越优先调度。例如：主账号设为 10，备用账号设为 20">
                        <Input type="number" value={priority()} onInput={setPriority} class="!w-32" />
                      </Field>

                      {/* API Key / 凭据输入 (仅非纯 OAuth 模式，或为 OpenCode 时显示) */}
                      <Show
                        when={
                          c().authType !== 'oauth' ||
                          (c().provider === 'opencode' && c().authType !== 'none') ||
                          (modelsData().authModes?.includes('api-key') && c().authType !== 'oauth')
                        }
                        fallback={
                          <div class="p-3.5 rounded-control bg-accent/10 border border-accent/30 space-y-2 text-xs">
                            <div class="flex items-center justify-between">
                              <span class="font-medium text-accent flex items-center gap-1.5">
                                <IconCheck size={14} /> 当前为 OAuth 授权账号
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
                                重新发起授权 ↗
                              </Button>
                            </div>
                            <p class="text-faint leading-relaxed">
                              本账号凭证由官方 OAuth 单点登录/设备码颁发管理，无需手动输入 API Key。如需更新授权请点击上方重新授权。
                            </p>
                          </div>
                        }
                      >
                        <Field
                          label={c().provider === 'qoder' ? 'Personal Access Token (PAT) / API Key' : 'API Key / 访问凭据'}
                          hint={
                            c().provider === 'qoder'
                              ? '支持填入 Qoder Personal Access Token (pt-...) 或 API Key'
                              : c().data?.hasApiKey
                              ? '留空表示保持当前密钥不变；输入新密钥并点击保存后将安全覆盖'
                              : '请输入有效 API Key'
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
                                ? '输入 OpenCode Go/Zen 套餐 API Key'
                                : 'sk-...'
                            }
                          />
                          <Show when={c().data?.hasApiKey}>
                            <div class="text-[11px] text-emerald-400 mt-1 flex items-center gap-1">
                              <IconCheck size={12} /> <span>当前账号已配置密钥</span>
                            </div>
                          </Show>
                        </Field>
                      </Show>

                      <Show when={c().provider.startsWith('custom-') || regInfo()?.category === 'custom'}>
                        <Field
                          label="Base URL (必填)"
                          hint="标准上游 API 地址，如 https://api.my-host.com/v1（必填）"
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
                          <span>高级协议与客户端标识覆盖（Headers / 版本参数）</span>
                        </button>

                        <Show when={showAdvanced()}>
                          <div class="mt-3 p-3.5 rounded-control bg-bg-elevated/50 border border-subtle/70 space-y-3 text-xs">
                            <Show when={modelsData().defaultHeaders && Object.keys(modelsData().defaultHeaders!).length > 0}>
                              <div>
                                <div class="text-faint mb-1.5 font-medium">当前网关内置默认 Header（供参考）：</div>
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
                              label="自定义请求头覆盖 (JSON 格式)"
                              hint="用于应对上游客户端强制校验新版本号。此处指定的 Header 会覆盖默认 Header 发送给上游。"
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
                          当前凭证标识：<span class="font-mono">{String(c().data?.credentialHint ?? '')}</span>
                          <Show when={c().data?.hasAccessToken}> · Access Token 已就绪</Show>
                          <Show when={c().data?.hasRefreshToken}> · Refresh Token 已就绪</Show>
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
                            title: '删除账号',
                            message: `确定要删除此账号「${c().name || c().provider}」吗？`,
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
                        删除此账号
                      </Button>
                      <Button variant="primary" size="sm" loading={saving()} onClick={save}>
                        保存当前配置
                      </Button>
                    </div>
                  </Card>
                </div>
              </div>
            </Show>

            {/* 模型 */}
            <Show when={tab() === 'models'}>
              <div class="space-y-4">
                {/* OpenCode 专用套餐与免密提示横幅 */}
                <Show when={c().provider === 'opencode'}>
                  <div class="p-4 rounded-control border border-subtle bg-bg-elevated/70 flex flex-col sm:flex-row sm:items-center justify-between gap-3 text-xs shadow-glass">
                    <div class="flex items-start sm:items-center gap-2.5 text-text">
                      <Show
                        when={modelsData().isFreeMode}
                        fallback={
                          <>
                            <span class="w-2.5 h-2.5 rounded-full bg-accent animate-pulse shrink-0 mt-0.5 sm:mt-0" />
                            <div>
                              <span class="font-medium text-foreground">商业授权模式：</span>
                              <span class="text-muted">已配置 API Key，网关已解锁 OpenCode Go 套餐与 Zen 充值额度的全量商业模型。</span>
                            </div>
                          </>
                        }
                      >
                        <span class="w-2.5 h-2.5 rounded-full bg-emerald-400 shrink-0 mt-0.5 sm:mt-0" />
                        <div>
                          <span class="font-medium text-emerald-400">免密体验模式（Zen Free）：</span>
                          <span class="text-muted">当前未填写 API Key，网关严格锁定并仅激活官方免费模型（Big Pickle、Mimo Free、Ling Free 等）。填入 API Key 即可解锁全量商业模型。</span>
                        </div>
                      </Show>
                    </div>
                    <Show when={modelsData().isFreeMode}>
                      <Button
                        size="sm"
                        variant="secondary"
                        class="shrink-0 self-start sm:self-auto text-accent hover:border-accent"
                        onClick={() => setTab('overview')}
                      >
                        配置 API Key →
                      </Button>
                    </Show>
                  </div>
                </Show>

                <Card class="p-5">
                  <div class="flex items-center justify-between mb-3">
                    <div>
                      <h3 class="text-sm font-semibold">自定义模型</h3>
                      <p class="text-xs text-faint mt-0.5">注册未在官方列表中的模型 ID，供网关路由匹配与对外转发</p>
                    </div>
                  </div>
                  <div class="flex gap-2">
                    <Input
                      value={newCustomModelId()}
                      onInput={setNewCustomModelId}
                      placeholder="模型 ID，例如 my-finetune-v1"
                      onKeyDown={e => {
                        if (e.key === 'Enter') {
                          e.preventDefault()
                          const val = newCustomModelId().trim()
                          if (val) { addCustomModel(val, ''); setNewCustomModelId('') }
                        }
                      }}
                    />
                    <Button
                      variant="secondary"
                      disabled={!newCustomModelId().trim()}
                      onClick={() => {
                        const val = newCustomModelId().trim()
                        if (val) { addCustomModel(val, ''); setNewCustomModelId('') }
                      }}
                    >
                      添加
                    </Button>
                  </div>
                  <div class="mt-3 flex flex-wrap gap-2">
                    <Show when={(modelsData().customModels ?? models()?.customModels ?? []).length > 0} fallback={
                      <span class="text-xs text-faint">暂无自定义模型</span>
                    }>
                      <For each={modelsData().customModels ?? models()?.customModels ?? []}>
                        {m => (
                          <span class={`inline-flex items-center gap-2 px-2.5 py-1 rounded-control border text-xs transition-colors ${
                            m.enabled !== false ? 'bg-hover border-subtle text-text' : 'bg-bg-elevated/40 border-subtle/30 text-faint opacity-60'
                          }`}>
                            <span class={`font-mono ${m.enabled === false ? 'line-through' : ''}`}>{m.name || m.id}</span>
                            <button
                              type="button"
                              class="text-faint hover:text-accent text-xs"
                              title="编辑模型元数据"
                              onClick={() => startEditModel(m)}
                            >
                              <IconEdit size={12} />
                            </button>
                            <Toggle
                              checked={m.enabled !== false}
                              onChange={() => toggleModel(m.id || m.name, m.enabled !== false)}
                            />
                            <button class="text-faint hover:text-danger ml-0.5 cursor-pointer" title="删除" onClick={() => removeCustomModel(m.id || m.name)}><IconClose size={12} /></button>
                          </span>
                        )}
                      </For>
                    </Show>
                  </div>
                </Card>

                <Card class="p-5 flex flex-col">
                  <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-3 mb-3">
                    <div class="flex items-center gap-2 flex-wrap">
                      <h3 class="text-sm font-semibold">可用模型</h3>
                      <span class="text-xs text-faint">
                        共 {(modelsData().registryModels ?? models()?.registryModels ?? []).length} 个 · 开放中 {(modelsData().registryModels ?? models()?.registryModels ?? []).filter(m => m.enabled !== false).length} 个
                      </span>
                    </div>
                    <div class="flex items-center gap-2 w-full sm:w-auto">
                      <div class="flex-1 sm:w-64">
                        <Input
                          value={modelSearch()}
                          onInput={setModelSearch}
                          placeholder="搜索模型 ID 或名称…"
                          size="sm"
                        />
                      </div>
                      <Button
                        size="sm"
                        variant="secondary"
                        loading={syncingModels()}
                        onClick={handleSyncModels}
                        title="向官方上游或端点发起查询并更新本地模型缓存"
                      >
                        同步上游模型
                      </Button>
                    </div>
                  </div>

                  <Show
                    when={(modelsData().registryModels ?? models()?.registryModels ?? []).length > 0}
                    fallback={<Empty message="该提供商未上报模型列表。" />}
                  >
                    {/* 独立可滚动区域：带对外开放开关的 Liquid Glass 卡片网格 */}
                    <div class="max-h-[420px] overflow-y-auto pr-1">
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
                                      <span class="px-1.5 py-0.5 rounded text-[10px] font-medium bg-emerald-500/15 text-emerald-400 border border-emerald-500/30 shrink-0">
                                        免费
                                      </span>
                                    </Show>
                                    <Show when={m.enabled === false}>
                                      <span class="px-1.5 py-0.5 rounded text-[10px] font-medium bg-zinc-500/15 text-zinc-400 border border-zinc-500/30 shrink-0">
                                        不对外
                                      </span>
                                    </Show>
                                    <Show when={m.hasOverride}>
                                      <span class="px-1.5 py-0.5 rounded text-[10px] font-medium bg-accent/15 text-accent border border-accent/30 shrink-0" title="包含用户自定义元数据">
                                        已改
                                      </span>
                                    </Show>
                                  </div>
                                  <div class="text-[11px] text-faint font-mono truncate mt-0.5">{m.id || m.name}</div>
                                </div>
                                <div class="shrink-0 flex items-center gap-1">
                                  <Show
                                    when={m.canEdit !== false}
                                    fallback={
                                      <span
                                        class="text-faint px-1 text-[11px] cursor-not-allowed opacity-50"
                                        title="官方动态同步元数据，已锁定保护"
                                      >
                                        <IconLock size={12} />
                                      </span>
                                    }
                                  >
                                    <button
                                      type="button"
                                      class="text-faint hover:text-accent p-1 text-xs rounded transition-colors"
                                      title="编辑模型元数据（名称、上下文长度等）"
                                      onClick={() => startEditModel(m)}
                                    >
                                      <IconEdit size={12} />
                                    </button>
                                  </Show>
                                  <div title={m.enabled !== false ? '点击关闭，禁止对外提供' : '点击开启，恢复对外提供'}>
                                    <Toggle
                                      checked={m.enabled !== false}
                                      onChange={() => toggleModel(m.id || m.name, m.enabled !== false)}
                                    />
                                  </div>
                                </div>
                              </div>

                              {/* 元数据 Badge 栏 */}
                              <div class="mt-2 pt-2 border-t border-subtle/40 flex items-center gap-1.5 flex-wrap text-[10px] text-faint">
                                <Show when={m.contextLength && m.contextLength > 0}>
                                  <span class="bg-hover px-1.5 py-0.5 rounded font-mono">
                                    {m.contextLength! >= 1024 ? `${Math.round(m.contextLength! / 1024)}k` : m.contextLength} 上下文
                                  </span>
                                </Show>
                                <Show when={m.maxOutputTokens && m.maxOutputTokens > 0}>
                                  <span class="bg-hover px-1.5 py-0.5 rounded font-mono">
                                    {m.maxOutputTokens! >= 1024 ? `${Math.round(m.maxOutputTokens! / 1024)}k` : m.maxOutputTokens} 输出
                                  </span>
                                </Show>
                                <Show when={!m.contextLength && !m.maxOutputTokens}>
                                  <span class="italic text-[10px] opacity-60">未定义上下文长度</span>
                                </Show>
                              </div>
                            </div>
                          )}
                        </For>
                      </div>
                    </div>
                  </Show>
                </Card>
              </div>
            </Show>
            {/* 会话测试 */}
            <Show when={tab() === 'chat'}>
              <div class="space-y-4">
                <Card class="p-4 flex flex-wrap items-center justify-between gap-3">
                  <div class="flex items-center gap-3 flex-1 min-w-[240px]">
                    <span class="text-xs text-faint shrink-0">测试模型：</span>
                    <Select
                      class="flex-1 min-w-[200px]"
                      value={selectedModel()}
                      onChange={setSelectedModel}
                      options={[
                        { value: '', label: '默认模型（首个可用）' },
                        ...activeModels().map(m => ({
                          value: m.id || '',
                          label: m.name ? `${m.name} (${m.id})` : m.id || '',
                        })),
                      ]}
                    />
                  </div>
                  <div class="flex items-center gap-2">
                    <Show when={selectedModel()}>
                      <Badge tone="blue">{selectedModel()}</Badge>
                    </Show>
                    <Button size="sm" variant="ghost" onClick={() => setChatHistory([])}>清空对话</Button>
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
                    fallback={<Empty message={`向 ${c().name || c().provider} 发送一条消息，验证该连接的连通性与模型输出。`} />}
                  >
                    <For each={chatHistory()}>
                      {t => (
                        <div class={`flex flex-col ${t.role === 'user' ? 'items-end' : 'items-start'}`}>
                          <div class={`max-w-[80%] px-4 py-2.5 rounded-2xl text-sm whitespace-pre-wrap leading-relaxed shadow-xs ${
                            t.role === 'user'
                              ? 'bg-accent text-on-accent'
                              : 'bg-hover text-foreground border border-subtle'
                          }`}>
                            {t.content}
                          </div>
                          <Show when={t.role === 'assistant' && t.servedModel}>
                            <span class="mt-1 px-1.5 py-0.5 text-[10px] text-faint font-mono">
                              由节点 {t.servedModel} 响应
                            </span>
                          </Show>
                        </div>
                      )}
                    </For>
                    <Show when={chatBusy()}>
                      <div class="flex justify-start">
                        <div class="px-4 py-2.5 rounded-2xl bg-hover text-sm text-faint flex items-center gap-2">
                          <span class="inline-block w-2 h-2 rounded-full bg-accent animate-pulse" />
                          思考与接收回复中…
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
                    placeholder="输入测试提示词，回车或点击发送…"
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
                    发送
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
        title={`编辑模型元数据 - ${editingModel()?.id || ''}`}
        onClose={() => setEditingModel(null)}
      >
        <div class="space-y-4">
          <Field label="模型 ID" hint="路由匹配唯一标识（不可修改）">
            <Input value={editingModel()?.id || ''} disabled class="bg-hover font-mono opacity-80" />
          </Field>
          <Field label="显示名称" hint="对外展现的模型友好名称">
            <Input value={editDisplayName()} onInput={setEditDisplayName} placeholder="例如 GPT-4o Custom" />
          </Field>
          <div class="grid grid-cols-2 gap-3">
            <Field label="上下文长度 (Tokens)" hint="例如 128000 或 200000">
              <Input
                type="number"
                value={editContextLength()}
                onInput={setEditContextLength}
                placeholder="128000"
              />
            </Field>
            <Field label="最大输出 (Tokens)" hint="例如 8192 或 16384">
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
                  恢复官方默认
                </Button>
              </Show>
            </div>
            <div class="flex items-center gap-2">
              <Button size="sm" variant="secondary" onClick={() => setEditingModel(null)}>
                取消
              </Button>
              <Button size="sm" variant="primary" loading={savingMeta()} onClick={handleSaveModelMeta}>
                保存元数据
              </Button>
            </div>
          </div>
        </div>
      </Modal>

      {/* 添加新账号 / 凭证弹窗 */}
      <Modal
        open={addAccountOpen()}
        title={`为此供应商添加新账号 - ${conn()?.name || conn()?.provider}`}
        onClose={() => setAddAccountOpen(false)}
      >
        <div class="space-y-4">
          <div class="p-3 rounded-control bg-hover border border-subtle text-xs text-faint leading-relaxed">
            支持接入多个账户。网关将基于优先级依次调度、实现故障转移（Fallback 容灾）与分摊 Token 限流。
          </div>
          <Field label="账号名称" hint="例如：主账号、备用 PAT、Team B 商业号">
            <Input
              value={newAccountName()}
              onInput={setNewAccountName}
              placeholder={`我的 ${conn()?.provider} 备用账号`}
            />
          </Field>
          <Field label="认证方式" hint="选择凭证模式">
            <Select
              value={newAccountAuthType()}
              options={accountAuthOptions()}
              onChange={setNewAccountAuthType}
            />
          </Field>
          <Show when={newAccountAuthType() === 'api-key'}>
            <Field
              label={conn()?.provider === 'qoder' ? 'Personal Access Token (PAT) / API Key' : 'API Key / 访问凭据'}
              hint="凭据将安全加密存储"
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
                      ? '输入 OpenCode Go/Zen 套餐 API Key'
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
                      前往官方控制台获取 Key ↗
                    </a>
                  </div>
                </Show>
              </div>
            </Field>
          </Show>
          <Show when={newAccountAuthType() === 'oauth'}>
            <div class="p-3 rounded-control bg-accent/10 border border-accent/30 text-xs text-accent space-y-1">
              <div class="font-medium flex items-center gap-1.5"><IconCheck size={14} /> OAuth 授权模式</div>
              <p class="text-faint">保存创建后可直接在当前页发起设备码一键授权，新标签页登录后自动完成绑定。</p>
            </div>
          </Show>
          <Field label="调度优先级" hint="数值越小越优先调度。主账号设为 10，备用账号设为 20">
            <Input
              type="number"
              value={newAccountPriority()}
              onInput={setNewAccountPriority}
              class="!w-32"
            />
          </Field>
          <div class="flex items-center justify-end gap-2 pt-3 border-t border-subtle">
            <Button size="sm" variant="secondary" onClick={() => setAddAccountOpen(false)}>
              取消
            </Button>
            <Button size="sm" variant="primary" loading={addingAccount()} onClick={handleAddAccountSubmit}>
              保存账号
            </Button>
          </div>
        </div>
      </Modal>
      {/* 统一设备码 OAuth 授权弹窗 (9router 同款弹窗体验) */}
      <Modal
        open={!!deviceFlow() || devicePolling() || isImportFlow() || !!deviceError()}
        title={isImportFlow() ? `导入 ${conn()?.name || conn()?.provider || '供应商'} 访问令牌` : `连接 ${conn()?.name || conn()?.provider || '供应商'}`}
        onClose={cancelDeviceFlow}
      >
        <div class="space-y-4 py-2">
          {/* 1. 纯错误展示（未处于导入流程时优先接管展示与重试） */}
          <Show when={deviceError() && !isImportFlow()}>
            <div class="space-y-4 py-2">
              <Alert variant="danger" title="授权遇到问题">
                {deviceError()}
              </Alert>
              <div class="flex justify-end gap-2">
                <Button size="sm" variant="secondary" onClick={cancelDeviceFlow}>
                  关闭
                </Button>
                <Button size="sm" variant="primary" onClick={startDeviceFlow}>
                  重试
                </Button>
              </div>
            </div>
          </Show>

          {/* 2. 令牌导入流程 (如 Cursor) */}
          <Show when={isImportFlow()}>
            <div class="space-y-3 text-left py-1">
              <p class="text-xs text-faint leading-relaxed">
                该提供商为令牌导入模式，请填入从本地环境或配置提取的访问凭据 (Access Token / Session Token)：
              </p>
              <div class="space-y-1.5">
                <textarea
                  rows={4}
                  value={importTokenText()}
                  onInput={e => setImportTokenText(e.currentTarget.value)}
                  placeholder="粘贴 Access Token / JWT..."
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
                  取消
                </Button>
                <Button
                  size="sm"
                  variant="primary"
                  loading={importingToken()}
                  disabled={!importTokenText().trim()}
                  onClick={handleImportToken}
                >
                  导入并保存
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
                  <p class="text-xs text-faint">正在向上游申请授权验证码...</p>
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
                        <div class="text-xs text-faint">已在浏览器新标签页中打开授权网页，请在网页中完成登录并确认授权：</div>
                        <div class="flex items-center justify-center gap-2">
                          <Button
                            size="sm"
                            variant="secondary"
                            onClick={() => {
                              const url = flow().verificationUriComplete || flow().verificationUri
                              if (url) window.open(url, '_blank')
                            }}
                          >
                            打开授权网页 ↗
                          </Button>
                        </div>
                      </div>
                    }
                  >
                    <p class="text-xs text-faint leading-relaxed">
                      请访问下方登录授权网址并在浏览器中确认授权：
                    </p>
                    {/* 验证码卡片 */}
                    <div class="p-4 rounded-card bg-accent/10 border border-accent/30 space-y-1.5">
                      <div class="text-[11px] text-faint font-medium">Your Code / 授权验证码</div>
                      <div class="flex items-center justify-center gap-3">
                        <span class="font-mono text-2xl sm:text-3xl font-bold text-accent tracking-widest select-all">
                          {flow().userCode}
                        </span>
                        <button
                          type="button"
                          class="p-1.5 rounded hover:bg-accent/20 text-accent transition-colors cursor-pointer"
                          title="复制验证码"
                          onClick={() => {
                            if (flow().userCode) {
                              navigator.clipboard.writeText(flow().userCode!)
                              setCopiedCode(true)
                              toast.success('验证码已复制')
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
                            toast.success('登录网址已复制')
                          }
                        }}
                      >
                        复制网址
                      </Button>
                      <Button
                        size="sm"
                        variant="secondary"
                        onClick={() => {
                          const url = flow().verificationUriComplete || flow().verificationUri
                          if (url) window.open(url, '_blank')
                        }}
                      >
                        打开网页 ↗
                      </Button>
                    </div>
                  </div>

                  {/* 轮询等待状态 */}
                  <div class="flex items-center justify-center gap-2 pt-2 text-xs text-faint">
                    <span class="w-2.5 h-2.5 rounded-full border-2 border-accent border-t-transparent animate-spin" />
                    <span>Waiting for authorization / 等待浏览器授权完成...</span>
                  </div>

                  <div class="pt-2 border-t border-subtle flex justify-end">
                    <Button size="sm" variant="secondary" onClick={cancelDeviceFlow}>
                      取消
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
