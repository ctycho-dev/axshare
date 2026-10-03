// Sign-in state: a "login" button that opens a dialog when anonymous, or
// avatar, name and "sign out" when signed in. Used by both pages.

interface Me {
  signed_in: boolean
  name?: string
  avatar_url?: string
  providers: string[]
}

const LABELS: Record<string, string> = { github: 'GitHub', google: 'Google' }

function loginDialog(providers: string[]): HTMLDialogElement {
  const dlg = document.createElement('dialog')
  dlg.className = 'login-dialog'

  const box = document.createElement('div')
  box.className = 'login-box'

  const title = document.createElement('h2')
  title.textContent = 'login'

  const links = providers.map((p) => {
    const a = document.createElement('a')
    a.className = 'provider'
    a.href = `/auth/${p}/login`
    a.textContent = `Continue with ${LABELS[p] ?? p}`
    return a
  })

  const cancel = document.createElement('button')
  cancel.textContent = 'cancel'
  cancel.addEventListener('click', () => dlg.close())

  box.append(title, ...links, cancel)
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