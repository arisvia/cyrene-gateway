import { api } from './api'

export interface GatewayModel {
  id: string
  object?: string
  owned_by?: string
  display_name?: string
  context_length?: number
  max_output_tokens?: number
}

let cachedPromise: Promise<GatewayModel[]> | null = null

async function loadModels(): Promise<GatewayModel[]> {
  try {
    const res = await api('/v1/models') as { data?: GatewayModel[] } | null
    return res?.data ?? []
  } catch {
    // 失败不驻留缓存：下一次读取重新发起请求，调用方拿到空表自行兜底。
    cachedPromise = null
    return []
  }
}

/** 网关统一模型表（带内存缓存），供只读展示名称等场景使用。 */
export function fetchModels(): Promise<GatewayModel[]> {
  if (!cachedPromise) cachedPromise = loadModels()
  return cachedPromise
}

/** 强制刷新模型表并回填缓存，用于模型目录可能刚变化的交互入口。 */
export function reloadModels(): Promise<GatewayModel[]> {
  cachedPromise = loadModels()
  return cachedPromise
}

/**
 * Build the model ID → friendly display name dictionary (missing names fall
 * back to the raw id). The catalog lists `provider/model` full names while
 * usage rows record the bare id, so each full entry is mirrored onto its
 * bare key; a bare key claimed by two providers with different names is
 * dropped so it falls back to the raw id instead of mislabeling.
 */
export function buildModelDisplayNameMap(list: GatewayModel[]): Record<string, string> {
  const map: Record<string, string> = {}
  for (const m of list) {
    if (m && m.id) map[m.id] = m.display_name?.trim() || m.id
  }
  const bareName: Record<string, string> = {}
  const ambiguous = new Set<string>()
  for (const m of list) {
    if (!m || !m.id) continue
    const i = m.id.lastIndexOf('/')
    if (i < 0) continue
    const name = m.display_name?.trim()
    if (!name) continue
    const bare = m.id.slice(i + 1)
    if (ambiguous.has(bare)) continue
    if (bareName[bare] !== undefined && bareName[bare] !== name) {
      ambiguous.add(bare)
      delete map[bare]
      continue
    }
    bareName[bare] = name
    if (map[bare] === undefined) map[bare] = name
  }
  return map
}

export async function fetchModelDisplayNameMap(): Promise<Record<string, string>> {
  return buildModelDisplayNameMap(await fetchModels())
}

/**
 * 解析展示名称：优先 `provider/model` 全名精确命中（歧义裸 ID 靠行内
 * provider 定名），其次裸键/原键镜像，缺失时回退原始 ID；空输入返回空
 * 字符串，由调用方自行兜底。
 */
export function resolveModelDisplayName(
  map: Record<string, string> | undefined,
  modelId: string | undefined,
  provider?: string,
): string {
  if (!modelId) return ''
  if (provider && !modelId.includes('/')) {
    const qualified = map?.[`${provider}/${modelId}`]
    if (qualified) return qualified
  }
  return map?.[modelId] || modelId
}

/** 规范 `provider/model` 展示 ID：裸 ID 配 provider 时补前缀，供副行统一显示全名。 */
export function canonicalModelId(modelId: string | undefined, provider?: string): string {
  if (!modelId) return ''
  if (provider && !modelId.includes('/')) return `${provider}/${modelId}`
  return modelId
}
