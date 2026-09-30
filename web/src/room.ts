import './style.css'
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
document.title = `${id.slice(0, 8)} — axshare`

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
const LANG_KEY = `axshare.lang.${id}`
for (const l of LANGUAGES) langSel.add(new Option(l.label, l.id))
function applyLanguage(lang: string, persist: boolean) {
  langSel.value = lang
  editor.setLanguage(lang)
  if (persist) {
    try { localStorage.setItem(LANG_KEY, lang) } catch { /* ignore */ }
  }
}
langSel.addEventListener('change', () => applyLanguage(langSel.value, true))

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
      let saved: string | null = null
      try { saved = localStorage.getItem(LANG_KEY) } catch { /* ignore */ }
      applyLanguage(saved ?? guessLanguage(e.data), false)
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

// The room must exist before we open a socket; the GET also tells us
// nothing is there (404) so the page can say so instead of spinning.
fetch(`/api/rooms/${id}`).then((r) => {
  if (r.status === 404) {
    showGone()
    return
  }
  if (!r.ok) {
    setStatus('offline', `error ${r.status}`)
    return
  }
  connect()
})