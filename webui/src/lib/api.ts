export class ApiError extends Error {
  status: number
  constructor(status: number, message: string) {
    super(message)
    this.status = status
  }
}

export async function api<T = any>(path: string, opts: RequestInit = {}): Promise<T> {
  const res = await fetch(path, {
    headers: { 'Content-Type': 'application/json' },
    ...opts,
  })
  if (!res.ok && res.status !== 429) {
    throw new ApiError(res.status, `HTTP ${res.status}`)
  }
  return res.json()
}

export function apiPost<T = any>(path: string, body?: unknown): Promise<T> {
  return api(path, { method: 'POST', body: body !== undefined ? JSON.stringify(body) : undefined })
}

export function apiPut<T = any>(path: string, body?: unknown): Promise<T> {
  return api(path, { method: 'PUT', body: body !== undefined ? JSON.stringify(body) : undefined })
}

export function apiDelete<T = any>(path: string, body?: unknown): Promise<T> {
  return api(path, { method: 'DELETE', body: body !== undefined ? JSON.stringify(body) : undefined })
}

export function apiPatch<T = any>(path: string, body?: unknown): Promise<T> {
  return api(path, { method: 'PATCH', body: body !== undefined ? JSON.stringify(body) : undefined })
}
