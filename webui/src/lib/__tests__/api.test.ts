import { describe, it, expect, vi, beforeEach } from 'vitest'
import { api, apiPost, apiPut, apiDelete, ApiError } from '@/lib/api'

describe('api lib', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
  })

  it('api() sends GET and parses JSON', async () => {
    const mockData = { version: '1.0.0' }
    global.fetch = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      text: () => Promise.resolve(JSON.stringify(mockData)),
    })

    const result = await api('/api/version')
    expect(result).toEqual(mockData)
    expect(fetch).toHaveBeenCalledWith('/api/version', expect.objectContaining({ method: 'GET' }))
  })

  it('apiPost() sends POST with JSON body', async () => {
    const mockFetch = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      text: () => Promise.resolve(JSON.stringify({ id: '123' })),
    })
    global.fetch = mockFetch

    const result = await apiPost('/api/providers', { name: 'test' })
    expect(result).toEqual({ id: '123' })
    const call = mockFetch.mock.calls[0] as [string, RequestInit]
    expect(call[1].method).toBe('POST')
    expect(call[1].body).toBe(JSON.stringify({ name: 'test' }))
    expect((call[1].headers as Record<string, string>)['Content-Type']).toBe('application/json')
  })

  it('throws on non-ok response with error message', async () => {
    global.fetch = vi.fn().mockResolvedValue({
      ok: false,
      status: 404,
      statusText: 'Not Found',
      json: () => Promise.resolve({ error: 'provider not found' }),
    })

    try {
      await api('/api/providers/bad')
      expect.unreachable()
    } catch (err: unknown) {
      expect(err).toBeInstanceOf(ApiError)
      expect((err as ApiError).status).toBe(404)
      expect((err as ApiError).message).toBe('provider not found')
    }
  })

  it('propagates AbortSignal to fetch', async () => {
    const controller = new AbortController()
    global.fetch = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      text: () => Promise.resolve('{"ok":true}'),
    })

    await api('/api/test', { signal: controller.signal })
    expect(fetch).toHaveBeenCalledWith('/api/test', expect.objectContaining({ signal: controller.signal }))
  })

  it('returns undefined for 204 responses', async () => {
    global.fetch = vi.fn().mockResolvedValue({
      ok: true,
      status: 204,
      text: () => Promise.resolve(''),
    })

    const result = await apiDelete('/api/keys/123')
    expect(result).toBeUndefined()
  })

  it('apiPut() sends PUT', async () => {
    const mockFetch = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      text: () => Promise.resolve('{}'),
    })
    global.fetch = mockFetch

    await apiPut('/api/providers/1', { isActive: true })
    const call = mockFetch.mock.calls[0] as [string, RequestInit]
    expect(call[1].method).toBe('PUT')
  })
  it('apiDelete() sends DELETE with optional body', async () => {
    const mockFetch = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      text: () => Promise.resolve('{"ok":true}'),
    })
    global.fetch = mockFetch

    await apiDelete('/api/models/alias', { alias: 'test-alias' })
    const call = mockFetch.mock.calls[0] as [string, RequestInit]
    expect(call[1].method).toBe('DELETE')
    expect((call[1].headers as Record<string, string>)['Content-Type']).toBe('application/json')
    expect(call[1].body).toBe(JSON.stringify({ alias: 'test-alias' }))
  })
})
