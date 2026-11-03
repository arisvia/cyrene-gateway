import { type Component, For, Show, createSignal, onMount } from 'solid-js'
import { A } from '@solidjs/router'
import { api, apiPost } from '@/lib/api'
import { Card, Button, Input, Field, Select, Modal, StatusPulse, ProviderAvatar, Alert, PageHeader, SegmentedControl, IconPlug, IconSparkles, IconClipboard, TabTransition, Empty } from '@/components/ui'
import { useToast } from '@/lib/toast'
import { copyToClipboard } from '@/lib/clipboard'
import { useI18n } from '@/i18n'

type Cap = 'image' | 'search' | 'tts' | 'stt' | 'video' | 'embeddings'

interface MediaModelEntry {
  id: string
  name: string
  kind: string
}

interface MediaProvider {
  provider: string
  name: string
  kinds: string[]
  models?: MediaModelEntry[]
  hasConnection: boolean
  activeConnections: number
  primaryConnectionId?: string
  color?: string
  website?: string
  apiKeyUrl?: string
}


const Media: Component = () => {
  const toast = useToast()
  const { t } = useI18n()
  const caps = () => [
    { id: 'image' as Cap, label: t('media.caps.image.label'), endpoint: '/v1/images/generations', hint: t('media.caps.image.hint'), kindKey: 'image' },
    { id: 'search' as Cap, label: t('media.caps.search.label'), endpoint: '/v1/search', hint: t('media.caps.search.hint'), kindKey: 'web-search' },
    { id: 'tts' as Cap, label: t('media.caps.tts.label'), endpoint: '/v1/audio/speech', hint: t('media.caps.tts.hint'), kindKey: 'tts' },
    { id: 'stt' as Cap, label: t('media.caps.stt.label'), endpoint: '/v1/audio/transcriptions', hint: t('media.caps.stt.hint'), kindKey: 'stt' },
    { id: 'video' as Cap, label: t('media.caps.video.label'), endpoint: '/v1/videos/generations', hint: t('media.caps.video.hint'), kindKey: 'video' },
    { id: 'embeddings' as Cap, label: t('media.caps.embeddings.label'), endpoint: '/v1/embeddings', hint: t('media.caps.embeddings.hint'), kindKey: 'embedding' },
  ]
  const [active, setActive] = createSignal<Cap>('image')
  const [providers, setProviders] = createSignal<MediaProvider[]>([])
  const [loadingProviders, setLoadingProviders] = createSignal(false)

  // 专属试用弹窗状态
  const [workbenchOpen, setWorkbenchOpen] = createSignal(false)
  const [selectedProvider, setSelectedProvider] = createSignal<MediaProvider | null>(null)
  const [model, setModel] = createSignal('')
  const [text, setText] = createSignal('')
  const [busy, setBusy] = createSignal(false)
  const [result, setResult] = createSignal<unknown>(null)
  const [error, setError] = createSignal('')
  // Video is the only async kind: the POST returns a job id and the render is
  // fetched separately, so the workspace keeps the id around to poll.
  const [videoJobId, setVideoJobId] = createSignal('')
  const [polling, setPolling] = createSignal(false)
  // Bumped on every open/close so a poll started for one job cannot write into
  // the panel after the user moved on.
  let pollToken = 0

  async function loadProviders() {
    setLoadingProviders(true)
    try {
      const cap = caps().find(c => c.id === active())!
      const res = (await api(`/api/media-providers?kind=${cap.kindKey}&connectedOnly=true`)) as { providers?: MediaProvider[] }
      setProviders(res?.providers || [])
    } catch {
      // ignore
    } finally {
      setLoadingProviders(false)
    }
  }

  onMount(() => {
    loadProviders()
  })

  function handleTabChange(capId: Cap) {
    setActive(capId)
    loadProviders()
  }

  function openWorkbench(p: MediaProvider) {
    setSelectedProvider(p)
    setError('')
    setResult(null)
    setVideoJobId('')
    setPolling(false)
    pollToken++
    const defaultM = p.models?.[0]?.id || ''
    setModel(defaultM)
    if (active() === 'image') {
      setText('A futuristic cybernetic city at twilight with glowing neon reflections on wet streets, 8k render, masterpiece')
    } else if (active() === 'video') {
      setText('A slow drone shot over a misty mountain lake at sunrise, cinematic, 4k')
    } else if (active() === 'search') {
      setText('2026 AI coding agents and LLM developments')
    } else {
      setText('Hello, this is a media test from Cyrene Gateway.')
    }
    setWorkbenchOpen(true)
  }
  const currentCap = () => caps().find(c => c.id === active())

  async function copyEndpoint(ep: string) {
    if (!ep) return
    const ok = await copyToClipboard(ep)
    if (ok) {
      toast.success(t('media.endpointCopied', { endpoint: ep }))
    } else {
      toast.info(ep)
    }
  }

  // 提取响应中的图片 URL 或 base64
  const imageUrls = () => {
    const res = result() as Record<string, unknown> | null
    if (!res) return []
    const urls: string[] = []
    if (Array.isArray(res.data)) {
      for (const item of res.data) {
        if (typeof item === 'object' && item !== null) {
          const d = item as Record<string, unknown>
          if (typeof d.url === 'string') urls.push(d.url)
          else if (typeof d.b64_json === 'string') urls.push(`data:image/png;base64,${d.b64_json}`)
        }
      }
    }
    return urls
  }

  // A finished render surfaces as a URL on the job object. xAI nests it under
  // data[], other providers put a single url/video_url at the top level.
  const videoUrls = () => {
    const res = result() as Record<string, unknown> | null
    if (!res) return []
    const urls: string[] = []
    const push = (v: unknown) => {
      if (typeof v === 'string' && v) urls.push(v)
      else if (v && typeof v === 'object') {
        const o = v as Record<string, unknown>
        for (const k of ['url', 'video_url', 'download_url']) {
          if (typeof o[k] === 'string' && o[k]) urls.push(o[k] as string)
        }
      }
    }
    push(res)
    if (Array.isArray(res.data)) res.data.forEach(push)
    return [...new Set(urls)]
  }

  const videoStatus = () => String((result() as Record<string, unknown> | null)?.status || '')

  // 提取搜索结果列表
  const searchResults = () => {
    const res = result() as { summary?: string; count?: number; results?: Array<{ title?: string; url?: string; snippet?: string; position?: number }> } | null
    return res || null
  }

  async function runWorkbench() {
    const p = selectedProvider()
    const cap = caps().find(c => c.id === active())!
    if (!text().trim() || !p) return
    setBusy(true)
    setError('')
    setResult(null)
    try {
      const body: Record<string, unknown> = {
        provider: p.provider,
        input: text(),
        prompt: text(),
        query: text(),
      }
      if (model()) body.model = model()
      const r = await apiPost(cap.endpoint, body)
      setResult(r)
      if (active() === 'video') {
        const id = (r as { id?: string } | null)?.id || ''
        setVideoJobId(id)
        if (id) {
          toast.success(t('media.videoQueued'))
          pollVideo(id)
          return
        }
      }
      toast.success(t('media.callComplete', { cap: cap.label }))
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : t('media.requestFailed')
      setError(msg)
    } finally {
      setBusy(false)
    }
  }

  // Polls a video job until it leaves a pending state. Backs off from 3s to 15s
  // so a slow render does not hammer the upstream for minutes.
  async function pollVideo(jobId: string) {
    const token = ++pollToken
    setPolling(true)
    let delay = 3000
    try {
      for (;;) {
        await new Promise(res => setTimeout(res, delay))
        if (token !== pollToken) return
        const job = (await api(`/v1/videos/${encodeURIComponent(jobId)}`, { silent: true })) as Record<string, unknown> | null
        if (token !== pollToken) return
        setResult(job)
        const status = String(job?.status || '')
        if (!job || !['queued', 'pending', 'processing', 'in_progress', 'running'].includes(status)) {
          if (status === 'failed' || status === 'error') {
            setError(t('media.videoFailed'))
          } else {
            toast.success(t('media.videoReady'))
          }
          return
        }
        delay = Math.min(15000, Math.round(delay * 1.5))
      }
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : t('media.requestFailed'))
    } finally {
      if (token === pollToken) setPolling(false)
    }
  }

  return (
    <div class="space-y-5 stagger">
      <PageHeader
        title={t('media.title')}
        subtitle={t('media.subtitle')}
        actions={
          <A href="/providers?tab=catalog&category=media">
            <Button size="sm" variant="secondary" class="gap-1.5 shrink-0">
              <span>{t('media.marketBtn')}</span>
            </Button>
          </A>
        }
      >
        <div class="pt-2.5 border-t border-subtle/40 flex flex-wrap items-center justify-between gap-3">
          <SegmentedControl
            value={active()}
            onChange={handleTabChange}
            options={caps().map(c => ({ value: c.id, label: c.label }))}
            class="w-fit"
          />

          <div class="flex flex-wrap items-center gap-2.5 text-xs min-w-0 max-w-full">
            <span class="text-faint hidden lg:inline max-w-sm truncate">
              {currentCap()?.hint}
            </span>
            <Button
              size="sm"
              variant="secondary"
              onClick={() => copyEndpoint(currentCap()?.endpoint || '')}
              class="max-w-full min-w-0! font-mono group"
              title={t('media.copyEndpointTitle')}
            >
              <IconClipboard size={12} class="text-accent group-hover:scale-110 transition-transform" />
              <span class="min-w-0 wrap-anywhere whitespace-normal text-left">{currentCap()?.endpoint}</span>
            </Button>
          </div>
        </div>
      </PageHeader>

      {/* 提供商能力主视窗（带双向横向滑动转场过渡） */}
      <TabTransition
        value={active()}
        order={['image', 'search', 'tts', 'stt', 'video', 'embeddings']}
      >
        {() => (
          <div class="space-y-4">
            <Show
              when={providers().length > 0}
              fallback={
                <Show when={!loadingProviders()}>
                  <Card class="p-12">
                    <Empty
                      icon={<IconPlug size={24} />}
                      title={t('media.emptyTitle', { cap: currentCap()?.label || '' })}
                      description={t('media.emptyDesc')}
                      action={
                        <A href="/providers?tab=catalog&category=media">
                          <Button variant="primary" size="sm">
                            {t('media.emptyAction')}
                          </Button>
                        </A>
                      }
                    />
                  </Card>
                </Show>
              }
            >
              <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
                <For each={providers()}>
                  {p => (
                    <Card class="p-4 flex flex-col justify-between space-y-3 hover:border-accent/40 transition">
                      <div class="space-y-2.5">
                        <div class="flex flex-wrap items-center justify-between gap-2 min-w-0">
                          <div class="flex items-center gap-2.5 min-w-0">
                            <ProviderAvatar provider={p.provider} name={p.name} color={p.color} size="md" />
                            <div class="min-w-0">
                              <h3 class="text-sm font-semibold text-foreground truncate">{p.name}</h3>
                              <p class="text-[11px] font-mono text-faint truncate">{p.provider}</p>
                            </div>
                          </div>
                          <div class="flex items-center gap-1.5 shrink-0">
                            <StatusPulse status={p.hasConnection ? 'active' : 'idle'} size="xs" />
                            <span class="text-[11px] text-faint">
                              {p.hasConnection ? t('media.accountsCount', { count: p.activeConnections }) : t('media.notConnected')}
                            </span>
                          </div>
                        </div>

                        <div class="space-y-1">
                          <div class="text-[11px] text-faint">{t('media.supportedModelsEndpoints')}</div>
                          <div class="flex flex-wrap gap-1 max-h-24 overflow-y-auto px-1 py-0.5 min-w-0">
                            <Show
                              when={p.models && p.models.length > 0}
                              fallback={
                                <span class="text-[11px] font-mono text-muted bg-surface-inset px-2 py-0.5 rounded border border-subtle max-w-full min-w-0 wrap-anywhere">
                                  {t('media.defaultEndpoint')}
                                </span>
                              }
                            >
                              <For each={p.models}>
                                {m => (
                                  <span class="text-xs font-mono text-foreground bg-surface-inset px-2 py-0.5 rounded border border-subtle max-w-full min-w-0 wrap-anywhere">
                                    {m.name || m.id}
                                  </span>
                                )}
                              </For>
                            </Show>
                          </div>
                        </div>
                      </div>

                      <div class="pt-2.5 border-t border-subtle flex flex-wrap items-center justify-between gap-2">
                        <span class="text-[11px] text-faint">
                          {t('media.readyToCall')}
                        </span>
                        <div class="flex flex-wrap items-center gap-2 min-w-0">
                          <Show when={p.primaryConnectionId}>
                            <A href={`/providers/${p.primaryConnectionId}`}>
                              <Button size="sm" variant="secondary" title={t('media.manageAccountTitle')}>
                                {t('media.manage')}
                              </Button>
                            </A>
                          </Show>
                          <Button
                            size="sm"
                            variant="primary"
                            onClick={() => openWorkbench(p)}
                          >
                            {t('media.tryWorkbench')} {currentCap()?.label}
                          </Button>
                        </div>
                      </div>
                    </Card>
                  )}
                </For>
              </div>
            </Show>
          </div>
        )}
      </TabTransition>
      <Modal
        open={workbenchOpen()}
        title={`${selectedProvider()?.name || ''} · ${caps().find(c => c.id === active())?.label || ''}`}
        onClose={() => { pollToken++; setWorkbenchOpen(false); setPolling(false) }}
      >
        <div class="space-y-4 min-w-0">
          <Show when={selectedProvider()?.models && selectedProvider()!.models!.length > 0}>
            <Field label={t('media.selectModel')}>
              <Select
                value={model()}
                onChange={setModel}
                options={(selectedProvider()?.models || []).map(m => ({
                  value: m.id,
                  label: m.name || m.id,
                }))}
              />
            </Field>
          </Show>

          <Field label={active() === 'image' ? t('media.promptLabelImage') : active() === 'search' ? t('media.promptLabelSearch') : t('media.promptLabelDefault')}>
            <Input
              value={text()}
              onInput={setText}
              placeholder={active() === 'image' ? t('media.placeholderImage') : t('media.placeholderDefault')}
            />
          </Field>

          <div class="flex justify-end gap-2">
            <Button size="sm" variant="secondary" onClick={() => setWorkbenchOpen(false)}>
              {t('common.close')}
            </Button>
            <Button size="sm" variant="primary" loading={busy()} disabled={!text().trim()} onClick={runWorkbench}>
              {t('media.runBtn')}
            </Button>
          </div>

          <Show when={error()}>
            <Alert variant="danger">{error()}</Alert>
          </Show>

          {/* 生图结果预览画廊 */}
          <Show when={imageUrls().length > 0}>
            <div class="space-y-2 pt-2 border-t border-subtle">
              <div class="flex flex-wrap items-center justify-between gap-2 min-w-0">
                <span class="text-xs font-semibold text-foreground">{t('media.resultsPreview')}</span>
                <span class="text-[11px] text-faint">{t('media.imageCount', { count: imageUrls().length })}</span>
              </div>
              <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
                <For each={imageUrls()}>
                  {(url, idx) => (
                    <div class="relative group rounded-card overflow-hidden border border-subtle bg-bg-elevated aspect-square flex items-center justify-center">
                      <img src={url} alt={`Generated result ${idx() + 1}`} class="w-full h-full object-cover transition duration-300 group-hover:scale-105" />
                      <div class="absolute inset-0 bg-overlay/40 opacity-100 [@media(hover:hover)]:opacity-0 group-hover:opacity-100 group-focus-within:opacity-100 transition-opacity flex items-center justify-center gap-2">
                        <a href={url} target="_blank" rel="noreferrer" class="inline-flex min-h-11 items-center px-4 py-2 rounded-control text-sm bg-card text-foreground font-medium shadow-sm hover:bg-hover transition-colors focus-visible:outline-2 focus-visible:outline-ring focus-visible:outline-offset-2">
                          {t('media.viewOriginal')}
                        </a>
                      </div>
                    </div>
                  )}
                </For>
              </div>
            </div>
          </Show>

          {/* 联网搜索结果卡片流 */}
          <Show when={searchResults() && searchResults()!.results && searchResults()!.results!.length > 0}>
            <div class="space-y-2.5 pt-2 border-t border-subtle">
              <Show when={searchResults()?.summary}>
                <div class="p-3 rounded-card bg-surface-inset text-sm text-foreground leading-relaxed wrap-anywhere">
                  <div class="font-semibold text-accent mb-1 flex items-center gap-1.5">
                    <IconSparkles size={14} />
                    <span>{t('media.aiSummary')}</span>
                  </div>
                  {searchResults()?.summary}
                </div>
              </Show>

              <div class="text-xs font-semibold text-foreground">{t('media.citations', { count: searchResults()?.count || searchResults()?.results?.length || 0 })}</div>
              <div class="space-y-2">
                <For each={searchResults()?.results}>
                  {item => (
                    <div class="p-3 rounded-card bg-bg-elevated border border-subtle text-xs space-y-1 hover:border-accent/30 transition">
                      <div class="flex flex-wrap items-center justify-between gap-2 min-w-0">
                        <a href={item.url} target="_blank" rel="noreferrer" class="min-w-0 font-medium text-accent hover:underline wrap-anywhere focus-visible:outline-2 focus-visible:outline-ring">
                          <span>{item.position}. {item.title || item.url}</span>
                        </a>
                      </div>
                      <p class="text-xs text-faint wrap-anywhere font-mono">{item.url}</p>
                    </div>
                  )}
                </For>
              </div>
            </div>
          </Show>

          {/* 视频任务状态与成品预览（异步渲染，先出 id 后出地址） */}
          <Show when={active() === 'video'}>
            <div class="space-y-2 pt-2 border-t border-subtle">
              <div class="flex flex-wrap items-center justify-between gap-2 min-w-0">
                <span class="text-xs font-semibold text-foreground">{t('media.caps.video.label')}</span>
                <span class="text-[11px] font-mono text-faint wrap-anywhere">
                  {videoJobId() ? `${videoJobId()} · ${videoStatus() || 'queued'}` : t('media.videoIdle')}
                </span>
              </div>
              <Show when={polling()}>
                <div class="flex items-center gap-2 text-[11px] text-faint">
                  <StatusPulse tone="amber" size="xs" />
                  <span>{t('media.videoPolling', { sec: 3 })}</span>
                </div>
              </Show>
              <Show when={videoUrls().length > 0}>
                <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
                  <For each={videoUrls()}>
                    {url => (
                      <div class="rounded-card overflow-hidden border border-subtle bg-bg-elevated">
                        <video src={url} controls playsinline class="w-full h-auto" />
                        <a href={url} target="_blank" rel="noreferrer" class="block px-3 py-2 text-xs text-accent hover:underline">
                          {t('media.openVideo')}
                        </a>
                      </div>
                    )}
                  </For>
                </div>
              </Show>
            </div>
          </Show>

          {/* 原始响应 JSON 调试折叠面板 */}
          <Show when={result()}>
            <div class="pt-2 border-t border-subtle space-y-1.5">
              <div class="text-[11px] font-semibold text-faint uppercase">{t('media.rawJson')}</div>
              <pre class="text-xs leading-relaxed font-mono text-foreground whitespace-pre-wrap wrap-anywhere bg-surface-inset p-3 rounded-control max-h-72 overflow-y-auto min-w-0">
                {JSON.stringify(result(), null, 2)}
              </pre>
            </div>
          </Show>
        </div>
      </Modal>
    </div>
  )
}

export default Media
