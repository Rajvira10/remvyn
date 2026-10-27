import { useEffect, useState } from 'react'
import { Toaster, toast } from 'sonner'
import { AgentApprovalDialog } from '@/components/agent/AgentApprovalDialog'
import { ErrorBoundary } from '@/components/common/ErrorBoundary'
import { LoadingBlock } from '@/components/common/Spinner'
import { HostEditor } from '@/components/hosts/HostEditor'
import { LockScreen } from '@/components/lock/LockScreen'
import { QuickConnect } from '@/components/layout/QuickConnect'
import { ShortcutsDialog } from '@/components/layout/ShortcutsDialog'
import { TabBar, useCloseTab } from '@/components/layout/TabBar'
import { UpdateDialog } from '@/components/layout/UpdateDialog'
import { VaultSidebar } from '@/components/layout/VaultSidebar'
import { HostKeyDialog } from '@/components/session/HostKeyDialog'
import { KeyUseDialog } from '@/components/session/KeyUseDialog'
import { SftpView } from '@/components/sftp/SftpView'
import { SplitDividers } from '@/components/terminal/SplitDividers'
import { TerminalView } from '@/components/terminal/TerminalView'
import { TooltipProvider } from '@/components/ui/tooltip'
import { useAutoLock } from '@/hooks/useAutoLock'
import { useHotkeys } from '@/hooks/useHotkeys'
import { useResolvedTheme } from '@/hooks/useTheme'
import { api, errorMessage, events, onEvent } from '@/lib/api'
import type { AgentApproval, AgentLogEntry, AgentStatus, AlertEvent, ForwardStatus, LockStatus, Metrics, MonitorStatus, SyncStatus, Transfer, UpdateStatus } from '@/lib/types'
import { AgentsPage } from '@/pages/AgentsPage'
import { ForwardingPage } from '@/pages/ForwardingPage'
import { HostsPage } from '@/pages/HostsPage'
import { KeychainPage } from '@/pages/KeychainPage'
import { KnownHostsPage } from '@/pages/KnownHostsPage'
import { MonitoringPage } from '@/pages/MonitoringPage'
import { SettingsPage } from '@/pages/SettingsPage'
import { PasswordsPage } from '@/pages/PasswordsPage'
import { SnippetsPage } from '@/pages/SnippetsPage'
import { useAgent } from '@/stores/agent'
import { useLock } from '@/stores/lock'
import { useMonitor } from '@/stores/monitor'
import { usePasswords } from '@/stores/passwords'
import { focusedPane, paneRects, topTabs, useSessions } from '@/stores/sessions'
import { useSettings } from '@/stores/settings'
import { useTransfers } from '@/stores/transfers'
import { useUI } from '@/stores/ui'
import { useUpdate } from '@/stores/update'
import { useVault } from '@/stores/vault'

