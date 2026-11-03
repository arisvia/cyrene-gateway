import { cleanup, render } from '@solidjs/testing-library'
import { afterEach, describe, expect, it } from 'vitest'
import { RequestDetailModal } from './RequestDetailModal'
import type { RequestDetail } from '@/types/domain'

afterEach(() => {
  cleanup()
  document.body.innerHTML = ''
})

const base: RequestDetail = {
  id: 'x-1',
  provider: 'antigravity',
  model: 'claude-sonnet-4',
  status: '200',
  timestamp: '2026-11-03T00:00:00Z',
}

// Modal renders through a Portal, so content lives on document.body.
const text = () => document.body.textContent || ''

describe('RequestDetailModal', () => {
  it('derives output throughput from completion tokens over latency', () => {
    render(() => (
      <RequestDetailModal
        item={{ ...base, completionTokens: 900, promptTokens: 40_000, latencyMs: 3000 }}
        onClose={() => {}}
      />
    ))
    // 900 tokens / 3s = 300 tok/s. Prompt tokens must not inflate this.
    expect(text()).toContain('300.0')
    expect(text()).toContain('tok/s')
  })

  it('hides the rate when there is nothing to divide', () => {
    render(() => (
      <RequestDetailModal item={{ ...base, latencyMs: 3000 }} onClose={() => {}} />
    ))
    expect(text()).not.toContain('tok/s')

    render(() => (
      <RequestDetailModal item={{ ...base, completionTokens: 900 }} onClose={() => {}} />
    ))
    expect(text()).not.toContain('tok/s')
  })

  it('shows the turn count', () => {
    render(() => (
      <RequestDetailModal item={{ ...base, turns: 7 }} onClose={() => {}} />
    ))
    expect(text()).toContain('7')
  })

  it('explains an "other" client with the raw user agent', () => {
    render(() => (
      <RequestDetailModal
        item={{ ...base, client: 'other', clientRaw: 'SomeNewAgent/1.2' }}
        onClose={() => {}}
      />
    ))
    expect(text()).toContain('SomeNewAgent/1.2')
  })

  it('does not show a raw UA row when the client was classified', () => {
    render(() => (
      <RequestDetailModal
        item={{ ...base, client: 'claude-code', clientRaw: 'claude-cli/2.0' }}
        onClose={() => {}}
      />
    ))
    expect(text()).not.toContain('claude-cli/2.0')
  })
})
