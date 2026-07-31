import { describe, it, expect, vi, beforeEach } from 'vitest'
import { api, apiPost, apiPut, apiDelete } from '@/lib/api'

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
    global.fetch = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      text: () => Promise.resolve(JSON.stringify({ id: '123' })),
    })

    const result = await apiPost('/api/providers', { name: 'test' })
    expect(result).toEqual({ id: '123' })
    const call = (fetch as any).mock.calls[0]
    expect(call[1].method).toBe('POST')
    expect(call[1].body).toBe(JSON.stringify({ name: 'test' }))
    expect(call[1].headers['Content-Type']).toBe('application/json')
  })

  it('throws on non-ok response with error message', async () => {
    global.fetch = vi.fn().mockResolvedValue({
      ok: false,
      status: 404,
      statusText: 'Not Found',
      json: () => Promise.resolve({ error: 'provider not found' }),
    })

    await expect(api('/api/providers/bad')).rejects.toThrow('provider not found')
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
    global.fetch = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      text: () => Promise.resolve('{}'),
    })

    await apiPut('/api/providers/1', { isActive: true })
    const call = (fetch as any).mock.calls[0]
    expect(call[1].method).toBe('PUT')
  })
})
