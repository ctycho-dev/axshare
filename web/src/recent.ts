// Recent rooms, kept in localStorage on this browser only. The server has
// no listing endpoint on purpose (unguessable IDs are the access control),
// so "recent" can only ever mean "rooms this machine has opened".

export interface Recent {
  id: string
  bytes: number
  seen: number // ms since epoch
  expires: number // ms since epoch
}

const KEY = 'axshare.recent'
const MAX = 8

export function loadRecent(): Recent[] {
  try {
    const raw = localStorage.getItem(KEY)
    return raw ? (JSON.parse(raw) as Recent[]) : []
  } catch {
    return []
  }
}

export function touchRecent(entry: Recent): void {
  const rest = loadRecent().filter((r) => r.id !== entry.id)
  save([entry, ...rest].slice(0, MAX))
}

export function markExpired(id: string): void {
  save(loadRecent().map((r) => (r.id === id ? { ...r, expires: 0 } : r)))
}

export function clearRecent(): void {
  save([])
}

function save(list: Recent[]): void {
  try {
    localStorage.setItem(KEY, JSON.stringify(list))
  } catch {
    /* private mode or quota: recent list is a convenience, not state */
  }
}

export function fmtBytes(n: number): string {
  if (n < 1024) return `${n} B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`
  return `${(n / 1024 / 1024).toFixed(1)} MB`
}

export function fmtAgo(ms: number): string {
  const s = Math.max(0, Math.round((Date.now() - ms) / 1000))
  if (s < 60) return 'just now'
  const m = Math.round(s / 60)
  if (m < 60) return `${m}m ago`
  const h = Math.round(m / 60)
  if (h < 24) return `${h}h ago`
  const d = Math.round(h / 24)
  return d === 1 ? 'yesterday' : `${d}d ago`
}

export function fmtIn(ms: number): string {
  const s = Math.round((ms - Date.now()) / 1000)
  if (s <= 0) return 'expired'
  const h = Math.floor(s / 3600)
  const m = Math.floor((s % 3600) / 60)
  return h > 0 ? `expires in ${h}h ${m}m` : `expires in ${m}m`
}