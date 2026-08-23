export interface ConnectionDTO {
  id: string
  provider: string
  authType: string
  name?: string
  email?: string
  priority: number
  isActive: boolean
  createdAt: string
  updatedAt: string
  data: ConnectionDataDTO
}

export interface ConnectionDataDTO {
  hasApiKey: boolean
  hasAccessToken: boolean
  hasRefreshToken: boolean
  credentialHint?: string
  expiresAt?: string
  baseUrl?: string
  testStatus?: string
  lastError?: string
  rateLimitedUntil?: string
  backoffLevel?: number
  quotaLimit?: number
  quotaPeriod?: string
  providerSpecificData?: Record<string, any>
}

export interface ProviderCatalogItem {
  id: string
  name: string
  alias: string
  aliases: string[]
  apiType: string
  authModes: string[]
  baseUrl: string
  modelsUrl?: string
  modelsAuth?: string
  noAuth?: boolean
}

export interface ApiError {
  code?: string
  message: string
  details?: Record<string, any>
  requestId?: string
}
