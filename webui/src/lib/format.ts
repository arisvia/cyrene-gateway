export function formatNum(n: number | null | undefined): string {
  return n == null ? '0' : n.toLocaleString()
}

export function formatDate(ts: string | null | undefined): string {
  if (!ts) return '—'
  return new Date(ts).toLocaleString(undefined, { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' })
}

export function formatUptime(s: number | null | undefined): string {
  if (s == null) return '—'
  const d = Math.floor(s / 86400)
  const h = Math.floor((s % 86400) / 3600)
  const m = Math.floor((s % 3600) / 60)
  return d > 0 ? `${d}d ${h}h` : h > 0 ? `${h}h ${m}m` : `${m}m`
}

export function maskKey(key: string | null | undefined): string {
  if (!key || key.length <= 12) return key || ''
  return key.slice(0, 8) + '····' + key.slice(-4)
}

export function copyText(text: string) {
  navigator.clipboard?.writeText(text).catch(() => {})
}
