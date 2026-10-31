// Downstream client-agent display labels keyed by the gateway's stable
// fingerprint ids (internal/handler/client_agent.go). Proper product/tool
// names need no i18n; generic buckets (browser/unknown/other) resolve via t()
// at the call site.
export const CLIENT_LABELS: Record<string, string> = {
  'claude-code': 'Claude Code',
  'codex-cli': 'Codex CLI',
  'opencode': 'OpenCode',
  'aider': 'Aider',
  'cursor': 'Cursor',
  'cline': 'Cline',
  'roocode': 'Roo Code',
  'windsurf': 'Windsurf',
  'lobechat': 'LobeChat',
  'open-webui': 'Open WebUI',
  'dify': 'Dify',
  'cherry-studio': 'Cherry Studio',
  'chatwise': 'ChatWise',
  'nextchat': 'NextChat',
  'jan': 'Jan',
  'msty': 'Msty',
  'langchain': 'LangChain',
  'anthropic-sdk': 'Anthropic SDK',
  'openai-sdk': 'OpenAI SDK',
  'gemini-sdk': 'Gemini SDK',
  'insomnia': 'Insomnia',
  'postman': 'Postman',
  'apifox': 'Apifox',
  'apipost': 'ApiPost',
  'hoppscotch': 'Hoppscotch',
  'curl': 'curl',
  'wget': 'wget',
  'httpie': 'HTTPie',
  'go-http': 'Go HTTP',
  'bun': 'Bun',
  'node-http': 'Node HTTP',
  'python-http': 'Python HTTP',
  'java-http': 'Java HTTP',
}

export type ClientLabelTranslator = (key: 'usage.clients.browser' | 'usage.clients.unknown' | 'usage.clients.other') => string

export function clientDisplayName(id: string | undefined | null, t: ClientLabelTranslator): string {
  if (!id) return '-'
  switch (id) {
    case 'browser':
      return t('usage.clients.browser')
    case 'unknown':
      return t('usage.clients.unknown')
    case 'other':
      return t('usage.clients.other')
    default:
      return CLIENT_LABELS[id] ?? id
  }
}
