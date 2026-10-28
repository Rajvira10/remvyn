import type { IDecoration, IDisposable, IMarker, Terminal } from '@xterm/xterm'

/**
 * Fish-style inline suggestions: the rest of a past command is drawn in faint
 * text after the cursor, and → or End accepts it. Ctrl+Space opens a list of
 * more matches, filtered as you keep typing.
 *
 * Typing a path (/var/www/, ~/, src/co) lists what's in that folder under the
 * cursor, narrowing as you type: ↑↓ choose, Tab inserts, Esc hides it.
 *
 * The remote shell owns the command line, so it is read back from the screen:
 * the text between where the first key of a line was typed (the end of the
 * prompt) and the cursor. That follows the shell's own editing, Tab completion
 * and ↑ history, and never sees passwords, which are not echoed. Whenever the
 * line can't be read reliably, nothing is suggested or recorded:
 * - full-screen programs (vim, less, tmux) on the alternate screen
 * - text after the cursor (editing mid-line, or the shell's own suggestion)
 * - keys typed while output is still arriving (typeahead)
 * - after a resize or Ctrl+L, until the next Enter
 */

// Rows in the suggestion list.
const LIST_SIZE = 8
// Rows shown at once in the path list; it scrolls with the selection.
const PATH_ROWS = 10

// Output must have been quiet this long when the first key of a line is typed.
// Short, so fast typing right after a prompt still counts.
const QUIET_MS = 30

