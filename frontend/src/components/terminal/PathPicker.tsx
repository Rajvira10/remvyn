import { ArrowUpIcon, CheckIcon, FileIcon, FolderIcon, Loader2Icon, SearchIcon } from 'lucide-react'
import { useEffect, useMemo, useRef, useState } from 'react'
import { Kbd } from '@/components/ui/kbd'
import { api, errorMessage } from '@/lib/api'
import type { FileIndex } from '@/lib/types'
import { cn, isMac } from '@/lib/utils'
import { matchPositions, pathScore, shellQuote } from './pathMatch'

/** Rows shown for a search; the index can hold tens of thousands. */
const MAX_RESULTS = 200

export interface PickerStart {
  /** The shell's working directory; inserted paths are relative to it. */
  cwd: string
  home: string
}

interface Item {
  /** Relative to the folder being browsed, using "/". */
  rel: string
  abs: string
  dir: boolean
  pruned: boolean
}

const sepOf = (p: string) => (p.includes('\\') && !p.includes('/') ? '\\' : '/')

function join(root: string, rel: string): string {
  const sep = sepOf(root)
  return (root.endsWith(sep) ? root : root + sep) + (sep === '/' ? rel : rel.replaceAll('/', sep))
}

function parent(p: string): string {
  const i = Math.max(p.lastIndexOf('/'), p.lastIndexOf('\\'))
  if (i < 0) return p
  if (i === 0) return p.slice(0, 1)
  // C:\ stays a drive root.
  if (i === 2 && p[1] === ':') return p.slice(0, 3)
  return p.slice(0, i)
}

/** p relative to dir, or undefined when p is not inside dir. */
function inside(p: string, dir: string): string | undefined {
  if (!dir) return undefined
  if (p === dir) return ''
  const sep = sepOf(dir)
  const base = dir.endsWith(sep) ? dir : dir + sep
  return p.startsWith(base) ? p.slice(base.length) : undefined
}

function tilde(p: string, home: string): string {
  const rel = inside(p, home)
  return rel === undefined || sepOf(home) !== '/' ? p : rel ? `~/${rel}` : '~'
}

/**
 * Floating file finder over a terminal: type to fuzzy-search the tree under
 * the shell's directory and insert the chosen paths at the cursor. Lists come
 * from the computer the shell runs on, through the session's connection.
 */
