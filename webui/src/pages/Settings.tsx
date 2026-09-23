import { type Component, For, Show, Switch, Match, createSignal, createEffect, on, onMount } from 'solid-js'
import { useGatewayStore } from '@/stores/gateway'
import { useBackgroundStore } from '@/stores/background'
import { useThemeStore } from '@/stores/theme'
import { fetchRemoteImageDataUrl } from '@/lib/backgroundStore'
import {
  Card, Badge, Button, IconButton, Input, Select, Toggle, Field, Empty, Alert, confirm, PageHeader, SegmentedControl, Checkbox, Slider, FileUpload, StatusPulse, TabTransition, Skeleton, LoadState, UnsavedChangesBar,
  IconLock, IconKey, IconZap, IconSparkles, IconPalette, IconInfo, IconCheck, IconSun, IconMoon,
  IconDownload, IconUpload, IconAlertTriangle, IconDatabase, IconServer, IconRotateCcw, IconActivity,
} from '@/components/ui'
import { formatUptime, formatVersion, formatBytes, type UptimeUnits } from '@/lib/format'
import { useToast } from '@/lib/toast'
import { api, apiPost } from '@/lib/api'
import { useI18n } from '@/i18n'

interface CacheStats {
  hits: number
  misses: number
  hitRate: number
  entries: number
  maxEntries: number
  bytesUsed: number
  tokensSaved: number
}

