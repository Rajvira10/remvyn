// CodeMirror 6 wrapper. Loaded lazily so the file browser doesn't pay for
// the editor until a file is opened.
import { closeBrackets, closeBracketsKeymap } from '@codemirror/autocomplete'
import { defaultKeymap, history, historyKeymap, indentWithTab } from '@codemirror/commands'
import {
  bracketMatching,
  defaultHighlightStyle,
  foldGutter,
  foldKeymap,
  indentOnInput,
  indentUnit,
  syntaxHighlighting,
} from '@codemirror/language'
import { highlightSelectionMatches, openSearchPanel, searchKeymap } from '@codemirror/search'
import { Compartment, EditorState, type Extension, type Text } from '@codemirror/state'
import { oneDarkHighlightStyle } from '@codemirror/theme-one-dark'
import {
  crosshairCursor,
  drawSelection,
  dropCursor,
  EditorView,
  highlightActiveLine,
  highlightActiveLineGutter,
  highlightSpecialChars,
  keymap,
  lineNumbers,
  rectangularSelection,
} from '@codemirror/view'
import { useEffect, useImperativeHandle, useRef, type Ref } from 'react'
import { languageFor } from './languages'

export interface CodeEditorHandle {
  getContent: () => string
  /** Replaces the whole document (e.g. after reloading from disk) and marks it saved. */
  setContent: (content: string) => void
  /** Records the current document as the saved version. */
  markSaved: () => void
  focus: () => void
  find: () => void
}

export interface CursorInfo {
  line: number
  col: number
  selected: number
}

const appTheme = EditorView.theme({
  '&': { height: '100%', fontSize: '13px', backgroundColor: 'var(--background)', color: 'var(--foreground)' },
  '.cm-scroller': { fontFamily: 'var(--font-mono)', lineHeight: '1.55' },
  '.cm-content': { caretColor: 'var(--primary)' },
  '.cm-gutters': { backgroundColor: 'var(--panel)', color: 'var(--muted-foreground)', borderRight: '1px solid var(--border)' },
  '.cm-activeLineGutter': { backgroundColor: 'var(--row-hover)', color: 'var(--foreground)' },
  '.cm-activeLine': { backgroundColor: 'color-mix(in oklch, var(--row-hover) 55%, transparent)' },
  '&.cm-focused .cm-cursor': { borderLeftColor: 'var(--primary)', borderLeftWidth: '2px' },
  '&.cm-focused > .cm-scroller > .cm-selectionLayer .cm-selectionBackground, .cm-selectionBackground': {
    backgroundColor: 'var(--row-selected)',
  },
  '.cm-selectionMatch': { backgroundColor: 'color-mix(in oklch, var(--primary) 18%, transparent)' },
  '.cm-searchMatch': { backgroundColor: 'color-mix(in oklch, var(--warning) 30%, transparent)', outline: '1px solid color-mix(in oklch, var(--warning) 60%, transparent)' },
  '.cm-searchMatch.cm-searchMatch-selected': { backgroundColor: 'color-mix(in oklch, var(--warning) 55%, transparent)' },
  '&.cm-focused .cm-matchingBracket': { backgroundColor: 'color-mix(in oklch, var(--primary) 25%, transparent)', outline: 'none' },
  '.cm-foldPlaceholder': { backgroundColor: 'var(--muted)', border: '1px solid var(--border)', color: 'var(--muted-foreground)' },
  '.cm-tooltip': { backgroundColor: 'var(--popover)', color: 'var(--popover-foreground)', border: '1px solid var(--border)', borderRadius: '6px' },
  '.cm-panels': { backgroundColor: 'var(--panel)', color: 'var(--foreground)', fontFamily: 'var(--font-sans)', fontSize: '12px' },
  '.cm-panels.cm-panels-top': { borderBottom: '1px solid var(--border)' },
  '.cm-panels.cm-panels-bottom': { borderTop: '1px solid var(--border)' },
  '.cm-panel.cm-search': { padding: '6px 8px', display: 'flex', flexWrap: 'wrap', alignItems: 'center', gap: '4px' },
  '.cm-panel.cm-search input.cm-textfield': {
    backgroundColor: 'var(--background)',
    color: 'var(--foreground)',
    border: '1px solid var(--input)',
    borderRadius: '5px',
    padding: '3px 6px',
    fontSize: '12px',
    outline: 'none',
  },
  '.cm-panel.cm-search input.cm-textfield:focus': { borderColor: 'var(--ring)' },
  '.cm-panel.cm-search .cm-button': {
    backgroundImage: 'none',
    backgroundColor: 'var(--secondary)',
    color: 'var(--secondary-foreground)',
    border: '1px solid var(--border)',
    borderRadius: '5px',
    padding: '2px 8px',
    fontSize: '12px',
  },
  '.cm-panel.cm-search label': { display: 'inline-flex', alignItems: 'center', gap: '3px', fontSize: '12px', color: 'var(--muted-foreground)' },
  '.cm-panel.cm-search [name=close]': { color: 'var(--muted-foreground)', fontSize: '16px', right: '8px' },
})

function sameDoc(a: Text, b: Text): boolean {
  return a.length === b.length && a.eq(b)
}

