import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, screen, within } from '@solidjs/testing-library'
import { render } from 'solid-js/web'
import type { JSX } from 'solid-js'
import { api, type RequestOptions } from '@/lib/api'
import { t } from '@/i18n'
import { useGatewayStore } from '@/stores/gateway'
import type { Provider } from '@/types/domain'
import Quota from '@/pages/Quota'

vi.mock('@/lib/api', () => ({
  api: vi.fn(), apiPost: vi.fn(), apiPut: vi.fn(), apiPatch: vi.fn(), apiDelete: vi.fn(), setOnUnauthorized: vi.fn(),
}))
vi.mock('@solidjs/router', () => ({
  A: (props: JSX.AnchorHTMLAttributes<HTMLAnchorElement>) => <a {...props} />,
}))

function deferred<T = unknown>() {
  let resolve!: (value: T | PromiseLike<T>) => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<T>((res, rej) => { resolve = res; reject = rej })
  return { promise, resolve, reject }
}

type PendingRequest = ReturnType<typeof deferred> & { path: string; signal?: AbortSignal }
const connections: Provider[] = ['first', 'second'].map(id => ({
  id, provider: 'anthropic', email: `${id}@example.com`, authType: 'api-key', priority: 1, isActive: true,
}))
const summaryPath = '/api/usage/providers'
const quotaPath = (id: string) => `/api/usage/connection/${id}`
const summary = (requests = 123) => ({
  providers: [{ provider: 'anthropic', requests, promptTokens: 100, completionTokens: 50, cost: 0, connections: 2, activeConnections: 2 }],
})
const quota = (remaining = 90) => ({
  plan: 'Pro',
  quotas: { user: { used: 100 - remaining, total: 100, remaining, remainingPercentage: remaining, resetAt: '', unit: 'USD' } },
})
const unsupported = { message: 'Usage API not implemented for anthropic' }
const skeletons = (element: Element) => element.querySelectorAll('.animate-pulse[aria-hidden="true"]')
// Drain all chained promise handlers, including allSettled/finally, before asserting unchanged UI.
const flush = () => new Promise<void>(resolve => setTimeout(resolve, 0))
let requests: PendingRequest[]
let providersResponse: Promise<Provider[]>

function request(path: string, round = 0) {
  const match = requests.filter(r => r.path === path)[round]
  if (!match) throw new Error(`Missing request: ${path}, round ${round}`)
  return match
}

function card(id: string) {
  const element = screen.getByText(`${id}@example.com`).closest('.glass-card')
  if (!(element instanceof HTMLElement)) throw new Error(`Missing card: ${id}`)
  return element
}

const refreshButton = () => screen.getByRole<HTMLButtonElement>('button', { name: t('quota.refreshData') })
const unmounts: (() => void)[] = []

async function mount() {
  const container = document.createElement('div')
  document.body.append(container)
  const dispose = render(() => <Quota />, container)
  const unmount = () => { dispose(); container.remove() }
  unmounts.push(unmount)
  await flush()
  return { unmount }
}

async function refresh() {
  fireEvent.click(refreshButton())
  await flush()
}

beforeEach(() => {
  vi.clearAllMocks()
  requests = []
  providersResponse = Promise.resolve(connections)
  useGatewayStore().setProviders(connections)
  vi.mocked(api).mockImplementation((path: string, options?: string | RequestOptions) => {
    if (path === '/api/providers') return providersResponse
    const pending = deferred()
    requests.push({ ...pending, path: path.split('?')[0], signal: typeof options === 'object' ? options.signal : undefined })
    // Intentionally ignore abort here: stale replies must also be guarded by the page.
    return pending.promise
  })
})

afterEach(async () => {
  for (const unmount of unmounts.splice(0)) unmount()
  for (const pending of requests) pending.reject(new Error('cleanup'))
  await flush()
  useGatewayStore().setProviders([])
  vi.restoreAllMocks()
})

