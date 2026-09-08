import {type Component, createMemo, createSignal, For, onCleanup, onMount, Show} from 'solid-js'
import {A} from '@solidjs/router'
import {useGatewayStore} from '@/stores/gateway'
import {
  Badge,
  Button,
  Card,
  Empty,
  Field,
  Input,
  Modal,
  ProviderAvatar,
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

const CATEGORY_LABEL: Record<string, string> = {
  all: '全部类别',
  apikey: 'API Key',
  oauth: 'OAuth 授权',
  freeTier: '免费额度',
  free: '免密免费',
  webCookie: '网页 Cookie',
  custom: '自定义通用',
  media: '多模态与媒体',
}

const AUTHTYPE_LABEL: Record<string, string> = {
  'api-key': 'API Key',
  apikey: 'API Key',
  oauth: 'OAuth 授权',
  none: '免认证',
  cookie: 'Cookie',
}

export const CAPABILITY_CONFIG: Record<string, { label: string; tone: BadgeTone; icon: Component<{ size?: number; class?: string }> }> = {
  llm: { label: 'LLM 对话', tone: 'blue', icon: IconChat },
  image: { label: '图像生成', tone: 'blue', icon: IconPalette },
  tts: { label: '语音合成 (TTS)', tone: 'green', icon: IconVolume },
  stt: { label: '语音识别 (STT)', tone: 'amber', icon: IconMic },
  video: { label: '视频生成', tone: 'red', icon: IconVideo },
  embedding: { label: '文本向量', tone: 'gray', icon: IconVector },
  'web-search': { label: '网络搜索', tone: 'amber', icon: IconSearch },
  'web-fetch': { label: '网页抓取', tone: 'gray', icon: IconGlobe },
}

const Providers: Component = () => {
  const store = useGatewayStore()
  const toast = useToast()

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
    const isOpencode = reg.id === 'opencode'
    const defaultAuth = isOpencode ? 'none' : (reg.authType === 'oauth' ? 'oauth' : 'api-key')
    setForm({
      name: reg.name,
      authType: defaultAuth,
      apiKey: '',
      baseUrl: reg.baseUrl || '',
      priority: String(reg.priority ?? 50),
    })
    setWizardOpen(true)
  }

  // 一键接入免密提供商（仅 OpenCode 原生支持完全免密免鉴权调用公共模型）
  async function quickEnableFree(reg: RegistryProvider) {
    if (reg.id !== 'opencode') {
      openWizard(reg)
      return
    }
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
      toast.success(`已启用免密提供商：${reg.name}`)
      await store.loadProvidersOnly()
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : String(e)
      toast.error(`启用失败: ${msg}`)
      console.error('[providers] quick enable failed:', e)
    } finally {
      setSaving(false)
    }
  }

  // 向导中测试 API Key / 凭据有效性
  async function handleTestWizardCreds() {
    const reg = selectedReg()
    if (!reg) return
    const f = form()
    const isCustom = reg.category === 'custom' || reg.id.startsWith('custom-')
    if (isCustom && !f.baseUrl.trim()) {
      toast.error('通用自定义接口必须填写有效的 Base URL 端点')
      return
    }
    if (!f.apiKey.trim()) {
      toast.error('请先输入要验证的 API Key / 访问凭证')
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
        setTestedCreds({ ok: true, msg: `验证通过 (${res.latency || '正常'})` })
        toast.success(`凭据测试成功！延时: ${res.latency || '正常'}`)
      } else {
        setTestedCreds({ ok: false, msg: res.error || '验证失败，请检查密钥或端点' })
        toast.error(`验证失败: ${res.error || '上游响应错误'}`)
      }
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : String(e)
      setTestedCreds({ ok: false, msg })
      toast.error(`测试连接异常: ${msg}`)
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
          toast.info(`已在新窗口打开 ${reg.name} 授权页面，完成授权后网关将自动绑定。`)
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
              toast.success(`${reg.name} 网页授权成功！连接已自动建立。`)
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
      const res = (await apiPost(`/api/oauth/${reg.id}/device-code`)) as {
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
          })) as { success?: boolean; error?: string; pending?: boolean; connection?: Provider }
          if (pollRes?.success) {
            clearInterval(wizardPollTimer)
            wizardPollTimer = undefined
            setWizardOAuthPolling(false)
            setWizardOAuthFlow(null)
            toast.success(`${reg.name} OAuth 授权成功！连接已自动建立。`)
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
      toast.error(`发起授权失败: ${msg}`)
    }
  }

  // 提交 Token 导入
  async function handleWizardImportSubmit() {
    const reg = selectedReg()
    if (!reg) return
    const token = wizardImportToken().trim()
    if (!token) {
      toast.error('请输入访问令牌 (Access Token)')
      return
    }
    setWizardImporting(true)
    try {
      await apiPost(`/api/oauth/${reg.id}/import`, {
        accessToken: token,
        name: form().name || reg.name,
      })
      toast.success(`${reg.name} 令牌导入成功！已保存连接。`)
      cancelWizardOAuth()
      setWizardOpen(false)
      setActiveTab('connections')
      await store.loadProvidersOnly()
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : String(e)
      setWizardOAuthError(msg)
      toast.error(`导入失败: ${msg}`)
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

    const isOpencode = reg.id === 'opencode'
    if (normAuth === 'none') {
      if (!isOpencode) {
        toast.error('该提供商不支持免密模式')
        return
      }
    } else if (normAuth === 'api-key') {
      const isCustom = reg.category === 'custom' || reg.id.startsWith('custom-')
      if (isCustom && !f.baseUrl.trim()) {
        toast.error('通用自定义接口必须填写有效的 Base URL 端点')
        return
      }
      if (!f.apiKey.trim()) {
        toast.error('请填写 API Key')
        return
      }
      // 必须通过凭证测试才允许保存
      if (!testedCreds() || !testedCreds()!.ok) {
        toast.error('请先点击右侧「测试连接」验证凭据可用性，验证通过后方可接入')
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
    <div class="space-y-5 stagger">
      {/* 头部标题与视窗切换：悬浮圆角毛玻璃卡片 */}
      <div class="sticky top-[4.75rem] z-20 rounded-2xl glass-card p-4 sm:p-5 space-y-3 shadow-glass transition-all">
        <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
          <div>
            <h1 class="text-xl font-semibold">模型提供商接入</h1>
            <p class="text-sm text-faint mt-0.5">
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
              class="w-64!"
              placeholder={activeTab() === 'connections' ? '搜索已连接提供商…' : '搜索提供商市场/模型…'}
              value={query()}
              onInput={setQuery}
            />

            <Select
              value={capFilter()}
              options={[
                { value: '', label: '全部支持能力' },
                { value: 'llm', label: 'LLM 对话' },
                { value: 'image', label: '图像生成' },
                { value: 'tts', label: '语音合成 (TTS)' },
                { value: 'stt', label: '语音识别 (STT)' },
                { value: 'video', label: '视频生成' },
                { value: 'embedding', label: '文本向量' },
                { value: 'web-search', label: '网络搜索' },
                { value: 'web-fetch', label: '网页抓取' },
              ]}
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
                  { value: 'custom', label: '自定义通用 API' },
                  { value: 'free', label: '免密免费' },
                  { value: 'freeTier', label: '免费额度' },
                  { value: 'apikey', label: 'API Key' },
                  { value: 'media', label: '多模态与媒体 (Media)' },
                  { value: 'oauth', label: 'OAuth 渠道' },
                ]}
                onChange={setCatFilter}
              />
              <button
                type="button"
                onClick={() => setHideAdded(!hideAdded())}
                class={`text-xs px-2.5 py-1.5 rounded-control border transition-all flex items-center gap-1.5 cursor-pointer ${
                  hideAdded()
                    ? 'bg-accent/10 border-accent/40 text-accent font-medium'
                    : 'bg-hover border-subtle text-muted hover:text-foreground'
                }`}
                title={hideAdded() ? '点击显示所有提供商（含已接入）' : '点击只看尚未接入的提供商'}
              >
                <span class="flex items-center gap-1.5">{hideAdded() ? <><IconCheck size={12} /><span>已隐藏已接入</span></> : '显示全部市场'}</span>
                <Show when={connectedCount() > 0}>
                  <span class="text-[10px] opacity-75">
                    ({hideAdded() ? `已藏 ${connectedCount()}` : `${connectedCount()} 已接入`})
                  </span>
                </Show>
              </button>
            </Show>
          </div>

          <div class="flex items-center gap-3 text-xs text-faint">
            <span class="hidden sm:inline">
              匹配 <strong class="text-foreground font-mono">{activeTab() === 'connections' ? groupedConnections().length : brandGroups().length}</strong> 个{activeTab() === 'connections' ? '供应商' : '项目'}
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
          when={groupedConnections().length > 0}
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
          <div class="grid gap-4">
            <For each={groupedConnections()}>
              {group => {
                const reg = () => registryFor(group.providerId)
                const hasMultiple = () => group.connections.length > 1
                const allActive = () => group.activeCount === group.connections.length
                const noneActive = () => group.activeCount === 0

                return (
                  <Card hover class="p-4 group border border-subtle/80 bg-bg-elevated/70 shadow-sm transition-all hover:border-accent/40">
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
                              {allActive() ? '全部启用' : noneActive() ? '全部停用' : `部分启用 (${group.activeCount}/${group.connections.length})`}
                            </Badge>
                            <Badge tone="blue" class="text-[10px] px-1.5 py-0.5 shrink-0">
                              {group.connections.length} 个账号
                            </Badge>
                            <Show when={hasMultiple()}>
                              <Badge tone="blue" class="bg-purple-500/15 text-purple-400 border border-purple-500/30 text-[10px] px-1.5 py-0.5 shrink-0">
                                Fallback 就绪
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
                                  const conf = CAPABILITY_CONFIG[capKey] || { label: capKey, tone: 'gray' as BadgeTone, icon: IconZap }
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
                            title="为此供应商添加备用账号（支持 API Key 或 OAuth）"
                          >
                            + 加账号
                          </Button>
                        </Show>
                        <A href={`/providers/${group.primaryConnectionId}`}>
                          <Button size="sm" variant="primary">
                            管理 →
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
                  <span class="text-sm font-semibold text-foreground">自定义通用接口 (Compatible APIs)</span>
                  <Badge tone="blue" class="text-[10px]">支持自定义 Base URL</Badge>
                </div>
                <span class="text-xs text-faint">接入私有化部署、开源代理或三方标准中转服务</span>
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
                                <span class="text-[10px] font-mono px-1.5 py-0.5 rounded bg-bg text-faint border border-subtle">
                                  {reg().id}
                                </span>
                              </div>
                              <div class="text-xs text-faint mt-1 line-clamp-2">
                                {reg().authHint || "标准协议兼容接入，需填写 Base URL 与对应 API Key"}
                              </div>
                            </div>
                          </div>
                          <Badge tone="blue" class="shrink-0">
                            {reg().apiType === 'anthropic' ? 'Anthropic' : 'OpenAI'}
                          </Badge>
                        </div>
                        <div class="mt-4 pt-3 border-t border-subtle/60 flex items-center justify-between">
                          <span class="text-xs text-faint font-mono">
                            {reg().id === 'custom-openai' ? 'Chat / Responses 兼容' : 'Messages 兼容'}
                          </span>
                          <Button
                            size="sm"
                            variant={connected() ? 'secondary' : 'primary'}
                            onClick={() => openWizard(reg())}
                          >
                            {connected() ? '+ 加节点' : '配置接入 →'}
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
              <span class="text-sm font-semibold text-foreground">认证提供商服务 (Official Providers)</span>
              <span class="text-xs text-faint">开箱即用官方路由，无需手动维护端点</span>
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
              const isFree = () => reg().id === 'opencode' && reg().noAuth
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

                      <Badge tone={isFree() ? 'green' : 'blue' as BadgeTone} class="shrink-0">
                        {CATEGORY_LABEL[reg().category] || reg().category}
                      </Badge>
                    </div>

                    {/* 区域 / 渠道小标签切换器 (如 cn / intl) 或等高占位 */}
                    <div class="mt-3 min-h-8 flex items-center">
                      <Show when={hasVariants()} fallback={<div class="h-8" />}>
                        <div class="w-full flex items-center gap-1 p-1 bg-hover rounded-lg border border-subtle">
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
                    </div>

                    {/* 能力胶囊徽章列表 */}
                    <div class="mt-2.5 flex flex-wrap items-center gap-1 min-h-5.5">
                      <For each={reg().capabilities || ['llm']}>
                        {capKey => {
                          const conf = CAPABILITY_CONFIG[capKey] || { label: capKey, tone: 'gray' as BadgeTone, icon: IconZap }
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
                      <Show when={reg().authHint} fallback={<span class="text-[11px] text-faint/60">官方标准接口</span>}>
                        <span class="text-[11px] text-muted italic line-clamp-1">{reg().authHint}</span>
                      </Show>
                    </div>
                  </div>

                  {/* 下半部分：横向完全对齐的协议与优先级 */}
                  <div class="mt-3 pt-3 border-t border-subtle space-y-3">
                    <div class="flex items-center justify-between text-xs text-faint">
                      <span>协议: <code class="font-mono text-foreground font-semibold">{reg().apiType || 'openai'}</code></span>
                      <span>默认优先级: <span class="font-mono text-foreground font-medium">{reg().priority ?? 50}</span></span>
                    </div>

                    <div class="flex items-center justify-between gap-2 pt-0.5">
                      <Show
                        when={reg().apiKeyUrl || reg().website}
                        fallback={<span class="text-[11px] text-faint">原生内置</span>}
                      >
                        <a
                          href={reg().apiKeyUrl || reg().website}
                          target="_blank"
                          rel="noreferrer"
                          class="text-xs text-accent hover:underline inline-flex items-center gap-1 shrink-0"
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
        title={selectedReg() ? `接入提供商：${selectedReg()!.name}` : '添加提供商'}
        onClose={() => { cancelWizardOAuth(); setWizardOpen(false) }}
      >
        <Show when={selectedReg()}>
          {reg => {
            const authModes = () => {
              const r = reg()
              if (!r) return []
              if (r.id === 'opencode') {
                return ['none', 'api-key']
              }
              const raw = r.authModes || [r.authType || 'api-key']
              return raw.map(m => m === 'apikey' ? 'api-key' : m).filter(m => m !== 'none')
            }
            const isFree = () => reg().id === 'opencode' && form().authType === 'none'

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
                <Show when={authModes().length > 1}>
                  <Field label="认证方式" hint="该上游支持多种鉴权模式">
                    <Select
                      value={form().authType === 'apikey' ? 'api-key' : form().authType}
                      options={authModes().map(m => {
                        const norm = m === 'apikey' ? 'api-key' : m
                        return { value: norm, label: AUTHTYPE_LABEL[norm] || norm }
                      })}
                      onChange={v => setForm(f => ({ ...f, authType: v }))}
                    />
                  </Field>
                </Show>
                <Show when={form().authType === 'api-key' || form().authType === 'apikey'}>
                  <div class="space-y-2">
                    <Field label="API Key / 访问凭据" hint={reg().authHint || "凭证安全存储于服务端并进行掩码处理"}>
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
                          测试连接
                        </Button>
                      </div>
                    </Field>
                    <Show when={testedCreds()}>
                      {res => (
                        <div class={`text-xs px-3 py-1.5 rounded-control flex items-center justify-between ${
                          res().ok ? 'bg-success/10 text-success border border-success/20' : 'bg-danger/10 text-danger border border-danger/20'
                        }`}>
                          <span class="flex items-center gap-1.5">{res().ok ? <><IconCheck size={12} class="text-success" /><span>{res().msg}</span></> : <><IconClose size={12} class="text-danger" /><span>{res().msg}</span></>}</span>
                          <span class="text-[11px] opacity-75">{res().ok ? '凭据有效，允许保存' : '请核对凭证与网络端点'}</span>
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
                        title="授权遇到问题"
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
                          <span>Token 导入授权模式</span>
                        </div>
                        <p class="text-xs text-faint leading-relaxed">
                          该提供商不支持网页跳转或设备码直接登录，请填入从客户端或环境获取的访问凭据 (Access Token / Session Token)：
                        </p>
                        <div class="space-y-1.5">
                          <textarea
                            rows={3}
                            value={wizardImportToken()}
                            onInput={e => setWizardImportToken(e.currentTarget.value)}
                            placeholder="粘贴从本地配置或数据库提取的 Access Token / JWT..."
                            class="w-full rounded-control border border-subtle bg-bg/80 px-3 py-2 text-xs font-mono focus-visible:outline-2 focus-visible:outline-ring"
                          />
                        </div>
                        <div class="flex items-center justify-end gap-2 pt-1">
                          <Button size="sm" variant="secondary" onClick={cancelWizardOAuth}>
                            取消
                          </Button>
                          <Button
                            size="sm"
                            variant="primary"
                            loading={wizardImporting()}
                            disabled={!wizardImportToken().trim()}
                            onClick={handleWizardImportSubmit}
                          >
                            导入并连接
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
                              <IconCheck size={14} /> 已选择 OAuth 快捷授权模式
                            </div>
                            <p class="text-faint leading-relaxed">
                              点击下方「发起 OAuth 授权」后将呼出浏览器授权页面或设备码，<strong>在第三方平台成功授权通过后才保存连接</strong>，免去繁琐的密钥配置。
                            </p>
                          </div>
                        }
                      >
                        {flow => (
                          <div class="p-4 rounded-xl bg-bg-elevated border border-accent/40 shadow-glass-hover space-y-3 text-center">
                            <div class="text-xs font-semibold text-accent">正在进行 OAuth 快捷授权</div>

                            {/* 有 UserCode 时展示验证码；无 UserCode 时展示网页跳转指引 */}
                            <Show
                              when={flow().userCode}
                              fallback={
                                <div class="space-y-2 py-1">
                                  <div class="text-xs text-faint">已在新窗口中打开授权网页，请在网页中完成登录并确认授权：</div>
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
                              <div class="text-xs text-faint">请在新打开的页面中输入下方验证码完成授权：</div>
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
                                  {wizardOAuthCopied() ? '已复制' : '复制码'}
                                </Button>
                              </div>
                            </Show>

                            <div class="text-[11px] text-faint break-all bg-bg/80 p-2 rounded border border-subtle">
                              {flow().verificationUriComplete || flow().verificationUri}
                            </div>
                            <div class="flex items-center justify-center gap-2 text-xs text-muted pt-1">
                              <span class="animate-spin inline-block w-3.5 h-3.5 border-2 border-accent border-t-transparent rounded-full" />
                              <span>等待浏览器授权完成... 授权成功将自动保存</span>
                            </div>
                            <div class="pt-2 flex justify-center">
                              <Button size="sm" variant="secondary" onClick={cancelWizardOAuth}>
                                取消当前授权
                              </Button>
                            </div>
                          </div>
                        )}
                      </Show>
                    </Show>
                  </div>
                </Show>

                {/* 免密模式说明 */}
                <Show when={form().authType === 'none'}>
                  <div class="p-3.5 rounded-xl border border-emerald-500/30 bg-emerald-500/10 text-xs text-foreground space-y-1 shadow-sm">
                    <div class="font-medium text-emerald-400 flex items-center gap-1.5"><IconCheck size={14} /> 免密体验模式</div>
                    <p class="text-faint">该上游无需任何密钥凭证，保存后即可开箱即用体验免费公共模型。</p>
                  </div>
                </Show>

                <Show when={reg().category === 'custom' || reg().id.startsWith('custom-')} fallback={
                  <Field label="调度优先级" hint="数值越小越优先调度">
                    <Input
                      type="number"
                      value={form().priority}
                      onInput={v => setForm(f => ({ ...f, priority: v }))}
                    />
                  </Field>
                }>
                  <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
                    <Field label="调度优先级" hint="数值越小越优先调度">
                      <Input
                        type="number"
                        value={form().priority}
                        onInput={v => setForm(f => ({ ...f, priority: v }))}
                      />
                    </Field>
                    <Field
                      label="Base URL (必填)"
                      hint="标准端点地址，如 https://api.my-host.com/v1"
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
                    取消
                  </Button>
                  <Button
                    variant="primary"
                    loading={saving() || wizardOAuthPolling()}
                    disabled={
                      (form().authType === 'api-key' || form().authType === 'apikey') && (!testedCreds() || !testedCreds()!.ok)
                    }
                    onClick={handleWizardSubmit}
                  >
                    {isFree() ? '立即接入' : form().authType === 'oauth' ? (wizardOAuthPolling() ? '正在授权中...' : '发起 OAuth 授权 ↗') : '验证并通过后保存'}
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
