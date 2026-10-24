import { ArrowRightIcon, CableIcon, MoreHorizontalIcon, PencilIcon, PlayIcon, PlusIcon, SquareIcon, Trash2Icon } from 'lucide-react'
import { useEffect, useState } from 'react'
import { toast } from 'sonner'
import { ConfirmDialog } from '@/components/common/ConfirmDialog'
import { EmptyState } from '@/components/common/EmptyState'
import { Spinner } from '@/components/common/Spinner'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { api, errorMessage } from '@/lib/api'
import type { ForwardRule, ForwardType } from '@/lib/types'
import { cn, formatBytes } from '@/lib/utils'
import { useVault } from '@/stores/vault'

const TYPE_INFO: Record<ForwardType, { label: string; flag: string; help: string }> = {
  local: { label: 'Local', flag: '-L', help: 'Open a port on this computer that reaches a service through the SSH server (e.g. a remote database).' },
  remote: { label: 'Remote', flag: '-R', help: 'Open a port on the SSH server that reaches a service on (or reachable from) this computer.' },
  dynamic: { label: 'Dynamic', flag: '-D', help: 'Run a SOCKS5 proxy on this computer; connections exit to the internet from the SSH server.' },
}

function describe(r: ForwardRule, hostLabel: string): React.ReactNode {
  const bind = `${r.bindHost}:${r.bindPort}`
  const dest = `${r.destHost}:${r.destPort}`
  if (r.type === 'dynamic') return <>SOCKS5 on <code>{bind}</code> via {hostLabel}</>
  if (r.type === 'local') return <><code>{bind}</code> <ArrowRightIcon className="inline size-3" /> <code>{dest}</code> via {hostLabel}</>
  return <>{hostLabel} <code>{bind}</code> <ArrowRightIcon className="inline size-3" /> <code>{dest}</code> here</>
}

