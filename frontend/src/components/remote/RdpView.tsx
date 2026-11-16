import { AlertTriangleIcon, CopyIcon, ExternalLinkIcon, MonitorIcon, RotateCwIcon, ShieldCheckIcon, XIcon } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { toast } from 'sonner'
import { ConnectOverlay } from '@/components/session/ConnectOverlay'
import { Button } from '@/components/ui/button'
import { api, copyText, errorCode, errorMessage, events, onEvent } from '@/lib/api'
import type { RemoteDesktop, RemoteDesktopEnded } from '@/lib/types'
import { isMac, isWindows } from '@/lib/utils'
import { useSessions, type Tab } from '@/stores/sessions'
import { useVault } from '@/stores/vault'
import { BrowserOpenURL } from '../../../wailsjs/runtime/runtime'

const WINDOWS_APP_URL = 'https://apps.apple.com/app/windows-app/id1295203466'

/** RDP opens in the system's Remote Desktop app; this tab keeps the SSH tunnel (if any) up and shows its state. */
export function RdpView({ tab, visible }: { tab: Tab; visible: boolean }) {
  const { update, close } = useSessions()
  const host = useVault((s) => s.hosts.find((h) => h.id === tab.hostId))
  const sidRef = useRef(tab.sessionId)
  const [info, setInfo] = useState<RemoteDesktop | null>(null)
  const [launchError, setLaunchError] = useState<{ message: string; code: string } | null>(null)
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    if (tab.state !== 'connecting') return
    const sid = tab.sessionId
    sidRef.current = sid
    setInfo(null)
    setLaunchError(null)
    api
      .openRemoteDesktop(sid, tab.hostId)
      .then((i) => {
        if (sidRef.current !== sid) return
        setInfo(i)
        setLaunchError(i.launchError ? { message: i.launchError, code: i.launchCode ?? '' } : null)
        update(tab.key, { state: 'connected', remote: i.via ? `${i.target} via ${i.via}` : i.target })
      })
      .catch((err) => {
        if (sidRef.current !== sid) return
        update(tab.key, { state: 'error', error: errorMessage(err), errorCode: errorCode(err) })
      })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [tab.sessionId])

  useEffect(
    () =>
      onEvent<RemoteDesktopEnded>(events.REMOTE_ENDED, (e) => {
        if (e.id === sidRef.current) update(tab.key, { state: 'closed', error: e.error || 'The tunnel closed.' })
      }),
    [tab.key, update],
  )

  const openAgain = async () => {
    setBusy(true)
    try {
      const i = await api.launchRDP(tab.sessionId)
      setInfo(i)
      setLaunchError(null)
    } catch (e) {
      setLaunchError({ message: errorMessage(e), code: errorCode(e) })
    } finally {
      setBusy(false)
    }
  }

  const copyPassword = async () => {
    try {
      const pw = await api.revealHostPassword(tab.hostId)
      if (!pw) {
        toast('No password is saved for this host')
        return
      }
      if (await copyText(pw)) toast.success('Password copied')
      else toast.error('Could not copy the password')
    } catch (e) {
      toast.error('Could not load the saved password', { description: errorMessage(e) })
    }
  }

  const tunnelled = !!info?.via
  const app = isMac ? 'Windows App' : isWindows ? 'Remote Desktop Connection' : 'your RDP app'

  return (
    <div className={visible ? 'absolute inset-0 flex flex-col' : 'absolute inset-0 hidden'}>
      <div className="flex h-9 shrink-0 items-center gap-2 border-b px-3 text-xs">
        <span className="size-2 rounded-full" style={{ background: tab.state === 'connected' ? 'var(--success)' : tab.state === 'error' ? 'var(--danger)' : 'var(--muted-foreground)' }} />
        <span className="font-medium">{tab.title}</span>
        <span className="truncate font-mono text-muted-foreground">{tab.remote}</span>
        <span className="text-muted-foreground">· RDP</span>
      </div>
      <div className="relative flex min-h-0 flex-1 items-center justify-center overflow-auto p-6">
        {tab.state === 'connected' && info && (
          <div className="flex w-full max-w-md flex-col gap-4 rounded-xl border bg-card p-6">
            <div className="flex items-center gap-3">
              <div className="flex size-10 items-center justify-center rounded-lg bg-primary/15">
                <MonitorIcon className="size-5 text-primary" />
              </div>
              <div className="min-w-0">
                <div className="truncate font-medium">{tab.title}</div>
                <div className="truncate font-mono text-xs text-muted-foreground" data-selectable>
                  {info.username ? `${info.username}@` : ''}
                  {info.target}
                </div>
              </div>
            </div>
            <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1.5 text-[12.5px]">
              <dt className="text-muted-foreground">Connection</dt>
              <dd className="flex min-w-0 items-center gap-1.5">
                {tunnelled ? (
                  <>
                    <ShieldCheckIcon className="size-3.5 shrink-0 text-success" />
                    <span className="truncate">Tunnelled through {info.via}</span>
                  </>
                ) : (
                  'Direct'
                )}
              </dd>
              {tunnelled && (
                <>
                  <dt className="text-muted-foreground">Local port</dt>
                  <dd className="font-mono" data-selectable>
                    {info.addr}
                  </dd>
                </>
              )}
              <dt className="text-muted-foreground">App</dt>
              <dd>{launchError ? <span className="text-danger">Not opened</span> : `Opened in ${info.client || app}`}</dd>
            </dl>
            {launchError ? (
              <div className="flex gap-2 rounded-md bg-danger/10 px-3 py-2 text-xs text-danger">
                <AlertTriangleIcon className="mt-0.5 size-3.5 shrink-0" />
                <div className="flex flex-col gap-1.5">
                  <span>{launchError.message}</span>
                  {launchError.code === 'no_rdp_app' && isMac && (
                    <button className="flex items-center gap-1 self-start font-medium underline-offset-2 hover:underline" onClick={() => BrowserOpenURL(WINDOWS_APP_URL)}>
                      Get Windows App on the App Store <ExternalLinkIcon className="size-3" />
                    </button>
                  )}
                </div>
              </div>
            ) : (
              <p className="text-xs text-muted-foreground">
                {tunnelled
                  ? `The desktop opens in ${app}. Keep this tab open while you use it: closing it closes the tunnel.`
                  : `The desktop opens in ${app}. Closing this tab doesn't close that window.`}
              </p>
            )}
            <div className="flex flex-wrap gap-2">
              <Button size="sm" disabled={busy} onClick={() => void openAgain()}>
                <RotateCwIcon /> Open again
              </Button>
              {host?.hasPassword && (
                <Button size="sm" variant="outline" onClick={() => void copyPassword()}>
                  <CopyIcon /> Copy password
                </Button>
              )}
              <Button size="sm" variant="ghost" className="ml-auto" onClick={() => close(tab.key)}>
                <XIcon /> Close
              </Button>
            </div>
          </div>
        )}
        <ConnectOverlay
          tab={tab}
          host={host}
          onSubmitCredentials={() => {}}
          onCancel={() => {
            sidRef.current = ''
            void api.closeRemoteDesktop(tab.sessionId)
            update(tab.key, { state: 'closed', error: 'Connection cancelled.' })
          }}
        />
      </div>
    </div>
  )
}
