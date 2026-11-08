import { CheckIcon, ChevronsUpDownIcon, SettingsIcon } from 'lucide-react'
import { useEffect, useState } from 'react'
import { toast } from 'sonner'
import { ConfirmDialog } from '@/components/common/ConfirmDialog'
import { Logo } from '@/components/common/Logo'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuLabel, DropdownMenuSeparator, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { api, errorMessage } from '@/lib/api'
import type { VaultProfile } from '@/lib/types'
import { useLock } from '@/stores/lock'
import { useSessions } from '@/stores/sessions'
import { reopenPage, useUI } from '@/stores/ui'
import { useVault } from '@/stores/vault'
import { useVaults } from '@/stores/vaults'

/** The open vault's name above the sidebar; click to switch to another. */
export function VaultSwitcher() {
  const list = useVaults((s) => s.list)
  const repo = useLock((s) => s.sync?.repo)
  const [confirm, setConfirm] = useState<{ vault: VaultProfile; open: number; forwards: number } | null>(null)
  // Reload when sync connects or disconnects, so a vault's repository shows.
  useEffect(() => void useVaults.getState().load(), [repo])

  const current = list.find((v) => v.current)

  const open = async (v: VaultProfile) => {
    try {
      await api.switchVault(v.id)
      reopenPage(useUI.getState().page)
      window.location.reload()
    } catch (e) {
      toast.error('Could not open that vault', { description: errorMessage(e) })
    }
  }

  const choose = (v: VaultProfile) => {
    const open_ = useSessions.getState().tabs.filter((t) => t.state === 'connected').length
    const forwards = Object.values(useVault.getState().statuses).filter((x) => x.state === 'running').length
    if (open_ || forwards) setConfirm({ vault: v, open: open_, forwards })
    else void open(v)
  }

  const manage = () => {
    useSessions.getState().activate('vault')
    useUI.getState().setPage('settings')
  }

  const closing = confirm ? [confirm.open && `${confirm.open} open session${confirm.open === 1 ? '' : 's'}`, confirm.forwards && `${confirm.forwards} running forward${confirm.forwards === 1 ? '' : 's'}`].filter(Boolean).join(' and ') : ''

  return (
    <>
      <DropdownMenu onOpenChange={(o) => o && void useVaults.getState().load()}>
        <DropdownMenuTrigger asChild>
          <button className="no-drag flex h-8 max-w-full min-w-0 items-center gap-2 rounded-md px-2 hover:bg-accent/50" aria-label={`Vault: ${current?.name ?? 'Remvyn'}. Switch vault`}>
            <div className="flex size-5 shrink-0 items-center justify-center rounded-md bg-primary/15">
              <Logo className="size-3 text-primary" />
            </div>
            <span className="min-w-0 truncate text-[13px] font-semibold tracking-tight">{current?.name ?? 'Remvyn'}</span>
            <ChevronsUpDownIcon className="size-3 shrink-0 text-muted-foreground" />
          </button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="start" className="w-64">
          <DropdownMenuLabel className="text-xs text-muted-foreground">Vaults</DropdownMenuLabel>
          {list.map((v) => (
            <DropdownMenuItem key={v.id} disabled={v.current} onSelect={() => choose(v)} className="data-[disabled]:opacity-100">
              <div className="flex min-w-0 flex-1 flex-col">
                <span className="truncate font-medium">{v.name}</span>
                <span className="truncate font-mono text-[11px] text-muted-foreground">{v.repo || 'Not synced'}</span>
              </div>
              {v.current && <CheckIcon className="text-primary" />}
            </DropdownMenuItem>
          ))}
          <DropdownMenuSeparator />
          <DropdownMenuItem onSelect={manage}>
            <SettingsIcon /> New or manage vaults…
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
      <ConfirmDialog
        open={!!confirm}
        onOpenChange={(o) => !o && setConfirm(null)}
        title={`Open “${confirm?.vault.name}”?`}
        description={`Switching vaults closes ${closing}.`}
        confirmLabel="Switch vault"
        onConfirm={() => {
          if (confirm) void open(confirm.vault)
        }}
      />
    </>
  )
}
