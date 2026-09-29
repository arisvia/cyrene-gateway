import { type Component, Show } from 'solid-js'
import { useI18n } from '@/i18n'
import type { ProviderModelsApi } from '@/lib/providerModels'
import { Button, IconZap, Input } from '@/components/ui'

const ModelsToolbar: Component<{ models: ProviderModelsApi }> = props => {
  const { t } = useI18n()
  const {
    allDisplayModels,
    enabledModelsCount,
    disabledModelsCount,
    modelSearch,
    setModelSearch,
    syncingModels,
    handleSyncModels,
    batchBusy,
    testingAll,
    testAllProgress,
    handleTestAll,
    failedModelsList,
    handleEnableAll,
    handleDisableAll,
    handleDisableFailed
  } = props.models

  return (
  <div class="animate-slide-up flex flex-wrap items-center gap-2 text-xs">
    <div class="flex flex-wrap items-center gap-2 min-w-0">
      <Input
        class="w-40 sm:w-52!"
        size="sm"
        value={modelSearch()}
        onInput={setModelSearch}
        placeholder={t('providerDetail.modelSearchPlaceholder')}
      />
      <Button
        size="sm"
        variant="secondary"
        loading={syncingModels()}
        onClick={handleSyncModels}
        title={t('providerDetail.syncModelsTitle')}
      >
        {t('providerDetail.syncUpstreamModels')}
      </Button>
    </div>
    <div class="hidden xl:block h-3.5 w-px bg-subtle/60 mx-0.5" />
    <div class="flex items-center gap-1.5 flex-wrap">
      {/* 实时状态指示徽章 */}
      <span
        class={`inline-flex items-center gap-1.5 px-2 py-1 rounded-md text-[11px] font-mono border transition-colors ${
          enabledModelsCount() === allDisplayModels().length
            ? 'bg-success/10 text-success border-success/30'
            : (enabledModelsCount() === 0
                ? 'bg-surface-inset text-faint border-subtle'
                : 'bg-accent/10 text-accent border-accent/30')
        }`}
        title={t('providerDetail.modelsCountSummary', { total: allDisplayModels().length, enabled: enabledModelsCount() })}
      >
        <span class={`w-1.5 h-1.5 rounded-full ${
          enabledModelsCount() === allDisplayModels().length
            ? 'bg-success'
            : (enabledModelsCount() === 0 ? 'bg-faint' : 'bg-accent animate-pulse')
        }`} />
        <span>{enabledModelsCount()}/{allDisplayModels().length}</span>
      </span>

      {/* 状态联动批量启停控制器（带实时数字反馈与状态色） */}
      <div class="inline-flex flex-wrap rounded-lg p-0.5 bg-surface-inset border border-subtle text-xs select-none">
        <Button size="sm" variant="ghost"
          type="button"
          class={`px-2.5 py-1 rounded-md transition-all flex items-center gap-1 font-medium ${
            disabledModelsCount() === 0 || batchBusy()
              ? 'opacity-40 cursor-not-allowed text-faint'
              : 'text-foreground hover:text-success hover:bg-success/10 cursor-pointer'
          }`}
          disabled={disabledModelsCount() === 0 || batchBusy()}
          onClick={handleEnableAll}
          title={disabledModelsCount() === 0 ? t('toast.allModelsEnabled') : t('providerDetail.tooltipEnableAll')}
        >
          <span class={`w-1.5 h-1.5 rounded-full ${disabledModelsCount() === 0 ? 'bg-success' : 'bg-accent'}`} />
          <span>{t('providerDetail.enableAllModels')}</span>
          <Show when={disabledModelsCount() > 0}>
            <span class="font-mono text-[10px] opacity-75">({disabledModelsCount()})</span>
          </Show>
        </Button>
        <Button size="sm" variant="ghost"
          type="button"
          class={`px-2.5 py-1 rounded-md transition-all flex items-center gap-1 font-medium ${
            enabledModelsCount() === 0 || batchBusy()
              ? 'opacity-40 cursor-not-allowed text-faint'
              : 'text-faint hover:text-danger hover:bg-danger/10 cursor-pointer'
          }`}
          disabled={enabledModelsCount() === 0 || batchBusy()}
          onClick={handleDisableAll}
          title={enabledModelsCount() === 0 ? t('toast.allModelsDisabled') : t('providerDetail.tooltipDisableAll')}
        >
          <span class={`w-1.5 h-1.5 rounded-full ${enabledModelsCount() === 0 ? 'bg-faint' : 'bg-danger/70'}`} />
          <span>{t('providerDetail.disableAllModels')}</span>
          <Show when={enabledModelsCount() > 0 && disabledModelsCount() > 0}>
            <span class="font-mono text-[10px] opacity-75">({enabledModelsCount()})</span>
          </Show>
        </Button>
      </div>
      <Button
        size="sm"
        variant="secondary"
        loading={testingAll()}
        onClick={handleTestAll}
        title={t('providerDetail.tooltipTestAll')}
      >
        <IconZap size={12} class="mr-1 inline" />
        {t('providerDetail.testAllModels')}
      </Button>
      <Show when={testAllProgress()}>
        {prog => (
          <span class="text-[11px] text-faint font-mono">
            {t('providerDetail.progress', { current: prog().current, total: prog().total })}
          </span>
        )}
      </Show>
      <Show when={failedModelsList().length > 0}>
        <Button
          size="sm"
          variant="danger"
          disabled={batchBusy()}
          onClick={handleDisableFailed}
          title={t('providerDetail.tooltipDisableFailed')}
        >
          {t('providerDetail.disableFailedModels')} ({failedModelsList().length})
        </Button>
      </Show>
    </div>
  </div>
  )
}

export default ModelsToolbar
