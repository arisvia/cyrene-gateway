import { type Component, For, Show, createSignal, createEffect, onMount } from 'solid-js'
import { useGatewayStore } from '@/stores/gateway'
import { useBackgroundStore } from '@/stores/background'
import { fetchRemoteImageDataUrl } from '@/lib/backgroundStore'
import {
  Card, Badge, Button, Input, Select, Toggle, Field, confirm, PageHeader, SegmentedControl, Checkbox, Slider, FileUpload, StatusPulse,
  IconLock, IconKey, IconShield, IconZap, IconSparkles, IconPalette, IconInfo,
  IconDatabase, IconDownload, IconUpload, IconAlertTriangle,
} from '@/components/ui'
import { formatUptime, formatVersion, type UptimeUnits } from '@/lib/format'
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

function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(2)} MB`
}

const Settings: Component = () => {
  const { t } = useI18n()
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
  const [local, setLocal] = createSignal<Record<string, unknown>>({})
  const [pw, setPw] = createSignal('')
  const [hasPw, setHasPw] = createSignal(false)

  // 背景自定义状态
  const [bgUrlInput, setBgUrlInput] = createSignal(bgStore.config().remoteUrl || '')
  const [loadingBgUrl, setLoadingBgUrl] = createSignal(false)
  createEffect(() => {
    const rUrl = bgStore.config().remoteUrl
    if (rUrl && !bgUrlInput()) {
      setBgUrlInput(rUrl)
    }
  })
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
    } catch (e: unknown) {
      toast.error(t('toast.cacheClearFailed'))
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
      toast.error(t('toast.saveFailed', { error: e instanceof Error ? e.message : 'error' }))
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
    } catch (e: unknown) {
      toast.error(t('toast.passwordUpdateFailed'))
    }
  }

  return (
    <div class="max-w-3xl mx-auto space-y-6 stagger">
      <PageHeader
        title={t('settings.title')}
        subtitle={t('settings.subtitle')}
        actions={
          <Show when={activeTab() === 'gateway'}>
            <Button variant="primary" loading={saving()} disabled={!dirty()} onClick={save} class="shrink-0">
              {dirty() ? t('common.save') : t('common.saved')}
            </Button>
          </Show>
        }
      >
        <div class="pt-2 border-t border-subtle/40 flex flex-col sm:flex-row sm:items-center justify-between gap-2.5">
          <SegmentedControl
            class="shrink-0"
            options={[
              { value: 'gateway', label: t('settings.tabs.gateway') },
              { value: 'appearance', label: t('settings.tabs.appearance') },
              { value: 'data', label: t('settings.tabs.data') },
            ]}
            value={activeTab()}
            onChange={handleTabChange}
          />
          <div class="text-[11px] text-faint flex items-center gap-1.5 px-0.5 min-w-0 overflow-hidden">
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
          </div>
        </div>
      </PageHeader>

      {/* ── Tab 1：网关核心设置 ── */}
      <Show when={activeTab() === 'gateway'}>

      {/* ── 分组 1：安全与访问控制 ── */}
      <div class="space-y-3.5">
        <div class="flex items-center gap-1.5 px-0.5 text-[11px] font-semibold uppercase tracking-wider text-faint">
          <IconShield size={14} class="text-accent shrink-0" />
          <span>{t('settings.access.groupTitle')}</span>
        </div>

        {/* 访问控制卡片 */}
        <Card class="p-5 space-y-4">
          <div class="flex items-center justify-between border-b border-subtle/50 pb-3">
            <div class="flex items-center gap-2">
              <IconLock size={16} class="text-accent shrink-0" />
              <h3 class="text-sm font-semibold">{t('settings.access.cardTitle')}</h3>
            </div>
            <Badge tone={local().requireLogin || local().requireApiKey ? 'blue' : 'gray'}>
              {local().requireLogin || local().requireApiKey ? t('settings.access.protected') : t('settings.access.open')}
            </Badge>
          </div>

          <div class="flex items-start justify-between gap-4">
            <Field
              label={t('settings.access.requireLogin')}
              hint={
                hasPw()
                  ? t('settings.access.requireLoginHint')
                  : t('settings.access.requireLoginNoPw')
              }
            >
              <span />
            </Field>
            <Toggle
              checked={!!local().requireLogin}
              disabled={!hasPw()}
              onChange={v => {
                if (v && !hasPw()) {
                  toast.warning(t('toast.setAdminPasswordFirst'))
                  return
                }
                set('requireLogin', v)
              }}
            />
          </div>

          <div class="flex items-start justify-between gap-4 pt-1 border-t border-subtle/50">
            <Field label={t('settings.access.requireApiKey')} hint={t('settings.access.requireApiKeyHint')}>
              <span />
            </Field>
            <Toggle checked={!!local().requireApiKey} onChange={v => set('requireApiKey', v)} />
          </div>

          <div class="pt-1 border-t border-subtle/50">
            <Field label={t('settings.access.rpmLimit')} hint={t('settings.access.rpmLimitHint')}>
              <Input
                type="number"
                class="w-full! sm:w-36! mt-1"
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
              <h3 class="text-sm font-semibold">{t('settings.access.managePassword')}</h3>
            </div>
            <Badge tone={hasPw() ? 'green' : 'amber'}>
              {hasPw() ? t('settings.access.pwConfigured') : t('settings.access.pwUnset')}
            </Badge>
          </div>
          <Field label={t('settings.access.changePassword')} hint={t('settings.access.pwHelpText')}>
            <div class="flex flex-col sm:flex-row items-stretch sm:items-center gap-2.5 pt-1">
              <Input
                type="password"
                value={pw()}
                onInput={setPw}
                placeholder={t('settings.pwPlaceholder')}
                class="flex-1 w-full!"
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
        </Card>
      </div>

      {/* ── 分组 2：效能与节省引擎 ── */}
      <div class="space-y-3.5">
        <div class="flex items-center gap-1.5 px-0.5 pt-2 text-[11px] font-semibold uppercase tracking-wider text-faint">
          <IconZap size={14} class="text-accent shrink-0" />
          <span>{t('settings.cache.groupTitle')}</span>
        </div>

        {/* 响应精确缓存卡片 */}
        <Card class="p-5 space-y-4">
          <div class="flex items-center justify-between border-b border-subtle/50 pb-3">
            <div class="flex items-center gap-2">
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
          <div class="grid grid-cols-2 sm:grid-cols-4 gap-2.5 bg-subtle/20 p-3 rounded-control border border-subtle/40 text-xs">
            <div>
              <span class="text-faint block text-[11px]">{t('settings.cache.hitRate')}</span>
              <span class="font-semibold text-foreground text-sm">
                {((cacheStats()?.hitRate ?? 0) * 100).toFixed(1)}%
              </span>
              <span class="text-[10px] text-muted block mt-0.5 truncate">
                {t('settings.cache.hitsAndMisses', { hits: cacheStats()?.hits ?? 0, misses: cacheStats()?.misses ?? 0 })}
              </span>
            </div>
            <div>
              <span class="text-faint block text-[11px]">{t('settings.cache.tokensSaved')}</span>
              <span class="font-semibold text-accent text-sm">
                {(cacheStats()?.tokensSaved ?? 0).toLocaleString()}
              </span>
              <span class="text-[10px] text-muted block mt-0.5 truncate">{t('settings.cache.directSavings')}</span>
            </div>
            <div>
              <span class="text-faint block text-[11px]">{t('settings.cache.entries')}</span>
              <span class="font-semibold text-foreground text-sm">
                {cacheStats()?.entries ?? 0} / {cacheStats()?.maxEntries ?? 1000}
              </span>
              <span class="text-[10px] text-muted block mt-0.5 truncate">{t('settings.cache.lruPool')}</span>
            </div>
            <div>
              <span class="text-faint block text-[11px]">{t('settings.cache.memory')}</span>
              <span class="font-semibold text-foreground text-sm">
                {formatBytes(cacheStats()?.bytesUsed ?? 0)}
              </span>
              <span class="text-[10px] text-muted block mt-0.5 truncate">{t('settings.cache.entrySizeLimit')}</span>
            </div>
          </div>

          {/* 控制项 */}
          <div class="flex items-start justify-between gap-4 pt-1">
            <Field label={t('settings.cache.enabled')} hint={t('settings.cache.enabledHint')}>
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
              <Field label={t('settings.cache.ttl')} hint={t('settings.cache.ttlHint')}>
                <Input
                  type="number"
                  class="w-full! sm:w-36! mt-1"
                  value={String(local().responseCacheTTL ?? 3600)}
                  onInput={v => set('responseCacheTTL', Math.max(1, Number(v) || 3600))}
                />
              </Field>
              <div class="flex items-start justify-between gap-4 pt-1 border-t border-subtle/50">
                <Field label={t('settings.cache.enableAllRequests')} hint={t('settings.cache.enableAllRequestsHint')}>
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
              <h3 class="text-sm font-semibold">{t('settings.tokenSaver.title')}</h3>
              <Badge tone="gray">RTK · Caveman · Ponytail</Badge>
            </div>
          </div>

          {/* RTK 压缩 */}
          <div class="flex items-start justify-between gap-4">
            <Field label={t('settings.tokenSaver.rtkTitle')} hint={t('settings.tokenSaver.rtkHint')}>
              <span />
            </Field>
            <Toggle checked={!!local().rtkEnabled} onChange={v => set('rtkEnabled', v)} />
          </div>

          {/* Caveman 极简表达 */}
          <div class="space-y-3 pt-2 border-t border-subtle/50">
            <div class="flex items-start justify-between gap-4">
              <Field label={t('settings.tokenSaver.cavemanTitle')} hint={t('settings.tokenSaver.cavemanHint')}>
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
          <div class="space-y-3 pt-2 border-t border-subtle/50">
            <div class="flex items-start justify-between gap-4">
              <Field label={t('settings.tokenSaver.ponytailTitle')} hint={t('settings.tokenSaver.ponytailHint')}>
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

          {/* 排除名单 */}
          <div class="space-y-3 pt-3 border-t border-subtle/50">
            <div>
              <Field
                label={t('settings.tokenSaver.exclusionsTitle')}
                hint={t('settings.tokenSaver.exclusionsHint')}
              >
                <span />
              </Field>
            </div>

            <div class="flex flex-wrap items-center gap-1.5 min-h-[32px] p-2 rounded-control bg-black/4 dark:bg-white/6 border border-black/10 dark:border-white/12">
              <Show
                when={excludedProviders().length > 0}
                fallback={<span class="text-xs text-faint flex items-center gap-1.5"><IconInfo size={13} class="text-faint/80 shrink-0" />{t('settings.tokenSaver.noExclusions')}</span>}
              >
                <For each={excludedProviders()}>
                  {p => (
                    <span class="inline-flex items-center gap-1.5 px-2 py-0.5 rounded bg-warning/10 text-warning text-xs font-mono">
                      <span>{p}</span>
                      <button
                        type="button"
                        class="hover:opacity-75 focus:outline-none"
                        title={t('settings.removeExclude')}
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
                placeholder={t('settings.excludePlaceholder')}
                class="flex-1 text-xs w-full!"
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
              <div class="flex flex-wrap items-center gap-1 text-[11px] text-faint">
                <span>{t('settings.tokenSaver.quickAddConfigured')}</span>
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
          <span>{t('settings.appearance.groupTitle')}</span>
        </div>

        {/* 界面与壁纸卡片 */}
        <Card class="p-5 space-y-4">
          <div class="flex items-center justify-between border-b border-subtle/50 pb-3">
            <div class="flex items-center gap-2">
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

          <div class="grid grid-cols-1 sm:grid-cols-2 gap-4 pt-1">
            {/* 远程图片链接 */}
            <div class="space-y-1.5">
              <label class="text-xs font-medium text-muted">{t('settings.remoteImageUrl')}</label>
              <div class="flex flex-col sm:flex-row gap-2">
                <Input
                  value={bgUrlInput()}
                  placeholder="https://..."
                  onInput={setBgUrlInput}
                  class="flex-1 w-full!"
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
            <div class="space-y-1.5">
              <label class="text-xs font-medium text-muted">{t('settings.localImageUpload')}</label>
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

          {/* 壁纸与毛玻璃微调控制面板（仿 zashboard 外观微调系统） */}
          <Show when={bgStore.hasCustomBg()}>
            <div class="space-y-3 pt-3 border-t border-subtle/50">
              <div class="text-xs font-semibold text-muted">{t('settings.appearance.appearanceSliders')}</div>
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
    </Show>

      {/* ── Tab 3：数据管理与备份恢复 ── */}
      <Show when={activeTab() === 'data'}>
        <div class="space-y-3.5">
          <div class="flex items-center gap-1.5 px-0.5 text-[11px] font-semibold uppercase tracking-wider text-faint">
            <IconDatabase size={14} class="text-accent shrink-0" />
            <span>{t('settings.data.groupTitle')}</span>
          </div>

          {/* 数据备份导出卡片 */}
          <Card class="p-5 space-y-4">
            <div class="flex items-center justify-between border-b border-subtle/50 pb-3">
              <div class="flex items-center gap-2">
                <IconDownload size={16} class="text-accent shrink-0" />
                <div>
                  <h3 class="text-sm font-semibold">{t('settings.data.backupCardTitle')}</h3>
                  <p class="text-xs text-faint mt-0.5">{t('settings.data.backupCardSubtitle')}</p>
                </div>
              </div>
              <Badge tone="blue">{t('settings.data.snapshotBadge')}</Badge>
            </div>

            <div class="space-y-3.5">
              <Checkbox
                checked={includeSecrets()}
                onChange={setIncludeSecrets}
                label={t('settings.data.includeSecrets')}
                description={
                  <div class="flex items-center gap-1.5 mt-1 text-[11px]">
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

            <div class="pt-2 border-t border-subtle/50 flex justify-end">
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
            <div class="flex items-center justify-between border-b border-subtle/50 pb-3">
              <div class="flex items-center gap-2">
                <IconUpload size={16} class="text-warning shrink-0" />
                <div>
                  <h3 class="text-sm font-semibold">{t('settings.data.restoreCardTitle')}</h3>
                  <p class="text-xs text-faint mt-0.5">{t('settings.data.restoreCardSubtitle')}</p>
                </div>
              </div>
              <Badge tone="amber">{t('settings.data.restoreBadge')}</Badge>
            </div>

            <div class="space-y-3.5 text-xs">
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

              <div class="p-3 bg-warning/10 border border-warning/25 rounded-control text-[11px] text-warning space-y-1">
                <div class="font-semibold flex items-center gap-1">
                  <IconAlertTriangle size={13} />
                  <span>{t('settings.data.importantNotice')}</span>
                </div>
                <p>
                  {t('settings.restoreNotice')}
                </p>
              </div>
            </div>

            <div class="pt-2 border-t border-subtle/50 flex items-center justify-between">
              <span class="text-xs text-faint font-mono">
                {restoreFile() ? t('settings.fileReady', { name: restoreFile()!.name }) : t('settings.noFileSelected')}
              </span>
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
              class="font-mono text-[11px] px-2 py-0.5 rounded-full bg-black/6 dark:bg-white/10 text-muted border border-black/10 dark:border-white/15 whitespace-nowrap"
              title={`${t('settings.fullVersion')}: v${store.version() || 'dev'}`}
            >
              v{formatVersion(store.version())}
            </span>
          </div>
          <div class="flex flex-wrap items-center gap-x-3 gap-y-1 text-[11px] text-faint sm:justify-end">
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
    </div>
  )
}

export default Settings
