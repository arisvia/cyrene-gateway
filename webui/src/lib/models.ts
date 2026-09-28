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
 * 模型 ID 到友好展示名称的映射字典（缺失名称回退至 raw id）。
 */
export async function fetchModelDisplayNameMap(): Promise<Record<string, string>> {
  const list = await fetchModels()
  const map: Record<string, string> = {}
  for (const m of list) {
    if (m && m.id) map[m.id] = m.display_name?.trim() || m.id
  }
  return map
}

/**
 * 解析展示名称，如果不存在则回退至原始 ID；空输入返回空字符串，由调用方自行兜底。
 */
export function resolveModelDisplayName(map: Record<string, string> | undefined, modelId: string | undefined): string {
  if (!modelId) return ''
  return map?.[modelId] || modelId
}
