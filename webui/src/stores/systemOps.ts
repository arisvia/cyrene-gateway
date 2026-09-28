import { createRoot, createSignal } from 'solid-js'
import { api, apiPost } from '@/lib/api'
import { t } from '@/i18n'
import { toast } from '@/lib/toast'
import { confirm } from '@/lib/confirm'
import { useGatewayStore } from './gateway'

export interface SystemStatsData {
  version: string
  uptimeSeconds: number
  startTime: string
  pid: number
  os: string
  arch: string
  numCPU: number
  goVersion: string
  goroutines: number
  inDocker: boolean
  memory: {
    allocBytes: number
    totalAllocBytes: number
    sysBytes: number
    heapAllocBytes: number
    heapInuseBytes: number
    numGC: number
  }
  storage?: {
    dbSizeBytes: number
    walSizeBytes: number
    pageCount: number
    pageSize: number
    totalRequests: number
    totalDetails: number
  }
}

export interface UpdateCheckData {
  currentVersion: string
  latestVersion: string
  hasUpdate: boolean
  releaseNotes: string
  publishedAt: string
  assetName?: string
  assetSize?: number
}

type MaintAction = 'checkpoint' | 'vacuum' | 'prune_logs'

// 运维状态必须活过设置面板的卸载：重启等待期间切换标签页，遮罩仍要留在页面上。
function createSystemOpsStore() {
  const store = useGatewayStore()
  const [systemStats, setSystemStats] = createSignal<SystemStatsData | null>(null)
  const [loadingStats, setLoadingStats] = createSignal(false)
  const [updateInfo, setUpdateCheck] = createSignal<UpdateCheckData | null>(null)
  const [checkingUpdate, setCheckingUpdate] = createSignal(false)
  const [applyingUpdate, setApplyingUpdate] = createSignal(false)
  const [restarting, setRestarting] = createSignal(false)
  const [pruneDays, setPruneDays] = createSignal(30)
  const [runningMaint, setRunningMaint] = createSignal<string | null>(null)

  async function fetchSystemStats() {
    setLoadingStats(true)
    try {
      const data = await api<SystemStatsData>('/api/system/stats')
      setSystemStats(data)
    } catch {
      // Error surfaced by api.ts
    } finally {
      setLoadingStats(false)
    }
  }

  async function handleCheckUpdate() {
    setCheckingUpdate(true)
    try {
      const data = await api<UpdateCheckData>('/api/system/update/check')
      setUpdateCheck(data)
      if (data.hasUpdate) {
        toast.info(`${t('settings.ops.hasUpdateBadge')}: v${data.latestVersion}`)
      } else {
        toast.success(t('settings.ops.upToDateBadge'))
      }
    } catch {
      // Error surfaced by api.ts
    } finally {
      setCheckingUpdate(false)
    }
  }

  async function handleApplyUpdate() {
    if (!updateInfo()?.hasUpdate) return
    const confirmed = await confirm({
      title: t('settings.ops.updateTitle'),
      message: `${t('settings.ops.updateNow')} (v${updateInfo()?.latestVersion})?`,
      variant: 'primary',
    })
    if (!confirmed) return

    setApplyingUpdate(true)
    try {
      await apiPost('/api/system/update', {})
      toast.success(t('settings.ops.updateSuccess'))
      const shouldRestart = await confirm({
        title: t('settings.ops.restartTitle'),
        message: t('settings.ops.restartConfirm'),
        variant: 'primary',
      })
      if (shouldRestart) {
        await handleTriggerRestart()
      }
    } catch {
      // Error surfaced by api.ts
    } finally {
      setApplyingUpdate(false)
    }
  }

  async function handleTriggerRestart() {
    const confirmed = await confirm({
      title: t('settings.ops.restartTitle'),
      message: t('settings.ops.restartConfirm'),
      variant: 'danger',
    })
    if (!confirmed) return

    setRestarting(true)
    try {
      await apiPost('/api/system/restart', {})
    } catch {
      // Ignore network abort if server closes instantly
    }

    let attempts = 0
    const maxAttempts = 30
    const interval = setInterval(async () => {
      attempts++
      try {
        const h = await api<{ status: string }>('/api/health', { silent: true })
        if (h && h.status === 'ok') {
          clearInterval(interval)
          setRestarting(false)
          toast.success(t('settings.ops.reconnectSuccess'))
          void fetchSystemStats()
          void store.loadSettings()
        }
      } catch {
        if (attempts >= maxAttempts) {
          clearInterval(interval)
          setRestarting(false)
          toast.error(t('settings.ops.reconnectTimeout'))
        }
      }
    }, 1000)
  }

  async function handleRollback() {
    const confirmed = await confirm({
      title: t('settings.ops.rollbackBtn'),
      message: t('settings.ops.rollbackConfirm'),
      variant: 'danger',
    })
    if (!confirmed) return

    try {
      await apiPost('/api/system/rollback', {})
      toast.success(t('settings.ops.rollbackSuccess'))
    } catch {
      // Error surfaced by api.ts
    }
  }

  async function handleMaintenance(action: MaintAction) {
    setRunningMaint(action)
    try {
      const res = await apiPost<{ success: boolean; prunedHistory?: number; prunedDetails?: number }>(
        '/api/system/maintenance',
        { action, retentionDays: pruneDays() }
      )
      if (action === 'checkpoint') {
        toast.success(t('settings.ops.checkpointSuccess'))
      } else if (action === 'vacuum') {
        toast.success(t('settings.ops.vacuumSuccess'))
      } else if (action === 'prune_logs') {
        toast.success(
          t('settings.ops.pruneSuccess', {
            history: String(res.prunedHistory ?? 0),
            details: String(res.prunedDetails ?? 0),
          })
        )
      }
      void fetchSystemStats()
    } catch {
      // Error surfaced by api.ts
    } finally {
      setRunningMaint(null)
    }
  }

  return {
    systemStats, loadingStats, updateInfo, checkingUpdate, applyingUpdate,
    restarting, pruneDays, setPruneDays, runningMaint,
    fetchSystemStats, handleCheckUpdate, handleApplyUpdate,
    handleTriggerRestart, handleRollback, handleMaintenance,
  }
}

let _store: ReturnType<typeof createSystemOpsStore> | null = null

export function useSystemOps() {
  if (!_store) _store = createRoot(createSystemOpsStore)
  return _store
}
