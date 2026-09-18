import { api } from './api'

export interface ModelLookupEntry {
  id: string
  name: string
  displayName?: string
}

let cachedPromise: Promise<Record<string, string>> | null = null

/**
 * 获取模型 ID 到友好展示名称的映射字典（带内存缓存与回退）。
 * 请求失败或模型不存在时回退至 raw model id。
 */
export async function fetchModelDisplayNameMap(): Promise<Record<string, string>> {
  if (!cachedPromise) {
    cachedPromise = api('/v1/models')
      .then((res: unknown) => {
        const map: Record<string, string> = {}
        const data = (res as { data?: Array<{ id: string; display_name?: string }> })?.data
        if (Array.isArray(data)) {
          for (const m of data) {
            if (m && m.id) {
              map[m.id] = m.display_name?.trim() || m.id
            }
          }
        }
        return map
      })
      .catch(() => {
        cachedPromise = null
        return {}
      })
  }
  return cachedPromise
}

/**
 * 解析展示名称，如果不存在则回退至原始 ID；空输入返回空字符串，由调用方自行兜底。
 */
export function resolveModelDisplayName(map: Record<string, string> | undefined, modelId: string | undefined): string {
  if (!modelId) return ''
  return map?.[modelId] || modelId
}
