import { type Component, For, Show, createSignal } from 'solid-js'
import { useI18n } from '@/i18n'
import type { ProviderModelsApi } from '@/lib/providerModels'
import { Badge, Button, Card, Empty, IconButton, IconCheck, IconClose, IconEdit, IconLock, IconZap, Input, Toggle } from '@/components/ui'

const ModelsPanel: Component<{ models: ProviderModelsApi }> = props => {
  const { t } = useI18n()
  const {
    payload,
    sortedRegistryModels,
    sortedCustomModels,
    customCount,
    showCustomModels,
    setShowCustomModels,
    modelSearch,
    testingModels,
    modelTestResults,
    toggleModel,
    testSingleModel,
    addCustomModel,
    removeCustomModel,
    startEditModel
  } = props.models

  const [newCustomModelId, setNewCustomModelId] = createSignal('')


  return (
  <div class="flex flex-col gap-3.5">
    <Card class="p-3.5 sm:p-4">
      <div class="flex items-center justify-between gap-3">
        <div class="flex items-center gap-2 flex-wrap min-w-0">
          <h3 class="text-sm font-semibold whitespace-nowrap">{t('providerDetail.customModelsTitle')}</h3>
          <Badge tone="gray" class="text-[10px] font-mono px-1.5 py-0.2 shrink-0">
            {customCount()}
          </Badge>
          <span class="text-xs text-faint truncate hidden sm:inline">{t('providerDetail.customModelsDesc')}</span>
        </div>
        <Button
          size="sm"
          variant="ghost"
          class="text-xs shrink-0"
          onClick={() => setShowCustomModels(s => !s)}
        >
          {showCustomModels() ? t('common.collapse') : (customCount() > 0 ? t('common.expand') : `+ ${t('common.add')}`)}
        </Button>
      </div>
      <Show when={showCustomModels()}>
        <div class="pt-3 border-t border-subtle/50 mt-2.5 space-y-2.5 animate-fade-in">
          <div class="flex gap-2">
            <Input
              value={newCustomModelId()}
              onInput={setNewCustomModelId}
              placeholder={t('providerDetail.customModelPlaceholder')}
              size="sm"
              onKeyDown={e => {
                if (e.key === 'Enter') {
                  e.preventDefault()
                  const val = newCustomModelId().trim()
                  if (val) { addCustomModel(val, ''); setNewCustomModelId('') }
                }
              }}
            />
            <Button
              size="sm"
              variant="secondary"
              disabled={!newCustomModelId().trim()}
              onClick={() => {
                const val = newCustomModelId().trim()
                if (val) { addCustomModel(val, ''); setNewCustomModelId('') }
              }}
            >
              {t('common.add')}
            </Button>
          </div>
          <div class="flex flex-wrap gap-2">
            <Show when={customCount() > 0} fallback={
              <span class="text-xs text-faint">{t('providerDetail.noCustomModels')}</span>
            }>
              <For each={sortedCustomModels()}>
                {m => (
                  <span class={`inline-flex flex-wrap max-w-full min-w-0 items-center gap-2 px-2.5 py-1 rounded-control border text-xs wrap-anywhere transition-colors ${
                    m.enabled !== false ? 'bg-control border-subtle text-foreground' : 'bg-surface-inset border-subtle/50 text-faint opacity-60'
                  }`}>
                    <span class={`font-mono ${m.enabled === false ? 'line-through' : ''}`}>{m.name || m.id}</span>
                    <IconButton size="sm" variant="ghost"
                      type="button"
                      class="text-faint hover:text-accent"
                      title={t('providerDetail.editModelMetaTitle')}
                      onClick={() => startEditModel(m)}
                    >
                      <IconEdit size={12} />
                    </IconButton>
                    <Toggle
                      ariaLabel={`${t('common.enabled')}: ${m.name || m.id}`}
                      checked={m.enabled !== false}
                      onChange={() => toggleModel(m.id || m.name, m.enabled !== false)}
                    />
                    <IconButton size="sm" variant="ghost" class="text-faint hover:text-danger ml-0.5" title={t('providerDetail.deleteModelTitle2')} onClick={() => removeCustomModel(m.id || m.name)}><IconClose size={12} /></IconButton>
                  </span>
                )}
              </For>
            </Show>
          </div>
        </div>
      </Show>
    </Card>

    <Card class="p-4 sm:p-4.5 space-y-3">
      <div class="flex items-center justify-between gap-2 flex-wrap text-xs text-faint">
        <div class="flex flex-wrap items-center gap-2 min-w-0">
          <h3 class="text-sm font-semibold text-foreground">{t('providerDetail.availableModels')}</h3>
          <span>
            {t('providerDetail.modelsCountSummary', {
              total: (payload().registryModels ?? []).length,
              enabled: (payload().registryModels ?? []).filter(m => m.enabled !== false).length,
            })}
          </span>
        </div>
        <Show when={modelSearch().trim()}>
          <span class="text-accent font-mono">
            {(() => {
              const count = (payload().registryModels ?? []).filter(m => {
                const q = modelSearch().trim().toLowerCase()
                return (m.name || '').toLowerCase().includes(q) || (m.id || '').toLowerCase().includes(q)
              }).length
              return `${count} / ${(payload().registryModels ?? []).length}`
            })()}
          </span>
        </Show>
      </div>
      <Show
        when={(payload().registryModels ?? []).length > 0}
        fallback={<Empty message={t('providerDetail.noModels')} />}
      >
        <div class="grid sm:grid-cols-2 lg:grid-cols-3 gap-2.5">
            <For each={sortedRegistryModels().filter(m => {
              const q = modelSearch().trim().toLowerCase()
              if (!q) return true
              return (m.name || '').toLowerCase().includes(q) || (m.id || '').toLowerCase().includes(q)
            })}>
              {m => (
                <div
                  class={`flex flex-col justify-between p-3 rounded-control border transition-all ${
                    m.enabled !== false
                      ? 'border-subtle bg-bg-elevated/70 hover:border-accent/40 shadow-sm'
                      : 'border-subtle/30 bg-bg-elevated/25 opacity-55 hover:opacity-80'
                  }`}
                >
                  <div class="flex flex-wrap items-start justify-between gap-2">
                    <div class="min-w-0 flex-1 pr-1">
                      <div class="flex items-center gap-1.5 flex-wrap">
                        <span class={`text-xs truncate font-medium ${m.enabled !== false ? 'text-foreground' : 'text-faint line-through'}`}>
                          {m.name || m.id}
                        </span>
                        <Show when={m.isFree}>
                          <span class="px-1.5 py-0.5 rounded text-[10px] font-medium bg-success/12 text-success border border-success/30 shrink-0">
                            {t('providerDetail.freeBadge')}
                          </span>
                        </Show>
                        <Show when={m.priceFactor !== undefined && m.priceFactor !== null && m.priceFactor > 0 && !m.isFree}>
                          <span class="px-1.5 py-0.5 rounded text-[10px] font-mono font-medium bg-accent/10 text-accent border border-accent/25 shrink-0" title={t('providerDetail.priceFactorTitle', { factor: m.priceFactor! })}>
                            {m.priceFactor}x
                          </span>
                        </Show>
                        <Show when={m.enabled === false}>
                          <span class="px-1.5 py-0.5 rounded text-[10px] font-medium bg-control text-muted border border-subtle shrink-0">
                            {t('providerDetail.chipDisabled')}
                          </span>
                        </Show>
                        <Show when={m.hasOverride}>
                          <span class="px-1.5 py-0.5 rounded text-[10px] font-medium bg-accent/15 text-accent border border-accent/30 shrink-0" title={t('providerDetail.customMetaChipTitle')}>
                            {t('providerDetail.chipCustom')}
                          </span>
                        </Show>
                      </div>
                      <div class="text-[11px] text-faint font-mono truncate mt-0.5">{m.id || m.name}</div>
                    </div>
                    <div class="shrink-0 flex items-center gap-1">
                      <IconButton size="sm" variant="ghost"
                        type="button"
                        class={`transition-colors ${testingModels()[m.id || m.name || ''] ? 'text-accent animate-spin cursor-wait' : 'text-faint hover:text-accent cursor-pointer'}`}
                        title={t('providerDetail.testSingleTitle')}
                        disabled={testingModels()[m.id || m.name || '']}
                        onClick={() => testSingleModel(m.id || m.name || '')}
                      >
                        <IconZap size={12} />
                      </IconButton>
                      <Show
                        when={m.canEdit !== false}
                        fallback={
                          <span
                            class="text-faint px-1 text-[11px] cursor-not-allowed opacity-50"
                            title={t('providerDetail.lockedMetaTitle')}
                          >
                            <IconLock size={12} />
                          </span>
                        }
                      >
                        <IconButton size="sm" variant="ghost"
                          type="button"
                          class="text-faint hover:text-accent"
                          title={t('providerDetail.editModelMetaLongTitle')}
                          onClick={() => startEditModel(m)}
                        >
                          <IconEdit size={12} />
                        </IconButton>
                      </Show>
                      <div title={m.enabled !== false ? t('providerDetail.toggleOffHint') : t('providerDetail.toggleOnHint')}>
                        <Toggle
                          ariaLabel={`${t('common.enabled')}: ${m.name || m.id}`}
                          checked={m.enabled !== false}
                          onChange={() => toggleModel(m.id || m.name, m.enabled !== false)}
                        />
                      </div>
                    </div>
                  </div>

                  {/* 元数据 Badge 栏 */}
                  <div class="mt-2 pt-2 border-t border-subtle/40 flex items-center gap-1.5 flex-wrap text-[10px] text-faint">
                    <Show when={modelTestResults()[m.id || m.name || '']}>
                      {res => (
                        <Show
                          when={res().ok}
                          fallback={
                            <span
                              class="inline-flex items-center gap-1 px-1.5 py-0.5 rounded text-[10px] font-medium bg-danger/12 text-danger border border-danger/30 cursor-help"
                              title={res().error || t('providerDetail.testNotPassed')}
                            >
                              <IconClose size={10} class="shrink-0" />
                              <span>{res().error ? (res().error!.length > 12 ? res().error!.slice(0, 10) + '…' : res().error) : t('common.failed')}</span>
                            </span>
                          }
                        >
                          <span
                            class="inline-flex items-center gap-1 px-1.5 py-0.5 rounded text-[10px] font-medium bg-success/12 text-success border border-success/30 font-mono"
                            title={t('providerDetail.testPassedLatency', { latency: res().latency || t('providerDetail.normal') })}
                          >
                            <IconCheck size={10} class="shrink-0" />
                            <span>{res().latency || t('providerDetail.connected')}</span>
                          </span>
                        </Show>
                      )}
                    </Show>
                    <Show when={m.contextLength && m.contextLength > 0}>
                      <span class="bg-hover px-1.5 py-0.5 rounded font-mono">
                        {t('providerDetail.contextLength', { length: m.contextLength! >= 1024 ? `${Math.round(m.contextLength! / 1024)}k` : m.contextLength! })}
                      </span>
                    </Show>
                    <Show when={m.maxOutputTokens && m.maxOutputTokens > 0}>
                      <span class="bg-hover px-1.5 py-0.5 rounded font-mono">
                        {t('providerDetail.outputLength', { length: m.maxOutputTokens! >= 1024 ? `${Math.round(m.maxOutputTokens! / 1024)}k` : m.maxOutputTokens! })}
                      </span>
                    </Show>
                    <Show when={!m.contextLength && !m.maxOutputTokens}>
                      <span class="italic text-[10px] opacity-60">{t('providerDetail.noContext')}</span>
                    </Show>
                  </div>
                </div>
              )}
            </For>
          </div>
      </Show>
    </Card>
  </div>
  )
}

export default ModelsPanel
