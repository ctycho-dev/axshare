import './style.css'
import { mountAccount } from './account'
import { clearRecent, fmtAgo, fmtBytes, fmtIn, loadRecent } from './recent'
import { mountTheme } from './theme'

// Each list shows at most this many rows.
const SHOW = 5

interface MyFile {
  id: string
  ext: string
  bytes: number
  expires: number // Unix seconds
}

const createBtn = document.getElementById('create') as HTMLButtonElement
const mineBox = document.getElementById('mine') as HTMLElement
const mineRows = document.getElementById('mine-rows') as HTMLElement
const mineToggle = document.getElementById('mine-toggle') as HTMLButtonElement
const recentBox = document.getElementById('recent') as HTMLElement
const rows = document.getElementById('recent-rows') as HTMLElement
const info = document.getElementById('server-info') as HTMLElement

let mine: MyFile[] = []
let mineExpanded = false

async function create(): Promise<void> {
  createBtn.disabled = true
  try {
    const res = await fetch('/api/rooms', { method: 'POST', body: '' })
    if (!res.ok) throw new Error(`HTTP ${res.status}`)
    const { id } = (await res.json()) as { id: string }
    location.href = `/room/${id}`
  } catch (err) {
    createBtn.disabled = false
    createBtn.textContent = `failed: ${(err as Error).message}`
  }
}

function span(className: string, text: string): HTMLSpanElement {
  const s = document.createElement('span')
  s.className = className
  s.textContent = text
  return s
}

// One row, used by both lists: short id and file type, then two columns.
function fileRow(id: string, ext: string | undefined, col1: string, col2: string): HTMLAnchorElement {
  const a = document.createElement('a')
  a.className = 'row'
  a.href = `/room/${id}`

  const name = document.createElement('span')
  name.append(span('id', id.slice(0, 8)))
  if (ext && ext !== 'plain') name.append(span('ext', ext))

  a.append(name, span('meta', col1), span('meta', col2))
  return a
}

function renderMine(): void {
  mineBox.hidden = mine.length === 0
  const shown = mineExpanded ? mine : mine.slice(0, SHOW)
  mineRows.replaceChildren(
    ...shown.map((f) => fileRow(f.id, f.ext, fmtBytes(f.bytes), fmtIn(f.expires * 1000))),
  )
  mineToggle.hidden = mine.length <= SHOW
  mineToggle.textContent = mineExpanded ? 'show less' : `show all (${mine.length})`
}

function renderRecent(): void {
  // A file listed under "my files" is not repeated here.
  const owned = new Set(mine.map((f) => f.id))
  const list = loadRecent()
    .filter((r) => !owned.has(r.id))
    .slice(0, SHOW)
  recentBox.hidden = list.length === 0
  rows.replaceChildren(...list.map((r) => fileRow(r.id, r.ext, fmtBytes(r.bytes), fmtAgo(r.seen))))
}

async function loadMine(): Promise<void> {
  try {
    const me = (await (await fetch('/api/me')).json()) as { signed_in: boolean }
    if (!me.signed_in) return
    const r = await fetch('/api/me/rooms')
    if (!r.ok) return
    mine = (await r.json()) as MyFile[]
  } catch {
    return
  }
  renderMine()
  renderRecent()
}

createBtn.addEventListener('click', create)
mineToggle.addEventListener('click', () => {
  mineExpanded = !mineExpanded
  renderMine()
})
document.getElementById('clear')!.addEventListener('click', () => {
  clearRecent()
  renderRecent()
})
document.addEventListener('keydown', (e) => {
  if (document.querySelector('dialog[open]')) return
  if (e.key === 'n' && !e.metaKey && !e.ctrlKey && !e.altKey) create()
})

renderRecent()
loadMine()
mountAccount(document.getElementById('account') as HTMLElement)
mountTheme(document.getElementById('theme') as HTMLElement)

info.textContent = `axshare — ${location.host}`
fetch('/healthz')
  .then((r) => r.json())
  .then((h: { clients: number }) => {
    info.textContent = `axshare — ${location.host} — ${h.clients} online`
  })
  .catch(() => {})