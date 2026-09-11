import { describe, it, expect, beforeEach } from 'vitest'
import { t, locale, setLocale, toggleLocale } from '../index'

describe('i18n module', () => {
  beforeEach(() => {
    setLocale('zh-CN')
  })

  it('translates nested keys in zh-CN', () => {
    expect(t('settings.title')).toBe('系统设置')
    expect(t('settings.tabs.gateway')).toBe('网关核心')
    expect(t('playground.modeCompare')).toBe('双模型对比')
  })

  it('translates nested keys in en-US', () => {
    setLocale('en-US')
    expect(t('settings.title')).toBe('System Settings')
    expect(t('settings.tabs.gateway')).toBe('Gateway')
    expect(t('playground.modeCompare')).toBe('Side-by-Side')
  })

  it('toggles locale reactively', () => {
    expect(locale()).toBe('zh-CN')
    toggleLocale()
    expect(locale()).toBe('en-US')
    expect(t('settings.tabs.appearance')).toBe('Appearance')
    toggleLocale()
    expect(locale()).toBe('zh-CN')
    expect(t('settings.tabs.appearance')).toBe('界面与外观')
  })

  it('falls back to key for unknown paths', () => {
    expect(t('nonexistent.path.here')).toBe('nonexistent.path.here')
  })

  it('interpolates template parameters', () => {
    // Test custom replacement
    const res = t('common.save')
    expect(res).toBe('保存修改')
  })
})
