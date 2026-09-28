import { type Component, Show, onMount } from 'solid-js'
import { useI18n } from '@/i18n'
import { formatBytes, formatUptime, uptimeUnitsFor } from '@/lib/format'
import { useGatewayStore } from '@/stores/gateway'
import { useSystemOps } from '@/stores/systemOps'
import { Alert, Badge, Button, Card, Empty, Select, IconActivity, IconDatabase, IconDownload, IconRotateCcw, IconServer } from '@/components/ui'

const SystemOpsPanel: Component = () => {
  const store = useGatewayStore()
  const { t } = useI18n()
  const {
    systemStats, loadingStats, updateInfo, checkingUpdate, applyingUpdate,
    pruneDays, setPruneDays, runningMaint,
    fetchSystemStats, handleCheckUpdate, handleApplyUpdate, handleTriggerRestart, handleRollback, handleMaintenance,
  } = useSystemOps()
  const uptimeUnits = () => uptimeUnitsFor(t)

  onMount(() => { void fetchSystemStats() })

  return (
        <div class="space-y-4">
          {/* 1. 系统运行指标卡片 */}
          <Card class="p-5 space-y-4">
            <div class="flex flex-wrap items-center justify-between gap-3 border-b border-subtle/50 pb-3">
              <div class="flex flex-wrap items-center gap-2 min-w-0">
                <IconActivity size={16} class="text-accent shrink-0" />
                <div>
                  <h3 class="text-sm font-semibold">{t('settings.ops.statsTitle')}</h3>
                  <p class="text-xs text-faint mt-0.5">{t('settings.ops.statsSubtitle')}</p>
                </div>
              </div>
              <Button
                variant="secondary"
                size="sm"
                loading={loadingStats()}
                onClick={fetchSystemStats}
                class="flex items-center gap-1 text-xs"
              >
                <IconRotateCcw size={12} />
                <span>{t('settings.ops.refreshStats')}</span>
              </Button>
            </div>

            <Show
              when={systemStats()}
              fallback={
                <Empty
                  title={t('settings.ops.noStatsLoaded')}
                  action={
                    <Button variant="secondary" size="sm" onClick={fetchSystemStats}>
                      {t('settings.ops.refreshStats')}
                    </Button>
                  }
                />
              }
            >
              {stats => (
                <div class="space-y-4">
                  {/* 状态徽标与基础环境 */}
                  <div class="grid grid-cols-2 sm:grid-cols-4 gap-3">
                    <div class="p-3.5 rounded-xl bg-surface-inset space-y-1 hover:bg-hover transition-colors">
                      <span class="text-xs text-faint font-medium">{t('settings.ops.osArch')}</span>
                      <div class="text-xs font-mono font-semibold text-foreground truncate">
                        {stats().os} / {stats().arch}
                      </div>
                    </div>
                    <div class="p-3.5 rounded-xl bg-surface-inset space-y-1 hover:bg-hover transition-colors">
                      <span class="text-xs text-faint font-medium">{t('settings.ops.uptime')}</span>
                      <div class="text-xs font-mono font-semibold text-foreground">
                        {formatUptime(stats().uptimeSeconds, uptimeUnits())}
                      </div>
                    </div>
                    <div class="p-3.5 rounded-xl bg-surface-inset space-y-1 hover:bg-hover transition-colors">
                      <span class="text-xs text-faint font-medium">{t('settings.ops.pid')} / CPU</span>
                      <div class="text-xs font-mono font-semibold text-foreground">
                        PID {stats().pid} · {stats().numCPU} Cores
                      </div>
                    </div>
                    <div class="p-3.5 rounded-xl bg-surface-inset space-y-1 hover:bg-hover transition-colors">
                      <span class="text-xs text-faint font-medium">{t('settings.ops.goroutines')}</span>
                      <div class="text-xs font-mono font-semibold text-foreground">
                        {stats().goroutines}
                      </div>
                    </div>
                  </div>

                  {/* 内存与存储仪表 */}
                  <div class="grid grid-cols-1 sm:grid-cols-2 gap-3 pt-3 border-t border-subtle/50">
                    {/* 内存 */}
                    <div class="p-4 rounded-xl bg-surface-inset space-y-3">
                      <div class="flex items-center justify-between text-xs font-semibold">
                        <span class="flex items-center gap-1.5 text-accent">
                          <IconServer size={15} />
                          <span>{t('settings.ops.goRuntimeMemory')}</span>
                        </span>
                        <Badge tone="blue">GC: {stats().memory.numGC}</Badge>
                      </div>
                      <div class="grid grid-cols-2 gap-3 text-xs pt-0.5">
                        <div class="space-y-0.5">
                          <span class="text-xs text-faint block">{t('settings.ops.memoryAlloc')}</span>
                          <span class="font-mono font-semibold text-foreground">{formatBytes(stats().memory.allocBytes)}</span>
                        </div>
                        <div class="space-y-0.5">
                          <span class="text-xs text-faint block">{t('settings.ops.memoryInuse')}</span>
                          <span class="font-mono font-semibold text-foreground">{formatBytes(stats().memory.heapInuseBytes)}</span>
                        </div>
                        <div class="space-y-0.5">
                          <span class="text-xs text-faint block">{t('settings.ops.memorySys')}</span>
                          <span class="font-mono font-semibold text-foreground">{formatBytes(stats().memory.sysBytes)}</span>
                        </div>
                        <div class="space-y-0.5">
                          <span class="text-xs text-faint block">{t('settings.ops.memoryTotalAlloc')}</span>
                          <span class="font-mono font-semibold text-foreground">{formatBytes(stats().memory.totalAllocBytes)}</span>
                        </div>
                      </div>
                    </div>

                    {/* SQLite 存储 */}
                    <div class="p-4 rounded-xl bg-surface-inset space-y-3">
                      <div class="flex items-center justify-between text-xs font-semibold">
                        <span class="flex items-center gap-1.5 text-warning">
                          <IconDatabase size={15} />
                          <span>{t('settings.ops.sqlitePhysicalStorage')}</span>
                        </span>
                        <Badge tone={stats().inDocker ? 'amber' : 'green'}>
                          {stats().inDocker ? t('settings.ops.inDockerBadge') : t('settings.ops.nativeBadge')}
                        </Badge>
                      </div>
                      <div class="grid grid-cols-2 gap-3 text-xs pt-0.5">
                        <div class="space-y-0.5">
                          <span class="text-xs text-faint block">{t('settings.ops.dbSize')}</span>
                          <span class="font-mono font-semibold text-foreground">{formatBytes(stats().storage?.dbSizeBytes ?? 0)}</span>
                        </div>
                        <div class="space-y-0.5">
                          <span class="text-xs text-faint block">{t('settings.ops.walSize')}</span>
                          <span class="font-mono font-semibold text-foreground">{formatBytes(stats().storage?.walSizeBytes ?? 0)}</span>
                        </div>
                        <div class="space-y-0.5">
                          <span class="text-xs text-faint block">{t('settings.ops.totalRequests')}</span>
                          <span class="font-mono font-semibold text-foreground">{stats().storage?.totalRequests ?? 0}</span>
                        </div>
                        <div class="space-y-0.5">
                          <span class="text-xs text-faint block">{t('settings.ops.totalDetails')}</span>
                          <span class="font-mono font-semibold text-foreground">{stats().storage?.totalDetails ?? 0}</span>
                        </div>
                      </div>
                    </div>
                  </div>
                </div>
              )}
            </Show>
          </Card>

          {/* 2. 在线自更新卡片 */}
          <Card class="p-5 space-y-4">
            <div class="flex flex-wrap items-center justify-between gap-3 border-b border-subtle/50 pb-3">
              <div class="flex flex-wrap items-center gap-2 min-w-0">
                <IconDownload size={16} class="text-accent shrink-0" />
                <div>
                  <h3 class="text-sm font-semibold">{t('settings.ops.updateTitle')}</h3>
                  <p class="text-xs text-faint mt-0.5">{t('settings.ops.updateSubtitle')}</p>
                </div>
              </div>
              <div class="flex flex-wrap items-center gap-2 min-w-0">
                <Button
                  variant="secondary"
                  size="sm"
                  loading={checkingUpdate()}
                  onClick={handleCheckUpdate}
                  class="flex items-center gap-1 text-xs"
                >
                  <IconRotateCcw size={12} />
                  <span>{t('settings.ops.checkUpdate')}</span>
                </Button>
              </div>
            </div>

            <Show
              when={!systemStats()?.inDocker}
              fallback={
                <Alert variant="warning">{t('settings.ops.dockerNotice')}</Alert>
              }
            >
              <div class="space-y-3.5">
                <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-3 p-4 rounded-xl bg-surface-inset">
                  <div class="space-y-1">
                    <div class="flex flex-wrap items-center gap-2 min-w-0">
                      <span class="text-xs text-faint">{t('settings.ops.currentVersion')}:</span>
                      <span class="font-mono text-xs font-semibold">v{store.version()}</span>
                      <Show when={updateInfo()}>
                        <Show
                          when={updateInfo()?.hasUpdate}
                          fallback={<Badge tone="green">{t('settings.ops.upToDateBadge')}</Badge>}
                        >
                          <Badge tone="amber">{t('settings.ops.hasUpdateBadge')} v{updateInfo()?.latestVersion}</Badge>
                        </Show>
                      </Show>
                    </div>
                    <Show when={updateInfo()?.hasUpdate}>
                      <p class="text-xs text-faint">
                        {updateInfo()?.assetName} ({formatBytes(updateInfo()?.assetSize ?? 0)}) · {updateInfo()?.publishedAt?.slice(0, 10)}
                      </p>
                    </Show>
                  </div>

                  <div class="flex items-center gap-2 shrink-0">
                    <Button
                      variant="secondary"
                      size="sm"
                      onClick={handleRollback}
                      class="text-xs text-muted"
                    >
                      {t('settings.ops.rollbackBtn')}
                    </Button>
                    <Show when={updateInfo()?.hasUpdate}>
                      <Button
                        variant="primary"
                        size="sm"
                        loading={applyingUpdate()}
                        onClick={handleApplyUpdate}
                        class="text-xs"
                      >
                        {t('settings.ops.updateNow')}
                      </Button>
                    </Show>
                  </div>
                </div>

                <Show when={updateInfo()?.hasUpdate && updateInfo()?.releaseNotes}>
                  <div class="p-3.5 rounded-xl bg-code-bg border border-subtle text-xs font-mono text-muted max-h-40 overflow-y-auto whitespace-pre-wrap wrap-anywhere leading-relaxed">
                    {updateInfo()?.releaseNotes}
                  </div>
                </Show>
              </div>
            </Show>
          </Card>

          {/* 3. 服务控制与平滑重启 */}
          <Card class="p-5 space-y-4">
            <div class="flex flex-wrap items-center justify-between gap-3 border-b border-subtle/50 pb-3">
              <div class="flex flex-wrap items-center gap-2 min-w-0">
                <IconRotateCcw size={16} class="text-warning shrink-0" />
                <div>
                  <h3 class="text-sm font-semibold">{t('settings.ops.restartTitle')}</h3>
                  <p class="text-xs text-faint mt-0.5">{t('settings.ops.restartSubtitle')}</p>
                </div>
              </div>
            </div>

            <div class="p-4 rounded-xl bg-surface-inset flex flex-col sm:flex-row sm:items-center justify-between gap-4">
              <div class="text-xs text-faint leading-relaxed max-w-xl">
                {t('settings.ops.restartDesc')}
              </div>
              <Button
                variant="danger"
                disabled={systemStats()?.inDocker}
                onClick={handleTriggerRestart}
                class="flex items-center gap-1.5 shrink-0"
              >
                <IconRotateCcw size={14} />
                <span>{t('settings.ops.restartBtn')}</span>
              </Button>
            </div>
          </Card>

          {/* 4. 存储与数据库维护 */}
          <Card class="p-5 space-y-4">
            <div class="flex flex-wrap items-center justify-between gap-3 border-b border-subtle/50 pb-3">
              <div class="flex flex-wrap items-center gap-2 min-w-0">
                <IconDatabase size={16} class="text-accent shrink-0" />
                <div>
                  <h3 class="text-sm font-semibold">{t('settings.ops.maintTitle')}</h3>
                  <p class="text-xs text-faint mt-0.5">{t('settings.ops.maintSubtitle')}</p>
                </div>
              </div>
            </div>

            <div class="space-y-3.5">
              {/* Checkpoint */}
              <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-3 p-4 rounded-xl bg-surface-inset hover:bg-hover transition-colors">
                <div class="space-y-0.5">
                  <div class="text-xs font-semibold text-foreground">{t('settings.ops.checkpointBtn')}</div>
                  <div class="text-xs text-faint">{t('settings.ops.checkpointDesc')}</div>
                </div>
                <Button
                  variant="secondary"
                  size="sm"
                  loading={runningMaint() === 'checkpoint'}
                  onClick={() => handleMaintenance('checkpoint')}
                  class="shrink-0 text-xs"
                >
                  {t('settings.ops.checkpointBtn')}
                </Button>
              </div>

              {/* Vacuum */}
              <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-3 p-4 rounded-xl bg-surface-inset hover:bg-hover transition-colors">
                <div class="space-y-0.5">
                  <div class="text-xs font-semibold text-foreground">{t('settings.ops.vacuumBtn')}</div>
                  <div class="text-xs text-faint">{t('settings.ops.vacuumDesc')}</div>
                </div>
                <Button
                  variant="secondary"
                  size="sm"
                  loading={runningMaint() === 'vacuum'}
                  onClick={() => handleMaintenance('vacuum')}
                  class="shrink-0 text-xs"
                >
                  {t('settings.ops.vacuumBtn')}
                </Button>
              </div>

              {/* Prune Logs */}
              <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-3 p-4 rounded-xl bg-surface-inset hover:bg-hover transition-colors">
                <div class="min-w-0 space-y-1">
                  <div class="text-xs font-semibold text-foreground">{t('settings.ops.pruneBtn')}</div>
                  <div class="text-xs text-faint">{t('settings.ops.pruneDesc')}</div>
                  <div class="flex flex-wrap items-center gap-2 pt-1">
                    <span class="text-xs text-muted">{t('settings.ops.retentionDaysLabel')}:</span>
                    <Select
                      options={[7, 14, 30, 60, 90].map(n => ({ value: String(n), label: t('settings.ops.days', { n }) }))}
                      value={String(pruneDays())}
                      onChange={v => setPruneDays(Number(v))}
                      class="w-24 shrink-0 text-xs"
                    />
                  </div>
                </div>
                <Button
                  variant="danger"
                  size="sm"
                  loading={runningMaint() === 'prune_logs'}
                  onClick={() => handleMaintenance('prune_logs')}
                  class="shrink-0 text-xs"
                >
                  {t('settings.ops.pruneBtn')}
                </Button>
              </div>
            </div>
          </Card>
        </div>
  )
}

export default SystemOpsPanel
