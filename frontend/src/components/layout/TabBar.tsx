import { BotIcon, CloudAlertIcon, CloudCheckIcon, CloudIcon, FolderIcon, LaptopIcon, LayoutGridIcon, LockIcon, PlusIcon, RadioTowerIcon, RefreshCwIcon, TerminalIcon, XIcon } from 'lucide-react'
import { useState } from 'react'
import { ConfirmDialog } from '@/components/common/ConfirmDialog'
import { Logo } from '@/components/common/Logo'
import { Tooltip } from '@/components/ui/tooltip'
import { api } from '@/lib/api'
import { appKey, cn, isMac, relativeTime } from '@/lib/utils'
import { useAgent } from '@/stores/agent'
import { useEditorState } from '@/stores/editor'
import { useLock } from '@/stores/lock'
import { isBroadcasting, paneKeys, topTabs, useSessions, type Tab } from '@/stores/sessions'
import { useSettings } from '@/stores/settings'
import { useUI } from '@/stores/ui'

/** Asks before closing a connected tab (or, with pane set, one pane of a split tab). */
export function useCloseTab() {
  const [pending, setPending] = useState<{ tab: Tab; pane: boolean } | null>(null)
  const closeNow = (t: Tab, pane: boolean) => (pane ? useSessions.getState().closePane(t.key) : useSessions.getState().close(t.key))
  const request = (t: Tab, pane = false) => {
    const confirm = useSettings.getState().settings?.confirmOnClose ?? true
    const unsaved = useEditorState.getState().unsaved[t.key] ?? 0
    const keys = pane ? [t.key] : paneKeys(t)
    const connected = useSessions.getState().tabs.some((x) => keys.includes(x.key) && x.state === 'connected')
    if (unsaved || (confirm && connected)) setPending({ tab: t, pane })
    else closeNow(t, pane)
  }
  const panes = pending && !pending.pane ? paneKeys(pending.tab).length : 1
  const unsaved = useEditorState((s) => (pending ? (s.unsaved[pending.tab.key] ?? 0) : 0))
  const dialog = (
    <ConfirmDialog
      open={!!pending}
      onOpenChange={(o) => !o && setPending(null)}
      title={`Close ${pending?.pane ? 'pane' : pending?.tab.title}?`}
      description={
        unsaved
          ? `${unsaved} open file${unsaved === 1 ? ' has' : 's have'} unsaved changes that will be lost.`
          : pending?.tab.kind === 'sftp'
            ? 'The SFTP session and any running transfers will be stopped.'
            : panes > 1
              ? `All ${panes} panes in this tab will be closed.`
              : pending?.tab.kind === 'local'
                ? 'The shell and anything running in it will be stopped.'
                : 'The SSH session will be disconnected.'
      }
      confirmLabel={unsaved ? 'Close without saving' : 'Close'}
      destructive={!!unsaved}
      onConfirm={() => {
        if (pending) closeNow(pending.tab, pending.pane)
      }}
    />
  )
  return { request, dialog }
}

