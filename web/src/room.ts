import './style.css'
import { mountAccount } from './account'
import { createEditor, guessLanguage, LANGUAGES } from './editor'
import { fmtBytes, fmtIn, markExpired, touchRecent } from './recent'

const id = location.pathname.split('/').filter(Boolean)[1] ?? ''
const TTL_MS = 24 * 60 * 60 * 1000

const $ = <T extends HTMLElement>(sel: string) => document.querySelector(sel) as T
const statusEl = $<HTMLElement>('#status')
const bytesEl = $<HTMLElement>('#bytes')
const expiresEl = $<HTMLElement>('#expires')
const editorEl = $<HTMLElement>('#editor')
const goneEl = $<HTMLElement>('#gone')
const copyBtn = $<HTMLButtonElement>('#copy')
const langSel = $<HTMLSelectElement>('#lang')

$<HTMLElement>('#room-id').textContent = id.slice(0, 8)
document.title = `Axshare Room | ${id.slice(0, 8)}`

let ws: WebSocket | null = null
let expiresAt = Date.now() + TTL_MS
let lastSentAt = 0
let retry = 0

type Conn = 'connecting' | 'live' | 'reconnecting' | 'offline'
function setStatus(state: Conn, text: string = state) {
  statusEl.dataset.state = state
  statusEl.textContent = text
}

const editor = createEditor(editorEl, (text) => {
  // Debounce: one frame per 150ms of typing, whole buffer, last write wins.
  clearTimeout(sendTimer)
  sendTimer = window.setTimeout(() => send(text), 150)
})
let sendTimer = 0

// --- language ---
// The room's file type lives on the server. Picking one in the header saves
// it for every viewer; they see it on their next load.
for (const l of LANGUAGES) langSel.add(new Option(l.label, l.id))
let langChosen = false

function applyLanguage(lang: string) {
  // A file type this build doesn't know shows as plain.
  const known = LANGUAGES.some((l) => l.id === lang) ? lang : 'plain'
  langSel.value = known
  editor.setLanguage(known)
}

langSel.addEventListener('change', async () => {
  const lang = langSel.value
  langChosen = true
  applyLanguage(lang)
  try {
    await fetch(`/api/rooms/${id}`, {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ ext: lang }),
    })
  } catch {
    /* offline: the choice still applies on this page */
  }
})

function send(text: string) {
  if (!ws || ws.readyState !== WebSocket.OPEN) return
  ws.send(text)
  lastSentAt = Date.now()
  expiresAt = lastSentAt + TTL_MS // server extends TTL on every persist
  bump(text)
}

function bump(text: string) {
  const bytes = new TextEncoder().encode(text).length
  bytesEl.textContent = fmtBytes(bytes)
  touchRecent({ id, bytes, seen: Date.now(), expires: expiresAt })
}

function connect() {
  const proto = location.protocol === 'https:' ? 'wss' : 'ws'
  ws = new WebSocket(`${proto}://${location.host}/ws/${id}`)
  let first = true

  ws.onopen = () => {
    retry = 0
    setStatus('live')
  }
  ws.onmessage = (e: MessageEvent<string>) => {
    editor.setText(e.data)
    if (first) {
      first = false
      editor.view.focus()
      // Nobody has picked a file type yet: guess one for this viewer only.
      if (!langChosen) applyLanguage(guessLanguage(e.data))
      // The language is final now, so the selector can appear.
      langSel.style.visibility = 'visible'
    }
    bump(e.data)
  }
  ws.onclose = (e) => {
    ws = null
    // 1008 is what the server sends for an unknown/expired room (policy
    // violation) via the HTTP 404 before upgrade; the browser surfaces it
    // as a close with no open. Distinguish that from a network drop.
    if (first) {
      showGone()
      return
    }
    if (e.code === 1000) {
      setStatus('offline', 'closed')
      return
    }
    const delay = Math.min(10_000, 500 * 2 ** retry++)
    setStatus('reconnecting', `reconnecting in ${Math.round(delay / 1000)}s`)
    setTimeout(connect, delay)
  }
  ws.onerror = () => {
    /* onclose follows; handled there */
  }
}

function showGone() {
  editorEl.hidden = true
  goneEl.hidden = false
  setStatus('offline', 'not found')
  markExpired(id)
}

copyBtn.addEventListener('click', async () => {
  await navigator.clipboard.writeText(location.href)
  copyBtn.classList.add('done')
  copyBtn.textContent = 'copied'
  setTimeout(() => {
    copyBtn.classList.remove('done')
    copyBtn.textContent = 'copy link'
  }, 1200)
})

setInterval(() => {
  expiresEl.textContent = fmtIn(expiresAt)
}, 1000)

// The room must exist before we open a socket. The same response carries
// the room's file type and real expiry in headers.
fetch(`/api/rooms/${id}`).then((r) => {
  if (r.status === 404) {
    showGone()
    return
  }
  if (!r.ok) {
    setStatus('offline', `error ${r.status}`)
    return
  }

  const ext = r.headers.get('X-Room-Ext') ?? 'plain'
  if (ext !== 'plain') {
    langChosen = true
    applyLanguage(ext)
  }
  const expires = Number(r.headers.get('X-Room-Expires'))
  if (expires > 0) expiresAt = expires * 1000

  connect()
})

mountAccount(document.getElementById('account') as HTMLElement)