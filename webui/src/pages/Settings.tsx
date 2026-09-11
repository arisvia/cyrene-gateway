import { type Component, For, Show, createSignal, onMount } from 'solid-js'
import { useGatewayStore } from '@/stores/gateway'
import { useBackgroundStore } from '@/stores/background'
import {
  Card, Badge, Button, Input, Select, Toggle, Field, confirm, PageHeader, SegmentedControl, Checkbox,
  IconLock, IconKey, IconShield, IconZap, IconSparkles, IconPalette, IconInfo,
  IconDatabase, IconDownload, IconUpload, IconAlertTriangle,
} from '@/components/ui'
import { formatUptime, formatVersion } from '@/lib/format'
import { useToast } from '@/lib/toast'
import { api, apiPost } from '@/lib/api'

const cavemanOptions = [
  { value: 'lite', label: '精简 (lite)' },
  { value: 'full', label: '标准极简 (full)' },
  { value: 'ultra', label: '极致极简 (ultra)' },
  { value: 'wenyan-lite', label: '半文言 (wenyan-lite)' },
  { value: 'wenyan', label: '文言文 (wenyan)' },
  { value: 'wenyan-ultra', label: '极限文言 (wenyan-ultra)' },
]

const ponytailOptions = [
  { value: 'lite', label: '精简建议 (lite)' },
  { value: 'full', label: '阶梯原则 (full)' },
  { value: 'ultra', label: '极致极简 (ultra)' },
]

interface CacheStats {
  hits: number
  misses: number
  hitRate: number
  entries: number
  maxEntries: number
  bytesUsed: number
  tokensSaved: number
}

