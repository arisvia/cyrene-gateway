export function formatNumber(n: number | undefined | null): string {
  if (n == null) return '0'
  if (n >= 1_000_000) return (n / 1_000_000).toFixed(1) + 'M'
  if (n >= 1_000) return (n / 1_000).toFixed(1) + 'K'
  return String(n)
}

export function formatCost(cost: number | undefined | null): string {
  if (cost == null) return '$0'
  if (cost < 0.01) return '$' + cost.toFixed(4)
  return '$' + cost.toFixed(2)
}

export function timeAgo(dateStr: string | undefined | null): string {
  if (!dateStr) return '—'
  const diff = Date.now() - new Date(dateStr).getTime()
  if (Number.isNaN(diff)) return '—'
  const mins = Math.floor(diff / 60000)
  if (mins < 1) return 'just now'
  if (mins < 60) return `${mins}m ago`
  const hours = Math.floor(mins / 60)
  if (hours < 24) return `${hours}h ago`
  return `${Math.floor(hours / 24)}d ago`
}

export function maskKey(key: string): string {
  if (!key || key.length < 12) return key || ''
  return key.slice(0, 8) + '…' + key.slice(-4)
}
