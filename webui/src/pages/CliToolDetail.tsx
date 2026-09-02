import { type Component, For, Show, createSignal, createResource } from 'solid-js'
import { A, useParams } from '@solidjs/router'
import { api } from '@/lib/api'
import { Card, Badge, Button, Empty, Skeleton } from '@/components/ui'

const CliToolDetail: Component = () => {
  const params = useParams<{ id: string }>()
  const [copied, setCopied] = createSignal('')
  const [baseUrl, setBaseUrl] = createSignal('http://127.0.0.1:20128/v1')

  // 后端返回 { tool, status } —— 必须解包
  const [data] = createResource(() => params.id, async id => {
    try { return await api('/api/cli-tools/' + id) } catch { return null }
  })

  const tool = () => data()?.tool
  const status = () => data()?.status ?? {}

  // guideSteps 里的 {{baseUrl}} 占位符需替换为实际地址
  const resolve = (v?: string) => (v || '').replace('{{baseUrl}}', baseUrl())

  async function copy(text: string, tag: string) {
    try { await navigator.clipboard.writeText(text) } catch { /* 忽略 */ }
    setCopied(tag)
    setTimeout(() => setCopied(''), 1500)
  }

  return (
    <div class="space-y-5">
      <A href="/cli-tools" class="text-xs text-faint hover:text-accent">← 返回工具列表</A>

      <Show when={!data.loading} fallback={<Card class="p-6"><Skeleton class="h-40 w-full" /></Card>}>
        <Show when={tool()} fallback={<Card class="p-6"><Empty message="工具不存在。" /></Card>}>
          {/* 头部 */}
          <div class="flex items-start gap-4">
            <Show when={tool().icon} fallback={
              <div class="w-12 h-12 rounded-xl shrink-0" style={{ background: tool().color || 'var(--accent)' }} />
            }>
              <img src={tool().icon} alt="" class="w-12 h-12 rounded-xl object-contain shrink-0" />
            </Show>
            <div class="min-w-0 flex-1">
              <h1 class="text-xl font-semibold">{tool().name}</h1>
              <p class="text-sm text-faint mt-0.5">{tool().description}</p>
              <div class="mt-2 flex items-center gap-2 flex-wrap">
                <Badge tone={status().installed ? 'green' : 'gray'}>
                  {status().installed ? '已安装' : '未安装'}
                </Badge>
                <Badge tone={status().has9Router ? 'blue' : 'gray'}>
                  {status().has9Router ? '已接入网关' : '未接入'}
                </Badge>
                <Show when={tool().configType}><Badge tone="gray">{tool().configType}</Badge></Show>
                <Show when={tool().docsUrl}>
                  <a href={tool().docsUrl} target="_blank" rel="noreferrer"
                     class="text-xs text-accent hover:underline">官方文档 ↗</a>
                </Show>
              </div>
            </div>
          </div>

          {/* 提示 */}
          <Show when={(tool().notes ?? []).length > 0}>
            <div class="grid gap-2 mt-5">
              <For each={tool().notes ?? []}>
                {n => (
                  <div class={`px-3 py-2 rounded-control text-xs border ${
                    n.type === 'warning' ? 'border-warning/30 bg-warning/10 text-warning'
                    : n.type === 'error' ? 'border-danger/30 bg-danger/10 text-danger'
                    : 'border-subtle bg-hover text-muted'}`}>
                    {n.text}
                  </div>
                )}
              </For>
            </div>
          </Show>

          {/* 状态详情 */}
          <Card class="p-5 mt-5">
            <h3 class="text-sm font-semibold mb-3">检测状态</h3>
            <div class="space-y-1.5 text-xs">
              <div class="flex justify-between">
                <span class="text-muted">配置文件</span>
                <span class="font-mono text-faint truncate max-w-[60%]" title={status().configPath}>
                  {status().configPath || '—'}
                </span>
              </div>
              <Show when={status().message}>
                <div class="flex justify-between">
                  <span class="text-muted">说明</span>
                  <span class="text-faint">{status().message}</span>
                </div>
              </Show>
            </div>
          </Card>

          {/* 手动引导步骤（guide 类型）*/}
          <Show when={(tool().guideSteps ?? []).length > 0}>
            <Card class="p-5">
              <div class="flex items-center justify-between mb-3">
                <h3 class="text-sm font-semibold">手动配置步骤</h3>
                <div class="flex items-center gap-2">
                  <span class="text-[11px] text-faint">Base URL</span>
                  <input
                    value={baseUrl()}
                    onInput={e => setBaseUrl(e.currentTarget.value)}
                    class="px-2 py-1 rounded-control bg-bg-elevated border border-subtle text-xs text-text font-mono w-56 focus:outline-none focus:border-accent"
                  />
                </div>
              </div>
              <ol class="space-y-3">
                <For each={tool().guideSteps ?? []}>
                  {s => (
                    <li class="flex gap-3">
                      <span class="w-6 h-6 shrink-0 rounded-full bg-accent/15 text-accent text-xs flex items-center justify-center font-medium">
                        {s.step}
                      </span>
                      <div class="min-w-0 flex-1">
                        <div class="text-sm font-medium">{s.title}</div>
                        <Show when={s.desc}><div class="text-xs text-faint mt-0.5">{s.desc}</div></Show>
                        <Show when={s.value}>
                          <div class="flex items-center gap-2 mt-1.5">
                            <code class="text-xs font-mono text-muted break-all bg-bg-elevated px-2 py-1 rounded border border-subtle">
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

          {/* 可复制代码块 */}
          <Show when={tool().codeBlock}>
            <Card class="p-5">
              <div class="flex items-center justify-between mb-3">
                <h3 class="text-sm font-semibold">
                  配置示例{tool().codeBlock.language ? ` · ${tool().codeBlock.language}` : ''}
                </h3>
                <Button size="sm" variant="secondary" onClick={() => copy(tool().codeBlock.code, 'code')}>
                  {copied() === 'code' ? '已复制' : '复制'}
                </Button>
              </div>
              <pre class="text-[11px] font-mono text-muted whitespace-pre-wrap break-all bg-bg-elevated p-3 rounded-control border border-subtle max-h-96 overflow-y-auto">
                {tool().codeBlock.code}
              </pre>
            </Card>
          </Show>

          {/* 推荐模型 */}
          <Show when={(tool().defaultModels ?? []).length > 0}>
            <Card class="p-5">
              <h3 class="text-sm font-semibold mb-3">推荐模型</h3>
              <div class="grid sm:grid-cols-2 gap-2">
                <For each={tool().defaultModels ?? []}>
                  {m => (
                    <div class="px-3 py-2 rounded-control border border-subtle bg-bg-elevated">
                      <div class="text-sm">{m.name || m.id}</div>
                      <div class="text-[11px] text-faint font-mono truncate">{m.id}</div>
                      <Show when={m.alias}>
                        <div class="text-[10px] text-faint">别名 {m.alias}</div>
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
