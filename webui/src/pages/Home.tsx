import { type Component, For, Show, createSignal, onMount } from 'solid-js'
import { useGatewayStore } from '@/stores/gateway'
import { useI18n } from '@/i18n'
import { Card, Badge, Empty, Button, Input, IconCheck, IconEdit, IconLink, IconKey, Modal, Field, confirm } from '@/components/ui'
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
      toast.error(t('toast.saveKeyFailed'))
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
                <Badge tone="green" class="font-medium">{t('nav.running')}</Badge>
              </div>
              <p class="text-xs text-faint mt-1 flex items-center gap-3">
                <span>{t('home.heroSubtitle')}</span>
              </p>
            </div>
          </div>

          <div class="flex items-center gap-3 flex-wrap sm:flex-nowrap">
            <div class="flex items-center gap-3 px-4 py-2.5 rounded-2xl bg-card/60 border border-subtle backdrop-blur-md">
              <div class="text-right">
                <div class="text-base font-bold text-foreground leading-none">{store.activeConnections()}</div>
                <div class="text-[11px] text-faint mt-0.5">{t('home.activeChannels')}</div>
              </div>
              <div class="w-2 h-2 rounded-full bg-accent animate-pulse" />
            </div>
            <div class="flex items-center gap-3 px-4 py-2.5 rounded-2xl bg-card/60 border border-subtle backdrop-blur-md">
              <div class="text-right">
                <div class="text-base font-bold text-foreground leading-none">{store.combos().length}</div>
                <div class="text-[11px] text-faint mt-0.5">{t('home.fallbackCombos')}</div>
              </div>
              <div class="w-2 h-2 rounded-full bg-accent-2 animate-pulse" />
            </div>
          </div>
        </div>
      </Card>

      {/* 核心功能区：API 密钥管理 + 端点与快速接入 */}
      <div class="grid grid-cols-1 lg:grid-cols-12 gap-6">
        {/* 网关统一端点与快捷客户端配置 (占据 5 列) */}
        <div class="lg:col-span-5 space-y-6">
          {/* 端点卡片 */}
          <Card class="p-6 space-y-4">
            <div class="flex items-center justify-between">
              <h2 class="text-base font-semibold text-foreground flex items-center gap-2">
                <IconLink size={16} class="text-accent" />
                <span>{t('home.unifiedEndpoints')}</span>
              </h2>
              <Badge tone="blue">{t('home.multiProtocol')}</Badge>
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
                          title={t('home.clickToCopy')}
                      >
                        <div class="min-w-0 flex-1">
                          <div class="text-sm font-medium text-foreground">{ep.label}</div>
                          <code class="text-xs text-faint truncate block font-mono mt-0.5">{ep.url}</code>
                        </div>
                        <span class="text-xs text-muted group-hover:text-accent font-medium shrink-0 flex items-center gap-1">
                        {isCopied() ? <><span>{t('common.copied')}</span><IconCheck size={12} class="text-success" /></> : t('common.copy')}
                      </span>
                      </button>
                  )
                }}
              </For>
              <Show when={store.endpoints().length === 0}>
                <Empty message={t('home.noEndpoints')} />
              </Show>
            </div>
          </Card>
        </div>

        {/* API 密钥一等公民控制区 (占据 7 列) */}
        <div class="lg:col-span-7 space-y-6">
          <Card class="p-6 space-y-5">
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
            <div class="flex gap-2 p-1.5 rounded-2xl bg-card/50 border border-subtle transition-colors">
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
                        toast.error(t('toast.createKeyFailed'))
                      } finally {
                        setCreatingKey(false)
                      }
                    }
                  }}
                  disabled={creatingKey()}
                  class="border-0! bg-transparent! shadow-none! focus:ring-0! text-sm"
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
                      toast.error(t('toast.createKeyFailed'))
                    } finally {
                      setCreatingKey(false)
                    }
                  }}
              >
                {t('home.createKeyBtn')}
              </Button>
            </div>

            {/* 密钥列表展示 */}
            <div class="space-y-2.5">
              <Show
                  when={store.apiKeys().length > 0}
                  fallback={
                    <div class="py-8 text-center border border-dashed border-subtle rounded-2xl">
                      <p class="text-xs text-faint">{t('home.noKeys')}</p>
                    </div>
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
                              <Badge tone="gray" class="text-[10px] scale-95">Bearer</Badge>
                              <Show
                                when={k.allowedModels && k.allowedModels.length > 0}
                                fallback={<Badge tone="gray" class="text-[10px]">{t('home.allModels')}</Badge>}
                              >
                                <Badge tone="blue" class="text-[10px]">
                                  {t('home.modelsCount', { count: k.allowedModels?.length ?? 0 })}
                                </Badge>
                              </Show>
                              <Show when={k.rpm && k.rpm > 0}>
                                <Badge tone="amber" class="text-[10px]">{k.rpm} RPM</Badge>
                              </Show>
                              <Show when={k.systemPrompt}>
                                <span title={k.systemPrompt}>
                                  <Badge tone="blue" class="text-[10px]">{t('home.injectedContext')}</Badge>
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
            <textarea
              class="w-full h-24 p-2.5 text-xs rounded-control bg-bg-elevated border border-subtle focus:border-accent font-mono resize-y mt-1 text-foreground"
              value={editSystemPrompt()}
              onInput={e => setEditSystemPrompt(e.currentTarget.value)}
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