const Settings: Component = () => {
  const { t } = useI18n()
  const themeStore = useThemeStore()
  const cavemanOptions = () => [
    { value: 'lite', label: t('settings.tokenSaver.cavemanLevels.lite') },
    { value: 'full', label: t('settings.tokenSaver.cavemanLevels.full') },
    { value: 'ultra', label: t('settings.tokenSaver.cavemanLevels.ultra') },
    { value: 'wenyan-lite', label: t('settings.tokenSaver.cavemanLevels.wenyanLite') },
    { value: 'wenyan', label: t('settings.tokenSaver.cavemanLevels.wenyan') },
    { value: 'wenyan-ultra', label: t('settings.tokenSaver.cavemanLevels.wenyanUltra') },
  ]

  const ponytailOptions = () => [
    { value: 'lite', label: t('settings.tokenSaver.ponytailLevels.lite') },
    { value: 'full', label: t('settings.tokenSaver.ponytailLevels.full') },
    { value: 'ultra', label: t('settings.tokenSaver.ponytailLevels.ultra') },
  ]
  const uptimeUnits = (): UptimeUnits => ({
    s: t('units.second'),
    m: t('units.minuteShort'),
    h: t('units.hourShort'),
    d: t('units.day'),
    mLong: t('units.minute'),
    hLong: t('units.hour'),
  })
  const store = useGatewayStore()
  const bgStore = useBackgroundStore()
  const toast = useToast()
  const [saving, setSaving] = createSignal(false)
  const [local, setLocal] = createSignal<Record<string, unknown>>({ ...store.settings() })
  const [initialized, setInitialized] = createSignal(store.loaded.settings)
  const [edited, setEdited] = createSignal(false)
  const [pw, setPw] = createSignal('')
  const [hasPw, setHasPw] = createSignal(!!store.settings().hasPassword)

  // 背景自定义状态
  const [bgUrlInput, setBgUrlInput] = createSignal(bgStore.config().remoteUrl || '')
  const [loadingBgUrl, setLoadingBgUrl] = createSignal(false)
  createEffect(() => {
    const rUrl = bgStore.config().remoteUrl
    if (rUrl && !bgUrlInput()) {
      setBgUrlInput(rUrl)
    }
  })
  createEffect(on([store.settings, () => store.loaded.settings], ([settings, ready]) => {
    if (ready) {
      if (!edited()) setLocal({ ...settings })
      setHasPw(!!settings.hasPassword)
      setInitialized(true)
    }
  }))
  // 响应缓存状态
  const [cacheStats, setCacheStats] = createSignal<CacheStats | null>(null)
  const [clearingCache, setClearingCache] = createSignal(false)
  const [refreshingStats, setRefreshingStats] = createSignal(false)

  // TokenSaver 排除项状态
  // 设置分类 Tab
  type ActiveTabType = 'gateway' | 'appearance' | 'data' | 'ops'
  const [activeTab, setActiveTab] = createSignal<ActiveTabType>('gateway')
  async function handleTabChange(tab: ActiveTabType) {
    if (activeTab() === 'gateway' && dirty()) {
      const ok = await confirm({
        title: t('common.unsavedChangesTitle'),
        message: t('common.unsavedChangesMessage'),
        confirmText: t('common.discardAndLeave'),
        cancelText: t('common.stayAndSave'),
        variant: 'warning',
      })
      if (!ok) return
      setLocal({ ...(store.settings() || {}) })
    }
    setActiveTab(tab)
    if (tab === 'ops') {
      void fetchSystemStats()
    }
    if (typeof window !== 'undefined') {
      window.scrollTo({ top: 0, behavior: 'smooth' })
    }
  }

  // ── 系统运维与监控状态 ──
  interface SystemStatsData {
    version: string
    uptimeSeconds: number
    startTime: string
    pid: number
    os: string
    arch: string
    numCPU: number
    goVersion: string
    goroutines: number
    inDocker: boolean
    memory: {
      allocBytes: number
      totalAllocBytes: number
      sysBytes: number
      heapAllocBytes: number
      heapInuseBytes: number
      numGC: number
    }
    storage?: {
      dbSizeBytes: number
      walSizeBytes: number
      pageCount: number
      pageSize: number
      totalRequests: number
      totalDetails: number
    }
  }

  interface UpdateCheckData {
    currentVersion: string
    latestVersion: string
    hasUpdate: boolean
    releaseNotes: string
    publishedAt: string
    assetName?: string
    assetSize?: number
  }

  const [systemStats, setSystemStats] = createSignal<SystemStatsData | null>(null)
  const [loadingStats, setLoadingStats] = createSignal(false)
  const [updateInfo, setUpdateInfo] = createSignal<UpdateCheckData | null>(null)
  const [checkingUpdate, setCheckingUpdate] = createSignal(false)
  const [applyingUpdate, setApplyingUpdate] = createSignal(false)
  const [restarting, setRestarting] = createSignal(false)
  const [pruneDays, setPruneDays] = createSignal(30)
  const [runningMaint, setRunningMaint] = createSignal<string | null>(null)

  async function fetchSystemStats() {
    setLoadingStats(true)
    try {
      const data = await api<SystemStatsData>('/api/system/stats')
      setSystemStats(data)
    } catch {
      // Error surfaced by api.ts
    } finally {
      setLoadingStats(false)
    }
  }

  async function handleCheckUpdate() {
    setCheckingUpdate(true)
    try {
      const data = await api<UpdateCheckData>('/api/system/update/check')
      setUpdateInfo(data)
      if (data.hasUpdate) {
        toast.info(`${t('settings.ops.hasUpdateBadge')}: v${data.latestVersion}`)
      } else {
        toast.success(t('settings.ops.upToDateBadge'))
      }
    } catch {
      // Error surfaced by api.ts
    } finally {
      setCheckingUpdate(false)
    }
  }

  async function handleApplyUpdate() {
    if (!updateInfo()?.hasUpdate) return
    const confirmed = await confirm({
      title: t('settings.ops.updateTitle'),
      message: `${t('settings.ops.updateNow')} (v${updateInfo()?.latestVersion})?`,
      variant: 'primary',
    })
    if (!confirmed) return

    setApplyingUpdate(true)
    try {
      await apiPost('/api/system/update', {})
      toast.success(t('settings.ops.updateSuccess'))
      const shouldRestart = await confirm({
        title: t('settings.ops.restartTitle'),
        message: t('settings.ops.restartConfirm'),
        variant: 'primary',
      })
      if (shouldRestart) {
        await handleTriggerRestart()
      }
    } catch {
      // Error surfaced by api.ts
    } finally {
      setApplyingUpdate(false)
    }
  }

  async function handleTriggerRestart() {
    const confirmed = await confirm({
      title: t('settings.ops.restartTitle'),
      message: t('settings.ops.restartConfirm'),
      variant: 'danger',
    })
    if (!confirmed) return

    setRestarting(true)
    try {
      await apiPost('/api/system/restart', {})
    } catch {
      // Ignore network abort if server closes instantly
    }

    let attempts = 0
    const maxAttempts = 30
    const interval = setInterval(async () => {
      attempts++
      try {
        const h = await api<{ status: string }>('/api/health', { silent: true })
        if (h && h.status === 'ok') {
          clearInterval(interval)
          setRestarting(false)
          toast.success(t('settings.ops.reconnectSuccess'))
          void fetchSystemStats()
          void store.loadSettings()
        }
      } catch {
        if (attempts >= maxAttempts) {
          clearInterval(interval)
          setRestarting(false)
          toast.error(t('settings.ops.reconnectTimeout'))
        }
      }
    }, 1000)
  }

  async function handleRollback() {
    const confirmed = await confirm({
      title: t('settings.ops.rollbackBtn'),
      message: t('settings.ops.rollbackConfirm'),
      variant: 'danger',
    })
    if (!confirmed) return

    try {
      await apiPost('/api/system/rollback', {})
      toast.success(t('settings.ops.rollbackSuccess'))
    } catch {
      // Error surfaced by api.ts
    }
  }

  async function handleMaintenance(action: 'checkpoint' | 'vacuum' | 'prune_logs') {
    setRunningMaint(action)
    try {
      const res = await apiPost<{ success: boolean; prunedHistory?: number; prunedDetails?: number }>(
        '/api/system/maintenance',
        { action, retentionDays: pruneDays() }
      )
      if (action === 'checkpoint') {
        toast.success(t('settings.ops.checkpointSuccess'))
      } else if (action === 'vacuum') {
        toast.success(t('settings.ops.vacuumSuccess'))
      } else if (action === 'prune_logs') {
        toast.success(
          t('settings.ops.pruneSuccess', {
            history: String(res.prunedHistory ?? 0),
            details: String(res.prunedDetails ?? 0),
          })
        )
      }
      void fetchSystemStats()
    } catch {
      // Error surfaced by api.ts
    } finally {
      setRunningMaint(null)
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
      toast.success(t('toast.backupExportSuccess'))
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : 'error'
      toast.error(t('toast.backupExportFailed', { error: msg }))
    } finally {
      setDownloading(false)
    }
  }

  async function handleRestoreSubmit() {
    const file = restoreFile()
    if (!file) {
      toast.warning(t('toast.selectBackupFileFirst'))
      return
    }
    const modeText = restoreMode() === 'replace'
      ? t('settings.data.restoreModeReplace')
      : t('settings.data.restoreModeMerge')

    const ok = await confirm({
      title: t('settings.data.restoreConfirmTitle'),
      message: t('settings.data.restoreConfirmMessage', { file: file.name, mode: modeText }),
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
      toast.success(t('toast.restoreSuccess'))
      setRestoreFile(null)
      await store.loadCore()
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : t('settings.restoreFailed')
      toast.error(t('toast.restoreFailed', { error: msg }))
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
      toast.success(t('toast.cacheStatsRefreshed'))
    } finally {
      setRefreshingStats(false)
    }
  }

  async function handleClearCache() {
    const ok = await confirm({
      title: t('settings.cache.clearConfirmTitle'),
      message: t('settings.cache.clearConfirmMessage'),
      variant: 'danger',
    })
    if (!ok) return
    setClearingCache(true)
    try {
      await apiPost('/api/cache/clear')
      toast.success(t('toast.cacheCleared'))
      await fetchCacheStats()
    } catch {
      // Error surfaced by api.ts
    } finally {
      setClearingCache(false)
    }
  }

  onMount(() => {
    void store.loadSettings()
    if (store.providers().length === 0) {
      void store.loadProvidersOnly()
    }
    if (bgStore.config().sourceType === 'remote' && bgStore.config().remoteUrl) {
      setBgUrlInput(bgStore.config().remoteUrl || '')
    }
    void fetchCacheStats()
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

  const set = (k: string, v: unknown) => {
    setEdited(true)
    setLocal(l => ({ ...l, [k]: v }))
  }

  async function save() {
    setSaving(true)
    try {
      const payload = { ...local() }
      delete payload.hasPassword
      delete payload.passwordHash
      await store.saveSettings(payload)
      if (!store.loadErrors.settings) {
        setEdited(false)
        setLocal({ ...store.settings() })
        setHasPw(!!store.settings().hasPassword)
      }
    } catch {
      // Error surfaced by api.ts
    } finally {
      setSaving(false)
    }
  }

  async function changePassword() {
    if (pw().length < 8) {
      toast.warning(t('toast.passwordMinLength'))
      return
    }
    try {
      await store.setPassword(pw())
      setPw('')
      setHasPw(true)
      toast.success(t('toast.passwordUpdateSuccess'))
    } catch {
      // Error surfaced by api.ts
    }
  }

  return (
    <div class="max-w-3xl mx-auto space-y-6 stagger">
      <PageHeader
        title={t('settings.title')}
        subtitle={t('settings.subtitle')}
        actions={
          <Show when={activeTab() === 'gateway'}>
            <Button variant="primary" loading={saving()} disabled={!initialized() || !dirty()} onClick={save} class="shrink-0">
              {dirty() ? t('common.save') : t('common.saved')}
            </Button>
          </Show>
        }
      />

      {/* 紧凑型吸顶分类切换器 */}
      <div class="sticky top-17 z-20 glass-panel py-2.5 rounded-2xl px-3 flex flex-col sm:flex-row sm:items-center justify-between gap-2.5 transition-all">
        <SegmentedControl
          class="shrink-0"
          options={[
            { value: 'gateway', label: t('settings.tabs.gateway') },
            { value: 'appearance', label: t('settings.tabs.appearance') },
            { value: 'data', label: t('settings.tabs.data') },
            { value: 'ops', label: t('settings.tabs.ops') },
          ]}
          value={activeTab()}
          onChange={handleTabChange}
        />
        <div class="text-xs text-faint flex items-center gap-1.5 px-0.5 min-w-0 overflow-hidden">
          <Show when={activeTab() === 'gateway'}>
            <span class="w-1.5 h-1.5 rounded-full bg-accent animate-pulse shrink-0" />
            <span class="truncate">{t('settings.hosts.gateway')}</span>
          </Show>
          <Show when={activeTab() === 'appearance'}>
            <span class="w-1.5 h-1.5 rounded-full bg-warning shrink-0" />
            <span class="truncate">{t('settings.hosts.appearance')}</span>
          </Show>
          <Show when={activeTab() === 'data'}>
            <span class="w-1.5 h-1.5 rounded-full bg-success shrink-0" />
            <span class="truncate">{t('settings.hosts.data')}</span>
          </Show>
          <Show when={activeTab() === 'ops'}>
            <span class="w-1.5 h-1.5 rounded-full bg-info shrink-0" />
            <span class="truncate">{t('settings.hosts.ops')}</span>
          </Show>
        </div>
      </div>

      <TabTransition
        value={activeTab()}
        order={['gateway', 'appearance', 'data', 'ops']}
      >
        {tab => (
          <Switch>
            <Match when={tab === 'gateway'}>
              <LoadState ready={initialized()} error={store.loadErrors.settings} onRetry={() => store.loadSettings()} fallback={
                <div class="space-y-3.5">
                  <For each={[0, 1, 2]}>{() => <Card class="p-5 space-y-4"><Skeleton class="h-5 w-32" /><Skeleton class="h-32 w-full" /></Card>}</For>
                </div>
              }>
              <div class="space-y-3.5">

        {/* 访问控制卡片 */}
        <Card class="p-5 space-y-4">
          <div class="flex flex-wrap items-center justify-between gap-3 border-b border-subtle/50 pb-3">
            <div class="flex flex-wrap items-center gap-2 min-w-0">
              <IconLock size={16} class="text-accent shrink-0" />
              <h3 class="text-sm font-semibold">{t('settings.access.cardTitle')}</h3>
            </div>
            <Badge tone={local().requireLogin || local().requireApiKey ? 'blue' : 'gray'}>
              {local().requireLogin || local().requireApiKey ? t('settings.access.protected') : t('settings.access.open')}
            </Badge>
          </div>
          <div class="p-4 rounded-xl bg-surface-inset space-y-4">
            <div class="flex items-start justify-between gap-4">
              <Field
                label={t('settings.access.requireLogin')}
                hint={
                  store.isWan()
                    ? t('settings.access.requireLoginWanLocked')
                    : hasPw()
                      ? t('settings.access.requireLoginHint')
                      : t('settings.access.requireLoginNoPw')
                }
              >
                <span />
              </Field>
              <Toggle
                ariaLabel={t('settings.access.requireLogin')}
                checked={store.isWan() || !!local().requireLogin}
                disabled={store.isWan() || !hasPw()}
                onChange={v => {
                  if (store.isWan()) return
                  if (v && !hasPw()) {
                    toast.warning(t('toast.setAdminPasswordFirst'))
                    return
                  }
                  set('requireLogin', v)
                }}
              />
            </div>

            <div class="flex items-start justify-between gap-4 pt-3 border-t border-subtle/50">
              <Field label={t('settings.access.requireApiKey')} hint={t('settings.access.requireApiKeyHint')}>
                <span />
              </Field>
              <Toggle ariaLabel={t('settings.access.requireApiKey')} checked={!!local().requireApiKey} onChange={v => set('requireApiKey', v)} />
            </div>

            <div class="pt-3 border-t border-subtle/50">
              <Field label={t('settings.access.rpmLimit')} hint={t('settings.access.rpmLimitHint')}>
                <Input
                  type="number"
                  class="w-full! sm:w-36! mt-1"
                  value={String(local().apiKeyRpm ?? 0)}
                  onInput={v => set('apiKeyRpm', Number(v) || 0)}
                />
              </Field>
            </div>
          </div>
        </Card>

        {/* 管理密码卡片 */}
        <Card class="p-5 space-y-4">
          <div class="flex flex-wrap items-center justify-between gap-3 border-b border-subtle/50 pb-3">
            <div class="flex flex-wrap items-center gap-2 min-w-0">
              <IconKey size={16} class="text-accent shrink-0" />
              <h3 class="text-sm font-semibold">{t('settings.access.managePassword')}</h3>
            </div>
            <Badge tone={hasPw() ? 'green' : 'amber'}>
              {hasPw() ? t('settings.access.pwConfigured') : t('settings.access.pwUnset')}
            </Badge>
          </div>
          <div class="p-4 rounded-xl bg-surface-inset min-w-0">
            <Field label={t('settings.access.changePassword')} hint={t('settings.access.pwHelpText')}>
              <div class="grid grid-cols-1 sm:grid-cols-[minmax(0,1fr)_auto] items-center gap-2.5 pt-1">
                <Input
                  type="password"
                  value={pw()}
                  onInput={setPw}
                  placeholder={t('settings.pwPlaceholder')}
                  class="min-w-0"
                />
                <Button
                  variant="secondary"
                  disabled={pw().length < 8}
                  onClick={changePassword}
                  class="shrink-0"
                >
                  {t('settings.updatePassword')}
                </Button>
              </div>
            </Field>
          </div>
        </Card>

        {/* 响应精确缓存卡片 */}
        <Card class="p-5 space-y-4">
          <div class="flex flex-wrap items-center justify-between gap-3 border-b border-subtle/50 pb-3">
            <div class="flex flex-wrap items-center gap-2 min-w-0">
              <IconZap size={16} class="text-accent shrink-0" />
              <h3 class="text-sm font-semibold">{t('settings.cache.title')}</h3>
              <Badge tone="blue">{t('settings.cache.badge')}</Badge>
            </div>
            <div class="flex items-center gap-1.5">
              <Button
                size="sm"
                variant="ghost"
                loading={refreshingStats()}
                onClick={handleRefreshStats}
                title={t('settings.refreshStatsTitle')}
              >
                {t('common.refresh')}
              </Button>
              <Button
                size="sm"
                variant="ghost"
                class="text-danger hover:text-danger hover:bg-danger/10"
                loading={clearingCache()}
                onClick={handleClearCache}
                title={t('settings.clearCacheTitle')}
              >
                {t('settings.cache.clearCache')}
              </Button>
            </div>
          </div>

          {/* 缓存指标数据小横条：移动端 2 列，平板及以上 4 列 */}
          <div class="grid grid-cols-2 sm:grid-cols-4 gap-4 bg-surface-inset p-4 rounded-xl text-xs wrap-anywhere">
            <div class="space-y-0.5">
              <span class="text-faint block text-xs">{t('settings.cache.hitRate')}</span>
              <span class="font-semibold text-foreground text-sm">
                {((cacheStats()?.hitRate ?? 0) * 100).toFixed(1)}%
              </span>
              <span class="text-xs leading-relaxed text-muted block mt-0.5">
                {t('settings.cache.hitsAndMisses', { hits: cacheStats()?.hits ?? 0, misses: cacheStats()?.misses ?? 0 })}
              </span>
            </div>
            <div class="space-y-0.5">
              <span class="text-faint block text-xs">{t('settings.cache.tokensSaved')}</span>
              <span class="font-semibold text-accent text-sm">
                {(cacheStats()?.tokensSaved ?? 0).toLocaleString()}
              </span>
              <span class="text-xs leading-relaxed text-muted block mt-0.5">{t('settings.cache.directSavings')}</span>
            </div>
            <div class="space-y-0.5">
              <span class="text-faint block text-xs">{t('settings.cache.entries')}</span>
              <span class="font-semibold text-foreground text-sm">
                {cacheStats()?.entries ?? 0} / {cacheStats()?.maxEntries ?? 1000}
              </span>
              <span class="text-xs leading-relaxed text-muted block mt-0.5">{t('settings.cache.lruPool')}</span>
            </div>
            <div class="space-y-0.5">
              <span class="text-faint block text-xs">{t('settings.cache.memory')}</span>
              <span class="font-semibold text-foreground text-sm">
                {formatBytes(cacheStats()?.bytesUsed ?? 0)}
              </span>
              <span class="text-xs leading-relaxed text-muted block mt-0.5">{t('settings.cache.entrySizeLimit')}</span>
            </div>
          </div>
          {/* 控制项 */}
          <div class="space-y-3.5">
            <div class="flex items-start justify-between gap-4">
              <Field label={t('settings.cache.enabled')} hint={t('settings.cache.enabledHint')}>
                <span />
              </Field>
              <Toggle
                ariaLabel={t('settings.cache.enabled')}
                checked={!!local().responseCacheEnabled}
                onChange={v => {
                  set('responseCacheEnabled', v)
                  if (v && !local().responseCacheTTL) set('responseCacheTTL', 3600)
                }}
              />
            </div>

            <Show when={!!local().responseCacheEnabled}>
              <div class="space-y-3 pt-3 border-t border-subtle/50">
                <Field label={t('settings.cache.ttl')} hint={t('settings.cache.ttlHint')}>
                  <Input
                    type="number"
                    class="w-full! sm:w-36! mt-1"
                    value={String(local().responseCacheTTL ?? 3600)}
                    onInput={v => set('responseCacheTTL', Math.max(1, Number(v) || 3600))}
                  />
                </Field>
                <div class="flex items-start justify-between gap-4 pt-3 border-t border-subtle/50">
                  <Field label={t('settings.cache.enableAllRequests')} hint={t('settings.cache.enableAllRequestsHint')}>
                    <span />
                  </Field>
                  <Toggle
                    ariaLabel={t('settings.cache.enableAllRequests')}
                    checked={!!local().responseCacheAll}
                    onChange={v => set('responseCacheAll', v)}
                  />
                </div>
              </div>
            </Show>
          </div>
        </Card>

        {/* 令牌节省引擎卡片 */}
        <Card class="p-5 space-y-4">
          <div class="flex flex-wrap items-center justify-between gap-3 border-b border-subtle/50 pb-3">
            <div class="flex flex-wrap items-center gap-2 min-w-0">
              <IconSparkles size={16} class="text-accent shrink-0" />
              <h3 class="text-sm font-semibold">{t('settings.tokenSaver.title')}</h3>
              <Badge tone="gray">RTK · Caveman · Ponytail</Badge>
            </div>
          </div>

          {/* 压缩策略模块 */}
          <div class="p-4 rounded-xl bg-surface-inset space-y-4">
            {/* RTK 压缩 */}
            <div class="flex items-start justify-between gap-4">
              <Field label={t('settings.tokenSaver.rtkTitle')} hint={t('settings.tokenSaver.rtkHint')}>
                <span />
              </Field>
              <Toggle ariaLabel={t('settings.tokenSaver.rtkTitle')} checked={!!local().rtkEnabled} onChange={v => set('rtkEnabled', v)} />
            </div>

            {/* Caveman 极简表达 */}
            <div class="space-y-3 pt-3 border-t border-subtle/50">
              <div class="flex items-start justify-between gap-4">
                <Field label={t('settings.tokenSaver.cavemanTitle')} hint={t('settings.tokenSaver.cavemanHint')}>
                  <span />
                </Field>
                <Toggle
                  ariaLabel={t('settings.tokenSaver.cavemanTitle')}
                  checked={!!local().cavemanEnabled}
                  onChange={v => {
                    set('cavemanEnabled', v)
                    if (v && !local().cavemanLevel) set('cavemanLevel', 'lite')
                  }}
                />
              </div>
              <Show when={!!local().cavemanEnabled}>
                <div class="pl-3 sm:pl-4 border-l-2 border-accent/40">
                  <Field label={t('settings.tokenSaver.compressionLevel')} hint={t('settings.cavemanHint')}>
                    <Select
                      value={String(local().cavemanLevel || 'lite')}
                      options={cavemanOptions()}
                      onChange={v => set('cavemanLevel', v)}
                      class="w-full mt-1"
                    />
                  </Field>
                </div>
              </Show>
            </div>

            {/* Ponytail 极简代码 */}
            <div class="space-y-3 pt-3 border-t border-subtle/50">
              <div class="flex items-start justify-between gap-4">
                <Field label={t('settings.tokenSaver.ponytailTitle')} hint={t('settings.tokenSaver.ponytailHint')}>
                  <span />
                </Field>
                <Toggle
                  ariaLabel={t('settings.tokenSaver.ponytailTitle')}
                  checked={!!local().ponytailEnabled}
                  onChange={v => {
                    set('ponytailEnabled', v)
                    if (v && !local().ponytailLevel) set('ponytailLevel', 'lite')
                  }}
                />
              </div>
              <Show when={!!local().ponytailEnabled}>
                <div class="pl-3 sm:pl-4 border-l-2 border-accent/40">
                  <Field label={t('settings.tokenSaver.compressionLevel')} hint={t('settings.ponytailHint')}>
                    <Select
                      value={String(local().ponytailLevel || 'lite')}
                      options={ponytailOptions()}
                      onChange={v => set('ponytailLevel', v)}
                      class="w-full mt-1"
                    />
                  </Field>
                </div>
              </Show>
            </div>
          </div>

          {/* 排除名单模块 */}
          <div class="pt-4 border-t border-subtle/50 space-y-3">
            <div>
              <Field
                label={t('settings.tokenSaver.exclusionsTitle')}
                hint={t('settings.tokenSaver.exclusionsHint')}
              >
                <span />
              </Field>
            </div>

            <div class="flex flex-wrap items-center gap-1.5 min-w-0 min-h-[38px] py-1">
              <Show
                when={excludedProviders().length > 0}
                fallback={<Empty message={t('settings.tokenSaver.noExclusions')} class="py-2! text-left!" />}
              >
                <For each={excludedProviders()}>
                  {p => (
                    <span class="inline-flex max-w-full min-w-0 items-center gap-1.5 px-2 py-0.5 rounded bg-control text-foreground text-xs font-mono wrap-anywhere">
                      <span>{p}</span>
                      <IconButton
                        size="sm"
                        variant="ghost"
                        type="button"
                        class="text-muted hover:text-danger"
                        title={t('settings.removeExclude')}
                        aria-label={t('settings.removeExclude')}
                        onClick={() => removeExcludeProvider(p)}
                      >
                        &times;
                      </IconButton>
                    </span>
                  )}
                </For>
              </Show>
            </div>

            {/* 输入框与添加按钮 */}
            <div class="grid grid-cols-1 sm:grid-cols-[minmax(0,1fr)_auto] items-center gap-2">
              <Input
                placeholder={t('settings.excludePlaceholder')}
                class="text-xs"
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
                {t('common.add')}
              </Button>
            </div>

            <Show when={quickSuggestions().length > 0}>
              <div class="flex flex-wrap items-center gap-1 text-xs text-faint">
                <span>{t('settings.tokenSaver.quickAddConfigured')}</span>
                <For each={quickSuggestions()}>
                  {name => (
                    <Button
                      size="sm"
                      variant="secondary"
                      type="button"
                      class="max-w-full min-w-0! h-auto! min-h-8 py-2 whitespace-normal! wrap-anywhere font-mono"
                      onClick={() => addExcludeProvider(name)}
                    >
                      +{name}
                    </Button>
                  )}
                </For>
              </div>
            </Show>
          </div>
        </Card>
              </div>
              </LoadState>
            </Match>
            <Match when={tab === 'appearance'}>
              <div class="space-y-3.5">

        {/* 主题色与视觉风格卡片 */}
        <Card class="p-5 space-y-4">
          <div class="flex flex-wrap items-center justify-between gap-3 border-b border-subtle/50 pb-3">
            <div class="flex flex-wrap items-center gap-2 min-w-0">
              <IconPalette size={16} class="text-accent shrink-0" />
              <div>
                <h3 class="text-sm font-semibold">{t('settings.appearance.themeCardTitle')}</h3>
                <p class="text-xs text-faint mt-0.5">{t('settings.appearance.themeCardSubtitle')}</p>
              </div>
            </div>
            <Badge tone="blue">
              {t(themeStore.colorOptions.find(o => o.id === themeStore.color())?.nameKey ?? 'settings.appearance.themeColorIndigo')}
            </Badge>
          </div>

          <div class="space-y-4">
            {/* 明暗模式 */}
            <div class="p-4 rounded-xl bg-surface-inset space-y-2.5">
              <div class="text-xs font-semibold text-foreground flex items-center gap-1.5">
                <IconSun size={14} class="text-accent" />
                <span>{t('settings.appearance.themeModeTitle')}</span>
                <span class="text-faint font-normal">· {t('settings.appearance.themeModeSubtitle')}</span>
              </div>
              <div class="w-full sm:w-64 pt-0.5">
                <SegmentedControl
                  value={themeStore.mode()}
                  onChange={v => themeStore.setMode(v as 'dark' | 'light')}
                  options={[
                    { value: 'dark', label: t('settings.appearance.themeModeDark'), icon: <IconMoon size={14} /> },
                    { value: 'light', label: t('settings.appearance.themeModeLight'), icon: <IconSun size={14} /> },
                  ]}
                  size="sm"
                />
              </div>
            </div>

            {/* 品牌主题色选择网格 */}
            <div class="p-4 rounded-xl bg-surface-inset space-y-2.5">
              <div class="text-xs font-semibold text-foreground flex items-center justify-between gap-1.5">
                <div class="flex items-center gap-1.5">
                  <IconSparkles size={14} class="text-accent" />
                  <span>{t('settings.appearance.themeColorTitle')}</span>
                  <span class="text-faint font-normal">· {t('settings.appearance.themeColorSubtitle')}</span>
                </div>
              </div>

              <div class="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-4 gap-3 pt-0.5">
                <For each={themeStore.colorOptions}>
                  {opt => {
                    const isSelected = () => themeStore.color() === opt.id
                    return (
                      <button
                        type="button"
                        class={`group relative flex flex-col p-3 rounded-xl border text-left transition-all duration-200 cursor-pointer overflow-hidden ${
                          isSelected()
                            ? 'bg-accent/10 border-accent shadow-xs ring-1 ring-accent/30'
                            : 'bg-surface-inset hover:bg-glass border-control-border hover:border-subtle/80'
                        }`}
                        onClick={() => themeStore.setColor(opt.id)}
                      >
                        <div class="flex items-center justify-between w-full mb-2">
                          <div class="flex items-center gap-2">
                            <span
                              class="w-5 h-5 rounded-full shrink-0 shadow-xs border border-white/20 flex items-center justify-center transition-transform group-hover:scale-105"
                              style={{
                                background: `linear-gradient(135deg, ${opt.primary} 0%, ${opt.secondary} 100%)`,
                              }}
                            />
                            <span class="text-xs font-semibold text-foreground">{t(opt.nameKey)}</span>
                          </div>
                          <Show when={isSelected()}>
                            <span class="w-4 h-4 rounded-full bg-accent text-on-accent flex items-center justify-center shrink-0 animate-scale-in">
                              <IconCheck size={11} stroke-width={3} />
                            </span>
                          </Show>
                        </div>
                        <p class="text-[11px] text-faint leading-relaxed line-clamp-2">
                          {t(opt.descKey)}
                        </p>
                      </button>
                    )
                  }}
                </For>
              </div>
            </div>
          </div>
        </Card>

        {/* 界面与壁纸卡片 */}
        <Card class="p-5 space-y-4">
          <div class="flex flex-wrap items-center justify-between gap-3 border-b border-subtle/50 pb-3">
            <div class="flex flex-wrap items-center gap-2 min-w-0">
              <IconPalette size={16} class="text-accent shrink-0" />
              <div>
                <h3 class="text-sm font-semibold">{t('settings.appearance.cardTitle')}</h3>
                <p class="text-xs text-faint mt-0.5">{t('settings.appearance.cardSubtitle')}</p>
              </div>
            </div>
            <Show when={bgStore.hasCustomBg()}>
              <Button
                size="sm"
                variant="danger"
                onClick={async () => {
                  const ok = await confirm({
                    title: t('settings.appearance.clearBgConfirmTitle'),
                    message: t('settings.appearance.clearBgConfirmMessage'),
                    variant: 'danger',
                  })
                  if (!ok) return
                  await bgStore.resetWallpaper()
                  setBgUrlInput('')
                  toast.success(t('toast.resetWallpaperSuccess'))
                }}
              >
                {t('settings.clearWallpaper')}
              </Button>
            </Show>
          </div>

          <div class="grid grid-cols-1 xl:grid-cols-2 gap-4">
            {/* 远程图片链接 */}
            <div class="min-w-0 p-4 rounded-xl bg-surface-inset space-y-2.5">
              <label class="text-xs font-semibold text-foreground flex items-center gap-1.5">
                <IconDownload size={14} class="text-accent" />
                <span>{t('settings.remoteImageUrl')}</span>
              </label>
              <div class="grid grid-cols-1 sm:grid-cols-[minmax(0,1fr)_auto] items-center gap-2 pt-0.5">
                <Input
                  value={bgUrlInput()}
                  placeholder="https://..."
                  onInput={setBgUrlInput}
                  class="min-w-0"
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
                        toast.error(t('toast.remoteImageLoadFailed'))
                        return
                      }

                      let dataUrl = ''
                      try {
                        const base64 = await fetchRemoteImageDataUrl(url)
                        if (base64) dataUrl = base64
                      } catch {
                        // 跨域或安全拦截直接拉取时，降级使用 direct url 保存进 IndexedDB
                      }

                      if (!dataUrl) {
                        dataUrl = url
                      }

                      await bgStore.setWallpaper(dataUrl, { sourceType: 'remote', remoteUrl: url })
                      toast.success(t('toast.saveRemoteWallpaperSuccess'))
                    } catch {
                      toast.error(t('toast.saveRemoteWallpaperFailed'))
                    } finally {
                      setLoadingBgUrl(false)
                    }
                  }}
                >
                  {t('settings.apply')}
                </Button>
              </div>
            </div>

            {/* 本地图片上传 */}
            <div class="p-4 rounded-xl bg-surface-inset space-y-2.5">
              <label class="text-xs font-semibold text-foreground flex items-center gap-1.5">
                <IconUpload size={14} class="text-accent" />
                <span>{t('settings.localImageUpload')}</span>
              </label>
              <div class="pt-0.5">
                <FileUpload
                  compact
                  accept="image/*"
                  placeholder={t('settings.appearance.selectLocalImage')}
                  onChange={async file => {
                    if (!file) return
                    const reader = new FileReader()
                    reader.onload = async () => {
                      const dataUrl = reader.result as string
                      await bgStore.setWallpaper(dataUrl, { sourceType: 'upload' })
                      toast.success(t('toast.saveUploadWallpaperSuccess', { name: file.name }))
                    }
                    reader.readAsDataURL(file)
                  }}
                />
              </div>
            </div>
          </div>

          {/* 壁纸与毛玻璃微调控制面板（仿 zashboard 外观微调系统） */}
          <Show when={bgStore.hasCustomBg()}>
            <div class="p-4 rounded-xl bg-surface-inset space-y-3.5">
              <div class="text-xs font-semibold text-foreground flex items-center gap-1.5">
                <IconSparkles size={14} class="text-accent" />
                <span>{t('settings.appearance.appearanceSliders')}</span>
              </div>
              <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
                <Slider
                  label={t('settings.appearance.blur')}
                  min={0}
                  max={30}
                  step={1}
                  unit="px"
                  value={bgStore.config().blur || 0}
                  onChange={v => bgStore.updateConfig({ blur: v })}
                />
                <Slider
                  label={t('settings.appearance.opacity')}
                  min={0.1}
                  max={1}
                  step={0.05}
                  valueDisplay={`${Math.round((bgStore.config().opacity ?? 1) * 100)}%`}
                  value={bgStore.config().opacity ?? 1}
                  onChange={v => bgStore.updateConfig({ opacity: v })}
                />
                <Slider
                  label={t('settings.appearance.surfaceAlpha')}
                  min={0.2}
                  max={0.95}
                  step={0.02}
                  valueDisplay={`${Math.round((bgStore.config().surfaceAlpha ?? 0.62) * 100)}%`}
                  value={bgStore.config().surfaceAlpha ?? 0.62}
                  onChange={v => bgStore.updateConfig({ surfaceAlpha: v })}
                />
                <Slider
                  label={t('settings.appearance.glassBlur')}
                  min={0}
                  max={40}
                  step={2}
                  unit="px"
                  value={bgStore.config().glassBlur ?? 20}
                  onChange={v => bgStore.updateConfig({ glassBlur: v })}
                />
              </div>
            </div>
          </Show>
        </Card>
              </div>
            </Match>
            <Match when={tab === 'data'}>
              <div class="space-y-3.5">

          {/* 数据备份导出卡片 */}
          <Card class="p-5 space-y-4">
            <div class="flex flex-wrap items-center justify-between gap-3 border-b border-subtle/50 pb-3">
              <div class="flex flex-wrap items-center gap-2 min-w-0">
                <IconDownload size={16} class="text-accent shrink-0" />
                <div>
                  <h3 class="text-sm font-semibold">{t('settings.data.backupCardTitle')}</h3>
                  <p class="text-xs text-faint mt-0.5">{t('settings.data.backupCardSubtitle')}</p>
                </div>
              </div>
              <Badge tone="blue">{t('settings.data.snapshotBadge')}</Badge>
            </div>

          <div class="p-4 rounded-xl bg-surface-inset space-y-3.5">
            <Checkbox
              checked={includeSecrets()}
              onChange={setIncludeSecrets}
              label={t('settings.data.includeSecrets')}
              description={
                <div class="flex items-center gap-1.5 mt-1 text-xs">
                  <Show
                    when={includeSecrets()}
                    fallback={
                      <span class="text-faint flex items-center gap-1">
                        <IconInfo size={13} class="text-info shrink-0" />
                        <span>{t('settings.data.secretsSafe')}</span>
                      </span>
                    }
                  >
                    <span class="text-warning flex items-center gap-1">
                      <IconAlertTriangle size={13} class="text-warning shrink-0" />
                      <span>{t('settings.data.secretsWarning')}</span>
                    </span>
                  </Show>
                </div>
              }
            />

            <div class="pt-3 border-t border-subtle/40">
              <Checkbox
                checked={includeUsage()}
                onChange={setIncludeUsage}
                label={t('settings.data.includeUsage')}
                description={t('settings.data.usageDesc')}
              />
            </div>
          </div>

          <div class="pt-1 flex justify-end">
              <Button
                variant="primary"
                loading={downloading()}
                onClick={handleDownloadBackup}
                class="flex items-center gap-1.5"
              >
                <IconDownload size={14} />
                <span>{t('settings.data.downloadBackup')}</span>
              </Button>
            </div>
          </Card>

          {/* 备份恢复与导入卡片 */}
          <Card class="p-5 space-y-4">
            <div class="flex flex-wrap items-center justify-between gap-3 border-b border-subtle/50 pb-3">
              <div class="flex flex-wrap items-center gap-2 min-w-0">
                <IconUpload size={16} class="text-warning shrink-0" />
                <div>
                  <h3 class="text-sm font-semibold">{t('settings.data.restoreCardTitle')}</h3>
                  <p class="text-xs text-faint mt-0.5">{t('settings.data.restoreCardSubtitle')}</p>
                </div>
              </div>
              <Badge tone="amber">{t('settings.data.restoreBadge')}</Badge>
            </div>

          <div class="space-y-3.5 text-xs">
            <div class="p-4 rounded-xl bg-surface-inset space-y-4">
              <Field label={t('settings.backupFileLabel')} hint={t('settings.backupFileHint')}>
                <FileUpload
                  accept=".json,.cyrene.json"
                  value={restoreFile()}
                  onChange={setRestoreFile}
                  placeholder={t('common.dragOrClickFile')}
                  hint={`${t('common.supportedFormats')}: .cyrene.json, .json`}
                />
              </Field>

              <Field label={t('settings.restoreStrategyLabel')} hint={t('settings.restoreStrategyHint')}>
                <Select
                  value={restoreMode()}
                  options={[
                    { value: 'replace', label: t('settings.restoreModeReplaceLabel') },
                    { value: 'merge', label: t('settings.restoreModeMergeLabel') },
                  ]}
                  onChange={v => setRestoreMode(v as 'replace' | 'merge')}
                />
              </Field>
            </div>

            <Alert variant="warning" title={t('settings.data.importantNotice')}>
              {t('settings.restoreNotice')}
            </Alert>
          </div>

          <div class="pt-1 flex items-center justify-end">
              <Button
                variant="danger"
                disabled={!restoreFile()}
                loading={restoring()}
                onClick={handleRestoreSubmit}
                class="flex items-center gap-1.5"
              >
                <IconUpload size={14} />
                <span>{t('settings.data.startRestore')}</span>
              </Button>
            </div>
          </Card>
              </div>
            </Match>
            <Match when={tab === 'ops'}>
              <div class="space-y-4">
                {/* 1. 系统运行指标卡片 */}
                <Card class="p-5 space-y-4">
                  <div class="flex flex-wrap items-center justify-between gap-3 border-b border-subtle/50 pb-3">
                    <div class="flex flex-wrap items-center gap-2 min-w-0">
                      <IconActivity size={16} class="text-accent shrink-0" />
                      <div>
                        <h3 class="text-sm font-semibold">{t('settings.ops.statsTitle')}</h3>
                        <p class="text-xs text-faint mt-0.5">{t('settings.ops.statsSubtitle')}</p>
                      </div>
                    </div>
                    <Button
                      variant="secondary"
                      size="sm"
                      loading={loadingStats()}
                      onClick={fetchSystemStats}
                      class="flex items-center gap-1 text-xs"
                    >
                      <IconRotateCcw size={12} />
                      <span>{t('settings.ops.refreshStats')}</span>
                    </Button>
                  </div>

                  <Show
                    when={systemStats()}
                    fallback={
                      <Empty
                        title={t('settings.ops.noStatsLoaded')}
                        action={
                          <Button variant="secondary" size="sm" onClick={fetchSystemStats}>
                            {t('settings.ops.refreshStats')}
                          </Button>
                        }
                      />
                    }
                  >
                    {stats => (
                      <div class="space-y-4">
                        {/* 状态徽标与基础环境 */}
                        <div class="grid grid-cols-2 sm:grid-cols-4 gap-3">
                          <div class="p-3.5 rounded-xl bg-surface-inset space-y-1 hover:bg-hover transition-colors">
                            <span class="text-xs text-faint font-medium">{t('settings.ops.osArch')}</span>
                            <div class="text-xs font-mono font-semibold text-foreground truncate">
                              {stats().os} / {stats().arch}
                            </div>
                          </div>
                          <div class="p-3.5 rounded-xl bg-surface-inset space-y-1 hover:bg-hover transition-colors">
                            <span class="text-xs text-faint font-medium">{t('settings.ops.uptime')}</span>
                            <div class="text-xs font-mono font-semibold text-foreground">
                              {formatUptime(stats().uptimeSeconds, uptimeUnits())}
                            </div>
                          </div>
                          <div class="p-3.5 rounded-xl bg-surface-inset space-y-1 hover:bg-hover transition-colors">
                            <span class="text-xs text-faint font-medium">{t('settings.ops.pid')} / CPU</span>
                            <div class="text-xs font-mono font-semibold text-foreground">
                              PID {stats().pid} · {stats().numCPU} Cores
                            </div>
                          </div>
                          <div class="p-3.5 rounded-xl bg-surface-inset space-y-1 hover:bg-hover transition-colors">
                            <span class="text-xs text-faint font-medium">{t('settings.ops.goroutines')}</span>
                            <div class="text-xs font-mono font-semibold text-foreground">
                              {stats().goroutines}
                            </div>
                          </div>
                        </div>

                        {/* 内存与存储仪表 */}
                        <div class="grid grid-cols-1 sm:grid-cols-2 gap-3 pt-3 border-t border-subtle/50">
                          {/* 内存 */}
                          <div class="p-4 rounded-xl bg-surface-inset space-y-3">
                            <div class="flex items-center justify-between text-xs font-semibold">
                              <span class="flex items-center gap-1.5 text-accent">
                                <IconServer size={15} />
                                <span>{t('settings.ops.goRuntimeMemory')}</span>
                              </span>
                              <Badge tone="blue">GC: {stats().memory.numGC}</Badge>
                            </div>
                            <div class="grid grid-cols-2 gap-3 text-xs pt-0.5">
                              <div class="space-y-0.5">
                                <span class="text-xs text-faint block">{t('settings.ops.memoryAlloc')}</span>
                                <span class="font-mono font-semibold text-foreground">{formatBytes(stats().memory.allocBytes)}</span>
                              </div>
                              <div class="space-y-0.5">
                                <span class="text-xs text-faint block">{t('settings.ops.memoryInuse')}</span>
                                <span class="font-mono font-semibold text-foreground">{formatBytes(stats().memory.heapInuseBytes)}</span>
                              </div>
                              <div class="space-y-0.5">
                                <span class="text-xs text-faint block">{t('settings.ops.memorySys')}</span>
                                <span class="font-mono font-semibold text-foreground">{formatBytes(stats().memory.sysBytes)}</span>
                              </div>
                              <div class="space-y-0.5">
                                <span class="text-xs text-faint block">{t('settings.ops.memoryTotalAlloc')}</span>
                                <span class="font-mono font-semibold text-foreground">{formatBytes(stats().memory.totalAllocBytes)}</span>
                              </div>
                            </div>
                          </div>

                          {/* SQLite 存储 */}
                          <div class="p-4 rounded-xl bg-surface-inset space-y-3">
                            <div class="flex items-center justify-between text-xs font-semibold">
                              <span class="flex items-center gap-1.5 text-warning">
                                <IconDatabase size={15} />
                                <span>{t('settings.ops.sqlitePhysicalStorage')}</span>
                              </span>
                              <Badge tone={stats().inDocker ? 'amber' : 'green'}>
                                {stats().inDocker ? t('settings.ops.inDockerBadge') : t('settings.ops.nativeBadge')}
                              </Badge>
                            </div>
                            <div class="grid grid-cols-2 gap-3 text-xs pt-0.5">
                              <div class="space-y-0.5">
                                <span class="text-xs text-faint block">{t('settings.ops.dbSize')}</span>
                                <span class="font-mono font-semibold text-foreground">{formatBytes(stats().storage?.dbSizeBytes ?? 0)}</span>
                              </div>
                              <div class="space-y-0.5">
                                <span class="text-xs text-faint block">{t('settings.ops.walSize')}</span>
                                <span class="font-mono font-semibold text-foreground">{formatBytes(stats().storage?.walSizeBytes ?? 0)}</span>
                              </div>
                              <div class="space-y-0.5">
                                <span class="text-xs text-faint block">{t('settings.ops.totalRequests')}</span>
                                <span class="font-mono font-semibold text-foreground">{stats().storage?.totalRequests ?? 0}</span>
                              </div>
                              <div class="space-y-0.5">
                                <span class="text-xs text-faint block">{t('settings.ops.totalDetails')}</span>
                                <span class="font-mono font-semibold text-foreground">{stats().storage?.totalDetails ?? 0}</span>
                              </div>
                            </div>
                          </div>
                        </div>
                      </div>
                    )}
                  </Show>
                </Card>

                {/* 2. 在线自更新卡片 */}
                <Card class="p-5 space-y-4">
                  <div class="flex flex-wrap items-center justify-between gap-3 border-b border-subtle/50 pb-3">
                    <div class="flex flex-wrap items-center gap-2 min-w-0">
                      <IconDownload size={16} class="text-accent shrink-0" />
                      <div>
                        <h3 class="text-sm font-semibold">{t('settings.ops.updateTitle')}</h3>
                        <p class="text-xs text-faint mt-0.5">{t('settings.ops.updateSubtitle')}</p>
                      </div>
                    </div>
                    <div class="flex flex-wrap items-center gap-2 min-w-0">
                      <Button
                        variant="secondary"
                        size="sm"
                        loading={checkingUpdate()}
                        onClick={handleCheckUpdate}
                        class="flex items-center gap-1 text-xs"
                      >
                        <IconRotateCcw size={12} />
                        <span>{t('settings.ops.checkUpdate')}</span>
                      </Button>
                    </div>
                  </div>

                  <Show
                    when={!systemStats()?.inDocker}
                    fallback={
                      <Alert variant="warning">{t('settings.ops.dockerNotice')}</Alert>
                    }
                  >
                    <div class="space-y-3.5">
                      <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-3 p-4 rounded-xl bg-surface-inset">
                        <div class="space-y-1">
                          <div class="flex flex-wrap items-center gap-2 min-w-0">
                            <span class="text-xs text-faint">{t('settings.ops.currentVersion')}:</span>
                            <span class="font-mono text-xs font-semibold">v{store.version()}</span>
                            <Show when={updateInfo()}>
                              <Show
                                when={updateInfo()?.hasUpdate}
                                fallback={<Badge tone="green">{t('settings.ops.upToDateBadge')}</Badge>}
                              >
                                <Badge tone="amber">{t('settings.ops.hasUpdateBadge')} v{updateInfo()?.latestVersion}</Badge>
                              </Show>
                            </Show>
                          </div>
                          <Show when={updateInfo()?.hasUpdate}>
                            <p class="text-xs text-faint">
                              {updateInfo()?.assetName} ({formatBytes(updateInfo()?.assetSize ?? 0)}) · {updateInfo()?.publishedAt?.slice(0, 10)}
                            </p>
                          </Show>
                        </div>

                        <div class="flex items-center gap-2 shrink-0">
                          <Button
                            variant="secondary"
                            size="sm"
                            onClick={handleRollback}
                            class="text-xs text-muted"
                          >
                            {t('settings.ops.rollbackBtn')}
                          </Button>
                          <Show when={updateInfo()?.hasUpdate}>
                            <Button
                              variant="primary"
                              size="sm"
                              loading={applyingUpdate()}
                              onClick={handleApplyUpdate}
                              class="text-xs"
                            >
                              {t('settings.ops.updateNow')}
                            </Button>
                          </Show>
                        </div>
                      </div>

                      <Show when={updateInfo()?.hasUpdate && updateInfo()?.releaseNotes}>
                        <div class="p-3.5 rounded-xl bg-code-bg border border-subtle text-xs font-mono text-muted max-h-40 overflow-y-auto whitespace-pre-wrap wrap-anywhere leading-relaxed">
                          {updateInfo()?.releaseNotes}
                        </div>
                      </Show>
                    </div>
                  </Show>
                </Card>

                {/* 3. 服务控制与平滑重启 */}
                <Card class="p-5 space-y-4">
                  <div class="flex flex-wrap items-center justify-between gap-3 border-b border-subtle/50 pb-3">
                    <div class="flex flex-wrap items-center gap-2 min-w-0">
                      <IconRotateCcw size={16} class="text-warning shrink-0" />
                      <div>
                        <h3 class="text-sm font-semibold">{t('settings.ops.restartTitle')}</h3>
                        <p class="text-xs text-faint mt-0.5">{t('settings.ops.restartSubtitle')}</p>
                      </div>
                    </div>
                  </div>

                  <div class="p-4 rounded-xl bg-surface-inset flex flex-col sm:flex-row sm:items-center justify-between gap-4">
                    <div class="text-xs text-faint leading-relaxed max-w-xl">
                      {t('settings.ops.restartDesc')}
                    </div>
                    <Button
                      variant="danger"
                      disabled={systemStats()?.inDocker}
                      onClick={handleTriggerRestart}
                      class="flex items-center gap-1.5 shrink-0"
                    >
                      <IconRotateCcw size={14} />
                      <span>{t('settings.ops.restartBtn')}</span>
                    </Button>
                  </div>
                </Card>

                {/* 4. 存储与数据库维护 */}
                <Card class="p-5 space-y-4">
                  <div class="flex flex-wrap items-center justify-between gap-3 border-b border-subtle/50 pb-3">
                    <div class="flex flex-wrap items-center gap-2 min-w-0">
                      <IconDatabase size={16} class="text-accent shrink-0" />
                      <div>
                        <h3 class="text-sm font-semibold">{t('settings.ops.maintTitle')}</h3>
                        <p class="text-xs text-faint mt-0.5">{t('settings.ops.maintSubtitle')}</p>
                      </div>
                    </div>
                  </div>

                  <div class="space-y-3.5">
                    {/* Checkpoint */}
                    <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-3 p-4 rounded-xl bg-surface-inset hover:bg-hover transition-colors">
                      <div class="space-y-0.5">
                        <div class="text-xs font-semibold text-foreground">{t('settings.ops.checkpointBtn')}</div>
                        <div class="text-xs text-faint">{t('settings.ops.checkpointDesc')}</div>
                      </div>
                      <Button
                        variant="secondary"
                        size="sm"
                        loading={runningMaint() === 'checkpoint'}
                        onClick={() => handleMaintenance('checkpoint')}
                        class="shrink-0 text-xs"
                      >
                        {t('settings.ops.checkpointBtn')}
                      </Button>
                    </div>

                    {/* Vacuum */}
                    <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-3 p-4 rounded-xl bg-surface-inset hover:bg-hover transition-colors">
                      <div class="space-y-0.5">
                        <div class="text-xs font-semibold text-foreground">{t('settings.ops.vacuumBtn')}</div>
                        <div class="text-xs text-faint">{t('settings.ops.vacuumDesc')}</div>
                      </div>
                      <Button
                        variant="secondary"
                        size="sm"
                        loading={runningMaint() === 'vacuum'}
                        onClick={() => handleMaintenance('vacuum')}
                        class="shrink-0 text-xs"
                      >
                        {t('settings.ops.vacuumBtn')}
                      </Button>
                    </div>

                    {/* Prune Logs */}
                    <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-3 p-4 rounded-xl bg-surface-inset hover:bg-hover transition-colors">
                      <div class="min-w-0 space-y-1">
                        <div class="text-xs font-semibold text-foreground">{t('settings.ops.pruneBtn')}</div>
                        <div class="text-xs text-faint">{t('settings.ops.pruneDesc')}</div>
                        <div class="flex flex-wrap items-center gap-2 pt-1">
                          <span class="text-xs text-muted">{t('settings.ops.retentionDaysLabel')}:</span>
                          <Select
                            options={[7, 14, 30, 60, 90].map(n => ({ value: String(n), label: t('settings.ops.days', { n }) }))}
                            value={String(pruneDays())}
                            onChange={v => setPruneDays(Number(v))}
                            class="w-24 shrink-0 text-xs"
                          />
                        </div>
                      </div>
                      <Button
                        variant="danger"
                        size="sm"
                        loading={runningMaint() === 'prune_logs'}
                        onClick={() => handleMaintenance('prune_logs')}
                        class="shrink-0 text-xs"
                      >
                        {t('settings.ops.pruneBtn')}
                      </Button>
                    </div>
                  </div>
                </Card>
              </div>
            </Match>
          </Switch>
        )}
      </TabTransition>

      {/* ── 底部通用系统与存储信息 ── */}
      <Card class="p-4 text-xs text-faint">
        <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-3">
          <div class="flex flex-wrap items-center gap-2.5 min-w-0">
            <div class="w-6 h-6 rounded-lg bg-accent/10 border border-accent/20 flex items-center justify-center shrink-0">
              <IconInfo size={14} class="text-accent" />
            </div>
            <span class="font-semibold text-foreground whitespace-nowrap">Cyrene Gateway</span>
            <span
              class="font-mono text-xs px-2 py-0.5 rounded-full bg-control text-muted border border-subtle whitespace-nowrap"
              title={`${t('settings.fullVersion')}: v${store.version() || 'dev'}`}
            >
              v{formatVersion(store.version())}
            </span>
          </div>
          <div class="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-faint sm:justify-end">
            <div class="flex items-center gap-1 whitespace-nowrap">
              <StatusPulse status={store.health().db === 'ok' ? 'active' : 'paused'} tone={store.health().db === 'ok' ? 'green' : 'amber'} size="xs" />
              <span>{t('common.database')}:</span>
              <span class="font-medium text-foreground">{store.health().db === 'ok' ? `${t('common.ready')} (WAL)` : t('common.checking')}</span>
            </div>
            <span class="text-subtle select-none hidden sm:inline">•</span>
            <div class="flex items-center gap-1 whitespace-nowrap">
              <span>{t('common.activeConns')}:</span>
              <span class="font-medium font-mono text-foreground">{store.activeConnections()} / {store.providers().length}</span>
            </div>
            <span class="text-subtle select-none hidden sm:inline">•</span>
            <div class="flex items-center gap-1 whitespace-nowrap">
              <span>{t('common.uptime')}:</span>
              <span class="font-medium font-mono text-foreground">{formatUptime(Number(store.health().uptimeSeconds), uptimeUnits())}</span>
            </div>
          </div>
        </div>
      </Card>

      {/* ── 重启全屏探活与重连遮罩 ── */}
      <Show when={restarting()}>
        <div class="fixed inset-0 z-50 flex items-center justify-center bg-overlay/60 backdrop-blur-sm p-4 animate-fade-in">
          <Card class="max-w-md w-full p-6 text-center space-y-4 shadow-glass-hover border border-accent/30">
            <div class="w-12 h-12 rounded-full bg-accent/10 border border-accent/20 flex items-center justify-center mx-auto text-accent">
              <IconRotateCcw size={24} class="animate-spin" />
            </div>
            <div class="space-y-1.5">
              <h3 class="text-base font-semibold">{t('settings.ops.restartingModalTitle')}</h3>
              <p class="text-xs text-faint leading-relaxed">{t('settings.ops.restartingModalDesc')}</p>
            </div>
            <div class="flex items-center justify-center gap-2 text-xs font-mono text-accent pt-1">
              <span class="inline-block w-2 h-2 rounded-full bg-accent animate-ping" />
              <span>{t('settings.ops.reconnecting')}</span>
            </div>
          </Card>
        </div>
      </Show>
      {/* 浮动未保存操作底栏 */}
      <UnsavedChangesBar
        show={activeTab() === 'gateway' && dirty()}
        loading={saving()}
        onSave={save}
        onReset={() => setLocal({ ...(store.settings() || {}) })}
      />
    </div>
  )
}

export default Settings
