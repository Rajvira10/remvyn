import {
  AppWindowIcon,
  ArrowUpIcon,
  ChevronRightIcon,
  ClipboardPasteIcon,
  CopyIcon,
  CopyPlusIcon,
  EyeIcon,
  EyeOffIcon,
  FileArchiveIcon,
  FileCodeIcon,
  FileIcon,
  FilePlusIcon,
  FolderIcon,
  FolderInputIcon,
  FolderOpenIcon,
  FolderOutputIcon,
  FolderPlusIcon,
  HardDriveIcon,
  HomeIcon,
  KeyRoundIcon,
  Link2Icon,
  PencilIcon,
  PackageOpenIcon,
  RefreshCwIcon,
  ScissorsIcon,
  ServerIcon,
  SquarePenIcon,
  TextCursorInputIcon,
  Trash2Icon,
} from 'lucide-react'
import { useRef, useState, type ReactNode } from 'react'
import { EmptyState } from '@/components/common/EmptyState'
import { Spinner } from '@/components/common/Spinner'
import { Button } from '@/components/ui/button'
import {
  ContextMenu,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuLabel,
  ContextMenuSeparator,
  ContextMenuShortcut,
  ContextMenuSub,
  ContextMenuSubContent,
  ContextMenuSubTrigger,
  ContextMenuTrigger,
} from '@/components/ui/context-menu'
import { Tooltip } from '@/components/ui/tooltip'
import type { ArchiveFormat, DirListing, Editor, FileEntry } from '@/lib/types'
import { cn, formatBytes, formatDate, isMac } from '@/lib/utils'

/** Context-menu and keyboard actions, beyond the toolbar's. */
export interface PaneActions {
  /** Open a file in the built-in editor. */
  open: (e: FileEntry) => void
  openWith: (e: FileEntry, editorId: string) => void
  editors: Editor[]
  newFile: () => void
  copy: (paths: string[], cut: boolean) => void
  /** Paste the clipboard into dir; null when there is nothing to paste. */
  paste: ((dir: string) => void) | null
  /** Paths cut to the clipboard from this pane (shown dimmed). */
  cutPaths: Set<string>
  duplicate: (paths: string[]) => void
  copyText: (text: string, what: string) => void
  compress: (entries: FileEntry[], format: ArchiveFormat) => void
  extract: (e: FileEntry, intoFolder: boolean) => void
  /** Change permissions (remote only). */
  chmod?: (e: FileEntry) => void
  /** Show in the system file manager (local only). */
  reveal?: (path: string) => void
}

