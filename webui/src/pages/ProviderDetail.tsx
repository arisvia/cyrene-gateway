import { type Component, For, Show, createSignal, createResource, createEffect, onMount } from 'solid-js'
import { A, useParams } from '@solidjs/router'
import { useGatewayStore } from '@/stores/gateway'
import { api, apiPost } from '@/lib/api'
import type { Provider, ProviderModel } from '@/types/domain'
import { Card, Badge, Button, Input, Toggle, Field, Empty, Skeleton, Select, Modal } from '@/components/ui'
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

  async function load() {
    setLoading(true)
    setNotFound(false)
    try {
      const r = await api(`/api/providers/${params.id}`) as Provider | null
      if (!r) { setNotFound(true); return }
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

  onMount(load)

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
                { id: 'overview' as const, label: '连接配置' },
                { id: 'models' as const, label: '可用模型' },
                { id: 'chat' as const, label: '会话测试' },
                ...(c().authType === 'oauth' || (modelsData().authModes?.includes('oauth') ?? false)
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

            {/* 配置 */}
            <Show when={tab() === 'overview'}>
              <Card class="p-5 space-y-4">
                <Field label="显示名称">
                  <Input value={name()} onInput={setName} placeholder="便于识别的名称" />
                </Field>
                <Field label="优先级" hint="数值越小越优先被调度">
                  <Input type="number" value={priority()} onInput={setPriority} class="!w-32" />
                </Field>
                <Show when={c().authType === 'api-key' || (modelsData().authModes?.includes('api-key') ?? false)}>
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
                      placeholder={c().authType === 'none' ? '输入商业授权 API Key' : 'sk-…'}
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
                    删除连接
                  </Button>
                  <Button variant="primary" loading={saving()} onClick={save}>保存</Button>
                </div>
              </Card>
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
            {/* 授权 */}
            <Show when={tab() === 'oauth'}>
              <Card class="p-5 space-y-3">
                <Show when={oauthStatus()} fallback={<Empty message="该提供商无需 OAuth 授权。" />}>
                  {oa => (
                    <div class="space-y-3">
                      <div class="flex items-center gap-2 flex-wrap">
                        <span class="text-sm">授权方式：</span>
                        <Badge tone="blue">{oa().flowType || 'none'}</Badge>
                        <Button size="sm" variant="secondary" onClick={() => refetchOAuth()}>刷新状态</Button>
                      </div>
                      <Show when={(oa().connections ?? []).length > 0}>
                        <div class="text-xs text-faint">已授权 {oa().connections!.length} 个账号</div>
                      </Show>
                      <Show when={oa().flowType === 'import_token'}>
                        <div class="pt-2 border-t border-subtle">
                          <div class="text-xs text-faint mb-2">导入 Token（适用于 device-code / 手动获取的场景）</div>
                          <TokenImport provider={c().provider} onDone={() => { refetchOAuth(); load() }} />
                        </div>
                      </Show>
                    </div>
                  )}
                </Show>
              </Card>
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
