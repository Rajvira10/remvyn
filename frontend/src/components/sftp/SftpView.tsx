import { ArrowLeftIcon, ArrowRightIcon, SquarePenIcon, TerminalIcon } from 'lucide-react'
import { useCallback, useEffect, useRef, useState } from 'react'
import { Group, Panel, Separator } from 'react-resizable-panels'
import { toast } from 'sonner'
import { ConfirmDialog } from '@/components/common/ConfirmDialog'
import { PromptDialog } from '@/components/common/PromptDialog'
import { ConnectOverlay, type Credentials } from '@/components/session/ConnectOverlay'
import { Button } from '@/components/ui/button'
import { api, copyText, errorCode, errorMessage, events, onEvent } from '@/lib/api'
import type { ArchiveFormat, DirListing, EditSync, Editor, FileEntry } from '@/lib/types'
import { uid } from '@/lib/utils'
import { useEditorState } from '@/stores/editor'
import { useSessions, type Tab } from '@/stores/sessions'
import { useSettings } from '@/stores/settings'
import { useVault } from '@/stores/vault'
import { EditorPanel, type OpenFile } from './editor/EditorPanel'
import { FilePane, type PaneActions } from './FilePane'
import { TransfersPanel } from './TransfersPanel'

interface PaneState {
  listing: DirListing | null
  loading: boolean
  error: string | null
  selected: Set<string>
}

type Side = 'local' | 'remote'

const emptyPane = (): PaneState => ({ listing: null, loading: false, error: null, selected: new Set() })

type Pending =
  | { kind: 'mkdir'; side: Side }
  | { kind: 'newfile'; side: Side }
  | { kind: 'rename'; side: Side; entry: FileEntry }
  | { kind: 'delete'; side: Side; paths: string[] }
  | { kind: 'overwrite'; names: string[]; run: () => void }
  | { kind: 'compress'; side: Side; entries: FileEntry[]; format: ArchiveFormat }
  | { kind: 'chmod'; entry: FileEntry }

interface Clipboard {
  side: Side
  paths: string[]
  cut: boolean
}

const baseName = (p: string) => p.replace(/[\\/]+$/, '').split(/[\\/]/).pop() ?? p
const parentDir = (p: string) => p.replace(/[\\/]+$/, '').replace(/[\\/][^\\/]*$/, '') || (p.startsWith('/') ? '/' : p)
const stripArchive = (name: string) => name.replace(/\.(tar\.gz|tar\.bz2|tar\.xz|[a-z0-9]+)$/i, '') || name

/** "-rwxr-xr-x" → "755" (with a leading digit for setuid/setgid/sticky). */
function modeToOctal(mode: string): string {
  const m = mode.slice(-9)
  if (m.length !== 9) return '644'
  let special = 0
  const digit = (i: number, bit: number) => {
    const t = m.slice(i, i + 3)
    if (/[sS]/.test(t[2])) special |= bit === 2 ? 4 : 2
    if (/[tT]/.test(t[2])) special |= 1
    return (t[0] === 'r' ? 4 : 0) + (t[1] === 'w' ? 2 : 0) + (/[xst]/.test(t[2]) ? 1 : 0)
  }
  const s = `${digit(0, 2)}${digit(3, 1)}${digit(6, 0)}`
  return special ? `${special}${s}` : s
}