const ARCHIVE_RE = /\.(zip|jar|war|tar|tgz|tar\.gz|tbz2|tar\.bz2|txz|tar\.xz|gz)$/i
const isArchive = (name: string) => ARCHIVE_RE.test(name)
const stripArchive = (name: string) => name.replace(/\.(tar\.gz|tar\.bz2|tar\.xz|[a-z0-9]+)$/i, '') || name
const mod = isMac ? '⌘' : 'Ctrl+'
const fileManager = isMac ? 'Finder' : /Win/i.test(navigator.userAgent) ? 'Explorer' : 'file manager'

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
  actions: PaneActions
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

  const [menuTargets, setMenuTargets] = useState<FileEntry[]>([])
  const a = p.actions

  const click = (e: React.MouseEvent, entry: FileEntry, index: number) => {
    // Right-clicking inside the selection keeps it, so the menu acts on all of it.
    if (e.button === 2 && p.selected.has(entry.path)) return
    if (e.button === 2) {
      anchor.current = entry.path
      p.onSelect(new Set([entry.path]))
      return
    }
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
    else a.open(entry)
  }

  const dir = p.listing?.path ?? ''

  const pasteItem = (into: string, label = 'Paste') => (
    <ContextMenuItem disabled={!a.paste} onSelect={() => a.paste?.(into)}>
      <ClipboardPasteIcon /> {label}
      <ContextMenuShortcut>{mod}V</ContextMenuShortcut>
    </ContextMenuItem>
  )

  const entryMenu = (targets: FileEntry[]) => {
    const one = targets.length === 1 ? targets[0] : null
    const paths = targets.map((t) => t.path)
    const base = one ? (one.isDir ? one.name : stripArchive(one.name)) : 'archive'
    return (
      <>
        {targets.length > 1 && <ContextMenuLabel>{targets.length} items selected</ContextMenuLabel>}
        {one && !one.isDir && (
          <>
            <ContextMenuItem onSelect={() => a.open(one)} className="font-medium">
              <SquarePenIcon /> Open in editor
              <ContextMenuShortcut>Enter</ContextMenuShortcut>
            </ContextMenuItem>
            <ContextMenuSub>
              <ContextMenuSubTrigger>
                <AppWindowIcon /> Open with
              </ContextMenuSubTrigger>
              <ContextMenuSubContent className="w-56">
                {a.editors.map((ed) => (
                  <ContextMenuItem key={ed.id} onSelect={() => a.openWith(one, ed.id)}>
                    <FileCodeIcon /> {ed.name}
                  </ContextMenuItem>
                ))}
                {a.editors.length > 0 && <ContextMenuSeparator />}
                <ContextMenuItem onSelect={() => a.openWith(one, 'system')}>
                  <AppWindowIcon /> Default app
                </ContextMenuItem>
                {p.side === 'remote' && <div className="px-2 pt-1 pb-1.5 text-[11px] leading-snug text-muted-foreground">Saves in the editor are uploaded back to the server.</div>}
              </ContextMenuSubContent>
            </ContextMenuSub>
          </>
        )}
        {one?.isDir && (
          <ContextMenuItem onSelect={() => p.onNavigate(one.path)} className="font-medium">
            <FolderOpenIcon /> Open
            <ContextMenuShortcut>Enter</ContextMenuShortcut>
          </ContextMenuItem>
        )}
        {one && a.reveal && (
          <ContextMenuItem onSelect={() => a.reveal?.(one.path)}>
            <FolderOpenIcon /> Show in {fileManager}
          </ContextMenuItem>
        )}
        <ContextMenuItem onSelect={() => p.onTransfer(targets)}>
          {p.side === 'remote' ? <FolderInputIcon /> : <FolderOutputIcon />} {p.transferLabel}
          {targets.length > 1 ? ` ${targets.length} items` : ''}
        </ContextMenuItem>
        <ContextMenuSeparator />
        <ContextMenuItem onSelect={() => a.copy(paths, true)}>
          <ScissorsIcon /> Cut
          <ContextMenuShortcut>{mod}X</ContextMenuShortcut>
        </ContextMenuItem>
        <ContextMenuItem onSelect={() => a.copy(paths, false)}>
          <CopyIcon /> Copy
          <ContextMenuShortcut>{mod}C</ContextMenuShortcut>
        </ContextMenuItem>
        {one?.isDir ? pasteItem(one.path, 'Paste into folder') : pasteItem(dir)}
        <ContextMenuItem onSelect={() => a.duplicate(paths)}>
          <CopyPlusIcon /> Duplicate
        </ContextMenuItem>
        <ContextMenuSub>
          <ContextMenuSubTrigger>
            <TextCursorInputIcon /> Copy path
          </ContextMenuSubTrigger>
          <ContextMenuSubContent>
            <ContextMenuItem onSelect={() => a.copyText(paths.join('\n'), targets.length > 1 ? 'Paths' : 'Path')}>Full path</ContextMenuItem>
            <ContextMenuItem onSelect={() => a.copyText(targets.map((t) => t.name).join('\n'), targets.length > 1 ? 'Names' : 'Name')}>Name</ContextMenuItem>
            <ContextMenuItem onSelect={() => a.copyText(dir, 'Folder path')}>Folder path</ContextMenuItem>
          </ContextMenuSubContent>
        </ContextMenuSub>
        <ContextMenuSeparator />
        <ContextMenuSub>
          <ContextMenuSubTrigger>
            <FileArchiveIcon /> Compress
          </ContextMenuSubTrigger>
          <ContextMenuSubContent>
            <ContextMenuItem onSelect={() => a.compress(targets, 'zip')}>{base}.zip</ContextMenuItem>
            <ContextMenuItem onSelect={() => a.compress(targets, 'tar.gz')}>{base}.tar.gz</ContextMenuItem>
          </ContextMenuSubContent>
        </ContextMenuSub>
        {one && !one.isDir && isArchive(one.name) && (
          <>
            <ContextMenuItem onSelect={() => a.extract(one, false)}>
              <PackageOpenIcon /> Extract here
            </ContextMenuItem>
            <ContextMenuItem onSelect={() => a.extract(one, true)}>
              <PackageOpenIcon /> Extract to “{stripArchive(one.name)}/”
            </ContextMenuItem>
          </>
        )}
        <ContextMenuSeparator />
        {one && (
          <ContextMenuItem onSelect={() => p.onRename(one)}>
            <PencilIcon /> Rename
            <ContextMenuShortcut>F2</ContextMenuShortcut>
          </ContextMenuItem>
        )}
        {one && a.chmod && (
          <ContextMenuItem onSelect={() => a.chmod?.(one)}>
            <KeyRoundIcon /> Permissions…
            <ContextMenuShortcut className="font-mono">{one.mode}</ContextMenuShortcut>
          </ContextMenuItem>
        )}
        <ContextMenuItem variant="destructive" onSelect={() => p.onDelete(paths)}>
          <Trash2Icon /> Delete
          <ContextMenuShortcut>Del</ContextMenuShortcut>
        </ContextMenuItem>
      </>
    )
  }

  const folderMenu = () => (
    <>
      <ContextMenuItem onSelect={a.newFile}>
        <FilePlusIcon /> New file
      </ContextMenuItem>
      <ContextMenuItem onSelect={p.onMkdir}>
        <FolderPlusIcon /> New folder
      </ContextMenuItem>
      {pasteItem(dir)}
      <ContextMenuSeparator />
      <ContextMenuItem onSelect={p.onRefresh}>
        <RefreshCwIcon /> Refresh
      </ContextMenuItem>
      <ContextMenuItem onSelect={p.onToggleHidden}>
        {p.showHidden ? <EyeOffIcon /> : <EyeIcon />} {p.showHidden ? 'Hide hidden files' : 'Show hidden files'}
      </ContextMenuItem>
      <ContextMenuItem onSelect={() => a.copyText(dir, 'Folder path')}>
        <TextCursorInputIcon /> Copy folder path
      </ContextMenuItem>
      {a.reveal && dir && (
        <ContextMenuItem onSelect={() => a.reveal?.(dir)}>
          <FolderOpenIcon /> Open in {fileManager}
        </ContextMenuItem>
      )}
    </>
  )

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
      <ContextMenu>
        <ContextMenuTrigger asChild disabled={!p.listing}>
          <div
            className="min-h-0 flex-1 overflow-auto outline-none"
            tabIndex={0}
            onKeyDown={(e) => {
              const cmd = e.metaKey || e.ctrlKey
              const paths = selectedEntries.map((x) => x.path)
              const del = e.key === 'Delete' || (e.key === 'Backspace' && cmd)
              if (del && selectedEntries.length) p.onDelete(paths)
              if (e.key === 'Enter' && selectedEntries.length === 1) open(selectedEntries[0])
              if (e.key === 'F2' && selectedEntries.length === 1) p.onRename(selectedEntries[0])
              if (cmd && !e.shiftKey && !e.altKey) {
                const k = e.key.toLowerCase()
                if (k === 'a') {
                  e.preventDefault()
                  p.onSelect(new Set(entries.map((x) => x.path)))
                }
                if ((k === 'c' || k === 'x') && paths.length) {
                  e.preventDefault()
                  a.copy(paths, k === 'x')
                }
                if (k === 'v' && a.paste && dir) {
                  e.preventDefault()
                  a.paste(dir)
                }
              }
            }}
            onClick={(e) => e.target === e.currentTarget && p.onSelect(new Set())}
            onContextMenu={(e) => {
              // Right-click on empty space: act on the folder itself.
              if (e.target === e.currentTarget) {
                p.onSelect(new Set())
                setMenuTargets([])
              }
            }}
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
                    onContextMenu={() => setMenuTargets(sel ? selectedEntries : [entry])}
                    className={cn(
                      'grid cursor-default grid-cols-[1fr_80px_150px_96px] items-center px-2 py-[3px] text-[12.5px]',
                      sel ? 'bg-row-selected' : 'hover:bg-row-hover',
                      (entry.isHidden || a.cutPaths.has(entry.path)) && 'opacity-60',
                      a.cutPaths.has(entry.path) && 'italic',
                    )}
                  >
                    <span className="flex min-w-0 items-center gap-2">
                      {entry.isDir ? (
                        <FolderIcon className="size-4 shrink-0 fill-primary/20 text-primary" />
                      ) : isArchive(entry.name) ? (
                        <FileArchiveIcon className="size-4 shrink-0 text-muted-foreground" />
                      ) : (
                        <FileIcon className="size-4 shrink-0 text-muted-foreground" />
                      )}
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
        </ContextMenuTrigger>
        <ContextMenuContent className="w-60">{menuTargets.length ? entryMenu(menuTargets) : folderMenu()}</ContextMenuContent>
      </ContextMenu>
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
