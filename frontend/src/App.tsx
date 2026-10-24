import { useEffect, useState } from 'react'
import { Toaster, toast } from 'sonner'
import { ErrorBoundary } from '@/components/common/ErrorBoundary'
import { LoadingBlock } from '@/components/common/Spinner'
import { HostEditor } from '@/components/hosts/HostEditor'
import { LockScreen } from '@/components/lock/LockScreen'
import { QuickConnect } from '@/components/layout/QuickConnect'
import { ShortcutsDialog } from '@/components/layout/ShortcutsDialog'
import { TabBar, useCloseTab } from '@/components/layout/TabBar'
import { VaultSidebar } from '@/components/layout/VaultSidebar'
import { HostKeyDialog } from '@/components/session/HostKeyDialog'
import { SftpView } from '@/components/sftp/SftpView'
import { TerminalView } from '@/components/terminal/TerminalView'
import { TooltipProvider } from '@/components/ui/tooltip'
import { useAutoLock } from '@/hooks/useAutoLock'
import { useHotkeys } from '@/hooks/useHotkeys'
import { useResolvedTheme } from '@/hooks/useTheme'
import { api, errorMessage, events, onEvent } from '@/lib/api'
import type { AlertEvent, ForwardStatus, LockStatus, Metrics, MonitorStatus, SyncStatus, Transfer } from '@/lib/types'
import { ForwardingPage } from '@/pages/ForwardingPage'
import { HostsPage } from '@/pages/HostsPage'
import { KeychainPage } from '@/pages/KeychainPage'
import { KnownHostsPage } from '@/pages/KnownHostsPage'
import { MonitoringPage } from '@/pages/MonitoringPage'
import { SettingsPage } from '@/pages/SettingsPage'
import { SnippetsPage } from '@/pages/SnippetsPage'
import { useLock } from '@/stores/lock'
import { useMonitor } from '@/stores/monitor'
import { useSessions } from '@/stores/sessions'
import { useSettings } from '@/stores/settings'
import { useTransfers } from '@/stores/transfers'
import { useUI } from '@/stores/ui'
import { useVault } from '@/stores/vault'

export default function App() {
  const theme = useResolvedTheme()
  const page = useUI((s) => s.page)
  const { tabs, active } = useSessions()
  const [ready, setReady] = useState(false)
  const { request: requestClose, dialog: closeDialog } = useCloseTab()

  useEffect(() => {
    Promise.all([
      useLock.getState().load(),
      useSettings.getState().load(),
      useVault.getState().loadAll(),
      useTransfers.getState().load(),
      useMonitor.getState().loadAlerts(),
    ])
      .catch((e) => toast.error('Could not reach the Flux backend', { description: errorMessage(e) }))
      .finally(() => setReady(true))
    const offs = [
      onEvent<ForwardStatus>(events.FORWARD_STATUS, (s) => useVault.getState().setStatus(s)),
      onEvent<Metrics>(events.MONITOR_SAMPLE, (m) => useMonitor.getState().applySample(m)),
      onEvent<MonitorStatus>(events.MONITOR_STATUS, (s) => useMonitor.getState().applyStatus(s)),
      onEvent<AlertEvent>(events.ALERT, (e) => {
        useMonitor.getState().applyAlert(e)
        if (!e.resolvedAt) toast.error(`${e.ruleName} · ${e.subject}`, { description: e.message })
      }),
      onEvent<LockStatus>(events.LOCK, (s) => useLock.getState().setStatus(s)),
      onEvent<SyncStatus>(events.SYNC_STATUS, (s) => useLock.getState().setSync(s)),
      // Another device changed the vault.
      onEvent(events.VAULT_CHANGED, () => void useVault.getState().loadAll()),
      onEvent<Transfer>(events.TRANSFER, (t) => {
        const prev = useTransfers.getState().byId[t.id]
        useTransfers.getState().apply(t)
        if (prev?.state === 'running' && t.state === 'failed') toast.error(`Transfer failed: ${t.name}`, { description: t.error })
      }),
    ]
    // Pick up changes from other devices when Flux comes to the front.
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
        const s = useSessions.getState()
        const t = s.tabs.find((x) => x.key === s.active)
        if (t) requestClose(t)
      },
      allowInInputs: true,
    },
    { combo: 'app+]', handler: () => useSessions.getState().move(1), allowInInputs: true },
    { combo: 'app+[', handler: () => useSessions.getState().move(-1), allowInInputs: true },
    { combo: 'app+shift+]', handler: () => useSessions.getState().move(1), allowInInputs: true },
    { combo: 'app+shift+[', handler: () => useSessions.getState().move(-1), allowInInputs: true },
    { combo: 'app+0', handler: () => useSessions.getState().activate('vault'), allowInInputs: true },
    ...[1, 2, 3, 4, 5, 6, 7, 8, 9].map((n) => ({
      combo: `app+${n}`,
      handler: () => {
        const t = useSessions.getState().tabs[n - 1]
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
            <LoadingBlock label="Starting Flux…" />
          ) : (
            <>
              <div className={active === 'vault' ? 'flex h-full' : 'hidden'}>
                <VaultSidebar />
                <main className="min-w-0 flex-1">
                  <ErrorBoundary resetKey={page}>
                    {page === 'hosts' && <HostsPage />}
                    {page === 'monitoring' && <MonitoringPage />}
                    {page === 'keychain' && <KeychainPage />}
                    {page === 'forwarding' && <ForwardingPage />}
                    {page === 'snippets' && <SnippetsPage />}
                    {page === 'knownhosts' && <KnownHostsPage />}
                    {page === 'settings' && <SettingsPage />}
                  </ErrorBoundary>
                </main>
              </div>
              {/* Sessions stay mounted while hidden so terminals keep their state. */}
              {tabs.map((t) => (
                <ErrorBoundary key={t.key}>
                  {t.kind === 'terminal' ? <TerminalView tab={t} visible={active === t.key} /> : <SftpView tab={t} visible={active === t.key} />}
                </ErrorBoundary>
              ))}
            </>
          )}
        </div>
      </div>
      <HostEditor />
      <HostKeyDialog />
      <QuickConnect />
      <ShortcutsDialog />
      {closeDialog}
      <LockScreen />
      <Toaster theme={theme} position="bottom-right" richColors closeButton toastOptions={{ className: 'text-[13px]' }} />
    </TooltipProvider>
  )
}