export function ForwardingPage() {
  const { forwards, statuses, hosts, loadForwards } = useVault()
  const [editing, setEditing] = useState<ForwardRule | null>(null)
  const [deleting, setDeleting] = useState<ForwardRule | null>(null)
  const [busy, setBusy] = useState<Set<string>>(new Set())

  const hostLabel = (id: string) => hosts.find((h) => h.id === id)?.label ?? 'deleted host'

  const toggle = async (r: ForwardRule) => {
    const running = statuses[r.id]?.state === 'running'
    setBusy((b) => new Set(b).add(r.id))
    try {
      if (running) await api.stopForward(r.id)
      else {
        await api.startForward(r.id)
        toast.success(`${r.label} is running`)
      }
    } catch (e) {
      toast.error(`Could not start ${r.label}`, { description: errorMessage(e) })
    } finally {
      setBusy((b) => {
        const n = new Set(b)
        n.delete(r.id)
        return n
      })
    }
  }

  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className="flex h-14 shrink-0 items-center gap-3 border-b px-5">
        <h1 className="text-[15px] font-semibold">Port forwarding</h1>
        <div className="ml-auto">
          <Button size="sm" disabled={hosts.length === 0} onClick={() => setEditing(newRule(hosts[0]?.id ?? ''))}>
            <PlusIcon /> New rule
          </Button>
        </div>
      </div>
      <div className="min-h-0 flex-1 overflow-auto p-5">
        {forwards.length === 0 ? (
          <EmptyState icon={CableIcon} title="No forwarding rules">
            {hosts.length === 0 ? 'Add a host first, then create local, remote or SOCKS forwarding rules through it.' : 'Tunnel ports through your SSH hosts: local (-L), remote (-R) or dynamic SOCKS (-D).'}
          </EmptyState>
        ) : (
          <div className="flex flex-col gap-2">
            {forwards.map((r) => {
              const st = statuses[r.id]
              const state = busy.has(r.id) && st?.state !== 'running' ? 'starting' : (st?.state ?? 'stopped')
              return (
                <div key={r.id} className="flex items-center gap-4 rounded-xl border bg-card px-4 py-3">
                  <span
                    className={cn('size-2.5 shrink-0 rounded-full', {
                      'bg-success shadow-[0_0_6px_var(--success)]': state === 'running',
                      'bg-danger': state === 'error',
                      'bg-warning': state === 'starting',
                      'bg-muted-foreground/40': state === 'stopped',
                    })}
                  />
                  <div className="min-w-0 flex-1">
                    <div className="flex items-center gap-2">
                      <span className="truncate font-medium">{r.label}</span>
                      <Badge variant="outline">
                        {TYPE_INFO[r.type].label} {TYPE_INFO[r.type].flag}
                      </Badge>
                      {r.autoStart && <Badge variant="secondary">auto-start</Badge>}
                    </div>
                    <div className="mt-0.5 truncate text-xs text-muted-foreground [&_code]:font-mono [&_code]:text-foreground/80">{describe(r, hostLabel(r.hostId))}</div>
                    {state === 'error' && st?.error && <div className="mt-1 text-xs text-danger">{st.error}</div>}
                  </div>
                  {state === 'running' && st && (
                    <div className="text-right text-[11px] text-muted-foreground tabular-nums">
                      <div>
                        {st.activeConns} active · {st.totalConns} total
                      </div>
                      <div>
                        ↑ {formatBytes(st.bytesOut)} ↓ {formatBytes(st.bytesIn)}
                      </div>
                    </div>
                  )}
                  <Button variant={state === 'running' ? 'outline' : 'default'} size="sm" className="w-24" disabled={state === 'starting'} onClick={() => void toggle(r)}>
                    {state === 'starting' ? <Spinner className="size-3.5 text-current" /> : state === 'running' ? <SquareIcon className="size-3 fill-current" /> : <PlayIcon className="size-3.5 fill-current" />}
                    {state === 'running' ? 'Stop' : state === 'starting' ? 'Starting' : 'Start'}
                  </Button>
                  <DropdownMenu>
                    <DropdownMenuTrigger asChild>
                      <Button variant="ghost" size="icon-sm" aria-label="Rule actions">
                        <MoreHorizontalIcon />
                      </Button>
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align="end">
                      <DropdownMenuItem onSelect={() => setEditing(r)}>
                        <PencilIcon /> Edit
                      </DropdownMenuItem>
                      <DropdownMenuSeparator />
                      <DropdownMenuItem variant="destructive" onSelect={() => setDeleting(r)}>
                        <Trash2Icon /> Delete
                      </DropdownMenuItem>
                    </DropdownMenuContent>
                  </DropdownMenu>
                </div>
              )
            })}
          </div>
        )}
      </div>
      <RuleDialog rule={editing} onOpenChange={(o) => !o && setEditing(null)} onSaved={loadForwards} running={!!editing && statuses[editing.id]?.state === 'running'} />
      <ConfirmDialog
        open={!!deleting}
        onOpenChange={(o) => !o && setDeleting(null)}
        title={`Delete “${deleting?.label}”?`}
        description="The rule is stopped and removed."
        confirmLabel="Delete"
        destructive
        onConfirm={async () => {
          if (!deleting) return
          await api.deleteForward(deleting.id)
          await loadForwards()
        }}
      />
    </div>
  )
}

function newRule(hostId: string): ForwardRule {
  return { id: '', label: '', hostId, type: 'local', bindHost: '127.0.0.1', bindPort: 8080, destHost: 'localhost', destPort: 80, autoStart: false, createdAt: 0 }
}

