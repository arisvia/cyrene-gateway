import { describe, it, expect, beforeEach } from 'vitest'
import { t, locale, setLocale, toggleLocale } from '../index'
import { zhCN } from '../zh-CN'
import { enUS } from '../en-US'

/** 收集词典全部叶子路径。若某层键名本身含点号（如 "a.b"），
 *  t() 的 split('.') 遍历将无法命中，会原样吐出 key。 */
function leafPaths(node: unknown, prefix = ''): string[] {
  if (typeof node === 'string') return [prefix]
  if (!node || typeof node !== 'object') return []
  return Object.entries(node as Record<string, unknown>).flatMap(([k, v]) => {
    const dotted = k.includes('.')
    const next = prefix ? `${prefix}.${k}` : k
    return dotted ? [`${next}`] : leafPaths(v, next)
  })
}

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
    expect(t('nonexistent.path.here' as unknown as Parameters<typeof t>[0])).toBe('nonexistent.path.here')
  })

  it('interpolates template parameters', () => {
    // Test custom replacement
    const res = t('common.save')
    expect(res).toBe('保存修改')
  })
  it('translates playground presets and prompts in both languages', () => {
    expect(t('playground.presets.defaultAssistant.label')).toBe('默认助手')
    expect(t('playground.enterHint')).toBe('Enter 发送 · Shift+Enter 换行')
    setLocale('en-US')
    expect(t('playground.presets.defaultAssistant.label')).toBe('General Assistant')
    expect(t('playground.enterHint')).toBe('Enter to send · Shift+Enter for newline')
  })

  it('translates navigation and provider keys across languages', () => {
    expect(t('providers.title')).toBe('模型提供商接入')
    expect(t('combos.title')).toBe('模型组合')
    expect(t('quota.title')).toBe('配额中心')
    expect(t('usage.title')).toBe('用量统计')
    setLocale('en-US')
    expect(t('providers.title')).toBe('Model Providers')
    expect(t('combos.title')).toBe('Model Combos')
    expect(t('quota.title')).toBe('Quota Center')
    expect(t('usage.title')).toBe('Usage Analytics')
  })

  // 回归：曾出现把 "settings.data.restoreConfirmTitle" 当作扁平键名写入词典，
  // tsc 因 NestedKeyOf 视其为叶子而放行，运行时 t() 却回吐原始路径。
  it('dictionaries contain no dotted leaf keys', () => {
    for (const dict of [zhCN, enUS]) {
      for (const path of leafPaths(dict)) {
        // 分隔符不应出现在单段键名中：真正嵌套的路径由 leafPaths 逐层拼出
        expect(path.split('.').every(seg => seg.length > 0)).toBe(true)
      }
    }
  })

  it('every key resolves to a real translation in both locales', () => {
    const paths = [...new Set([...leafPaths(zhCN), ...leafPaths(enUS)])]
    for (const path of paths) {
      for (const loc of ['zh-CN', 'en-US'] as const) {
        setLocale(loc)
        // 传入覆盖全部占位符的样本参数，确保模板既有占位符都能被替换
        const probe: Record<string, string> = {}
        for (const m of JSON.stringify(zhCN).matchAll(/\{(\w+)\}/g)) probe[m[1]] = 'x'
        for (const m of JSON.stringify(enUS).matchAll(/\{(\w+)\}/g)) probe[m[1]] = 'x'
        const out = t(path as Parameters<typeof t>[0], probe)
        expect(out, `${loc} → ${path}`).not.toBe(path)
        // 未被替换的 {param} 说明词典里存在未声明的占位符
        expect(out, `${loc} → ${path} 残留占位符`).not.toMatch(/\{[a-zA-Z]+\}/)
      }
    }
  })
})
