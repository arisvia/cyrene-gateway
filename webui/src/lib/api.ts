const BASE = ''

export class ApiError extends Error {
  status: number

  constructor(status: number, message: string) {
    super(message)
    this.name = 'ApiError'
    this.status = status
  }
}

export interface RequestOptions {
  method?: string
  body?: unknown
  headers?: Record<string, string>
  signal?: AbortSignal
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
  const res = await fetch(BASE + path, opts)
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
    throw new ApiError(res.status, msg)
  }
  if (res.status === 204) return undefined as T
  const text = await res.text()
  return text ? JSON.parse(text) : undefined as T
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