function RuleDialog({ rule, onOpenChange, onSaved, running }: { rule: ForwardRule | null; onOpenChange: (o: boolean) => void; onSaved: () => Promise<void>; running: boolean }) {
  const hosts = useVault((s) => s.hosts)
  const [r, setR] = useState<ForwardRule>(newRule(''))
  const [busy, setBusy] = useState(false)
  useEffect(() => {
    if (rule) setR(rule)
  }, [rule])
  const set = (patch: Partial<ForwardRule>) => setR((x) => ({ ...x, ...patch }))
  const publicBind = r.bindHost !== '127.0.0.1' && r.bindHost !== 'localhost' && r.bindHost !== '::1'

  const save = async () => {
    setBusy(true)
    try {
      await api.saveForward(r)
      await onSaved()
      toast.success(rule?.id ? (running ? 'Rule saved. Restart it to apply changes.' : 'Rule saved') : 'Rule created')
      onOpenChange(false)
    } catch (e) {
      toast.error('Could not save rule', { description: errorMessage(e) })
    } finally {
      setBusy(false)
    }
  }

  const bindLabel = r.type === 'remote' ? 'Listen on the server' : 'Listen on this computer'
  const destLabel = r.type === 'remote' ? 'Forward to (from this computer)' : 'Forward to (from the server)'

  return (
    <Dialog open={!!rule} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{rule?.id ? 'Edit forwarding rule' : 'New forwarding rule'}</DialogTitle>
          <DialogDescription>{TYPE_INFO[r.type].help}</DialogDescription>
        </DialogHeader>
        <div className="flex flex-col gap-3">
          <div className="grid grid-cols-3 gap-2">
            {(Object.keys(TYPE_INFO) as ForwardType[]).map((t) => (
              <button
                key={t}
                onClick={() => set({ type: t })}
                className={cn('rounded-md border py-2 text-[13px]', r.type === t ? 'border-primary bg-primary/10' : 'text-muted-foreground hover:bg-accent')}
              >
                {TYPE_INFO[t].label} <span className="font-mono text-[11px] opacity-70">{TYPE_INFO[t].flag}</span>
              </button>
            ))}
          </div>
          <div className="grid grid-cols-6 gap-3">
            <div className="col-span-6 flex flex-col gap-1.5">
              <Label>SSH host</Label>
              <Select value={r.hostId} onValueChange={(hostId) => set({ hostId })}>
                <SelectTrigger className="w-full">
                  <SelectValue placeholder="Choose a host" />
                </SelectTrigger>
                <SelectContent>
                  {hosts.map((h) => (
                    <SelectItem key={h.id} value={h.id}>
                      {h.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="col-span-4 flex flex-col gap-1.5">
              <Label>{bindLabel}</Label>
              <Input value={r.bindHost} onChange={(e) => set({ bindHost: e.target.value })} className="font-mono" />
            </div>
            <div className="col-span-2 flex flex-col gap-1.5">
              <Label>Port</Label>
              <Input type="number" value={r.bindPort} onChange={(e) => set({ bindPort: Number(e.target.value) })} />
            </div>
            {publicBind && (
              <div className="col-span-6 rounded-md bg-warning/10 px-3 py-2 text-xs text-warning">
                Binding to {r.bindHost || 'all interfaces'} exposes this port to other machines on the network. Use 127.0.0.1 unless you need that.
              </div>
            )}
            {r.type !== 'dynamic' && (
              <>
                <div className="col-span-4 flex flex-col gap-1.5">
                  <Label>{destLabel}</Label>
                  <Input value={r.destHost} onChange={(e) => set({ destHost: e.target.value })} className="font-mono" placeholder="localhost" />
                </div>
                <div className="col-span-2 flex flex-col gap-1.5">
                  <Label>Port</Label>
                  <Input type="number" value={r.destPort} onChange={(e) => set({ destPort: Number(e.target.value) })} />
                </div>
              </>
            )}
            <div className="col-span-6 flex flex-col gap-1.5">
              <Label>Label (optional)</Label>
              <Input value={r.label} onChange={(e) => set({ label: e.target.value })} placeholder="Postgres on prod" />
            </div>
          </div>
          <Label className="flex items-center justify-between font-normal">
            Start automatically when Remvyn launches
            <Switch checked={r.autoStart} onCheckedChange={(autoStart) => set({ autoStart })} />
          </Label>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button disabled={busy || !r.hostId} onClick={save}>
            Save
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
