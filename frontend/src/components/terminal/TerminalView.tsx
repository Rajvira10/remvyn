import '@xterm/xterm/css/xterm.css'
import { FitAddon } from '@xterm/addon-fit'
import { SearchAddon } from '@xterm/addon-search'
import { Unicode11Addon } from '@xterm/addon-unicode11'
import { WebLinksAddon } from '@xterm/addon-web-links'
import { WebglAddon } from '@xterm/addon-webgl'
import { Terminal, type ITheme } from '@xterm/xterm'
import { ActivityIcon, ChevronDownIcon, ChevronUpIcon, CodeIcon, Columns2Icon, FolderIcon, RotateCwIcon, Rows2Icon, SearchIcon, XIcon } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { toast } from 'sonner'
import { BrowserOpenURL } from '../../../wailsjs/runtime/runtime'
import { AppearancePicker, changeFontSize, cycleTerminalTheme } from '@/components/terminal/AppearancePicker'
import { MonitorPanel } from '@/components/monitor/MonitorPanel'
import { ConnectOverlay, type Credentials } from '@/components/session/ConnectOverlay'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Tooltip } from '@/components/ui/tooltip'
import { matchesCombo } from '@/hooks/useHotkeys'
import { api, copyText, errorCode, errorMessage, events, onEvent, readClipboard } from '@/lib/api'
import type { SessionExit } from '@/lib/types'
import { terminalTheme } from '@/lib/terminalThemes'
import { appKey, cn, isMac, uid } from '@/lib/utils'
import { isLocked } from '@/stores/lock'
import { useSessions, type Rect, type Tab } from '@/stores/sessions'
import { useSettings } from '@/stores/settings'
import { useUI } from '@/stores/ui'
import { useVault } from '@/stores/vault'

function decode(b64: string): Uint8Array {
  const bin = atob(b64)
  const out = new Uint8Array(bin.length)
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i)
  return out
}

/** App-level shortcuts that must never be sent to the remote shell. */
const RESERVED = ['app+t', 'app+w', 'app+k', 'app+f', 'app+,', 'app+[', 'app+]', 'app+shift+[', 'app+shift+]',
  'app+1', 'app+2', 'app+3', 'app+4', 'app+5', 'app+6', 'app+7', 'app+8', 'app+9', 'app+0', 'app+/', 'app+i',
  'app+alt+[', 'app+alt+]', 'app+=', 'app+-', 'app+l', 'app+d', 'app+alt+d', 'app+alt+t',
  'app+alt+arrowleft', 'app+alt+arrowright', 'app+alt+arrowup', 'app+alt+arrowdown']

/**
 * One terminal session (SSH or local). In a split tab, rect places it within
 * the tab area and only the focused pane takes keyboard shortcuts.
 */
