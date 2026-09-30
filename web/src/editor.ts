// CodeMirror 6 setup. Kept separate so room.ts reads as sync logic only.

import { EditorState, Compartment } from '@codemirror/state'
import {
  EditorView,
  keymap,
  lineNumbers,
  highlightActiveLine,
  highlightActiveLineGutter,
  drawSelection,
  rectangularSelection,
  crosshairCursor,
  highlightSpecialChars,
  type ViewUpdate,
} from '@codemirror/view'
import { defaultKeymap, history, historyKeymap, indentWithTab } from '@codemirror/commands'
import {
  bracketMatching,
  indentOnInput,
  syntaxHighlighting,
  HighlightStyle,
  type LanguageSupport,
} from '@codemirror/language'
import { tags as t } from '@lezer/highlight'
import { javascript } from '@codemirror/lang-javascript'
import { go } from '@codemirror/lang-go'
import { python } from '@codemirror/lang-python'
import { rust } from '@codemirror/lang-rust'
import { markdown } from '@codemirror/lang-markdown'
import { json } from '@codemirror/lang-json'
import { yaml } from '@codemirror/lang-yaml'

// Order here is the order in the header <select>.
export const LANGUAGES: { id: string; label: string; load?: () => LanguageSupport }[] = [
  { id: 'plain', label: 'plain' },
  { id: 'ts', label: 'ts', load: () => javascript({ typescript: true }) },
  { id: 'js', label: 'js', load: () => javascript() },
  { id: 'go', label: 'go', load: go },
  { id: 'py', label: 'py', load: python },
  { id: 'rs', label: 'rs', load: rust },
  { id: 'md', label: 'md', load: markdown },
  { id: 'json', label: 'json', load: json },
  { id: 'yaml', label: 'yaml', load: yaml },
]

const byId = new Map(LANGUAGES.map((l) => [l.id, l]))

export function languageFor(id: string): LanguageSupport | [] {
  const l = byId.get(id)
  return l?.load ? l.load() : []
}

// Cheap first guess from the content, used only when the room has no
// saved choice. Wrong guesses cost one click on the selector.
export function guessLanguage(text: string): string {
  const head = text.slice(0, 2000)
  if (/^\s*[{[]/.test(head) && /[}\]]\s*$/.test(text.slice(-200))) return 'json'
  if (/^package \w+|\bfunc \w+\(|:= /m.test(head)) return 'go'
  if (/^(def |class |import |from \w+ import )/m.test(head)) return 'py'
  if (/\bfn \w+\(|\blet mut\b|impl\b.*\{/m.test(head)) return 'rs'
  if (/^(interface |type \w+ = |export (const|function|type)|import .* from ')/m.test(head)) return 'ts'
  if (/\b(const|let|function)\b.*[=(]|=>/.test(head)) return 'js'
  if (/^#{1,6} |^\* |^- |\[.*\]\(.*\)/m.test(head)) return 'md'
  if (/^\w[\w-]*:\s*(\S|$)/m.test(head) && !/[;{}]/.test(head)) return 'yaml'
  return 'plain'
}

// Colors come from CSS variables so light and dark modes are handled in
// style.css, not here. CodeMirror injects these as class rules.
const highlight = HighlightStyle.define([
  { tag: [t.keyword, t.modifier, t.controlKeyword, t.operatorKeyword], color: 'var(--syn-keyword)' },
  { tag: [t.string, t.special(t.string), t.regexp], color: 'var(--syn-string)' },
  { tag: [t.comment, t.lineComment, t.blockComment, t.docComment], color: 'var(--syn-comment)', fontStyle: 'italic' },
  { tag: [t.number, t.integer, t.float, t.bool, t.null, t.atom], color: 'var(--syn-number)' },
  { tag: [t.typeName, t.className, t.namespace, t.tagName], color: 'var(--syn-type)' },
  { tag: [t.function(t.variableName), t.function(t.propertyName), t.definition(t.function(t.variableName))], color: 'var(--syn-function)' },
  { tag: [t.propertyName, t.attributeName, t.labelName], color: 'var(--syn-property)' },
  { tag: [t.variableName, t.definition(t.variableName)], color: 'var(--fg)' },
  { tag: [t.operator, t.punctuation, t.bracket, t.separator], color: 'var(--syn-punct)' },
  { tag: t.heading, fontWeight: 'bold', color: 'var(--syn-keyword)' },
  { tag: t.emphasis, fontStyle: 'italic' },
  { tag: t.strong, fontWeight: 'bold' },
  { tag: t.link, color: 'var(--syn-string)', textDecoration: 'underline' },
  { tag: t.monospace, color: 'var(--syn-property)' },
  { tag: t.invalid, color: 'var(--danger)' },
])

const langCompartment = new Compartment()

export interface EditorHandle {
  view: EditorView
  setText(text: string): void
  getText(): string
  setLanguage(id: string): void
}

// onChange fires only for user edits, not for setText, so the sync layer
// never echoes a remote update back to the server.
export function createEditor(parent: HTMLElement, onChange: (text: string) => void): EditorHandle {
  let applyingRemote = false

  const view = new EditorView({
    parent,
    state: EditorState.create({
      doc: '',
      extensions: [
        lineNumbers(),
        highlightActiveLineGutter(),
        highlightSpecialChars(),
        history(),
        drawSelection(),
        rectangularSelection(),
        crosshairCursor(),
        indentOnInput(),
        bracketMatching(),
        highlightActiveLine(),
        syntaxHighlighting(highlight),
        keymap.of([...defaultKeymap, ...historyKeymap, indentWithTab]),
        langCompartment.of([]),
        EditorView.lineWrapping,
        EditorView.updateListener.of((u: ViewUpdate) => {
          if (u.docChanged && !applyingRemote) onChange(u.state.doc.toString())
        }),
      ],
    }),
  })

  return {
    view,
    getText: () => view.state.doc.toString(),
    setText(text) {
      if (text === view.state.doc.toString()) return
      applyingRemote = true
      const sel = view.state.selection.main.head
      view.dispatch({
        changes: { from: 0, to: view.state.doc.length, insert: text },
        selection: { anchor: Math.min(sel, text.length) },
      })
      applyingRemote = false
    },
    setLanguage(id) {
      view.dispatch({ effects: langCompartment.reconfigure(languageFor(id)) })
    },
  }
}