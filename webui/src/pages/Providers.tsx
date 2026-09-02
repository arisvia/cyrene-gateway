import { type Component, For, Show, createSignal, createMemo } from 'solid-js'
import { A } from '@solidjs/router'
import { useGatewayStore } from '@/stores/gateway'
import { Card, Badge, Button, Input, Select, Toggle, Modal, Field, Empty, Skeleton } from '@/components/ui'
import { useToast } from '@/lib/toast'
import type { Provider, RegistryProvider } from '@/types/domain'

// registry category 值域（apikey/oauth/free/freeTier/webCookie）→ 连接 authType 值域（api-key/oauth/cookie/none）
const CATEGORY_TO_AUTHTYPE: Record<string, string> = {
  apikey: 'api-key',
  oauth: 'oauth',
  free: 'none',
  freeTier: 'none',
  webCookie: 'cookie',
}

const AUTHTYPE_LABEL: Record<string, string> = {
  'api-key': 'API Key',
  oauth: 'OAuth',
  none: '免认证',
  cookie: 'Cookie',
}

const CATEGORY_LABEL: Record<string, string> = {
  apikey: 'API Key', oauth: 'OAuth', freeTier: '免费额度', free: '免费', webCookie: 'Cookie',
}


const Providers: Component = () => {
  const store = useGatewayStore()
  const toast = useToast()
  const [query, setQuery] = createSignal('')
  const [catFilter, setCatFilter] = createSignal('')
  const [addOpen, setAddOpen] = createSignal(false)
  const [saving, setSaving] = createSignal(false)
  const [testing, setTesting] = createSignal<string | null>(null)
  const [testResult, setTestResult] = createSignal<{ id: string; ok: boolean; msg: string } | null>(null)

  // 新增表单
  const [form, setForm] = createSignal({ provider: '', name: '', apiKey: '', baseUrl: '' })

  const filtered = createMemo(() => {
    const q = query().toLowerCase().trim()
    return store.providers().filter(p => {
      // 连接的 authType 值域是 api-key/oauth/none/cookie（与 registry category 值域不同）
      if (catFilter() && p.authType !== catFilter()) return false
      if (!q) return true
      return (p.name || '').toLowerCase().includes(q)
        || p.provider.toLowerCase().includes(q)
        || (p.email || '').toLowerCase().includes(q)
    })
  })

  const registryFor = (id: string): RegistryProvider | undefined =>
    store.registryList().find(r => r.id === id)

  async function handleAdd() {
    const f = form()
    if (!f.provider) return
    const reg = registryFor(f.provider)
    if (!reg) return
    // authType 以 registry 为准（category→authType 映射），而不是依赖后端默认 api-key
    const authType = CATEGORY_TO_AUTHTYPE[reg.category] || reg.authType || 'api-key'
    if (authType === 'api-key' && !f.apiKey.trim()) {
      toast.error('API Key 类型需要填写密钥')
      return
    }
    // 重复添加防护：同一 provider 已存在连接时提示，不重复建
    if (store.providers().some(p => p.provider === f.provider)) {
      toast.error(`已存在 ${reg.name} 的连接，请在列表中直接使用或先删除`)
      return
    }
    setSaving(true)
    try {
      await store.addProvider({
        provider: f.provider,
        authType,
        name: f.name || undefined,
        data: { apiKey: f.apiKey.trim() || undefined, baseUrl: f.baseUrl.trim() || undefined },
      })
      setAddOpen(false)
      setForm({ provider: '', name: '', apiKey: '', baseUrl: '' })
      if (authType === 'oauth') {
        toast.info(`已创建 ${reg.name} 连接 — 请在列表中点击进入详情页完成 OAuth 授权`)
      }
    } catch (e: unknown) {
      // toast 由 store 统一处理，这里仅避免未捕获
      console.error('[providers] add failed:', e)
    } finally {
      setSaving(false)
    }
  }

  async function handleTest(p: Provider) {
    setTesting(p.id)
    setTestResult(null)
    try {
      const r = await store.testProvider(p.id)
      setTestResult({
        id: p.id,
        ok: !!r?.ok,
        msg: r?.ok ? `连通 · ${r.latencyMs ?? '?'}ms` : (r?.error || '失败'),
      })
    } catch (e: unknown) {
      setTestResult({ id: p.id, ok: false, msg: e instanceof Error ? e.message : String(e) || '请求失败' })
      setTesting(null)
    }
  }

  return (
    <div class="space-y-5">
      {/* 头部 */}
      <div class="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 class="text-xl font-semibold">提供商连接</h1>
          <p class="text-sm text-faint mt-0.5">
            共 {store.providers().length} 个连接 · {store.activeConnections()} 个启用
          </p>
        </div>
        <div class="flex items-center gap-2">
          <Button variant="secondary" onClick={() => store.loadProvidersOnly()}>刷新</Button>
          <Button variant="primary" onClick={() => setAddOpen(true)}>+ 添加连接</Button>
        </div>
      </div>

      {/* 工具栏 */}
      <Card class="p-3 flex flex-wrap items-center gap-2">
        <Input
          class="!w-56" placeholder="搜索名称 / 邮箱…" value={query()}
          onInput={setQuery}
        />
        <Select
          value={catFilter()}
          options={[
            { value: '', label: '全部类型' },
            { value: 'api-key', label: 'API Key' },
            { value: 'oauth', label: 'OAuth' },
            { value: 'none', label: '免认证' },
            { value: 'cookie', label: 'Cookie' },
          ]}
          onChange={setCatFilter}
        />
        <div class="ml-auto text-xs text-faint">
          筛选出 {filtered().length} 个
        </div>
      </Card>

      {/* 列表 */}
      <Show when={store.providers().length > 0} fallback={
        <Card class="p-6">
          <Empty message="还没有任何连接。点击「+ 添加连接」开始，或用「一键启用免费提供商」快速起步。" />
          <div class="flex justify-center pb-4">
            <Button
              variant="primary"
              loading={saving()}
              onClick={async () => { setSaving(true); try { await store.enableFree() } finally { setSaving(false) } }}
            >
              一键启用免费提供商
            </Button>
          </div>
        </Card>
      }>
        <div class="grid gap-3">
          <For each={filtered()}>
            {p => {
              const reg = () => registryFor(p.provider)
              const cooling = () => !!p.data?.rateLimitedUntil
              return (
                <Card class="p-4 hover:border-accent/50 transition-colors">
                  <div class="flex items-start gap-4">
                    {/* 品牌色标 */}
                    <div
                      class="w-10 h-10 shrink-0 rounded-xl flex items-center justify-center text-xs font-bold text-white"
                      style={{ background: reg()?.color || 'var(--accent)' }}
                    >
                      {(p.name || p.provider).slice(0, 2).toUpperCase()}
                    </div>

                    {/* 主信息 */}
                    <div class="min-w-0 flex-1">
                      <div class="flex items-center gap-2 flex-wrap">
                        <A
                          href={`/providers/${p.id}`}
                          class="font-medium text-sm hover:text-accent transition-colors"
                        >
                          {p.name || p.provider}
                        </A>
                        <Badge tone={p.isActive ? 'green' : 'gray'}>
                          {p.isActive ? '启用' : '停用'}
                        </Badge>
                        <Badge tone="blue">{AUTHTYPE_LABEL[p.authType] || p.authType}</Badge>
                        <Show when={cooling()}>
                          <Badge tone="amber">冷却中</Badge>
                        </Show>
                        <Show when={p.data?.hasApiKey}>
                          <Badge tone="green">已配置凭证</Badge>
                        </Show>
                        <Show when={!p.data?.hasApiKey && p.authType === 'api-key'}>
                          <Badge tone="red">缺凭证</Badge>
                        </Show>
                      </div>
                      <div class="mt-1 text-xs text-faint truncate">
                        <span class="font-mono">{p.provider}</span>
                        <Show when={p.email}> · {p.email}</Show>
                        <Show when={p.data?.credentialHint}>
                          {' '}· 凭证 <span class="font-mono">{String(p.data?.credentialHint ?? '')}</span>
                        </Show>
                        {' '}· 优先级 {p.priority}
                      </div>
                      <Show when={testResult()?.id === p.id}>
                        <div class={`mt-1.5 text-xs ${testResult()!.ok ? 'text-success' : 'text-danger'}`}>
                          {testResult()!.msg}
                        </div>
                      </Show>
                    </div>

                    {/* 操作 */}
                    <div class="flex items-center gap-1.5 shrink-0">
                      <Button size="sm" variant="ghost" loading={testing() === p.id} onClick={() => handleTest(p)}>
                        {testing() === p.id ? '' : '测试'}
                      </Button>
                      <Show when={cooling()}>
                        <Button size="sm" variant="ghost" onClick={() => store.resetCooldown(p)}>解除冷却</Button>
                      </Show>
                      <Toggle checked={p.isActive} onChange={() => store.toggleProvider(p)} />
                      <Button size="sm" variant="danger" onClick={() => store.deleteProvider(p)}>删除</Button>
                    </div>
                  </div>
                </Card>
              )
            }}
          </For>
        </div>
      </Show>

      {/* 添加对话框 */}
      <Modal open={addOpen()} title="添加提供商连接" onClose={() => setAddOpen(false)}>
        <div class="space-y-4">
          <Field label="提供商" hint="选择要接入的服务；带 OAuth 的需在详情页完成授权">
            <Select
              value={form().provider}
              options={[
                { value: '', label: '请选择…' },
                ...store.registryList().map(r => ({ value: r.id, label: `${r.name}（${CATEGORY_LABEL[r.category] || r.category}）` })),
              ]}
              onChange={v => setForm(f => ({ ...f, provider: v }))}
            />
          </Field>
          <Field label="显示名称" hint="留空则使用提供商名">
            <Input value={form().name} placeholder="例如：我的主力 Anthropic" onInput={v => setForm(f => ({ ...f, name: v }))} />
          </Field>
          <Show when={registryFor(form().provider)?.authType === 'api-key'}>
            <Field label="API Key" hint="密钥只写入后端，不会在界面回显">
              <Input type="password" value={form().apiKey} placeholder="sk-…" onInput={v => setForm(f => ({ ...f, apiKey: v }))} />
            </Field>
            <Field label="Base URL" hint="留空使用官方默认地址">
              <Input value={form().baseUrl} placeholder="https://api.example.com/v1" onInput={v => setForm(f => ({ ...f, baseUrl: v }))} />
            </Field>
          </Show>
          <div class="flex justify-end gap-2 pt-1">
            <Button variant="ghost" onClick={() => setAddOpen(false)}>取消</Button>
            <Button variant="primary" loading={saving()} disabled={!form().provider} onClick={handleAdd}>
              添加
            </Button>
          </div>
        </div>
      </Modal>
    </div>
  )
}

export default Providers