function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(2)} MB`
}

const Settings: Component = () => {
  const store = useGatewayStore()
  const bgStore = useBackgroundStore()
  const toast = useToast()
  const [saving, setSaving] = createSignal(false)
  const [local, setLocal] = createSignal<Record<string, unknown>>({})
  const [pw, setPw] = createSignal('')
  const [hasPw, setHasPw] = createSignal(false)

  // 背景自定义状态
  const [bgUrlInput, setBgUrlInput] = createSignal(bgStore.config().remoteUrl || '')
  const [loadingBgUrl, setLoadingBgUrl] = createSignal(false)
  // 响应缓存状态
  const [cacheStats, setCacheStats] = createSignal<CacheStats | null>(null)
  const [clearingCache, setClearingCache] = createSignal(false)
  const [refreshingStats, setRefreshingStats] = createSignal(false)

  // TokenSaver 排除项状态
  // 设置分类 Tab
  const [activeTab, setActiveTab] = createSignal<'gateway' | 'appearance' | 'data'>('gateway')
  function handleTabChange(tab: 'gateway' | 'appearance' | 'data') {
    setActiveTab(tab)
    if (typeof window !== 'undefined') {
      window.scrollTo({ top: 0, behavior: 'smooth' })
    }
  }

  // 数据备份与导出状态
  const [includeSecrets, setIncludeSecrets] = createSignal(true)
  const [includeUsage, setIncludeUsage] = createSignal(false)
  const [downloading, setDownloading] = createSignal(false)

  // 数据恢复状态
  const [restoreMode, setRestoreMode] = createSignal<'replace' | 'merge'>('replace')
  const [restoreFile, setRestoreFile] = createSignal<File | null>(null)
  const [restoring, setRestoring] = createSignal(false)

  async function handleDownloadBackup() {
    setDownloading(true)
    try {
      const url = `/api/system/backup?include_secrets=${includeSecrets()}&include_usage=${includeUsage()}`
      const res = await fetch(url)
      if (!res.ok) throw new Error(`HTTP ${res.status}`)
      const blob = await res.blob()
      const a = document.createElement('a')
      a.href = URL.createObjectURL(blob)
      const dateStr = new Date().toISOString().slice(0, 10)
      a.download = `cyrene-backup-${dateStr}${includeSecrets() ? '' : '-sanitized'}.cyrene.json`
      document.body.appendChild(a)
      a.click()
      document.body.removeChild(a)
      URL.revokeObjectURL(a.href)
      toast.success('备份快照已成功导出')
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : '导出失败'
      toast.error(`备份导出失败: ${msg}`)
    } finally {
      setDownloading(false)
    }
  }

  async function handleRestoreSubmit() {
    const file = restoreFile()
    if (!file) {
      toast.warning('请先选择要导入的备份文件')
      return
    }
    const modeText = restoreMode() === 'replace'
      ? '全量覆盖（清空现有用户配置并完整恢复快照）'
      : '增量合并（保留现有配置，仅对快照包含条目进行覆盖/更新）'

    const ok = await confirm({
      title: '确认恢复网关数据？',
      message: `即将使用备份文件「${file.name}」以【${modeText}】模式重载数据库。该操作不可撤销，确定要执行吗？`,
      variant: 'danger',
    })
    if (!ok) return

    setRestoring(true)
    try {
      const res = await fetch(`/api/system/restore?mode=${restoreMode()}`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: file,
      })
      if (!res.ok) {
        const errJson = await res.json().catch(() => ({}))
        throw new Error(errJson.error || `HTTP ${res.status}`)
      }
      toast.success('数据库已成功恢复并重新装载！')
      setRestoreFile(null)
      await store.loadCore()
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : '恢复失败'
      toast.error(`数据恢复失败: ${msg}`)
    } finally {
      setRestoring(false)
    }
  }
  const [excludeInput, setExcludeInput] = createSignal('')
  const excludedProviders = () => {
    const list = local().tokenSaverExclude
    return Array.isArray(list) ? (list as string[]) : []
  }
  const addExcludeProvider = (providerName: string) => {
    const trimmed = providerName.trim()
    if (!trimmed) return
    const current = excludedProviders()
    if (!current.includes(trimmed)) {
      set('tokenSaverExclude', [...current, trimmed])
    }
    setExcludeInput('')
  }
  const removeExcludeProvider = (providerName: string) => {
    const current = excludedProviders()
    set('tokenSaverExclude', current.filter(p => p !== providerName))
  }
  const quickSuggestions = () => {
    const configured = store.providers().map(p => p.provider)
    const defaults = ['deepseek', 'openai', 'anthropic', 'gemini']
    const merged = Array.from(new Set([...defaults, ...configured]))
    const current = excludedProviders()
    return merged.filter(p => !current.includes(p))
  }

  async function fetchCacheStats() {
    try {
      const res = await api<CacheStats>('/api/cache/stats')
      setCacheStats(res)
    } catch {
      // ignore
    }
  }

  async function handleRefreshStats() {
    setRefreshingStats(true)
    try {
      await fetchCacheStats()
      toast.success('缓存统计已刷新')
    } finally {
      setRefreshingStats(false)
    }
  }

  async function handleClearCache() {
    const ok = await confirm({
      title: '清空响应缓存',
      message: '确定清空当前内存中的所有响应缓存条目吗？后续相同请求将重新向上游发起。',
      variant: 'danger',
    })
    if (!ok) return
    setClearingCache(true)
    try {
      await apiPost('/api/cache/clear')
      toast.success('响应缓存已清空')
      await fetchCacheStats()
    } catch (e: unknown) {
      toast.error(e instanceof Error ? e.message : '清空缓存失败')
    } finally {
      setClearingCache(false)
    }
  }

  onMount(async () => {
    await store.loadSettings()
    if (store.providers().length === 0) {
      void store.loadProvidersOnly()
    }
    setLocal({ ...store.settings() })
    setHasPw(!!store.settings().hasPassword)
    if (bgStore.config().sourceType === 'remote' && bgStore.config().remoteUrl) {
      setBgUrlInput(bgStore.config().remoteUrl || '')
    }
    await fetchCacheStats()
  })

  const dirty = () => {
    const orig = store.settings()
    const ignored = new Set(['hasPassword', 'passwordHash'])
    const keys = new Set([...Object.keys(local()), ...Object.keys(orig)])
    for (const k of keys) {
      if (ignored.has(k)) continue
      const lv = local()[k]
      const ov = orig[k]
      if (Array.isArray(lv) || Array.isArray(ov)) {
        if (JSON.stringify(lv ?? []) !== JSON.stringify(ov ?? [])) return true
      } else if (lv !== ov) {
        return true
      }
    }
    return false
  }

  const set = (k: string, v: unknown) => setLocal(l => ({ ...l, [k]: v }))

  async function save() {
    setSaving(true)
    try {
      const payload = { ...local() }
      delete payload.hasPassword
      delete payload.passwordHash
      await store.saveSettings(payload)
      setLocal({ ...store.settings() })
      setHasPw(!!store.settings().hasPassword)
    } catch (e: unknown) {
      toast.error(e instanceof Error ? e.message : '保存设置失败')
    } finally {
      setSaving(false)
    }
  }

  async function changePassword() {
    if (pw().length < 8) {
      toast.warning('新密码长度至少需要 8 位')
      return
    }
    try {
      await store.setPassword(pw())
      setPw('')
      setHasPw(true)
    } catch (e: unknown) {
      toast.error(e instanceof Error ? e.message : '密码更新失败')
    }
  }

  return (
    <div class="max-w-3xl mx-auto space-y-6 stagger">
      <PageHeader
        title="系统设置"
        subtitle="网关运行参数、访问控制与效能引擎"
        actions={
          <Show when={activeTab() === 'gateway'}>
            <Button variant="primary" loading={saving()} disabled={!dirty()} onClick={save} class="shrink-0">
              {dirty() ? '保存修改' : '全部已保存'}
            </Button>
          </Show>
        }
      >
        <div class="pt-2 border-t border-subtle/40 flex flex-col sm:flex-row sm:items-center justify-between gap-2.5">
          <SegmentedControl
            options={[
              { value: 'gateway', label: '网关核心 (Gateway)' },
              { value: 'appearance', label: '界面与外观 (Appearance)' },
              { value: 'data', label: '数据管理与备份 (Data & Backup)' },
            ]}
            value={activeTab()}
            onChange={handleTabChange}
          />
          <div class="text-[11px] text-faint flex items-center gap-1.5 px-0.5 whitespace-nowrap">
            <Show when={activeTab() === 'gateway'}>
              <span class="w-1.5 h-1.5 rounded-full bg-accent animate-pulse shrink-0" />
              <span>存储宿主：<span class="font-semibold text-foreground">SQLite 核心库</span> · 全局实时生效</span>
            </Show>
            <Show when={activeTab() === 'appearance'}>
              <span class="w-1.5 h-1.5 rounded-full bg-amber-400 shrink-0" />
              <span>存储宿主：<span class="font-semibold text-foreground">本地浏览器</span> · 本设备独享</span>
            </Show>
            <Show when={activeTab() === 'data'}>
              <span class="w-1.5 h-1.5 rounded-full bg-emerald-400 shrink-0" />
              <span>存储宿主：<span class="font-semibold text-foreground">SQLite 核心库</span> · 支持单事务热恢复</span>
            </Show>
          </div>
        </div>
      </PageHeader>

      {/* ── Tab 1：网关核心设置 ── */}
      <Show when={activeTab() === 'gateway'}>

      {/* ── 分组 1：安全与访问控制 ── */}
      <div class="space-y-3.5">
        <div class="flex items-center gap-1.5 px-0.5 text-[11px] font-semibold uppercase tracking-wider text-faint">
          <IconShield size={14} class="text-accent shrink-0" />
          <span>安全与访问控制</span>
        </div>

        {/* 访问控制卡片 */}
        <Card class="p-5 space-y-4">
          <div class="flex items-center justify-between border-b border-subtle/50 pb-3">
            <div class="flex items-center gap-2">
              <IconLock size={16} class="text-accent shrink-0" />
              <h3 class="text-sm font-semibold">访问控制</h3>
            </div>
            <Badge tone={local().requireLogin || local().requireApiKey ? 'blue' : 'gray'}>
              {local().requireLogin || local().requireApiKey ? '已启用防护' : '公开直通'}
            </Badge>
          </div>

          <div class="flex items-start justify-between gap-4">
            <Field
              label="要求登录"
              hint={
                hasPw()
                  ? '开启后管理面板需密码登录（远程非本机访问默认强制要求）'
                  : '请先在下方设置管理密码再开启要求登录，以防面板被永久锁死'
              }
            >
              <span />
            </Field>
            <Toggle
              checked={!!local().requireLogin}
              disabled={!hasPw()}
              onChange={v => {
                if (v && !hasPw()) {
                  toast.warning('请先在下方设置管理员密码再开启要求登录')
                  return
                }
                set('requireLogin', v)
              }}
            />
          </div>

          <div class="flex items-start justify-between gap-4 pt-1 border-t border-subtle/50">
            <Field label="要求 API Key" hint="开启后所有 /v1/* 接口调用均必须在请求头中携带有效密钥 (Bearer cg-...)">
              <span />
            </Field>
            <Toggle checked={!!local().requireApiKey} onChange={v => set('requireApiKey', v)} />
          </div>

          <div class="pt-1 border-t border-subtle/50">
            <Field label="API Key 速率限制 (RPM)" hint="单个 Key 每分钟请求上限，超出返回 429；填 0 表示不限速">
              <Input
                type="number"
                class="!w-full sm:!w-36 mt-1"
                value={String(local().apiKeyRpm ?? 0)}
                onInput={v => set('apiKeyRpm', Number(v) || 0)}
              />
            </Field>
          </div>
        </Card>

        {/* 管理密码卡片 */}
        <Card class="p-5 space-y-4">
          <div class="flex items-center justify-between border-b border-subtle/50 pb-3">
            <div class="flex items-center gap-2">
              <IconKey size={16} class="text-accent shrink-0" />
              <h3 class="text-sm font-semibold">管理密码</h3>
            </div>
            <Badge tone={hasPw() ? 'green' : 'amber'}>
              {hasPw() ? '已设置密码' : '未初始化密码'}
            </Badge>
          </div>
          <Field label="更新管理员密码" hint="长度至少 8 位，密码通过 Argon2id 散列存储，保障管理接口防护">
            <div class="flex flex-col sm:flex-row items-stretch sm:items-center gap-2.5 pt-1">
              <Input
                type="password"
                value={pw()}
                onInput={setPw}
                placeholder="输入新管理密码..."
                class="flex-1 !w-full"
              />
              <Button
                variant="secondary"
                disabled={pw().length < 8}
                onClick={changePassword}
                class="shrink-0"
              >
                更新密码
              </Button>
            </div>
          </Field>
        </Card>
      </div>

      {/* ── 分组 2：效能与节省引擎 ── */}
      <div class="space-y-3.5">
        <div class="flex items-center gap-1.5 px-0.5 pt-2 text-[11px] font-semibold uppercase tracking-wider text-faint">
          <IconZap size={14} class="text-accent shrink-0" />
          <span>效能与节省引擎</span>
        </div>

        {/* 响应精确缓存卡片 */}
        <Card class="p-5 space-y-4">
          <div class="flex items-center justify-between border-b border-subtle/50 pb-3">
            <div class="flex items-center gap-2">
              <IconZap size={16} class="text-accent shrink-0" />
              <h3 class="text-sm font-semibold">响应精确缓存</h3>
              <Badge tone="blue">1ms 直出 · 0 Token</Badge>
            </div>
            <div class="flex items-center gap-1.5">
              <Button
                size="sm"
                variant="ghost"
                loading={refreshingStats()}
                onClick={handleRefreshStats}
                title="刷新统计指标"
              >
                刷新
              </Button>
              <Button
                size="sm"
                variant="ghost"
                class="text-danger hover:text-danger hover:bg-danger/10"
                loading={clearingCache()}
                onClick={handleClearCache}
                title="清空所有内存缓存条目"
              >
                清空缓存
              </Button>
            </div>
          </div>

          {/* 缓存指标数据小横条：移动端 2 列，平板及以上 4 列 */}
          <div class="grid grid-cols-2 sm:grid-cols-4 gap-2.5 bg-subtle/20 p-3 rounded-control border border-subtle/40 text-xs">
            <div>
              <span class="text-faint block text-[11px]">命中率</span>
              <span class="font-semibold text-foreground text-sm">
                {((cacheStats()?.hitRate ?? 0) * 100).toFixed(1)}%
              </span>
              <span class="text-[10px] text-muted block mt-0.5 truncate">
                {cacheStats()?.hits ?? 0} 命中 / {cacheStats()?.misses ?? 0} 未中
              </span>
            </div>
            <div>
              <span class="text-faint block text-[11px]">累计节省 Token</span>
              <span class="font-semibold text-accent text-sm">
                {(cacheStats()?.tokensSaved ?? 0).toLocaleString()}
              </span>
              <span class="text-[10px] text-muted block mt-0.5 truncate">直接节省消耗</span>
            </div>
            <div>
              <span class="text-faint block text-[11px]">在存条目</span>
              <span class="font-semibold text-foreground text-sm">
                {cacheStats()?.entries ?? 0} / {cacheStats()?.maxEntries ?? 1000}
              </span>
              <span class="text-[10px] text-muted block mt-0.5 truncate">LRU 内存置换池</span>
            </div>
            <div>
              <span class="text-faint block text-[11px]">内存占用</span>
              <span class="font-semibold text-foreground text-sm">
                {formatBytes(cacheStats()?.bytesUsed ?? 0)}
              </span>
              <span class="text-[10px] text-muted block mt-0.5 truncate">单条 ≤2MB 保护</span>
            </div>
          </div>

          {/* 控制项 */}
          <div class="flex items-start justify-between gap-4 pt-1">
            <Field label="启用响应精确缓存" hint="对完全一致的对话请求（temperature=0、相同上下文）直接直出缓存">
              <span />
            </Field>
            <Toggle
              checked={!!local().responseCacheEnabled}
              onChange={v => {
                set('responseCacheEnabled', v)
                if (v && !local().responseCacheTTL) set('responseCacheTTL', 3600)
              }}
            />
          </div>

          <Show when={!!local().responseCacheEnabled}>
            <div class="space-y-3 pt-2 border-t border-subtle/50">
              <Field label="缓存有效期 (TTL)" hint="缓存条目的生存秒数，默认 3600 秒（1 小时），过期自动清理">
                <Input
                  type="number"
                  class="!w-full sm:!w-36 mt-1"
                  value={String(local().responseCacheTTL ?? 3600)}
                  onInput={v => set('responseCacheTTL', Math.max(1, Number(v) || 3600))}
                />
              </Field>
              <div class="flex items-start justify-between gap-4 pt-1 border-t border-subtle/50">
                <Field label="缓存全部非流式请求" hint="开启后不论 temperature 为何值均缓存；关闭则仅缓存确定性请求">
                  <span />
                </Field>
                <Toggle
                  checked={!!local().responseCacheAll}
                  onChange={v => set('responseCacheAll', v)}
                />
              </div>
            </div>
          </Show>
        </Card>

        {/* 令牌节省引擎卡片 */}
        <Card class="p-5 space-y-4">
          <div class="flex items-center justify-between border-b border-subtle/50 pb-3">
            <div class="flex items-center gap-2">
              <IconSparkles size={16} class="text-accent shrink-0" />
              <h3 class="text-sm font-semibold">令牌节省引擎</h3>
              <Badge tone="gray">RTK · Caveman · Ponytail</Badge>
            </div>
          </div>

          {/* RTK 压缩 */}
          <div class="flex items-start justify-between gap-4">
            <Field label="RTK 压缩" hint="无损清洗工具输出并对超长结果实施首尾智能截断（保留前 120 行与后 60 行）">
              <span />
            </Field>
            <Toggle checked={!!local().rtkEnabled} onChange={v => set('rtkEnabled', v)} />
          </div>

          {/* Caveman 极简表达 */}
          <div class="space-y-3 pt-2 border-t border-subtle/50">
            <div class="flex items-start justify-between gap-4">
              <Field label="Caveman 极简表达" hint="注入极简表达指令（洞穴人模式），压制客套寒暄，大幅削减回复 Token">
                <span />
              </Field>
              <Toggle
                checked={!!local().cavemanEnabled}
                onChange={v => {
                  set('cavemanEnabled', v)
                  if (v && !local().cavemanLevel) set('cavemanLevel', 'lite')
                }}
              />
            </div>
            <Show when={!!local().cavemanEnabled}>
              <div class="pl-3 sm:pl-4 border-l-2 border-primary/30">
                <Field label="压缩级别" hint="lite: 保留要点 | full: 极限简洁 | ultra: 绝不废话 | wenyan: 文言风格">
                  <Select
                    value={String(local().cavemanLevel || 'lite')}
                    options={cavemanOptions}
                    onChange={v => set('cavemanLevel', v)}
                    class="w-full mt-1"
                  />
                </Field>
              </div>
            </Show>
          </div>

          {/* Ponytail 极简代码 */}
          <div class="space-y-3 pt-2 border-t border-subtle/50">
            <div class="flex items-start justify-between gap-4">
              <Field label="Ponytail 极简代码" hint="注入极简代码原则指令（马尾模式），推崇 YAGNI，严禁多余抽象与样板代码">
                <span />
              </Field>
              <Toggle
                checked={!!local().ponytailEnabled}
                onChange={v => {
                  set('ponytailEnabled', v)
                  if (v && !local().ponytailLevel) set('ponytailLevel', 'lite')
                }}
              />
            </div>
            <Show when={!!local().ponytailEnabled}>
              <div class="pl-3 sm:pl-4 border-l-2 border-primary/30">
                <Field label="压缩级别" hint="lite: 最简替代 | full: 严格阶梯 | ultra: 极致单行">
                  <Select
                    value={String(local().ponytailLevel || 'lite')}
                    options={ponytailOptions}
                    onChange={v => set('ponytailLevel', v)}
                    class="w-full mt-1"
                  />
                </Field>
              </div>
            </Show>
          </div>

          {/* 排除名单 */}
          <div class="space-y-3 pt-3 border-t border-subtle/50">
            <div>
              <Field
                label="排除提供商名单 (TokenSaver Exclude)"
                hint="指定跳过 RTK 压缩与极简提示词注入的提供商（如保护复杂指令遵循模型）"
              >
                <span />
              </Field>
            </div>

            <div class="flex flex-wrap items-center gap-1.5 min-h-[32px] p-2 rounded-control bg-bg-elevated border border-subtle">
              <Show
                when={excludedProviders().length > 0}
                fallback={<span class="text-xs text-faint">暂无排除项（所有提供商均应用 Token 节省规则）</span>}
              >
                <For each={excludedProviders()}>
                  {p => (
                    <span class="inline-flex items-center gap-1.5 px-2 py-0.5 rounded bg-warning/10 text-warning text-xs font-mono">
                      <span>{p}</span>
                      <button
                        type="button"
                        class="hover:opacity-75 focus:outline-none"
                        title="移除排除"
                        onClick={() => removeExcludeProvider(p)}
                      >
                        &times;
                      </button>
                    </span>
                  )}
                </For>
              </Show>
            </div>

            {/* 输入框与添加按钮：移动端自动换行/占满，大屏并排 */}
            <div class="flex flex-col sm:flex-row items-stretch sm:items-center gap-2">
              <Input
                placeholder="输入提供商名称（如 deepseek）后回车或点击添加"
                class="flex-1 text-xs !w-full"
                value={excludeInput()}
                onInput={setExcludeInput}
                onKeyDown={e => {
                  if (e.key === 'Enter') {
                    e.preventDefault()
                    addExcludeProvider(excludeInput())
                  }
                }}
              />
              <Button
                size="sm"
                variant="secondary"
                class="shrink-0"
                onClick={() => addExcludeProvider(excludeInput())}
              >
                添加
              </Button>
            </div>

            <Show when={quickSuggestions().length > 0}>
              <div class="flex flex-wrap items-center gap-1 text-[11px] text-faint">
                <span>快速添加已配提供商:</span>
                <For each={quickSuggestions()}>
                  {name => (
                    <button
                      type="button"
                      class="px-1.5 py-0.5 rounded bg-subtle hover:bg-hover text-muted hover:text-foreground transition-colors font-mono"
                      onClick={() => addExcludeProvider(name)}
                    >
                      +{name}
                    </button>
                  )}
                </For>
              </div>
            </Show>
          </div>
        </Card>
      </div>
      </Show>

      {/* ── Tab 2：界面与外观偏好 ── */}
      <Show when={activeTab() === 'appearance'}>

      {/* ── 分组 3：外观与系统偏好 ── */}
      <div class="space-y-3.5">
        <div class="flex items-center gap-1.5 px-0.5 pt-2 text-[11px] font-semibold uppercase tracking-wider text-faint">
          <IconPalette size={14} class="text-accent shrink-0" />
          <span>界面与系统偏好</span>
        </div>

        {/* 界面与壁纸卡片 */}
        <Card class="p-5 space-y-4">
          <div class="flex items-center justify-between border-b border-subtle/50 pb-3">
            <div class="flex items-center gap-2">
              <IconPalette size={16} class="text-accent shrink-0" />
              <div>
                <h3 class="text-sm font-semibold">界面与壁纸</h3>
                <p class="text-xs text-faint mt-0.5">大图持久化于浏览器 IndexedDB，外观配置实时保存在 localStorage</p>
              </div>
            </div>
            <Show when={bgStore.hasCustomBg()}>
              <Button
                size="sm"
                variant="danger"
                onClick={async () => {
                  const ok = await confirm({
                    title: '清除自定义壁纸',
                    message: '确定清除当前壁纸并恢复默认纯净背景吗？',
                    variant: 'danger',
                  })
                  if (!ok) return
                  await bgStore.resetWallpaper()
                  setBgUrlInput('')
                  toast.success('已恢复默认背景')
                }}
              >
                清除壁纸
              </Button>
            </Show>
          </div>

          <div class="grid grid-cols-1 sm:grid-cols-2 gap-4 pt-1">
            {/* 远程图片链接 */}
            <div class="space-y-1.5">
              <label class="text-xs font-medium text-muted">远程图片 URL</label>
              <div class="flex flex-col sm:flex-row gap-2">
                <Input
                  value={bgUrlInput()}
                  placeholder="https://..."
                  onInput={setBgUrlInput}
                  class="flex-1 !w-full"
                />
                <Button
                  variant="secondary"
                  disabled={!bgUrlInput().trim()}
                  loading={loadingBgUrl()}
                  class="shrink-0"
                  onClick={async () => {
                    const url = bgUrlInput().trim()
                    if (!url) return
                    setLoadingBgUrl(true)
                    try {
                      // 预先校验图片是否能够成功渲染与加载，杜绝失效 URL 或拦截导致的静默白屏
                      const img = new Image()
                      img.referrerPolicy = 'no-referrer'
                      const { promise: imgPromise, resolve: imgResolve, reject: imgReject } = Promise.withResolvers<boolean>()
                      img.onload = () => imgResolve(true)
                      img.onerror = () => imgReject(new Error('image load failed'))
                      img.src = url

                      try {
                        await imgPromise
                      } catch {
                        toast.error('远程图片加载失败：请检查链接有效性或防盗链设置')
                        return
                      }

                      let dataUrl = ''
                      try {
                        const resp = await fetch(url, { referrerPolicy: 'no-referrer' })
                        if (resp.ok) {
                          const blob = await resp.blob()
                          if (blob.type.startsWith('image/')) {
                            const reader = new FileReader()
                            const { promise, resolve, reject } = Promise.withResolvers<string>()
                            reader.onload = () => resolve(reader.result as string)
                            reader.onerror = () => reject(reader.error)
                            reader.readAsDataURL(blob)
                            dataUrl = await promise
                          }
                        }
                      } catch {
                        // 跨域或安全拦截直接拉取时，降级使用 direct url 保存进 IndexedDB
                      }

                      if (!dataUrl) {
                        dataUrl = url
                      }

                      await bgStore.setWallpaper(dataUrl, { sourceType: 'remote', remoteUrl: url })
                      toast.success('已保存远程壁纸至本地存储')
                    } catch {
                      toast.error('保存远程壁纸失败')
                    } finally {
                      setLoadingBgUrl(false)
                    }
                  }}
                >
                  应用
                </Button>
              </div>
            </div>

            {/* 本地图片上传 */}
            <div class="space-y-1.5">
              <label class="text-xs font-medium text-muted">本地图片上传</label>
              <label class="cursor-pointer flex items-center justify-center gap-2 px-3 py-2 rounded-control bg-black/4 dark:bg-white/6 hover:bg-black/7 dark:hover:bg-white/10 text-xs text-muted hover:text-foreground transition-all min-h-[34px] shadow-xs">
                <svg class="w-4 h-4 shrink-0" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                  <rect width="18" height="18" x="3" y="3" rx="2" ry="2" />
                  <circle cx="9" cy="9" r="2" />
                  <path d="m21 15-3.086-3.086a2 2 0 0 0-2.828 0L6 21" />
                </svg>
                <span class="truncate">选择本地图片...</span>
                <input
                  type="file"
                  accept="image/*"
                  class="hidden"
                  onChange={e => {
                    const file = e.currentTarget.files?.[0]
                    if (!file) return
                    const reader = new FileReader()
                    reader.onload = async () => {
                      const dataUrl = reader.result as string
                      await bgStore.setWallpaper(dataUrl, { sourceType: 'upload' })
                      toast.success(`已保存本地图片至数据库 (${file.name})`)
                    }
                    reader.readAsDataURL(file)
                  }}
                />
              </label>
            </div>
          </div>

          {/* 壁纸与毛玻璃微调控制面板（仿 zashboard 外观微调系统） */}
          <Show when={bgStore.hasCustomBg()}>
            <div class="space-y-3 pt-3 border-t border-subtle/50">
              <div class="text-xs font-semibold text-muted">毛玻璃拟态与外观微调</div>
              <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
                {/* 背景虚化 */}
                <div class="space-y-1">
                  <div class="flex justify-between text-xs">
                    <span class="text-muted">背景虚化 (Blur)</span>
                    <span class="font-mono text-faint">{bgStore.config().blur || 0}px</span>
                  </div>
                  <input
                    type="range"
                    min="0"
                    max="30"
                    step="1"
                    class="w-full accent-accent cursor-pointer"
                    value={bgStore.config().blur || 0}
                    onInput={e => {
                      bgStore.updateConfig({ blur: Number(e.currentTarget.value) })
                    }}
                  />
                </div>

                {/* 背景不透明度 */}
                <div class="space-y-1">
                  <div class="flex justify-between text-xs">
                    <span class="text-muted">背景不透明度 (Opacity)</span>
                    <span class="font-mono text-faint">{Math.round((bgStore.config().opacity ?? 1) * 100)}%</span>
                  </div>
                  <input
                    type="range"
                    min="0.1"
                    max="1"
                    step="0.05"
                    class="w-full accent-accent cursor-pointer"
                    value={bgStore.config().opacity ?? 1}
                    onInput={e => {
                      bgStore.updateConfig({ opacity: Number(e.currentTarget.value) })
                    }}
                  />
                </div>

                {/* 面板底色不透明度 */}
                <div class="space-y-1">
                  <div class="flex justify-between text-xs">
                    <span class="text-muted">面板底色透明度 (Surface Alpha)</span>
                    <span class="font-mono text-faint">{Math.round((bgStore.config().surfaceAlpha ?? 0.78) * 100)}%</span>
                  </div>
                  <input
                    type="range"
                    min="0.4"
                    max="0.95"
                    step="0.02"
                    class="w-full accent-accent cursor-pointer"
                    value={bgStore.config().surfaceAlpha ?? 0.78}
                    onInput={e => {
                      bgStore.updateConfig({ surfaceAlpha: Number(e.currentTarget.value) })
                    }}
                  />
                </div>

                {/* 面板毛玻璃强度 */}
                <div class="space-y-1">
                  <div class="flex justify-between text-xs">
                    <span class="text-muted">毛玻璃模糊度 (Glass Blur)</span>
                    <span class="font-mono text-faint">{bgStore.config().glassBlur ?? 20}px</span>
                  </div>
                  <input
                    type="range"
                    min="0"
                    max="40"
                    step="2"
                    class="w-full accent-accent cursor-pointer"
                    value={bgStore.config().glassBlur ?? 20}
                    onInput={e => {
                      bgStore.updateConfig({ glassBlur: Number(e.currentTarget.value) })
                    }}
                  />
                </div>
              </div>
            </div>
          </Show>
        </Card>
      </div>
    </Show>

      {/* ── Tab 3：数据管理与备份恢复 ── */}
      <Show when={activeTab() === 'data'}>
        <div class="space-y-3.5">
          <div class="flex items-center gap-1.5 px-0.5 text-[11px] font-semibold uppercase tracking-wider text-faint">
            <IconDatabase size={14} class="text-accent shrink-0" />
            <span>数据导出与恢复</span>
          </div>

          {/* 数据备份导出卡片 */}
          <Card class="p-5 space-y-4">
            <div class="flex items-center justify-between border-b border-subtle/50 pb-3">
              <div class="flex items-center gap-2">
                <IconDownload size={16} class="text-accent shrink-0" />
                <div>
                  <h3 class="text-sm font-semibold">生成数据备份快照</h3>
                  <p class="text-xs text-faint mt-0.5">将网关全量或脱敏配置导出为版本化 JSON 文件 (.cyrene.json)</p>
                </div>
              </div>
              <Badge tone="blue">快照导出</Badge>
            </div>

            <div class="space-y-3.5">
              <Checkbox
                checked={includeSecrets()}
                onChange={setIncludeSecrets}
                label="包含敏感凭据 (API Keys、OAuth Tokens、JWT 密钥)"
                description={
                  <div class="flex items-center gap-1.5 mt-1 text-[11px]">
                    <Show
                      when={includeSecrets()}
                      fallback={
                        <span class="text-faint flex items-center gap-1">
                          <IconInfo size={13} class="text-info shrink-0" />
                          <span>已开启脱敏导出。恢复时将保留现有运行中连接的密钥，适合作为公开模板共享。</span>
                        </span>
                      }
                    >
                      <span class="text-amber-500/90 dark:text-amber-400 flex items-center gap-1">
                        <IconAlertTriangle size={13} class="text-warning shrink-0" />
                        <span>导出的备份将包含完整上游及下游明文密钥，请妥善保管，切勿公开发布或提交至代码仓库！</span>
                      </span>
                    </Show>
                  </div>
                }
              />

              <div class="pt-3 border-t border-subtle/40">
                <Checkbox
                  checked={includeUsage()}
                  onChange={setIncludeUsage}
                  label="包含历史调用日志与请求排障明细"
                  description="一并打包全部 API 历史日志与完整排障报文（若调用量大将增加导出文件体积）。"
                />
              </div>
            </div>

            <div class="pt-2 border-t border-subtle/50 flex justify-end">
              <Button
                variant="primary"
                loading={downloading()}
                onClick={handleDownloadBackup}
                class="flex items-center gap-1.5"
              >
                <IconDownload size={14} />
                <span>下载备份文件</span>
              </Button>
            </div>
          </Card>

          {/* 备份恢复与导入卡片 */}
          <Card class="p-5 space-y-4">
            <div class="flex items-center justify-between border-b border-subtle/50 pb-3">
              <div class="flex items-center gap-2">
                <IconUpload size={16} class="text-amber-400 shrink-0" />
                <div>
                  <h3 class="text-sm font-semibold">从备份文件恢复</h3>
                  <p class="text-xs text-faint mt-0.5">基于 SQLite 单事务原子重载网关配置，杜绝中间态故障</p>
                </div>
              </div>
              <Badge tone="amber">数据恢复</Badge>
            </div>

            <div class="space-y-3.5 text-xs">
              <Field label="备份文件 (.cyrene.json / .json)" hint="选择之前导出的网关备份文件">
                <input
                  type="file"
                  accept=".json,.cyrene.json"
                  class="block w-full text-xs text-faint file:mr-3 file:py-1.5 file:px-3 file:rounded-control file:border-0 file:text-xs file:font-semibold file:bg-hover file:text-foreground hover:file:bg-active cursor-pointer"
                  onChange={e => {
                    const f = e.currentTarget.files?.[0]
                    setRestoreFile(f || null)
                  }}
                />
              </Field>

              <Field label="恢复策略" hint="选择导入数据与当前数据库的合并方式">
                <Select
                  value={restoreMode()}
                  options={[
                    { value: 'replace', label: '全量覆盖 (清空现有用户配置并完整恢复为快照)' },
                    { value: 'merge', label: '增量合并 (保留未冲突项，仅对快照条目进行更新或追加)' },
                  ]}
                  onChange={v => setRestoreMode(v as 'replace' | 'merge')}
                />
              </Field>

              <div class="p-3 bg-amber-500/10 border border-amber-500/20 rounded-control text-[11px] text-amber-500 space-y-1">
                <div class="font-semibold flex items-center gap-1">
                  <IconAlertTriangle size={13} />
                  <span>重要提示</span>
                </div>
                <p>
                  恢复操作会在数据库事务中安全执行并即时重载服务。若导入的是脱敏备份，系统将自动保留现有连接的既有有效密钥。建议在操作前先下载一份当前备份作为底稿！
                </p>
              </div>
            </div>

            <div class="pt-2 border-t border-subtle/50 flex items-center justify-between">
              <span class="text-xs text-faint font-mono">
                {restoreFile() ? `已就绪: ${restoreFile()!.name}` : '未选择文件'}
              </span>
              <Button
                variant="danger"
                disabled={!restoreFile()}
                loading={restoring()}
                onClick={handleRestoreSubmit}
                class="flex items-center gap-1.5"
              >
                <IconUpload size={14} />
                <span>开始恢复数据</span>
              </Button>
            </div>
          </Card>
        </div>
      </Show>

      {/* ── 底部通用系统与存储信息 ── */}
      <Card class="p-4 text-xs text-faint">
        <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-3">
          <div class="flex items-center gap-2.5 min-w-0 shrink-0">
            <div class="w-6 h-6 rounded-lg bg-accent/10 border border-accent/20 flex items-center justify-center shrink-0">
              <IconInfo size={14} class="text-accent" />
            </div>
            <span class="font-semibold text-foreground whitespace-nowrap">Cyrene Gateway</span>
            <span
              class="font-mono text-[11px] px-2 py-0.5 rounded-full bg-hover text-faint border border-subtle/50 whitespace-nowrap"
              title={`完整版本：v${store.version() || 'dev'}`}
            >
              v{formatVersion(store.version())}
            </span>
          </div>
          <div class="flex flex-wrap items-center gap-x-4 gap-y-1.5 text-[11px] text-faint sm:justify-end">
            <div class="flex items-center gap-1.5 whitespace-nowrap">
              <span class="w-1.5 h-1.5 rounded-full bg-emerald-400 shrink-0" />
              <span>数据库：</span>
              <span class="font-medium text-foreground">{store.health().db === 'ok' ? '已就绪 (WAL)' : '检测中'}</span>
            </div>
            <span class="text-subtle select-none hidden sm:inline">•</span>
            <div class="flex items-center gap-1 whitespace-nowrap">
              <span>活动连接：</span>
              <span class="font-medium font-mono text-foreground">{store.activeConnections()} / {store.providers().length}</span>
            </div>
            <span class="text-subtle select-none hidden sm:inline">•</span>
            <div class="flex items-center gap-1 whitespace-nowrap">
              <span>运行时间：</span>
              <span class="font-medium font-mono text-foreground">{formatUptime(Number(store.health().uptimeSeconds))}</span>
            </div>
          </div>
        </div>
      </Card>
    </div>
  )
}

export default Settings
