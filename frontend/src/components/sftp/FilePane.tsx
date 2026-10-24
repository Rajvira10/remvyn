import {
  ArrowUpIcon,
  ChevronRightIcon,
  EyeIcon,
  EyeOffIcon,
  FileIcon,
  FolderIcon,
  FolderPlusIcon,
  HardDriveIcon,
  HomeIcon,
  Link2Icon,
  PencilIcon,
  RefreshCwIcon,
  ServerIcon,
  Trash2Icon,
} from 'lucide-react'
import { useRef, useState, type ReactNode } from 'react'
import { EmptyState } from '@/components/common/EmptyState'
import { Spinner } from '@/components/common/Spinner'
import { Button } from '@/components/ui/button'
import { Tooltip } from '@/components/ui/tooltip'
import type { DirListing, FileEntry } from '@/lib/types'
import { cn, formatBytes, formatDate } from '@/lib/utils'

export interface PaneProps {
  side: 'local' | 'remote'
  title: string
  listing: DirListing | null
  loading: boolean
  error: string | null
  showHidden: boolean
  selected: Set<string>
  onSelect: (s: Set<string>) => void
  onNavigate: (path: string) => void
  onRefresh: () => void
  onHome: () => void
  onToggleHidden: () => void
  onMkdir: () => void
  onRename: (e: FileEntry) => void
  onDelete: (paths: string[]) => void
  /** Transfer these entries to the other pane. */
  onTransfer: (entries: FileEntry[]) => void
  /** Entries dropped from the other pane. */
  onDrop: (paths: string[]) => void
  transferLabel: string
  transferIcon: ReactNode
}

const DRAG_MIME = 'application/x-remvyn-paths'