export function TerminalView({ tab, visible, rect, focused = true, split = false }: { tab: Tab; visible: boolean; rect?: Rect; focused?: boolean; split?: boolean }) {
  const local = tab.kind === 'local'
  const active = visible && focused
  const settings = useSettings((s) => s.settings)
  const host = useVault((s) => s.hosts.find((h) => h.id === tab.hostId))
  const snippets = useVault((s) => s.snippets)
  const monitorOpen = useUI((s) => s.monitorOpen)
  const { update, open } = useSessions()
  const setPage = useUI((s) => s.setPage)

  const containerRef = useRef<HTMLDivElement>(null)
  const termRef = useRef<Terminal | null>(null)
  const fitRef = useRef<FitAddon | null>(null)
  const searchRef = useRef<SearchAddon | null>(null)
  const sidRef = useRef(tab.sessionId)
  const connectedRef = useRef(false)
  const credsRef = useRef<Credentials | null>(null)
  const [searchOpen, setSearchOpen] = useState(false)
  const [query, setQuery] = useState('')
  const [preview, setPreview] = useState<ITheme | null>(null)
  const searchInput = useRef<HTMLInputElement>(null)

  const send = (data: string) => {
    if (connectedRef.current) void api.writeTerminal(sidRef.current, data).catch(() => {})
  }

  // Create the terminal once.
  useEffect(() => {
    const s = useSettings.getState().settings
    const term = new Terminal({
      fontFamily: s?.terminalFontFamily,
      fontSize: s?.terminalFontSize ?? 13,
      cursorStyle: s?.cursorStyle ?? 'block',
      cursorBlink: s?.cursorBlink ?? true,
      scrollback: s?.scrollback ?? 10000,
      theme: terminalTheme(s?.terminalTheme ?? 'flux').theme,
      allowProposedApi: true,
      macOptionIsMeta: true,
      rightClickSelectsWord: true,
      drawBoldTextInBrightColors: true,
      lineHeight: 1.15,
    })
    const fit = new FitAddon()
    const search = new SearchAddon()
    term.loadAddon(fit)
    term.loadAddon(search)
    term.loadAddon(new WebLinksAddon((_e, uri) => BrowserOpenURL(uri)))
    const unicode = new Unicode11Addon()
    term.loadAddon(unicode)
    term.unicode.activeVersion = '11'
    term.open(containerRef.current!)
    try {
      const webgl = new WebglAddon()
      webgl.onContextLoss(() => webgl.dispose())
      term.loadAddon(webgl)
    } catch {
      // DOM renderer fallback.
    }

    term.attachCustomKeyEventHandler((e) => {
      // Nothing reaches the remote shell while Remvyn is locked.
      if (isLocked()) return false
      if (e.type !== 'keydown') return true
      // Copy: ⌘C on macOS; Ctrl+Shift+C elsewhere (plain Ctrl+C is SIGINT).
      const copyCombo = isMac ? e.metaKey && !e.ctrlKey && e.key.toLowerCase() === 'c' : e.ctrlKey && e.shiftKey && e.code === 'KeyC'
      if (copyCombo) {
        if (term.hasSelection()) {
          e.preventDefault()
          void copyText(term.getSelection())
        }
        return false
      }
      const pasteCombo = isMac ? e.metaKey && e.key.toLowerCase() === 'v' : e.ctrlKey && e.shiftKey && e.code === 'KeyV'
      if (pasteCombo) {
        e.preventDefault()
        void readClipboard().then((text) => text && term.paste(text))
        return false
      }
      if (RESERVED.some((c) => matchesCombo(e, c))) return false
      return true
    })
    term.onData((d) => send(d))
    term.onBinary((d) => send(d))
    term.onResize(({ cols, rows }) => {
      if (connectedRef.current) void api.resizeTerminal(sidRef.current, cols, rows).catch(() => {})
    })
    term.onSelectionChange(() => {
      if (useSettings.getState().settings?.copyOnSelect && term.hasSelection()) void copyText(term.getSelection())
    })

    termRef.current = term
    fitRef.current = fit
    searchRef.current = search

    const ro = new ResizeObserver(() => {
      const el = containerRef.current
      if (el && el.clientWidth > 0 && el.clientHeight > 0) {
        try {
          fit.fit()
        } catch {
          /* not laid out yet */
        }
      }
    })
    ro.observe(containerRef.current!)
    return () => {
      ro.disconnect()
      term.dispose()
      termRef.current = null
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  // Apply settings changes live.
  useEffect(() => {
    const term = termRef.current
    if (!term || !settings) return
    term.options.fontFamily = settings.terminalFontFamily
    term.options.fontSize = settings.terminalFontSize
    term.options.cursorStyle = settings.cursorStyle
    term.options.cursorBlink = settings.cursorBlink
    term.options.scrollback = settings.scrollback
    try {
      fitRef.current?.fit()
    } catch {
      /* hidden */
    }
  }, [settings])

  // Color scheme, including a temporary preview from the style picker.
  useEffect(() => {
    const term = termRef.current
    if (term && settings) term.options.theme = preview ?? terminalTheme(settings.terminalTheme).theme
  }, [settings, preview])

  // Refit when the tab becomes visible, and focus the active pane.
  useEffect(() => {
    if (!visible) return
    const id = requestAnimationFrame(() => {
      try {
        fitRef.current?.fit()
      } catch {
        /* ignore */
      }
      if (active && !searchOpen) termRef.current?.focus()
    })
    return () => cancelAnimationFrame(id)
  }, [visible, active, searchOpen])

  // Connect whenever a new attempt (session id) starts.
  useEffect(() => {
    const term = termRef.current
    if (!term || tab.state !== 'connecting') return
    const sid = tab.sessionId
    sidRef.current = sid
    connectedRef.current = false
    if (tab.attempt > 0) term.write('\r\n\x1b[2m── reconnecting ──\x1b[0m\r\n')

    const offData = onEvent<string>(events.termData(sid), (b64) => term.write(decode(b64)))
    const offExit = onEvent<SessionExit>(events.termExit(sid), (x) => {
      connectedRef.current = false
      term.write(`\r\n\x1b[2m── session ended${x.exitCode >= 0 ? ` (status ${x.exitCode})` : ''} ──\x1b[0m\r\n`)
      update(tab.key, { state: 'closed', exitCode: x.exitCode, error: x.error })
    })

    try {
      fitRef.current?.fit()
    } catch {
      /* hidden */
    }
    const creds = credsRef.current
    credsRef.current = null
    const opts = {
      password: creds?.password ?? '',
      savePassword: creds?.savePassword ?? false,
      passphrase: creds?.passphrase ?? '',
      cols: term.cols,
      rows: term.rows,
    }
    ;(local ? api.openLocalTerminal(sid, opts) : api.openTerminal(sid, tab.hostId, opts))
      .then((info) => {
        if (sidRef.current !== sid) return
        connectedRef.current = true
        update(tab.key, { state: 'connected', remote: info.remote })
        // The PTY may have been sized before the fit settled.
        void api.resizeTerminal(sid, term.cols, term.rows).catch(() => {})
        if (creds?.savePassword) void useVault.getState().loadHosts()
        term.focus()
      })
      .catch((err) => {
        if (sidRef.current !== sid) return
        const code = errorCode(err)
        const auth = code === 'password_required' || code === 'passphrase_required' || (code === 'auth_failed' && host?.authMethod === 'password')
        if (code === 'cancelled') update(tab.key, { state: 'error', error: 'Connection cancelled.', errorCode: code })
        else update(tab.key, { state: auth ? 'auth' : 'error', error: errorMessage(err), errorCode: code })
      })

    return () => {
      offData()
      offExit()
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [tab.sessionId])

  const splitFrom = (dir: 'row' | 'col') => {
    const s = useSessions.getState()
    s.focusPane(tab.key)
    s.split(dir)
  }

  const submitCredentials = (c: Credentials) => {
    credsRef.current = c
    update(tab.key, { state: 'connecting', sessionId: uid(), error: undefined })
  }

  const runSnippet = (command: string) => {
    if (!connectedRef.current) {
      toast.error('This session is not connected')
      return
    }
    send(command.endsWith('\n') ? command : command + '\r')
    termRef.current?.focus()
  }

  const doSearch = (dir: 'next' | 'prev') => {
    if (!query) return
    const opts = { caseSensitive: false, incremental: dir === 'next' }
    const found = dir === 'next' ? searchRef.current?.findNext(query, opts) : searchRef.current?.findPrevious(query, opts)
    if (!found) toast('No matches', { duration: 1200 })
  }

  // Shortcuts local to the visible, focused terminal.
  useEffect(() => {
    if (!active) return
    const onKey = (e: KeyboardEvent) => {
      if (isLocked()) return
      if (matchesCombo(e, 'app+f')) {
        e.preventDefault()
        setSearchOpen(true)
        setTimeout(() => searchInput.current?.select(), 0)
      } else if (matchesCombo(e, 'app+k')) {
        e.preventDefault()
        termRef.current?.clear()
      } else if (matchesCombo(e, 'app+i')) {
        e.preventDefault()
        useUI.getState().setMonitorOpen(!useUI.getState().monitorOpen)
      } else if (matchesCombo(e, 'app+alt+]') || matchesCombo(e, 'app+alt+[')) {
        e.preventDefault()
        cycleTerminalTheme(e.code === 'BracketRight' ? 1 : -1)
      } else if (matchesCombo(e, 'app+=') || matchesCombo(e, 'app+-')) {
        e.preventDefault()
        changeFontSize(e.code === 'Equal' || e.key === '=' ? 1 : -1)
      }
    }
    window.addEventListener('keydown', onKey, true)
    return () => window.removeEventListener('keydown', onKey, true)
  }, [active])

  const theme = terminalTheme(settings?.terminalTheme ?? 'flux')

  const place = rect ? { left: `${rect.x * 100}%`, top: `${rect.y * 100}%`, width: `${rect.w * 100}%`, height: `${rect.h * 100}%` } : undefined

  return (
    <div
      className={cn('absolute', !rect && 'inset-0', visible ? 'flex flex-col' : 'hidden')}
      style={place}
      onMouseDownCapture={() => split && useSessions.getState().focusPane(tab.key)}
    >
      <div className={cn('flex h-9 shrink-0 items-center gap-2 border-b px-3 text-xs', split && focused && 'bg-accent/50 shadow-[inset_0_2px_0_var(--primary)]')}>
        <span className="size-2 rounded-full" style={{ background: tab.state === 'connected' ? 'var(--success)' : tab.state === 'error' ? 'var(--danger)' : 'var(--muted-foreground)' }} />
        <span className="font-medium">{tab.title}</span>
        <span className="truncate font-mono text-muted-foreground">{tab.remote}</span>
        <div className="ml-auto flex items-center gap-0.5">
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button variant="ghost" size="xs" disabled={tab.state !== 'connected'}>
                <CodeIcon /> Snippets
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" className="w-72">
              <DropdownMenuLabel>Run snippet</DropdownMenuLabel>
              {snippets.length === 0 && <div className="px-2 py-1.5 text-xs text-muted-foreground">No snippets yet.</div>}
              {snippets.map((s) => (
                <DropdownMenuItem key={s.id} onSelect={() => runSnippet(s.command)} className="flex-col items-start gap-0">
                  <span>{s.name}</span>
                  <span className="w-full truncate font-mono text-[11px] text-muted-foreground">{s.command}</span>
                </DropdownMenuItem>
              ))}
              <DropdownMenuSeparator />
              <DropdownMenuItem
                onSelect={() => {
                  useSessions.getState().activate('vault')
                  setPage('snippets')
                }}
              >
                Manage snippets…
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
          {!local && (
            <Tooltip content={`Server metrics (${appKey}I)`}>
              <Button
                variant={monitorOpen ? 'secondary' : 'ghost'}
                size="xs"
                aria-pressed={monitorOpen}
                onClick={() => useUI.getState().setMonitorOpen(!monitorOpen)}
              >
                <ActivityIcon /> Metrics
              </Button>
            </Tooltip>
          )}
          <AppearancePicker onPreview={setPreview} />
          <Tooltip content={`Find (${appKey}F)`}>
            <Button variant="ghost" size="icon-xs" aria-label="Find" onClick={() => { setSearchOpen((o) => !o); setTimeout(() => searchInput.current?.focus(), 0) }}>
              <SearchIcon />
            </Button>
          </Tooltip>
          {!local && (
            <Tooltip content="Open SFTP for this host">
              <Button variant="ghost" size="icon-xs" aria-label="Open SFTP" onClick={() => open(tab.hostId, 'sftp')}>
                <FolderIcon />
              </Button>
            </Tooltip>
          )}
          <Tooltip content={`Split right (${appKey}D)`}>
            <Button variant="ghost" size="icon-xs" aria-label="Split right" onClick={() => splitFrom('row')}>
              <Columns2Icon />
            </Button>
          </Tooltip>
          <Tooltip content={`Split down (${appKey}${isMac ? '⌥' : 'Alt+'}D)`}>
            <Button variant="ghost" size="icon-xs" aria-label="Split down" onClick={() => splitFrom('col')}>
              <Rows2Icon />
            </Button>
          </Tooltip>
          <Tooltip content={local ? 'Restart shell' : 'Reconnect'}>
            <Button variant="ghost" size="icon-xs" aria-label={local ? 'Restart shell' : 'Reconnect'} onClick={() => useSessions.getState().reconnect(tab.key)}>
              <RotateCwIcon />
            </Button>
          </Tooltip>
          {split && (
            <Tooltip content={`Close pane (${appKey}W)`}>
              <Button variant="ghost" size="icon-xs" aria-label="Close pane" onClick={() => useSessions.getState().closePane(tab.key)}>
                <XIcon />
              </Button>
            </Tooltip>
          )}
        </div>
      </div>
      {searchOpen && (
        <div className="flex h-9 shrink-0 items-center gap-1 border-b bg-muted/30 px-3">
          <SearchIcon className="size-3.5 text-muted-foreground" />
          <input
            ref={searchInput}
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter') doSearch(e.shiftKey ? 'prev' : 'next')
              if (e.key === 'Escape') {
                setSearchOpen(false)
                searchRef.current?.clearDecorations()
                termRef.current?.focus()
              }
            }}
            placeholder="Find in terminal"
            spellCheck={false}
            className="h-7 flex-1 bg-transparent text-xs outline-none"
          />
          <Button variant="ghost" size="icon-xs" onClick={() => doSearch('prev')} aria-label="Previous match">
            <ChevronUpIcon />
          </Button>
          <Button variant="ghost" size="icon-xs" onClick={() => doSearch('next')} aria-label="Next match">
            <ChevronDownIcon />
          </Button>
          <Button variant="ghost" size="icon-xs" onClick={() => { setSearchOpen(false); searchRef.current?.clearDecorations() }} aria-label="Close find">
            <XIcon />
          </Button>
        </div>
      )}
      <div className="flex min-h-0 flex-1">
        <div className="relative min-w-0 flex-1" style={{ background: (preview ?? theme.theme).background }}>
          <div ref={containerRef} className="absolute inset-0 m-2" />
          <ConnectOverlay
            tab={tab}
            host={host}
            onSubmitCredentials={submitCredentials}
            onCancel={() => void api.cancelConnect(tab.sessionId)}
          />
        </div>
        {monitorOpen && !local && (
          <aside className="w-[320px] shrink-0 overflow-y-auto border-l bg-sidebar" aria-label="Server metrics">
            {tab.state === 'connected' ? (
              <MonitorPanel hostId={tab.hostId} active={visible} />
            ) : (
              <div className="p-4 text-xs text-muted-foreground">Metrics appear once the session is connected.</div>
            )}
          </aside>
        )}
      </div>
    </div>
  )
}
