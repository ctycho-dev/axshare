// Sign-in state: a "login" button that opens a dialog when anonymous, or
// avatar, name and "sign out" when signed in. Used by both pages.

interface Me {
  signed_in: boolean
  name?: string
  avatar_url?: string
  providers: string[]
}

const LABELS: Record<string, string> = { github: 'GitHub', google: 'Google' }

// Static markup, never user data, so innerHTML is safe for these.
const ICONS: Record<string, string> = {
  github:
    '<svg viewBox="0 0 16 16" aria-hidden="true"><path fill="currentColor" d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82.64-.18 1.32-.27 2-.27.68 0 1.36.09 2 .27 1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.013 8.013 0 0016 8c0-4.42-3.58-8-8-8z"/></svg>',
  google:
    '<svg viewBox="0 0 18 18" aria-hidden="true"><path fill="#4285F4" d="M17.64 9.2c0-.637-.057-1.251-.164-1.84H9v3.481h4.844c-.209 1.125-.843 2.078-1.796 2.717v2.258h2.908c1.702-1.567 2.684-3.874 2.684-6.615z"/><path fill="#34A853" d="M9 18c2.43 0 4.467-.806 5.956-2.18l-2.908-2.259c-.806.54-1.837.86-3.048.86-2.344 0-4.328-1.584-5.036-3.711H.957v2.332A8.997 8.997 0 0 0 9 18z"/><path fill="#FBBC05" d="M3.964 10.71A5.41 5.41 0 0 1 3.682 9c0-.593.102-1.17.282-1.71V4.958H.957A8.996 8.996 0 0 0 0 9c0 1.452.348 2.827.957 4.042l3.007-2.332z"/><path fill="#EA4335" d="M9 3.58c1.321 0 2.508.454 3.44 1.345l2.582-2.58C13.463.891 11.426 0 9 0A8.997 8.997 0 0 0 .957 4.958L3.964 7.29C4.672 5.163 6.656 3.58 9 3.58z"/></svg>',
}

function el<K extends keyof HTMLElementTagNameMap>(tag: K, className: string, text = ''): HTMLElementTagNameMap[K] {
  const node = document.createElement(tag)
  node.className = className
  if (text) node.textContent = text
  return node
}

function loginDialog(providers: string[]): HTMLDialogElement {
  const dlg = el('dialog', 'login-dialog')
  const box = el('div', 'login-box')

  const head = el('div', 'login-head')
  const close = el('button', 'login-close', 'esc')
  close.setAttribute('aria-label', 'close')
  close.addEventListener('click', () => dlg.close())
  head.append(el('span', 'login-title', 'login'), close)

  const note = el('p', 'login-note', 'Optional. Everything works without an account.')

  const list = el('div', 'login-providers')
  providers.forEach((p, i) => {
    const a = el('a', 'provider')
    a.href = `/auth/${p}/login`
    if (i === 0) a.setAttribute('autofocus', '')

    const icon = el('span', 'icon')
    icon.innerHTML = ICONS[p] ?? ''
    a.append(icon, el('span', 'label', `continue with ${LABELS[p] ?? p}`), el('span', 'arrow', '→'))
    list.append(a)
  })

  const foot = el('div', 'login-foot')
  const privacy = el('a', '', 'privacy')
  privacy.href = '/privacy.html'
  foot.append(privacy)

  box.append(head, note, list, foot)
  dlg.append(box)

  // The dialog has no padding, so a click whose target is the dialog
  // itself landed on the backdrop: close.
  dlg.addEventListener('click', (e) => {
    if (e.target === dlg) dlg.close()
  })

  document.body.append(dlg)
  return dlg
}

export async function mountAccount(el: HTMLElement): Promise<void> {
  let me: Me
  try {
    const r = await fetch('/api/me')
    if (!r.ok) return
    me = (await r.json()) as Me
  } catch {
    return
  }

  if (!me.signed_in) {
    if (me.providers.length === 0) return
    const dlg = loginDialog(me.providers)
    const btn = document.createElement('button')
    btn.textContent = 'login'
    btn.addEventListener('click', () => dlg.showModal())
    el.replaceChildren(btn)
    return
  }

  const parts: HTMLElement[] = []
  if (me.avatar_url) {
    const img = document.createElement('img')
    img.src = me.avatar_url
    img.alt = ''
    img.width = 20
    img.height = 20
    parts.push(img)
  }
  const name = document.createElement('span')
  name.textContent = me.name ?? ''
  parts.push(name)

  const out = document.createElement('button')
  out.textContent = 'sign out'
  out.addEventListener('click', async () => {
    await fetch('/auth/logout', { method: 'POST' })
    location.reload()
  })
  parts.push(out)

  el.replaceChildren(...parts)
}