export function SftpView({ tab, visible }: { tab: Tab; visible: boolean }) {
  const host = useVault((s) => s.hosts.find((h) => h.id === tab.hostId))
  const settings = useSettings((s) => s.settings)
  const { update, open } = useSessions()
  const [local, setLocal] = useState<PaneState>(emptyPane)
  const [remote, setRemote] = useState<PaneState>(emptyPane)
  const [showHidden, setShowHidden] = useState(settings?.showHiddenFiles ?? false)
  const [pending, setPending] = useState<Pending | null>(null)
  const [clipboard, setClipboard] = useState<Clipboard | null>(null)
  const [editors, setEditors] = useState<Editor[]>([])
  const [files, setFiles] = useState<OpenFile[]>([])
  const [activeFile, setActiveFile] = useState<string | null>(null)
  const [editorVisible, setEditorVisible] = useState(false)
  const credsRef = useRef<Credentials | null>(null)
  const sidRef = useRef(tab.sessionId)
  const remoteHome = useRef('')
  // Latest listings, for event handlers registered once.
  const listings = useRef({ local: local.listing, remote: remote.listing })
  listings.current = { local: local.listing, remote: remote.listing }

  const loadLocal = useCallback(async (dir: string) => {
    setLocal((s) => ({ ...s, loading: true }))
    try {
      const listing = await api.listLocal(dir)
      setLocal({ listing, loading: false, error: null, selected: new Set() })
    } catch (e) {
      setLocal((s) => {
        if (s.listing) toast.error(errorMessage(e))
        return { ...s, loading: false, error: s.listing ? null : errorMessage(e) }
      })
    }
  }, [])

  const loadRemote = useCallback(async (dir: string) => {
    setRemote((s) => ({ ...s, loading: true }))
    try {
      const listing = await api.listRemote(sidRef.current, dir)
      setRemote({ listing, loading: false, error: null, selected: new Set() })
    } catch (e) {
      setRemote((s) => {
        if (s.listing) toast.error(errorMessage(e))
        return { ...s, loading: false, error: s.listing ? null : errorMessage(e) }
      })
    }
  }, [])

  /** Reloads a pane, keeping its selection when select is omitted. */
  const refresh = useCallback(
    async (side: Side, select?: string[]) => {
      const listing = listings.current[side]
      if (!listing) return
      await (side === 'local' ? loadLocal(listing.path) : loadRemote(listing.path))
      if (select) (side === 'local' ? setLocal : setRemote)((s) => ({ ...s, selected: new Set(select) }))
    },
    [loadLocal, loadRemote],
  )

  useEffect(() => {
    void loadLocal('')
    api
      .editors()
      .then(setEditors)
      .catch(() => {})
  }, [loadLocal])

  // Connect on each new attempt.
  useEffect(() => {
    if (tab.state !== 'connecting') return
    const sid = tab.sessionId
    sidRef.current = sid
    const creds = credsRef.current
    credsRef.current = null
    api
      .openSFTP(sid, tab.hostId, {
        password: creds?.password ?? '',
        savePassword: creds?.savePassword ?? false,
        passphrase: creds?.passphrase ?? '',
        cols: 0,
        rows: 0,
      })
      .then(async (info) => {
        if (sidRef.current !== sid) return
        update(tab.key, { state: 'connected', remote: info.remote })
        if (creds?.savePassword) void useVault.getState().loadHosts()
        try {
          remoteHome.current = await api.remoteHome(sid)
        } catch {
          remoteHome.current = '/'
        }
        void loadRemote(remote.listing?.path || remoteHome.current)
      })
      .catch((err) => {
        if (sidRef.current !== sid) return
        const code = errorCode(err)
        const auth = code === 'password_required' || code === 'passphrase_required' || (code === 'auth_failed' && host?.authMethod === 'password')
        update(tab.key, { state: auth ? 'auth' : 'error', error: errorMessage(err), errorCode: code })
      })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [tab.sessionId])

  useEffect(
    () =>
      onEvent<string>(events.SFTP_CLOSED, (id) => {
        if (id === sidRef.current) update(tab.key, { state: 'closed', error: 'The SFTP connection was closed.' })
      }),
    [tab.key, update],
  )

  // Refresh panes when this session's transfers finish.
  useEffect(
    () =>
      onEvent<{ sessionId: string; state: string; direction: string }>(events.TRANSFER, (t) => {
        if (t.sessionId !== sidRef.current || t.state === 'running') return
        if (t.direction === 'upload' && remote.listing) void loadRemote(remote.listing.path)
        if (t.direction === 'download' && local.listing) void loadLocal(local.listing.path)
      }),
    [remote.listing, local.listing, loadRemote, loadLocal],
  )

  // Files opened in an external editor are uploaded on save.
  useEffect(
    () =>
      onEvent<EditSync>(events.EDIT_SYNC, (ev) => {
        if (ev.sessionId !== sidRef.current) return
        if (ev.conflict) {
          toast.warning(`${ev.name} changed on the server`, {
            description: 'Your save was not uploaded, so the server copy is unchanged.',
            duration: 20000,
            action: {
              label: 'Overwrite',
              onClick: () => void api.forceEditUpload(sidRef.current, ev.path).catch((e) => toast.error(errorMessage(e))),
            },
          })
        } else if (ev.error) {
          toast.error(`Could not upload ${ev.name}`, { description: ev.error })
        } else {
          toast.success(`Uploaded ${ev.name}`)
          if (listings.current.remote?.path === parentDir(ev.path)) void refresh('remote')
        }
      }),
    [refresh],
  )

  // Unsaved editor files make closing the tab ask first.
  const unsaved = files.filter((f) => f.dirty).length
  useEffect(() => {
    useEditorState.getState().setUnsaved(tab.key, unsaved)
  }, [tab.key, unsaved])
  useEffect(() => () => useEditorState.getState().setUnsaved(tab.key, 0), [tab.key])

  const withOverwriteCheck = (names: string[], target: DirListing | null, run: () => void) => {
    const existing = new Set(target?.entries.map((e) => e.name) ?? [])
    const clashes = names.filter((n) => existing.has(n))
    if (clashes.length) setPending({ kind: 'overwrite', names: clashes, run })
    else run()
  }

  /** The listing of dir on side: the visible one if it matches, otherwise fetched. */
  const listingOf = async (side: Side, dir: string): Promise<DirListing | null> => {
    const shown = listings.current[side]
    if (shown?.path === dir) return shown
    try {
      return side === 'local' ? await api.listLocal(dir) : await api.listRemote(sidRef.current, dir)
    } catch {
      return null
    }
  }

  const upload = async (paths: string[], dir = remote.listing?.path) => {
    if (!dir || tab.state !== 'connected') return
    withOverwriteCheck(paths.map(baseName), await listingOf('remote', dir), () => {
      api.upload(sidRef.current, paths, dir).catch((e) => toast.error('Upload failed', { description: errorMessage(e) }))
    })
  }

  const download = async (paths: string[], dir = local.listing?.path) => {
    if (!dir || tab.state !== 'connected') return
    withOverwriteCheck(paths.map(baseName), await listingOf('local', dir), () => {
      api.download(sidRef.current, paths, dir).catch((e) => toast.error('Download failed', { description: errorMessage(e) }))
    })
  }

  /** Runs a slow operation behind a loading toast, then refreshes the pane. */
  const longOp = async (side: Side, label: string, run: () => Promise<string | void>, done: (result: string | void) => string) => {
    const id = toast.loading(label)
    try {
      const result = await run()
      toast.success(done(result), { id })
      await refresh(side, typeof result === 'string' ? [result] : undefined)
    } catch (e) {
      toast.error(errorMessage(e), { id })
    }
  }

  // ------------------------------------------------------------ editor

  const openInEditor = async (side: Side, entry: FileEntry) => {
    const key = `${side}:${side === 'remote' ? sidRef.current : ''}:${entry.path}`
    if (files.some((f) => f.key === key)) {
      setActiveFile(key)
      setEditorVisible(true)
      return
    }
    try {
      const fc = side === 'remote' ? await api.readRemoteText(sidRef.current, entry.path) : await api.readLocalText(entry.path)
      setFiles((fs) => [...fs, { key, side, path: entry.path, name: entry.name, content: fc.content, size: fc.size, modTime: fc.modTime, dirty: false }])
      setActiveFile(key)
      setEditorVisible(true)
    } catch (e) {
      const code = errorCode(e)
      if (code === 'binary_file' || code === 'file_too_large') {
        toast.error(errorMessage(e), {
          description: 'Open it with another app instead.',
          action: { label: 'Open with default app', onClick: () => void openWith(side, entry, 'system') },
        })
      } else toast.error(`Could not open ${entry.name}`, { description: errorMessage(e) })
    }
  }

  const openWith = async (side: Side, entry: FileEntry, editorId: string) => {
    const name = editorId === 'system' ? 'the default app' : (editors.find((e) => e.id === editorId)?.name ?? 'the editor')
    try {
      if (side === 'remote') {
        await api.openRemoteWith(sidRef.current, entry.path, editorId)
        toast.success(`Opened ${entry.name} in ${name}`, { description: 'Each save is uploaded back to the server.' })
      } else {
        await api.openLocalWith(editorId, entry.path)
      }
    } catch (e) {
      toast.error(`Could not open ${entry.name}`, { description: errorMessage(e) })
    }
  }

  const closeFile = (key: string) => {
    const idx = files.findIndex((f) => f.key === key)
    const next = files.filter((f) => f.key !== key)
    setFiles(next)
    if (key === activeFile) setActiveFile(next[idx]?.key ?? next[idx - 1]?.key ?? null)
    if (next.length === 0) setEditorVisible(false)
  }

  // ------------------------------------------------------------ clipboard

  const paste = async (side: Side, dir: string) => {
    const clip = clipboard
    if (!clip || tab.state !== 'connected') return
    if (clip.side !== side) {
      // Pasting into the other pane transfers the files.
      if (side === 'local') await download(clip.paths, dir)
      else await upload(clip.paths, dir)
      return
    }
    const n = clip.paths.length
    const what = n === 1 ? baseName(clip.paths[0]) : `${n} items`
    await longOp(
      side,
      clip.cut ? `Moving ${what}…` : `Copying ${what}…`,
      async () => {
        if (side === 'local') await (clip.cut ? api.moveLocal(clip.paths, dir) : api.copyLocal(clip.paths, dir))
        else await (clip.cut ? api.moveRemote(sidRef.current, clip.paths, dir) : api.copyRemote(sidRef.current, clip.paths, dir))
      },
      () => (clip.cut ? `Moved ${what}` : `Copied ${what}`),
    )
    if (clip.cut) setClipboard(null)
  }

  const actionsFor = (side: Side): PaneActions => ({
    open: (e) => void openInEditor(side, e),
    openWith: (e, id) => void openWith(side, e, id),
    editors,
    newFile: () => setPending({ kind: 'newfile', side }),
    copy: (paths, cut) => {
      setClipboard({ side, paths, cut })
      toast(`${cut ? 'Cut' : 'Copied'} ${paths.length === 1 ? baseName(paths[0]) : `${paths.length} items`}`, {
        description: 'Paste into a folder in either pane.',
        duration: 2000,
      })
    },
    paste: clipboard && (clipboard.side === side || tab.state === 'connected') ? (dir) => void paste(side, dir) : null,
    cutPaths: new Set(clipboard?.cut && clipboard.side === side ? clipboard.paths : []),
    duplicate: (paths) => {
      const dir = listings.current[side]?.path
      if (!dir) return
      void longOp(
        side,
        'Duplicating…',
        () => (side === 'local' ? api.copyLocal(paths, dir) : api.copyRemote(sidRef.current, paths, dir)),
        () => `Duplicated ${paths.length === 1 ? baseName(paths[0]) : `${paths.length} items`}`,
      )
    },
    copyText: (text, what) => {
      void copyText(text).then((ok) => (ok ? toast.success(`${what} copied`) : toast.error('Could not copy to the clipboard')))
    },
    compress: (entries, format) => setPending({ kind: 'compress', side, entries, format }),
    extract: (entry, intoFolder) =>
      void longOp(
        side,
        `Extracting ${entry.name}…`,
        () => (side === 'local' ? api.extractLocal(entry.path, intoFolder) : api.extractRemote(sidRef.current, entry.path, intoFolder)),
        (dest) => (intoFolder && dest ? `Extracted to ${baseName(dest)}` : `Extracted ${entry.name}`),
      ),
    chmod: side === 'remote' ? (entry) => setPending({ kind: 'chmod', entry }) : undefined,
    reveal: side === 'local' ? (p) => void api.revealLocal(p).catch((e) => toast.error(errorMessage(e))) : undefined,
  })

  // ------------------------------------------------------------ dialogs

  const runPending = async (value?: string) => {
    if (!pending) return
    try {
      if (pending.kind === 'mkdir' && value) {
        if (pending.side === 'local') {
          await api.mkdirLocal(local.listing!.path, value)
          await loadLocal(local.listing!.path)
        } else {
          await api.mkdirRemote(sidRef.current, remote.listing!.path, value)
          await loadRemote(remote.listing!.path)
        }
      } else if (pending.kind === 'newfile' && value) {
        const side = pending.side
        const dir = listings.current[side]!.path
        const p = side === 'local' ? await api.createLocalFile(dir, value) : await api.createRemoteFile(sidRef.current, dir, value)
        await refresh(side, [p])
        void openInEditor(side, { name: value, path: p, size: 0, mode: '', modTime: 0, isDir: false, isLink: false, isHidden: value.startsWith('.') })
      } else if (pending.kind === 'rename' && value) {
        if (pending.side === 'local') {
          await api.renameLocal(pending.entry.path, value)
          await loadLocal(local.listing!.path)
        } else {
          await api.renameRemote(sidRef.current, pending.entry.path, value)
          await loadRemote(remote.listing!.path)
        }
      } else if (pending.kind === 'delete') {
        if (pending.side === 'local') {
          await api.removeLocal(pending.paths)
          await loadLocal(local.listing!.path)
        } else {
          await api.removeRemote(sidRef.current, pending.paths)
          await loadRemote(remote.listing!.path)
        }
      } else if (pending.kind === 'overwrite') {
        pending.run()
      } else if (pending.kind === 'compress' && value) {
        const { side, entries, format } = pending
        const dir = listings.current[side]!.path
        const names = entries.map((e) => e.name)
        void longOp(
          side,
          `Compressing ${value}…`,
          () => (side === 'local' ? api.compressLocal(dir, names, value, format) : api.compressRemote(sidRef.current, dir, names, value, format)),
          () => `Created ${value}`,
        )
      } else if (pending.kind === 'chmod' && value) {
        if (!/^[0-7]{3,4}$/.test(value)) {
          toast.error('Enter permissions as 3 or 4 octal digits, like 644 or 755')
          throw new Error('invalid mode')
        }
        await api.chmodRemote(sidRef.current, pending.entry.path, parseInt(value, 8))
        await refresh('remote', [pending.entry.path])
      }
    } catch (e) {
      if (!(e instanceof Error && e.message === 'invalid mode')) toast.error(errorMessage(e))
      throw e
    }
  }

  const connected = tab.state === 'connected'
  const promptKinds = ['mkdir', 'newfile', 'rename', 'compress', 'chmod']
  const prompt = (() => {
    switch (pending?.kind) {
      case 'mkdir':
        return { title: 'New folder', initial: '', placeholder: 'Name', confirm: 'Create' }
      case 'newfile':
        return { title: 'New file', initial: '', placeholder: 'Name, e.g. notes.txt', confirm: 'Create and open' }
      case 'rename':
        return { title: 'Rename', initial: pending.entry.name, placeholder: 'Name', confirm: 'Rename' }
      case 'compress': {
        const base = pending.entries.length === 1 ? (pending.entries[0].isDir ? pending.entries[0].name : stripArchive(pending.entries[0].name)) : 'archive'
        return {
          title: `Compress ${pending.entries.length === 1 ? pending.entries[0].name : `${pending.entries.length} items`}`,
          description: pending.side === 'remote' ? 'The archive is created on the server, next to the selected items.' : 'The archive is created next to the selected items.',
          initial: `${base}.${pending.format}`,
          placeholder: 'Archive name',
          confirm: 'Compress',
        }
      }
      case 'chmod':
        return {
          title: `Permissions for ${pending.entry.name}`,
          description: `Currently ${pending.entry.mode}. Use octal notation: 644 for files anyone can read, 600 for private files, 755 for folders and scripts.`,
          initial: modeToOctal(pending.entry.mode),
          placeholder: '644',
          confirm: 'Apply',
        }
    }
    return { title: '', initial: '', placeholder: '', confirm: 'Save' }
  })()

  return (
    <div className={visible ? 'absolute inset-0 flex flex-col' : 'absolute inset-0 hidden'}>
      <div className="flex h-9 shrink-0 items-center gap-2 border-b px-3 text-xs">
        <span className="size-2 rounded-full" style={{ background: connected ? 'var(--success)' : tab.state === 'error' ? 'var(--danger)' : 'var(--muted-foreground)' }} />
        <span className="font-medium">{tab.title}</span>
        <span className="truncate font-mono text-muted-foreground">{tab.remote}</span>
        <span className="text-muted-foreground">· SFTP</span>
        <div className="ml-auto flex items-center gap-1">
          {files.length > 0 && !editorVisible && (
            <Button variant="secondary" size="xs" onClick={() => setEditorVisible(true)}>
              <SquarePenIcon /> Editor ({files.length}
              {unsaved ? `, ${unsaved} unsaved` : ''})
            </Button>
          )}
          <Button variant="ghost" size="xs" onClick={() => open(tab.hostId, 'terminal')}>
            <TerminalIcon /> Open terminal
          </Button>
        </div>
      </div>
      <div className="relative min-h-0 flex-1">
        <Group orientation="horizontal" className="h-full">
          <Panel id="local" defaultSize="50%" minSize="25%" className="flex">
            <FilePane
              side="local"
              title="This computer"
              {...local}
              showHidden={showHidden}
              onSelect={(selected) => setLocal((s) => ({ ...s, selected }))}
              onNavigate={(p) => void loadLocal(p)}
              onRefresh={() => local.listing && void loadLocal(local.listing.path)}
              onHome={() => void loadLocal('')}
              onToggleHidden={() => setShowHidden((h) => !h)}
              onMkdir={() => setPending({ kind: 'mkdir', side: 'local' })}
              onRename={(entry) => setPending({ kind: 'rename', side: 'local', entry })}
              onDelete={(paths) => setPending({ kind: 'delete', side: 'local', paths })}
              onTransfer={(entries) => void upload(entries.map((e) => e.path))}
              onDrop={(paths) => void download(paths)}
              transferLabel="Upload"
              transferIcon={<ArrowRightIcon />}
              actions={actionsFor('local')}
            />
          </Panel>
          <Separator className="w-px bg-border transition-colors hover:bg-primary data-[separator=active]:bg-primary" />
          <Panel id="remote" defaultSize="50%" minSize="25%" className="flex">
            <FilePane
              side="remote"
              title={host?.label ?? 'Remote'}
              {...remote}
              showHidden={showHidden}
              onSelect={(selected) => setRemote((s) => ({ ...s, selected }))}
              onNavigate={(p) => void loadRemote(p)}
              onRefresh={() => remote.listing && void loadRemote(remote.listing.path)}
              onHome={() => void loadRemote(remoteHome.current)}
              onToggleHidden={() => setShowHidden((h) => !h)}
              onMkdir={() => setPending({ kind: 'mkdir', side: 'remote' })}
              onRename={(entry) => setPending({ kind: 'rename', side: 'remote', entry })}
              onDelete={(paths) => setPending({ kind: 'delete', side: 'remote', paths })}
              onTransfer={(entries) => void download(entries.map((e) => e.path))}
              onDrop={(paths) => void upload(paths)}
              transferLabel="Download"
              transferIcon={<ArrowLeftIcon />}
              actions={actionsFor('remote')}
            />
          </Panel>
        </Group>
        {files.length > 0 && (
          <EditorPanel
            files={files}
            activeKey={activeFile}
            visible={visible && editorVisible}
            hostLabel={host?.label ?? 'server'}
            sessionId={() => sidRef.current}
            onActivate={setActiveFile}
            onHide={() => setEditorVisible(false)}
            onClose={closeFile}
            onChange={(key, patch) => setFiles((fs) => fs.map((f) => (f.key === key ? { ...f, ...patch } : f)))}
            onSaved={(f) => {
              if (listings.current[f.side]?.path === parentDir(f.path)) void refresh(f.side)
            }}
          />
        )}
        <ConnectOverlay
          tab={tab}
          host={host}
          onSubmitCredentials={(c) => {
            credsRef.current = c
            update(tab.key, { state: 'connecting', sessionId: uid(), error: undefined })
          }}
          onCancel={() => void api.cancelConnect(tab.sessionId)}
        />
      </div>
      <TransfersPanel sessionId={tab.sessionId} />

      <PromptDialog
        open={!!pending && promptKinds.includes(pending.kind)}
        onOpenChange={(o) => !o && setPending(null)}
        title={prompt.title}
        description={'description' in prompt ? prompt.description : undefined}
        initial={prompt.initial}
        placeholder={prompt.placeholder}
        confirmLabel={prompt.confirm}
        onSubmit={(v) => runPending(v)}
      />
      <ConfirmDialog
        open={pending?.kind === 'delete'}
        onOpenChange={(o) => !o && setPending(null)}
        title={`Delete ${pending?.kind === 'delete' ? pending.paths.length : 0} item(s)?`}
        description={
          pending?.kind === 'delete'
            ? `${pending.paths.map(baseName).slice(0, 5).join(', ')}${pending.paths.length > 5 ? '…' : ''} will be permanently deleted ${pending.side === 'remote' ? 'from the server' : 'from this computer'}. Folders are deleted with all their contents.`
            : ''
        }
        confirmLabel="Delete"
        destructive
        onConfirm={() => runPending()}
      />
      <ConfirmDialog
        open={pending?.kind === 'overwrite'}
        onOpenChange={(o) => !o && setPending(null)}
        title="Replace existing items?"
        description={pending?.kind === 'overwrite' ? `${pending.names.join(', ')} already exist in the destination and will be overwritten.` : ''}
        confirmLabel="Replace"
        destructive
        onConfirm={() => runPending()}
      />
    </div>
  )
}
