import { createEffect, createMemo, createResource, createSignal, on } from 'solid-js'
import { confirm } from './confirm'
import { t } from '@/i18n'
import { useToast } from './toast'
import { useGatewayStore } from '@/stores/gateway'
import type { ProviderModel } from '@/types/domain'

export interface ProviderModelsPayload {
  registryModels?: ProviderModel[]
  customModels?: ProviderModel[]
  isFreeMode?: boolean
  authType?: string
  authModes?: string[]
  defaultHeaders?: Record<string, string>
  hasApiKey?: boolean
}

const EMPTY_MODELS: ProviderModelsPayload = {}

export interface ModelTestResult {
  ok: boolean
  latency?: string
  error?: string
}

export interface ProviderModelsDeps {
  /** 当前账号连接 id，模型接口按连接取数。 */
  connectionId: () => string | undefined
  /** 当前账号所属供应商 id，用于拼 model key。 */
  providerId: () => string | undefined
}

/** 模型在本连接内的唯一键：自定义模型可能只有 name。视图侧索引探测结果时也必须用它。 */
export const modelKeyOf = (m: ProviderModel) => m.id || m.name || ''

/** 未显式停用的都算启用，两条清单共用同一判定。 */
const filterEnabled = (list: ProviderModel[]) => list.filter(m => m.enabled !== false)

/** 启用者优先、再按模型 ID 自然序，注册表与自定义清单共用同一套排序。 */
function sortModels(list: ProviderModel[]): ProviderModel[] {
  return list.slice().sort((a, b) => {
    const aEnabled = a.enabled !== false
    const bEnabled = b.enabled !== false
    if (aEnabled !== bEnabled) return aEnabled ? -1 : 1
    return modelKeyOf(a).localeCompare(modelKeyOf(b), undefined, { numeric: true, sensitivity: 'base' })
  })
}

/**
 * 一个供应商连接的模型清单：读取、启停、连通性探测与元数据覆写。
 *
 * 由 ProviderDetail 在自己的作用域内创建。注意 /providers/:id 是非 keyed 路由，
 * 切换账号不会重建组件，所以连接 id 变化时必须自己复位视图状态（见下方 createEffect）。
 */
