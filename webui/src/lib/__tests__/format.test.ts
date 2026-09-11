import { describe, it, expect } from 'vitest'
import { formatNumber, formatCost, timeAgo, maskKey, formatUptime, formatVersion } from '@/lib/format'

describe('formatNumber', () => {
  it('returns "0" for null/undefined', () => {
    expect(formatNumber(null)).toBe('0')
    expect(formatNumber(undefined)).toBe('0')
  })

  it('formats millions', () => {
    expect(formatNumber(1_500_000)).toBe('1.5M')
    expect(formatNumber(2_000_000)).toBe('2.0M')
  })

  it('formats thousands', () => {
    expect(formatNumber(1_500)).toBe('1.5K')
    expect(formatNumber(999)).toBe('999')
  })

  it('formats small numbers', () => {
    expect(formatNumber(42)).toBe('42')
    expect(formatNumber(0)).toBe('0')
  })
})

describe('formatCost', () => {
  it('returns "$0" for null/undefined', () => {
    expect(formatCost(null)).toBe('$0')
    expect(formatCost(undefined)).toBe('$0')
  })

  it('formats sub-cent costs with 4 decimals', () => {
    expect(formatCost(0.005)).toBe('$0.0050')
    expect(formatCost(0.001)).toBe('$0.0010')
  })

  it('formats normal costs with 2 decimals', () => {
    expect(formatCost(1.5)).toBe('$1.50')
    expect(formatCost(0.01)).toBe('$0.01')
  })
})

describe('timeAgo', () => {
  it('returns em-dash for empty/null', () => {
    expect(timeAgo('')).toBe('—')
    expect(timeAgo(null)).toBe('—')
    expect(timeAgo(undefined)).toBe('—')
  })

  it('returns "just now" for recent timestamps', () => {
    expect(timeAgo(new Date().toISOString())).toBe('just now')
  })

  it('returns em-dash for invalid dates', () => {
    expect(timeAgo('not-a-date')).toBe('—')
  })
})

describe('maskKey', () => {
  it('masks long keys', () => {
    const key = 'sk-1234567890abcdef'
    expect(maskKey(key)).toBe('sk-12345…cdef')
  })

  it('returns short keys as-is', () => {
    expect(maskKey('short')).toBe('short')
  })

  it('handles empty/null', () => {
    expect(maskKey('')).toBe('')
  })
})
describe('formatUptime', () => {
  it('formats seconds', () => {
    expect(formatUptime(45)).toBe('45 秒')
  })

  it('formats minutes', () => {
    expect(formatUptime(125)).toBe('2 分钟')
  })

  it('formats hours and minutes', () => {
    expect(formatUptime(3660)).toBe('1 小时 1 分')
    expect(formatUptime(7200)).toBe('2 小时')
  })

  it('formats days and hours', () => {
    expect(formatUptime(90000)).toBe('1 天 1 时')
  })

  it('handles null/undefined/0', () => {
    expect(formatUptime(null)).toBe('0 秒')
    expect(formatUptime(undefined)).toBe('0 秒')
    expect(formatUptime(0)).toBe('0 秒')
  })
})

describe('formatVersion', () => {
  it('formats git build commit version to clean string', () => {
    expect(formatVersion('0.0.0-20260911025156-2e77abe8fbe3')).toBe('0.0.0 (2e77abe)')
    expect(formatVersion('v1.2.3-20260911025156-abcdef123456')).toBe('1.2.3 (abcdef1)')
  })

  it('formats standard semver tags', () => {
    expect(formatVersion('1.0.0')).toBe('1.0.0')
    expect(formatVersion('v2.1.0')).toBe('2.1.0')
  })

  it('handles null/undefined/dev', () => {
    expect(formatVersion(null)).toBe('dev')
    expect(formatVersion(undefined)).toBe('dev')
    expect(formatVersion('custom-build')).toBe('custom-build')
  })
})
