import { type Component, For, Show, Switch, Match, createSignal, createEffect, on, onMount } from 'solid-js'
import DataPanel from '@/components/settings/DataPanel'
import { useGatewayStore } from '@/stores/gateway'
import AppearancePanel from '@/components/settings/AppearancePanel'
import { useSystemOps } from '@/stores/systemOps'
import SystemOpsPanel from '@/components/settings/SystemOps'
import {
  Card, Badge, Button, IconButton, Input, Select, Toggle, Field, Empty, confirm, PageHeader, SegmentedControl, StatusPulse, TabTransition, Skeleton, LoadState, UnsavedChangesBar,
  IconLock, IconKey, IconZap, IconSparkles, IconInfo,
  IconRotateCcw,
} from '@/components/ui'
import { formatUptime, formatVersion, formatBytes, uptimeUnitsFor } from '@/lib/format'
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
  const uptimeUnits = () => uptimeUnitsFor(t)
  const store = useGatewayStore()
  const { restarting } = useSystemOps()
  const toast = useToast()
  const [saving, setSaving] = createSignal(false)
  const [local, setLocal] = createSignal<Record<string, unknown>>({ ...store.settings() })
  const [initialized, setInitialized] = createSignal(store.loaded.settings)
  const [edited, setEdited] = createSignal(false)
  const [pw, setPw] = createSignal('')
  const [hasPw, setHasPw] = createSignal(!!store.settings().hasPassword)

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
    if (typeof window !== 'undefined') {
      window.scrollTo({ top: 0, behavior: 'smooth' })
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
            {/* 循环守卫 */}
            <div class="flex items-start justify-between gap-4 pt-3 border-t border-subtle/50">
              <Field label={t('settings.tokenSaver.loopGuardTitle')} hint={t('settings.tokenSaver.loopGuardHint')}>
                <span />
              </Field>
              <Toggle
                ariaLabel={t('settings.tokenSaver.loopGuardTitle')}
                checked={!!local().loopGuardEnabled}
                onChange={v => set('loopGuardEnabled', v)}
              />
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
              <AppearancePanel />
            </Match>
            <Match when={tab === 'data'}>
              <DataPanel />
            </Match>
            <Match when={tab === 'ops'}>
              <SystemOpsPanel />
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
