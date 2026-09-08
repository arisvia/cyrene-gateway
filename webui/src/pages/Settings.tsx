import { type Component, For, Show, createSignal, onMount } from 'solid-js'
import { useGatewayStore } from '@/stores/gateway'
import { useBackgroundStore } from '@/stores/background'
import { Card, Badge, Button, Input, Select, Toggle, Field, confirm } from '@/components/ui'
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
  // 背景自定义状态
  const [bgUrlInput, setBgUrlInput] = createSignal('')
  // 响应缓存状态
  const [cacheStats, setCacheStats] = createSignal<CacheStats | null>(null)
  const [clearingCache, setClearingCache] = createSignal(false)
  const [refreshingStats, setRefreshingStats] = createSignal(false)

  // TokenSaver 排除项状态
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
    if (bgStore.bgConfig().type === 'url') {
      setBgUrlInput(bgStore.bgConfig().value)
    }
    await fetchCacheStats()
  })

  const dirty = () => {
    const orig = store.settings()
    const keys = new Set([...Object.keys(local()), ...Object.keys(orig)])
    for (const k of keys) {
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
      await store.saveSettings(local())
      setLocal({ ...store.settings() })
      toast.success('设置已保存')
    } catch (e: unknown) {
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
    } catch (e: unknown) {
      toast.error(e instanceof Error ? e.message : '密码更新失败')
    }
  }

  return (
    <div class="space-y-5 stagger">
      <div class="sticky top-16 z-20 bg-bg/90 backdrop-blur-md pt-1 pb-3 flex items-center justify-between gap-3 border-b border-subtle/50">
        <div>
          <h1 class="text-xl font-semibold">设置</h1>
          <p class="text-sm text-faint mt-0.5">网关运行参数与访问控制</p>
        </div>
        <Button variant="primary" loading={saving()} disabled={!dirty()} onClick={save}>
          {dirty() ? '保存修改' : '全部已保存'}
        </Button>
      </div>
      {/* 界面与背景自定义 */}
      <Card class="p-5 space-y-4">
        <div class="flex items-center justify-between">
          <div>
            <h3 class="text-sm font-semibold">界面与壁纸</h3>
            <p class="text-xs text-faint mt-0.5">支持上传本地图片或远程图片链接作为网关背景，数据存储于浏览器 IndexedDB</p>
          </div>
          <Show when={bgStore.bgConfig().type !== 'none'}>
            <Button
              size="sm"
              variant="danger"
              onClick={async () => {
                const ok = await confirm({
                  title: '清除自定义壁纸',
                  message: '确定要清除当前壁纸并恢复默认纯净背景吗？',
                  variant: 'danger',
                })
                if (!ok) return
                await bgStore.resetBackground()
                setBgUrlInput('')
                toast.success('已恢复默认背景')
              }}
            >
              清除壁纸
            </Button>
          </Show>
        </div>

        <div class="grid grid-cols-1 md:grid-cols-2 gap-4 pt-1">
          {/* 远程图片链接 */}
          <div class="space-y-2">
            <label class="text-xs font-medium text-muted">远程图片 URL</label>
            <div class="flex gap-2">
              <Input
                value={bgUrlInput()}
                placeholder="https://example.com/wallpaper.jpg"
                onInput={setBgUrlInput}
              />
              <Button
                variant="secondary"
                disabled={!bgUrlInput().trim()}
                onClick={async () => {
                  const url = bgUrlInput().trim()
                  if (!url) return
                  await bgStore.setBackground({
                    type: 'url',
                    value: url,
                    blur: bgStore.bgConfig().blur ?? 0,
                    opacity: bgStore.bgConfig().opacity ?? 1,
                  })
                  toast.success('已应用远程壁纸')
                }}
              >
                应用
              </Button>
            </div>
          </div>

          {/* 本地图片上传 */}
          <div class="space-y-2">
            <label class="text-xs font-medium text-muted">本地图片上传</label>
            <div class="flex items-center gap-2">
              <label class="flex-1 cursor-pointer flex items-center justify-center gap-2 px-3 py-2 rounded-control border border-dashed border-subtle hover:border-accent text-xs text-muted hover:text-text transition-colors bg-card/40">
                <svg class="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                  <rect width="18" height="18" x="3" y="3" rx="2" ry="2" />
                  <circle cx="9" cy="9" r="2" />
                  <path d="m21 15-3.086-3.086a2 2 0 0 0-2.828 0L6 21" />
                </svg>
                <span>选择本地图片...</span>
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
                      await bgStore.setBackground({
                        type: 'image',
                        value: dataUrl,
                        blur: bgStore.bgConfig().blur ?? 0,
                        opacity: bgStore.bgConfig().opacity ?? 1,
                      })
                      toast.success(`已加载本地图片 (${file.name})`)
                    }
                    reader.readAsDataURL(file)
                  }}
                />
              </label>
            </div>
          </div>
        </div>

        {/* 壁纸虚化与透明度微调 */}
        <Show when={bgStore.bgConfig().type !== 'none'}>
          <div class="grid grid-cols-1 sm:grid-cols-2 gap-4 pt-2 border-t border-subtle/50">
            <div class="space-y-1">
              <div class="flex justify-between text-xs">
                <span class="text-muted">背景虚化 (Blur)</span>
                <span class="font-mono text-faint">{bgStore.bgConfig().blur || 0}px</span>
              </div>
              <input
                type="range"
                min="0"
                max="30"
                step="1"
                class="w-full accent-accent cursor-pointer"
                value={bgStore.bgConfig().blur || 0}
                onInput={async e => {
                  const val = Number(e.currentTarget.value)
                  await bgStore.setBackground({
                    ...bgStore.bgConfig(),
                    blur: val,
                  })
                }}
              />
            </div>

            <div class="space-y-1">
              <div class="flex justify-between text-xs">
                <span class="text-muted">背景不透明度 (Opacity)</span>
                <span class="font-mono text-faint">{Math.round((bgStore.bgConfig().opacity ?? 1) * 100)}%</span>
              </div>
              <input
                type="range"
                min="0.1"
                max="1"
                step="0.05"
                class="w-full accent-accent cursor-pointer"
                value={bgStore.bgConfig().opacity ?? 1}
                onInput={async e => {
                  const val = Number(e.currentTarget.value)
                  await bgStore.setBackground({
                    ...bgStore.bgConfig(),
                    opacity: val,
                  })
                }}
              />
            </div>
          </div>
        </Show>


      </Card>

      {/* 访问控制 */}
      <Card class="p-5 space-y-4">
        <h3 class="text-sm font-semibold">访问控制</h3>

        <div class="flex items-start justify-between gap-4">
          <Field label="要求登录" hint="开启后管理面板需密码登录">
            <span />
          </Field>
          <Toggle checked={!!local().requireLogin} onChange={v => set('requireLogin', v)} />
        </div>

        <div class="flex items-start justify-between gap-4">
          <Field label="要求 API Key" hint="开启后 /v1/* 请求必须携带有效 Key">
            <span />
          </Field>
          <Toggle checked={!!local().requireApiKey} onChange={v => set('requireApiKey', v)} />
        </div>

        <Field label="API Key 速率限制" hint="单个 Key 每分钟请求上限，0 表示不限">
          <Input
            type="number"
            class="!w-32"
            value={String(local().apiKeyRpm ?? 0)}
            onInput={v => set('apiKeyRpm', Number(v) || 0)}
          />
        </Field>
      </Card>

      {/* 响应精确缓存 */}
      <Card class="p-5 space-y-4">
        <div class="flex items-center justify-between border-b border-subtle/50 pb-3">
          <div class="flex items-center gap-2.5">
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

        {/* 缓存指标数据小横条 */}
        <div class="grid grid-cols-2 sm:grid-cols-4 gap-3 bg-subtle/20 p-3 rounded-control border border-subtle/40 text-xs">
          <div>
            <span class="text-faint block">命中率</span>
            <span class="font-semibold text-foreground text-sm">
              {((cacheStats()?.hitRate ?? 0) * 100).toFixed(1)}%
            </span>
            <span class="text-[10px] text-muted block mt-0.5">
              {cacheStats()?.hits ?? 0} 命中 / {cacheStats()?.misses ?? 0} 未命中
            </span>
          </div>
          <div>
            <span class="text-faint block">累计节省 Token</span>
            <span class="font-semibold text-accent text-sm">
              {(cacheStats()?.tokensSaved ?? 0).toLocaleString()}
            </span>
            <span class="text-[10px] text-muted block mt-0.5">直出避免消耗</span>
          </div>
          <div>
            <span class="text-faint block">在存条目</span>
            <span class="font-semibold text-foreground text-sm">
              {cacheStats()?.entries ?? 0} / {cacheStats()?.maxEntries ?? 1000}
            </span>
            <span class="text-[10px] text-muted block mt-0.5">LRU 内存置换池</span>
          </div>
          <div>
            <span class="text-faint block">内存占用</span>
            <span class="font-semibold text-foreground text-sm">
              {formatBytes(cacheStats()?.bytesUsed ?? 0)}
            </span>
            <span class="text-[10px] text-muted block mt-0.5">单条 ≤2 MiB 保护</span>
          </div>
        </div>

        {/* 控制项 */}
        <div class="flex items-start justify-between gap-4 pt-1">
          <Field label="启用响应精确缓存" hint="对完全一致的对话请求（temperature=0、相同消息与模型）直接直出缓存，1ms 返回且 0 Token 消耗">
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
            <Field label="缓存有效期 (TTL)" hint="缓存条目的生存时间（秒），默认 3600 秒（1 小时），过期自动剔除">
              <Input
                type="number"
                class="!w-36"
                value={String(local().responseCacheTTL ?? 3600)}
                onInput={v => set('responseCacheTTL', Math.max(1, Number(v) || 3600))}
              />
            </Field>
            <div class="flex items-start justify-between gap-4 pt-1">
              <Field label="缓存全部非流式请求" hint="开启后不论 temperature 为何值均全量缓存；关闭则仅精确缓存确定性请求（temperature=0 与 embeddings）">
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

      {/* 令牌节省引擎 */}
      <Card class="p-5 space-y-4">
        <div class="flex items-center justify-between border-b border-subtle/50 pb-3">
          <div class="flex items-center gap-2.5">
            <h3 class="text-sm font-semibold">令牌节省引擎</h3>
            <Badge tone="gray">RTK · Caveman · Ponytail</Badge>
          </div>
        </div>

        {/* RTK 压缩 */}
        <div class="flex items-start justify-between gap-4">
          <Field label="RTK 压缩" hint="无损清洗工具输出（ANSI 剥离、JSON 紧凑化、空行折叠）与超长结果首尾截断（保留前 120 行与后 60 行，避免大文件淹没上下文）">
            <span />
          </Field>
          <Toggle checked={!!local().rtkEnabled} onChange={v => set('rtkEnabled', v)} />
        </div>

        {/* Caveman 极简表达 */}
        <div class="space-y-3 pt-2 border-t border-subtle/50">
          <div class="flex items-start justify-between gap-4">
            <Field label="Caveman 极简表达" hint="注入极简表达指令（洞穴人模式），压制寒暄客套，最大化削减模型回复 Token 消耗">
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
            <div class="pl-4 border-l-2 border-primary/30">
              <Field label="压缩级别" hint="lite: 保留要点 | full: 极限简洁 | ultra: 绝不废话 | wenyan: 文言风格">
                <Select
                  value={String(local().cavemanLevel || 'lite')}
                  options={cavemanOptions}
                  onChange={v => set('cavemanLevel', v)}
                />
              </Field>
            </div>
          </Show>
        </div>

        {/* Ponytail 极简代码 */}
        <div class="space-y-3 pt-2 border-t border-subtle/50">
          <div class="flex items-start justify-between gap-4">
            <Field label="Ponytail 极简代码" hint="注入极简代码规范指令（马尾模式），推崇 YAGNI，严禁过度设计与冗余样板代码">
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
            <div class="pl-4 border-l-2 border-primary/30">
              <Field label="压缩级别" hint="lite: 提供最简替代方案 | full: 严格执行阶梯原则 | ultra: 极致单行与挑战需求">
                <Select
                  value={String(local().ponytailLevel || 'lite')}
                  options={ponytailOptions}
                  onChange={v => set('ponytailLevel', v)}
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
              hint="指定跳过 RTK 压缩与 Caveman/Ponytail 提示词注入的提供商（例如保护遵循能力强的深度推理模型）"
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

          <div class="flex items-center gap-2">
            <Input
              placeholder="输入提供商名称（如 deepseek）后回车或点击添加"
              class="flex-1 text-xs"
              value={excludeInput()}
              onInput={setExcludeInput}
              onKeyDown={e => {
                if (e.key === 'Enter') {
                  e.preventDefault()
                  addExcludeProvider(excludeInput())
                }
              }}
            />
            <Button size="sm" variant="secondary" onClick={() => addExcludeProvider(excludeInput())}>
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

      {/* 改密码 */}
      <Card class="p-5 space-y-3">
        <h3 class="text-sm font-semibold">管理密码</h3>
        <Field label="新密码" hint="至少 8 位">
          <Input type="password" value={pw()} onInput={setPw} placeholder="••••••••" class="!w-64" />
        </Field>
        <div class="flex items-center gap-2">
          <Button variant="secondary" disabled={pw().length < 8} onClick={changePassword}>更新密码</Button>
        </div>
      </Card>
      {/* 版本 */}
      <Card class="p-5">
        <div class="flex items-center justify-between text-xs text-faint">
          <span>Cyrene Gateway</span>
          <Badge tone="gray">v{store.version()}</Badge>
        </div>
      </Card>
    </div>
  )
}

export default Settings
