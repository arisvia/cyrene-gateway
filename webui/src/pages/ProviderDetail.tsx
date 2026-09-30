import { type Component, For, Show, Switch, Match, createSignal, createResource, createEffect, onMount, onCleanup } from 'solid-js'
import { A, useParams, useNavigate } from '@solidjs/router'
import { useGatewayStore } from '@/stores/gateway'
import { api } from '@/lib/api'
import { useToast } from '@/lib/toast'
import { keyPlaceholder } from '@/lib/credentialHint'
import { useOAuthFlow } from '@/lib/oauthFlow'
import { useProviderModels } from '@/lib/providerModels'
import ModelsToolbar from '@/components/provider/ModelsToolbar'
import ModelsPanel from '@/components/provider/ModelsPanel'
import ModelMetaModal from '@/components/provider/ModelMetaModal'
import OAuthFlowModal from '@/components/provider/OAuthFlowModal'
import { useI18n } from '@/i18n'
import type { Provider } from '@/types/domain'
import { Card, Badge, AuthBadge, Button, IconButton, Input, Textarea, Toggle, Field, Empty, Skeleton, Select, Modal, Alert, PageHeader, SegmentedControl, ProviderAvatar, IconBulb, IconCheck, IconClose, confirm, TabTransition } from '@/components/ui'

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
  // ── 多账号管理状态 ──
  const regInfo = () => {
    const p = conn()?.provider
    return p ? store.registryList().find(r => r.id === p || r.alias === p) : undefined
  }

  const accounts = () => {
    const current = conn()
    if (!current) return []
    const reg = regInfo()
    const targetId = reg?.id || current.provider
    return store
      .providers()
      .filter(p => p.provider === targetId)
      .sort((a, b) => (a.priority ?? 50) - (b.priority ?? 50))
  }

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
    const opts: Array<{ value: string; label: string }> = []
    if (modes.includes('api-key') || (!modes.includes('oauth') && regInfo()?.category !== 'oauth')) {
      opts.push({ value: 'api-key', label: t('providerDetail.authApiKeyPat') })
    }
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
    if (!p || addingAccount()) return
    const aType = newAccountAuthType()
    const parsedPriority = Number(newAccountPriority())
    const pVal = Number.isFinite(parsedPriority) ? parsedPriority : 0
    const nVal = newAccountName().trim() || t('providerDetail.accountIndex', { index: accounts().length + 1 })
    if (aType === 'oauth') {
      setAddAccountOpen(false)
      await flowApi.startDeviceFlow(pVal, nVal)
      return
    }
    if (aType === 'api-key' && !newAccountApiKey().trim()) {
      toast.error(t('toast.requireApiKey'))
      return
    }
    setAddingAccount(true)
    try {
      const added = await store.addProvider({
        provider: p,
        authType: aType,
        name: nVal,
        priority: pVal,
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
    } catch {
      // Error is automatically surfaced by api.ts
    } finally {
      setAddingAccount(false)
    }
  }

  // ── 设备码 OAuth 工作流状态 ──
  const flowApi = useOAuthFlow({
    providerId: () => conn()?.provider,
    account: conn,
    accountName: name,
    displayName: providerDisplayName,
    onAuthorized: async () => {
      await store.loadProvidersOnly()
      await load()
      refetchOAuth()
    },
  })


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

  const modelsApi = useProviderModels({
    connectionId: () => params.id,
    providerId: () => conn()?.provider,
  })

  const { payload, activeModels, allDisplayModels } = modelsApi
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
    } catch {
      // Error surfaced by api.ts
    } finally {
      setTesting(false)
    }
  }

  return (
    <div class="space-y-5 stagger">
      <Show when={!loading() && conn()}>
        {c => (
          <PageHeader
            title={
              <div class="flex flex-wrap items-center gap-2.5 min-w-0 wrap-anywhere">
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
                <AuthBadge authType={c().authType} />
                <span class="text-xs text-faint font-mono px-1.5 py-0.5 rounded bg-surface-inset border border-subtle max-w-full wrap-anywhere">
                  {c().provider}
                </span>
                <span class="text-xs text-faint">{t('providerDetail.accountCount', { count: accounts().length })}</span>
              </div>
            }
            actions={
              <div class="flex flex-wrap items-center gap-3 min-w-0">
                <Button size="sm" variant="secondary" loading={testing()} onClick={runTest}>{t('providerDetail.testConnection')}</Button>
                <Toggle ariaLabel={`${t('common.enabled')}: ${providerDisplayName()}`} checked={c().isActive} onChange={() => { store.toggleProvider(c()); load() }} />
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
                  { value: 'models', label: `${t('providerDetail.tabs.models')} (${allDisplayModels().length})` },
                  { value: 'chat', label: t('providerDetail.tabs.chat') },
                ]}
              />

              {/* 仅在 models 视图下激活的快捷操作工具组 */}
              <Show when={tab() === 'models'}>
                <ModelsToolbar models={modelsApi} />
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
            <TabTransition
              value={tab()}
              order={['overview', 'models', 'chat']}
            >
              {currentTab => (
                <Switch>
                  <Match when={currentTab === 'overview'}>
                    <div class="grid lg:grid-cols-12 gap-5 items-start">
                {/* 左侧 (5 cols)：多账号与调度看板 (带独立滚动区，不会被挤出视野) */}
                <div class="lg:col-span-5 min-w-0 space-y-3">
                  <Card class="p-4 space-y-3">
                    <div class="flex flex-wrap items-center justify-between gap-3 pb-2.5 border-b border-subtle">
                      <div>
                        <h3 class="text-sm font-semibold flex flex-wrap items-center gap-2">
                          <span>{t('providerDetail.accountsAndPool')}</span>
                          <Badge tone="blue">{t('providerDetail.accountCount', { count: accounts().length })}</Badge>
                        </h3>
                        <p class="text-xs text-faint mt-0.5 leading-relaxed">
                          {t('providerDetail.schedulingHint')}
                        </p>
                      </div>
                      <Button
                        size="sm"
                        variant="primary"
                        onClick={() => {
                          const modes = regInfo()?.authModes || [regInfo()?.authType || 'api-key']
                          const initialType = modes.includes('api-key') ? 'api-key' : (modes[0] || 'oauth')
                          const nextNum = accounts().length + 1
                          setNewAccountName(t('providerDetail.accountIndex', { index: nextNum }))
                          setNewAccountAuthType(initialType)
                          setNewAccountApiKey('')
                          setNewAccountPriority(String((Number(priority()) || 0) + 10))
                          setAddAccountOpen(true)
                        }}
                      >
                        {t('providerDetail.addAccount')}
                      </Button>
                    </div>

                    {/* 账号列表滚动容器 */}
                    <div class="max-h-[360px] lg:max-h-[max(12rem,calc(100dvh-340px))] overflow-y-auto px-1 space-y-2">
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
                              <div class="flex items-center justify-between gap-2 min-w-0">
                                <div class="flex items-center gap-1.5 min-w-0 flex-1">
                                  <span class={`text-[11px] font-mono font-bold px-1.5 py-0.5 rounded border shrink-0 ${
                                    isCurrent() ? 'bg-accent text-on-accent border-accent' : 'bg-bg text-faint border-subtle'
                                  }`}>
                                    #{idx() + 1}
                                  </span>
                                  <span class="text-xs font-semibold text-foreground truncate min-w-0 shrink-0 max-w-[130px] sm:max-w-[160px]" title={getAccountDisplayName(acc, idx())}>
                                    {getAccountDisplayName(acc, idx())}
                                  </span>
                                  <Show when={acc.email && acc.email !== getAccountDisplayName(acc, idx())}>
                                    <span class="text-xs text-faint truncate min-w-0 font-mono" title={acc.email}>
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
                                    ariaLabel={`${t('common.enabled')}: ${getAccountDisplayName(acc, idx())}`}
                                    checked={acc.isActive}
                                    onChange={async () => {
                                      await store.toggleProvider(acc)
                                      await store.loadProvidersOnly()
                                      await load()
                                    }}
                                  />
                                  <Show when={accounts().length > 1}>
                                    <IconButton size="sm" variant="ghost"
                                      type="button"
                                      class="text-muted hover:text-danger"
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
                                    </IconButton>
                                  </Show>
                                </div>
                              </div>

                              <div class="mt-2.5 flex items-center gap-2 flex-wrap text-xs">
                                <AuthBadge authType={acc.authType} />
                                <span class={`inline-flex items-center h-5 gap-1.5 text-[10px] px-2 rounded-full border font-mono ${
                                  idx() === 0
                                    ? 'bg-accent/15 border-accent/40 text-accent font-semibold'
                                    : 'bg-control border-subtle text-muted'
                                }`}>
                                  <span class={`inline-block w-1.5 h-1.5 rounded-full shrink-0 ${idx() === 0 ? 'bg-accent animate-pulse' : 'bg-faint'}`} />
                                  <span class="leading-none">{t('providerDetail.priorityValue', { value: acc.priority })}</span>
                                </span>
                                <Show when={accounts().filter(a => (a.priority ?? 0) === (acc.priority ?? 0)).length > 1}>
                                  <span class="text-[10px] font-sans text-accent/80 font-medium">({t('providers.loadBalancing')})</span>
                                </Show>
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
                    <div class="p-2.5 rounded-lg bg-surface-inset text-xs text-faint leading-relaxed flex items-start gap-1.5">
                      <IconBulb size={14} class="text-accent shrink-0 mt-0.5" />
                      <div><span class="text-foreground font-medium">{t('providerDetail.dispatchMechanismTitle')}</span>{t('providerDetail.dispatchMechanismDesc')}</div>
                    </div>
                  </Card>
                </div>

                {/* 右侧 (7 cols)：当前正在编辑的账号详情与端点配置 (高度受控，内部滚动，底部按钮绝对吸底可见) */}
                <div class="lg:col-span-7 min-w-0">
                  <Card class="p-4 sm:p-5 flex flex-col min-w-0 lg:max-h-[max(30rem,calc(100dvh-220px))]">
                    <div class="flex flex-wrap items-start justify-between gap-3 pb-3 border-b border-subtle shrink-0">
                      <div class="min-w-0 flex-1 basis-full sm:basis-64">
                        <div class="text-sm font-semibold flex flex-wrap items-center gap-2 wrap-anywhere">
                          <span class="min-w-0">{t('providerDetail.editAccountWith', { name: getAccountDisplayName(c()) })}</span>
                          <AuthBadge authType={c().authType} />
                        </div>
                        <div class="text-xs text-faint mt-1 font-mono flex flex-wrap items-center gap-x-2 gap-y-1 wrap-anywhere">
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
                          class="text-xs text-accent hover:underline inline-flex items-center gap-1 max-w-full wrap-anywhere"
                        >
                          {t('providerDetail.getOfficialKey')}
                        </a>
                      </Show>
                    </div>

                    {/* 表单内容滚动区 */}
                    <div class="min-w-0 lg:flex-1 lg:min-h-0 lg:overflow-y-auto px-1 py-3 space-y-4">
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
                          (payload().authModes?.includes('api-key') && c().authType !== 'oauth')
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
                                  flowApi.cancelDeviceFlow()
                                  flowApi.startDeviceFlow(c().priority ?? 0, c().name ?? '')
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
                          label={c().provider === 'qoder' || c().provider === 'qoder-cn' ? t('providerDetail.qoderCredLabel') : t('providerDetail.authCredential')}
                          hint={
                            c().provider === 'qoder' || c().provider === 'qoder-cn'
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
                            placeholder={keyPlaceholder(c().provider)}
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
                        <Button size="sm" variant="ghost"
                          type="button"
                          class="text-xs text-muted hover:text-foreground gap-1.5 py-2 font-medium max-w-full min-w-0! h-auto! min-h-8 whitespace-normal! text-left"
                          onClick={() => setShowAdvanced(!showAdvanced())}
                        >
                          <span>{showAdvanced() ? '▼' : '▶'}</span>
                          <span>{t('providerDetail.customHeadersOverride')}</span>
                        </Button>

                        <Show when={showAdvanced()}>
                          <div class="mt-3 p-3.5 rounded-control bg-bg-elevated/50 border border-subtle/70 space-y-3 text-xs">
                            <Show when={payload().defaultHeaders && Object.keys(payload().defaultHeaders!).length > 0}>
                              <div>
                                <div class="text-faint mb-1.5 font-medium">{t('providerDetail.defaultHeaderHint')}</div>
                                <div class="bg-bg/80 p-2 rounded border border-subtle font-mono text-[11px] space-y-1">
                                  <For each={Object.entries(payload().defaultHeaders!)}>
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
                              <Textarea
                                rows={4}
                                value={customHeadersText()}
                                onInput={setCustomHeadersText}
                                placeholder={'{\n  "User-Agent": "MyClient/2.0",\n  "anthropic-version": "2023-06-01"\n}'}
                                class="px-3 py-2 text-xs font-mono"
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
                    <div class="flex flex-wrap items-center justify-between gap-3 pt-3 border-t border-subtle shrink-0">
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
            </Match>
            <Match when={currentTab === 'models'}>
              <ModelsPanel models={modelsApi} />
            </Match>
            <Match when={currentTab === 'chat'}>
              <div class="space-y-4">
                <Card class="p-4 flex flex-wrap items-center justify-between gap-3">
                  <div class="flex flex-col sm:flex-row sm:items-center gap-3 flex-1 min-w-0 basis-full lg:basis-64">
                    <span class="text-xs text-faint shrink-0">{t('providerDetail.chatModel')}</span>
                    <Select
                      class="flex-1 min-w-0 w-full"
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
                  <div class="flex flex-wrap items-center gap-3 min-w-0">
                    <Show when={selectedModel()}>
                      <Badge tone="blue" class="max-w-full min-w-0 whitespace-normal! wrap-anywhere">{selectedModel()}</Badge>
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
                    class="p-5 h-[420px] max-h-[max(16rem,calc(100dvh-360px))] overflow-y-auto space-y-3 scroll-smooth"
                  >
                  <Show
                    when={chatHistory().length > 0}
                    fallback={<Empty message={t('providerDetail.chatEmpty', { name: providerDisplayName() })} />}
                  >
                    <For each={chatHistory()}>
                      {msg => (
                        <div class={`flex flex-col ${msg.role === 'user' ? 'items-end' : 'items-start'}`}>
                          <div class={`min-w-0 max-w-full sm:max-w-[80%] px-4 py-2.5 rounded-2xl text-sm whitespace-pre-wrap wrap-anywhere leading-relaxed shadow-xs ${
                            msg.role === 'user'
                              ? 'bg-accent text-on-accent'
                              : 'bg-hover text-foreground border border-subtle'
                          }`}>
                            {msg.content}
                          </div>
                          <Show when={msg.role === 'assistant' && msg.servedModel}>
                            <span class="mt-1 max-w-full px-1.5 py-0.5 text-xs text-faint font-mono wrap-anywhere">
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
            </Match>
          </Switch>
        )}
      </TabTransition>
          </div>
        )}
      </Show>

      <ModelMetaModal models={modelsApi} />

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
          <Show when={newAccountAuthType() === 'api-key'}>
            <Field label={t('providerDetail.accountNameLabel')} hint={t('providerDetail.accountNameHint')}>
              <Input
                value={newAccountName()}
                onInput={setNewAccountName}
                placeholder={t('providerDetail.newAccountPlaceholder', { index: accounts().length + 1 })}
              />
            </Field>
          </Show>
          <Show when={accountAuthOptions().length > 1}>
            <Field label={t('providerDetail.authMethodLabel')} hint={t('providerDetail.authMethodSelectHint')}>
              <Select
                value={newAccountAuthType()}
                options={accountAuthOptions()}
                onChange={setNewAccountAuthType}
              />
            </Field>
          </Show>
          <Show when={newAccountAuthType() === 'api-key'}>
            <Field
              label={conn()?.provider === 'qoder' || conn()?.provider === 'qoder-cn' ? t('providerDetail.qoderCredLabel') : t('providerDetail.authCredential')}
              hint={t('providerDetail.credSecureHint')}
            >
              <div class="space-y-1.5">
                <Input
                  type="password"
                  value={newAccountApiKey()}
                  onInput={setNewAccountApiKey}
                  placeholder={keyPlaceholder(conn()?.provider)}
                />
                <Show when={regInfo()?.apiKeyUrl}>
                  <div class="flex justify-end">
                    <a
                      href={regInfo()!.apiKeyUrl!}
                      target="_blank"
                      rel="noreferrer"
                      class="text-xs text-accent hover:underline inline-flex items-center gap-1 max-w-full wrap-anywhere"
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
          <Show when={newAccountAuthType() === 'api-key'}>
            <Field label={t('providerDetail.priorityLabel')} hint={t('providerDetail.newAccountPriorityHint')}>
              <Input
                type="number"
                value={newAccountPriority()}
                onInput={setNewAccountPriority}
                class="w-32!"
              />
            </Field>
          </Show>
          <div class="flex items-center justify-end gap-2 pt-3 border-t border-subtle">
            <Button size="sm" variant="secondary" onClick={() => setAddAccountOpen(false)}>
              {t('common.cancel')}
            </Button>
            <Button size="sm" variant="primary" loading={addingAccount()} onClick={handleAddAccountSubmit}>
              {newAccountAuthType() === 'oauth' ? t('providers.startOAuth') : t('providerDetail.saveAccount')}
            </Button>
          </div>
        </div>
      </Modal>
      {/* 统一设备码 OAuth 授权弹窗 */}
      <OAuthFlowModal flow={flowApi} displayName={providerDisplayName} />
    </div>
  )
}

export default ProviderDetail