export function FilePane(p: PaneProps) {
  const [pathInput, setPathInput] = useState('')
  const [editingPath, setEditingPath] = useState(false)
  const [dragOver, setDragOver] = useState(false)
  const anchor = useRef<string | null>(null)
  const startEditing = () => {
    setPathInput(p.listing?.path ?? '')
    setEditingPath(true)
  }

  const entries = (p.listing?.entries ?? []).filter((e) => p.showHidden || !e.isHidden)
  const selectedEntries = entries.filter((e) => p.selected.has(e.path))

  const click = (e: React.MouseEvent, entry: FileEntry, index: number) => {
    if (e.shiftKey && anchor.current) {
      const a = entries.findIndex((x) => x.path === anchor.current)
      if (a >= 0) {
        const [lo, hi] = a < index ? [a, index] : [index, a]
        p.onSelect(new Set(entries.slice(lo, hi + 1).map((x) => x.path)))
        return
      }
    }
    if (e.metaKey || e.ctrlKey) {
      const next = new Set(p.selected)
      if (next.has(entry.path)) next.delete(entry.path)
      else next.add(entry.path)
      anchor.current = entry.path
      p.onSelect(next)
      return
    }
    anchor.current = entry.path
    p.onSelect(new Set([entry.path]))
  }

  const open = (entry: FileEntry) => {
    if (entry.isDir) p.onNavigate(entry.path)
    else p.onTransfer([entry])
  }

  const crumbs = breadcrumbs(p.listing?.path ?? '', p.side)

  return (
    <div
      className={cn('flex min-w-0 flex-1 flex-col', dragOver && 'bg-primary/5 ring-2 ring-primary/40 ring-inset')}
      onDragOver={(e) => {
        if (e.dataTransfer.types.includes(DRAG_MIME + '-' + (p.side === 'local' ? 'remote' : 'local'))) {
          e.preventDefault()
          setDragOver(true)
        }
      }}
      onDragLeave={() => setDragOver(false)}
      onDrop={(e) => {
        setDragOver(false)
        const raw = e.dataTransfer.getData(DRAG_MIME + '-' + (p.side === 'local' ? 'remote' : 'local'))
        if (raw) {
          e.preventDefault()
          p.onDrop(JSON.parse(raw) as string[])
        }
      }}
    >
      <div className="flex h-9 shrink-0 items-center gap-1 border-b px-2">
        {p.side === 'local' ? <HardDriveIcon className="size-3.5 text-muted-foreground" /> : <ServerIcon className="size-3.5 text-muted-foreground" />}
        <span className="mr-1 text-xs font-medium">{p.title}</span>
        <Tooltip content="Up one level">
          <Button variant="ghost" size="icon-xs" aria-label="Up" disabled={!p.listing || p.listing.parent === p.listing.path} onClick={() => p.listing && p.onNavigate(p.listing.parent)}>
            <ArrowUpIcon />
          </Button>
        </Tooltip>
        <Tooltip content="Home">
          <Button variant="ghost" size="icon-xs" aria-label="Home" onClick={p.onHome}>
            <HomeIcon />
          </Button>
        </Tooltip>
        <Tooltip content="Refresh">
          <Button variant="ghost" size="icon-xs" aria-label="Refresh" onClick={p.onRefresh}>
            <RefreshCwIcon />
          </Button>
        </Tooltip>
        <Tooltip content={p.showHidden ? 'Hide hidden files' : 'Show hidden files'}>
          <Button variant="ghost" size="icon-xs" aria-label="Toggle hidden files" onClick={p.onToggleHidden}>
            {p.showHidden ? <EyeIcon /> : <EyeOffIcon />}
          </Button>
        </Tooltip>
        <div className="mx-1 h-4 w-px bg-border" />
        <Tooltip content="New folder">
          <Button variant="ghost" size="icon-xs" aria-label="New folder" onClick={p.onMkdir} disabled={!p.listing}>
            <FolderPlusIcon />
          </Button>
        </Tooltip>
        <Tooltip content="Rename">
          <Button variant="ghost" size="icon-xs" aria-label="Rename" disabled={selectedEntries.length !== 1} onClick={() => p.onRename(selectedEntries[0])}>
            <PencilIcon />
          </Button>
        </Tooltip>
        <Tooltip content="Delete">
          <Button variant="ghost" size="icon-xs" aria-label="Delete" disabled={selectedEntries.length === 0} onClick={() => p.onDelete(selectedEntries.map((e) => e.path))}>
            <Trash2Icon />
          </Button>
        </Tooltip>
        <Button variant="secondary" size="xs" className="ml-auto" disabled={selectedEntries.length === 0} onClick={() => p.onTransfer(selectedEntries)}>
          {p.transferIcon}
          {p.transferLabel}
          {selectedEntries.length > 1 ? ` (${selectedEntries.length})` : ''}
        </Button>
      </div>
      <div className="flex h-8 shrink-0 items-center gap-0.5 overflow-hidden border-b px-2">
        {editingPath ? (
          <input
            autoFocus
            value={pathInput}
            onChange={(e) => setPathInput(e.target.value)}
            onFocus={(e) => e.currentTarget.select()}
            onKeyDown={(e) => {
              if (e.key === 'Enter') {
                setEditingPath(false)
                p.onNavigate(pathInput)
              }
              if (e.key === 'Escape') setEditingPath(false)
            }}
            onBlur={() => setEditingPath(false)}
            spellCheck={false}
            className="h-6 w-full rounded border bg-background px-1.5 font-mono text-[12px] outline-none"
            aria-label="Path"
          />
        ) : (
          <div
            className="flex h-full min-w-0 flex-1 cursor-text items-center gap-0.5 font-mono text-[12px]"
            onClick={(e) => e.target === e.currentTarget && startEditing()}
            title="Click empty space to type a path"
          >
            {crumbs.map((c, i) => (
              <span key={c.path} className="flex min-w-0 items-center">
                {i > 0 && <ChevronRightIcon className="size-3 shrink-0 text-muted-foreground" />}
                <button className="truncate rounded px-1 hover:bg-accent" onClick={() => p.onNavigate(c.path)}>
                  {c.label}
                </button>
              </span>
            ))}
          </div>
        )}
      </div>
      <div className="grid shrink-0 grid-cols-[1fr_80px_150px_96px] border-b px-2 py-1 text-[11px] font-medium tracking-wide text-muted-foreground uppercase">
        <span>Name</span>
        <span className="text-right">Size</span>
        <span className="pl-3">Modified</span>
        <span className="pl-2">Mode</span>
      </div>
      <div
        className="min-h-0 flex-1 overflow-auto outline-none"
        tabIndex={0}
        onKeyDown={(e) => {
          const del = e.key === 'Delete' || (e.key === 'Backspace' && (e.metaKey || e.ctrlKey))
          if (del && selectedEntries.length) p.onDelete(selectedEntries.map((x) => x.path))
          if (e.key === 'Enter' && selectedEntries.length === 1) open(selectedEntries[0])
          if (e.key === 'a' && (e.metaKey || e.ctrlKey)) {
            e.preventDefault()
            p.onSelect(new Set(entries.map((x) => x.path)))
          }
        }}
        onClick={(e) => e.target === e.currentTarget && p.onSelect(new Set())}
      >
        {p.loading && !p.listing ? (
          <div className="flex h-full items-center justify-center">
            <Spinner />
          </div>
        ) : p.error ? (
          <div className="p-4 text-xs text-danger" data-selectable>
            {p.error}
          </div>
        ) : entries.length === 0 ? (
          <EmptyState icon={FolderIcon} title="Empty folder" />
        ) : (
          entries.map((entry, i) => {
            const sel = p.selected.has(entry.path)
            return (
              <div
                key={entry.path}
                draggable
                onDragStart={(e) => {
                  const paths = sel ? selectedEntries.map((x) => x.path) : [entry.path]
                  if (!sel) p.onSelect(new Set([entry.path]))
                  e.dataTransfer.setData(DRAG_MIME + '-' + p.side, JSON.stringify(paths))
                  e.dataTransfer.effectAllowed = 'copy'
                }}
                onMouseDown={(e) => click(e, entry, i)}
                onDoubleClick={() => open(entry)}
                className={cn(
                  'grid cursor-default grid-cols-[1fr_80px_150px_96px] items-center px-2 py-[3px] text-[12.5px]',
                  sel ? 'bg-row-selected' : 'hover:bg-row-hover',
                  entry.isHidden && 'opacity-60',
                )}
              >
                <span className="flex min-w-0 items-center gap-2">
                  {entry.isDir ? <FolderIcon className="size-4 shrink-0 fill-primary/20 text-primary" /> : <FileIcon className="size-4 shrink-0 text-muted-foreground" />}
                  <span className="truncate">{entry.name}</span>
                  {entry.isLink && <Link2Icon className="size-3 shrink-0 text-muted-foreground" />}
                </span>
                <span className="text-right text-xs text-muted-foreground tabular-nums">{entry.isDir ? '—' : formatBytes(entry.size)}</span>
                <span className="truncate pl-3 text-xs text-muted-foreground">{formatDate(entry.modTime)}</span>
                <span className="truncate pl-2 font-mono text-[11px] text-muted-foreground">{entry.mode}</span>
              </div>
            )
          })
        )}
      </div>
      <div className="flex h-6 shrink-0 items-center border-t px-2 text-[11px] text-muted-foreground">
        {entries.length} item{entries.length === 1 ? '' : 's'}
        {selectedEntries.length > 0 && ` · ${selectedEntries.length} selected`}
        {p.loading && p.listing && <Spinner className="ml-auto size-3" />}
      </div>
    </div>
  )
}

function breadcrumbs(path: string, side: 'local' | 'remote'): { label: string; path: string }[] {
  if (!path) return []
  const win = side === 'local' && /^[A-Za-z]:\\/.test(path)
  const sep = win ? '\\' : '/'
  const parts = path.split(sep).filter(Boolean)
  const out: { label: string; path: string }[] = []
  if (win) {
    let acc = parts[0] + '\\'
    out.push({ label: parts[0], path: acc })
    for (const part of parts.slice(1)) {
      acc = acc.endsWith('\\') ? acc + part : acc + '\\' + part
      out.push({ label: part, path: acc })
    }
    return out
  }
  out.push({ label: '/', path: '/' })
  let acc = ''
  for (const part of parts) {
    acc += '/' + part
    out.push({ label: part, path: acc })
  }
  return out
}
