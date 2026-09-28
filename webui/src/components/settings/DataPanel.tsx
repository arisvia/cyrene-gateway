import { type Component, Show, createSignal } from 'solid-js'
import { useI18n } from '@/i18n'
import { useGatewayStore } from '@/stores/gateway'
import { useToast } from '@/lib/toast'
import { Alert, Badge, Button, Card, Checkbox, Field, FileUpload, Select, confirm, IconAlertTriangle, IconDownload, IconInfo, IconUpload } from '@/components/ui'

const DataPanel: Component = () => {
  const store = useGatewayStore()
  const { t } = useI18n()
  const toast = useToast()

// 数据备份与导出状态
const [includeSecrets, setIncludeSecrets] = createSignal(true)
const [includeUsage, setIncludeUsage] = createSignal(false)
const [downloading, setDownloading] = createSignal(false)

// 数据恢复状态
const [restoreMode, setRestoreMode] = createSignal<'replace' | 'merge'>('replace')
const [restoreFile, setRestoreFile] = createSignal<File | null>(null)
const [restoring, setRestoring] = createSignal(false)

async function handleDownloadBackup() {
  setDownloading(true)
  try {
    const url = `/api/system/backup?include_secrets=${includeSecrets()}&include_usage=${includeUsage()}`
    const res = await fetch(url)
    if (!res.ok) throw new Error(`HTTP ${res.status}`)
    const blob = await res.blob()
    const a = document.createElement('a')
    a.href = URL.createObjectURL(blob)
    const dateStr = new Date().toISOString().slice(0, 10)
    a.download = `cyrene-backup-${dateStr}${includeSecrets() ? '' : '-sanitized'}.cyrene.json`
    document.body.appendChild(a)
    a.click()
    document.body.removeChild(a)
    URL.revokeObjectURL(a.href)
    toast.success(t('toast.backupExportSuccess'))
  } catch (e: unknown) {
    const msg = e instanceof Error ? e.message : 'error'
    toast.error(t('toast.backupExportFailed', { error: msg }))
  } finally {
    setDownloading(false)
  }
}

async function handleRestoreSubmit() {
  const file = restoreFile()
  if (!file) {
    toast.warning(t('toast.selectBackupFileFirst'))
    return
  }
  const modeText = restoreMode() === 'replace'
    ? t('settings.data.restoreModeReplace')
    : t('settings.data.restoreModeMerge')

  const ok = await confirm({
    title: t('settings.data.restoreConfirmTitle'),
    message: t('settings.data.restoreConfirmMessage', { file: file.name, mode: modeText }),
    variant: 'danger',
  })
  if (!ok) return

  setRestoring(true)
  try {
    const res = await fetch(`/api/system/restore?mode=${restoreMode()}`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: file,
    })
    if (!res.ok) {
      const errJson = await res.json().catch(() => ({}))
      throw new Error(errJson.error || `HTTP ${res.status}`)
    }
    toast.success(t('toast.restoreSuccess'))
    setRestoreFile(null)
    await store.loadCore()
  } catch (e: unknown) {
    const msg = e instanceof Error ? e.message : t('settings.restoreFailed')
    toast.error(t('toast.restoreFailed', { error: msg }))
  } finally {
    setRestoring(false)
  }
}
  return (
        <div class="space-y-3.5">

    {/* 数据备份导出卡片 */}
    <Card class="p-5 space-y-4">
      <div class="flex flex-wrap items-center justify-between gap-3 border-b border-subtle/50 pb-3">
        <div class="flex flex-wrap items-center gap-2 min-w-0">
          <IconDownload size={16} class="text-accent shrink-0" />
          <div>
            <h3 class="text-sm font-semibold">{t('settings.data.backupCardTitle')}</h3>
            <p class="text-xs text-faint mt-0.5">{t('settings.data.backupCardSubtitle')}</p>
          </div>
        </div>
        <Badge tone="blue">{t('settings.data.snapshotBadge')}</Badge>
      </div>

    <div class="p-4 rounded-xl bg-surface-inset space-y-3.5">
      <Checkbox
        checked={includeSecrets()}
        onChange={setIncludeSecrets}
        label={t('settings.data.includeSecrets')}
        description={
          <div class="flex items-center gap-1.5 mt-1 text-xs">
            <Show
              when={includeSecrets()}
              fallback={
                <span class="text-faint flex items-center gap-1">
                  <IconInfo size={13} class="text-info shrink-0" />
                  <span>{t('settings.data.secretsSafe')}</span>
                </span>
              }
            >
              <span class="text-warning flex items-center gap-1">
                <IconAlertTriangle size={13} class="text-warning shrink-0" />
                <span>{t('settings.data.secretsWarning')}</span>
              </span>
            </Show>
          </div>
        }
      />

      <div class="pt-3 border-t border-subtle/40">
        <Checkbox
          checked={includeUsage()}
          onChange={setIncludeUsage}
          label={t('settings.data.includeUsage')}
          description={t('settings.data.usageDesc')}
        />
      </div>
    </div>

    <div class="pt-1 flex justify-end">
        <Button
          variant="primary"
          loading={downloading()}
          onClick={handleDownloadBackup}
          class="flex items-center gap-1.5"
        >
          <IconDownload size={14} />
          <span>{t('settings.data.downloadBackup')}</span>
        </Button>
      </div>
    </Card>

    {/* 备份恢复与导入卡片 */}
    <Card class="p-5 space-y-4">
      <div class="flex flex-wrap items-center justify-between gap-3 border-b border-subtle/50 pb-3">
        <div class="flex flex-wrap items-center gap-2 min-w-0">
          <IconUpload size={16} class="text-warning shrink-0" />
          <div>
            <h3 class="text-sm font-semibold">{t('settings.data.restoreCardTitle')}</h3>
            <p class="text-xs text-faint mt-0.5">{t('settings.data.restoreCardSubtitle')}</p>
          </div>
        </div>
        <Badge tone="amber">{t('settings.data.restoreBadge')}</Badge>
      </div>

    <div class="space-y-3.5 text-xs">
      <div class="p-4 rounded-xl bg-surface-inset space-y-4">
        <Field label={t('settings.backupFileLabel')} hint={t('settings.backupFileHint')}>
          <FileUpload
            accept=".json,.cyrene.json"
            value={restoreFile()}
            onChange={setRestoreFile}
            placeholder={t('common.dragOrClickFile')}
            hint={`${t('common.supportedFormats')}: .cyrene.json, .json`}
          />
        </Field>

        <Field label={t('settings.restoreStrategyLabel')} hint={t('settings.restoreStrategyHint')}>
          <Select
            value={restoreMode()}
            options={[
              { value: 'replace', label: t('settings.restoreModeReplaceLabel') },
              { value: 'merge', label: t('settings.restoreModeMergeLabel') },
            ]}
            onChange={v => setRestoreMode(v as 'replace' | 'merge')}
          />
        </Field>
      </div>

      <Alert variant="warning" title={t('settings.data.importantNotice')}>
        {t('settings.restoreNotice')}
      </Alert>
    </div>

    <div class="pt-1 flex items-center justify-end">
        <Button
          variant="danger"
          disabled={!restoreFile()}
          loading={restoring()}
          onClick={handleRestoreSubmit}
          class="flex items-center gap-1.5"
        >
          <IconUpload size={14} />
          <span>{t('settings.data.startRestore')}</span>
        </Button>
      </div>
    </Card>
        </div>
  )
}

export default DataPanel
