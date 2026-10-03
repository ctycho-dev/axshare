// Theme choice: auto (follow the OS), light or dark, stored per browser.
// The attribute is first applied by a small inline script in <head>, before
// the page paints; this module adds the control that changes it.

type Theme = 'auto' | 'light' | 'dark'

const KEY = 'axshare.theme'
const ORDER: Theme[] = ['auto', 'light', 'dark']

function current(): Theme {
  const t = document.documentElement.dataset.theme
  return t === 'light' || t === 'dark' ? t : 'auto'
}

function apply(t: Theme): void {
  if (t === 'auto') delete document.documentElement.dataset.theme
  else document.documentElement.dataset.theme = t
  try {
    if (t === 'auto') localStorage.removeItem(KEY)
    else localStorage.setItem(KEY, t)
  } catch {
    /* private mode: the choice lasts for this page only */
  }
}

export function mountTheme(el: HTMLElement): void {
  const btn = document.createElement('button')
  btn.className = 'theme-toggle'
  btn.title = 'switch theme'
  const label = () => {
    btn.textContent = `◐ ${current()}`
  }
  btn.addEventListener('click', () => {
    apply(ORDER[(ORDER.indexOf(current()) + 1) % ORDER.length])
    label()
  })
  label()
  el.replaceChildren(btn)
}