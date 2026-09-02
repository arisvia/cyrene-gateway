import { type Component, For, Show, createSignal, createMemo, onMount } from 'solid-js'
import { A } from '@solidjs/router'
import { useGatewayStore } from '@/stores/gateway'
import { Card, Badge, Button, Input, Select, Toggle, Modal, Field, Empty, ProviderAvatar } from '@/components/ui'
import { useToast } from '@/lib/toast'
import type { Provider, RegistryProvider, BadgeTone } from '@/types/domain'

const CATEGORY_LABEL: Record<string, string> = {
  all: '全部类别',
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
  const [refreshing, setRefreshing] = createSignal(false)
  const [testing, setTesting] = createSignal<string | null>(null)
  const [testResult, setTestResult] = createSignal<{ id: string; ok: boolean; msg: string } | null>(null)

  // 挂载时刷新一次，确保 Registry 完整
  onMount(() => {
    if (store.registryList().length === 0) {
      store.loadCore()
    }
  })

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

  // 按品牌归集或单例展示的市场提供商列表
  interface CatalogBrandGroup {
    brandKey: string
    name: string
    items: RegistryProvider[]
  }

  const brandGroups = createMemo<CatalogBrandGroup[]>(() => {
    const list = store.registryList()
    const q = query().toLowerCase().trim()
    const cat = catFilter()

    // 先按过滤条件筛选
    const matched = list.filter(r => {
      if (cat && r.category !== cat) return false
      if (!q) return true
      return (
        r.name.toLowerCase().includes(q) ||
        r.id.toLowerCase().includes(q) ||
        (r.brand || '').toLowerCase().includes(q) ||
        (r.category || '').toLowerCase().includes(q)
      )
    })

    // 按 Brand 或 单例 ID 分组
    const groups: Record<string, RegistryProvider[]> = {}
    for (const r of matched) {
      const key = r.brand || r.id
      if (!groups[key]) groups[key] = []
      groups[key].push(r)
    }

    return Object.entries(groups).map(([key, items]) => {
      // 首选主名称，如果有带品牌名则用品牌名
      const first = items[0]
      const displayName = first.brand ? first.brand : first.name
      return {
        brandKey: key,
        name: displayName,
        items,
      }
    })
  })

  // 选中的区域版本 (brandKey -> provider.id)
  const [selectedVariants, setSelectedVariants] = createSignal<Record<string, string>>({})

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
      await store.loadProvidersOnly()
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : String(e)
      toast.error(`启用失败: ${msg}`)
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

    setSaving(true)
    try {
      await store.addProvider({
        provider: reg.id,
        authType: f.authType,
        name: f.name || reg.name,
        data: {
          apiKey: f.apiKey.trim() || undefined,
          baseUrl: f.baseUrl.trim() || undefined,
        },
      })
      toast.success(`成功接入提供商：${f.name || reg.name}`)
      setWizardOpen(false)
      setActiveTab('connections')
      await store.loadProvidersOnly()
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : String(e)
      toast.error(`接入失败: ${msg}`)
      console.error('[providers] add provider failed:', e)
    } finally {
      setSaving(false)
    }
  }

  // 测试连接健康度
  async function handleTest(p: Provider) {
    setTesting(p.id)
    setTestResult(null)
    try {
      const res = await store.testProvider(p.id)
      setTestResult({
        id: p.id,
        ok: res.ok,
        msg: res.ok ? `连通正常 (${res.latencyMs ?? 0}ms)` : (res.error || '连通失败'),
      })
      if (res.ok) {
        toast.success(`${p.name || p.provider} 测试通过 (${res.latencyMs ?? 0}ms)`)
      } else {
        toast.error(`${p.name || p.provider} 测试失败: ${res.error || '未知错误'}`)
      }
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : '连接异常'
      setTestResult({
        id: p.id,
        ok: false,
        msg,
      })
      toast.error(`${p.name || p.provider} 测试失败: ${msg}`)
    } finally {
      setTesting(null)
    }
  }

  // 刷新所有模型与连接
  async function handleRefreshAll() {
    setRefreshing(true)
    try {
      await store.loadProvidersOnly()
      await store.loadCore()
      toast.success('已刷新提供商列表与实时状态')
    } catch (e: unknown) {
      toast.error('刷新失败')
    } finally {
      setRefreshing(false)
    }
  }

  return (
    <div class="space-y-4">
      {/* 头部标题与视窗切换 (吸顶固定) */}
      <div class="sticky top-0 z-20 bg-bg/95 backdrop-blur-md pt-1 pb-3 space-y-3">
        <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
          <div>
            <h1 class="text-2xl font-bold tracking-tight text-foreground">模型提供商接入</h1>
            <p class="text-sm text-faint mt-1">
              统一管理各大模型商用上游、OAuth 动态凭证与免认证公共代理池
            </p>
          </div>

          {/* 现代分段切换药丸 (Segmented Control) */}
          <div class="inline-flex p-1 rounded-xl bg-card border border-subtle shadow-sm">
            <button
              type="button"
              class={`px-4 py-1.5 text-xs font-semibold rounded-lg transition-all flex items-center gap-1.5 ${
                activeTab() === 'connections'
                  ? 'bg-accent text-on-accent shadow-sm'
                  : 'text-muted hover:text-foreground'
              }`}
              onClick={() => { setActiveTab('connections'); setCatFilter(''); }}
            >
              我的连接 ({store.providers().length})
            </button>
            <button
              type="button"
              class={`px-4 py-1.5 text-xs font-semibold rounded-lg transition-all flex items-center gap-1.5 ${
                activeTab() === 'catalog'
                  ? 'bg-accent text-on-accent shadow-sm'
                  : 'text-muted hover:text-foreground'
              }`}
              onClick={() => { setActiveTab('catalog'); setCatFilter(''); }}
            >
              提供商市场 ({store.registryList().length})
            </button>
          </div>
        </div>

        {/* 搜索与过滤工具栏 */}
        <Card class="p-3.5 flex flex-wrap items-center justify-between gap-3 shadow-sm">
          <div class="flex flex-wrap items-center gap-3 flex-1">
            <Input
              class="!w-64"
              placeholder={activeTab() === 'connections' ? '搜索已连接提供商…' : '搜索提供商市场/模型…'}
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
          </div>

          <div class="flex items-center gap-3 text-xs text-faint">
            <span class="hidden sm:inline">
              匹配 <strong class="text-foreground font-mono">{activeTab() === 'connections' ? filteredConnections().length : brandGroups().length}</strong> 项
            </span>
            <Button size="sm" variant="secondary" loading={refreshing()} onClick={handleRefreshAll}>
              刷新
            </Button>
            <Show when={activeTab() === 'connections'}>
              <Button size="sm" variant="primary" onClick={() => setActiveTab('catalog')}>
                + 接入新提供商
              </Button>
            </Show>
          </div>
        </Card>
      </div>

      {/* 视窗 1：我的连接列表 */}
      <Show when={activeTab() === 'connections'}>
        <Show
          when={store.providers().length > 0}
          fallback={
            <Card class="p-12 text-center space-y-4">
              <Empty message="还没有接入任何提供商连接" />
              <p class="text-xs text-faint max-w-md mx-auto leading-relaxed">
                你可以前往「提供商市场」挑选主流商用大模型（OpenAI, Claude, Gemini, DeepSeek），或一键开启免认证公共上游。
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
                    try {
                      await store.enableFree()
                      toast.success('已一键接入所有免费上游')
                    } finally {
                      setSaving(false)
                    }
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
                const test = () => testResult()?.id === p.id ? testResult() : null

                return (
                  <Card class="p-4 hover:border-accent/40 transition-all group">
                    <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
                      {/* 左侧：图标 + 标题与标签 */}
                      <div class="flex items-start sm:items-center gap-3.5 min-w-0">
                        <ProviderAvatar
                          provider={p.provider}
                          name={p.name}
                          color={reg()?.color}
                          size="lg"
                        />

                        <div class="min-w-0 space-y-1">
                          <div class="flex items-center gap-2 flex-wrap">
                            <A
                              href={`/providers/${p.id}`}
                              class="font-semibold text-sm hover:text-accent transition-colors truncate max-w-xs"
                            >
                              {p.name || p.provider}
                            </A>
                            <Badge tone={p.isActive ? 'green' : 'gray'}>
                              {p.isActive ? '已启用' : '已停用'}
                            </Badge>
                            <Badge tone="blue">
                              {AUTHTYPE_LABEL[p.authType] || p.authType}
                            </Badge>
                            <Show when={p.data?.hasApiKey !== undefined}>
                              <Badge tone={p.data?.hasApiKey ? 'green' : 'amber'}>
                                {p.data?.hasApiKey ? '已配置凭证' : '缺凭证'}
                              </Badge>
                            </Show>
                            <Show when={cooling()}>
                              <Badge tone="amber">限流冷却中</Badge>
                            </Show>
                          </div>

                          <div class="text-xs text-faint flex items-center gap-2 flex-wrap font-mono">
                            <span>{p.provider}</span>
                            <span>·</span>
                            <span>优先级 {p.priority}</span>
                            <Show when={p.email}>
                              <span>·</span>
                              <span>{p.email}</span>
                            </Show>
                            <Show when={test()}>
                              <span>·</span>
                              <span class={test()!.ok ? 'text-success font-medium' : 'text-danger font-medium'}>
                                {test()!.msg}
                              </span>
                            </Show>
                          </div>
                        </div>
                      </div>

                      {/* 右侧：动作控制区 */}
                      <div class="flex items-center gap-2.5 self-end sm:self-center shrink-0">
                        <Button
                          size="sm"
                          variant="secondary"
                          loading={testing() === p.id}
                          onClick={() => handleTest(p)}
                        >
                          测试连通
                        </Button>
                        <A href={`/providers/${p.id}`}>
                          <Button size="sm" variant="secondary">
                            配置
                          </Button>
                        </A>
                        <Toggle
                          checked={p.isActive}
                          onChange={async () => {
                            await store.toggleProvider(p)
                          }}
                        />
                        <Button
                          size="sm"
                          variant="danger"
                          onClick={async () => {
                            if (confirm(`确定要删除连接「${p.name || p.provider}」吗？`)) {
                              await store.deleteProvider(p)
                            }
                          }}
                        >
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
        <div class="max-h-[calc(100vh-210px)] overflow-y-auto pr-1">
          <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4 pb-6">
            <For each={brandGroups()}>
            {group => {
              // 当前选中的变体（默认第一项）
              const activeReg = () => {
                const selectedId = selectedVariants()[group.brandKey]
                if (selectedId) {
                  const found = group.items.find(it => it.id === selectedId)
                  if (found) return found
                }
                return group.items[0]
              }

              const reg = activeReg
              const connected = () => store.providers().some(p => p.provider === reg().id)
              const isFree = () => reg().noAuth || reg().category === 'free'
              const hasVariants = () => group.items.length > 1

              return (
                <Card class="p-4 flex flex-col justify-between hover:border-accent/40 transition-all space-y-4 group">
                  <div>
                    <div class="flex items-start justify-between gap-2">
                      <div class="flex items-center gap-3">
                        <ProviderAvatar
                          provider={reg().id}
                          name={group.name}
                          color={reg().color}
                          size="md"
                        />
                        <div>
                          <div class="font-semibold text-sm text-foreground flex items-center gap-1.5">
                            {group.name}
                          </div>
                          <div class="text-xs text-faint font-mono">{reg().id}</div>
                        </div>
                      </div>

                      <Badge tone={isFree() ? 'green' : 'blue' as BadgeTone}>
                        {CATEGORY_LABEL[reg().category] || reg().category}
                      </Badge>
                    </div>

                    {/* 区域 / 渠道小标签切换器 (如 cn / intl) */}
                    <Show when={hasVariants()}>
                      <div class="mt-2.5 flex items-center gap-1 p-1 bg-hover rounded-lg border border-subtle">
                        <For each={group.items}>
                          {variant => {
                            const isSelected = () => reg().id === variant.id
                            const label = () => {
                              if (variant.region === 'cn') return '国内版 (CN)'
                              if (variant.region === 'intl') return '国际版 (Intl)'
                              return variant.name.replace(group.name, '').trim() || variant.id
                            }
                            return (
                              <button
                                type="button"
                                class={`flex-1 text-[11px] py-1 px-2 rounded-md font-medium transition-all ${
                                  isSelected()
                                    ? 'bg-card text-foreground shadow-xs font-semibold'
                                    : 'text-faint hover:text-foreground'
                                }`}
                                onClick={() => {
                                  setSelectedVariants(prev => ({
                                    ...prev,
                                    [group.brandKey]: variant.id,
                                  }))
                                }}
                              >
                                {label()}
                              </button>
                            )
                          }}
                        </For>
                      </div>
                    </Show>

                    <div class="mt-3 text-xs text-faint space-y-1">
                      <div class="flex items-center justify-between">
                        <span>协议: <code class="font-mono text-foreground">{reg().apiType || 'openai'}</code></span>
                        <span>默认优先级: {reg().priority ?? 50}</span>
                      </div>
                      <Show when={reg().authHint}>
                        <div class="text-[11px] text-muted italic line-clamp-1">{reg().authHint}</div>
                      </Show>
                    </div>
                  </div>

                  <div class="pt-3 border-t border-subtle flex items-center justify-between gap-2">
                    <Show
                      when={reg().apiKeyUrl || reg().website}
                      fallback={<span class="text-[11px] text-faint">原生内置</span>}
                    >
                      <a
                        href={reg().apiKeyUrl || reg().website}
                        target="_blank"
                        rel="noreferrer"
                        class="text-xs text-accent hover:underline inline-flex items-center gap-1"
                      >
                        {reg().apiKeyUrl ? '获取密钥 ↗' : '官网 ↗'}
                      </a>
                    </Show>

                    <div class="flex items-center gap-2">
                      <Show when={connected()}>
                        <span class="text-xs text-success font-semibold px-2 py-0.5 rounded bg-success/10">已接入</span>
                      </Show>
                      <Show when={isFree() && !connected()}>
                        <Button
                          size="sm"
                          variant="secondary"
                          loading={saving()}
                          onClick={() => quickEnableFree(reg())}
                        >
                          一键启用
                        </Button>
                      </Show>
                      <Button
                        size="sm"
                        variant={connected() ? 'secondary' : 'primary'}
                        onClick={() => openWizard(reg())}
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
                <div class="p-3.5 rounded-xl bg-hover text-xs space-y-1.5 text-faint border border-subtle">
                  <div class="flex items-center justify-between">
                    <span>接口协议：<strong class="font-mono text-foreground">{reg().apiType || 'openai'}</strong></span>
                    <span>类别：<strong class="text-foreground">{CATEGORY_LABEL[reg().category] || reg().category}</strong></span>
                  </div>
                  <Show when={reg().apiKeyUrl}>
                    <div>
                      获取密钥链接：
                      <a href={reg().apiKeyUrl} target="_blank" rel="noreferrer" class="text-accent underline font-mono ml-1">
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
                      placeholder="sk-..."
                      onInput={v => setForm(f => ({ ...f, apiKey: v }))}
                    />
                  </Field>
                </Show>

                {/* BaseURL 自定义 */}
                <Field label="自定义 Base URL (可选)" hint="用于中转站、本地代理或私有化部署地址">
                  <Input
                    value={form().baseUrl}
                    placeholder={reg().baseUrl || 'https://...'}
                    onInput={v => setForm(f => ({ ...f, baseUrl: v }))}
                  />
                </Field>

                <div class="pt-3 border-t border-subtle flex justify-end gap-2.5">
                  <Button variant="secondary" onClick={() => setWizardOpen(false)}>
                    取消
                  </Button>
                  <Button variant="primary" loading={saving()} onClick={handleWizardSubmit}>
                    {isFree ? '立即接入' : '保存并接入'}
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
