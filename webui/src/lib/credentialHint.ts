import { t } from '@/i18n'
import type { I18nKey } from '@/i18n/types'

/**
 * 上游凭证的输入形态各不相同，集中一张表，页面不再各写一份提供商 id 三元。
 * 键是规范后的提供商 id（别名在写入与读取边界都已归一）。
 */
const KEY_PLACEHOLDER_KEYS: Record<string, I18nKey> = {
  qoder: 'providerDetail.patKeyPlaceholder',
  'qoder-cn': 'providerDetail.patKeyPlaceholder',
  opencode: 'providerDetail.opencodeKeyPlaceholder',
  'opencode-go': 'providerDetail.opencodeKeyPlaceholder',
}

/** 未登记的提供商按 OpenAI 兼容 Key 的常见形态提示。 */
const DEFAULT_KEY_PLACEHOLDER = 'sk-...'

export function keyPlaceholder(providerId?: string): string {
  const i18nKey = providerId ? KEY_PLACEHOLDER_KEYS[providerId] : undefined
  return i18nKey ? t(i18nKey) : DEFAULT_KEY_PLACEHOLDER
}
