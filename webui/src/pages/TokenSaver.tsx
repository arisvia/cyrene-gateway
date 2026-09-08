import { type Component, For, Show, createSignal, onMount } from 'solid-js'
import { useGatewayStore } from '@/stores/gateway'
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
  if (bytes <= 0) return '0 B'
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(2)} MB`
}

function formatNumber(n: number): string {
  return new Intl.NumberFormat('zh-CN').format(n)
}

const TokenSaverPage: Component = () => {
  const store = useGatewayStore()
  const toast = useToast()
  const [saving, setSaving] = createSignal(false)
  const [local, setLocal] = createSignal<Record<string, unknown>>({})
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
    const defaults = ['deepseek', 'openai', 'anthropic', 'gemini', 'openrouter', 'groq']
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
      message: '确定要清空全部在存的响应缓存吗？清空后相同请求将重新向上游发起并消耗 Token。',
      confirmText: '立即清空',
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
      } else if (typeof lv === 'object' && lv !== null || typeof ov === 'object' && ov !== null) {
        if (JSON.stringify(lv ?? {}) !== JSON.stringify(ov ?? {})) return true
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
    } catch (e: unknown) {
      toast.error(e instanceof Error ? e.message : '保存设置失败')
    } finally {
      setSaving(false)
    }
  }

  return (
    <div class="space-y-5 stagger">
      {/* 顶部粘性栏 */}
      <div class="sticky top-16 z-20 bg-bg/90 backdrop-blur-md pt-1 pb-3 flex items-center justify-between gap-3 border-b border-subtle/50">
        <div>
          <div class="flex items-center gap-2">
            <h1 class="text-xl font-semibold">Token 节省与优化</h1>
            <Badge tone="blue">加速引擎</Badge>
          </div>
          <p class="text-sm text-faint mt-0.5">多层级请求缓存、工具输出压缩、极简表达指令与提供商保护隔离</p>
        </div>
        <div class="flex items-center gap-2">
          <Button variant="primary" loading={saving()} disabled={!dirty()} onClick={save}>
            {dirty() ? '保存更改' : '已是最新'}
          </Button>
        </div>
      </div>

      {/* 实时缓存指标行 (Stats Banner) */}
      <div class="grid grid-cols-2 sm:grid-cols-4 gap-3">
        <Card class="p-4 space-y-1">
          <div class="text-xs text-muted">缓存命中率</div>
          <div class="text-2xl font-bold font-mono tracking-tight text-foreground">
            {cacheStats() ? `${(cacheStats()!.hitRate * 100).toFixed(1)}%` : '-'}
          </div>
          <div class="text-[11px] text-faint">
            命中 {cacheStats() ? formatNumber(cacheStats()!.hits) : 0} / 未命中 {cacheStats() ? formatNumber(cacheStats()!.misses) : 0}
          </div>
        </Card>
        <Card class="p-4 space-y-1">
          <div class="text-xs text-muted">累计节省 Token</div>
          <div class="text-2xl font-bold font-mono tracking-tight text-success">
            {cacheStats() ? formatNumber(cacheStats()!.tokensSaved) : '0'}
          </div>
          <div class="text-[11px] text-faint">0-Token 命中直出节省</div>
        </Card>
        <Card class="p-4 space-y-1">
          <div class="text-xs text-muted">在存条目</div>
          <div class="text-2xl font-bold font-mono tracking-tight text-foreground">
            {cacheStats() ? `${cacheStats()!.entries} / ${cacheStats()!.maxEntries}` : '-'}
          </div>
          <div class="text-[11px] text-faint">LRU 内存缓存池容量</div>
        </Card>
        <Card class="p-4 space-y-1">
          <div class="text-xs text-muted">内存占用</div>
          <div class="text-2xl font-bold font-mono tracking-tight text-foreground">
            {cacheStats() ? formatBytes(cacheStats()!.bytesUsed) : '0 B'}
          </div>
          <div class="text-[11px] text-faint">硬限 2 MiB / 条保护</div>
        </Card>
      </div>

      {/* 核心功能卡片栅格 */}
      <div class="space-y-4">
        {/* 卡片 1: 全链路响应精确缓存 */}
        <Card class="p-5 space-y-4">
          <div class="flex items-start justify-between gap-4">
            <div>
              <div class="flex items-center gap-2">
                <h3 class="text-sm font-semibold text-foreground">响应精确缓存 (Exact Response Cache)</h3>
                <Show when={local().responseCacheEnabled}>
                  <Badge tone="green">已启用</Badge>
                </Show>
              </div>
              <p class="text-xs text-faint mt-1 leading-relaxed">
                对重复的确定性请求（temperature=0、结构化提取与嵌入向量）进行精确缓存，网关内 &lt;1ms 直接返回且完全不消耗上游 Token。
              </p>
            </div>
            <Toggle
              checked={!!local().responseCacheEnabled}
              onChange={v => {
                set('responseCacheEnabled', v)
                if (v && !local().responseCacheTTL) set('responseCacheTTL', 3600)
              }}
            />
          </div>

          <Show when={local().responseCacheEnabled}>
            <div class="space-y-3 pt-3 border-t border-subtle/50 pl-4 border-l-2 border-subtle">
              <div class="flex items-start justify-between gap-4">
                <Field
                  label="缓存全部非流式请求"
                  hint="开启后不论 temperature 为何值均做响应缓存；关闭则仅缓存确定性请求（temperature=0 与 embeddings）"
                >
                  <span />
                </Field>
                <Toggle
                  checked={!!local().responseCacheAll}
                  onChange={v => set('responseCacheAll', v)}
                />
              </div>

              <div class="flex items-center justify-between gap-4">
                <div>
                  <span class="text-xs text-muted font-medium">缓存有效期 (TTL)</span>
                  <p class="text-[11px] text-faint mt-0.5">超过该时长的旧条目将被自动失效，默认 3600 秒（1 小时）</p>
                </div>
                <div class="flex items-center gap-2">
                  <Input
                    type="number"
                    value={String(local().responseCacheTTL ?? 3600)}
                    class="!w-28 !h-8 text-xs font-mono"
                    onInput={v => set('responseCacheTTL', Number(v) || 0)}
                  />
                  <span class="text-xs text-faint">秒</span>
                </div>
              </div>

              <div class="pt-2 flex items-center justify-between border-t border-subtle/30">
                <span class="text-xs text-faint">手动维护操作</span>
                <div class="flex items-center gap-2">
                  <Button
                    size="sm"
                    variant="ghost"
                    loading={refreshingStats()}
                    onClick={handleRefreshStats}
                  >
                    刷新统计
                  </Button>
                  <Button
                    size="sm"
                    variant="danger"
                    loading={clearingCache()}
                    onClick={handleClearCache}
                  >
                    清空全部缓存
                  </Button>
                </div>
              </div>
            </div>
          </Show>
        </Card>

        {/* 卡片 2: 上下文与工具压缩 (RTK) */}
        <Card class="p-5 space-y-4">
          <div class="flex items-start justify-between gap-4">
            <div>
              <div class="flex items-center gap-2">
                <h3 class="text-sm font-semibold text-foreground">RTK 压缩 (Runtime Tool Kit Truncation)</h3>
                <Show when={local().rtkEnabled}>
                  <Badge tone="green">已启用</Badge>
                </Show>
              </div>
              <p class="text-xs text-faint mt-1 leading-relaxed">
                无损清洗工具输出（ANSI 转义符剥离、JSON 紧凑化、空行折叠），并在超长工具输出（&gt;250 行）时自动保留头部 120 行与尾部 60 行、折叠中间内容，成倍降低上下文膨胀。
              </p>
            </div>
            <Toggle checked={!!local().rtkEnabled} onChange={v => set('rtkEnabled', v)} />
          </div>
        </Card>

        {/* 卡片 3: 极简提示词注入 (Caveman & Ponytail) */}
        <Card class="p-5 space-y-4">
          <h3 class="text-sm font-semibold text-foreground">极简提示词注入 (Prompt Token Savers)</h3>

          {/* Caveman */}
          <div class="space-y-3">
            <div class="flex items-start justify-between gap-4">
              <div>
                <div class="flex items-center gap-2">
                  <span class="text-xs font-medium text-foreground">Caveman (极简表达)</span>
                  <Show when={local().cavemanEnabled}>
                    <Badge tone="blue">已启用</Badge>
                  </Show>
                </div>
                <p class="text-xs text-faint mt-0.5">向系统提示词注入极简表达指令，大幅削减模型回答时的填充词、客套套话与冗余解释</p>
              </div>
              <Toggle
                checked={!!local().cavemanEnabled}
                onChange={v => {
                  set('cavemanEnabled', v)
                  if (v && !local().cavemanLevel) set('cavemanLevel', 'lite')
                }}
              />
            </div>
            <Show when={local().cavemanEnabled}>
              <div class="flex items-center justify-between gap-4 pl-4 border-l-2 border-subtle">
                <span class="text-xs text-muted">压缩级别</span>
                <Select
                  size="sm"
                  class="!w-48"
                  value={String(local().cavemanLevel || 'lite')}
                  options={cavemanOptions}
                  onChange={v => set('cavemanLevel', v)}
                />
              </div>
            </Show>
          </div>

          {/* Ponytail */}
          <div class="space-y-3 pt-3 border-t border-subtle/50">
            <div class="flex items-start justify-between gap-4">
              <div>
                <div class="flex items-center gap-2">
                  <span class="text-xs font-medium text-foreground">Ponytail (极简代码)</span>
                  <Show when={local().ponytailEnabled}>
                    <Badge tone="blue">已启用</Badge>
                  </Show>
                </div>
                <p class="text-xs text-faint mt-0.5">注入极简代码原则（Lazy Senior Dev 阶梯原则），要求模型优先使用内置库与精简代码、避免过度封装</p>
              </div>
              <Toggle
                checked={!!local().ponytailEnabled}
                onChange={v => {
                  set('ponytailEnabled', v)
                  if (v && !local().ponytailLevel) set('ponytailLevel', 'lite')
                }}
              />
            </div>
            <Show when={local().ponytailEnabled}>
              <div class="flex items-center justify-between gap-4 pl-4 border-l-2 border-subtle">
                <span class="text-xs text-muted">约束级别</span>
                <Select
                  size="sm"
                  class="!w-48"
                  value={String(local().ponytailLevel || 'lite')}
                  options={ponytailOptions}
                  onChange={v => set('ponytailLevel', v)}
                />
              </div>
            </Show>
          </div>
        </Card>

        {/* 卡片 4: 规则排除与保护名单 */}
        <Card class="p-5 space-y-3">
          <div>
            <div class="flex items-center gap-2">
              <h3 class="text-sm font-semibold text-foreground">排除提供商名单 (TokenSaver 隔离保护)</h3>
              <Badge tone="gray">精准路由保护</Badge>
            </div>
            <p class="text-xs text-faint mt-1 leading-relaxed">
              针对特定上游提供商或深度推理模型（如 DeepSeek R1 / V3、OpenAI o1 等）跳过 RTK 压缩与 Caveman/Ponytail 提示词注入，杜绝思维链中断或格式异常。
            </p>
          </div>

          <div class="space-y-3 pt-1">
            {/* 当前排除列表 */}
            <div class="flex items-center gap-1.5 flex-wrap min-h-6">
              <Show
                when={excludedProviders().length > 0}
                fallback={<span class="text-xs text-faint italic">暂无排除项（所有提供商均应用 Token 节省规则）</span>}
              >
                <For each={excludedProviders()}>
                  {p => (
                    <span class="inline-flex items-center gap-1.5 px-2 py-0.5 rounded bg-warning/10 text-warning text-xs font-mono">
                      <span>{p}</span>
                      <button
                        type="button"
                        class="hover:text-foreground cursor-pointer text-sm leading-none opacity-70 hover:opacity-100"
                        onClick={() => removeExcludeProvider(p)}
                        title="移除排除"
                      >
                        ×
                      </button>
                    </span>
                  )}
                </For>
              </Show>
            </div>

            {/* 输入与添加 */}
            <div class="flex items-center gap-2 max-w-md">
              <Input
                value={excludeInput()}
                placeholder="输入提供商标识（如 deepseek）回车添加"
                class="flex-1 !h-8 text-xs font-mono"
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
                disabled={!excludeInput().trim()}
                onClick={() => addExcludeProvider(excludeInput())}
              >
                添加
              </Button>
            </div>

            {/* 快速推荐标签 */}
            <Show when={quickSuggestions().length > 0}>
              <div class="flex items-center gap-1.5 flex-wrap text-xs text-faint pt-0.5">
                <span>快捷候选：</span>
                <For each={quickSuggestions()}>
                  {p => (
                    <button
                      type="button"
                      class="px-1.5 py-0.5 rounded bg-hover hover:bg-subtle text-muted hover:text-foreground text-[11px] font-mono cursor-pointer transition-colors"
                      onClick={() => addExcludeProvider(p)}
                    >
                      + {p}
                    </button>
                  )}
                </For>
              </div>
            </Show>
          </div>
        </Card>
      </div>
    </div>
  )
}

export default TokenSaverPage
