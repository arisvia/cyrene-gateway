import { toast } from './toast'
import { t } from '@/i18n'

const BASE = ''

export class ApiError extends Error {
  status: number

  constructor(status: number, message: string) {
    super(message)
    this.name = 'ApiError'
    this.status = status
  }
}

type UnauthorizedHandler = () => void
let onUnauthorizedCallback: UnauthorizedHandler | null = null

export function setOnUnauthorized(handler: UnauthorizedHandler | null) {
  onUnauthorizedCallback = handler
}

export interface RequestOptions {
  method?: string
  body?: unknown
  headers?: Record<string, string>
  signal?: AbortSignal
  /** Suppress automated toast notifications for background polling or custom-handled errors */
  silent?: boolean
}

let lastToastMsg = ''
let lastToastTime = 0

function showApiToast(msg: string) {
  const now = Date.now()
  if (msg === lastToastMsg && now - lastToastTime < 1500) {
    return
  }
  lastToastMsg = msg
  lastToastTime = now
  toast.error(msg)
}
async function request<T = unknown>(path: string, options: RequestOptions = {}): Promise<T> {
  const method = options.method ?? 'GET'
  const opts: RequestInit = {
    method,
    headers: { ...options.headers },
    signal: options.signal,
  }
  if (options.body !== undefined) {
    ;(opts.headers as Record<string, string>)['Content-Type'] = 'application/json'
    opts.body = JSON.stringify(options.body)
  }
  let res: Response
  try {
    res = await fetch(BASE + path, opts)
  } catch (err: unknown) {
    if (err instanceof Error && err.name === 'AbortError') {
      throw err
    }
    const netMsg = t('api.networkError')
    if (!options.silent) {
      showApiToast(netMsg)
    }
    throw new ApiError(0, netMsg)
  }

  if (!res.ok) {
    let msg = `${res.status} ${res.statusText}`
    try {
      const j = await res.json()
      if (typeof j.error === 'string') {
        msg = j.error
      } else if (j.error?.message) {
        msg = j.error.message
      } else if (j.message) {
        msg = typeof j.message === 'string' ? j.message : JSON.stringify(j.message)
      } else if (j.error) {
        msg = JSON.stringify(j.error)
      }
    } catch { /* non-JSON error body */ }

    if (res.status === 401 && !path.startsWith('/api/auth/')) {
      onUnauthorizedCallback?.()
      if (!options.silent) {
        showApiToast(msg === '401 Unauthorized' || msg === 'unauthorized' ? t('api.authRequired') : msg)
      }
    } else if (!options.silent) {
      showApiToast(msg)
    }

    throw new ApiError(res.status, msg)
  }
  if (res.status === 204) return undefined as T
  const text = await res.text()
  return text ? JSON.parse(text) : (undefined as T)
}

export const api = <T = unknown>(path: string, methodOrOptions?: string | RequestOptions) => {
  if (typeof methodOrOptions === 'string') {
    return request<T>(path, { method: methodOrOptions })
  }
  return request<T>(path, methodOrOptions)
}

export const apiPost = <T = unknown>(path: string, body?: unknown, options?: Omit<RequestOptions, 'method' | 'body'>) =>
  request<T>(path, { method: 'POST', body, ...options })

export const apiPut = <T = unknown>(path: string, body?: unknown, options?: Omit<RequestOptions, 'method' | 'body'>) =>
  request<T>(path, { method: 'PUT', body, ...options })

export const apiPatch = <T = unknown>(path: string, body?: unknown, options?: Omit<RequestOptions, 'method' | 'body'>) =>
  request<T>(path, { method: 'PATCH', body, ...options })

export const apiDelete = <T = unknown>(path: string, body?: unknown, options?: Omit<RequestOptions, 'method' | 'body'>) =>
  request<T>(path, { method: 'DELETE', body, ...options })