export default function App() {
  const theme = useResolvedTheme()
  const page = useUI((s) => s.page)
  const { tabs, active } = useSessions()
  const byKey = new Map(tabs.map((t) => [t.key, t]))
  const [ready, setReady] = useState(false)
  const { request: requestClose, dialog: closeDialog } = useCloseTab()

  useEffect(() => {
    Promise.all([
      useLock.getState().load(),
      useSettings.getState().load(),
      useVault.getState().loadAll(),
      usePasswords.getState().load(),
      useTransfers.getState().load(),
      useMonitor.getState().loadAlerts(),
      useAgent.getState().load(),
      useUpdate.getState().load(),
    ])
      .catch((e) => toast.error('Could not reach the Remvyn backend', { description: errorMessage(e) }))
      .finally(() => setReady(true))
    const offs = [
      onEvent<ForwardStatus>(events.FORWARD_STATUS, (s) => useVault.getState().setStatus(s)),
      onEvent<Metrics>(events.MONITOR_SAMPLE, (m) => useMonitor.getState().applySample(m)),
      onEvent<MonitorStatus>(events.MONITOR_STATUS, (s) => useMonitor.getState().applyStatus(s)),
      onEvent<AlertEvent>(events.ALERT, (e) => {
        useMonitor.getState().applyAlert(e)
        if (!e.resolvedAt) toast.error(`${e.ruleName} · ${e.subject}`, { description: e.message })
      }),
      onEvent<LockStatus>(events.LOCK, (s) => {
        useLock.getState().setStatus(s)
        if (s.locked) usePasswords.getState().clear()
        else void usePasswords.getState().load()
      }),
      onEvent<SyncStatus>(events.SYNC_STATUS, (s) => useLock.getState().setSync(s)),
      // Another device changed the vault.
      onEvent(events.VAULT_CHANGED, () => {
        void useVault.getState().loadAll()
        void usePasswords.getState().load()
      }),
      onEvent<AgentStatus>(events.AGENT_STATUS, (s) => useAgent.getState().setStatus(s)),
      onEvent<AgentApproval>(events.AGENT_APPROVAL, (a) => useAgent.getState().addApproval(a)),
      onEvent<string>(events.AGENT_APPROVAL_DONE, (id) => useAgent.getState().removeApproval(id)),
      onEvent<AgentLogEntry>(events.AGENT_ACTIVITY, (e) => useAgent.getState().addLog(e)),
      onEvent<UpdateStatus>(events.UPDATE_STATUS, (s) => useUpdate.getState().apply(s)),
      onEvent<Transfer>(events.TRANSFER, (t) => {
        const prev = useTransfers.getState().byId[t.id]
        useTransfers.getState().apply(t)
        if (prev?.state === 'running' && t.state === 'failed') toast.error(`Transfer failed: ${t.name}`, { description: t.error })
      }),
    ]
    // Pick up changes from other devices when Remvyn comes to the front.
    const onFocus = () => void api.syncWake().catch(() => {})
    window.addEventListener('focus', onFocus)
    return () => {
      offs.forEach((off) => off())
      window.removeEventListener('focus', onFocus)
    }
  }, [])

  useAutoLock()

  useHotkeys([
    { combo: 'app+t', handler: () => useUI.getState().setPaletteOpen(true), allowInInputs: true },
    {
      combo: 'app+w',
      handler: () => {
        // Closes the focused pane of a split tab, otherwise the tab.
        const s = useSessions.getState()
        const t = s.tabs.find((x) => x.key === s.active)
        if (!t) return
        if (t.layout) {
          const p = s.tabs.find((x) => x.key === focusedPane(t))
          if (p) requestClose(p, true)
        } else requestClose(t)
      },
      allowInInputs: true,
    },
    { combo: 'app+alt+t', handler: () => useSessions.getState().openLocal(), allowInInputs: true },
    { combo: 'app+d', handler: () => useSessions.getState().split('row'), allowInInputs: true },
    { combo: 'app+alt+d', handler: () => useSessions.getState().split('col'), allowInInputs: true },
    { combo: 'app+alt+arrowleft', handler: () => useSessions.getState().focusDirection('left'), allowInInputs: true },
    { combo: 'app+alt+arrowright', handler: () => useSessions.getState().focusDirection('right'), allowInInputs: true },
    { combo: 'app+alt+arrowup', handler: () => useSessions.getState().focusDirection('up'), allowInInputs: true },
    { combo: 'app+alt+arrowdown', handler: () => useSessions.getState().focusDirection('down'), allowInInputs: true },
    { combo: 'app+]', handler: () => useSessions.getState().move(1), allowInInputs: true },
    { combo: 'app+[', handler: () => useSessions.getState().move(-1), allowInInputs: true },
    { combo: 'app+shift+]', handler: () => useSessions.getState().move(1), allowInInputs: true },
    { combo: 'app+shift+[', handler: () => useSessions.getState().move(-1), allowInInputs: true },
    { combo: 'app+0', handler: () => useSessions.getState().activate('vault'), allowInInputs: true },
    ...[1, 2, 3, 4, 5, 6, 7, 8, 9].map((n) => ({
      combo: `app+${n}`,
      handler: () => {
        const t = topTabs(useSessions.getState().tabs)[n - 1]
        if (t) useSessions.getState().activate(t.key)
      },
      allowInInputs: true,
    })),
    {
      combo: 'app+,',
      handler: () => {
        useSessions.getState().activate('vault')
        useUI.getState().setPage('settings')
      },
      allowInInputs: true,
    },
    { combo: 'app+/', handler: () => useUI.getState().setShortcutsOpen(true), allowInInputs: true },
    {
      combo: 'app+l',
      handler: () => {
        if (useLock.getState().status?.protected) void api.lock()
      },
      allowInInputs: true,
    },
    // Block the webview's reload shortcut, which would drop every session.
    { combo: 'app+r', handler: () => {}, allowInInputs: true },
  ])

  return (
    <TooltipProvider delayDuration={400}>
      <div className="flex h-full flex-col">
        <TabBar />
        <div className="relative min-h-0 flex-1">
          {!ready ? (
            <LoadingBlock label="Starting Remvyn…" />
          ) : (
            <>
              <div className={active === 'vault' ? 'flex h-full' : 'hidden'}>
                <VaultSidebar />
                <main className="min-w-0 flex-1">
                  <ErrorBoundary resetKey={page}>
                    {page === 'hosts' && <HostsPage />}
                    {page === 'monitoring' && <MonitoringPage />}
                    {page === 'passwords' && <PasswordsPage />}
                    {page === 'keychain' && <KeychainPage />}
                    {page === 'forwarding' && <ForwardingPage />}
                    {page === 'snippets' && <SnippetsPage />}
                    {page === 'knownhosts' && <KnownHostsPage />}
                    {page === 'agents' && <AgentsPage />}
                    {page === 'settings' && <SettingsPage />}
                  </ErrorBoundary>
                </main>
              </div>
              {/* Sessions stay mounted while hidden so terminals keep their state.
                  Panes of split tabs render flat too, so changing a layout never remounts one. */}
              {tabs.map((t) => {
                const root = (t.parent && byKey.get(t.parent)) || t
                const visible = active === root.key
                if (t.kind === 'sftp') {
                  return (
                    <ErrorBoundary key={t.key}>
                      <SftpView tab={t} visible={visible} />
                    </ErrorBoundary>
                  )
                }
                const split = !!root.layout
                return (
                  <ErrorBoundary key={t.key}>
                    <TerminalView
                      tab={t}
                      visible={visible}
                      split={split}
                      focused={!split || focusedPane(root) === t.key}
                      rect={split ? paneRects(root.layout, root.key)[t.key] : undefined}
                    />
                  </ErrorBoundary>
                )
              })}
              {tabs.map((t) => t.layout && <SplitDividers key={`split-${t.key}`} rootKey={t.key} layout={t.layout} visible={active === t.key} />)}
            </>
          )}
        </div>
      </div>
      <HostEditor />
      <HostKeyDialog />
      <KeyUseDialog />
      <AgentApprovalDialog />
      <QuickConnect />
      <ShortcutsDialog />
      <UpdateDialog />
      {closeDialog}
      <LockScreen />
      <Toaster theme={theme} position="bottom-right" richColors closeButton toastOptions={{ className: 'text-[13px]' }} />
    </TooltipProvider>
  )
}
