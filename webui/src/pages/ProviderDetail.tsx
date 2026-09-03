import { type Component, For, Show, createSignal, createResource, createEffect, onMount } from 'solid-js'
import { A, useParams } from '@solidjs/router'
import { useGatewayStore } from '@/stores/gateway'
import { api, apiPost } from '@/lib/api'
import type { Provider, ProviderModel } from '@/types/domain'
import { Card, Badge, Button, Input, Toggle, Field, Empty, Skeleton } from '@/components/ui'

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
      // 如果未指定具体模型，使用该提供商第一个可用模型或前缀通配
      const available = (models()?.registryModels ?? []).concat(models()?.customModels ?? [])
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

  async function load() {
    setLoading(true)
    setNotFound(false)
    try {
      const r = await api(`/api/providers/${params.id}`) as Provider | null
      if (!r) { setNotFound(true); return }
      setConn(r)
      setName(r.name || '')
      setPriority(String(r.priority ?? 0))
      const d = r.data as { baseUrl?: string } | undefined
      setBaseUrl(d?.baseUrl || '')
    } catch {
      setNotFound(true)
    } finally {
      setLoading(false)
    }
  }

  onMount(load)

  const [models, { refetch: refetchModels }] = createResource(
    () => params.id,
    async id => {
      try {
        const r = await api(`/api/providers/${id}/models`)
        return r as { registryModels?: ProviderModel[]; customModels?: ProviderModel[] }
      } catch { return { registryModels: [], customModels: [] } }
    },
  )

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

  async function save() {
    setSaving(true)
    try {
      const current = conn()
      const patch: Partial<Provider> & { data?: Record<string, unknown> } = { name: name(), priority: Number(priority()) || 0 }
      const currData = (current?.data || {}) as Record<string, unknown>
      if (apiKey()) patch.data = { ...currData, apiKey: apiKey() }
      if (baseUrl()) patch.data = { ...(patch.data || currData), baseUrl: baseUrl() }
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
                { id: 'oauth' as const, label: '授权管理' },
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
                <Show when={c().authType === 'api-key'}>
                  <Field label="API Key" hint="留空表示不修改；密钥写入后不在界面回显">
                    <Input type="password" value={apiKey()} onInput={setApiKey} placeholder="sk-…" />
                  </Field>
                </Show>
                <Field label="Base URL" hint="自定义端点，留空使用默认">
                  <Input value={baseUrl()} onInput={setBaseUrl} placeholder="https://api.example.com/v1" />
                </Field>

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
                <Card class="p-5">
                  <div class="flex items-center justify-between mb-3">
                    <div>
                      <h3 class="text-sm font-semibold">自定义模型</h3>
                      <p class="text-xs text-faint mt-0.5">注册未在官方列表中的模型 ID，供网关路由匹配</p>
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
                    <Show when={(models()?.customModels?.length ?? 0) > 0} fallback={
                      <span class="text-xs text-faint">暂无自定义模型</span>
                    }>
                      <For each={models()?.customModels ?? []}>
                        {m => (
                          <span class="inline-flex items-center gap-1.5 px-2 py-1 rounded-control bg-hover text-xs">
                            <span class="font-mono">{m.id || m.name}</span>
                            <button class="text-faint hover:text-danger" onClick={() => removeCustomModel(m.id || m.name)}>×</button>
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
                      <span class="text-xs text-faint">共 {models()?.registryModels?.length ?? 0} 个</span>
                    </div>
                    <div class="w-full sm:w-64">
                      <Input
                        value={modelSearch()}
                        onInput={setModelSearch}
                        placeholder="搜索可用模型 ID 或名称…"
                        class="h-8 text-xs"
                      />
                    </div>
                  </div>

                  <Show
                    when={(models()?.registryModels?.length ?? 0) > 0}
                    fallback={<Empty message="该提供商未上报模型列表。" />}
                  >
                    {/* 独立可滚动区域：避免整个页面下拉，使上方自定义模型常驻可见 */}
                    <div class="max-h-[380px] overflow-y-auto pr-1">
                      <div class="grid sm:grid-cols-2 lg:grid-cols-3 gap-2">
                        <For each={(models()?.registryModels ?? []).filter(m => {
                          const q = modelSearch().trim().toLowerCase()
                          if (!q) return true
                          return (m.name || '').toLowerCase().includes(q) || (m.id || '').toLowerCase().includes(q)
                        })}>
                          {m => (
                            <div class="px-3 py-2 rounded-control border border-subtle bg-bg-elevated/60 hover:bg-hover transition-colors">
                              <div class="text-sm truncate font-medium text-foreground">{m.name || m.id}</div>
                              <div class="text-[11px] text-faint font-mono truncate">{m.id || m.name}</div>
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
                    <select
                      class="h-9 px-3 text-xs bg-bg border border-subtle rounded-control flex-1 focus:border-accent focus:outline-none"
                      value={selectedModel()}
                      onChange={e => setSelectedModel(e.currentTarget.value)}
                    >
                      <option value="">默认模型（首个可用）</option>
                      <For each={(models()?.registryModels ?? []).concat(models()?.customModels ?? [])}>
                        {m => (
                          <option value={m.id}>{m.name ? `${m.name} (${m.id})` : m.id}</option>
                        )}
                      </For>
                    </select>
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
