import { ActivityIcon, BotIcon, CableIcon, CodeIcon, KeyboardIcon, KeyRoundIcon, LockKeyholeIcon, ServerIcon, SettingsIcon, ShieldCheckIcon } from 'lucide-react'
import { useShallow } from 'zustand/react/shallow'
import { cn } from '@/lib/utils'
import { useUI, type VaultPage } from '@/stores/ui'
import { useAgent } from '@/stores/agent'
import { useMonitor } from '@/stores/monitor'
import { usePasswords } from '@/stores/passwords'
import { useVault } from '@/stores/vault'

export function VaultSidebar() {
  const { page, setPage, setShortcutsOpen } = useUI()
  const counts = useVault(useShallow((s) => ({
    hosts: s.hosts.length,
    keychain: s.keys.length,
    snippets: s.snippets.length,
    forwarding: Object.values(s.statuses).filter((x) => x.state === 'running').length,
    knownhosts: s.knownHosts.length,
  })))
  const passwords = usePasswords((s) => s.items.length)
  const openAlerts = useMonitor((s) => Object.keys(s.openAlerts).length)
  const agentRunning = useAgent((s) => s.status?.running ?? 0)
  const items: { id: VaultPage; label: string; icon: typeof ServerIcon; count?: number; live?: boolean }[] = [
    { id: 'hosts', label: 'Hosts', icon: ServerIcon, count: counts.hosts },
    { id: 'monitoring', label: 'Monitoring', icon: ActivityIcon },
    { id: 'passwords', label: 'Passwords', icon: LockKeyholeIcon, count: passwords },
    { id: 'keychain', label: 'Keychain', icon: KeyRoundIcon, count: counts.keychain },
    { id: 'forwarding', label: 'Port forwarding', icon: CableIcon, count: counts.forwarding, live: true },
    { id: 'snippets', label: 'Snippets', icon: CodeIcon, count: counts.snippets },
    { id: 'knownhosts', label: 'Known hosts', icon: ShieldCheckIcon, count: counts.knownhosts },
    { id: 'agents', label: 'AI agents', icon: BotIcon, count: agentRunning, live: true },
  ]
  return (
    <nav className="flex w-[200px] shrink-0 flex-col border-r bg-sidebar">
      <div className="flex flex-col gap-0.5 px-2 pt-3">
        {items.map((it) => (
          <button
            key={it.id}
            onClick={() => setPage(it.id)}
            className={cn(
              'flex h-8 items-center gap-2.5 rounded-md px-2.5 text-[13px] transition-colors',
              page === it.id ? 'bg-accent font-medium text-foreground' : 'text-muted-foreground hover:bg-accent/50 hover:text-foreground',
            )}
          >
            <it.icon className={cn('size-4', page === it.id && 'text-primary')} />
            <span className="flex-1 text-left">{it.label}</span>
            {it.id === 'monitoring' && openAlerts > 0 ? (
              <span className="rounded-full bg-danger px-1.5 text-[10px] leading-4 font-medium text-white" aria-label={`${openAlerts} open alerts`}>
                {openAlerts}
              </span>
            ) : it.count ? (
              <span className={cn('text-[11px] tabular-nums', it.live ? 'rounded bg-success/15 px-1 text-success' : 'text-muted-foreground')}>{it.count}</span>
            ) : null}
          </button>
        ))}
      </div>
      <div className="mt-auto flex flex-col gap-0.5 p-2">
        <button
          onClick={() => setShortcutsOpen(true)}
          className="flex h-8 items-center gap-2.5 rounded-md px-2.5 text-[13px] text-muted-foreground hover:bg-accent/50 hover:text-foreground"
        >
          <KeyboardIcon className="size-4" /> Shortcuts
        </button>
        <button
          onClick={() => setPage('settings')}
          className={cn(
            'flex h-8 items-center gap-2.5 rounded-md px-2.5 text-[13px] transition-colors',
            page === 'settings' ? 'bg-accent font-medium text-foreground' : 'text-muted-foreground hover:bg-accent/50 hover:text-foreground',
          )}
        >
          <SettingsIcon className={cn('size-4', page === 'settings' && 'text-primary')} /> Settings
        </button>
      </div>
    </nav>
  )
}
