import { type Component, Show } from 'solid-js'
import { useI18n } from '@/i18n'
import type { ProviderModelsApi } from '@/lib/providerModels'
import { Button, Field, Input, Modal } from '@/components/ui'

const ModelMetaModal: Component<{ models: ProviderModelsApi }> = props => {
  const { t } = useI18n()
  const {
    editingModel,
    setEditingModel,
    editDisplayName,
    setEditDisplayName,
    editContextLength,
    setEditContextLength,
    editMaxOutput,
    setEditMaxOutput,
    savingMeta,
    handleSaveModelMeta,
    handleResetModelMeta
  } = props.models

  // 模型元数据编辑弹窗
  return (
<Modal
  open={Boolean(editingModel())}
  title={t('providerDetail.editModelMeta', { id: editingModel()?.id || '' })}
  onClose={() => setEditingModel(null)}
>
  <div class="space-y-4">
    <Field label={t('providerDetail.modelIdLabel')} hint={t('providerDetail.modelIdHint')}>
      <Input value={editingModel()?.id || ''} disabled class="bg-hover font-mono opacity-80" />
    </Field>
    <Field label={t('providerDetail.displayNameLabel')} hint={t('providerDetail.displayNameHint')}>
      <Input value={editDisplayName()} onInput={setEditDisplayName} placeholder={t('providerDetail.displayNamePlaceholder')} />
    </Field>
    <div class="grid grid-cols-2 gap-3">
      <Field label={t('providerDetail.contextLengthLabel')} hint={t('providerDetail.contextLengthHint')}>
        <Input
          type="number"
          value={editContextLength()}
          onInput={setEditContextLength}
          placeholder="128000"
        />
      </Field>
      <Field label={t('providerDetail.maxOutputLabel')} hint={t('providerDetail.maxOutputHint')}>
        <Input
          type="number"
          value={editMaxOutput()}
          onInput={setEditMaxOutput}
          placeholder="8192"
        />
      </Field>
    </div>
    <div class="flex flex-wrap items-center justify-between gap-3 pt-3 border-t border-subtle">
      <div>
        <Show when={editingModel()?.hasOverride}>
          <Button
            size="sm"
            variant="danger"
            loading={savingMeta()}
            onClick={handleResetModelMeta}
          >
            {t('providerDetail.restoreOfficialDefault')}
          </Button>
        </Show>
      </div>
      <div class="flex flex-wrap items-center gap-2 min-w-0">
        <Button size="sm" variant="secondary" onClick={() => setEditingModel(null)}>
          {t('common.cancel')}
        </Button>
        <Button size="sm" variant="primary" loading={savingMeta()} onClick={handleSaveModelMeta}>
          {t('providerDetail.saveMetadata')}
        </Button>
      </div>
    </div>
  </div>
</Modal>
  )
}

export default ModelMetaModal