describe('Quota slow-network regressions', () => {
  it('initializes pending before revealing cards after a delayed provider list, without an unsupported flash', async () => {
    const providers = deferred<Provider[]>()
    providersResponse = providers.promise
    useGatewayStore().setProviders([])
    await mount()
    expect(screen.queryByText('first@example.com')).toBeNull()

    const records: MutationRecord[] = []
    const observer = new MutationObserver(mutations => records.push(...mutations))
    observer.observe(document.body, { childList: true, subtree: true })
    try {
      providers.resolve(connections)
      await flush()
      records.push(...observer.takeRecords())
      const renderedText = records.flatMap(record => [...record.addedNodes, ...record.removedNodes])
        .map(node => node.textContent ?? '').join(' ')
      expect(renderedText).not.toContain(t('quota.noOnlineApiShort'))
      expect(renderedText).not.toContain(t('quota.noConnectionsShort'))
      expect(skeletons(card('first')).length).toBeGreaterThan(0)
      expect(skeletons(card('second')).length).toBeGreaterThan(0)
      expect(screen.queryByRole('alert')).toBeNull()
    } finally {
      observer.disconnect()
    }
  })

  it('updates summary and each account independently as their responses arrive', async () => {
    await mount()
    request(summaryPath).resolve(summary())
    await flush()
    expect(card('first').textContent).toContain(t('quota.requestMark', { requests: '123', tokens: '150' }))
    expect(skeletons(card('first')).length).toBeGreaterThan(0)
    expect(skeletons(card('second')).length).toBeGreaterThan(0)

    request(quotaPath('first')).resolve(quota())
    await flush()
    expect(card('first').textContent).toContain('90%')
    expect(skeletons(card('first')).length).toBe(0)
    expect(skeletons(card('second')).length).toBeGreaterThan(0)

    request(quotaPath('second')).resolve({})
    await flush()
    expect(card('second').textContent).toContain(t('quota.noOnlineApiShort'))
    expect(screen.queryByRole('alert')).toBeNull()
  })

  it('renders quotas and unsupported information without waiting for a slow summary', async () => {
    await mount()
    request(quotaPath('first')).resolve(quota())
    request(quotaPath('second')).resolve(unsupported)
    await flush()
    expect(card('first').textContent).toContain('90%')
    expect(card('second').textContent).toContain(t('quota.usageApiUnavailable'))
    expect(skeletons(card('second')).length).toBe(0)
    expect(screen.queryByRole('alert')).toBeNull()

    request(summaryPath).resolve(summary())
    await flush()
    expect(card('first').textContent).toContain(t('quota.requestMark', { requests: '123', tokens: '150' }))
  })

  it('keeps pending accounts and refresh busy when the summary fails, and distinguishes failure from unsupported', async () => {
    await mount()
    await refresh()
    request(summaryPath, 1).reject(new Error('summary unavailable'))
    await flush()
    expect(refreshButton().disabled).toBe(true)
    expect(skeletons(card('first')).length).toBeGreaterThan(0)
    expect(skeletons(card('second')).length).toBeGreaterThan(0)

    request(quotaPath('first'), 1).resolve(quota())
    await flush()
    expect(card('first').textContent).toContain('90%')
    expect(skeletons(card('second')).length).toBeGreaterThan(0)
    expect(refreshButton().disabled).toBe(true)

    request(quotaPath('second'), 1).reject(new Error('quota unavailable'))
    await flush()
    expect(refreshButton().disabled).toBe(false)
    expect(skeletons(card('second')).length).toBe(0)
    expect(within(card('second')).getByRole('alert').textContent).toContain(t('common.error'))
    expect(card('second').textContent).not.toContain(t('quota.noOnlineApiShort'))
    expect(card('second').textContent).not.toContain(t('quota.usageApiUnavailable'))

    fireEvent.click(within(card('second')).getByRole('button', { name: t('common.retry') }))
    await flush()
    expect(screen.queryByRole('alert')).toBeNull()
    expect(skeletons(card('second')).length).toBeGreaterThan(0)
    expect(card('first').textContent).toContain('90%')
    request(summaryPath, 2).resolve(summary())
    request(quotaPath('first'), 2).resolve(quota(60))
    request(quotaPath('second'), 2).resolve(quota(70))
    await flush()
    expect(card('first').textContent).toContain('60%')
    expect(card('second').textContent).toContain('70%')
    expect(refreshButton().disabled).toBe(false)
  })

  it('retains successful details during refresh and on failure, then updates the same quota keys on retry', async () => {
    await mount()
    request(summaryPath).resolve(summary())
    request(quotaPath('first')).resolve(quota())
    request(quotaPath('second')).resolve(unsupported)
    await flush()
    const originalQuota = within(card('first')).getByText('90%')

    await refresh()
    expect(within(card('first')).getByText('90%')).toBe(originalQuota)
    expect(skeletons(card('first')).length).toBe(0)
    expect(card('first').textContent).toContain('Pro')
    expect(card('second').textContent).toContain(t('quota.usageApiUnavailable'))
    expect(skeletons(card('second')).length).toBe(0)
    request(summaryPath, 1).resolve(summary())
    request(quotaPath('first'), 1).reject(new Error('temporary failure'))
    request(quotaPath('second'), 1).resolve(unsupported)
    await flush()
    expect(card('first').textContent).toContain('90%')
    expect(within(card('first')).getByRole('alert').textContent).toContain(t('common.error'))
    expect(card('first').textContent).not.toContain(t('quota.noOnlineApiShort'))
    expect(within(card('second')).queryByRole('alert')).toBeNull()

    fireEvent.click(within(card('first')).getByRole('button', { name: t('common.retry') }))
    await flush()
    expect(screen.queryByRole('alert')).toBeNull()
    expect(within(card('first')).getByText('90%')).toBe(originalQuota)
    request(summaryPath, 2).resolve(summary())
    request(quotaPath('first'), 2).resolve(quota(60))
    request(quotaPath('second'), 2).resolve(unsupported)
    await flush()
    expect(within(card('first')).getByText('60%')).toBe(originalQuota)
    expect(card('first').textContent).not.toContain('90%')
  })

  it.each(['resolve', 'reject'] as const)('ignores aborted round %s handlers, including pending and refresh finalizers', async outcome => {
    await mount()
    const staleRequests = [...requests]
    await refresh()
    expect(staleRequests.every(r => r.signal?.aborted)).toBe(true)
    for (const stale of staleRequests) {
      if (outcome === 'reject') stale.reject(new Error('late abort'))
      else stale.resolve(stale.path === summaryPath ? summary(999) : quota(10))
    }
    await flush()
    expect(refreshButton().disabled).toBe(true)
    expect(skeletons(card('first')).length).toBeGreaterThan(0)
    expect(skeletons(card('second')).length).toBeGreaterThan(0)
    expect(card('first').textContent).not.toContain('10%')
    expect(card('first').textContent).not.toContain('999')
    expect(screen.queryByRole('alert')).toBeNull()
    expect(screen.queryByText(t('quota.noOnlineApiShort'))).toBeNull()

    request(summaryPath, 1).resolve(summary())
    request(quotaPath('first'), 1).resolve(quota())
    request(quotaPath('second'), 1).resolve(unsupported)
    await flush()
    expect(card('first').textContent).toContain('90%')
    expect(card('second').textContent).toContain(t('quota.usageApiUnavailable'))
    expect(refreshButton().disabled).toBe(false)
  })

  it('aborts all in-flight usage requests on unmount', async () => {
    const { unmount } = await mount()
    expect(requests).toHaveLength(3)
    expect(requests.every(r => r.signal && !r.signal.aborted)).toBe(true)
    unmount()
    await flush()
    expect(requests.every(r => r.signal?.aborted)).toBe(true)
    for (const pending of requests) pending.resolve(quota())
    await flush()
    expect(screen.queryByText('first@example.com')).toBeNull()
  })

  it('allows skeleton columns to shrink on narrow cards instead of fixing every width', async () => {
    await mount()
    // happy-dom has no layout engine; guard the flex sizing contract rather than fake pixel measurements.
    const columns = [...skeletons(card('first'))].filter(element => element.classList.contains('h-3'))
    expect(columns).toHaveLength(12)
    expect(columns.every(element => element.classList.contains('min-w-0') && !element.classList.contains('shrink-0'))).toBe(true)
  })
})