// Replies the terminal sends on its own (device attributes, cursor position,
// focus, OSC answers) and mouse reports: not typing.
// eslint-disable-next-line no-control-regex
const REPORT = /^\x1b(\[[?>]?[\d;]*[cnRIO]|\[<|\[M|\]|P)/

export interface GhostOptions {
  /** A past command that starts with typed, if any. */
  suggest: (typed: string) => string | undefined
  /** Past commands for the list, best first. */
  list: (typed: string, limit: number) => string[]
  /** Sends text to the shell as if typed. */
  send: (text: string) => void
  record: (cmd: string) => void
  forget: (cmd: string) => void
  /**
   * A folder's entries, folders ending in /, or undefined while it loads
   * (call refresh() when it arrives). dir is unquoted and as typed:
   * absolute, ~/…, or relative to the shell's directory.
   */
  listDir?: (dir: string) => string[] | undefined
}

/** The path being typed at the cursor. */
interface PathWord {
  /** As typed, quotes and backslashes included. */
  raw: string
  /** The folder part, unquoted, ending in /. */
  dir: string
  /** The name being typed, unquoted, and as typed. */
  prefix: string
  rawPrefix: string
  quoted: boolean
}

export class GhostSuggest {
  enabled = true

  private anchor: { marker: IMarker; x: number } | null = null
  private fresh = true
  private lastOutput = 0
  private ghost = ''
  private full = ''
  private shownAt = ''
  private deco: IDecoration | null = null
  private menu: { items: string[]; sel: number; typed: string; el: HTMLDivElement } | null = null
  private paths: { items: string[]; sel: number; word: PathWord; el: HTMLDivElement } | null = null
  // The path whose list Esc hid; it stays hidden until the path changes.
  private dismissed = ''
  private raf = 0
  private subs: IDisposable[]

  constructor(
    private term: Terminal,
    private opts: GhostOptions,
  ) {
    this.subs = [
      term.onWriteParsed(() => {
        this.lastOutput = performance.now()
        this.schedule()
      }),
      term.onCursorMove(() => this.schedule()),
      // Reflow moves the line; a prompt not typed at yet is still fine.
      term.onResize(() => (this.anchor ? this.reset(false) : this.show('', ''))),
      // Leaving a full-screen program returns to a prompt.
      term.buffer.onBufferChange((b) => this.reset(b.type === 'normal')),
      term.onScroll(() => this.schedule()),
    ]
    const blur = () => {
      this.closeMenu()
      this.closePaths()
    }
    term.textarea?.addEventListener('blur', blur)
    this.subs.push({ dispose: () => term.textarea?.removeEventListener('blur', blur) })
  }

  /** Starts over, e.g. for a new connection; fresh means a prompt is expected. */
  reset(fresh: boolean) {
    this.anchor?.marker.dispose()
    this.anchor = null
    this.fresh = fresh
    this.closeMenu()
    this.closePaths()
    this.show('', '')
  }

  /** Redraws, e.g. once a folder listing arrives. */
  refresh() {
    this.schedule()
  }

  /** Call with every piece of user input, before it is sent. */
  input(data: string) {
    if (!this.enabled || REPORT.test(data)) return
    if (this.term.buffer.active.type === 'alternate') return this.reset(false)
    if (data === '\r') {
      const line = this.read()
      if (line && !line.tail) this.maybeRecord(line.text)
      return this.reset(true)
    }
    // Ctrl+C and Ctrl+D end the line; pasted lines run straight away.
    if (data === '\x03' || data === '\x04' || /[\r\n]/.test(data)) return this.reset(true)
    // Ctrl+L redraws the prompt somewhere else.
    if (data === '\x0c') return this.reset(false)
    // At column 0 there is no prompt yet: typing ahead while a command runs.
    if (this.fresh) this.startLine(performance.now() - this.lastOutput >= QUIET_MS && this.term.buffer.active.cursorX > 0)
    this.schedule()
  }

  /** The first key of a command line: the prompt ends at the cursor. */
  private startLine(trusted: boolean) {
    this.fresh = false
    if (trusted) this.anchor = { marker: this.term.registerMarker(0), x: this.term.buffer.active.cursorX }
  }

  /** Handles accept/forget keys; true means the key was used. */
  handleKey(e: KeyboardEvent): boolean {
    if (!this.enabled || e.type !== 'keydown') return false
    const noMods = !e.ctrlKey && !e.altKey && !e.metaKey
    if (e.ctrlKey && !e.altKey && !e.metaKey && !e.shiftKey && e.code === 'Space') {
      if (this.menu) this.closeMenu()
      else this.openMenu()
      e.preventDefault()
      return true
    }
    this.update()
    if (this.menu && this.menuKey(e, noMods)) {
      e.preventDefault()
      return true
    }
    if (this.paths && this.pathKey(e, noMods)) {
      e.preventDefault()
      return true
    }
    if (!this.ghost || e.metaKey) return false
    if (noMods && !e.shiftKey && (e.key === 'ArrowRight' || e.key === 'End')) {
      this.accept(this.ghost)
    } else if ((e.ctrlKey || e.altKey) && !e.shiftKey && e.key === 'ArrowRight') {
      // One word at a time.
      this.accept(/^\s*\S+/.exec(this.ghost)?.[0] ?? this.ghost)
    } else if (noMods && e.shiftKey && e.key === 'Delete') {
      this.opts.forget(this.full)
      this.show('', '')
      this.schedule()
    } else {
      return false
    }
    e.preventDefault()
    return true
  }

  dispose() {
    cancelAnimationFrame(this.raf)
    this.reset(false)
    for (const s of this.subs) s.dispose()
  }

  private openMenu() {
    // Asking for the list at a fresh prompt marks where the line starts.
    if (this.fresh && this.term.buffer.active.type === 'normal') this.startLine(true)
    const line = this.read()
    if (!line || line.tail) return
    const items = this.opts.list(line.text, LIST_SIZE)
    if (!items.length) return
    this.closePaths()
    const el = popup('Command suggestions')
    this.term.element?.appendChild(el)
    this.menu = { items, sel: 0, typed: line.text, el }
    this.update()
  }

  private closeMenu() {
    this.menu?.el.remove()
    this.menu = null
  }

  private menuKey(e: KeyboardEvent, noMods: boolean): boolean {
    const m = this.menu!
    if (noMods && !e.shiftKey && (e.key === 'ArrowDown' || e.key === 'ArrowUp')) {
      m.sel = (m.sel + (e.key === 'ArrowDown' ? 1 : -1) + m.items.length) % m.items.length
      this.update()
    } else if (noMods && !e.shiftKey && (e.key === 'Enter' || e.key === 'Tab' || e.key === 'ArrowRight')) {
      this.insert(m.items[m.sel])
    } else if (e.key === 'Escape') {
      this.closeMenu()
      this.update()
    } else if (noMods && e.shiftKey && e.key === 'Delete') {
      this.opts.forget(m.items[m.sel])
      this.update()
    } else {
      // Typing goes to the shell and the list follows it.
      return false
    }
    return true
  }

  private pathKey(e: KeyboardEvent, noMods: boolean): boolean {
    const p = this.paths!
    if (noMods && !e.shiftKey && (e.key === 'ArrowDown' || e.key === 'ArrowUp')) {
      p.sel = (p.sel + (e.key === 'ArrowDown' ? 1 : -1) + p.items.length) % p.items.length
      this.placePaths(p.word, p.items)
    } else if (noMods && !e.shiftKey && e.key === 'Tab') {
      this.insertPath(p.word, p.items[p.sel])
    } else if (e.key === 'Escape' && !e.ctrlKey && !e.altKey && !e.metaKey) {
      this.dismissed = p.word.raw
      this.closePaths()
      this.update()
    } else {
      return false
    }
    return true
  }

  private closePaths() {
    this.paths?.el.remove()
    this.paths = null
  }

  /** Completes the typed name to name, escaped to match how the path is typed. */
  private insertPath(word: PathWord, name: string) {
    const text = word.quoted ? name : name.replace(/[^\w@%+=:,./~-]/g, '\\$&')
    const raw = word.rawPrefix
    this.closePaths()
    this.accept(text.startsWith(raw) ? text.slice(raw.length) : '\x7f'.repeat([...raw].length) + text)
  }

  /** Entries of the typed folder that start with the typed name. */
  private pathItems(word: PathWord): string[] {
    const names = this.opts.listDir?.(word.dir)
    if (!names) return []
    // Hidden entries only once a name starting with "." is typed.
    const shown = (n: string) => word.prefix.startsWith('.') || !n.startsWith('.')
    let items = names.filter((n) => shown(n) && n.startsWith(word.prefix))
    if (!items.length && word.prefix) {
      const lower = word.prefix.toLowerCase()
      items = names.filter((n) => shown(n) && n.toLowerCase().startsWith(lower))
    }
    // Already complete.
    if (items.length === 1 && items[0] === word.prefix) return []
    return items
  }

  /** Draws the path list under (or above) the cursor; false when the cursor is out of view. */
  private placePaths(word: PathWord, items: string[]): boolean {
    const buf = this.term.buffer.active
    const screen = this.term.element?.querySelector<HTMLElement>('.xterm-screen')
    const row = buf.cursorY + buf.baseY - buf.viewportY
    if (!screen || row < 0 || row >= this.term.rows) return false
    if (!this.paths) {
      const el = popup('Folder contents')
      this.term.element?.appendChild(el)
      this.paths = { items, sel: 0, word, el }
    }
    const p = this.paths
    const prev = p.word.dir === word.dir ? p.items[p.sel] : undefined
    if (p.items !== items) p.sel = Math.max(0, prev ? items.indexOf(prev) : 0)
    p.items = items
    p.word = word

    const o = this.term.options
    const first = Math.max(0, Math.min(p.sel - Math.floor(PATH_ROWS / 2), items.length - PATH_ROWS))
    const rows = items.slice(first, first + PATH_ROWS).map((name, j) => {
      const i = first + j
      const item = document.createElement('div')
      item.setAttribute('role', 'option')
      item.setAttribute('aria-selected', String(i === p.sel))
      Object.assign(item.style, {
        padding: '2px 8px',
        borderRadius: 'calc(var(--radius) - 4px)',
        whiteSpace: 'pre',
        overflow: 'hidden',
        textOverflow: 'ellipsis',
        cursor: 'default',
        font: `${o.fontSize ?? 13}px ${o.fontFamily ?? 'monospace'}`,
        background: i === p.sel ? 'var(--accent)' : '',
        color: i === p.sel ? 'var(--accent-foreground)' : name.endsWith('/') ? 'var(--primary)' : '',
      } satisfies Partial<CSSStyleDeclaration>)
      const n = word.prefix.length
      const b = document.createElement('b')
      b.textContent = name.slice(0, n)
      item.append(b, name.slice(n))
      item.addEventListener('mousedown', (e) => {
        e.preventDefault()
        this.insertPath(p.word, name)
      })
      item.addEventListener('mousemove', () => {
        if (p.sel !== i) {
          p.sel = i
          this.placePaths(p.word, p.items)
        }
      })
      return item
    })
    const more = items.length > PATH_ROWS ? ` · ${p.sel + 1} of ${items.length}` : ''
    p.el.replaceChildren(...rows, hint(`↑↓ choose · Tab insert · Esc hide${more}`))

    const cellW = screen.clientWidth / this.term.cols
    const cellH = screen.clientHeight / this.term.rows
    const startCol = Math.max(0, buf.cursorX - [...word.rawPrefix].length)
    const left = Math.min(startCol * cellW, Math.max(0, screen.clientWidth - p.el.offsetWidth))
    const below = (row + 1) * cellH
    const top = below + p.el.offsetHeight <= screen.clientHeight ? below : Math.max(0, row * cellH - p.el.offsetHeight)
    p.el.style.left = `${left}px`
    p.el.style.top = `${top}px`
    return true
  }

  /** Replaces the typed text with cmd. */
  private insert(cmd: string) {
    const typed = this.menu?.typed ?? ''
    this.closeMenu()
    this.accept(cmd.startsWith(typed) ? cmd.slice(typed.length) : '\x7f'.repeat([...typed].length) + cmd)
  }
  private accept(text: string) {
    this.opts.send(text)
    this.show('', '')
  }

  private maybeRecord(line: string) {
    const cmd = line.trimEnd()
    // A leading space keeps a command out of history, as in bash and zsh;
    // a line of asterisks is a masked password.
    if (cmd.length < 2 || /^\s/.test(cmd) || /^\*+$/.test(cmd)) return
    this.opts.record(cmd)
  }

  private schedule() {
    if (!this.raf) this.raf = requestAnimationFrame(() => this.update())
  }

  /** The command line from the anchor to the cursor, and whether text follows the cursor. */
  private read(): { text: string; tail: boolean } | null {
    const a = this.anchor
    const buf = this.term.buffer.active
    if (!a || a.marker.isDisposed || buf.type !== 'normal') return null
    const row = buf.baseY + buf.cursorY
    if (row < a.marker.line || (row === a.marker.line && buf.cursorX < a.x)) return null
    let text = ''
    for (let r = a.marker.line; r <= row; r++) {
      const line = buf.getLine(r)
      // A real line break: not a single command line.
      if (!line || (r > a.marker.line && !line.isWrapped)) return null
      text += line.translateToString(false, r === a.marker.line ? a.x : 0, r === row ? buf.cursorX : this.term.cols)
    }
    let after = buf.getLine(row)?.translateToString(true, buf.cursorX) ?? ''
    for (let r = row + 1; !after.trim(); r++) {
      const line = buf.getLine(r)
      if (!line?.isWrapped) break
      after += line.translateToString(true)
    }
    return { text, tail: after.trim() !== '' }
  }

  private update() {
    cancelAnimationFrame(this.raf)
    this.raf = 0
    const line = this.enabled ? this.read() : null
    if (this.menu) {
      const items = line && !line.tail ? this.opts.list(line.text, LIST_SIZE) : []
      if (line && items.length && this.placeMenu(line.text, items)) {
        const sel = this.menu.items[this.menu.sel]
        this.show(sel.startsWith(line.text) ? sel.slice(line.text.length) : '', sel)
        return
      }
      this.closeMenu()
    }
    const word = line && !line.tail && this.opts.listDir ? pathWord(line.text) : null
    if (word?.raw !== this.dismissed) this.dismissed = ''
    const items = word && !this.dismissed ? this.pathItems(word) : []
    if (word && items.length && this.placePaths(word, items)) {
      this.show('', '')
      return
    }
    this.closePaths()
    const full = line && !line.tail ? this.opts.suggest(line.text) : undefined
    this.show(full && line ? full.slice(line.text.length) : '', full ?? '')
  }

  /** Draws the list under (or above) the cursor; false when the cursor is scrolled out of view. */
  private placeMenu(typed: string, items: string[]): boolean {
    const m = this.menu!
    const prev = m.items[m.sel]
    m.items = items
    m.sel = Math.max(0, items.indexOf(prev))
    m.typed = typed

    const buf = this.term.buffer.active
    const screen = this.term.element?.querySelector<HTMLElement>('.xterm-screen')
    const row = buf.cursorY + buf.baseY - buf.viewportY
    if (!screen || row < 0 || row >= this.term.rows) return false

    const o = this.term.options
    const needle = typed.trim()
    m.el.replaceChildren(
      ...items.map((cmd, i) => {
        const item = document.createElement('div')
        item.setAttribute('role', 'option')
        item.setAttribute('aria-selected', String(i === m.sel))
        Object.assign(item.style, {
          padding: '2px 8px',
          borderRadius: 'calc(var(--radius) - 4px)',
          whiteSpace: 'pre',
          overflow: 'hidden',
          textOverflow: 'ellipsis',
          cursor: 'default',
          font: `${o.fontSize ?? 13}px ${o.fontFamily ?? 'monospace'}`,
          background: i === m.sel ? 'var(--accent)' : '',
          color: i === m.sel ? 'var(--accent-foreground)' : '',
        } satisfies Partial<CSSStyleDeclaration>)
        // The typed part in bold.
        const [at, len] = cmd.startsWith(typed) ? [0, typed.length] : [needle ? cmd.indexOf(needle) : -1, needle.length]
        if (at >= 0 && len > 0) {
          const b = document.createElement('b')
          b.textContent = cmd.slice(at, at + len)
          item.append(cmd.slice(0, at), b, cmd.slice(at + len))
        } else {
          item.textContent = cmd
        }
        item.addEventListener('mousedown', (e) => {
          e.preventDefault()
          this.insert(cmd)
        })
        item.addEventListener('mousemove', () => {
          if (m.sel !== i) {
            m.sel = i
            this.update()
          }
        })
        return item
      }),
      hint('↑↓ choose · Enter insert · Shift+Del forget · Esc close'),
    )

    // Line it up with the start of the typed text, inside the terminal.
    const cellW = screen.clientWidth / this.term.cols
    const cellH = screen.clientHeight / this.term.rows
    const startCol = Math.max(0, buf.cursorX - [...typed].length)
    const left = Math.min(startCol * cellW, Math.max(0, screen.clientWidth - m.el.offsetWidth))
    const below = (row + 1) * cellH
    const top = below + m.el.offsetHeight <= screen.clientHeight ? below : Math.max(0, row * cellH - m.el.offsetHeight)
    m.el.style.left = `${left}px`
    m.el.style.top = `${top}px`
    return true
  }

  private show(ghost: string, full: string) {
    const buf = this.term.buffer.active
    const at = `${buf.baseY + buf.cursorY}:${buf.cursorX}`
    if (ghost === this.ghost && (!ghost || at === this.shownAt)) return
    this.deco?.dispose()
    this.deco = null
    this.ghost = ghost
    this.full = full
    this.shownAt = at
    if (!ghost) return

    const width = Math.max(1, this.term.cols - buf.cursorX)
    const marker = this.term.registerMarker(0)
    const deco = this.term.registerDecoration({ marker, x: buf.cursorX, width, layer: 'top' })
    if (!deco) {
      marker.dispose()
      return
    }
    deco.onDispose(() => marker.dispose())
    deco.onRender((el) => {
      const o = this.term.options
      const font = `${o.fontSize ?? 13}px ${o.fontFamily ?? 'monospace'}`
      el.textContent = ghost
      el.style.pointerEvents = 'none'
      el.style.whiteSpace = 'pre'
      el.style.overflow = 'hidden'
      el.style.font = font
      el.style.lineHeight = el.style.height
      el.style.color = o.theme?.foreground ?? '#ccc'
      el.style.opacity = '0.4'
      // Fit each character to a terminal cell.
      el.style.letterSpacing = `${el.clientWidth / width - charWidth(font)}px`
    })
    this.deco = deco
  }
}

function popup(label: string): HTMLDivElement {
  const el = document.createElement('div')
  el.setAttribute('role', 'listbox')
  el.setAttribute('aria-label', label)
  Object.assign(el.style, {
    position: 'absolute',
    zIndex: '10',
    minWidth: '240px',
    maxWidth: '70%',
    padding: '4px',
    borderRadius: 'var(--radius)',
    border: '1px solid var(--border)',
    background: 'var(--popover)',
    color: 'var(--popover-foreground)',
    boxShadow: '0 8px 24px rgb(0 0 0 / 0.35)',
  } satisfies Partial<CSSStyleDeclaration>)
  return el
}

/** The path being typed at the end of line, or null when the last word isn't one. */
export function pathWord(line: string): PathWord | null {
  let start = 0
  let quote = ''
  for (let i = 0; i < line.length; i++) {
    const c = line[i]
    if (quote) {
      if (c === quote) quote = ''
      else if (c === '\\' && quote === '"') i++
    } else if (c === '\\') i++
    else if (c === "'" || c === '"') quote = c
    else if (/\s/.test(c) || ';|&<>()'.includes(c)) start = i + 1
  }
  let raw = line.slice(start)
  // --option=/path and NAME=/path
  const eq = /^[^/'"]*=/.exec(raw)
  if (eq) raw = raw.slice(eq[0].length)
  if (!raw.includes('/') || raw.includes('://')) return null
  const value = unquote(raw)
  const cut = value.lastIndexOf('/') + 1
  return { raw, dir: value.slice(0, cut), prefix: value.slice(cut), rawPrefix: raw.slice(raw.lastIndexOf('/') + 1), quoted: /['"]/.test(raw) }
}

/** Removes shell quoting: quotes and backslash escapes. */
function unquote(s: string): string {
  let out = ''
  let quote = ''
  for (let i = 0; i < s.length; i++) {
    const c = s[i]
    if (quote) {
      if (c === quote) quote = ''
      else if (c === '\\' && quote === '"' && i + 1 < s.length) out += s[++i]
      else out += c
    } else if (c === "'" || c === '"') quote = c
    else if (c === '\\' && i + 1 < s.length) out += s[++i]
    else out += c
  }
  return out
}

function hint(text: string): HTMLElement {
  const el = document.createElement('div')
  el.textContent = text
  Object.assign(el.style, {
    marginTop: '4px',
    padding: '4px 8px 2px',
    borderTop: '1px solid var(--border)',
    fontSize: '11px',
    color: 'var(--muted-foreground)',
    whiteSpace: 'nowrap',
  } satisfies Partial<CSSStyleDeclaration>)
  return el
}

let canvas: HTMLCanvasElement | null = null
const widths = new Map<string, number>()

function charWidth(font: string): number {
  let w = widths.get(font)
  if (w === undefined) {
    canvas ??= document.createElement('canvas')
    const ctx = canvas.getContext('2d')!
    ctx.font = font
    w = ctx.measureText('W').width
    widths.set(font, w)
  }
  return w
}