export function useProviderModels(deps: ProviderModelsDeps) {
  const store = useGatewayStore()
  const toast = useToast()

  const [models, { refetch: refetchModels, mutate: mutateModels }] = createResource(
    deps.connectionId,
    async (id): Promise<ProviderModelsPayload> => {
      try {
        return await store.loadProviderModels(id) as ProviderModelsPayload
      } catch {
        return { registryModels: [], customModels: [] }
      }
    },
  )

  // 资源尚未就绪时以空对象占位，读侧只需一层 ?? []
  const payload = (): ProviderModelsPayload => models() ?? EMPTY_MODELS

  const allDisplayModels = () =>
    (payload().registryModels ?? []).concat(payload().customModels ?? [])
  const activeModels = () => allDisplayModels().filter(m => Boolean(m.id) && m.enabled !== false)

  const enabledModelsCount = createMemo(() => filterEnabled(allDisplayModels()).length)
  const disabledModelsCount = () => allDisplayModels().length - enabledModelsCount()

  const sortedRegistryModels = createMemo(() => sortModels(payload().registryModels ?? []))
  const sortedCustomModels = createMemo(() => sortModels(payload().customModels ?? []))
  const registryCount = createMemo(() => sortedRegistryModels().length)
  const registryEnabledCount = createMemo(() => filterEnabled(sortedRegistryModels()).length)
  const customCount = createMemo(() => sortedCustomModels().length)

  const [modelSearch, setModelSearch] = createSignal('')
  const [newCustomModelId, setNewCustomModelId] = createSignal('')
  const [showCustomModels, setShowCustomModels] = createSignal(false)
  const [syncingModels, setSyncingModels] = createSignal(false)
  const [batchBusy, setBatchBusy] = createSignal(false)
  const [testingModels, setTestingModels] = createSignal<Record<string, boolean>>({})
  const [modelTestResults, setModelTestResults] = createSignal<Record<string, ModelTestResult>>({})
  const [testingAll, setTestingAll] = createSignal(false)
  const [testAllProgress, setTestAllProgress] = createSignal<{ current: number; total: number } | null>(null)

  // 探测是异步的：请求已经在路上时切账号，回来那份结果属于上一个连接。
  // 每次复位自增，写状态前先比对，过期的直接丢弃，既不弹旧汇总也不给新连接挂徽标。
  let generation = 0

  // 有自定义模型的连接默认展开该区。直接跟着 payload 走：换账号后新清单到货时再判一次，
  // 否则复位折叠的状态会一直挂着，B 明明有自定义模型却只显示一个"+ 添加"按钮。
  createEffect(() => {
    if ((payload().customModels ?? []).length > 0) setShowCustomModels(true)
  })

  // 切换账号：清掉只属于上一个连接的视图状态，否则 A 的搜索词会继续隐藏 B 的行，
  // A 的探测结果徽标也会挂到同名的 B 模型上，A 未提交的自定义模型草稿也会被加进 B。
  createEffect(on(deps.connectionId, () => {
    generation++
    setModelSearch('')
    setNewCustomModelId('')
    setModelTestResults({})
    setTestingModels({})
    setShowCustomModels(false)
    setEditingModel(null)
    setTestingAll(false)
    setTestAllProgress(null)
  }))

  const filteredRegistryModels = createMemo(() => {
    const q = modelSearch().trim().toLowerCase()
    if (!q) return sortedRegistryModels()
    return sortedRegistryModels().filter(m =>
      (m.name || '').toLowerCase().includes(q) || (m.id || '').toLowerCase().includes(q))
  })

  /** 把一批模型的 enabled 落成目标值。 */
  function mutateEnabled(targets: Set<string>, enabled: boolean) {
    const apply = (list?: ProviderModel[]) =>
      (list ?? []).map(m => (targets.has(modelKeyOf(m)) ? { ...m, enabled } : m))
    mutateModels(prev => ({
      ...prev,
      registryModels: apply(prev?.registryModels),
      customModels: apply(prev?.customModels),
    }))
  }

  /**
   * 乐观落子 → 提交批量启停 → 失败则回读服务端状态。
   * 全部启用 / 全部禁用 / 禁用失败项三条路径只差目标集合、方向与文案。
   */
  async function setModelsEnabled(options: {
    targets: ProviderModel[]
    enabled: boolean
    confirmMessage?: string
    infoMessage: string
  }) {
    const p = deps.providerId()
    if (!p || batchBusy()) return
    if (options.targets.length === 0) {
      toast.info(options.infoMessage)
      return
    }
    if (options.confirmMessage && !await confirm(options.confirmMessage)) return

    const modelKeys = options.targets.map(m => `${p}/${modelKeyOf(m)}`)
    setBatchBusy(true)
    mutateEnabled(new Set(options.targets.map(modelKeyOf)), options.enabled)
    try {
      await store.batchSetModelsDisabled(modelKeys, !options.enabled)
      toast.success(t(options.enabled ? 'toast.batchEnableSuccess' : 'toast.batchDisableSuccess', { count: options.targets.length }))
    } catch {
      refetchModels()
    } finally {
      setBatchBusy(false)
    }
  }

  function toggleTargets(enabled: boolean) {
    return allDisplayModels().filter(m => enabled ? m.enabled === false : m.enabled !== false)
  }

  function handleEnableAll() {
    return setModelsEnabled({
      targets: toggleTargets(true),
      enabled: true,
      infoMessage: t('toast.allModelsEnabled'),
    })
  }

  async function handleDisableAll() {
    const targets = toggleTargets(false)
    await setModelsEnabled({
      targets,
      enabled: false,
      confirmMessage: t('providerDetail.confirmDisableAll', { count: targets.length }),
      infoMessage: t('toast.allModelsDisabled'),
    })
  }

  const failedModelsList = () => {
    const results = modelTestResults()
    const seen = new Set<string>()
    return allDisplayModels().filter(m => {
      const mid = modelKeyOf(m)
      if (!mid || seen.has(mid)) return false
      seen.add(mid)
      return results[mid] && !results[mid].ok && m.enabled !== false
    })
  }

  async function handleDisableFailed() {
    const targets = failedModelsList()
    await setModelsEnabled({
      targets,
      enabled: false,
      confirmMessage: t('providerDetail.confirmDisableFailed', { count: targets.length }),
      infoMessage: t('toast.noFailedModels'),
    })
  }

  async function toggleModel(modelId: string, currentEnabled: boolean) {
    const p = deps.providerId()
    if (!p) return
    const nextState = !currentEnabled

    // 立即乐观更新本地响应式状态
    mutateEnabled(new Set([modelId]), nextState)
    try {
      await store.setModelDisabled(`${p}/${modelId}`, !nextState)
    } catch {
      refetchModels()
    }
  }

  /** 探测单个模型并记录结果；单点测试与批量测试共用，仅失败兜底文案不同。 */
  async function probeModel(modelId: string, fallbackError = t('providerDetail.testRequestError')): Promise<boolean | undefined> {
    const p = deps.providerId()
    if (!p) return undefined
    const gen = generation
    setTestingModels(prev => ({ ...prev, [modelId]: true }))
    try {
      const fullModel = modelId.includes('/') ? modelId : `${p}/${modelId}`
      const res = await store.testModel(fullModel, deps.connectionId())
      if (gen !== generation) return undefined
      setModelTestResults(prev => ({ ...prev, [modelId]: { ok: res.ok, latency: res.latency, error: res.error } }))
      return res.ok
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : fallbackError
      if (gen === generation) {
        setModelTestResults(prev => ({ ...prev, [modelId]: { ok: false, error: msg } }))
      }
      return undefined
    } finally {
      if (gen === generation) setTestingModels(prev => ({ ...prev, [modelId]: false }))
    }
  }

  async function testSingleModel(modelId: string) {
    if (!deps.providerId() || testingModels()[modelId]) return
    const outcome = await probeModel(modelId)
    if (outcome === undefined) return
    const result = modelTestResults()[modelId]
    if (outcome) {
      toast.success(t('toast.modelTestPassed', { id: modelId, latency: result?.latency || t('providerDetail.normal') }))
    } else {
      toast.error(t('toast.modelTestFailed', { id: modelId, error: result?.error || t('providerDetail.testNotPassed') }))
    }
  }

  async function handleTestAll() {
    if (!deps.providerId() || testingAll()) return
    const seen = new Set<string>()
    const targetList = allDisplayModels().filter(m => {
      const k = modelKeyOf(m)
      if (!k || seen.has(k)) return false
      seen.add(k)
      return true
    })
    if (targetList.length === 0) {
      toast.info(t('toast.noTestableModels'))
      return
    }
    setTestingAll(true)
    const gen = generation
    setTestAllProgress({ current: 0, total: targetList.length })

    let completed = 0
    let passed = 0
    let failed = 0
    let index = 0
    // 三条并发，避免对上游造成突发压力
    const worker = async () => {
      while (index < targetList.length) {
        if (gen !== generation) return
        const cur = targetList[index++]
        const modelId = modelKeyOf(cur)
        if (!modelId) continue
        if (await probeModel(modelId, t('providerDetail.testUnexpected'))) passed++
        else failed++
        if (gen !== generation) return
        completed++
        setTestAllProgress({ current: completed, total: targetList.length })
      }
    }
    await Promise.all(Array.from({ length: Math.min(3, targetList.length) }, worker))

    if (gen !== generation) return
    setTestingAll(false)
    setTestAllProgress(null)
    toast.success(t('toast.testAllDone', { passed, failed }))
  }

  async function handleSyncModels() {
    const p = deps.providerId()
    if (!p) return
    setSyncingModels(true)
    try {
      await store.refreshModels(p)
      toast.success(t('toast.syncModelsSuccess'))
      await refetchModels()
    } catch {
      // Error is automatically surfaced by api.ts
    } finally {
      setSyncingModels(false)
    }
  }

  async function addCustomModel(modelId: string, modelName: string) {
    const id = modelId.trim()
    if (!id || !deps.connectionId()) return
    try {
      await store.addProviderModel(deps.connectionId()!, { id, name: modelName.trim() || id })
      toast.success(t('providerDetail.customModelAdded', { id }))
      refetchModels()
    } catch {
      // Error is automatically surfaced by api.ts
    }
  }

  async function removeCustomModel(modelId: string) {
    const connectionId = deps.connectionId()
    if (!connectionId) return
    const ok = await confirm({
      title: t('providerDetail.deleteModelTitle'),
      message: t('providerDetail.deleteModelMessage', { id: modelId }),
      variant: 'danger',
    })
    if (!ok) return
    try {
      await store.deleteProviderModel(connectionId, modelId)
      toast.success(t('providerDetail.modelDeleted', { id: modelId }))
      refetchModels()
    } catch {
      // Error is automatically surfaced by api.ts
    }
  }

  // ── 模型元数据覆写编辑 ──
  const [editingModel, setEditingModel] = createSignal<ProviderModel | null>(null)
  const [editDisplayName, setEditDisplayName] = createSignal('')
  const [editContextLength, setEditContextLength] = createSignal('')
  const [editMaxOutput, setEditMaxOutput] = createSignal('')
  const [savingMeta, setSavingMeta] = createSignal(false)

  function startEditModel(m: ProviderModel) {
    setEditingModel(m)
    setEditDisplayName(m.name || m.id || '')
    setEditContextLength(m.contextLength ? String(m.contextLength) : '')
    setEditMaxOutput(m.maxOutputTokens ? String(m.maxOutputTokens) : '')
  }

  async function submitModelMeta(run: (connectionId: string, modelId: string) => Promise<unknown>) {
    const m = editingModel()
    const connectionId = deps.connectionId()
    if (!m?.id || !connectionId) return
    setSavingMeta(true)
    try {
      await run(connectionId, m.id)
      await refetchModels()
      setEditingModel(null)
    } catch (e) {
      console.error('model metadata write failed:', e)
    } finally {
      setSavingMeta(false)
    }
  }

  function handleSaveModelMeta() {
    return submitModelMeta((connectionId, modelId) => store.saveProviderModelMeta(connectionId, {
      id: modelId,
      displayName: editDisplayName().trim(),
      contextLength: Number(editContextLength()) || 0,
      maxOutputTokens: Number(editMaxOutput()) || 0,
    }))
  }

  function handleResetModelMeta() {
    return submitModelMeta((connectionId, modelId) => store.resetProviderModelMeta(connectionId, modelId))
  }

  return {
    payload,
    activeModels, allDisplayModels, enabledModelsCount, disabledModelsCount,
    sortedRegistryModels, sortedCustomModels, filteredRegistryModels,
    registryCount, registryEnabledCount, customCount,
    showCustomModels, setShowCustomModels, modelSearch, setModelSearch,
    newCustomModelId, setNewCustomModelId,
    syncingModels, handleSyncModels, batchBusy,
    testingModels, modelTestResults, testingAll, testAllProgress,
    toggleModel, testSingleModel, handleTestAll, failedModelsList,
    handleEnableAll, handleDisableAll, handleDisableFailed,
    addCustomModel, removeCustomModel,
    editingModel, setEditingModel, editDisplayName, setEditDisplayName,
    editContextLength, setEditContextLength, editMaxOutput, setEditMaxOutput,
    savingMeta, startEditModel, handleSaveModelMeta, handleResetModelMeta,
  }
}

export type ProviderModelsApi = ReturnType<typeof useProviderModels>
