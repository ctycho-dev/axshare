import './style.css'
import { mountAccount } from './account'
import { clearRecent, fmtAgo, fmtBytes, loadRecent, type Recent } from './recent'

const createBtn = document.getElementById('create') as HTMLButtonElement
const recentBox = document.getElementById('recent') as HTMLElement
const rows = document.getElementById('recent-rows') as HTMLElement
const info = document.getElementById('server-info') as HTMLElement

mountAccount(document.getElementById('account') as HTMLElement)

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

function renderRecent(): void {
  const list = loadRecent()
  recentBox.hidden = list.length === 0
  rows.replaceChildren(
    ...list.map((r: Recent) => {
      const a = document.createElement('a')
      const expired = r.expires <= Date.now()
      a.className = 'row' + (expired ? ' expired' : '')
      a.href = `/room/${r.id}`
      a.innerHTML = `
        <span class="id">${r.id.slice(0, 8)}</span>
        <span class="meta">${expired ? '—' : fmtBytes(r.bytes)}</span>
        <span class="meta">${expired ? 'expired' : fmtAgo(r.seen)}</span>`
      return a
    }),
  )
}

createBtn.addEventListener('click', create)
document.getElementById('clear')!.addEventListener('click', () => {
  clearRecent()
  renderRecent()
})
document.addEventListener('keydown', (e) => {
  if (document.querySelector('dialog[open]')) return
  if (e.key === 'n' && !e.metaKey && !e.ctrlKey && !e.altKey) create()
})

renderRecent()
info.textContent = `axshare — ${location.host}`
fetch('/healthz')
  .then((r) => r.json())
  .then((h: { clients: number }) => {
    info.textContent = `axshare — ${location.host} — ${h.clients} online`
  })
  .catch(() => {})