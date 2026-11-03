import { describe, expect, it } from 'vitest'
import { clientDisplayName, clientDisplayWithVersion } from './clients'

const t = (k: string) => ({ 'usage.clients.browser': '浏览器', 'usage.clients.unknown': '未知', 'usage.clients.other': '其他' }[k] ?? k)

describe('clientDisplayName', () => {
  it('prettifies the products seen in practice', () => {
    expect(clientDisplayName('claude-cli', t)).toBe('Claude Code')
    expect(clientDisplayName('codex_cli_rs', t)).toBe('Codex CLI')
    expect(clientDisplayName('postmanruntime', t)).toBe('Postman')
  })

  it('renders an unseen product without needing a release', () => {
    // The point of parsing instead of cataloguing: this must not read "其他".
    expect(clientDisplayName('some-internal-runner', t)).toBe('Some Internal Runner')
    expect(clientDisplayName('guohongxing', t)).toBe('Guohongxing')
  })

  it('keeps the generic buckets translated', () => {
    expect(clientDisplayName('browser', t)).toBe('浏览器')
    expect(clientDisplayName('unknown', t)).toBe('未知')
    expect(clientDisplayName('other', t)).toBe('其他')
    expect(clientDisplayName('', t)).toBe('-')
  })

  it('appends the version when there is one', () => {
    expect(clientDisplayWithVersion('claude-cli', '2.0.59', t)).toBe('Claude Code 2.0.59')
    expect(clientDisplayWithVersion('claude-cli', undefined, t)).toBe('Claude Code')
  })
})
