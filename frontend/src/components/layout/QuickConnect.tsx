import { Dialog as DialogPrimitive } from 'radix-ui'
import { FolderOpenIcon, PlusIcon, SearchIcon, TerminalIcon } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { HostAvatar } from '@/components/hosts/HostAvatar'
import { Kbd } from '@/components/ui/kbd'
import { cn, isMac } from '@/lib/utils'
import { useSessions } from '@/stores/sessions'
import { useUI } from '@/stores/ui'
import { useVault } from '@/stores/vault'

/** ⌘T palette: fuzzy-find a host and open a terminal (Enter) or SFTP (⌘Enter). */
export function QuickConnect() {
  const { paletteOpen, setPaletteOpen, editHost } = useUI()
  const hosts = useVault((s) => s.hosts)
  const open = useSessions((s) => s.open)
  const activate = useSessions((s) => s.activate)
  const [q, setQ] = useState('')
  const [idx, setIdx] = useState(0)

  useEffect(() => {
    if (paletteOpen) {
      setQ('')
      setIdx(0)
      void useVault.getState().loadHosts()
    }
  }, [paletteOpen])

  const results = useMemo(() => {
    const query = q.trim().toLowerCase()
    const scored = hosts
      .map((h) => {
        const hay = `${h.label} ${h.username}@${h.hostname}`.toLowerCase()
        let score = -1
        if (!query) score = h.lastConnectedAt
        else if (h.label.toLowerCase().startsWith(query)) score = 3e15
        else if (hay.includes(query)) score = 2e15
        else if (subsequence(query, hay)) score = 1e15
        return { h, score }
      })
      .filter((x) => x.score >= 0)
    scored.sort((a, b) => b.score - a.score || a.h.label.localeCompare(b.h.label))
    return scored.map((x) => x.h).slice(0, 50)
  }, [hosts, q])

  const choose = (i: number, sftp: boolean) => {
    const h = results[i]
    if (!h) return
    setPaletteOpen(false)
    open(h.id, sftp ? 'sftp' : 'terminal')
  }

  return (
    <DialogPrimitive.Root open={paletteOpen} onOpenChange={setPaletteOpen}>
      <DialogPrimitive.Portal>
        <DialogPrimitive.Overlay className="fixed inset-0 z-50 bg-black/40" />
        <DialogPrimitive.Content className="fixed inset-x-0 top-[18%] z-50 mx-auto w-full max-w-lg overflow-hidden rounded-xl border bg-popover shadow-2xl" aria-describedby={undefined}>
          <DialogPrimitive.Title className="sr-only">Quick connect</DialogPrimitive.Title>
          <div className="flex items-center gap-2 border-b px-3">
            <SearchIcon className="size-4 text-muted-foreground" />
            <input
              autoFocus
              value={q}
              onChange={(e) => {
                setQ(e.target.value)
                setIdx(0)
              }}
              onKeyDown={(e) => {
                if (e.key === 'ArrowDown') {
                  e.preventDefault()
                  setIdx((i) => Math.min(results.length - 1, i + 1))
                } else if (e.key === 'ArrowUp') {
                  e.preventDefault()
                  setIdx((i) => Math.max(0, i - 1))
                } else if (e.key === 'Enter') {
                  e.preventDefault()
                  choose(idx, e.metaKey || e.ctrlKey)
                }
              }}
              placeholder="Connect to a host…"
              spellCheck={false}
              className="h-12 flex-1 bg-transparent text-[14px] outline-none"
            />
          </div>
          <div className="max-h-80 overflow-y-auto p-1.5">
            {results.length === 0 ? (
              <div className="flex flex-col items-center gap-2 p-6 text-center text-[13px] text-muted-foreground">
                {hosts.length === 0 ? 'No hosts yet.' : 'No matching hosts.'}
                <button
                  className="flex items-center gap-1 text-primary hover:underline"
                  onClick={() => {
                    setPaletteOpen(false)
                    activate('vault')
                    editHost('')
                  }}
                >
                  <PlusIcon className="size-3.5" /> New host
                </button>
              </div>
            ) : (
              results.map((h, i) => (
                <div
                  key={h.id}
                  onMouseMove={() => setIdx(i)}
                  onClick={() => choose(i, false)}
                  className={cn('flex cursor-default items-center gap-3 rounded-lg px-2.5 py-2', i === idx && 'bg-accent')}
                >
                  <HostAvatar label={h.label} color={h.color} className="size-7 text-[10px]" />
                  <div className="min-w-0 flex-1">
                    <div className="truncate text-[13px] font-medium">{h.label}</div>
                    <div className="truncate font-mono text-[11px] text-muted-foreground">
                      {h.username ? `${h.username}@` : ''}
                      {h.hostname}
                      {h.port !== 22 ? `:${h.port}` : ''}
                    </div>
                  </div>
                  {i === idx && (
                    <div className="flex items-center gap-1 text-[11px] text-muted-foreground">
                      <TerminalIcon className="size-3" /> <Kbd>↵</Kbd>
                      <FolderOpenIcon className="ml-2 size-3" /> <Kbd>{isMac ? '⌘' : 'Ctrl'}↵</Kbd>
                    </div>
                  )}
                </div>
              ))
            )}
          </div>
        </DialogPrimitive.Content>
      </DialogPrimitive.Portal>
    </DialogPrimitive.Root>
  )
}

function subsequence(needle: string, hay: string): boolean {
  let i = 0
  for (const ch of hay) if (ch === needle[i]) i++
  return i === needle.length
}
