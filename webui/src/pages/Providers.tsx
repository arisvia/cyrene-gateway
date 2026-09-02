import { type Component, For, Show, createSignal, createMemo } from 'solid-js'
import { A } from '@solidjs/router'
import { useGatewayStore } from '@/stores/gateway'
import { Card, Badge, Button, Input, Select, Toggle, Modal, Field, Empty } from '@/components/ui'
import { useToast } from '@/lib/toast'
import type { Provider, RegistryProvider, BadgeTone } from '@/types/domain'

const CATEGORY_LABEL: Record<string, string> = {
  apikey: 'API Key',
  oauth: 'OAuth 授权',
  freeTier: '免费额度',
  free: '免密免费',
  webCookie: '网页 Cookie',
}

const AUTHTYPE_LABEL: Record<string, string> = {
  'api-key': 'API Key',
  oauth: 'OAuth',
  none: '免认证',
  cookie: 'Cookie',
}

const Providers: Component = () => {
  const store = useGatewayStore()
  const toast = useToast()

  // 顶层视图切换：'connections' | 'catalog'
  const [activeTab, setActiveTab] = createSignal<'connections' | 'catalog'>('connections')

  // 搜索与分类过滤
  const [query, setQuery] = createSignal('')
  const [catFilter, setCatFilter] = createSignal('')

  // 添加/配置向导状态
  const [wizardOpen, setWizardOpen] = createSignal(false)
  const [selectedReg, setSelectedReg] = createSignal<RegistryProvider | null>(null)
  const [form, setForm] = createSignal({
    name: '',
    authType: 'api-key',
    apiKey: '',
    baseUrl: '',
  })
  const [saving, setSaving] = createSignal(false)
  const [testing, setTesting] = createSignal<string | null>(null)
  const [testResult, setTestResult] = createSignal<{ id: string; ok: boolean; msg: string } | null>(null)

  // 快捷获取 Registry 项
  const registryFor = (id: string): RegistryProvider | undefined =>
    store.registryList().find(r => r.id === id)

  // 过滤后的已接入连接
  const filteredConnections = createMemo(() => {
    const q = query().toLowerCase().trim()
    return store.providers().filter(p => {
      if (catFilter() && p.authType !== catFilter()) return false
      if (!q) return true
      return (
        (p.name || '').toLowerCase().includes(q) ||
        p.provider.toLowerCase().includes(q) ||
        (p.email || '').toLowerCase().includes(q)
      )
    })
  })

  // 过滤后的市场提供商
  const filteredCatalog = createMemo(() => {
    const q = query().toLowerCase().trim()
    return store.registryList().filter(r => {
      if (catFilter() && r.category !== catFilter()) return false
      if (!q) return true
      return (
        r.name.toLowerCase().includes(q) ||
        r.id.toLowerCase().includes(q) ||
        (r.category || '').toLowerCase().includes(q)
      )
    })
  })

  // 打开添加向导
  function openWizard(reg: RegistryProvider) {
    setSelectedReg(reg)
    const defaultAuth = reg.noAuth || reg.category === 'free' ? 'none' : (reg.authType || 'api-key')
    setForm({
      name: reg.name,
      authType: defaultAuth,
      apiKey: '',
      baseUrl: reg.baseUrl || '',
    })
    setWizardOpen(true)
  }

  // 一键接入免密提供商
  async function quickEnableFree(reg: RegistryProvider) {
    if (store.providers().some(p => p.provider === reg.id)) {
      toast.info(`${reg.name} 已经接入`)
      return
    }
    setSaving(true)
    try {
      await store.addProvider({
        provider: reg.id,
        authType: 'none',
        name: reg.name,
        data: { baseUrl: reg.baseUrl || undefined },
      })
      toast.success(`已启用免费提供商：${reg.name}`)
    } catch (e: unknown) {
      console.error('[providers] quick enable failed:', e)
    } finally {
      setSaving(false)
    }
  }

  // 提交向导表单
  async function handleWizardSubmit() {
    const reg = selectedReg()
    if (!reg) return

    const f = form()
    if (f.authType === 'api-key' && !f.apiKey.trim()) {
      toast.error('请填写 API Key')
      return
    }

    if (store.providers().some(p => p.provider === reg.id && p.authType === f.authType)) {
      toast.error(`已存在 ${reg.name}（${AUTHTYPE_LABEL[f.authType]}）连接`)
      return
    }

    setSaving(true)
    try {
      await store.addProvider({
        provider: reg.id,
        authType: f.authType,
        name: f.name.trim() || reg.name,
        data: {
          apiKey: f.apiKey.trim() || undefined,
          baseUrl: f.baseUrl.trim() || undefined,
        },
      })
      setWizardOpen(false)
      if (f.authType === 'oauth') {
        toast.info(`已创建 ${reg.name} 连接 — 请点击卡片进入详情页完成 OAuth 登录授权`)
      }
      setActiveTab('connections')
    } catch (e: unknown) {
      console.error('[providers] add provider failed:', e)
    } finally {
      setSaving(false)
    }
  }

  // 测试连接
  async function handleTest(p: Provider) {
    setTesting(p.id)
    setTestResult(null)
    try {
      const r = await store.testProvider(p.id)
      setTestResult({
        id: p.id,
        ok: !!r?.ok,
        msg: r?.ok ? `连通正常 · 延迟 ${r.latencyMs ?? '?'}ms` : (r?.error || '连接失败'),
      })
    } catch (e: unknown) {
      setTestResult({
        id: p.id,
        ok: false,
        msg: e instanceof Error ? e.message : String(e) || '请求异常',
      })
    } finally {
      setTesting(null)
    }
  }

  return (
    <div class="space-y-5">
      {/* 头部标题与视窗切换 */}
      <div class="flex flex-wrap items-center justify-between gap-4">
        <div>
          <h1 class="text-xl font-semibold">模型提供商接入</h1>
          <p class="text-sm text-faint mt-0.5">
            统一管理各上游 API 凭证、OAuth 会话与免认证提供商
          </p>
        </div>

        {/* Tab 切换胶囊 */}
        <div class="inline-flex p-1 rounded-xl bg-hover border border-subtle">
          <button
            type="button"
            class={`px-3 py-1.5 text-xs font-medium rounded-lg transition-all ${
              activeTab() === 'connections'
                ? 'bg-bg text-foreground shadow-sm font-semibold'
                : 'text-faint hover:text-foreground'
            }`}
            onClick={() => { setActiveTab('connections'); setCatFilter(''); }}
          >
            我的连接 ({store.providers().length})
          </button>
          <button
            type="button"
            class={`px-3 py-1.5 text-xs font-medium rounded-lg transition-all ${
              activeTab() === 'catalog'
                ? 'bg-bg text-foreground shadow-sm font-semibold'
                : 'text-faint hover:text-foreground'
            }`}
            onClick={() => { setActiveTab('catalog'); setCatFilter(''); }}
          >
            提供商市场 ({store.registryList().length})
          </button>
        </div>
      </div>

      {/* 搜索与过滤工具栏 */}
      <Card class="p-3 flex flex-wrap items-center gap-3">
        <Input
          class="!w-64"
          placeholder={activeTab() === 'connections' ? '搜索已连接提供商…' : '搜索提供商市场…'}
          value={query()}
          onInput={setQuery}
        />

        <Show when={activeTab() === 'connections'}>
          <Select
            value={catFilter()}
            options={[
              { value: '', label: '全部认证类型' },
              { value: 'api-key', label: 'API Key' },
              { value: 'oauth', label: 'OAuth 授权' },
              { value: 'none', label: '免密免费' },
              { value: 'cookie', label: 'Cookie' },
            ]}
            onChange={setCatFilter}
          />
        </Show>

        <Show when={activeTab() === 'catalog'}>
          <Select
            value={catFilter()}
            options={[
              { value: '', label: '全部分类' },
              { value: 'free', label: '免密免费' },
              { value: 'freeTier', label: '免费额度' },
              { value: 'apikey', label: 'API Key' },
              { value: 'oauth', label: 'OAuth 渠道' },
            ]}
            onChange={setCatFilter}
          />
        </Show>

        <div class="ml-auto flex items-center gap-2 text-xs text-faint">
          <span>匹配 {activeTab() === 'connections' ? filteredConnections().length : filteredCatalog().length} 项</span>
          <Show when={activeTab() === 'connections'}>
            <Button size="sm" variant="secondary" onClick={() => store.loadProvidersOnly()}>刷新</Button>
            <Button size="sm" variant="primary" onClick={() => setActiveTab('catalog')}>+ 添加新接入</Button>
          </Show>
        </div>
      </Card>

      {/* 视窗 1：我的连接列表 */}
      <Show when={activeTab() === 'connections'}>
        <Show
          when={store.providers().length > 0}
          fallback={
            <Card class="p-8 text-center space-y-4">
              <Empty message="还没有接入任何提供商连接" />
              <p class="text-xs text-faint max-w-md mx-auto">
                你可以前往「提供商市场」挑选主流商用大模型、一键接入免费免密渠道，或添加自定义 OpenAI 兼容接口。
              </p>
              <div class="flex justify-center gap-3 pt-2">
                <Button variant="primary" onClick={() => setActiveTab('catalog')}>
                  去市场选购提供商
                </Button>
                <Button
                  variant="secondary"
                  loading={saving()}
                  onClick={async () => {
                    setSaving(true)
                    try { await store.enableFree() } finally { setSaving(false) }
                  }}
                >
                  一键启用全部免费渠道
                </Button>
              </div>
            </Card>
          }
        >
          <div class="grid gap-3">
            <For each={filteredConnections()}>
              {p => {
                const reg = () => registryFor(p.provider)
                const cooling = () => !!p.data?.rateLimitedUntil
                return (
                  <Card class="p-4 hover:border-accent/40 transition-colors">
                    <div class="flex items-start gap-4">
                      {/* 品牌色彩块 */}
                      <div
                        class="w-11 h-11 shrink-0 rounded-xl flex items-center justify-center text-xs font-bold text-white shadow-sm"
                        style={{ background: reg()?.color || 'var(--accent)' }}
                      >
                        {(p.name || p.provider).slice(0, 2).toUpperCase()}
                      </div>

                      {/* 连接主要信息 */}
                      <div class="min-w-0 flex-1">
                        <div class="flex items-center gap-2 flex-wrap">
                          <A
                            href={`/providers/${p.id}`}
                            class="font-medium text-sm hover:text-accent transition-colors"
                          >
                            {p.name || p.provider}
                          </A>
                          <Badge tone={p.isActive ? 'green' : 'gray'}>
                            {p.isActive ? '已启用' : '已停用'}
                          </Badge>
                          <Badge tone="blue">{AUTHTYPE_LABEL[p.authType] || p.authType}</Badge>
                          <Show when={cooling()}>
                            <Badge tone="amber">限流冷却中</Badge>
                          </Show>
                          <Show when={p.data?.hasApiKey}>
                            <Badge tone="green">已配置凭证</Badge>
                          </Show>
                          <Show when={!p.data?.hasApiKey && p.authType === 'api-key'}>
                            <Badge tone="red">缺凭证</Badge>
                          </Show>
                        </div>

                        <div class="mt-1.5 text-xs text-faint flex flex-wrap items-center gap-x-2 gap-y-1">
                          <span class="font-mono">{p.provider}</span>
                          <Show when={p.email}><span>· {p.email}</span></Show>
                          <Show when={p.data?.credentialHint}>
                            <span>· 凭证 <span class="font-mono">{String(p.data?.credentialHint ?? '')}</span></span>
                          </Show>
                          <span>· 优先级 {p.priority}</span>
                          <Show when={p.data?.baseUrl}>
                            <span class="truncate max-w-[240px]">· URL: {String(p.data?.baseUrl ?? '')}</span>
                          </Show>
                        </div>

                        <Show when={testResult()?.id === p.id}>
                          <div class={`mt-2 text-xs font-mono ${testResult()!.ok ? 'text-success' : 'text-danger'}`}>
                            {testResult()!.msg}
                          </div>
                        </Show>
                      </div>

                      {/* 操作按钮区 */}
                      <div class="flex items-center gap-2 shrink-0">
                        <Button
                          size="sm"
                          variant="ghost"
                          loading={testing() === p.id}
                          onClick={() => handleTest(p)}
                        >
                          测试
                        </Button>
                        <Show when={cooling()}>
                          <Button size="sm" variant="ghost" onClick={() => store.resetCooldown(p)}>
                            解除冷却
                          </Button>
                        </Show>
                        <A href={`/providers/${p.id}`}>
                          <Button size="sm" variant="secondary">配置</Button>
                        </A>
                        <Toggle checked={p.isActive} onChange={() => store.toggleProvider(p)} />
                        <Button size="sm" variant="danger" onClick={() => store.deleteProvider(p)}>
                          删除
                        </Button>
                      </div>
                    </div>
                  </Card>
                )
              }}
            </For>
          </div>
        </Show>
      </Show>

      {/* 视窗 2：提供商市场 (Catalog Grid) */}
      <Show when={activeTab() === 'catalog'}>
        <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          <For each={filteredCatalog()}>
            {reg => {
              const connected = () => store.providers().some(p => p.provider === reg.id)
              const isFree = reg.noAuth || reg.category === 'free'

              return (
                <Card class="p-4 flex flex-col justify-between hover:border-accent/40 transition-all space-y-3">
                  <div>
                    <div class="flex items-start justify-between gap-2">
                      <div class="flex items-center gap-2.5">
                        <div
                          class="w-9 h-9 shrink-0 rounded-xl flex items-center justify-center text-xs font-bold text-white shadow-sm"
                          style={{ background: reg.color || 'var(--accent)' }}
                        >
                          {reg.name.slice(0, 2).toUpperCase()}
                        </div>
                        <div>
                          <div class="font-medium text-sm text-foreground flex items-center gap-1.5">
                            {reg.name}
                            <Show when={reg.region}>
                              <span class="text-[10px] px-1 py-0.5 rounded bg-hover text-faint uppercase font-mono">
                                {reg.region}
                              </span>
                            </Show>
                          </div>
                          <div class="text-xs text-faint font-mono">{reg.id}</div>
                        </div>
                      </div>

                      <Badge tone={isFree ? 'green' : 'blue' as BadgeTone}>
                        {CATEGORY_LABEL[reg.category] || reg.category}
                      </Badge>
                    </div>

                    <div class="mt-3 text-xs text-faint space-y-1">
                      <div class="flex items-center justify-between">
                        <span>协议类型: <code class="font-mono text-foreground">{reg.apiType}</code></span>
                        <span>默认优先级: {reg.priority ?? 50}</span>
                      </div>
                      <Show when={reg.authHint}>
                        <div class="text-[11px] text-muted italic line-clamp-1">{reg.authHint}</div>
                      </Show>
                    </div>
                  </div>

                  <div class="pt-3 border-t border-subtle flex items-center justify-between gap-2">
                    <Show
                      when={reg.apiKeyUrl || reg.website}
                      fallback={<span class="text-[11px] text-faint">内置支持</span>}
                    >
                      <a
                        href={reg.apiKeyUrl || reg.website}
                        target="_blank"
                        rel="noreferrer"
                        class="text-xs text-accent hover:underline inline-flex items-center gap-1"
                      >
                        {reg.apiKeyUrl ? '获取密钥 ↗' : '官方网站 ↗'}
                      </a>
                    </Show>

                    <div class="flex items-center gap-2">
                      <Show when={connected()}>
                        <span class="text-xs text-success font-medium">已接入</span>
                      </Show>
                      <Show when={isFree && !connected()}>
                        <Button
                          size="sm"
                          variant="secondary"
                          loading={saving()}
                          onClick={() => quickEnableFree(reg)}
                        >
                          一键启用
                        </Button>
                      </Show>
                      <Button
                        size="sm"
                        variant={connected() ? 'ghost' : 'primary'}
                        onClick={() => openWizard(reg)}
                      >
                        {connected() ? '再加一个' : '接入配置'}
                      </Button>
                    </div>
                  </div>
                </Card>
              )
            }}
          </For>
        </div>
      </Show>

      {/* 接入配置向导 Modal */}
      <Modal
        open={wizardOpen()}
        title={selectedReg() ? `接入提供商：${selectedReg()!.name}` : '添加提供商'}
        onClose={() => setWizardOpen(false)}
      >
        <Show when={selectedReg()}>
          {reg => {
            const authModes = reg().authModes || [reg().authType || 'api-key']
            const isFree = reg().noAuth || reg().category === 'free'

            return (
              <div class="space-y-4">
                <div class="p-3 rounded-control bg-hover text-xs space-y-1 text-faint">
                  <div class="flex justify-between">
                    <span>接口协议：<span class="font-mono text-foreground">{reg().apiType}</span></span>
                    <span>类别：<span class="text-foreground">{CATEGORY_LABEL[reg().category] || reg().category}</span></span>
                  </div>
                  <Show when={reg().apiKeyUrl}>
                    <div>
                      获取密钥链接：
                      <a href={reg().apiKeyUrl} target="_blank" rel="noreferrer" class="text-accent underline font-mono">
                        {reg().apiKeyUrl}
                      </a>
                    </div>
                  </Show>
                  <Show when={reg().authHint}>
                    <div class="text-muted">提示：{reg().authHint}</div>
                  </Show>
                </div>

                <Field label="连接显示名称" hint="自定义连接名称，便于多账号识别">
                  <Input
                    value={form().name}
                    placeholder={`例如：我的 ${reg().name}`}
                    onInput={v => setForm(f => ({ ...f, name: v }))}
                  />
                </Field>

                {/* 如果支持多种认证模式 */}
                <Show when={authModes.length > 1}>
                  <Field label="认证方式" hint="该上游支持多种鉴权模式">
                    <Select
                      value={form().authType}
                      options={authModes.map(m => ({ value: m, label: AUTHTYPE_LABEL[m] || m }))}
                      onChange={v => setForm(f => ({ ...f, authType: v }))}
                    />
                  </Field>
                </Show>

                {/* API Key 模式字段 */}
                <Show when={form().authType === 'api-key'}>
                  <Field label="API Key / 凭据" hint="凭证安全存储于服务端并进行掩码处理">
                    <Input
                      type="password"
                      value={form().apiKey}
                      placeholder="sk-…"
                      onInput={v => setForm(f => ({ ...f, apiKey: v }))}
                    />
                  </Field>

                  <Field label="Base URL (可选)" hint={`默认: ${reg().baseUrl || '官方 API 端点'}`}>
                    <Input
                      value={form().baseUrl}
                      placeholder={reg().baseUrl || 'https://api.example.com/v1'}
                      onInput={v => setForm(f => ({ ...f, baseUrl: v }))}
                    />
                  </Field>
                </Show>

                {/* OAuth 模式提示 */}
                <Show when={form().authType === 'oauth'}>
                  <div class="p-3 rounded-control border border-accent/20 bg-accent/5 text-xs text-muted leading-relaxed">
                    创建连接后，可在连接列表中点击该项进入详情页，通过设备码或浏览器授权完成 OAuth Token 获取。
                  </div>
                </Show>

                <Show when={isFree}>
                  <div class="p-3 rounded-control bg-success/10 text-xs text-success">
                    该提供商为公共免密服务，无需填写密钥即可直接接入。
                  </div>
                </Show>

                <div class="flex justify-end gap-2 pt-2">
                  <Button variant="ghost" onClick={() => setWizardOpen(false)}>
                    取消
                  </Button>
                  <Button
                    variant="primary"
                    loading={saving()}
                    disabled={form().authType === 'api-key' && !form().apiKey.trim()}
                    onClick={handleWizardSubmit}
                  >
                    确认接入
                  </Button>
                </div>
              </div>
            )
          }}
        </Show>
      </Modal>
    </div>
  )
}

export default Providers
