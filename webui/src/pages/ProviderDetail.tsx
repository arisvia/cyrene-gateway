import { type Component, For, Show, createSignal, createResource, onMount } from 'solid-js'
import { A, useParams } from '@solidjs/router'
import { useGatewayStore } from '@/stores/gateway'
import { api, apiPost } from '@/lib/api'
import { Card, Badge, Button, Input, Toggle, Field, Empty, Skeleton } from '@/components/ui'

const ProviderDetail: Component = () => {
  const params = useParams<{ id: string }>()
  const store = useGatewayStore()

  const [conn, setConn] = createSignal<any>(null)
  const [loading, setLoading] = createSignal(true)
  const [notFound, setNotFound] = createSignal(false)
  const [saving, setSaving] = createSignal(false)
  const [testing, setTesting] = createSignal(false)
  const [testResult, setTestResult] = createSignal<{ ok: boolean; msg: string } | null>(null)
  const [tab, setTab] = createSignal<'overview' | 'models' | 'oauth'>('overview')

  // 编辑字段
  const [name, setName] = createSignal('')
  const [apiKey, setApiKey] = createSignal('')
  const [baseUrl, setBaseUrl] = createSignal('')
  const [priority, setPriority] = createSignal('0')

  async function load() {
    setLoading(true)
    setNotFound(false)
    try {
      const r = await api(`/api/providers/${params.id}`)
      if (!r) { setNotFound(true); return }
      setConn(r)
      setName(r.name || '')
      setPriority(String(r.priority ?? 0))
      setBaseUrl(r.data?.baseUrl || '')
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
        return r as { registryModels?: any[]; customModels?: any[] }
      } catch { return { registryModels: [], customModels: [] } }
    },
  )

  const [oauthStatus, { refetch: refetchOAuth }] = createResource(
    () => conn()?.provider,
    async p => {
      if (!p) return null
      try { return await api(`/api/oauth/${p}/status`) } catch { return null }
    },
  )

  async function save() {
    setSaving(true)
    try {
      const patch: Record<string, any> = { name: name(), priority: Number(priority()) || 0 }
      if (apiKey()) patch.data = { ...(conn()?.data || {}), apiKey: apiKey() }
      if (baseUrl()) patch.data = { ...patch.data, baseUrl: baseUrl() }
      await store.updateProvider(params.id, patch)
      await load()
      setApiKey('')
    } catch (e: any) {
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
    } catch (e: any) {
      setTestResult({ ok: false, msg: e?.message || '请求失败' })
    } finally {
      setTesting(false)
    }
  }

  async function addCustomModel(modelId: string, modelName: string) {
    if (!modelId.trim()) return
    await store.addProviderModel(params.id, { id: modelId.trim(), name: modelName.trim() || undefined })
    refetchModels()
  }

  async function removeCustomModel(modelId: string) {
    await store.deleteProviderModel(params.id, modelId)
    refetchModels()
  }

  return (
    <div class="space-y-5">
      {/* 面包屑 + 标题 */}
      <div>
        <A href="/providers" class="text-xs text-faint hover:text-accent">← 返回连接列表</A>
        <Show when={!loading() && conn()}>
          <div class="flex items-center gap-3 mt-2 flex-wrap">
            <h1 class="text-xl font-semibold">{conn().name || conn().provider}</h1>
            <Badge tone={conn().isActive ? 'green' : 'gray'}>{conn().isActive ? '启用' : '停用'}</Badge>
            <Badge tone="blue">{conn().authType}</Badge>
            <span class="text-xs text-faint font-mono">{conn().id.slice(0, 12)}…</span>
            <div class="ml-auto flex gap-2">
              <Button size="sm" variant="secondary" loading={testing()} onClick={runTest}>测试连接</Button>
              <Toggle checked={conn().isActive} onChange={() => { store.toggleProvider(conn()); load() }} />
            </div>
          </div>
          <Show when={testResult()}>
            <div class={`mt-2 text-xs px-3 py-1.5 rounded-control inline-block ${testResult()!.ok
              ? 'bg-success/10 text-success'
              : 'bg-danger/10 text-danger'}`}>
              {testResult()!.msg}
            </div>
          </Show>
        </Show>
      </div>

      <Show when={loading()}>
        <Card class="p-6 space-y-3"><Skeleton class="h-6 w-48" /><Skeleton class="h-4 w-full" /><Skeleton class="h-4 w-2/3" /></Card>
      </Show>

      <Show when={notFound()}>
        <Card class="p-6"><Empty message="连接不存在或已被删除。" /></Card>
      </Show>

      <Show when={!loading() && conn()}>
        {/* Tab */}
        <div class="flex gap-1 border-b border-subtle">
          <For each={[
            { id: 'overview' as const, label: '配置' },
            { id: 'models' as const, label: '模型' },
            { id: 'oauth' as const, label: '授权' },
          ]}>
            {t => (
              <button
                class={`px-3.5 py-2 text-sm border-b-2 transition-colors ${tab() === t.id
                  ? 'border-[color:var(--accent)] text-text'
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
            <Show when={conn().authType === 'api-key'}>
              <Field label="API Key" hint="留空表示不修改；密钥写入后不在界面回显">
                <Input type="password" value={apiKey()} onInput={setApiKey} placeholder="sk-…" />
              </Field>
            </Show>
            <Field label="Base URL" hint="自定义端点，留空使用默认">
              <Input value={baseUrl()} onInput={setBaseUrl} placeholder="https://api.example.com/v1" />
            </Field>

            <Show when={conn().data?.credentialHint}>
              <div class="text-xs text-faint">
                当前凭证：<span class="font-mono">{conn().data.credentialHint}</span>
                <Show when={conn().data?.hasAccessToken}> · Access Token 已配置</Show>
                <Show when={conn().data?.hasRefreshToken}> · Refresh Token 已配置</Show>
              </div>
            </Show>

            <div class="flex justify-between pt-1">
              <Button variant="danger" onClick={async () => { await store.deleteProvider(conn()); history.back() }}>
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
              <h3 class="text-sm font-semibold mb-3">自定义模型</h3>
              <div class="flex gap-2">
                <Input placeholder="模型 ID，例如 my-finetune-v1" />
                <Button variant="secondary" onClick={() => {
                  const el = document.querySelector<HTMLInputElement>('input[placeholder^="模型 ID"]')
                  if (el?.value) { addCustomModel(el.value, ''); el.value = '' }
                }}>添加</Button>
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

            <Card class="p-5">
              <div class="flex items-center justify-between mb-3">
                <h3 class="text-sm font-semibold">可用模型</h3>
                <span class="text-xs text-faint">{models()?.registryModels?.length ?? 0} 个</span>
              </div>
              <Show when={(models()?.registryModels?.length ?? 0) > 0} fallback={<Empty message="该提供商未上报模型列表。" />}>
                <div class="grid sm:grid-cols-2 lg:grid-cols-3 gap-2">
                  <For each={models()?.registryModels ?? []}>
                    {m => (
                      <div class="px-3 py-2 rounded-control border border-subtle bg-bg-elevated">
                        <div class="text-sm truncate">{m.name || m.id}</div>
                        <div class="text-[11px] text-faint font-mono truncate">{m.id}</div>
                      </div>
                    )}
                  </For>
                </div>
              </Show>
            </Card>
          </div>
        </Show>

        {/* 授权 */}
        <Show when={tab() === 'oauth'}>
          <Card class="p-5 space-y-3">
            <Show when={oauthStatus()} fallback={<Empty message="该提供商无需 OAuth 授权。" />}>
              <div class="flex items-center gap-2 flex-wrap">
                <span class="text-sm">授权方式：</span>
                <Badge tone="blue">{oauthStatus().flowType || 'none'}</Badge>
                <Button size="sm" variant="secondary" onClick={() => refetchOAuth()}>刷新状态</Button>
              </div>
              <Show when={(oauthStatus().connections ?? []).length > 0}>
                <div class="text-xs text-faint">已授权 {oauthStatus().connections.length} 个账号</div>
              </Show>
              <Show when={oauthStatus().flowType === 'import_token'}>
                <div class="pt-2 border-t border-subtle">
                  <div class="text-xs text-faint mb-2">导入 Token（适用于 device-code / 手动获取的场景）</div>
                  <TokenImport provider={conn().provider} onDone={() => { refetchOAuth(); load() }} />
                </div>
              </Show>
            </Show>
          </Card>
        </Show>
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
    } catch (e: any) {
      setMsg(e?.message || '导入失败')
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