export default function CodeEditor({
  path,
  content,
  dark,
  wrap,
  active,
  onDirtyChange,
  onSave,
  onCursor,
  onLanguage,
  ref,
}: {
  path: string
  content: string
  dark: boolean
  wrap: boolean
  active: boolean
  onDirtyChange: (dirty: boolean) => void
  onSave: () => void
  onCursor: (c: CursorInfo) => void
  onLanguage: (label: string, eol: 'LF' | 'CRLF') => void
  ref?: Ref<CodeEditorHandle>
}) {
  const host = useRef<HTMLDivElement>(null)
  const view = useRef<EditorView | null>(null)
  const saved = useRef<Text | null>(null)
  const dirty = useRef(false)
  const comps = useRef({ language: new Compartment(), theme: new Compartment(), wrap: new Compartment() })
  // Latest callbacks, so the editor (created once) never calls stale ones.
  const cb = useRef({ onDirtyChange, onSave, onCursor })
  cb.current = { onDirtyChange, onSave, onCursor }
  const crlf = useRef(content.includes('\r\n'))

  const reportCursor = (state: EditorState) => {
    const sel = state.selection.main
    const line = state.doc.lineAt(sel.head)
    cb.current.onCursor({ line: line.number, col: sel.head - line.from + 1, selected: Math.abs(sel.to - sel.from) })
  }

  const setDirty = (d: boolean) => {
    if (dirty.current !== d) {
      dirty.current = d
      cb.current.onDirtyChange(d)
    }
  }

  useEffect(() => {
    const c = comps.current
    const extensions: Extension[] = [
      lineNumbers(),
      highlightActiveLineGutter(),
      highlightSpecialChars(),
      history(),
      foldGutter(),
      drawSelection(),
      dropCursor(),
      EditorState.allowMultipleSelections.of(true),
      indentOnInput(),
      indentUnit.of(detectIndent(content)),
      bracketMatching(),
      closeBrackets(),
      rectangularSelection(),
      crosshairCursor(),
      highlightActiveLine(),
      highlightSelectionMatches(),
      keymap.of([
        {
          key: 'Mod-s',
          preventDefault: true,
          run: () => {
            cb.current.onSave()
            return true
          },
        },
        ...closeBracketsKeymap,
        ...defaultKeymap,
        ...searchKeymap,
        ...historyKeymap,
        ...foldKeymap,
        indentWithTab,
      ]),
      appTheme,
      c.theme.of(syntaxHighlighting(dark ? oneDarkHighlightStyle : defaultHighlightStyle, { fallback: true })),
      c.wrap.of(wrap ? EditorView.lineWrapping : []),
      c.language.of([]),
      // Keep Windows line endings as they are when saving.
      crlf.current ? EditorState.lineSeparator.of('\r\n') : [],
      EditorView.updateListener.of((u) => {
        if (u.docChanged && saved.current) setDirty(!sameDoc(u.state.doc, saved.current))
        if (u.docChanged || u.selectionSet) reportCursor(u.state)
      }),
    ]
    const v = new EditorView({ parent: host.current!, state: EditorState.create({ doc: content, extensions }) })
    view.current = v
    saved.current = v.state.doc
    reportCursor(v.state)

    let cancelled = false
    void languageFor(path).then(({ label, ext }) => {
      if (cancelled) return
      onLanguage(label, crlf.current ? 'CRLF' : 'LF')
      if (ext) v.dispatch({ effects: c.language.reconfigure(ext) })
    })
    return () => {
      cancelled = true
      v.destroy()
      view.current = null
    }
    // Created once per open file; content changes go through the handle.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  useEffect(() => {
    view.current?.dispatch({
      effects: comps.current.theme.reconfigure(syntaxHighlighting(dark ? oneDarkHighlightStyle : defaultHighlightStyle, { fallback: true })),
    })
  }, [dark])

  useEffect(() => {
    view.current?.dispatch({ effects: comps.current.wrap.reconfigure(wrap ? EditorView.lineWrapping : []) })
  }, [wrap])

  useEffect(() => {
    if (!active || !view.current) return
    view.current.focus()
    reportCursor(view.current.state)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [active])

  useImperativeHandle(ref, () => ({
    getContent: () => view.current?.state.sliceDoc() ?? '',
    setContent: (next: string) => {
      const v = view.current
      if (!v) return
      v.dispatch({ changes: { from: 0, to: v.state.doc.length, insert: next } })
      saved.current = v.state.doc
      setDirty(false)
    },
    markSaved: () => {
      if (!view.current) return
      saved.current = view.current.state.doc
      setDirty(false)
    },
    focus: () => view.current?.focus(),
    find: () => view.current && openSearchPanel(view.current),
  }))

  return <div ref={host} className={active ? 'h-full min-h-0' : 'hidden'} />
}

/** Guesses the file's indentation from its first indented lines. */
function detectIndent(text: string): string {
  const lines = text.split('\n', 400)
  let tabs = 0
  const widths: Record<number, number> = {}
  let prev = 0
  for (const line of lines) {
    if (line.startsWith('\t')) {
      tabs++
      continue
    }
    const m = /^( +)\S/.exec(line)
    const n = m ? m[1].length : 0
    const d = Math.abs(n - prev)
    if (d > 0 && d <= 8) widths[d] = (widths[d] ?? 0) + 1
    if (m || line.trim()) prev = n
  }
  const spaces = Object.values(widths).reduce((a, b) => a + b, 0)
  if (tabs > spaces) return '\t'
  const best = Object.entries(widths).sort((a, b) => b[1] - a[1])[0]
  return ' '.repeat(best ? Number(best[0]) : 2)
}
