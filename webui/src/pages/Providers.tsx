import {type Component, createMemo, createSignal, For, onCleanup, onMount, Show} from 'solid-js'
import {A} from '@solidjs/router'
import {useGatewayStore} from '@/stores/gateway'
import {useI18n} from '@/i18n'
import {
  Badge,
  Button,
  Card,
  Empty,
  Field,
  Input,
  Modal,
  PageHeader,
  ProviderAvatar,
  SegmentedControl,
  Select,
  IconChat,
  IconPalette,
  IconVolume,
  IconMic,
  IconVideo,
  IconVector,
  IconSearch,
  IconGlobe,
  IconZap,
  IconCheck,
  IconClose,
  Alert,
} from '@/components/ui'
import {api, apiPost} from '@/lib/api'
import {useToast} from '@/lib/toast'
import type {BadgeTone, Provider, RegistryProvider} from '@/types/domain'


const Providers: Component = () => {
  const store = useGatewayStore()
  const { t } = useI18n()
  const toast = useToast()

  const categoryLabel = (): Record<string, string> => ({
    all: t('providers.categories.all'),
    apikey: t('providers.categories.apiKey'),
    oauth: t('providers.categories.oauth'),
    freeTier: t('providers.categories.freeTier'),
    custom: t('providers.categories.custom'),
    media: t('providers.categories.media'),
  })

  const authTypeLabel = (): Record<string, string> => ({
    'api-key': t('providers.authTypes.apiKey'),
    apikey: t('providers.authTypes.apiKey'),
    oauth: t('providers.authTypes.oauth'),
  })

  const getCapConfig = (capKey: string) => {
    const meta: Record<string, { tone: BadgeTone; icon: Component<{ size?: number; class?: string }> }> = {
      llm: { tone: 'blue', icon: IconChat },
      image: { tone: 'blue', icon: IconPalette },
      tts: { tone: 'green', icon: IconVolume },
      stt: { tone: 'amber', icon: IconMic },
      video: { tone: 'red', icon: IconVideo },
      embedding: { tone: 'gray', icon: IconVector },
      'web-search': { tone: 'amber', icon: IconSearch },
      'web-fetch': { tone: 'gray', icon: IconGlobe },
    }
    const capLabels: Record<string, string> = {
      llm: t('providers.capabilities.llm'),
      image: t('providers.capabilities.image'),
      tts: t('providers.capabilities.tts'),
      stt: t('providers.capabilities.stt'),
      video: t('providers.capabilities.video'),
      embedding: t('providers.capabilities.embedding'),
      'web-search': t('providers.capabilities.webSearch'),
      'web-fetch': t('providers.capabilities.webFetch'),
    }
    const m = meta[capKey] || { tone: 'gray' as BadgeTone, icon: IconZap }
    return {
      label: capLabels[capKey] || capKey,
      tone: m.tone,
      icon: m.icon,
    }
  }
  // 顶层视图切换：'connections' | 'catalog'
  const [activeTab, setActiveTab] = createSignal<'connections' | 'catalog'>('connections')

  // 搜索与分类/能力过滤
  const [query, setQuery] = createSignal('')
  const [catFilter, setCatFilter] = createSignal('')
  const [capFilter, setCapFilter] = createSignal('')
  // 添加/配置向导状态
  const [wizardOpen, setWizardOpen] = createSignal(false)
  const [selectedReg, setSelectedReg] = createSignal<RegistryProvider | null>(null)
  const [form, setForm] = createSignal({
    name: '',
    authType: 'api-key',
    apiKey: '',
    baseUrl: '',
    priority: '50',
  })
  const [saving, setSaving] = createSignal(false)
  const [refreshing, setRefreshing] = createSignal(false)

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
  // 过滤与聚合后的已接入供应商列表（以 Provider 为唯一主体聚合多账号）
  interface ProviderConnectionGroup {
    providerId: string
    providerName: string
    color?: string
    category: string
    connections: Provider[]
    activeCount: number
    primaryConnectionId: string
  }

  const groupedConnections = createMemo<ProviderConnectionGroup[]>(() => {
    const q = query().toLowerCase().trim()
    const cat = catFilter()
    const cap = capFilter()
    const all = store.providers()
    // 按 Provider 标识聚合账号
    const map: Record<string, Provider[]> = {}
    for (const p of all) {
      if (!map[p.provider]) map[p.provider] = []
      map[p.provider].push(p)
    }

    const groups: ProviderConnectionGroup[] = []
    for (const [providerId, conns] of Object.entries(map)) {
      const reg = registryFor(providerId)
      // 我的连接页专注于 LLM 对话管理：非 LLM 提供商转由媒体工作台管理
      const caps = reg?.capabilities || (reg?.category === 'media' ? [] : ['llm'])
      if (!caps.includes('llm')) {
        continue
      }

      // 组内账号按优先级升序排序（数值小者优先调度）
      conns.sort((a, b) => (a.priority ?? 50) - (b.priority ?? 50))

      const providerName = reg?.name || conns[0]?.name || providerId
      const category = reg?.category || conns[0]?.authType || 'apikey'
      const activeCount = conns.filter(c => c.isActive).length

      if (cat) {
        const matchesCat = conns.some(c => c.authType === cat || (cat === 'api-key' && c.authType === 'apikey')) || category === cat
        if (!matchesCat) continue
      }

      if (cap) {
        if (!caps.includes(cap)) continue
      }
      if (q) {
        const matchProvider = providerName.toLowerCase().includes(q) || providerId.toLowerCase().includes(q)
        const matchConns = conns.some(c =>
          (c.name || '').toLowerCase().includes(q) ||
          (c.email || '').toLowerCase().includes(q) ||
          c.id.toLowerCase().includes(q),
        )
        if (!matchProvider && !matchConns) continue
      }

      groups.push({
        providerId,
        providerName,
        color: reg?.color,
        category,
        connections: conns,
        activeCount,
        primaryConnectionId: conns[0].id,
      })
    }

    // 按活跃账号数与名称排序
    return groups.sort((a, b) => b.activeCount - a.activeCount || a.providerName.localeCompare(b.providerName))
  })

  // 隐藏已添加的提供商过滤（默认只看尚未接入的供应商，市场更清爽）
  const [hideAdded, setHideAdded] = createSignal(true)
  const connectedProviderIds = createMemo(() => new Set(store.providers().map(p => p.provider)))
  const connectedCount = () => connectedProviderIds().size

  // 按品牌归集或单例展示的市场提供商列表
  interface CatalogBrandGroup {
    brandKey: string
    name: string
    items: RegistryProvider[]
  }

  // 自定义通用接口分类组与主流供应商分组分开
  const customBrandGroups = createMemo<CatalogBrandGroup[]>(() => {
    const list = store.registryList()
    const q = query().toLowerCase().trim()
    const cat = catFilter()
    const cap = capFilter()
    const connected = connectedProviderIds()
    const hide = hideAdded()

    const matched = list.filter(r => {
      const isCustom = r.category === 'custom' || r.id.startsWith('custom-')
      if (!isCustom) return false
      if (hide && connected.has(r.id)) return false
      if (cat && r.category !== cat) return false
      if (cap && !r.capabilities?.includes(cap)) return false
      if (!q) return true
      return (
        r.name.toLowerCase().includes(q) ||
        r.id.toLowerCase().includes(q) ||
        (r.brand || '').toLowerCase().includes(q)
      )
    })

    return matched.map(r => ({
      brandKey: r.id,
      name: r.name,
      items: [r],
    }))
  })

  const brandGroups = createMemo<CatalogBrandGroup[]>(() => {
    const list = store.registryList()
    const q = query().toLowerCase().trim()
    const cat = catFilter()
    const cap = capFilter()
    const connected = connectedProviderIds()
    const hide = hideAdded()

    // 过滤掉通用自定义接口，只保留标准供应商
    const matched = list.filter(r => {
      const isCustom = r.category === 'custom' || r.id.startsWith('custom-')
      if (isCustom) return false
      if (hide && connected.has(r.id)) return false
      if (cat && r.category !== cat) return false
      if (cap && !r.capabilities?.includes(cap)) return false
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

  // 向导中测试凭证状态与验证通过标记
  const [testingCreds, setTestingCreds] = createSignal(false)
  const [testedCreds, setTestedCreds] = createSignal<{ ok: boolean; msg: string } | null>(null)

  // 向导直连 OAuth 流程状态
  const [wizardOAuthFlow, setWizardOAuthFlow] = createSignal<{
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
  const [wizardOAuthPolling, setWizardOAuthPolling] = createSignal(false)
  const [wizardOAuthError, setWizardOAuthError] = createSignal('')
  const [wizardOAuthCopied, setWizardOAuthCopied] = createSignal(false)
  const [wizardIsImport, setWizardIsImport] = createSignal(false)
  const [wizardImportToken, setWizardImportToken] = createSignal('')
  const [wizardImporting, setWizardImporting] = createSignal(false)
  let wizardPollTimer: number | undefined

  onCleanup(() => {
    clearInterval(wizardPollTimer)
  })

  function cancelWizardOAuth() {
    clearInterval(wizardPollTimer)
    wizardPollTimer = undefined
    setWizardOAuthPolling(false)
    setWizardOAuthFlow(null)
    setWizardOAuthError('')
    setWizardIsImport(false)
    setWizardImportToken('')
  }

  // 打开添加向导
  function openWizard(reg: RegistryProvider) {
    cancelWizardOAuth()
    setSelectedReg(reg)
    setTestedCreds(null)
    setTestingCreds(false)
    const defaultAuth = reg.authType === 'oauth' ? 'oauth' : 'api-key'
    setForm({
      name: reg.name,
      authType: defaultAuth,
      apiKey: '',
      baseUrl: reg.baseUrl || '',
      priority: String(reg.priority ?? 50),
    })
    setWizardOpen(true)
  }


  // 向导中测试 API Key / 凭据有效性
  async function handleTestWizardCreds() {
    const reg = selectedReg()
    if (!reg) return
    const f = form()
    const isCustom = reg.category === 'custom' || reg.id.startsWith('custom-')
    if (isCustom && !f.baseUrl.trim()) {
      toast.error(t('toast.customBaseUrlRequired'))
      return
    }
    if (!f.apiKey.trim()) {
      toast.error(t('toast.apiKeyRequired'))
      return
    }

    setTestingCreds(true)
    setTestedCreds(null)
    try {
      const res = (await apiPost('/api/providers/test-credentials', {
        provider: reg.id,
        apiKey: f.apiKey.trim(),
        baseUrl: f.baseUrl.trim() || undefined,
      })) as { ok: boolean; error?: string; latency?: string }

      if (res.ok) {
        setTestedCreds({ ok: true, msg: t('providers.credVerifiedMsg', { latency: res.latency || t('providerDetail.normal') }) })
        toast.success(t('toast.credTestSuccess', { latency: res.latency || 'ok' }))
      } else {
        setTestedCreds({ ok: false, msg: res.error || 'error' })
        toast.error(t('toast.credTestFailed', { error: res.error || 'error' }))
      }
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : String(e)
      setTestedCreds({ ok: false, msg })
      toast.error(t('toast.credTestFailed', { error: msg }))
    } finally {
      setTestingCreds(false)
    }
  }

  // 启动向导中的直接 OAuth 授权流程（根据供应商支持的流类型智能分流）
  async function startWizardOAuth() {
    const reg = selectedReg()
    if (!reg) return
    setWizardOAuthError('')
    setWizardOAuthPolling(true)

    try {
      // 先查询该供应商真实支持的 OAuth 流类型
      const statusRes = (await api(`/api/oauth/${reg.id}/status`)) as {
        flowType?: string
        connections?: Array<{ id: string; expiresAt?: string; updatedAt?: string }>
      }
      const flowType = statusRes?.flowType
      const initialConnMap = new Map((statusRes?.connections || []).map(c => [c.id, `${c.expiresAt || ''}:${c.updatedAt || ''}`]))

      // 1. 令牌导入流 (如 Cursor 等)
      if (flowType === 'import_token') {
        setWizardOAuthPolling(false)
        setWizardIsImport(true)
        return
      }

      // 2. 网页授权码 / PKCE 流（如 Antigravity, Claude, Google 等）
      if (flowType === 'authorization_code_pkce' || flowType === 'authorization_code') {
        const callbackUri = `${window.location.origin}/api/oauth/${reg.id}/callback`
        const authRes = (await api(`/api/oauth/${reg.id}/authorize?redirect_uri=${encodeURIComponent(callbackUri)}`)) as {
          authorizeUrl: string
          state: string
        }
        if (authRes?.authorizeUrl) {
          window.open(authRes.authorizeUrl, '_blank')
          toast.info(t('toast.oauthWindowOpened', { name: reg.name }))
        }

        // 轮询检测是否产生新连接
        if (wizardPollTimer) clearInterval(wizardPollTimer)
        wizardPollTimer = window.setInterval(async () => {
          try {
            const pollStatus = (await api(`/api/oauth/${reg.id}/status`)) as {
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
              clearInterval(wizardPollTimer)
              wizardPollTimer = undefined
              setWizardOAuthPolling(false)
              toast.success(t('toast.oauthSuccess', { name: reg.name }))
              setWizardOpen(false)
              setActiveTab('connections')
              await store.loadProvidersOnly()
            }
          } catch {
            // 忽略轮询网络偶发错误
          }
        }, 2500)
        return
      }

      // 3. 设备码流（Device Code Flow，如 GitHub, Kimi, CodeBuddy, Qoder, Grok CLI 等）
      const raw = (await apiPost(`/api/oauth/${reg.id}/device-code`)) as Record<string, unknown>
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
      setWizardOAuthFlow(res)
      const targetUrl = res.verificationUriComplete || res.verificationUri
      if (targetUrl) {
        window.open(targetUrl, '_blank')
      }

      if (wizardPollTimer) clearInterval(wizardPollTimer)
      let inFlight = false
      const intervalMs = res.interval ? res.interval * 1000 : 2500
      wizardPollTimer = window.setInterval(async () => {
        if (inFlight) return
        inFlight = true
        try {
          const pollRes = (await apiPost(`/api/oauth/${reg.id}/device-code/poll`, {
            deviceCode: res.deviceCode,
            nonce: res.nonce,
            codeVerifier: res.codeVerifier,
            machineId: res.machineId,
            extraData: res.extraData,
          })) as { success?: boolean; error?: string; pending?: boolean; connection?: Provider }
          if (pollRes?.success) {
            clearInterval(wizardPollTimer)
            wizardPollTimer = undefined
            setWizardOAuthPolling(false)
            setWizardOAuthFlow(null)
            toast.success(t('toast.oauthSuccess', { name: reg.name }))
            setWizardOpen(false)
            setActiveTab('connections')
            await store.loadProvidersOnly()
          } else if (pollRes?.error && !pollRes?.pending) {
            clearInterval(wizardPollTimer)
            wizardPollTimer = undefined
            setWizardOAuthPolling(false)
            setWizardOAuthError(pollRes.error)
          }
        } catch {
          // 忽略轮询偶发错误
        } finally {
          inFlight = false
        }
      }, intervalMs)
    } catch (e: unknown) {
      setWizardOAuthPolling(false)
      const msg = e instanceof Error ? e.message : String(e)
      setWizardOAuthError(msg)
      toast.error(t('toast.oauthFailed', { error: msg }))
    }
  }

  // 提交 Token 导入
  async function handleWizardImportSubmit() {
    const reg = selectedReg()
    if (!reg) return
    const token = wizardImportToken().trim()
    if (!token) {
      toast.error(t('toast.tokenRequired'))
      return
    }
    setWizardImporting(true)
    try {
      await apiPost(`/api/oauth/${reg.id}/import`, {
        accessToken: token,
        name: form().name || reg.name,
      })
      toast.success(t('toast.tokenImportSuccess', { name: reg.name }))
      cancelWizardOAuth()
      setWizardOpen(false)
      setActiveTab('connections')
      await store.loadProvidersOnly()
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : String(e)
      setWizardOAuthError(msg)
      toast.error(t('toast.tokenImportFailed', { error: msg }))
    } finally {
      setWizardImporting(false)
    }
  }

  // 提交向导表单
  async function handleWizardSubmit() {
    const reg = selectedReg()
    if (!reg) return

    const f = form()
    const normAuth = f.authType === 'apikey' ? 'api-key' : f.authType

    // 若是 OAuth 模式，直接点击即发起授权，不预存空账号
    if (normAuth === 'oauth') {
      await startWizardOAuth()
      return
    }

    if (normAuth === 'api-key') {
      const isCustom = reg.category === 'custom' || reg.id.startsWith('custom-')
      if (isCustom && !f.baseUrl.trim()) {
        toast.error(t('toast.customBaseUrlRequired'))
        return
      }
      if (!f.apiKey.trim()) {
        toast.error(t('toast.requireApiKey'))
        return
      }
      // 必须通过凭证测试才允许保存
      if (!testedCreds() || !testedCreds()!.ok) {
        toast.error(t('toast.credTestBeforeSave'))
        return
      }
    }

    setSaving(true)
    try {
      await store.addProvider({
        provider: reg.id,
        authType: normAuth,
        name: f.name || reg.name,
        priority: Number(f.priority) || reg.priority || 50,
        data: {
          apiKey: f.apiKey.trim() || undefined,
          baseUrl: f.baseUrl.trim() || undefined,
        },
      })
      setWizardOpen(false)
      setActiveTab('connections')
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : String(e)
      toast.error(t('toast.addProviderFailed', { error: msg }))
      console.error('[providers] add provider failed:', e)
    } finally {
      setSaving(false)
    }
  }

  // 刷新所有模型与连接
  async function handleRefreshAll() {
    setRefreshing(true)
    try {
      await store.loadProvidersOnly()
      await store.loadCore()
      toast.success(t('toast.refreshSuccess'))
    } catch (e: unknown) {
      toast.error(t('toast.refreshFailed'))
    } finally {
      setRefreshing(false)
    }
  }

  return (
    <div class="space-y-5 stagger">
      <PageHeader
        title={t('providers.title')}
        subtitle={t('providers.subtitle')}
        actions={
          <SegmentedControl
            value={activeTab()}
            onChange={tab => {
              setActiveTab(tab)
              setCatFilter('')
            }}
            options={[
              { value: 'connections', label: `${t('providers.tabs.connections')} (${store.providers().length})` },
              { value: 'catalog', label: `${t('providers.tabs.catalog')} (${store.registryList().length})` },
            ]}
          />
        }
      >
        {/* 搜索与过滤工具栏：保持半透明轻量容器，避免在 glass-sticky 上再叠一层实心面板 */}
        <div class="p-3 rounded-xl bg-black/4 dark:bg-white/6 border border-black/8 dark:border-white/10 flex flex-wrap items-center justify-between gap-3">
          <div class="flex flex-wrap items-center gap-3 flex-1">
            <Input
              class="w-64!"
              placeholder={activeTab() === 'connections' ? t('providers.searchConnPlaceholder') : t('providers.searchCatalogPlaceholder')}
              value={query()}
              onInput={setQuery}
            />

            <Select
              value={capFilter()}
              onChange={setCapFilter}
              options={[
                { value: '', label: t('providers.capabilities.all') },
                { value: 'llm', label: t('providers.capabilities.llm') },
                { value: 'image', label: t('providers.capabilities.image') },
                { value: 'tts', label: t('providers.capabilities.tts') },
                { value: 'stt', label: t('providers.capabilities.stt') },
                { value: 'video', label: t('providers.capabilities.video') },
                { value: 'embedding', label: t('providers.capabilities.embedding') },
                { value: 'web-search', label: t('providers.capabilities.webSearch') },
                { value: 'web-fetch', label: t('providers.capabilities.webFetch') },
              ]}
            />

            <Show when={activeTab() === 'connections'}>
              <Select
                value={catFilter()}
                options={[
                  { value: '', label: t('providers.authTypes.all') },
                  { value: 'api-key', label: t('providers.authTypes.apiKey') },
                  { value: 'oauth', label: t('providers.authTypes.oauth') },
                ]}
                onChange={setCatFilter}
              />
            </Show>

            <Show when={activeTab() === 'catalog'}>
              <Select
                value={catFilter()}
                options={[
                  { value: '', label: t('providers.categories.all') },
                  { value: 'apikey', label: t('providers.categories.apiKey') },
                  { value: 'oauth', label: t('providers.categories.oauth') },
                  { value: 'freeTier', label: t('providers.categories.freeTier') },
                  { value: 'custom', label: t('providers.categories.custom') },
                  { value: 'media', label: t('providers.categories.media') },
                ]}
                onChange={setCatFilter}
              />
              <Button
                size="sm"
                variant={hideAdded() ? 'secondary' : 'ghost'}
                class={`text-xs gap-1.5 ${hideAdded() ? 'border-accent/40 text-accent font-medium' : ''}`}
                onClick={() => setHideAdded(!hideAdded())}
                title={hideAdded() ? t('providers.catalogFilterAll') : t('providers.catalogFilterUnadded')}
              >
                <Show when={hideAdded()} fallback={<span>{t('providers.showAllMarket')}</span>}>
                  <IconCheck size={12} />
                  <span>{t('providers.hideAddedActive')}</span>
                </Show>
                <Show when={connectedCount() > 0}>
                  <span class="text-[10px] opacity-75 font-mono">
                    ({hideAdded() ? t('providers.hiddenCount', { count: connectedCount() }) : t('providers.connectedCount', { count: connectedCount() })})
                  </span>
                </Show>
              </Button>
            </Show>
          </div>

          <div class="flex items-center gap-3 text-xs text-faint">
            <span class="hidden sm:inline">
              {t('providers.matchedCount', { count: activeTab() === 'connections' ? groupedConnections().length : brandGroups().length })}
            </span>
            <Button size="sm" variant="secondary" loading={refreshing()} onClick={handleRefreshAll}>
              {t('common.refresh')}
            </Button>
            <Show when={activeTab() === 'connections'}>
              <Button size="sm" variant="primary" onClick={() => setActiveTab('catalog')}>
                + {t('providers.addConnection')}
              </Button>
            </Show>
          </div>
        </div>
      </PageHeader>

      {/* 视窗 1：我的连接列表 */}
      <Show when={activeTab() === 'connections'}>
        <Show
          when={groupedConnections().length > 0}
          fallback={
            <Card class="p-12 text-center space-y-4">
              <Empty message={t('providers.emptyTitle')} />
              <p class="text-xs text-faint max-w-md mx-auto leading-relaxed">
                {t('providers.emptyCatalogDesc')}
              </p>
              <div class="flex justify-center gap-3 pt-2">
                <Button variant="primary" onClick={() => setActiveTab('catalog')}>
                  {t('providers.emptyAction')}
                </Button>
              </div>
            </Card>
          }
        >
          <div class="grid gap-4">
            <For each={groupedConnections()}>
              {group => {
                const reg = () => registryFor(group.providerId)
                const hasMultiple = () => group.connections.length > 1
                const allActive = () => group.activeCount === group.connections.length
                const noneActive = () => group.activeCount === 0

                return (
                  <Card hover class="p-4 group shadow-sm transition-all">
                    <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-3">
                      <div class="flex items-center gap-3.5 min-w-0">
                        <ProviderAvatar
                          provider={group.providerId}
                          name={group.providerName}
                          color={group.color}
                          size="md"
                        />
                        <div class="min-w-0 flex-1">
                          {/* 第一行：提供商名称与状态 Badge 队列 */}
                          <div class="flex items-center gap-2 flex-wrap">
                            <div class="w-36 sm:w-44 shrink-0 truncate">
                              <A
                                href={`/providers/${group.primaryConnectionId}`}
                                class="font-semibold text-sm text-foreground hover:text-accent transition-colors truncate block"
                                title={group.providerName}
                              >
                                {group.providerName}
                              </A>
                            </div>
                            <Badge tone={allActive() ? 'green' : noneActive() ? 'gray' : 'amber'} class="text-[10px] px-1.5 py-0.5 shrink-0">
                              {allActive() ? t('providers.allEnabled') : noneActive() ? t('providers.allDisabled') : t('providers.partiallyEnabled', { active: group.activeCount, total: group.connections.length })}
                            </Badge>
                            <Badge tone="blue" class="text-[10px] px-1.5 py-0.5 shrink-0">
                              {t('providers.accountsCount', { count: group.connections.length })}
                            </Badge>
                            <Show when={hasMultiple()}>
                              <Badge tone="blue" class="bg-purple-500/15 text-purple-300 border-purple-500/35 text-[10px] px-1.5 py-0.5 shrink-0">
                                {t('providers.fallbackReady')}
                              </Badge>
                            </Show>
                          </div>

                          {/* 第二行：对齐宽度的提供商 ID 与能力徽章队列 */}
                          <div class="flex items-center gap-2 mt-1.5 flex-wrap">
                            <div class="w-36 sm:w-44 shrink-0 truncate">
                              <span class="text-[11px] text-faint font-mono truncate block" title={group.providerId}>
                                ID: {group.providerId}
                              </span>
                            </div>
                            <div class="flex items-center gap-1 flex-wrap">
                              <For each={reg()?.capabilities || ['llm']}>
                                {capKey => {
                                  const conf = getCapConfig(capKey)
                                  const IconComponent = conf.icon
                                  return (
                                    <Badge tone={conf.tone} class="text-[10px] px-1.5 py-0 gap-1 shrink-0">
                                      <IconComponent size={12} />
                                      <span>{conf.label}</span>
                                    </Badge>
                                  )
                                }}
                              </For>
                            </div>
                          </div>
                        </div>
                      </div>
                      {/* 供应商级别操作 */}
                      <div class="flex items-center gap-2 self-start sm:self-auto shrink-0">
                        <Show when={reg()}>
                          <Button
                            size="sm"
                            variant="secondary"
                            onClick={() => openWizard(reg()!)}
                            title={t('providers.addBackupAccountTitle')}
                          >
                            {t('providers.addAccount')}
                          </Button>
                        </Show>
                        <A href={`/providers/${group.primaryConnectionId}`}>
                          <Button size="sm" variant="primary">
                            {t('providers.manageArrow')}
                          </Button>
                        </A>
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
        <div class="space-y-6 pb-16">
          {/* 自定义通用兼容协议 (OpenAI Compatible & Anthropic Compatible) */}
          <Show when={customBrandGroups().length > 0}>
            <div class="space-y-3">
              <div class="flex items-center justify-between px-1">
                <div class="flex items-center gap-2">
                  <span class="text-sm font-semibold text-foreground">{t('providers.customApiTitle')}</span>
                  <Badge tone="blue" class="text-[10px]">{t('providers.customBaseUrlBadge')}</Badge>
                </div>
                <span class="text-xs text-faint">{t('providers.customApiDesc')}</span>
              </div>
              <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
                <For each={customBrandGroups()}>
                  {group => {
                    const reg = () => group.items[0]
                    const connected = () => store.providers().some(p => p.provider === reg().id)
                    return (
                      <Card hover class="p-4 flex flex-col justify-between group border-accent/20 bg-accent/5">
                        <div class="flex items-start justify-between gap-3">
                          <div class="flex items-center gap-3 min-w-0">
                            <ProviderAvatar
                              provider={reg().id}
                              name={group.name}
                              color={reg().color}
                              size="md"
                            />
                            <div class="min-w-0">
                              <div class="font-semibold text-sm text-foreground flex items-center gap-2">
                                <span>{group.name}</span>
                                <span class="text-[10px] font-mono px-1.5 py-0.5 rounded text-muted bg-black/6 dark:bg-white/10 border border-black/10 dark:border-white/15">
                                  {reg().id}
                                </span>
                              </div>
                              <div class="text-xs text-faint mt-1 line-clamp-2">
                                {reg().authHint || t('providers.defaultAuthHint')}
                              </div>
                            </div>
                          </div>
                          <Badge tone="blue" class="shrink-0">
                            {reg().apiType === 'anthropic' ? 'Anthropic' : 'OpenAI'}
                          </Badge>
                        </div>
                        <div class="mt-4 pt-3 border-t border-subtle/60 flex items-center justify-between">
                          <span class="text-xs text-faint font-mono">
                            {reg().id === 'custom-openai' ? t('providers.chatResponsesCompat') : t('providers.messagesCompat')}
                          </span>
                          <Button
                            size="sm"
                            variant={connected() ? 'secondary' : 'primary'}
                            onClick={() => openWizard(reg())}
                          >
                            {connected() ? t('providers.addNode') : t('providers.configureConnect')}
                          </Button>
                        </div>
                      </Card>
                    )
                  }}
                </For>
              </div>
            </div>
          </Show>

          {/* 官方认证主流供应商列表 */}
          <div class="space-y-3">
            <div class="flex items-center justify-between px-1">
              <span class="text-sm font-semibold text-foreground">{t('providers.officialProvidersTitle')}</span>
              <span class="text-xs text-faint">{t('providers.officialProvidersDesc')}</span>
            </div>
            <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4 pb-1">
              <For each={brandGroups()}>
            {group => {
              // 当前选中的变体（默认第一项）
              const reg = () => {
                const selectedId = selectedVariants()[group.brandKey]
                if (selectedId) {
                  const found = group.items.find(it => it.id === selectedId)
                  if (found) return found
                }
                return group.items[0]
              }
              const connected = () => store.providers().some(p => p.provider === reg().id)
              const hasVariants = () => group.items.length > 1

              return (
                <Card hover class="p-4 flex flex-col h-full justify-between group">
                  {/* 上半部分：品牌基础信息 + 变体切换 + 说明 */}
                  <div class="flex-1 flex flex-col">
                    <div class="flex items-start justify-between gap-2">
                      <div class="flex items-center gap-3 min-w-0">
                        <ProviderAvatar
                          provider={reg().id}
                          name={group.name}
                          color={reg().color}
                          size="md"
                        />
                        <div class="min-w-0">
                          <div class="font-semibold text-sm text-foreground truncate">
                            {group.name}
                          </div>
                          <div class="text-xs text-faint font-mono truncate">{reg().id}</div>
                        </div>
                      </div>

                      <Badge tone={reg().category === 'free' ? 'green' : 'blue' as BadgeTone} class="shrink-0">
                        {categoryLabel()[reg().category] || reg().category}
                      </Badge>
                    </div>

                    {/* 区域 / 渠道小标签切换器 (如 cn / intl) 或等高占位 */}
                    <div class="mt-3 min-h-8 flex items-center">
                      <Show when={hasVariants()} fallback={<div class="h-8" />}>
                        <div class="w-full flex items-center gap-1 p-1 rounded-lg bg-black/5 dark:bg-white/8 border border-black/10 dark:border-white/12">
                          <For each={group.items}>
                            {variant => {
                              const isSelected = () => reg().id === variant.id
                              const label = () => {
                                if (variant.region === 'cn') return t('providers.regionCn')
                                if (variant.region === 'intl') return t('providers.regionIntl')
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
                    </div>

                    {/* 能力胶囊徽章列表 */}
                    <div class="mt-2.5 flex flex-wrap items-center gap-1 min-h-5.5">
                      <For each={reg().capabilities || ['llm']}>
                        {capKey => {
                          const conf = getCapConfig(capKey)
                          const IconComponent = conf.icon
                          return (
                            <Badge tone={conf.tone} class="text-[10px] px-1.5 py-0.5 gap-1 shrink-0">
                              <IconComponent size={12} />
                              <span>{conf.label}</span>
                            </Badge>
                          )
                        }}
                      </For>
                    </div>
                    {/* 说明提示：固定最小高度保证网格卡片严格等高 */}
                    <div class="mt-2 min-h-5 flex items-center">
                      <Show when={reg().authHint} fallback={<span class="text-[11px] text-faint/60">{t('providers.officialStandardApi')}</span>}>
                        <span class="text-[11px] text-muted italic line-clamp-1">{reg().authHint}</span>
                      </Show>
                    </div>
                  </div>

                  {/* 下半部分：横向完全对齐的协议与优先级 */}
                  <div class="mt-3 pt-3 border-t border-subtle space-y-3">
                    <div class="flex items-center justify-between text-xs text-faint">
                      <span>{t('providers.protocol')}: <code class="font-mono text-foreground font-semibold">{reg().apiType || 'openai'}</code></span>
                      <span>{t('providers.defaultPriority')}: <span class="font-mono text-foreground font-medium">{reg().priority ?? 50}</span></span>
                    </div>

                    <div class="flex items-center justify-between gap-2 pt-0.5">
                      <Show
                        when={reg().apiKeyUrl || reg().website}
                        fallback={<span class="text-[11px] text-faint">{t('providers.nativeBuiltin')}</span>}
                      >
                        <a
                          href={reg().apiKeyUrl || reg().website}
                          target="_blank"
                          rel="noreferrer"
                          class="text-xs text-accent hover:underline inline-flex items-center gap-1 shrink-0"
                        >
                          {reg().apiKeyUrl ? t('providers.getKeyArrow') : t('providers.websiteArrow')}
                        </a>
                      </Show>

                      <div class="flex items-center gap-2">
                        <Show when={connected()}>
                          <span class="text-xs text-success font-semibold px-2 py-0.5 rounded-md bg-success/12 border border-success/30">{t('providers.alreadyConnected')}</span>
                        </Show>
                        <Button
                          size="sm"
                          variant={connected() ? 'secondary' : 'primary'}
                          onClick={() => openWizard(reg())}
                        >
                          {connected() ? t('providers.addAnother') : t('providers.connectConfig')}
                        </Button>
                      </div>
                    </div>
                  </div>
                </Card>
              )
            }}
            </For>
            </div>
          </div>
        </div>
      </Show>

      {/* 接入配置向导 Modal */}
      <Modal
        open={wizardOpen()}
        title={selectedReg() ? t('providers.wizardTitleConnect', { name: selectedReg()!.name }) : t('providers.wizardTitleAdd')}
        onClose={() => { cancelWizardOAuth(); setWizardOpen(false) }}
      >
        <Show when={selectedReg()}>
          {reg => {
            const authModes = () => {
              const r = reg()
              if (!r) return []
              const raw = r.authModes || [r.authType || 'api-key']
              return raw.map(m => m === 'apikey' ? 'api-key' : m).filter(m => m !== 'none')
            }

            return (
              <div class="space-y-4">
                <div class="p-3.5 rounded-xl bg-hover text-xs space-y-1.5 text-faint border border-subtle">
                  <div class="flex items-center justify-between">
                    <span>{t('providers.apiProtocol')}: <strong class="font-mono text-foreground">{reg().apiType || 'openai'}</strong></span>
                    <span>{t('providers.category')}: <strong class="text-foreground">{categoryLabel()[reg().category] || reg().category}</strong></span>
                  </div>
                  <Show when={reg().apiKeyUrl}>
                    <div>
                      {t('providers.getKeyLink')}
                      <a href={reg().apiKeyUrl} target="_blank" rel="noreferrer" class="text-accent underline font-mono ml-1">
                        {reg().apiKeyUrl}
                      </a>
                    </div>
                  </Show>
                  <Show when={reg().authHint}>
                    <div class="text-muted">{t('providers.hint')}: {reg().authHint}</div>
                  </Show>
                </div>

                <Field label={t('providers.connDisplayName')} hint={t('providers.connDisplayNameHint')}>
                  <Input
                    value={form().name}
                    placeholder={t('providers.connDisplayNamePlaceholder', { name: reg().name })}
                    onInput={v => setForm(f => ({ ...f, name: v }))}
                  />
                </Field>

                {/* 如果支持多种认证模式 */}
                <Show when={authModes().length > 1}>
                  <Field label={t('providers.authMethod')} hint={t('providers.authMethodHint')}>
                    <Select
                      value={form().authType === 'apikey' ? 'api-key' : form().authType}
                      options={authModes().map(m => {
                        const norm = m === 'apikey' ? 'api-key' : m
                        return { value: norm, label: authTypeLabel()[norm] || norm }
                      })}
                      onChange={v => setForm(f => ({ ...f, authType: v }))}
                    />
                  </Field>
                </Show>
                <Show when={form().authType === 'api-key' || form().authType === 'apikey'}>
                  <div class="space-y-2">
                    <Field label={t('providers.authCredential')} hint={reg().authHint || t('providers.credentialHint')}>
                      <div class="flex items-center gap-2">
                        <div class="flex-1">
                          <Input
                            type="password"
                            value={form().apiKey}
                            placeholder={reg().authHint?.includes('pt-') ? "pt-... (Personal Access Token)" : "sk-..."}
                            onInput={v => {
                              setForm(f => ({ ...f, apiKey: v }))
                              setTestedCreds(null)
                            }}
                          />
                        </div>
                        <Button
                          size="md"
                          variant="secondary"
                          loading={testingCreds()}
                          disabled={!form().apiKey.trim()}
                          onClick={handleTestWizardCreds}
                        >
                          {t('providerDetail.testConnection')}
                        </Button>
                      </div>
                    </Field>
                    <Show when={testedCreds()}>
                      {res => (
                        <div class={`text-xs px-3 py-1.5 rounded-control flex items-center justify-between ${
                          res().ok ? 'bg-success/10 text-success border border-success/20' : 'bg-danger/10 text-danger border border-danger/20'
                        }`}>
                          <span class="flex items-center gap-1.5">{res().ok ? <><IconCheck size={12} class="text-success" /><span>{res().msg}</span></> : <><IconClose size={12} class="text-danger" /><span>{res().msg}</span></>}</span>
                          <span class="text-[11px] opacity-75">{res().ok ? t('providers.credValidAllowSave') : t('providers.checkCredAndEndpoint')}</span>
                        </div>
                      )}
                    </Show>
                  </div>
                </Show>

                {/* OAuth 模式说明与直接授权发起 */}
                <Show when={form().authType === 'oauth'}>
                  <div class="space-y-3">
                    {/* 错误提示始终渲染在外层，杜绝静默吞错 */}
                    <Show when={wizardOAuthError()}>
                      <Alert
                        variant="danger"
                        title={t('providers.authProblemTitle')}
                        closable
                        onClose={() => setWizardOAuthError('')}
                      >
                        {wizardOAuthError()}
                      </Alert>
                    </Show>

                    {/* 令牌导入界面 (如 Cursor) */}
                    <Show when={wizardIsImport()}>
                      <div class="p-4 rounded-xl bg-bg-elevated border border-accent/40 shadow-glass-hover space-y-3">
                        <div class="text-xs font-semibold text-accent flex items-center gap-1.5">
                          <span>{t('providers.tokenImportMode')}</span>
                        </div>
                        <p class="text-xs text-faint leading-relaxed">
                          {t('providers.tokenImportHint')}
                        </p>
                        <div class="space-y-1.5">
                          <textarea
                            rows={3}
                            value={wizardImportToken()}
                            onInput={e => setWizardImportToken(e.currentTarget.value)}
                            placeholder={t('providers.tokenPastePlaceholder')}
                            class="w-full rounded-control border border-subtle bg-bg/80 px-3 py-2 text-xs font-mono focus-visible:outline-2 focus-visible:outline-ring"
                          />
                        </div>
                        <div class="flex items-center justify-end gap-2 pt-1">
                          <Button size="sm" variant="secondary" onClick={cancelWizardOAuth}>
                            {t('common.cancel')}
                          </Button>
                          <Button
                            size="sm"
                            variant="primary"
                            loading={wizardImporting()}
                            disabled={!wizardImportToken().trim()}
                            onClick={handleWizardImportSubmit}
                          >
                            {t('providers.importAndConnect')}
                          </Button>
                        </div>
                      </div>
                    </Show>

                    {/* 设备码 / 网页流卡片 */}
                    <Show when={!wizardIsImport()}>
                      <Show
                        when={wizardOAuthFlow()}
                        fallback={
                          <div class="p-4 rounded-xl border border-accent/30 bg-accent/10 text-xs text-foreground space-y-2.5 shadow-sm">
                            <div class="font-medium text-accent flex items-center gap-1.5">
                              <IconCheck size={14} /> {t('providers.oauthQuickModeSelected')}
                            </div>
                            <p class="text-faint leading-relaxed">
                              {t('providers.oauthNotice')}
                            </p>
                          </div>
                        }
                      >
                        {flow => (
                          <div class="p-4 rounded-xl bg-bg-elevated border border-accent/40 shadow-glass-hover space-y-3 text-center">
                            <div class="text-xs font-semibold text-accent">{t('providers.oauthInProgress')}</div>

                            {/* 有 UserCode 时展示验证码；无 UserCode 时展示网页跳转指引 */}
                            <Show
                              when={flow().userCode}
                              fallback={
                                <div class="space-y-2 py-1">
                                  <div class="text-xs text-faint">{t('providers.oauthBrowserOpened')}</div>
                                  <div class="flex items-center justify-center gap-2">
                                    <Button
                                      size="sm"
                                      variant="secondary"
                                      onClick={() => {
                                        const url = flow().verificationUriComplete || flow().verificationUri
                                        if (url) window.open(url, '_blank')
                                      }}
                                    >
                                      {t('providers.openAuthPage')}
                                    </Button>
                                  </div>
                                </div>
                              }
                            >
                              <div class="text-xs text-faint">{t('providers.oauthEnterCode')}</div>
                              <div class="flex items-center justify-center gap-2">
                                <span class="font-mono text-xl font-bold tracking-widest px-3 py-1 bg-accent/10 text-accent rounded border border-accent/30 select-all">
                                  {flow().userCode}
                                </span>
                                <Button
                                  size="sm"
                                  variant="secondary"
                                  onClick={() => {
                                    if (flow().userCode) {
                                      navigator.clipboard.writeText(flow().userCode!)
                                      setWizardOAuthCopied(true)
                                      setTimeout(() => setWizardOAuthCopied(false), 2000)
                                    }
                                  }}
                                >
                                  {wizardOAuthCopied() ? t('providers.copied') : t('providers.copyCode')}
                                </Button>
                              </div>
                            </Show>

                            <div class="text-[11px] text-faint break-all bg-bg/80 p-2 rounded border border-subtle">
                              {flow().verificationUriComplete || flow().verificationUri}
                            </div>
                            <div class="flex items-center justify-center gap-2 text-xs text-muted pt-1">
                              <span class="animate-spin inline-block w-3.5 h-3.5 border-2 border-accent border-t-transparent rounded-full" />
                              <span>{t('providers.oauthWaitingAuth')}</span>
                            </div>
                            <div class="pt-2 flex justify-center">
                              <Button size="sm" variant="secondary" onClick={cancelWizardOAuth}>
                                {t('providers.cancelAuth')}
                              </Button>
                            </div>
                          </div>
                        )}
                      </Show>
                    </Show>
                  </div>
                </Show>


                <Show when={reg().category === 'custom' || reg().id.startsWith('custom-')} fallback={
                  <Field label={t('providerDetail.priorityLabel')} hint={t('providers.priorityHint')}>
                    <Input
                      type="number"
                      value={form().priority}
                      onInput={v => setForm(f => ({ ...f, priority: v }))}
                    />
                  </Field>
                }>
                  <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
                    <Field label={t('providerDetail.priorityLabel')} hint={t('providers.priorityHint')}>
                      <Input
                        type="number"
                        value={form().priority}
                        onInput={v => setForm(f => ({ ...f, priority: v }))}
                      />
                    </Field>
                    <Field
                      label={t('providers.baseUrlRequired')}
                      hint={t('providers.baseUrlHint')}
                    >
                      <Input
                        value={form().baseUrl}
                        placeholder="https://api.example.com/v1"
                        onInput={v => setForm(f => ({ ...f, baseUrl: v }))}
                      />
                    </Field>
                  </div>
                </Show>
                <div class="pt-3 border-t border-subtle flex justify-end gap-2.5">
                  <Button variant="secondary" onClick={() => { cancelWizardOAuth(); setWizardOpen(false) }}>
                    {t('common.cancel')}
                  </Button>
                  <Button
                    variant="primary"
                    loading={saving() || wizardOAuthPolling()}
                    disabled={
                      (form().authType === 'api-key' || form().authType === 'apikey') && (!testedCreds() || !testedCreds()!.ok)
                    }
                    onClick={handleWizardSubmit}
                  >
                    {form().authType === 'oauth' ? (wizardOAuthPolling() ? t('providers.authorizing') : t('providers.startOAuth')) : t('providers.verifyAndSave')}
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
