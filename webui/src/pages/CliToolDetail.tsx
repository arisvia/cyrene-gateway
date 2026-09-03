import { type Component, For, Show, createSignal, createResource } from 'solid-js'
import { A, useParams } from '@solidjs/router'
import { api, apiPost, apiDelete } from '@/lib/api'
import { Card, Badge, Button, Input, Empty, Skeleton, ProviderAvatar } from '@/components/ui'
import { useToast } from '@/lib/toast'

const CliToolDetail: Component = () => {
  const params = useParams<{ id: string }>()
  const toast = useToast()
  const [copied, setCopied] = createSignal('')
  const [baseUrl, setBaseUrl] = createSignal('http://127.0.0.1:20128/v1')
  const [apiKey, setApiKey] = createSignal('')
  const [selectedModel, setSelectedModel] = createSignal('')
  const [busy, setBusy] = createSignal(false)
  const [shellTab, setShellTab] = createSignal<'bash' | 'powershell'>('bash')

  // 后端返回 { tool, status }
  const [data, { refetch }] = createResource(() => params.id, async id => {
    try {
      return await api('/api/cli-tools/' + id)
    } catch {
      return null
    }
  })

  const tool = () => data()?.tool
  const status = () => data()?.status ?? {}
  const hasGateway = () => !!status().hasGateway

  // guideSteps 里的 {{baseUrl}} 占位符需替换为实际地址
  const resolve = (v?: string) => (v || '').replace('{{baseUrl}}', baseUrl())

  async function copy(text: string, tag: string) {
    try {
      await navigator.clipboard.writeText(text)
      setCopied(tag)
      toast.success('已复制到剪贴板')
      setTimeout(() => setCopied(''), 2000)
    } catch {
      toast.error('复制失败')
    }
  }

  async function handleApply() {
    if (!tool()) return
    setBusy(true)
    try {
      await apiPost('/api/cli-tools/' + tool().id, {
        baseUrl: baseUrl(),
        apiKey: apiKey().trim() || undefined,
        model: selectedModel() || undefined,
      })
      toast.success(`已更新 ${tool().name} 的网关设置`)
      await refetch()
    } catch (e: unknown) {
      toast.error(e instanceof Error ? e.message : '写入异常')
    } finally {
      setBusy(false)
    }
  }

  async function handleReset() {
    if (!tool()) return
    setBusy(true)
    try {
      await apiDelete('/api/cli-tools/' + tool().id)
      toast.success(`已将 ${tool().name} 恢复为官方默认设置`)
      await refetch()
    } catch (e: unknown) {
      toast.error(e instanceof Error ? e.message : '重置异常')
    } finally {
      setBusy(false)
    }
  }

  // 终端临时会话变量生成
  const bashSnippet = () => {
    const b = baseUrl()
    const k = apiKey().trim() || 'sk-cyrene'
    if (tool()?.id === 'claude') {
      return `export ANTHROPIC_BASE_URL="${b}"\nexport ANTHROPIC_AUTH_TOKEN="${k}"`
    }
    return `export OPENAI_BASE_URL="${b}"\nexport OPENAI_API_KEY="${k}"`
  }

  const psSnippet = () => {
    const b = baseUrl()
    const k = apiKey().trim() || 'sk-cyrene'
    if (tool()?.id === 'claude') {
      return `$env:ANTHROPIC_BASE_URL="${b}"\n$env:ANTHROPIC_AUTH_TOKEN="${k}"`
    }
    return `$env:OPENAI_BASE_URL="${b}"\n$env:OPENAI_API_KEY="${k}"`
  }

  return (
    <div class="space-y-6">
      <A href="/cli-tools" class="inline-flex items-center gap-1 text-xs text-muted hover:text-accent transition-colors">
        <span>← 返回 CLI 工具列表</span>
      </A>

      <Show when={!data.loading} fallback={<Card class="p-8"><Skeleton class="h-48 w-full" /></Card>}>
        <Show when={tool()} fallback={<Card class="p-12 text-center glass-card"><Empty message="工具定义不存在。" /></Card>}>
          {/* 工具头部信息卡片 */}
          <Card class="p-6 glass-card">
            <div class="flex flex-col sm:flex-row items-start gap-4">
              <ProviderAvatar provider={tool().id} name={tool().name} color={tool().color} size="lg" />
              <div class="min-w-0 flex-1">
                <div class="flex items-center gap-3 flex-wrap">
                  <h1 class="text-xl font-semibold text-foreground">{tool().name}</h1>
                  <Badge tone={status().installed ? 'blue' : 'gray'}>
                    {status().installed ? '已检测到安装' : '未在本地环境检测到'}
                  </Badge>
                  <Show when={hasGateway()}>
                    <Badge tone="green">已接入网关</Badge>
                  </Show>
                  <Show when={tool().configType}>
                    <Badge tone="gray" class="font-mono text-xs">{tool().configType}</Badge>
                  </Show>
                </div>
                <p class="text-sm text-muted mt-1.5">{tool().description}</p>
                <Show when={tool().docsUrl}>
                  <div class="mt-2.5">
                    <a
                      href={tool().docsUrl}
                      target="_blank"
                      rel="noreferrer"
                      class="text-xs text-accent hover:underline inline-flex items-center gap-1"
                    >
                      <span>访问官方主页与说明文档</span>
                      <svg class="w-3 h-3" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                        <path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6" />
                        <polyline points="15 3 21 3 21 9" />
                        <line x1="10" y1="14" x2="21" y2="3" />
                      </svg>
                    </a>
                  </div>
                </Show>
              </div>
            </div>

            {/* 提示便签 */}
            <Show when={(tool().notes ?? []).length > 0}>
              <div class="grid gap-2 mt-5 pt-4 border-t border-subtle/50">
                <For each={tool().notes ?? []}>
                  {n => (
                    <div
                      class={`px-3.5 py-2.5 rounded-xl text-xs border ${
                        n.type === 'warning'
                          ? 'border-warning/30 bg-warning/10 text-warning'
                          : n.type === 'error'
                          ? 'border-danger/30 bg-danger/10 text-danger'
                          : 'border-subtle bg-hover/40 text-muted'
                      }`}
                    >
                      {n.text}
                    </div>
                  )}
                </For>
              </div>
            </Show>
          </Card>

          {/* 一键写入网关配置（custom 与 env 类型）*/}
          <Show when={tool().configType === 'custom' || tool().configType === 'env'}>
            <Card class="p-6 space-y-4 glass-card">
              <div class="flex items-center justify-between border-b border-subtle/50 pb-3">
                <div class="space-y-0.5">
                  <h2 class="text-base font-semibold text-foreground">持久化网关接入</h2>
                  <p class="text-xs text-muted">自动定位并覆写本地工具配置文件，无缝将流量代理至网关</p>
                </div>
                <Show when={status().configPath}>
                  <span class="text-[11px] font-mono text-faint truncate max-w-xs" title={status().configPath}>
                    目标文件: {status().configPath}
                  </span>
                </Show>
              </div>

              <div class="grid sm:grid-cols-2 gap-4">
                <div class="space-y-1.5">
                  <label class="text-xs font-medium text-foreground">网关 Base URL</label>
                  <Input value={baseUrl()} onInput={setBaseUrl} class="font-mono text-xs" />
                </div>
                <div class="space-y-1.5">
                  <label class="text-xs font-medium text-foreground">API 凭据 (可选留空则自动生成默认)</label>
                  <Input value={apiKey()} onInput={setApiKey} placeholder="留空使用 sk-cyrene" class="font-mono text-xs" />
                </div>
                <div class="space-y-1.5 sm:col-span-2">
                  <label class="text-xs font-medium text-foreground">指定转发模型 (可选)</label>
                  <Input
                    value={selectedModel()}
                    onInput={setSelectedModel}
                    placeholder="例如: gpt-5, claude-sonnet-4-6, deepseek-chat"
                    class="font-mono text-xs"
                  />
                </div>
              </div>

              <div class="flex items-center justify-end gap-3 pt-2">
                <Show when={hasGateway()}>
                  <Button variant="danger" loading={busy()} onClick={handleReset}>
                    清除网关配置 (恢复官方默认)
                  </Button>
                </Show>
                <Button variant="primary" loading={busy()} onClick={handleApply}>
                  {hasGateway() ? '更新配置' : '一键写入网关配置'}
                </Button>
              </div>
            </Card>
          </Show>

          {/* 终端会话临时环境变量 */}
          <Card class="p-6 space-y-3 glass-card">
            <div class="flex items-center justify-between">
              <div class="space-y-0.5">
                <h2 class="text-base font-semibold text-foreground">临时终端环境变量</h2>
                <p class="text-xs text-muted">在终端执行此导出命令，本次会话中运行该工具将直接接入网关，不改动持久配置文件</p>
              </div>
              <div class="flex items-center gap-1 rounded-lg bg-hover p-0.5 border border-subtle">
                <button
                  type="button"
                  onClick={() => setShellTab('bash')}
                  class={`px-2.5 py-1 rounded text-xs transition-colors ${shellTab() === 'bash' ? 'bg-card text-foreground shadow-sm' : 'text-muted'}`}
                >
                  Bash / Zsh
                </button>
                <button
                  type="button"
                  onClick={() => setShellTab('powershell')}
                  class={`px-2.5 py-1 rounded text-xs transition-colors ${shellTab() === 'powershell' ? 'bg-card text-foreground shadow-sm' : 'text-muted'}`}
                >
                  PowerShell
                </button>
              </div>
            </div>

            <div class="relative">
              <pre class="text-xs font-mono text-text bg-bg-elevated p-3.5 rounded-xl border border-subtle overflow-x-auto selection:bg-accent/30">
                {shellTab() === 'bash' ? bashSnippet() : psSnippet()}
              </pre>
              <div class="absolute top-2.5 right-2.5">
                <Button
                  size="sm"
                  variant="secondary"
                  onClick={() => copy(shellTab() === 'bash' ? bashSnippet() : psSnippet(), 'env')}
                >
                  {copied() === 'env' ? '已复制' : '复制命令'}
                </Button>
              </div>
            </div>
          </Card>

          {/* 手动配置分步指南（针对 guide 类型）*/}
          <Show when={(tool().guideSteps ?? []).length > 0}>
            <Card class="p-6 space-y-4 glass-card">
              <div class="border-b border-subtle/50 pb-3">
                <h2 class="text-base font-semibold text-foreground">分步手动配置指引</h2>
                <p class="text-xs text-muted">在工具界面中按照以下步骤完成模型转发参数绑定</p>
              </div>

              <ol class="space-y-3.5">
                <For each={tool().guideSteps ?? []}>
                  {s => (
                    <li class="flex gap-3.5 items-start">
                      <span class="w-6 h-6 shrink-0 rounded-full bg-accent/15 text-accent text-xs flex items-center justify-center font-bold">
                        {s.step}
                      </span>
                      <div class="min-w-0 flex-1 space-y-1">
                        <div class="text-sm font-medium text-foreground">{s.title}</div>
                        <Show when={s.desc}>
                          <div class="text-xs text-muted">{s.desc}</div>
                        </Show>
                        <Show when={s.value}>
                          <div class="flex items-center gap-2 mt-1.5">
                            <code class="text-xs font-mono text-foreground bg-bg-elevated px-2.5 py-1 rounded-lg border border-subtle break-all">
                              {resolve(s.value)}
                            </code>
                            <Button size="sm" variant="ghost" onClick={() => copy(resolve(s.value), 'step' + s.step)}>
                              {copied() === 'step' + s.step ? '已复制' : '复制'}
                            </Button>
                          </div>
                        </Show>
                      </div>
                    </li>
                  )}
                </For>
              </ol>
            </Card>
          </Show>

          {/* 推荐模型列表展示 */}
          <Show when={(tool().defaultModels ?? []).length > 0}>
            <Card class="p-6 space-y-3 glass-card">
              <h2 class="text-base font-semibold text-foreground">针对此工具优化的常用模型</h2>
              <div class="grid sm:grid-cols-3 gap-2.5">
                <For each={tool().defaultModels ?? []}>
                  {m => (
                    <div class="p-3 rounded-xl border border-subtle bg-bg-elevated/60 flex flex-col justify-between">
                      <div class="text-xs font-semibold text-foreground">{m.name || m.id}</div>
                      <div class="text-[10px] text-faint font-mono truncate mt-1">{m.id}</div>
                      <Show when={m.alias}>
                        <div class="text-[10px] text-accent mt-0.5">别名: {m.alias}</div>
                      </Show>
                    </div>
                  )}
                </For>
              </div>
            </Card>
          </Show>
        </Show>
      </Show>
    </div>
  )
}

export default CliToolDetail
