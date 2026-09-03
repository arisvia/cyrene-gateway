import { type Component, For, Show, createSignal, createResource, createMemo } from 'solid-js'
import type { CLITool } from '@/types/domain'
import { A } from '@solidjs/router'
import { api, apiPost, apiDelete } from '@/lib/api'
import { Card, Badge, Button, Input, Empty, Skeleton, ProviderAvatar } from '@/components/ui'
import { useToast } from '@/lib/toast'

interface ToolRow extends CLITool {
  installed: boolean
  hasGateway: boolean
  configPath?: string
  message?: string
}

type CategoryKey = 'all' | 'cli' | 'extension' | 'ide'
type StatusFilter = 'all' | 'connected' | 'installed' | 'unconnected'

const CATEGORY_NAMES: Record<CategoryKey, string> = {
  all: '全部工具',
  cli: '终端 CLI',
  extension: 'IDE 插件',
  ide: 'AI 编辑器',
}

const CliTools: Component = () => {
  const toast = useToast()
  const [busy, setBusy] = createSignal<string | null>(null)
  const [baseUrl, setBaseUrl] = createSignal('http://127.0.0.1:20128/v1')
  const [err, setErr] = createSignal('')
  const [category, setCategory] = createSignal<CategoryKey>('all')
  const [statusFilter, setStatusFilter] = createSignal<StatusFilter>('all')
  const [copiedId, setCopiedId] = createSignal<string | null>(null)

  // registry（静态定义） + all-statuses（探测结果）合并
  const [data, { refetch }] = createResource(async () => {
    const [reg, statuses] = await Promise.all([
      api('/api/cli-tools').catch(() => ({ tools: [] })),
      api('/api/cli-tools/all-statuses').catch(() => ({})),
    ])
    const tools = (reg?.tools ?? []) as CLITool[]
    const st = (statuses ?? {}) as Record<string, { installed?: boolean; hasGateway?: boolean; configPath?: string; message?: string }>
    return tools.map((t): ToolRow => {
      const s = st[t.id] ?? {}
      return {
        ...t,
        installed: !!s.installed,
        hasGateway: !!s.hasGateway,
        configPath: s.configPath,
        message: s.message,
      }
    })
  })

  const rows = createMemo<ToolRow[]>(() => data() ?? [])
  const configuredCount = createMemo(() => rows().filter(r => r.hasGateway).length)
  const installedCount = createMemo(() => rows().filter(r => r.installed).length)

  // 过滤后的列表
  const filteredRows = createMemo(() => {
    return rows().filter(r => {
      if (category() !== 'all' && (r.category || 'cli') !== category()) {
        return false
      }
      if (statusFilter() === 'connected' && !r.hasGateway) return false
      if (statusFilter() === 'installed' && !r.installed) return false
      if (statusFilter() === 'unconnected' && r.hasGateway) return false
      return true
    })
  })

  async function apply(id: string) {
    setBusy(id)
    setErr('')
    try {
      await apiPost('/api/cli-tools/' + id, { baseUrl: baseUrl() })
      toast.success(`已成功将网关配置写入 ${id}`)
      await refetch()
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : '接入失败'
      setErr(msg)
      toast.error(msg)
    } finally {
      setBusy(null)
    }
  }

  async function reset(id: string) {
    setBusy(id)
    setErr('')
    try {
      await apiDelete('/api/cli-tools/' + id)
      toast.success(`已清除 ${id} 的网关配置`)
      await refetch()
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : '重置失败'
      setErr(msg)
      toast.error(msg)
    } finally {
      setBusy(null)
    }
  }

  // 生成临时的终端导出命令
  function getEnvCommand(tool: ToolRow): string {
    const base = baseUrl()
    if (tool.id === 'claude') {
      return `export ANTHROPIC_BASE_URL="${base}"\nexport ANTHROPIC_AUTH_TOKEN="sk-cyrene"`
    }
    return `export OPENAI_BASE_URL="${base}"\nexport OPENAI_API_KEY="sk-cyrene"`
  }

  async function copyEnv(tool: ToolRow) {
    const cmd = getEnvCommand(tool)
    try {
      await navigator.clipboard.writeText(cmd)
      setCopiedId(tool.id)
      toast.success('已复制环境变量命令，粘贴到终端执行即可立即生效')
      setTimeout(() => setCopiedId(null), 2000)
    } catch {
      toast.error('无法写入剪贴板')
    }
  }

  return (
    <div class="space-y-6 stagger">
      {/* 顶部标题与状态统计 */}
      <div class="flex flex-wrap items-center justify-between gap-4">
        <div>
          <h1 class="text-2xl font-bold tracking-tight text-foreground">CLI 工具接入</h1>
          <p class="text-sm text-faint mt-1">
            一键将 Cyrene Gateway 模型调度能力注入本地 AI 编程工具与主流编辑器
          </p>
        </div>
        <div class="flex items-center gap-3">
          <Button variant="secondary" onClick={() => refetch()}>
            <svg class="w-4 h-4 mr-1.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <path d="M21.5 2v6h-6M21.34 15.57a10 10 0 1 1-.57-8.38l5.67-5.67" />
            </svg>
            重新探测环境
          </Button>
        </div>
      </div>

      {/* 网关接入参数基准配置卡片 */}
      <Card class="p-5 flex flex-wrap items-center justify-between gap-4 glass-card">
        <div class="space-y-1">
          <div class="text-sm font-semibold text-foreground flex items-center gap-2">
            <span>网关接入基准地址 (Base URL)</span>
            <Badge tone="green" class="font-mono text-[10px]">Active</Badge>
          </div>
          <p class="text-xs text-muted">工具一键注入及手动配置时所使用的本地或远程统一网关地址</p>
        </div>
        <div class="flex items-center gap-2 w-full sm:w-auto">
          <Input
            class="!w-full sm:!w-80 font-mono text-xs"
            value={baseUrl()}
            onInput={setBaseUrl}
            placeholder="http://127.0.0.1:20128/v1"
          />
        </div>
      </Card>

      {/* 状态统计条目与分类筛选 */}
      <div class="flex flex-wrap items-center justify-between gap-4 border-b border-subtle pb-4">
        {/* 形态分类 Tab Pills */}
        <div class="flex items-center gap-1.5 p-1 rounded-xl bg-card border border-subtle">
          <For each={(['all', 'cli', 'extension', 'ide'] as CategoryKey[])}>
            {cat => (
              <button
                type="button"
                onClick={() => setCategory(cat)}
                class={`px-3 py-1.5 rounded-lg text-xs font-medium transition-colors ${
                  category() === cat
                    ? 'bg-accent text-on-accent shadow-sm'
                    : 'text-muted hover:text-foreground hover:bg-hover'
                }`}
              >
                {CATEGORY_NAMES[cat]}
              </button>
            )}
          </For>
        </div>

        {/* 状态过滤器 */}
        <div class="flex items-center gap-2 text-xs">
          <span class="text-faint">状态筛选:</span>
          <div class="flex items-center gap-1">
            <button
              type="button"
              onClick={() => setStatusFilter('all')}
              class={`px-2.5 py-1 rounded-lg text-xs ${statusFilter() === 'all' ? 'bg-hover text-foreground font-semibold' : 'text-muted hover:text-foreground'}`}
            >
              全部 ({rows().length})
            </button>
            <button
              type="button"
              onClick={() => setStatusFilter('connected')}
              class={`px-2.5 py-1 rounded-lg text-xs ${statusFilter() === 'connected' ? 'bg-hover text-foreground font-semibold' : 'text-muted hover:text-foreground'}`}
            >
              已接入 ({configuredCount()})
            </button>
            <button
              type="button"
              onClick={() => setStatusFilter('installed')}
              class={`px-2.5 py-1 rounded-lg text-xs ${statusFilter() === 'installed' ? 'bg-hover text-foreground font-semibold' : 'text-muted hover:text-foreground'}`}
            >
              已检测到 ({installedCount()})
            </button>
          </div>
        </div>
      </div>

      <Show when={err()}>
        <div class="px-4 py-3 rounded-xl text-xs bg-danger/10 text-danger border border-danger/20 flex items-center justify-between">
          <span>{err()}</span>
          <button type="button" onClick={() => setErr('')} class="underline ml-4">关闭</button>
        </div>
      </Show>

      {/* 工具网格卡片 */}
      <Show when={!data.loading} fallback={<Card class="p-8"><Skeleton class="h-48 w-full" /></Card>}>
        <Show
          when={filteredRows().length > 0}
          fallback={
            <Card class="p-12 text-center glass-card">
              <Empty message="当前分类下暂无符合条件的工具。" />
            </Card>
          }
        >
          <div class="grid sm:grid-cols-2 lg:grid-cols-3 gap-4">
            <For each={filteredRows()}>
              {t => (
                <Card hover class="p-5 flex flex-col justify-between glass-card transition-all duration-200">
                  <div>
                    {/* 卡片头部：图标与基础信息 */}
                    <div class="flex items-start gap-3.5">
                      <ProviderAvatar provider={t.id} name={t.name} color={t.color} size="md" />
                      <div class="min-w-0 flex-1">
                        <div class="flex items-center justify-between gap-1">
                          <A href={`/cli-tools/${t.id}`} class="font-semibold text-sm text-foreground hover:text-accent transition-colors truncate">
                            {t.name}
                          </A>
                          <Show when={t.configType === 'custom' || t.configType === 'env'}>
                            <Badge tone="green" class="text-[10px] shrink-0">一键配置</Badge>
                          </Show>
                          <Show when={t.configType === 'guide'}>
                            <Badge tone="amber" class="text-[10px] shrink-0">配置引导</Badge>
                          </Show>
                          <Show when={t.configType === 'mitm'}>
                            <Badge tone="blue" class="text-[10px] shrink-0">MITM 代理</Badge>
                          </Show>
                        </div>
                        <p class="text-xs text-muted mt-1 line-clamp-2 leading-relaxed">
                          {t.description}
                        </p>
                      </div>
                    </div>

                    {/* 状态徽章与配置文件路径 */}
                    <div class="mt-3.5 pt-3 border-t border-subtle/50 flex flex-col gap-2">
                      <div class="flex items-center gap-1.5 flex-wrap">
                        <Badge tone={t.installed ? 'blue' : 'gray'}>
                          {t.installed ? '已检测到' : '未检测到'}
                        </Badge>
                        <Show when={t.hasGateway}>
                          <Badge tone="green">已接入网关</Badge>
                        </Show>
                      </div>

                      <Show when={t.configPath}>
                        <div class="text-[10px] text-faint font-mono truncate" title={t.configPath}>
                          {t.configPath}
                        </div>
                      </Show>
                      <Show when={t.message}>
                        <div class="text-[10px] text-faint truncate">{t.message}</div>
                      </Show>
                    </div>
                  </div>

                  {/* 卡片底部操作按钮 */}
                  <div class="mt-4 pt-3 border-t border-subtle/50 flex items-center justify-between gap-2">
                    <button
                      type="button"
                      onClick={() => copyEnv(t)}
                      class="text-xs text-muted hover:text-foreground hover:underline flex items-center gap-1"
                      title="复制此工具的终端临时环境变量启动命令"
                    >
                      <svg class="w-3.5 h-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                        <rect width="14" height="14" x="8" y="8" rx="2" ry="2" />
                        <path d="M4 16c-1.1 0-2-.9-2-2V4c0-1.1.9-2 2-2h10c1.1 0 2 .9 2 2" />
                      </svg>
                      <span>{copiedId() === t.id ? '已复制' : '复制 Env'}</span>
                    </button>

                    <div class="flex items-center gap-2">
                      <Show when={t.configType === 'custom' || t.configType === 'env'}>
                        <Show when={t.hasGateway} fallback={
                          <Button
                            size="sm"
                            variant="secondary"
                            loading={busy() === t.id}
                            disabled={!baseUrl().trim()}
                            onClick={() => apply(t.id)}
                          >
                            接入
                          </Button>
                        }>
                          <Button
                            size="sm"
                            variant="danger"
                            loading={busy() === t.id}
                            onClick={() => reset(t.id)}
                          >
                            重置
                          </Button>
                        </Show>
                      </Show>
                      <A
                        href={`/cli-tools/${t.id}`}
                        class="text-xs px-2.5 py-1.5 rounded-control font-medium bg-hover text-foreground hover:bg-accent hover:text-on-accent transition-colors"
                      >
                        详情 →
                      </A>
                    </div>
                  </div>
                </Card>
              )}
            </For>
          </div>
        </Show>
      </Show>
    </div>
  )
}

export default CliTools
