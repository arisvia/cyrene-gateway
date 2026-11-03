// Downstream client-agent display names.
//
// The gateway parses the User-Agent instead of matching a catalog, so it
// reports whatever product token the caller declared ("claude-cli",
// "codex_cli_rs", "some-internal-runner"). Those are wire values, not display
// names: underscores and all-lowercase look like machine output next to
// everything else in the UI. This table only prettifies the handful seen in
// practice; anything else falls through to a generic title-casing so a new tool
// still renders reasonably without a gateway release.
const CLIENT_DISPLAY_NAMES: Record<string, string> = {
  'claude-cli': 'Claude Code',
  codex_cli_rs: 'Codex CLI',
  'codex-cli': 'Codex CLI',
  opencode: 'OpenCode',
  aider: 'Aider',
  cursor: 'Cursor',
  cline: 'Cline',
  'roo-code': 'Roo Code',
  anthropic: 'Anthropic SDK',
  openai: 'OpenAI SDK',
  'google-genai': 'Gemini SDK',
  'python-requests': 'Python requests',
  'python-httpx': 'Python httpx',
  'go-http-client': 'Go HTTP',
  postmanruntime: 'Postman',
  insomnia: 'Insomnia',
}

export type ClientLabelTranslator = (
  key: 'usage.clients.browser' | 'usage.clients.unknown' | 'usage.clients.other',
) => string

export function clientDisplayName(id: string | undefined | null, t: ClientLabelTranslator): string {
  if (!id) return '-'
  if (id === 'browser') return t('usage.clients.browser')
  if (id === 'unknown') return t('usage.clients.unknown')
  if (id === 'other') return t('usage.clients.other')
  const known = CLIENT_DISPLAY_NAMES[id]
  if (known) return known
  // Unknown product: title-case the token rather than showing it raw. Dots and
  // dashes become spaces so "my-agent" reads as "My Agent".
  return id
    .replace(/[_.-]+/g, ' ')
    .split(' ')
    .filter(Boolean)
    .map(w => w.charAt(0).toUpperCase() + w.slice(1))
    .join(' ')
}

// clientDisplayWithVersion renders the product and version together, which is
// what a detail row needs: "Claude Code 2.0.59".
export function clientDisplayWithVersion(
  id: string | undefined | null,
  version: string | undefined | null,
  t: ClientLabelTranslator,
): string {
  const name = clientDisplayName(id, t)
  if (!version) return name
  return `${name} ${version}`
}
