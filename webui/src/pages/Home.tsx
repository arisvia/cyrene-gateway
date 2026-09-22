import { type Component, For, Show, createSignal, onMount } from 'solid-js'
import { useGatewayStore } from '@/stores/gateway'
import { useI18n } from '@/i18n'
import { Card, Badge, Empty, PageHeader, Button, Input, Textarea, IconCheck, IconEdit, IconLink, IconKey, Modal, Field, Skeleton, LoadState, confirm } from '@/components/ui'
import type { ApiKey } from '@/types/domain'
import { useToast } from '@/lib/toast'
import { copyToClipboard } from '@/lib/clipboard'

const Home: Component = () => {
  const store = useGatewayStore()
  const { t } = useI18n()
  const toast = useToast()

  const [keyName, setKeyName] = createSignal('')
  const [creatingKey, setCreatingKey] = createSignal(false)
  const [copiedKeyId, setCopiedKeyId] = createSignal<string | null>(null)
  const [copiedEndpoint, setCopiedEndpoint] = createSignal<string | null>(null)

  const effectiveEndpoints = () => {
    const list = store.endpoints()
    const currentOrigin = typeof window !== 'undefined' ? window.location.origin : ''
    if (!currentOrigin) return list

    if (list.some(e => e.type === 'current')) {
      return list
    }

    const existing = list.find(e => e.url === currentOrigin)
    if (existing) {
      return [
        { ...existing, label: `${t('home.currentEndpoint')} (${window.location.host})`, type: 'current' },
        ...list.filter(e => e !== existing),
      ]
    }

    return [
      {
        label: `${t('home.currentEndpoint')} (${window.location.host})`,
        url: currentOrigin,
        type: 'current',
      },
      ...list,
    ]
  }
  // 细粒度规则编辑状态
  const [editingKey, setEditingKey] = createSignal<ApiKey | null>(null)
  const [editName, setEditName] = createSignal('')
  const [editAllowedModels, setEditAllowedModels] = createSignal('')
  const [editRpm, setEditRpm] = createSignal(0)
  const [editSystemPrompt, setEditSystemPrompt] = createSignal('')
  const [savingEdit, setSavingEdit] = createSignal(false)

  const openEdit = (k: ApiKey) => {
    setEditingKey(k)
    setEditName(k.name || '')
    setEditAllowedModels((k.allowedModels || []).join(', '))
    setEditRpm(k.rpm || 0)
    setEditSystemPrompt(k.systemPrompt || '')
  }

  const saveKeyEdit = async () => {
    const k = editingKey()
    if (!k) return
    setSavingEdit(true)
    try {
      const models = editAllowedModels()
        .split(',')
        .map(m => m.trim())
        .filter(Boolean)
      await store.updateKey(k.id, {
        name: editName().trim(),
        allowedModels: models,
        rpm: editRpm(),
        systemPrompt: editSystemPrompt().trim(),
      })
      setEditingKey(null)
    } catch (err) {
      console.error(err)
    } finally {
      setSavingEdit(false)
    }
  }
  onMount(() => {
    store.loadKeys()
  })

  const copyText = async (text: string, label: string) => {
    const ok = await copyToClipboard(text)
    if (ok) {
      toast.success(t('toast.copySuccess', { label }))
    } else {
      toast.info(text)
    }
  }

  return (
    <div class="space-y-6 stagger">
      <PageHeader
        title={t('nav.home')}
        subtitle={t('home.heroSubtitle')}
        badge={<Badge tone="green">{t('nav.running')}</Badge>}
        actions={
          <div class="flex items-center gap-6">
            <div class="flex items-baseline gap-2">
              <span class="text-xl font-semibold tabular-nums">{store.activeConnections()}</span>
              <span class="text-xs text-muted">{t('home.activeChannels')}</span>
            </div>
            <div class="flex items-baseline gap-2">
              <span class="text-xl font-semibold tabular-nums">{store.combos().length}</span>
              <span class="text-xs text-muted">{t('home.fallbackCombos')}</span>
            </div>
          </div>
        }
      />

      {/* 核心功能区：API 密钥管理 + 端点与快速接入 */}
      <div class="grid grid-cols-1 lg:grid-cols-12 gap-6">
        {/* 网关统一端点与快捷客户端配置 (占据 5 列) */}
        <div class="lg:col-span-5 space-y-6">
          {/* 端点卡片 */}
          <Card class="p-4 sm:p-6 space-y-4">
            <div class="flex items-center justify-between">
              <h2 class="text-base font-semibold text-foreground flex items-center gap-2">
                <IconLink size={16} class="text-accent" />
                <span>{t('home.unifiedEndpoints')}</span>
              </h2>
              <Badge tone="blue">{t('home.multiProtocol')}</Badge>
            </div>

            <LoadState ready={store.loaded.endpoints} error={store.loadErrors.endpoints} onRetry={() => store.loadCore()} fallback={
              <div class="space-y-2.5">
                <For each={[1, 2, 3]}>
                  {() => (
                    <div class="rounded-xl border border-subtle px-3.5 py-3 bg-card/40 space-y-2">
                      <Skeleton class="h-4 w-32" />
                      <Skeleton class="h-3 w-48" />
                    </div>
                  )}
                </For>
              </div>
            }>
            <div class="space-y-2.5">
              <For each={effectiveEndpoints()}>
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
                          title={t('home.clickToCopy')}
                      >
                        <div class="min-w-0 flex-1">
                          <div class="text-sm font-medium text-foreground flex items-center gap-2">
                            <span class="truncate">{ep.label}</span>
                            <Show when={ep.type === 'current'}>
                              <Badge tone="green" class="text-[10px] py-0 px-1.5 shrink-0">
                                {t('home.currentTag')}
                              </Badge>
                            </Show>
                          </div>
                          <code class="text-xs text-faint truncate block font-mono mt-0.5">{ep.url}</code>
                        </div>
                        <span class="text-xs text-muted group-hover:text-accent font-medium shrink-0 flex items-center gap-1">
                        {isCopied() ? <><span>{t('common.copied')}</span><IconCheck size={12} class="text-success" /></> : t('common.copy')}
                      </span>
                      </button>
                  )
                }}
              </For>
              <Show when={effectiveEndpoints().length === 0}>
                <Empty message={t('home.noEndpoints')} />
              </Show>
            </div>
            </LoadState>
          </Card>
        </div>

        {/* API 密钥一等公民控制区 (占据 7 列) */}
        <div class="lg:col-span-7 space-y-6">
          <Card class="p-4 sm:p-6 space-y-5">
            <div class="flex items-center justify-between">
              <div>
                <h2 class="text-base font-semibold text-foreground flex items-center gap-2">
                  <IconKey size={16} class="text-accent" />
                  <span>{t('home.apiKeys')}</span>
                </h2>
                <p class="text-xs text-faint mt-0.5">{t('home.apiKeysHint')}</p>
              </div>
              <Button size="sm" variant="secondary" onClick={() => store.loadKeys()}>
                {t('common.refresh')}
              </Button>
            </div>

            {/* 创建新 Key 输入框 */}
            <div class="flex flex-col sm:flex-row gap-2">
              <Input
                  value={keyName()}
                  placeholder={t('home.createKeyPlaceholder')}
                  onInput={setKeyName}
                  onKeyDown={async e => {
                    if (e.key === 'Enter' && keyName().trim() && !creatingKey()) {
                      const n = keyName().trim()
                      setCreatingKey(true)
                      try {
                        await store.createKey(n)
                        setKeyName('')
                      } catch (err) {
                        console.error(err)
                      } finally {
                        setCreatingKey(false)
                      }
                    }
                  }}
                  disabled={creatingKey()}
                  class="min-w-0 sm:flex-1"
                  ariaLabel={t('home.keyName')}
              />
              <Button
                  variant="primary"
                  class="shrink-0"
                  loading={creatingKey()}
                  disabled={!keyName().trim()}
                  onClick={async () => {
                    const n = keyName().trim()
                    if (!n) return
                    setCreatingKey(true)
                    try {
                      await store.createKey(n)
                      setKeyName('')
                    } catch (err) {
                      console.error(err)
                    } finally {
                      setCreatingKey(false)
                    }
                  }}
              >
                {t('home.createKeyBtn')}
              </Button>
            </div>

            {/* 密钥列表展示 */}
            <LoadState ready={store.loaded.keys} error={store.loadErrors.keys} onRetry={() => store.loadKeys()} fallback={
              <div class="space-y-2.5">
                <For each={[1, 2]}>
                  {() => (
                    <div class="p-3.5 rounded-xl border border-subtle bg-card/40 space-y-2">
                      <div class="flex items-center gap-2">
                        <Skeleton class="h-4 w-24" />
                        <Skeleton class="h-4 w-12 rounded-full" />
                      </div>
                      <Skeleton class="h-3 w-40" />
                    </div>
                  )}
                </For>
              </div>
            }>
            <div class="space-y-2.5">
              <Show
                  when={store.apiKeys().length > 0}
                  fallback={
                    <Empty message={t('home.noKeys')} class="py-10" />
                  }
              >
                <For each={store.apiKeys()}>
                  {k => {
                    const isCopied = () => copiedKeyId() === k.id
                    return (
                        <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-3 p-3.5 rounded-xl border border-subtle bg-card/40 hover:bg-card hover:border-accent/30 transition-all group">
                          <div class="min-w-0 flex-1 space-y-1">
                            <div class="flex items-center gap-2 flex-wrap">
                              <span class="text-sm font-medium text-foreground truncate">{k.name || t('home.unnamedKey')}</span>
                              <Badge tone="gray" class="text-xs">Bearer</Badge>
                              <Show
                                when={k.allowedModels && k.allowedModels.length > 0}
                                fallback={<Badge tone="gray" class="text-xs">{t('home.allModels')}</Badge>}
                              >
                                <Badge tone="blue" class="text-xs">
                                  {t('home.modelsCount', { count: k.allowedModels?.length ?? 0 })}
                                </Badge>
                              </Show>
                              <Show when={k.rpm && k.rpm > 0}>
                                <Badge tone="amber" class="text-xs">{k.rpm} RPM</Badge>
                              </Show>
                              <Show when={k.systemPrompt}>
                                <span title={k.systemPrompt}>
                                  <Badge tone="blue" class="text-xs">{t('home.injectedContext')}</Badge>
                                </span>
                              </Show>
                            </div>
                            <div class="flex items-center gap-2 mt-0.5">
                              <code class="text-xs text-muted font-mono truncate max-w-full sm:max-w-70 select-all">
                                {k.key}
                              </code>
                            </div>
                          </div>

                          <div class="flex items-center gap-1.5 shrink-0 self-end sm:self-center">
                            <Button
                                size="sm"
                                variant="secondary"
                                onClick={() => openEdit(k)}
                                title={t('home.editRulesTitle')}
                            >
                              <span class="flex items-center gap-1">
                                <IconEdit size={12} class="text-muted" />
                                <span>{t('home.rulesBtn')}</span>
                              </span>
                            </Button>
                            <Button
                                size="sm"
                                variant="secondary"
                                onClick={() => {
                                  copyText(k.key, t('home.copiedKey', { name: k.name || '' }))
                                  setCopiedKeyId(k.id)
                                  setTimeout(() => setCopiedKeyId(null), 2000)
                                }}
                            >
                              {isCopied() ? <span class="flex items-center gap-1"><span>{t('common.copied')}</span><IconCheck size={12} class="text-success" /></span> : t('home.copyKey')}
                            </Button>
                            <Button
                                size="sm"
                                variant="danger"
                                onClick={async () => {
                                  const ok = await confirm({
                                    title: t('home.deleteKeyConfirmTitle'),
                                    message: t('home.deleteKeyConfirmMessage', { name: k.name || k.key }),
                                    variant: 'danger',
                                  })
                                  if (ok) {
                                    await store.deleteKey(k.id)
                                  }
                                }}
                            >
                              {t('common.delete')}
                            </Button>
                          </div>
                        </div>
                    )
                  }}
                </For>
              </Show>
            </div>
            </LoadState>
          </Card>
        </div>
      </div>

      {/* 细粒度规则配置弹窗 */}
      <Modal
        open={!!editingKey()}
        title={t('home.editKeyTitle')}
        onClose={() => setEditingKey(null)}
      >
        <div class="space-y-4">
          <Field label={t('home.keyName')} hint={t('home.keyNameHint')}>
            <Input
              value={editName()}
              onInput={setEditName}
              placeholder={t('home.keyNamePlaceholder')}
              class="w-full mt-1"
            />
          </Field>

          <Field
            label={t('home.allowedModels')}
            hint={t('home.allowedModelsHint')}
          >
            <Input
              value={editAllowedModels()}
              onInput={setEditAllowedModels}
              placeholder={t('home.allowedModelsPlaceholder')}
              class="w-full mt-1 font-mono text-xs"
            />
          </Field>

          <Field
            label={t('home.keyRpm')}
            hint={t('home.keyRpmHint')}
          >
            <Input
              type="number"
              value={String(editRpm())}
              onInput={v => setEditRpm(Math.max(0, Number(v) || 0))}
              class="w-full sm:w-36 mt-1"
            />
          </Field>

          <Field
            label={t('home.systemContext')}
            hint={t('home.systemContextHint')}
          >
            <Textarea
              class="h-24 p-2.5 text-xs font-mono resize-y mt-1"
              value={editSystemPrompt()}
              onInput={setEditSystemPrompt}
              placeholder={t('home.systemContextPlaceholder')}
            />
          </Field>

          <div class="flex items-center justify-end gap-2 pt-2 border-t border-subtle/50">
            <Button variant="secondary" onClick={() => setEditingKey(null)}>
              {t('common.cancel')}
            </Button>
            <Button variant="primary" loading={savingEdit()} onClick={saveKeyEdit}>
              {savingEdit() ? t('common.saving') : t('common.save')}
            </Button>
          </div>
        </div>
      </Modal>
    </div>
  )
}

export default Home
