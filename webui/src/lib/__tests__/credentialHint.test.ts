import { describe, it, expect } from 'vitest'
import { t } from '@/i18n'
import { keyPlaceholder } from '@/lib/credentialHint'

describe('keyPlaceholder 按上游给出凭证输入形态', () => {
  it('PAT 类提供商提示 pt- 形态', () => {
    for (const id of ['qoder', 'qoder-cn']) {
      expect(keyPlaceholder(id)).toBe(t('providerDetail.patKeyPlaceholder'))
    }
  })

  it('OpenCode 走自己的文案，中英文都受 i18n 约束', () => {
    for (const id of ['opencode', 'opencode-go']) {
      expect(keyPlaceholder(id)).toBe(t('providerDetail.opencodeKeyPlaceholder'))
    }
  })

  it('未登记的提供商与缺省都退回 sk- 形态', () => {
    expect(keyPlaceholder('github')).toBe('sk-...')
    expect(keyPlaceholder('custom-openai')).toBe('sk-...')
    expect(keyPlaceholder(undefined)).toBe('sk-...')
    expect(keyPlaceholder('')).toBe('sk-...')
  })

  it('只认规范 id：前端不再复刻一份后端别名表', () => {
    // 提供商 id 已在写入与读取边界归一，所以这里刻意不接受简写；
    // 若在前端补一张别名表，就会与注册表各写一份而同步失败。
    expect(keyPlaceholder('qd')).toBe('sk-...')
    expect(keyPlaceholder('qoder')).not.toBe('sk-...')
  })
})