export function TabBar() {
  const { tabs: all, active, activate, broadcastAll } = useSessions()
  const tabs = topTabs(all)
  const setPaletteOpen = useUI((s) => s.setPaletteOpen)
  const { request, dialog } = useCloseTab()

  return (
    <div className="titlebar-drag flex h-[50px] shrink-0 items-center border-b bg-sidebar">
      {/* Same width as the vault sidebar, so the window controls and brand sit above it and the tabs line up with the content. */}
      <div className={cn('flex h-full w-[200px] shrink-0 items-center justify-end gap-2 border-r pr-4', !isMac && 'justify-start pl-4')}>
        <div className="flex size-5 items-center justify-center rounded-md bg-primary/15">
          <Logo className="size-3 text-primary" />
        </div>
        <span className="text-[13px] font-semibold tracking-tight">Remvyn</span>
      </div>
      <button
        onClick={() => activate('vault')}
        className={cn(
          'no-drag ml-2 flex h-8 shrink-0 items-center gap-2 rounded-md px-3 text-[12.5px] font-medium transition-colors',
          active === 'vault' ? 'bg-accent text-foreground' : 'text-muted-foreground hover:bg-accent/50 hover:text-foreground',
        )}
      >
        <LayoutGridIcon className="size-3.5" /> Vault
      </button>
      <div className="mx-1.5 h-5 w-px shrink-0 bg-border" />
      <div className="no-drag mr-2 flex min-w-0 flex-1 items-center gap-1 overflow-x-auto [scrollbar-width:none] [&::-webkit-scrollbar]:hidden">
        {tabs.map((t) => (
          <div
            key={t.key}
            onMouseDown={(e) => {
              if (e.button === 1) {
                e.preventDefault()
                request(t)
              }
            }}
            onClick={() => activate(t.key)}
            className={cn(
              'group flex h-8 max-w-52 min-w-32 shrink-0 cursor-default items-center gap-2 rounded-md border px-2.5 text-[12.5px] transition-colors',
              active === t.key ? 'border-border bg-background text-foreground shadow-sm' : 'border-transparent text-muted-foreground hover:bg-accent/50 hover:text-foreground',
            )}
            title={t.remote}
          >
            {t.kind === 'terminal' ? (
              <TerminalIcon className="size-3.5 shrink-0" />
            ) : t.kind === 'local' ? (
              <LaptopIcon className="size-3.5 shrink-0" />
            ) : (
              <FolderIcon className="size-3.5 shrink-0" />
            )}
            <span className="min-w-0 flex-1 truncate">{t.title}</span>
            {t.layout && <span className="shrink-0 text-[10.5px] text-muted-foreground tabular-nums">{paneKeys(t).length}</span>}
            {isBroadcasting(all, broadcastAll, t.key) && <RadioTowerIcon className="size-3 shrink-0 text-warning" aria-label="Broadcasting input" />}
            <span
              className={cn('size-1.5 shrink-0 rounded-full', {
                'bg-success': t.state === 'connected',
                'animate-pulse bg-warning': t.state === 'connecting' || t.retries !== undefined,
                'bg-info': t.state === 'auth',
                'bg-danger': t.state === 'error' && t.retries === undefined,
                'bg-muted-foreground/50': t.state === 'closed' && t.retries === undefined,
              })}
            />
            {t.color && <span className="h-3 w-0.5 shrink-0 rounded-full" style={{ background: t.color }} />}
            <button
              onClick={(e) => {
                e.stopPropagation()
                request(t)
              }}
              className="-mr-1 rounded p-0.5 opacity-0 group-hover:opacity-100 hover:bg-accent"
              aria-label={`Close ${t.title}`}
            >
              <XIcon className="size-3" />
            </button>
          </div>
        ))}
        <Tooltip content={`Quick connect (${appKey}T)`}>
          <button onClick={() => setPaletteOpen(true)} className="flex size-8 shrink-0 items-center justify-center rounded-md text-muted-foreground hover:bg-accent/50 hover:text-foreground" aria-label="Quick connect">
            <PlusIcon className="size-4" />
          </button>
        </Tooltip>
        <Tooltip content={`Local terminal (${appKey}${isMac ? '⌥' : 'Alt+'}T)`}>
          <button onClick={() => useSessions.getState().openLocal()} className="-ml-1 flex size-8 shrink-0 items-center justify-center rounded-md text-muted-foreground hover:bg-accent/50 hover:text-foreground" aria-label="Local terminal">
            <LaptopIcon className="size-3.5" />
          </button>
        </Tooltip>
      </div>
      <StatusButtons />
      {dialog}
    </div>
  )
}

/** Agent access, sync state and the lock button, at the right of the title bar. */
function StatusButtons() {
  const lock = useLock((s) => s.status)
  const sync = useLock((s) => s.sync)
  const agentOn = useSettings((s) => !!s.settings?.agentEnabled)
  const agentRunning = useAgent((s) => s.status?.running ?? 0)
  const btn = 'no-drag flex size-8 shrink-0 items-center justify-center rounded-md text-muted-foreground hover:bg-accent/50 hover:text-foreground'
  const openSettings = () => {
    useSessions.getState().activate('vault')
    useUI.getState().setPage('settings')
  }
  const openAgents = () => {
    useSessions.getState().activate('vault')
    useUI.getState().setPage('agents')
  }
  if (!lock?.protected && !sync?.connected && !agentOn) return null
  return (
    <div className="mr-2 flex items-center gap-0.5">
      {agentOn && (
        <Tooltip content={agentRunning ? `AI agent working (${agentRunning} running)` : 'AI agents can use your servers'}>
          <button onClick={openAgents} className={cn(btn, 'relative')} aria-label="AI agent access">
            <BotIcon className={cn('size-4', agentRunning > 0 && 'text-primary')} />
            {agentRunning > 0 && <span className="absolute top-1.5 right-1.5 size-1.5 animate-pulse rounded-full bg-primary" />}
          </button>
        </Tooltip>
      )}
      {sync?.connected && (
        <Tooltip
          content={
            sync.state === 'error'
              ? `Sync failed: ${sync.error}`
              : sync.state === 'syncing'
                ? 'Syncing…'
                : sync.state === 'locked'
                  ? 'Sync paused while locked'
                  : `Synced with ${sync.repo} ${relativeTime(sync.lastSyncAt)}`
          }
        >
          <button onClick={openSettings} className={btn} aria-label="Sync status">
            {sync.state === 'error' ? (
              <CloudAlertIcon className="size-4 text-danger" />
            ) : sync.state === 'syncing' ? (
              <RefreshCwIcon className="size-3.5 animate-spin" />
            ) : sync.state === 'locked' ? (
              <CloudIcon className="size-4" />
            ) : (
              <CloudCheckIcon className="size-4" />
            )}
          </button>
        </Tooltip>
      )}
      {lock?.protected && (
        <Tooltip content={`Lock Remvyn (${appKey}L)`}>
          <button onClick={() => void api.lock()} className={btn} aria-label="Lock Remvyn">
            <LockIcon className="size-3.5" />
          </button>
        </Tooltip>
      )}
    </div>
  )
}
