import { ArrowLeftIcon, FileIcon, HardDriveIcon, SaveIcon, SearchIcon, ServerIcon, WrapTextIcon, XIcon } from 'lucide-react'
import { lazy, Suspense, useRef, useState } from 'react'
import { toast } from 'sonner'
import { Spinner } from '@/components/common/Spinner'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Tooltip } from '@/components/ui/tooltip'
import { useResolvedTheme } from '@/hooks/useTheme'
import { api, errorCode, errorMessage } from '@/lib/api'
import { cn, formatBytes, isMac } from '@/lib/utils'
import type { CodeEditorHandle, CursorInfo } from './CodeEditor'

const CodeEditor = lazy(() => import('./CodeEditor'))

export interface OpenFile {
  key: string
  side: 'local' | 'remote'
  path: string
  name: string
  /** Content when opened; the live text is in the editor. */
  content: string
  size: number
  /** Version on disk that the editor's saved state matches. */
  modTime: number
  dirty: boolean
}

type Choice = { kind: 'conflict'; file: OpenFile } | { kind: 'close'; file: OpenFile }

const WRAP_KEY = 'remvyn.editor.wrap'

function loadWrap(): boolean {
  try {
    return localStorage.getItem(WRAP_KEY) === '1'
  } catch {
    return false
  }
}

