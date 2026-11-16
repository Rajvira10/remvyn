import type RFB from '@novnc/novnc'
import { ClipboardPasteIcon, EyeIcon, KeyboardIcon, Maximize2Icon, MaximizeIcon, Minimize2Icon, MinimizeIcon, RotateCwIcon } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { toast } from 'sonner'
import { ConnectOverlay, type Credentials } from '@/components/session/ConnectOverlay'
import { Button } from '@/components/ui/button'
import { Tooltip } from '@/components/ui/tooltip'
import { api, copyText, errorCode, errorMessage, events, onEvent, readClipboard } from '@/lib/api'
import type { RemoteDesktopEnded } from '@/lib/types'
import { cn } from '@/lib/utils'
import { useSessions, type Tab } from '@/stores/sessions'
import { useVault } from '@/stores/vault'
import { WindowFullscreen, WindowIsFullscreen, WindowUnfullscreen } from '../../../wailsjs/runtime/runtime'

/** An embedded VNC viewer (noVNC) connected through Remvyn's local WebSocket proxy. */
export function VncView({ tab, visible }: { tab: Tab; visible: boolean }) {
  const { update, reconnect } = useSessions()
  const host = useVault((s) => s.hosts.find((h) => h.id === tab.hostId))
  const screenRef = useRef<HTMLDivElement>(null)
  const rfbRef = useRef<RFB | null>(null)
  const sidRef = useRef(tab.sessionId)
  // Credentials typed into the overlay, used for the next request.
  const credsRef = useRef<Credentials | null>(null)
  // The viewer is waiting for credentials right now.
  const waitingRef = useRef(false)
  // Why the connection failed, if the server told us before disconnecting.
  const failRef = useRef('')
  const [scale, setScale] = useState(true)
  const [viewOnly, setViewOnly] = useState(false)
  const [full, setFull] = useState(false)
  const wasFullscreen = useRef(false)
  const connected = tab.state === 'connected'

  // Connect on each new attempt.
  useEffect(() => {
    if (tab.state !== 'connecting') return
    const sid = tab.sessionId
    sidRef.current = sid
    failRef.current = ''
    waitingRef.current = false
    let rfb: RFB | null = null
    let cancelled = false
    const current = () => !cancelled && sidRef.current === sid

    const sendCredentials = async (types: string[]) => {
      const creds = credsRef.current
      const h = useVault.getState().hosts.find((x) => x.id === tab.hostId)
      let password = creds?.password ?? ''
      if (!password && h?.hasPassword) {
        try {
          // Fetched only now, when the server asks; never kept around.
          password = await api.revealHostPassword(tab.hostId)
        } catch {
          password = ''
        }
      }
      if (!current()) return
      if (!password) {
        waitingRef.current = true
        update(tab.key, { state: 'auth', error: undefined, errorCode: undefined })
        return
      }
      const c: { username?: string; password: string } = { password }
      if (types.includes('username')) c.username = h?.username ?? ''
      rfb?.sendCredentials(c)
    }

    // noVNC is loaded on first use so it stays out of the app's startup.
    Promise.all([api.openRemoteDesktop(sid, tab.hostId), import('@novnc/novnc')])
      .then(([info, { default: RFBClient }]) => {
        if (!current() || !screenRef.current) {
          void api.closeRemoteDesktop(sid)
          return
        }
        update(tab.key, { remote: info.via ? `${info.target} via ${info.via}` : info.target })
        rfb = new RFBClient(screenRef.current, info.url!, { shared: true })
        rfbRef.current = rfb
        rfb.scaleViewport = scale
        rfb.viewOnly = viewOnly
        rfb.background = 'var(--background)'
        rfb.focusOnClick = true
        rfb.addEventListener('connect', () => {
          if (!current()) return
          const creds = credsRef.current
          credsRef.current = null
          waitingRef.current = false
          update(tab.key, { state: 'connected', error: undefined, errorCode: undefined })
          if (creds?.savePassword && host) {
            void api
              .saveHost({ host, password: creds.password, setPassword: true })
              .then(() => useVault.getState().loadHosts())
              .catch(() => {})
          }
          rfb?.focus()
        })
        rfb.addEventListener('credentialsrequired', (e) => {
          const types = ((e as CustomEvent).detail?.types as string[] | undefined) ?? ['password']
          void sendCredentials(types)
        })
        rfb.addEventListener('securityfailure', (e) => {
          const d = (e as CustomEvent).detail as { status?: number; reason?: string } | undefined
          failRef.current = d?.reason || 'The server rejected the password.'
        })
        rfb.addEventListener('clipboard', (e) => {
          const text = (e as CustomEvent).detail?.text as string | undefined
          if (text) void copyText(text)
        })
        rfb.addEventListener('disconnect', (e) => {
          if (!current()) return
          rfbRef.current = null
          const clean = !!(e as CustomEvent).detail?.clean
          void api.closeRemoteDesktop(sid)
          if (failRef.current) {
            // Wrong password: ask again, then reconnect with what's typed.
            credsRef.current = null
            update(tab.key, { state: 'auth', error: failRef.current, errorCode: 'auth_failed' })
            return
          }
          const st = useSessions.getState().tabs.find((t) => t.key === tab.key)?.state
          if (st === 'error' || st === 'closed') return
          update(tab.key, clean && st === 'connected' ? { state: 'closed', error: 'The remote desktop disconnected.' } : { state: 'error', error: 'The connection to the VNC server was lost.' })
        })
      })
      .catch((err) => {
        if (!current()) return
        update(tab.key, { state: 'error', error: errorMessage(err), errorCode: errorCode(err) })
      })
    return () => {
      cancelled = true
      rfb?.disconnect()
      if (rfbRef.current === rfb) rfbRef.current = null
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [tab.sessionId])

  // Tunnel dropped, or Remvyn locked.
  useEffect(
    () =>
      onEvent<RemoteDesktopEnded>(events.REMOTE_ENDED, (e) => {
        if (e.id !== sidRef.current || !e.error) return
        rfbRef.current?.disconnect()
        update(tab.key, { state: 'error', error: e.error })
      }),
    [tab.key, update],
  )

  useEffect(() => {
    if (rfbRef.current) rfbRef.current.scaleViewport = scale
  }, [scale])
  useEffect(() => {
    if (rfbRef.current) rfbRef.current.viewOnly = viewOnly
  }, [viewOnly])
  useEffect(() => {
    if (visible && connected) rfbRef.current?.focus()
  }, [visible, connected])

  // Leave full screen when the tab is hidden or closed.
  const setFullscreen = async (on: boolean) => {
    if (on) {
      wasFullscreen.current = await WindowIsFullscreen().catch(() => false)
      if (!wasFullscreen.current) WindowFullscreen()
    } else if (!wasFullscreen.current) WindowUnfullscreen()
    setFull(on)
    setTimeout(() => rfbRef.current?.focus(), 50)
  }
  useEffect(() => {
    if (full && !visible) void setFullscreen(false)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [visible])
  useEffect(
    () => () => {
      if (full && !wasFullscreen.current) WindowUnfullscreen()
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [],
  )

  const paste = async () => {
    const text = await readClipboard()
    if (!text) {
      toast('Your clipboard has no text to send')
      return
    }
    rfbRef.current?.clipboardPasteFrom(text)
    toast.success('Clipboard sent', { description: 'Paste on the remote desktop as usual.' })
    rfbRef.current?.focus()
  }

  const submitCredentials = (c: Credentials) => {
    credsRef.current = c
    if (waitingRef.current && rfbRef.current) {
      // The viewer is still waiting for them on this connection.
      waitingRef.current = false
      update(tab.key, { state: 'connecting', error: undefined, errorCode: undefined })
      const h = useVault.getState().hosts.find((x) => x.id === tab.hostId)
      rfbRef.current.sendCredentials({ username: h?.username ?? '', password: c.password })
      return
    }
    reconnect(tab.key)
  }

  return (
    <div className={cn(visible ? 'flex flex-col' : 'hidden', full ? 'fixed inset-0 z-40 bg-background' : 'absolute inset-0')}>
      <div className="flex h-9 shrink-0 items-center gap-2 border-b px-3 text-xs">
        <span className="size-2 rounded-full" style={{ background: connected ? 'var(--success)' : tab.state === 'error' ? 'var(--danger)' : 'var(--muted-foreground)' }} />
        <span className="font-medium">{tab.title}</span>
        <span className="truncate font-mono text-muted-foreground">{tab.remote}</span>
        <span className="text-muted-foreground">· VNC</span>
        <div className="ml-auto flex items-center gap-0.5">
          <ToolButton label={scale ? 'Show at actual size (1:1)' : 'Scale to fit the window'} onClick={() => setScale((v) => !v)} disabled={!connected}>
            {scale ? <Maximize2Icon /> : <Minimize2Icon />}
          </ToolButton>
          <ToolButton label={viewOnly ? 'View only: on. Click to allow control' : 'View only (ignore your mouse and keyboard)'} onClick={() => setViewOnly((v) => !v)} active={viewOnly} disabled={!connected}>
            <EyeIcon />
          </ToolButton>
          <ToolButton label="Send Ctrl+Alt+Del" onClick={() => rfbRef.current?.sendCtrlAltDel()} disabled={!connected || viewOnly}>
            <KeyboardIcon />
          </ToolButton>
          <ToolButton label="Send clipboard text to the remote clipboard" onClick={() => void paste()} disabled={!connected || viewOnly}>
            <ClipboardPasteIcon />
          </ToolButton>
          <ToolButton label={full ? 'Exit full screen' : 'Full screen'} onClick={() => void setFullscreen(!full)}>
            {full ? <MinimizeIcon /> : <MaximizeIcon />}
          </ToolButton>
          <ToolButton label="Reconnect" onClick={() => reconnect(tab.key)} disabled={tab.state === 'connecting'}>
            <RotateCwIcon />
          </ToolButton>
        </div>
      </div>
      <div className="relative min-h-0 flex-1">
        <div ref={screenRef} className="absolute inset-0" />
        <ConnectOverlay
          tab={tab}
          host={host}
          onSubmitCredentials={submitCredentials}
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

function ToolButton({ label, onClick, disabled, active, children }: { label: string; onClick: () => void; disabled?: boolean; active?: boolean; children: React.ReactNode }) {
  return (
    <Tooltip content={label}>
      <Button variant="ghost" size="icon-xs" aria-label={label} aria-pressed={active} onClick={onClick} disabled={disabled} className={cn(active && 'bg-accent text-primary')}>
        {children}
      </Button>
    </Tooltip>
  )
}
