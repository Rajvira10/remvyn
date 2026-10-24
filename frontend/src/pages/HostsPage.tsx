import {
  CopyPlusIcon,
  FileDownIcon,
  FolderIcon,
  FolderOpenIcon,
  MoreHorizontalIcon,
  PencilIcon,
  PlusIcon,
  SearchIcon,
  ServerIcon,
  TerminalIcon,
  Trash2Icon,
} from 'lucide-react'
import { useMemo, useState } from 'react'
import { toast } from 'sonner'
import { ConfirmDialog } from '@/components/common/ConfirmDialog'
import { EmptyState } from '@/components/common/EmptyState'
import { PromptDialog } from '@/components/common/PromptDialog'
import { HostAvatar } from '@/components/hosts/HostAvatar'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Input } from '@/components/ui/input'
import { Tooltip } from '@/components/ui/tooltip'
import { api, errorCode, errorMessage } from '@/lib/api'
import type { Group, Host } from '@/lib/types'
import { relativeTime } from '@/lib/utils'
import { useSessions } from '@/stores/sessions'
import { useUI } from '@/stores/ui'
import { useVault } from '@/stores/vault'

export function HostsPage() {
  const { hosts, groups, keys, loadHosts } = useVault()
  const editHost = useUI((s) => s.editHost)
  const openSession = useSessions((s) => s.open)
  const [query, setQuery] = useState('')
  const [deleting, setDeleting] = useState<Host | null>(null)
  const [renamingGroup, setRenamingGroup] = useState<Group | null>(null)
  const [deletingGroup, setDeletingGroup] = useState<Group | null>(null)

  const q = query.trim().toLowerCase()
  const filtered = useMemo(
    () => hosts.filter((h) => !q || `${h.label} ${h.hostname} ${h.username}`.toLowerCase().includes(q)),
    [hosts, q],
  )
  const sections = useMemo(() => {
    const out: { group: Group | null; hosts: Host[] }[] = []
    for (const g of groups) {
      const hs = filtered.filter((h) => h.groupId === g.id)
      if (hs.length || !q) out.push({ group: g, hosts: hs })
    }
    const ungrouped = filtered.filter((h) => !h.groupId || !groups.some((g) => g.id === h.groupId))
    if (ungrouped.length) out.push({ group: null, hosts: ungrouped })
    return out
  }, [filtered, groups, q])

  const importConfig = async (pick = false) => {
    try {
      const res = pick ? await api.importSSHConfigFile() : await api.importSSHConfig()
      if (!res.path) return // picker cancelled
      await Promise.all([loadHosts(), useVault.getState().loadKeys()])
      const file = res.path.replace(/^\/(Users|home)\/[^/]+/, '~')
      if (res.imported === 0 && res.skipped === 0) toast(`No hosts found in ${file}`, { description: res.warnings.join(' ') || undefined })
      else
        toast.success(`Imported ${res.imported} host${res.imported === 1 ? '' : 's'}${res.keys ? ` and ${res.keys} key${res.keys === 1 ? '' : 's'}` : ''}`, {
          description: [res.skipped ? `${res.skipped} already existed.` : '', ...res.warnings].filter(Boolean).join(' '),
        })
    } catch (e) {
      if (errorCode(e) === 'no_ssh_config')
        toast('You don’t have an SSH config file', {
          description: 'Remvyn imports hosts from ~/.ssh/config, which doesn’t exist on this Mac. Choose a config file from elsewhere, or add hosts with New host.',
          action: { label: 'Choose file…', onClick: () => void importConfig(true) },
          duration: 10000,
        })
      else toast.error('Import failed', { description: errorMessage(e) })
    }
  }

  const duplicate = async (h: Host) => {
    try {
      await api.saveHost({ host: { ...h, id: '', label: `${h.label} copy` }, password: '', setPassword: false })
      await loadHosts()
    } catch (e) {
      toast.error(errorMessage(e))
    }
  }

  const keyName = (id: string) => keys.find((k) => k.id === id)?.name ?? 'missing key'

  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className="flex h-14 shrink-0 items-center gap-3 border-b px-5">
        <h1 className="text-[15px] font-semibold">Hosts</h1>
        <div className="relative ml-2 w-full max-w-xs">
          <SearchIcon className="pointer-events-none absolute top-1/2 left-2.5 size-3.5 -translate-y-1/2 text-muted-foreground" />
          <Input value={query} onChange={(e) => setQuery(e.target.value)} placeholder="Search hosts" className="h-8 pl-8" />
        </div>
        <div className="ml-auto flex items-center gap-2">
          <Tooltip content="Import hosts and keys from ~/.ssh/config">
            <Button variant="ghost" size="sm" onClick={() => void importConfig()}>
              <FileDownIcon /> Import SSH config
            </Button>
          </Tooltip>
          <Button size="sm" onClick={() => editHost('')}>
            <PlusIcon /> New host
          </Button>
        </div>
      </div>

      <div className="min-h-0 flex-1 overflow-auto p-5">
        {hosts.length === 0 ? (
          <EmptyState icon={ServerIcon} title="No hosts yet">
            Add a server to connect to it over SSH, or import your existing ~/.ssh/config.
            <div className="mt-4 flex justify-center gap-2">
              <Button variant="outline" size="sm" onClick={() => void importConfig()}>
                <FileDownIcon /> Import SSH config
              </Button>
              <Button size="sm" onClick={() => editHost('')}>
                <PlusIcon /> New host
              </Button>
            </div>
          </EmptyState>
        ) : filtered.length === 0 ? (
          <EmptyState icon={SearchIcon} title="No matching hosts" />
        ) : (
          <div className="flex flex-col gap-6">
            {sections.map(({ group, hosts: hs }) => (
              <section key={group?.id ?? 'ungrouped'}>
                <div className="mb-2 flex items-center gap-2">
                  {group ? <FolderIcon className="size-3.5 text-muted-foreground" /> : null}
                  <h2 className="text-[11.5px] font-semibold tracking-wide text-muted-foreground uppercase">{group?.name ?? 'Hosts'}</h2>
                  <span className="text-[11px] text-muted-foreground/70">{hs.length}</span>
                  {group && (
                    <DropdownMenu>
                      <DropdownMenuTrigger asChild>
                        <Button variant="ghost" size="icon-xs" aria-label="Group actions">
                          <MoreHorizontalIcon />
                        </Button>
                      </DropdownMenuTrigger>
                      <DropdownMenuContent align="start">
                        <DropdownMenuItem onSelect={() => setRenamingGroup(group)}>
                          <PencilIcon /> Rename group
                        </DropdownMenuItem>
                        <DropdownMenuItem variant="destructive" onSelect={() => setDeletingGroup(group)}>
                          <Trash2Icon /> Delete group
                        </DropdownMenuItem>
                      </DropdownMenuContent>
                    </DropdownMenu>
                  )}
                </div>
                {hs.length === 0 ? (
                  <div className="rounded-lg border border-dashed px-4 py-3 text-xs text-muted-foreground">No hosts in this group.</div>
                ) : (
                  <div className="grid grid-cols-[repeat(auto-fill,minmax(260px,1fr))] gap-3">
                    {hs.map((h) => (
                      <div
                        key={h.id}
                        onDoubleClick={() => openSession(h.id, 'terminal')}
                        className="group relative flex items-center gap-3 rounded-xl border bg-card p-3 transition-colors hover:border-primary/40 hover:bg-accent/40"
                      >
                        <HostAvatar label={h.label} color={h.color} />
                        <div className="min-w-0 flex-1">
                          <div className="truncate text-[13.5px] font-medium">{h.label}</div>
                          <div className="truncate font-mono text-[11.5px] text-muted-foreground">
                            {h.username ? `${h.username}@` : ''}
                            {h.hostname}
                            {h.port !== 22 ? `:${h.port}` : ''}
                          </div>
                          <div className="mt-1 flex items-center gap-1.5">
                            <Badge variant="outline" className="text-[10px]">
                              {h.authMethod === 'key' ? keyName(h.keyId) : h.authMethod === 'agent' ? 'agent' : h.hasPassword ? 'password saved' : 'password'}
                            </Badge>
                            <span className="text-[10.5px] text-muted-foreground/80">{relativeTime(h.lastConnectedAt)}</span>
                          </div>
                        </div>
                        <div className="flex flex-col gap-1 opacity-0 transition-opacity group-hover:opacity-100 focus-within:opacity-100">
                          <Tooltip content="Open terminal">
                            <Button size="icon-xs" onClick={() => openSession(h.id, 'terminal')} aria-label={`Connect to ${h.label}`}>
                              <TerminalIcon />
                            </Button>
                          </Tooltip>
                          <DropdownMenu>
                            <DropdownMenuTrigger asChild>
                              <Button variant="ghost" size="icon-xs" aria-label="Host actions">
                                <MoreHorizontalIcon />
                              </Button>
                            </DropdownMenuTrigger>
                            <DropdownMenuContent align="end">
                              <DropdownMenuItem onSelect={() => openSession(h.id, 'terminal')}>
                                <TerminalIcon /> Open terminal
                              </DropdownMenuItem>
                              <DropdownMenuItem onSelect={() => openSession(h.id, 'sftp')}>
                                <FolderOpenIcon /> Open SFTP
                              </DropdownMenuItem>
                              <DropdownMenuSeparator />
                              <DropdownMenuItem onSelect={() => editHost(h.id)}>
                                <PencilIcon /> Edit
                              </DropdownMenuItem>
                              <DropdownMenuItem onSelect={() => void duplicate(h)}>
                                <CopyPlusIcon /> Duplicate
                              </DropdownMenuItem>
                              <DropdownMenuSeparator />
                              <DropdownMenuItem variant="destructive" onSelect={() => setDeleting(h)}>
                                <Trash2Icon /> Delete
                              </DropdownMenuItem>
                            </DropdownMenuContent>
                          </DropdownMenu>
                        </div>
                      </div>
                    ))}
                  </div>
                )}
              </section>
            ))}
          </div>
        )}
      </div>

      <ConfirmDialog
        open={!!deleting}
        onOpenChange={(o) => !o && setDeleting(null)}
        title={`Delete “${deleting?.label}”?`}
        description="The host, its saved password and its port-forwarding rules are removed. Keys in the keychain are kept."
        confirmLabel="Delete"
        destructive
        onConfirm={async () => {
          if (!deleting) return
          await api.deleteHost(deleting.id)
          await Promise.all([loadHosts(), useVault.getState().loadForwards()])
        }}
      />
      <PromptDialog
        open={!!renamingGroup}
        onOpenChange={(o) => !o && setRenamingGroup(null)}
        title="Rename group"
        initial={renamingGroup?.name}
        onSubmit={async (name) => {
          if (!renamingGroup) return
          await api.saveGroup({ ...renamingGroup, name })
          await loadHosts()
        }}
      />
      <ConfirmDialog
        open={!!deletingGroup}
        onOpenChange={(o) => !o && setDeletingGroup(null)}
        title={`Delete group “${deletingGroup?.name}”?`}
        description="Hosts in this group are kept and moved out of the group."
        confirmLabel="Delete group"
        destructive
        onConfirm={async () => {
          if (!deletingGroup) return
          await api.deleteGroup(deletingGroup.id)
          await loadHosts()
        }}
      />
    </div>
  )
}