export function EditorPanel({
  files,
  activeKey,
  visible,
  hostLabel,
  sessionId,
  onActivate,
  onHide,
  onClose,
  onChange,
  onSaved,
}: {
  files: OpenFile[]
  activeKey: string | null
  visible: boolean
  hostLabel: string
  /** Current SFTP session (it changes on reconnect). */
  sessionId: () => string
  onActivate: (key: string) => void
  onHide: () => void
  onClose: (key: string) => void
  onChange: (key: string, patch: Partial<OpenFile>) => void
  /** A file was written; refresh the listing it's in. */
  onSaved: (file: OpenFile) => void
}) {
  const dark = useResolvedTheme() === 'dark'
  const handles = useRef(new Map<string, CodeEditorHandle>())
  const [wrap, setWrap] = useState(loadWrap)
  const [cursor, setCursor] = useState<CursorInfo>({ line: 1, col: 1, selected: 0 })
  const [langs, setLangs] = useState<Record<string, { label: string; eol: string }>>({})
  const [choice, setChoice] = useState<Choice | null>(null)
  const [saving, setSaving] = useState<string | null>(null)
  const active = files.find((f) => f.key === activeKey) ?? null

  const toggleWrap = () => {
    setWrap((w) => {
      try {
        localStorage.setItem(WRAP_KEY, w ? '0' : '1')
      } catch {
        /* per-viewer convenience only */
      }
      return !w
    })
  }

  const save = async (f: OpenFile, force = false): Promise<boolean> => {
    const h = handles.current.get(f.key)
    if (!h) return false
    const content = h.getContent()
    setSaving(f.key)
    try {
      const expect = force ? 0 : f.modTime
      const res = f.side === 'remote' ? await api.writeRemoteText(sessionId(), f.path, content, expect) : await api.writeLocalText(f.path, content, expect)
      h.markSaved()
      onChange(f.key, { modTime: res.modTime, size: res.size, dirty: false })
      onSaved(f)
      return true
    } catch (e) {
      if (errorCode(e) === 'file_changed') setChoice({ kind: 'conflict', file: f })
      else toast.error(`Could not save ${f.name}`, { description: errorMessage(e) })
      return false
    } finally {
      setSaving(null)
    }
  }

  const reload = async (f: OpenFile) => {
    try {
      const res = f.side === 'remote' ? await api.readRemoteText(sessionId(), f.path) : await api.readLocalText(f.path)
      handles.current.get(f.key)?.setContent(res.content)
      onChange(f.key, { modTime: res.modTime, size: res.size, dirty: false })
    } catch (e) {
      toast.error(`Could not reload ${f.name}`, { description: errorMessage(e) })
    }
  }

  const close = (f: OpenFile) => {
    handles.current.delete(f.key)
    onClose(f.key)
  }

  const requestClose = (f: OpenFile) => {
    if (f.dirty) setChoice({ kind: 'close', file: f })
    else close(f)
  }

  return (
    <div className={cn('absolute inset-0 z-20 flex flex-col bg-background', !visible && 'hidden')}>
      <div className="flex h-9 shrink-0 items-center gap-1 border-b bg-panel pr-2 pl-1">
        <Tooltip content="Back to files">
          <Button variant="ghost" size="xs" onClick={onHide}>
            <ArrowLeftIcon /> Files
          </Button>
        </Tooltip>
        <div className="mx-1 h-4 w-px bg-border" />
        <div className="flex min-w-0 flex-1 items-stretch gap-0.5 self-stretch overflow-x-auto">
          {files.map((f) => (
            <div
              key={f.key}
              onMouseDown={(e) => {
                if (e.button === 1) {
                  e.preventDefault()
                  requestClose(f)
                } else onActivate(f.key)
              }}
              title={f.path}
              className={cn(
                'group flex max-w-56 shrink-0 cursor-default items-center gap-1.5 border-b-2 px-2.5 text-[12.5px]',
                f.key === activeKey ? 'border-primary bg-background text-foreground' : 'border-transparent text-muted-foreground hover:bg-row-hover',
              )}
            >
              {f.side === 'remote' ? <ServerIcon className="size-3 shrink-0" /> : <HardDriveIcon className="size-3 shrink-0" />}
              <span className="truncate">{f.name}</span>
              <button
                className="relative flex size-4 shrink-0 items-center justify-center rounded hover:bg-accent"
                aria-label={`Close ${f.name}`}
                onMouseDown={(e) => e.stopPropagation()}
                onClick={() => requestClose(f)}
              >
                {f.dirty && <span className="absolute size-2 rounded-full bg-foreground/70 group-hover:hidden" />}
                <XIcon className={cn('size-3', f.dirty && 'invisible group-hover:visible')} />
              </button>
            </div>
          ))}
        </div>
        <Tooltip content="Find and replace (Ctrl+F)">
          <Button variant="ghost" size="icon-xs" aria-label="Find" disabled={!active} onClick={() => active && handles.current.get(active.key)?.find()}>
            <SearchIcon />
          </Button>
        </Tooltip>
        <Tooltip content={wrap ? 'Turn off word wrap' : 'Wrap long lines'}>
          <Button variant="ghost" size="icon-xs" aria-label="Toggle word wrap" className={cn(wrap && 'bg-accent text-foreground')} onClick={toggleWrap}>
            <WrapTextIcon />
          </Button>
        </Tooltip>
        <Button size="xs" disabled={!active || !active.dirty || saving === active.key} onClick={() => active && void save(active)}>
          {saving === active?.key ? <Spinner className="size-3" /> : <SaveIcon />}
          Save
          <span className="ml-1 opacity-60">{isMac ? '⌘S' : 'Ctrl+S'}</span>
        </Button>
      </div>

      <div className="relative min-h-0 flex-1">
        {files.length === 0 ? (
          <div className="flex h-full flex-col items-center justify-center gap-2 text-xs text-muted-foreground">
            <FileIcon className="size-6" />
            No open files
          </div>
        ) : (
          <Suspense
            fallback={
              <div className="flex h-full items-center justify-center">
                <Spinner />
              </div>
            }
          >
            {files.map((f) => (
              <CodeEditor
                key={f.key}
                ref={(h) => {
                  if (h) handles.current.set(f.key, h)
                }}
                path={f.path}
                content={f.content}
                dark={dark}
                wrap={wrap}
                active={visible && f.key === activeKey}
                onDirtyChange={(dirty) => onChange(f.key, { dirty })}
                onSave={() => void save(f)}
                onCursor={(c) => f.key === activeKey && setCursor(c)}
                onLanguage={(label, eol) => setLangs((l) => ({ ...l, [f.key]: { label, eol } }))}
              />
            ))}
          </Suspense>
        )}
      </div>

      <div className="flex h-6 shrink-0 items-center gap-3 border-t bg-panel px-3 text-[11px] text-muted-foreground">
        {active && (
          <>
            <span className="flex min-w-0 items-center gap-1 truncate font-mono">
              {active.side === 'remote' ? `${hostLabel}:` : ''}
              {active.path}
            </span>
            <span className="ml-auto shrink-0 tabular-nums">
              Ln {cursor.line}, Col {cursor.col}
              {cursor.selected > 0 && ` (${cursor.selected} selected)`}
            </span>
            <span className="shrink-0">{langs[active.key]?.eol ?? 'LF'}</span>
            <span className="shrink-0">{langs[active.key]?.label ?? ''}</span>
            <span className="shrink-0 tabular-nums">{formatBytes(active.size)}</span>
          </>
        )}
      </div>

      <Dialog open={!!choice} onOpenChange={(o) => !o && setChoice(null)}>
        <DialogContent>
          {choice?.kind === 'conflict' ? (
            <>
              <DialogHeader>
                <DialogTitle>{choice.file.name} was changed</DialogTitle>
                <DialogDescription>
                  Someone changed this file {choice.file.side === 'remote' ? 'on the server' : 'on disk'} after you opened it. Overwrite their changes with yours, or reload
                  it and lose your edits?
                </DialogDescription>
              </DialogHeader>
              <DialogFooter>
                <Button variant="outline" onClick={() => setChoice(null)}>
                  Cancel
                </Button>
                <Button
                  variant="outline"
                  onClick={() => {
                    const f = choice.file
                    setChoice(null)
                    void reload(f)
                  }}
                >
                  Reload
                </Button>
                <Button
                  variant="destructive"
                  onClick={() => {
                    const f = choice.file
                    setChoice(null)
                    void save(f, true)
                  }}
                >
                  Overwrite
                </Button>
              </DialogFooter>
            </>
          ) : choice?.kind === 'close' ? (
            <>
              <DialogHeader>
                <DialogTitle>Save changes to {choice.file.name}?</DialogTitle>
                <DialogDescription>Your changes will be lost if you don't save them.</DialogDescription>
              </DialogHeader>
              <DialogFooter>
                <Button variant="outline" onClick={() => setChoice(null)}>
                  Cancel
                </Button>
                <Button
                  variant="outline"
                  onClick={() => {
                    const f = choice.file
                    setChoice(null)
                    close(f)
                  }}
                >
                  Don't save
                </Button>
                <Button
                  onClick={async () => {
                    const f = choice.file
                    setChoice(null)
                    if (await save(f)) close(f)
                  }}
                >
                  Save
                </Button>
              </DialogFooter>
            </>
          ) : null}
        </DialogContent>
      </Dialog>
    </div>
  )
}
