import { type Component, For, Show, createSignal, createResource, createEffect, onMount, onCleanup } from 'solid-js'
import { A, useParams } from '@solidjs/router'
import { useGatewayStore } from '@/stores/gateway'
import { api, apiPost } from '@/lib/api'
import type { Provider, ProviderModel } from '@/types/domain'
import { Card, Badge, Button, Input, Toggle, Field, Empty, Skeleton, Select, Modal, ProviderAvatar } from '@/components/ui'
const ProviderDetail: Component = () => {
  const params = useParams<{ id: string }>()
  const store = useGatewayStore()

  const [conn, setConn] = createSignal<Provider | null>(null)
  const [loading, setLoading] = createSignal(true)
  const [notFound, setNotFound] = createSignal(false)
  const [saving, setSaving] = createSignal(false)
  const [testing, setTesting] = createSignal(false)
  const [testResult, setTestResult] = createSignal<{ ok: boolean; msg: string } | null>(null)
  const [tab, setTab] = createSignal<'overview' | 'models' | 'oauth' | 'chat'>('overview')
  // Chat 测试状态
  const [selectedModel, setSelectedModel] = createSignal('')
  const [prompt, setPrompt] = createSignal('')
  const [chatBusy, setChatBusy] = createSignal(false)
  const [chatHistory, setChatHistory] = createSignal<Array<{ role: string; content: string }>>([])
  const [chatErr, setChatErr] = createSignal('')
  const [modelSearch, setModelSearch] = createSignal('')
  const [newCustomModelId, setNewCustomModelId] = createSignal('')
  let chatBoxRef: HTMLDivElement | undefined

  createEffect(() => {
    chatHistory()
    if (chatBoxRef) {
      setTimeout(() => {
        if (chatBoxRef) chatBoxRef.scrollTop = chatBoxRef.scrollHeight
      }, 50)
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
      const r = await apiPost('/v1/chat/completions', {
        model: fullModel,
        messages: nextHistory,
      }) as { choices?: Array<{ message?: { content?: string } }> } | null
      const reply = r?.choices?.[0]?.message?.content ?? '(无返回)'
      setChatHistory(h => [...h, { role: 'assistant', content: reply }])
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
      alert('请填写 API Key')
      return
    }
    setAddingAccount(true)
    try {
      await store.addProvider({
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
    } catch (e: unknown) {
      alert(e instanceof Error ? e.message : '添加账号失败')
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
  const [deviceSuccess, setDeviceSuccess] = createSignal(false)
  const [deviceError, setDeviceError] = createSignal('')
  const [copiedCode, setCopiedCode] = createSignal(false)
  let pollTimer: ReturnType<typeof setInterval> | undefined

  onCleanup(() => {
    if (pollTimer) clearInterval(pollTimer)
  })

  async function startDeviceFlow() {
    const p = conn()?.provider
    if (!p) return
    setDeviceError('')
    setDeviceSuccess(false)
    setDevicePolling(true)
    try {
      const res = (await apiPost(`/api/oauth/${p}/device-code`)) as {
        verificationUri: string
        verificationUriComplete?: string
        userCode?: string
        deviceCode?: string
        nonce?: string
        codeVerifier?: string
        machineId?: string
        expiresIn?: number
        interval?: number
      }
      setDeviceFlow(res)
      const targetUrl = res.verificationUriComplete || res.verificationUri
      if (targetUrl) {
        window.open(targetUrl, '_blank')
      }

      if (pollTimer) clearInterval(pollTimer)
      const intervalMs = res.interval ? res.interval * 1000 : 2500
      pollTimer = setInterval(async () => {
        try {
          const pollRes = (await apiPost(`/api/oauth/${p}/device-code/poll`, {
            deviceCode: res.deviceCode,
            nonce: res.nonce,
            codeVerifier: res.codeVerifier,
            machineId: res.machineId,
          })) as { success?: boolean; error?: string; pending?: boolean; connection?: any }

          if (pollRes?.success) {
            clearInterval(pollTimer)
            pollTimer = undefined
            setDevicePolling(false)
            setDeviceSuccess(true)
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
        }
      }, intervalMs)
    } catch (e: unknown) {
      setDevicePolling(false)
      setDeviceError(e instanceof Error ? e.message : String(e))
    }
  }

  function cancelDeviceFlow() {
    if (pollTimer) {
      clearInterval(pollTimer)
      pollTimer = undefined
    }
    setDevicePolling(false)
    setDeviceFlow(null)
  }

  async function load() {
    setLoading(true)
    setNotFound(false)
    try {
      const r = (await api(`/api/providers/${params.id}`)) as Provider | null
      if (!r) {
        setNotFound(true)
        return
      }
      setConn(r)
      setName(r.name || '')
      setPriority(String(r.priority ?? 0))
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

  onMount(async () => {
    await load()
    if (store.providers().length === 0) {
      await store.loadProvidersOnly()
    }
    const hash = window.location.hash
    if (hash.includes('tab=')) {
      const t = hash.split('tab=')[1]?.split('&')[0]
      if (t && ['overview', 'models', 'oauth', 'chat'].includes(t)) {
        setTab(t as any)
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
  const [oauthStatus, { refetch: refetchOAuth }] = createResource(
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
    await store.addProviderModel(params.id, { id: modelId.trim(), name: modelName.trim() || modelId.trim() })
    refetchModels()
  }

  async function removeCustomModel(modelId: string) {
    await store.deleteProviderModel(params.id, modelId)
    refetchModels()
  }

  return (
    <div class="space-y-5 stagger">
      {/* 顶部吸顶区：保证「← 返回连接列表」与操作栏永远触手可及 */}
      <div class="sticky top-16 z-20 -mx-4 lg:-mx-10 px-4 lg:px-10 py-3 bg-bg/85 backdrop-blur-xl border-b border-subtle">
        <A href="/providers" class="text-xs text-faint hover:text-accent inline-flex items-center gap-1 mb-1">
          ← 返回连接列表
        </A>
        <Show when={!loading() && conn()}>
          {c => (
            <div class="flex items-center gap-3 mt-1 flex-wrap">
              <h1 class="text-xl font-semibold">{c().name || c().provider}</h1>
              <Badge tone={c().isActive ? 'green' : 'gray'}>{c().isActive ? '启用' : '停用'}</Badge>
              <Badge tone="blue">{c().authType}</Badge>
              <span class="text-xs text-faint font-mono">{c().id.slice(0, 12)}…</span>
              <div class="ml-auto flex items-center gap-3">
                <Button size="sm" variant="secondary" loading={testing()} onClick={runTest}>测试连接</Button>
                <Toggle checked={c().isActive} onChange={() => { store.toggleProvider(c()); load() }} />
              </div>
            </div>
          )}
        </Show>
        <Show when={testResult()}>
          <div class={`mt-2 text-xs px-3 py-1.5 rounded-control inline-block ${testResult()!.ok
            ? 'bg-success/10 text-success'
            : 'bg-danger/10 text-danger'}`}>
            {testResult()!.msg}
          </div>
        </Show>
      </div>

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
                ...(c().authType === 'oauth' || (modelsData().authModes?.includes('oauth') ?? false) || (regInfo()?.authModes?.includes('oauth') ?? false) || regInfo()?.category === 'oauth'
                  ? [{ id: 'oauth' as const, label: '授权管理' }]
                  : []),
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
              <div class="space-y-4">
                {/* 供应商名下多账号与调度看板 */}
                <Card class="p-5 space-y-3.5">
                  <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-3 pb-2 border-b border-subtle">
                    <div>
                      <h3 class="text-sm font-semibold flex items-center gap-2">
                        <span>已绑定的账号 / 凭证池</span>
                        <Badge tone="blue">{accounts().length} 个账号</Badge>
                      </h3>
                      <p class="text-xs text-faint mt-0.5">
                        基于账号优先级调度。数值越小越优先，限流时自动 Fallback 故障转移
                      </p>
                    </div>
                    <Button
                      size="sm"
                      variant="primary"
                      onClick={() => setAddAccountOpen(true)}
                    >
                      + 添加备用账号 / 凭证
                    </Button>
                  </div>

                  {/* 调度与容灾说明 Banner */}
                  <div class="p-3 rounded-control border border-accent/25 bg-accent/5 text-xs text-text space-y-1">
                    <div class="font-medium text-accent flex items-center gap-1.5">
                      <span>💡</span> 多账号 Fallback 故障转移与负载均衡机制
                    </div>
                    <p class="text-faint leading-relaxed">
                      当最高优先级账号（主账号）触发 429 限流或额度耗尽时，Cyrene Gateway 会自动 Fallback 切换至备用账号；同一优先级的多个账号自动进行 Round-Robin 均衡分摊并发压力与 Token 额度。
                    </p>
                  </div>

                  {/* 账号列表卡片 */}
                  <div class="grid gap-2">
                    <For each={accounts()}>
                      {(acc, idx) => {
                        const isCurrent = () => acc.id === c().id
                        const cooling = () => !!acc.data?.rateLimitedUntil

                        return (
                          <div class={`flex flex-col sm:flex-row sm:items-center justify-between gap-3 p-3 rounded-control border transition-all ${
                            isCurrent()
                              ? 'border-accent/60 bg-accent/10 shadow-xs'
                              : acc.isActive
                              ? 'border-subtle bg-bg-elevated/60'
                              : 'border-subtle/40 bg-bg-elevated/20 opacity-60'
                          }`}>
                            <div class="flex items-center gap-3 min-w-0">
                              <span class="text-xs font-mono font-bold px-1.5 py-0.5 rounded bg-bg border border-subtle shrink-0">
                                #{idx() + 1}
                              </span>
                              <div class="min-w-0">
                                <div class="flex items-center gap-2 flex-wrap">
                                  <span class="text-xs font-semibold text-foreground truncate">
                                    {acc.name || acc.provider}
                                  </span>
                                  <Show when={isCurrent()}>
                                    <span class="text-[10px] px-1.5 py-0.2 rounded bg-accent text-white font-medium">
                                      当前配置中
                                    </span>
                                  </Show>
                                  <Badge tone="blue" class="text-[10px] px-1.5 py-0">
                                    {acc.authType === 'api-key' || acc.authType === 'apikey' ? 'API Key' : acc.authType === 'oauth' ? 'OAuth' : acc.authType}
                                  </Badge>
                                  <span class="text-[11px] font-mono px-1.5 py-0.5 rounded bg-bg text-faint border border-subtle">
                                    优先级 {acc.priority} {idx() === 0 ? '(主)' : '(备用)'}
                                  </span>
                                  <Show when={cooling()}>
                                    <Badge tone="amber" class="text-[10px]">限流冷却中</Badge>
                                  </Show>
                                </div>
                                <div class="text-[11px] text-faint font-mono mt-0.5 flex items-center gap-2 flex-wrap">
                                  <Show when={acc.email}>
                                    <span>邮箱: {acc.email}</span>
                                    <span>·</span>
                                  </Show>
                                  <Show when={acc.data?.credentialHint}>
                                    <span>凭证: {String(acc.data?.credentialHint)}</span>
                                    <span>·</span>
                                  </Show>
                                  <span class="opacity-60">ID: {acc.id.slice(0, 8)}...</span>
                                </div>
                              </div>
                            </div>

                            <div class="flex items-center gap-2 self-end sm:self-auto shrink-0">
                              <Show when={!isCurrent()}>
                                <A href={`/providers/${acc.id}`}>
                                  <Button size="sm" variant="secondary">
                                    切换编辑
                                  </Button>
                                </A>
                              </Show>
                              <Toggle
                                checked={acc.isActive}
                                onChange={async () => {
                                  await store.toggleProvider(acc)
                                  await store.loadProvidersOnly()
                                  await load()
                                }}
                              />
                              <Button
                                size="sm"
                                variant="danger"
                                onClick={async () => {
                                  if (confirm(`确定要删除账号「${acc.name || acc.provider}」吗？`)) {
                                    await store.deleteProvider(acc)
                                    await store.loadProvidersOnly()
                                    if (isCurrent()) {
                                      const remaining = accounts().filter(a => a.id !== acc.id)
                                      if (remaining.length > 0) {
                                        window.location.href = `#/providers/${remaining[0].id}`
                                      } else {
                                        window.location.href = '#/providers'
                                      }
                                    }
                                  }
                                }}
                              >
                                删除
                              </Button>
                            </div>
                          </div>
                        )
                      }}
                    </For>
                  </div>
                </Card>

                {/* 当前选中账号的详细连接配置 */}
                <Card class="p-5 space-y-4">
                  <div class="text-sm font-semibold pb-2 border-b border-subtle">
                    当前账号详情与协议配置 ({c().name || c().provider})
                  </div>
                  <Field label="账号显示名称">
                    <Input value={name()} onInput={setName} placeholder="便于识别的名称" />
                  </Field>
                  <Field label="调度优先级" hint="数值越小越优先被调度。支持设置先后顺序依次消耗额度">
                    <Input type="number" value={priority()} onInput={setPriority} class="!w-32" />
                  </Field>
                  <Show when={c().authType === 'api-key' || c().authType === 'apikey' || (modelsData().authModes?.includes('api-key') ?? false)}>
                    <Field
                      label="API Key / 凭据"
                      hint={
                        c().authType === 'none'
                          ? '当前处于免密体验模式。填入 API Key 后点击保存即可升级为商业授权模式，解锁全量商业模型。'
                          : '留空表示不修改；密钥写入后不在界面回显'
                      }
                    >
                      <Input
                        type="password"
                        value={apiKey()}
                        onInput={setApiKey}
                        placeholder={c().authType === 'none' ? '输入商业授权 API Key' : 'sk-...'}
                      />
                    </Field>
                  </Show>
                  <Field label="Base URL" hint="自定义端点，留空使用默认">
                    <Input value={baseUrl()} onInput={setBaseUrl} placeholder="https://api.example.com/v1" />
                  </Field>

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
                          hint="用于应对上游客户端强制校验新版本号（例如 Antigravity、Copilot、CodeBuddy 等）。此处指定的 Header 会覆盖默认 Header 发送给上游。"
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
                      当前凭证：<span class="font-mono">{String(c().data?.credentialHint ?? '')}</span>
                      <Show when={c().data?.hasAccessToken}> · Access Token 已配置</Show>
                      <Show when={c().data?.hasRefreshToken}> · Refresh Token 已配置</Show>
                    </div>
                  </Show>

                  <div class="flex justify-between pt-1">
                    <Button variant="danger" onClick={async () => { await store.deleteProvider(c()); history.back() }}>
                      删除此连接
                    </Button>
                    <Button variant="primary" loading={saving()} onClick={save}>保存当前配置</Button>
                  </div>
                </Card>
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
                              ✎
                            </button>
                            <Toggle
                              checked={m.enabled !== false}
                              onChange={() => toggleModel(m.id || m.name, m.enabled !== false)}
                            />
                            <button class="text-faint hover:text-danger ml-0.5" title="删除" onClick={() => removeCustomModel(m.id || m.name)}>×</button>
                          </span>
                        )}
                      </For>
                    </Show>
                  </div>
                </Card>

                <Card class="p-5 flex flex-col">
                  <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-3 mb-3">
                    <div class="flex items-center gap-2">
                      <h3 class="text-sm font-semibold">可用模型</h3>
                      <span class="text-xs text-faint">
                        共 {(modelsData().registryModels ?? models()?.registryModels ?? []).length} 个 · 开放中 {(modelsData().registryModels ?? models()?.registryModels ?? []).filter(m => m.enabled !== false).length} 个
                      </span>
                    </div>
                    <div class="w-full sm:w-64">
                      <Input
                        value={modelSearch()}
                        onInput={setModelSearch}
                        placeholder="搜索模型 ID 或名称…"
                        size="sm"
                      />
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
                                      <span class="px-1.5 py-0.2 rounded text-[10px] font-medium bg-emerald-500/15 text-emerald-400 border border-emerald-500/30 shrink-0">
                                        免费
                                      </span>
                                    </Show>
                                    <Show when={m.enabled === false}>
                                      <span class="px-1.5 py-0.2 rounded text-[10px] font-medium bg-zinc-500/15 text-zinc-400 border border-zinc-500/30 shrink-0">
                                        不对外
                                      </span>
                                    </Show>
                                    <Show when={m.hasOverride}>
                                      <span class="px-1.5 py-0.2 rounded text-[10px] font-medium bg-accent/15 text-accent border border-accent/30 shrink-0" title="包含用户自定义元数据">
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
                                        🔒
                                      </span>
                                    }
                                  >
                                    <button
                                      type="button"
                                      class="text-faint hover:text-accent p-1 text-xs rounded transition-colors"
                                      title="编辑模型元数据（名称、上下文长度等）"
                                      onClick={() => startEditModel(m)}
                                    >
                                      ✎
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
                        <div class={`flex ${t.role === 'user' ? 'justify-end' : 'justify-start'}`}>
                          <div class={`max-w-[80%] px-4 py-2.5 rounded-2xl text-sm whitespace-pre-wrap leading-relaxed shadow-xs ${
                            t.role === 'user'
                              ? 'bg-accent text-on-accent'
                              : 'bg-hover text-foreground border border-subtle'
                          }`}>
                            {t.content}
                          </div>
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
                  <div class="px-3.5 py-2.5 rounded-xl text-xs bg-danger/10 text-danger border border-danger/20">
                    {chatErr()}
                  </div>
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
            {/* 授权管理 */}
            <Show when={tab() === 'oauth'}>
              <div class="space-y-4">
                <Card class="p-5 space-y-4">
                  <div class="flex items-center justify-between gap-3 pb-3 border-b border-subtle">
                    <div>
                      <h3 class="text-sm font-semibold flex items-center gap-2">
                        <span>OAuth 快捷授权登录</span>
                        <Badge tone="blue">{oauthStatus()?.flowType || 'device_code'}</Badge>
                      </h3>
                      <p class="text-xs text-faint mt-0.5">
                        通过官方设备码或浏览器单点登录，自动完成凭据颁发与定期刷新
                      </p>
                    </div>
                    <Button size="sm" variant="secondary" onClick={() => refetchOAuth()}>
                      刷新授权状态
                    </Button>
                  </div>

                  {/* 设备码登录工作流 (Qoder / GitHub / Grok-CLI 等) */}
                  <Show when={oauthStatus()?.flowType === 'device_code' || !oauthStatus()?.flowType || (regInfo()?.deviceCodeUrl)}>
                    <div class="p-4 rounded-control border border-accent/30 bg-accent/5 space-y-4">
                      <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-3">
                        <div>
                          <div class="font-semibold text-sm text-foreground">设备码授权登录 (Device Code Flow)</div>
                          <div class="text-xs text-faint mt-0.5">
                            点击发起后将在新标签页打开官方授权页面，填入/确认验证码后即可自动接入为新账号
                          </div>
                        </div>
                        <Show when={!devicePolling()}>
                          <Button
                            size="sm"
                            variant="primary"
                            onClick={startDeviceFlow}
                          >
                            发起设备码登录
                          </Button>
                        </Show>
                      </div>

                      {/* 正在进行中的设备码授权卡片 */}
                      <Show when={deviceFlow()}>
                        {flow => (
                          <div class="p-4 rounded-control bg-bg-elevated border border-subtle shadow-sm space-y-3.5">
                            <div class="flex items-center justify-between gap-2">
                              <span class="text-xs font-semibold text-accent flex items-center gap-1.5">
                                <span class="animate-pulse">●</span> 等待在浏览器中确认授权
                              </span>
                              <Button size="sm" variant="secondary" onClick={cancelDeviceFlow}>
                                取消授权
                              </Button>
                            </div>

                            <div class="grid sm:grid-cols-2 gap-3 pt-1">
                              <div class="p-3 rounded bg-hover border border-subtle space-y-1">
                                <div class="text-[11px] text-faint">步骤 1: 打开授权网址</div>
                                <a
                                  href={flow().verificationUriComplete || flow().verificationUri}
                                  target="_blank"
                                  rel="noreferrer"
                                  class="text-xs text-accent underline font-mono break-all font-semibold"
                                >
                                  {flow().verificationUri} ↗
                                </a>
                              </div>

                              <Show when={flow().userCode}>
                                <div class="p-3 rounded bg-hover border border-subtle space-y-1">
                                  <div class="text-[11px] text-faint">步骤 2: 验证码 (若提示要求输入)</div>
                                  <div class="flex items-center justify-between gap-2">
                                    <span class="font-mono text-base font-bold text-foreground tracking-wider">
                                      {flow().userCode}
                                    </span>
                                    <Button
                                      size="sm"
                                      variant="secondary"
                                      onClick={() => {
                                        if (flow().userCode) {
                                          navigator.clipboard.writeText(flow().userCode!)
                                          setCopiedCode(true)
                                          setTimeout(() => setCopiedCode(false), 2000)
                                        }
                                      }}
                                    >
                                      {copiedCode() ? '已复制 ✓' : '复制验证码'}
                                    </Button>
                                  </div>
                                </div>
                              </Show>
                            </div>

                            <div class="text-xs text-faint flex items-center gap-2 pt-1">
                              <span class="inline-block w-2 h-2 rounded-full bg-accent animate-ping" />
                              <span>网关正在自动轮询等待授权结果，完成授权后将自动关闭并入库...</span>
                            </div>
                          </div>
                        )}
                      </Show>

                      {/* 授权成功提示 */}
                      <Show when={deviceSuccess()}>
                        <div class="p-3 rounded bg-emerald-500/10 border border-emerald-500/30 text-xs text-emerald-400 font-medium flex items-center justify-between">
                          <span>✓ 设备码授权成功！已自动添加并刷新该供应商的账号列表。</span>
                          <Button size="sm" variant="secondary" onClick={() => setDeviceSuccess(false)}>
                            知道了
                          </Button>
                        </div>
                      </Show>

                      {/* 授权错误提示 */}
                      <Show when={deviceError()}>
                        <div class="p-3 rounded bg-red-500/10 border border-red-500/30 text-xs text-red-400 font-medium flex items-center justify-between">
                          <span>授权失败: {deviceError()}</span>
                          <Button size="sm" variant="secondary" onClick={() => setDeviceError('')}>
                            关闭
                          </Button>
                        </div>
                      </Show>
                    </div>
                  </Show>

                  {/* Token / PAT 导入 (适用于 Qoder PAT pt-... 或 Access Token) */}
                  <div class="pt-3 border-t border-subtle space-y-2">
                    <div class="text-xs font-semibold text-foreground">手动凭证 / Token 导入</div>
                    <div class="text-xs text-faint leading-relaxed">
                      适用于 Personal Access Token (如 Qoder 的 <code class="font-mono text-accent">pt-...</code> 密钥，将自动兑换为短期 Job Token 进行 COSY 签名) 或自建代理获取的 Access Token。
                    </div>
                    <TokenImport provider={c().provider} onDone={() => { refetchOAuth(); load(); store.loadProvidersOnly() }} />
                  </div>
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
              options={[
                { value: 'api-key', label: 'API Key / 个人凭据 (PAT)' },
                { value: 'oauth', label: 'OAuth 授权模式' },
                { value: 'none', label: '免密模式' },
              ]}
              onChange={setNewAccountAuthType}
            />
          </Field>
          <Show when={newAccountAuthType() === 'api-key'}>
            <Field label="API Key / 凭据 (PAT)" hint="凭据将安全加密存储">
              <Input
                type="password"
                value={newAccountApiKey()}
                onInput={setNewAccountApiKey}
                placeholder={conn()?.provider === 'qoder' ? 'pt-... (Personal Access Token)' : 'sk-...'}
              />
            </Field>
          </Show>
          <Show when={newAccountAuthType() === 'oauth'}>
            <div class="p-3 rounded-control bg-accent/10 border border-accent/30 text-xs text-accent space-y-1">
              <div class="font-medium">✓ OAuth 授权模式</div>
              <p class="text-faint">创建后可直接切换到「授权管理」Tab 发起设备码一键登录。</p>
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
    </div>
  )
}

function TokenImport(props: { provider: string; onDone: () => void }) {
  const store = useGatewayStore()
  const [token, setToken] = createSignal('')
  const [busy, setBusy] = createSignal(false)
  const [msg, setMsg] = createSignal('')

  async function submit() {
    if (!token().trim()) return
    setBusy(true); setMsg('')
    try {
      await store.oauthImport(props.provider, { accessToken: token().trim() })
      setMsg('导入成功'); setToken(''); props.onDone()
    } catch (e: unknown) {
      setMsg(e instanceof Error ? e.message : '导入失败')
    } finally { setBusy(false) }
  }

  return (
    <div class="space-y-2">
      <Input type="password" value={token()} onInput={setToken} placeholder="粘贴 access token" />
      <div class="flex items-center gap-2">
        <Button size="sm" variant="primary" loading={busy()} onClick={submit}>导入</Button>
        <Show when={msg()}><span class="text-xs text-faint">{msg()}</span></Show>
      </div>
    </div>
  )
}

export default ProviderDetail