export function PathPicker({ sessionId, start, powershell, onInsert, onClose }: {
  sessionId: string
  start: PickerStart
  powershell: boolean
  onInsert: (text: string) => void
  onClose: () => void
}) {
  const [index, setIndex] = useState<FileIndex | null>(null)
  const [base, setBase] = useState('')
  const [loading, setLoading] = useState('')
  const [error, setError] = useState('')
  const [query, setQuery] = useState('')
  const [sel, setSel] = useState(0)
  const [marked, setMarked] = useState<string[]>([])
  const inputRef = useRef<HTMLInputElement>(null)
  const listRef = useRef<HTMLDivElement>(null)
  const loadSeq = useRef(0)
  // Highlights the folder just left after going up a level.
  const reselect = useRef('')

  const load = (dir: string, opts: { fallback?: string; select?: string } = {}) => {
    const seq = ++loadSeq.current
    setLoading(dir)
    setError('')
    api
      .findFiles(sessionId, dir)
      .then((idx) => {
        if (seq !== loadSeq.current) return
        reselect.current = opts.select ?? ''
        setIndex(idx)
        setBase(idx.dir)
        setQuery('')
        setSel(0)
        setLoading('')
      })
      .catch((err) => {
        if (seq !== loadSeq.current) return
        if (opts.fallback && opts.fallback !== dir) return load(opts.fallback)
        setLoading('')
        setError(errorMessage(err))
      })
  }

  useEffect(() => {
    load(start.cwd || start.home, { fallback: start.home })
    inputRef.current?.focus()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const all = useMemo<{ path: string; dir: boolean; pruned: boolean }[]>(() => {
    if (!index) return []
    return [
      ...index.dirs.map((path) => ({ path, dir: true, pruned: false })),
      ...index.pruned.map((path) => ({ path, dir: true, pruned: true })),
      ...index.files.map((path) => ({ path, dir: false, pruned: false })),
    ]
  }, [index])

  const items = useMemo<Item[]>(() => {
    if (!index) return []
    const sub = inside(base, index.dir) ?? ''
    const prefix = sub ? sub.replaceAll('\\', '/') + '/' : ''
    const under = all
      .filter((e) => e.path.startsWith(prefix))
      .map((e) => ({ rel: e.path.slice(prefix.length), abs: join(index.dir, e.path), dir: e.dir, pruned: e.pruned }))
    const q = query.trim()
    if (!q) {
      const hidden = (e: Item) => (e.rel.startsWith('.') ? 1 : 0)
      return under
        .filter((e) => !e.rel.includes('/'))
        .sort((a, b) => Number(b.dir) - Number(a.dir) || hidden(a) - hidden(b) || a.rel.localeCompare(b.rel))
    }
    return under
      .map((e) => ({ e, s: pathScore(q, e.rel) }))
      .filter((x) => x.s >= 0)
      .sort((a, b) => b.s - a.s)
      .slice(0, MAX_RESULTS)
      .map((x) => x.e)
  }, [all, index, base, query])

  // After going up, highlight the folder we came from.
  useEffect(() => {
    if (!reselect.current) return
    const i = items.findIndex((e) => e.abs === reselect.current)
    reselect.current = ''
    if (i >= 0) setSel(i)
  }, [items])

  useEffect(() => {
    listRef.current?.querySelector(`[data-row="${sel}"]`)?.scrollIntoView({ block: 'nearest' })
  }, [sel])

  const open = (item: Item) => {
    if (item.pruned || index?.truncated) return load(item.abs)
    setBase(item.abs)
    setQuery('')
    setSel(0)
  }

  const up = () => {
    if (!index) return
    const to = parent(base)
    if (to === base) return
    if (inside(to, index.dir) === undefined) return load(to, { select: base })
    reselect.current = base
    setBase(to)
    setQuery('')
    setSel(0)
  }

  const format = (abs: string, absolute: boolean) => {
    let p = abs
    if (!absolute) {
      const rel = inside(abs, start.cwd)
      if (rel !== undefined) p = rel === '' ? '.' : rel.startsWith('-') ? `./${rel}` : rel
      else if (!powershell) p = tilde(abs, start.home)
    }
    return shellQuote(p, powershell)
  }

  const insert = (absolute: boolean, only?: Item) => {
    const chosen = only ? [only.abs] : marked.length ? marked : items[sel] ? [items[sel].abs] : []
    if (!chosen.length) return
    onInsert(chosen.map((p) => format(p, absolute)).join(' '))
  }

  const toggleMark = (item: Item) =>
    setMarked((m) => (m.includes(item.abs) ? m.filter((x) => x !== item.abs) : [...m, item.abs]))

  // A typed absolute or ~ path is a place to go.
  const typedPath = /^(~(\/|$)|\/)/.test(query.trim()) ? query.trim().replace(/^~(?=\/|$)/, start.home) : ''

  const onKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
    const atEnd = e.currentTarget.selectionStart === query.length && e.currentTarget.selectionEnd === query.length
    const item = items[sel]
    if (e.key === 'Escape') {
      e.preventDefault()
      onClose()
    } else if (e.key === 'ArrowDown' || e.key === 'ArrowUp' || e.key === 'PageDown' || e.key === 'PageUp') {
      e.preventDefault()
      const step = e.key.startsWith('Page') ? 10 : 1
      const dir = e.key === 'ArrowDown' || e.key === 'PageDown' ? step : -step
      setSel((i) => Math.max(0, Math.min(items.length - 1, i + dir)))
    } else if (e.key === 'Enter') {
      e.preventDefault()
      if (typedPath) load(typedPath)
      else insert(e.shiftKey || e.altKey)
    } else if (e.key === 'Tab' && !e.shiftKey) {
      e.preventDefault()
      if (!item) return
      toggleMark(item)
      setSel((i) => Math.min(items.length - 1, i + 1))
    } else if (e.key === 'ArrowRight' && atEnd && item?.dir) {
      e.preventDefault()
      open(item)
    } else if ((e.key === 'ArrowLeft' || e.key === 'Backspace') && query === '') {
      e.preventDefault()
      up()
    }
  }

  const shown = base ? tilde(base, start.home) : ''
  const q = query.trim()

  return (
    <>
      <div className="absolute inset-0 z-20" onMouseDown={onClose} />
      <div
        role="dialog"
        aria-label="Insert a path"
        className="absolute top-3 left-1/2 z-30 flex max-h-[min(60%,480px)] w-[min(640px,calc(100%-24px))] -translate-x-1/2 flex-col overflow-hidden rounded-xl border bg-popover text-popover-foreground shadow-2xl"
      >
        <div className="flex items-center gap-2 border-b px-3">
          <SearchIcon className="size-4 shrink-0 text-muted-foreground" />
          <input
            ref={inputRef}
            value={query}
            onChange={(e) => {
              setQuery(e.target.value)
              setSel(0)
            }}
            onKeyDown={onKeyDown}
            placeholder="Find a file or folder…"
            spellCheck={false}
            autoCapitalize="off"
            autoCorrect="off"
            className="h-11 min-w-0 flex-1 bg-transparent text-[13px] outline-none"
          />
          {marked.length > 0 && <span className="shrink-0 text-[11px] text-muted-foreground">{marked.length} selected</span>}
        </div>
        <div className="flex h-8 shrink-0 items-center gap-1.5 border-b bg-muted/30 px-2 text-[11.5px]">
          <button
            type="button"
            className="flex size-5 items-center justify-center rounded text-muted-foreground hover:bg-accent hover:text-foreground disabled:opacity-40"
            onClick={() => {
              up()
              inputRef.current?.focus()
            }}
            disabled={!base || parent(base) === base || !!loading}
            aria-label="Up one folder"
          >
            <ArrowUpIcon className="size-3.5" />
          </button>
          <span className="min-w-0 truncate font-mono text-muted-foreground" title={base}>{shown}</span>
          {index?.truncated && !loading && (
            <span className="ml-auto shrink-0 text-warning" title="Very large folder: open a subfolder to search all of it">
              partial list
            </span>
          )}
        </div>
        <div ref={listRef} className="min-h-0 flex-1 overflow-y-auto p-1">
          {loading ? (
            <div className="flex items-center gap-2 px-3 py-6 text-[12.5px] text-muted-foreground">
              <Loader2Icon className="size-3.5 animate-spin" /> Listing {tilde(loading, start.home)}…
            </div>
          ) : error ? (
            <div className="px-3 py-6 text-[12.5px] text-danger">{error}</div>
          ) : typedPath ? (
            <div className="px-3 py-6 text-[12.5px] text-muted-foreground">
              Press <Kbd>↵</Kbd> to go to <span className="font-mono text-foreground">{query.trim()}</span>
            </div>
          ) : items.length === 0 ? (
            <div className="px-3 py-6 text-[12.5px] text-muted-foreground">{q ? 'Nothing matches.' : 'This folder is empty.'}</div>
          ) : (
            items.map((item, i) => {
              const slash = item.rel.lastIndexOf('/')
              const hits = q ? matchPositions(q, item.rel) : null
              const isMarked = marked.includes(item.abs)
              return (
                <div
                  key={item.abs}
                  data-row={i}
                  onMouseMove={() => sel !== i && setSel(i)}
                  onMouseDown={(e) => e.preventDefault()}
                  onClick={(e) => {
                    if (e.metaKey || e.ctrlKey) toggleMark(item)
                    else if (item.dir) open(item)
                    else insert(e.shiftKey || e.altKey, item)
                  }}
                  className={cn('flex h-7 cursor-default items-center gap-2 rounded-md px-2 text-[12.5px]', i === sel && 'bg-accent')}
                >
                  <span className={cn('flex size-3.5 shrink-0 items-center justify-center rounded-sm border', isMarked ? 'border-primary bg-primary text-primary-foreground' : 'border-transparent')}>
                    {isMarked && <CheckIcon className="size-3" />}
                  </span>
                  {item.dir ? <FolderIcon className="size-3.5 shrink-0 text-primary" /> : <FileIcon className="size-3.5 shrink-0 text-muted-foreground" />}
                  <span className="min-w-0 truncate font-mono">
                    {[...item.rel].map((ch, j) => (
                      <span key={j} className={cn(j <= slash && 'text-muted-foreground', hits?.has(j) && 'font-semibold text-primary')}>
                        {ch}
                      </span>
                    ))}
                    {item.dir && <span className="text-muted-foreground">/</span>}
                  </span>
                  {item.pruned && <span className="ml-auto shrink-0 text-[10.5px] text-muted-foreground">not searched · open to search</span>}
                </div>
              )
            })
          )}
        </div>
        <div className="flex shrink-0 flex-wrap items-center gap-x-3 gap-y-1 border-t px-3 py-1.5 text-[10.5px] text-muted-foreground">
          <span><Kbd>↵</Kbd> insert</span>
          <span><Kbd>⇥</Kbd> select several</span>
          <span><Kbd>→</Kbd> open folder</span>
          <span><Kbd>←</Kbd> up</span>
          <span><Kbd>{isMac ? '⇧↵' : 'Shift+↵'}</Kbd> full path</span>
          <span><Kbd>esc</Kbd> close</span>
        </div>
      </div>
    </>
  )
}
