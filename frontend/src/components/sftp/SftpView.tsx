import { ArrowLeftIcon, ArrowRightIcon, TerminalIcon } from 'lucide-react'
import { useCallback, useEffect, useRef, useState } from 'react'
import { Group, Panel, Separator } from 'react-resizable-panels'
import { toast } from 'sonner'
import { ConfirmDialog } from '@/components/common/ConfirmDialog'
import { PromptDialog } from '@/components/common/PromptDialog'
import { ConnectOverlay, type Credentials } from '@/components/session/ConnectOverlay'
import { Button } from '@/components/ui/button'
import { api, errorCode, errorMessage, events, onEvent } from '@/lib/api'
import type { DirListing, FileEntry } from '@/lib/types'
import { uid } from '@/lib/utils'
import { useSessions, type Tab } from '@/stores/sessions'
import { useSettings } from '@/stores/settings'
import { useVault } from '@/stores/vault'
import { FilePane } from './FilePane'
import { TransfersPanel } from './TransfersPanel'

interface PaneState {
  listing: DirListing | null
  loading: boolean
  error: string | null
  selected: Set<string>
}

const emptyPane = (): PaneState => ({ listing: null, loading: false, error: null, selected: new Set() })

type Pending =
  | { kind: 'mkdir'; side: 'local' | 'remote' }
  | { kind: 'rename'; side: 'local' | 'remote'; entry: FileEntry }
  | { kind: 'delete'; side: 'local' | 'remote'; paths: string[] }
  | { kind: 'overwrite'; names: string[]; run: () => void }

const baseName = (p: string) => p.replace(/[\\/]+$/, '').split(/[\\/]/).pop() ?? p

export function SftpView({ tab, visible }: { tab: Tab; visible: boolean }) {
  const host = useVault((s) => s.hosts.find((h) => h.id === tab.hostId))
  const settings = useSettings((s) => s.settings)
  const { update, open } = useSessions()
  const [local, setLocal] = useState<PaneState>(emptyPane)
  const [remote, setRemote] = useState<PaneState>(emptyPane)
  const [showHidden, setShowHidden] = useState(settings?.showHiddenFiles ?? false)
  const [pending, setPending] = useState<Pending | null>(null)
  const credsRef = useRef<Credentials | null>(null)
  const sidRef = useRef(tab.sessionId)
  const remoteHome = useRef('')

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

  useEffect(() => {
    void loadLocal('')
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

  const withOverwriteCheck = (names: string[], target: DirListing | null, run: () => void) => {
    const existing = new Set(target?.entries.map((e) => e.name) ?? [])
    const clashes = names.filter((n) => existing.has(n))
    if (clashes.length) setPending({ kind: 'overwrite', names: clashes, run })
    else run()
  }

  const upload = (paths: string[]) => {
    if (!remote.listing || tab.state !== 'connected') return
    const dir = remote.listing.path
    withOverwriteCheck(paths.map(baseName), remote.listing, () => {
      api.upload(sidRef.current, paths, dir).catch((e) => toast.error('Upload failed', { description: errorMessage(e) }))
    })
  }

  const download = (paths: string[]) => {
    if (!local.listing || tab.state !== 'connected') return
    const dir = local.listing.path
    withOverwriteCheck(paths.map(baseName), local.listing, () => {
      api.download(sidRef.current, paths, dir).catch((e) => toast.error('Download failed', { description: errorMessage(e) }))
    })
  }

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
      }
    } catch (e) {
      toast.error(errorMessage(e))
      throw e
    }
  }

  const connected = tab.state === 'connected'

  return (
    <div className={visible ? 'absolute inset-0 flex flex-col' : 'absolute inset-0 hidden'}>
      <div className="flex h-9 shrink-0 items-center gap-2 border-b px-3 text-xs">
        <span className="size-2 rounded-full" style={{ background: connected ? 'var(--success)' : tab.state === 'error' ? 'var(--danger)' : 'var(--muted-foreground)' }} />
        <span className="font-medium">{tab.title}</span>
        <span className="truncate font-mono text-muted-foreground">{tab.remote}</span>
        <span className="text-muted-foreground">· SFTP</span>
        <Button variant="ghost" size="xs" className="ml-auto" onClick={() => open(tab.hostId, 'terminal')}>
          <TerminalIcon /> Open terminal
        </Button>
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
              onTransfer={(entries) => upload(entries.map((e) => e.path))}
              onDrop={(paths) => download(paths)}
              transferLabel="Upload"
              transferIcon={<ArrowRightIcon />}
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
              onTransfer={(entries) => download(entries.map((e) => e.path))}
              onDrop={(paths) => upload(paths)}
              transferLabel="Download"
              transferIcon={<ArrowLeftIcon />}
            />
          </Panel>
        </Group>
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
        open={pending?.kind === 'mkdir' || pending?.kind === 'rename'}
        onOpenChange={(o) => !o && setPending(null)}
        title={pending?.kind === 'rename' ? 'Rename' : 'New folder'}
        initial={pending?.kind === 'rename' ? pending.entry.name : ''}
        placeholder="Name"
        confirmLabel={pending?.kind === 'rename' ? 'Rename' : 'Create'}
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
