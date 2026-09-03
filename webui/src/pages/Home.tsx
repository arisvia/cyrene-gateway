import { type Component, For, Show, createSignal, onMount } from 'solid-js'
import { A } from '@solidjs/router'
import { useGatewayStore } from '@/stores/gateway'
import { Card, Badge, Empty, Button, Input, confirm } from '@/components/ui'
import { useToast } from '@/lib/toast'

const Home: Component = () => {
  const store = useGatewayStore()
  const toast = useToast()

  const [keyName, setKeyName] = createSignal('')
  const [creatingKey, setCreatingKey] = createSignal(false)
  const [copiedKeyId, setCopiedKeyId] = createSignal<string | null>(null)
  const [copiedEndpoint, setCopiedEndpoint] = createSignal<string | null>(null)

  onMount(() => {
    store.loadKeys()
  })

  const copyText = (text: string, label: string) => {
    navigator.clipboard?.writeText(text)
    toast.success(`已复制 ${label}`)
  }

  return (
    <div class="space-y-6 stagger">
      {/* 2026 现代沉浸式网关英雄卡片 (Hero Gateway Status Banner) */}
      <Card class="p-6 relative overflow-hidden group">
        <div class="absolute -right-12 -bottom-12 w-64 h-64 bg-accent/10 rounded-full blur-3xl pointer-events-none group-hover:bg-accent/15 transition-all duration-700" />
        <div class="flex flex-col md:flex-row md:items-center justify-between gap-6 relative z-10">
          <div class="flex items-center gap-4">
            <div class="relative flex items-center justify-center">
              <span class="absolute inline-flex h-8 w-8 rounded-full bg-success/25 animate-ping duration-1000" />
              <div class="relative h-10 w-10 rounded-2xl bg-success/15 border border-success/30 flex items-center justify-center text-success shadow-sm shadow-success/20">
                <span class="w-3.5 h-3.5 rounded-full bg-success animate-pulse" />
              </div>
            </div>
            <div>
              <div class="flex items-center gap-2.5">
                <h1 class="text-xl font-bold tracking-tight text-foreground">Cyrene Gateway</h1>
                <Badge tone="green" class="font-medium">运行中</Badge>
              </div>
              <p class="text-xs text-faint mt-1 flex items-center gap-3">
                <span>高并发统一 API 代理与模型路由枢纽</span>
              </p>
            </div>
          </div>

          <div class="flex items-center gap-3 flex-wrap sm:flex-nowrap">
            <div class="flex items-center gap-3 px-4 py-2.5 rounded-2xl bg-card/60 border border-subtle backdrop-blur-md">
              <div class="text-right">
                <div class="text-base font-bold text-foreground leading-none">{store.activeConnections()}</div>
                <div class="text-[11px] text-faint mt-0.5">活跃上游通道</div>
              </div>
              <div class="w-2 h-2 rounded-full bg-accent animate-pulse" />
            </div>
            <div class="flex items-center gap-3 px-4 py-2.5 rounded-2xl bg-card/60 border border-subtle backdrop-blur-md">
              <div class="text-right">
                <div class="text-base font-bold text-foreground leading-none">{store.combos().length}</div>
                <div class="text-[11px] text-faint mt-0.5">故障回退组合</div>
              </div>
              <div class="w-2 h-2 rounded-full bg-accent-2 animate-pulse" />
            </div>
          </div>
        </div>
      </Card>

      {/* 核心功能区：左侧 API 密钥管理 + 右侧端点与快速接入 */}
      <div class="grid grid-cols-1 lg:grid-cols-12 gap-6">
        {/* 左侧：API 密钥一等公民控制区 (占据 7 列) */}
        <div class="lg:col-span-7 space-y-6">
          <Card class="p-6 space-y-5">
            <div class="flex items-center justify-between">
              <div>
                <h2 class="text-base font-semibold text-foreground flex items-center gap-2">
                  <svg class="w-4 h-4 text-accent" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                    <circle cx="7.5" cy="15.5" r="5.5" />
                    <path d="m21 2-9.6 9.6" />
                    <path d="m15.5 7.5 3 3L22 7l-3-3" />
                  </svg>
                  <span>API 密钥 (API Keys)</span>
                </h2>
                <p class="text-xs text-faint mt-0.5">用于客户端（Claude Code / Codex / NextChat / Cursor 等）鉴权</p>
              </div>
              <Button size="sm" variant="secondary" onClick={() => store.loadKeys()}>
                刷新
              </Button>
            </div>

            {/* 创建新 Key 输入框 */}
            <div class="flex gap-2 p-1.5 rounded-2xl bg-card/50 border border-subtle focus-within:border-accent/50 transition-colors">
              <Input
                value={keyName()}
                placeholder="新建 API Key 名称（如: Work-MacBook / Claude-CLI）"
                onInput={setKeyName}
                onKeyDown={async e => {
                  if (e.key === 'Enter' && keyName().trim() && !creatingKey()) {
                    const n = keyName().trim()
                    setCreatingKey(true)
                    try {
                      await store.createKey(n)
                      setKeyName('')
                      toast.success(`API Key「${n}」创建成功`)
                    } catch (err) {
                      console.error(err)
                      toast.error('创建密钥失败')
                    } finally {
                      setCreatingKey(false)
                    }
                  }
                }}
                disabled={creatingKey()}
                class="!border-0 !bg-transparent !shadow-none focus:!ring-0 text-sm"
              />
              <Button
                variant="primary"
                loading={creatingKey()}
                disabled={!keyName().trim()}
                onClick={async () => {
                  const n = keyName().trim()
                  if (!n) return
                  setCreatingKey(true)
                  try {
                    await store.createKey(n)
                    setKeyName('')
                    toast.success(`API Key「${n}」创建成功`)
                  } catch (err) {
                    console.error(err)
                    toast.error('创建密钥失败')
                  } finally {
                    setCreatingKey(false)
                  }
                }}
              >
                生成密钥
              </Button>
            </div>

            {/* 密钥列表展示 */}
            <div class="space-y-2.5">
              <Show
                when={store.apiKeys().length > 0}
                fallback={
                  <div class="py-8 text-center border border-dashed border-subtle rounded-2xl">
                    <p class="text-xs text-faint">暂无可用 API Key，在上方输入名称即可一键生成</p>
                  </div>
                }
              >
                <For each={store.apiKeys()}>
                  {k => {
                    const isCopied = () => copiedKeyId() === k.id
                    return (
                      <div class="flex items-center justify-between gap-3 p-3.5 rounded-xl border border-subtle bg-card/40 hover:bg-card hover:border-accent/30 transition-all group">
                        <div class="min-w-0 flex-1">
                          <div class="flex items-center gap-2">
                            <span class="text-sm font-medium text-foreground truncate">{k.name || '(未命名)'}</span>
                            <Badge tone="gray" class="text-[10px] scale-95">Bearer</Badge>
                          </div>
                          <div class="flex items-center gap-2 mt-1">
                            <code class="text-xs text-muted font-mono truncate max-w-[280px] select-all">
                              {k.key}
                            </code>
                          </div>
                        </div>

                        <div class="flex items-center gap-2 shrink-0">
                          <Button
                            size="sm"
                            variant="secondary"
                            onClick={() => {
                              copyText(k.key, `密钥 ${k.name || ''}`)
                              setCopiedKeyId(k.id)
                              setTimeout(() => setCopiedKeyId(null), 2000)
                            }}
                          >
                            {isCopied() ? '已复制 ✓' : '复制 Key'}
                          </Button>
                          <Button
                            size="sm"
                            variant="danger"
                            onClick={async () => {
                              const ok = await confirm({
                                title: '删除 API Key',
                                message: `确定删除 API Key「${k.name || k.key}」吗？删除后调用将失效。`,
                                variant: 'danger',
                              })
                              if (ok) {
                                await store.deleteKey(k.id)
                                toast.info('密钥已删除')
                              }
                            }}
                          >
                            删除
                          </Button>
                        </div>
                      </div>
                    )
                  }}
                </For>
              </Show>
            </div>
          </Card>
        </div>

        {/* 右侧：网关统一端点与快捷客户端配置 (占据 5 列) */}
        <div class="lg:col-span-5 space-y-6">
          {/* 端点卡片 */}
          <Card class="p-6 space-y-4">
            <div class="flex items-center justify-between">
              <h2 class="text-base font-semibold text-foreground flex items-center gap-2">
                <svg class="w-4 h-4 text-accent" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                  <path d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71" />
                  <path d="M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71" />
                </svg>
                <span>网关统一端点</span>
              </h2>
              <Badge tone="blue">多协议兼容</Badge>
            </div>

            <div class="space-y-2.5">
              <For each={store.endpoints()}>
                {ep => {
                  const isCopied = () => copiedEndpoint() === ep.url
                  return (
                    <button
                      class="w-full flex items-center justify-between gap-3 rounded-xl border border-subtle px-3.5 py-3 text-left bg-card/40 hover:bg-card hover:border-accent/40 transition-all group"
                      onClick={() => {
                        copyText(ep.url, ep.label)
                        setCopiedEndpoint(ep.url)
                        setTimeout(() => setCopiedEndpoint(null), 2000)
                      }}
                      title="点击一键复制"
                    >
                      <div class="min-w-0 flex-1">
                        <div class="text-sm font-medium text-foreground">{ep.label}</div>
                        <code class="text-xs text-faint truncate block font-mono mt-0.5">{ep.url}</code>
                      </div>
                      <span class="text-xs text-muted group-hover:text-accent font-medium shrink-0">
                        {isCopied() ? '已复制 ✓' : '复制'}
                      </span>
                    </button>
                  )
                }}
              </For>
              <Show when={store.endpoints().length === 0}>
                <Empty message="暂无端点" />
              </Show>
            </div>
          </Card>

          {/* 快速接入一览 */}
          <Card class="p-6 space-y-4">
            <div class="flex items-center justify-between">
              <h2 class="text-base font-semibold text-foreground">快速接入指南</h2>
              <A href="/cli-tools" class="text-xs font-medium text-accent hover:underline flex items-center gap-1">
                <span>全部 11+ 工具</span>
                <span>→</span>
              </A>
            </div>
            <div class="grid grid-cols-2 gap-2.5">
              <A
                href="/cli-tools"
                class="flex items-center justify-between p-3 rounded-xl border border-subtle bg-card/40 hover:bg-card hover:border-accent/40 transition-all group"
              >
                <div class="text-sm font-medium text-foreground">Claude Code</div>
                <span class="text-xs text-faint group-hover:text-accent">配置 →</span>
              </A>
              <A
                href="/cli-tools"
                class="flex items-center justify-between p-3 rounded-xl border border-subtle bg-card/40 hover:bg-card hover:border-accent/40 transition-all group"
              >
                <div class="text-sm font-medium text-foreground">OpenCode</div>
                <span class="text-xs text-faint group-hover:text-accent">配置 →</span>
              </A>
              <A
                href="/cli-tools"
                class="flex items-center justify-between p-3 rounded-xl border border-subtle bg-card/40 hover:bg-card hover:border-accent/40 transition-all group"
              >
                <div class="text-sm font-medium text-foreground">Codex CLI</div>
                <span class="text-xs text-faint group-hover:text-accent">配置 →</span>
              </A>
              <A
                href="/cli-tools"
                class="flex items-center justify-between p-3 rounded-xl border border-subtle bg-card/40 hover:bg-card hover:border-accent/40 transition-all group"
              >
                <div class="text-sm font-medium text-foreground">Cursor / Cline</div>
                <span class="text-xs text-faint group-hover:text-accent">配置 →</span>
              </A>
            </div>
          </Card>
        </div>
      </div>
    </div>
  )
}

export default Home
