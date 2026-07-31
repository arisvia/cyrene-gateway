const BASE = ''

async function request<T = any>(method: string, path: string, body?: unknown): Promise<T> {
  const opts: RequestInit = { method, headers: {} }
  if (body !== undefined) {
    ;(opts.headers as Record<string, string>)['Content-Type'] = 'application/json'
    opts.body = JSON.stringify(body)
  }
  const res = await fetch(BASE + path, opts)
  if (!res.ok) {
    let msg = `${res.status} ${res.statusText}`
    try {
      const j = await res.json()
      if (j.error) msg = j.error
    } catch { /* non-JSON error body */ }
    throw new Error(msg)
  }
  if (res.status === 204) return undefined as T
  const text = await res.text()
  return text ? JSON.parse(text) : undefined as T
}

export const api = <T = any>(path: string) => request<T>('GET', path)
export const apiPost = <T = any>(path: string, body?: unknown) => request<T>('POST', path, body)
export const apiPut = <T = any>(path: string, body?: unknown) => request<T>('PUT', path, body)
export const apiPatch = <T = any>(path: string, body?: unknown) => request<T>('PATCH', path, body)
export const apiDelete = <T = any>(path: string, body?: unknown) => request<T>('DELETE', path, body)
