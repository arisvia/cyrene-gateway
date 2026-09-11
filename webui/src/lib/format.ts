export function formatNumber(n: number | undefined | null): string {
  if (n == null || isNaN(n)) return '0'
  if (n >= 1_000_000) return (n / 1_000_000).toFixed(1) + 'M'
  if (n >= 1_000) return (n / 1_000).toFixed(1) + 'K'
  // 若包含非整型微量浮点，四舍五入至多保留 1 位小数，避免 42.0059 撑破布局
  if (!Number.isInteger(n)) return (Math.round(n * 10) / 10).toString()
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
export type UptimeUnits = { s: string; m: string; h: string; d: string; mLong: string; hLong: string }

const ZH_UPTIME: UptimeUnits = { s: '秒', m: '分', h: '时', d: '天', mLong: '分钟', hLong: '小时' }

export function formatUptime(seconds: number | undefined | null, units: UptimeUnits = ZH_UPTIME): string {
  const sec = Math.max(0, Math.floor(Number(seconds) || 0))
  if (sec < 60) return `${sec} ${units.s}`
  const mins = Math.floor(sec / 60)
  if (mins < 60) return `${mins} ${units.mLong}`
  const hours = Math.floor(mins / 60)
  const remMins = mins % 60
  if (hours < 24) return `${hours} ${units.hLong}${remMins > 0 ? ` ${remMins} ${units.m}` : ''}`
  const days = Math.floor(hours / 24)
  const remHours = hours % 24
  return `${days} ${units.d}${remHours > 0 ? ` ${remHours} ${units.h}` : ''}`
}

export function formatVersion(v: string | undefined | null): string {
  if (!v) return 'dev'
  const match = v.match(/^v?(\d+\.\d+\.\d+)(?:-\d{14}-([a-f0-9]{7,}))?/)
  if (match) {
    const semver = match[1]
    const commit = match[2]
    if (commit) return `${semver} (${commit.slice(0, 7)})`
    return semver
  }
  return v
}
