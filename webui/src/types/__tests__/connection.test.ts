import { describe, it, expect } from 'vitest'
import type { ConnectionDTO } from '@/types/connection'

describe('ConnectionDTO contract test', () => {
  it('validates secret-redacted connection DTO structure', () => {
    const mockDTO: ConnectionDTO = {
      id: 'conn-1',
      provider: 'openai',
      authType: 'api-key',
      priority: 0,
      isActive: true,
      createdAt: '2026-08-23T00:00:00Z',
      updatedAt: '2026-08-23T00:00:00Z',
      data: {
        hasApiKey: true,
        hasAccessToken: false,
        hasRefreshToken: false,
        credentialHint: '...1234',
        baseUrl: 'https://api.openai.com/v1',
      },
    }

    expect(mockDTO.data.hasApiKey).toBe(true)
    expect(mockDTO.data.credentialHint).toBe('...1234')
    expect((mockDTO.data as any).apiKey).toBeUndefined()
    expect((mockDTO.data as any).accessToken).toBeUndefined()
    expect((mockDTO.data as any).refreshToken).toBeUndefined()
  })
})
