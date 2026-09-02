import { type Component, For, Show, createSignal, createResource, createMemo } from 'solid-js'
import { api } from '@/lib/api'
import type { Skill } from '@/types/domain'
import { Card, Badge, Button, Input, Empty, Skeleton } from '@/components/ui'

const Skills: Component = () => {
  const [data] = createResource(async () => {
    try { return await api('/api/skills') as { count?: number; skills?: Skill[] } | null } catch { return null }
  })
  const [q, setQ] = createSignal('')
  const [expanded, setExpanded] = createSignal<string | null>(null)

  const filtered = createMemo(() => {
    const list = data()?.skills ?? []
    const kw = q().toLowerCase().trim()
    if (!kw) return list
    return list.filter((s: Skill) =>
      (s.name || '').toLowerCase().includes(kw) || (s.description || '').toLowerCase().includes(kw))
  })

  async function copy(s: Skill) {
    try {
      await navigator.clipboard.writeText(s.content || '')
    } catch { /* 剪贴板不可用时忽略 */ }
  }

  return (
    <div class="space-y-5 stagger">
      <div>
        <h1 class="text-xl font-semibold">技能清单</h1>
        <p class="text-sm text-faint mt-0.5">
          供 AI 客户端发现的 cyrene-* 技能{data()?.count ? ` · 共 ${data()?.count} 个` : ''}
        </p>
      </div>

      <Input class="!w-72" placeholder="搜索技能…" value={q()} onInput={setQ} />

      <Show when={!data.loading} fallback={<Card class="p-6"><Skeleton class="h-40 w-full" /></Card>}>
        <Show when={filtered().length > 0} fallback={<Card class="p-6"><Empty message="没有匹配的技能。" /></Card>}>
          <div class="grid gap-3">
            <For each={filtered()}>
              {s => (
                <Card hover class="p-4">
                  <div class="flex items-start justify-between gap-3">
                    <div class="min-w-0 flex-1">
                      <div class="flex items-center gap-2">
                        <span class="font-mono text-sm text-accent">{s.name}</span>
                        <Show when={s.version}><Badge tone="gray">v{s.version}</Badge></Show>
                      </div>
                      <p class="text-xs text-faint mt-1">{s.description}</p>
                    </div>
                    <div class="flex gap-1.5 shrink-0">
                      <Button size="sm" variant="ghost" onClick={() => setExpanded(expanded() === s.id ? null : s.id)}>
                        {expanded() === s.id ? '收起' : '查看'}
                      </Button>
                      <Button size="sm" variant="secondary" onClick={() => copy(s)}>复制</Button>
                    </div>
                  </div>
                  <Show when={expanded() === s.id && s.content}>
                    <pre class="mt-3 text-[11px] font-mono text-muted whitespace-pre-wrap break-all bg-bg-elevated p-3 rounded-control border border-subtle max-h-80 overflow-y-auto">
                      {s.content}
                    </pre>
                  </Show>
                </Card>
              )}
            </For>
          </div>
        </Show>
      </Show>
    </div>
  )
}

export default Skills
