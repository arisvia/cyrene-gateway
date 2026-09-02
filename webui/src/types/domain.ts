export interface Provider {
  id: string
  provider: string
  name?: string
  authType: string
  email?: string
  priority: number
  isActive: boolean
  data?: Record<string, any>
  createdAt?: string
  updatedAt?: string
}

export interface RegistryProvider {
  id: string
  name: string
  category: string
  authType: string
  authModes?: string[]
  baseUrl?: string
  noAuth?: boolean
  hasFree?: boolean
  deviceCodeUrl?: string
  loginUrl?: string
  authorizeUrl?: string
  headers?: Record<string, string>
  models?: string[]
  apiKeyUrl?: string
  brand?: string
  region?: string
  authHint?: string
}

export interface RegistryCategory {
  category: string
  count: number
  providers: RegistryProvider[]
}

export interface Combo {
  id: string
  name: string
  kind: string
  models: string[]
  createdAt?: string
}

export interface ApiKey {
  id: string
  name?: string
  key: string
  isActive: boolean
  createdAt?: string
}

export interface ProxyPool {
  id: string
  isActive: boolean
  data: { name: string; type: string; proxyUrl: string; noProxy?: string; strictProxy?: boolean }
}

export interface Endpoint {
  label: string
  url: string
  type: string
}

export interface ProviderModel {
  name: string
  enabled?: boolean
  alias?: string
  contextLength?: number
  maxOutputTokens?: number
  capabilities?: string[]
  modalities?: string[]
  displayName?: string
  source?: string
}

export interface UsageStats {
  totalRequests?: number
  totalPromptTokens?: number
  totalCompletionTokens?: number
  totalCost?: number
  totalRequestsLifetime?: number
  byProvider?: Record<string, { requests: number; promptTokens: number; completionTokens: number }>
}

export interface RequestDetail {
  id?: string
  timestamp?: string
  provider?: string
  model?: string
  status?: string
  promptTokens?: number
  completionTokens?: number
  cost?: number
  latencyMs?: number
  connectionId?: string
  endpoint?: string
}

export interface Pagination {
  page: number
  pageSize: number
  totalItems: number
  totalPages: number
  hasNext: boolean
  hasPrev: boolean
}

export interface ProviderUsage {
  provider: string
  requests: number
  promptTokens: number
  completionTokens: number
  cost: number
  connections: number
  activeConnections: number
  quotaLimit?: number
  quotaUsed?: number
  overQuota?: boolean
}

export interface CLITool {
  id: string
  name: string
  description?: string
  icon?: string
  configType?: string
  configured?: boolean
}

export interface Skill {
  id: string
  name: string
  description?: string
  content?: string
}
