import { EyeIcon, EyeOffIcon, KeyRoundIcon, LockIcon, MonitorIcon, TerminalIcon, UsersIcon } from 'lucide-react'
import { useEffect, useState } from 'react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { api, errorMessage } from '@/lib/api'
import type { AuthMethod, Host, Protocol } from '@/lib/types'
import { HOST_COLORS, cn } from '@/lib/utils'
import { useSettings } from '@/stores/settings'
import { useUI } from '@/stores/ui'
import { isSSH, useVault } from '@/stores/vault'

const NO_GROUP = '__none__'
const NEW_GROUP = '__new__'
const DIRECT = '__direct__'

const DEFAULT_PORT: Record<Exclude<Protocol, ''>, number> = { ssh: 22, rdp: 3389, vnc: 5900 }

const PROTOCOLS: { v: Exclude<Protocol, ''>; label: string; hint: string; icon: typeof LockIcon }[] = [
  { v: 'ssh', label: 'SSH', hint: 'Terminal and files', icon: TerminalIcon },
  { v: 'rdp', label: 'RDP', hint: 'Windows desktop', icon: MonitorIcon },
  { v: 'vnc', label: 'VNC', hint: 'Screen sharing', icon: MonitorIcon },
]

function blank(): Host {
  return {
    id: '',
    label: '',
    groupId: '',
    protocol: 'ssh',
    hostname: '',
    port: 22,
    username: '',
    viaHostId: '',
    authMethod: 'password',
    keyId: '',
    hasPassword: false,
    color: '',
    startupCommand: '',
    createdAt: 0,
    updatedAt: 0,
    lastConnectedAt: 0,
  }
}

export function HostEditor() {
  const { editingHostId, editHost, setPage } = useUI()
  const { hosts, groups, keys, loadHosts } = useVault()
  const hasAgent = useSettings((s) => s.info?.hasAgent ?? false)
  const open = editingHostId !== null
  const existing = editingHostId ? hosts.find((h) => h.id === editingHostId) : undefined

  const [host, setHost] = useState<Host>(blank)
  const [password, setPassword] = useState('')
  const [passwordTouched, setPasswordTouched] = useState(false)
  const [showPassword, setShowPassword] = useState(false)
  const [revealed, setRevealed] = useState(false)
  const [newGroup, setNewGroup] = useState('')
  const [groupChoice, setGroupChoice] = useState(NO_GROUP)
  const [busy, setBusy] = useState(false)
  const forwarding = useSettings((s) => (existing ? (s.settings?.forwardKeyHosts ?? []).includes(existing.id) : false))
  const [forward, setForward] = useState(false)

  useEffect(() => {
    if (!open) return
    const h = existing ? { ...existing } : blank()
    setHost(h)
    setPassword('')
    setPasswordTouched(false)
    setShowPassword(false)
    setRevealed(false)
    setNewGroup('')
    setGroupChoice(h.groupId || NO_GROUP)
    setForward(forwarding)
  }, [open, existing, forwarding])

  const set = (patch: Partial<Host>) => setHost((h) => ({ ...h, ...patch }))
  const protocol: Exclude<Protocol, ''> = host.protocol || 'ssh'
  const ssh = protocol === 'ssh'
  const setProtocol = (p: Exclude<Protocol, ''>) =>
    setHost((h) => {
      const cur = h.protocol || 'ssh'
      // Keep a custom port; swap the protocol's default for the new one.
      const port = h.port === DEFAULT_PORT[cur] || !h.port ? DEFAULT_PORT[p] : h.port
      return { ...h, protocol: p, port, ...(p === 'ssh' ? { viaHostId: '' } : { authMethod: 'password' as AuthMethod }) }
    })
  const viaHosts = hosts.filter((h) => isSSH(h) && h.id !== host.id)
  const viaMissing = !!host.viaHostId && !viaHosts.some((h) => h.id === host.viaHostId)

  const savedHidden = !!existing?.hasPassword && !passwordTouched && !revealed

  const toggleShowPassword = async () => {
    // The saved password isn't sent to the editor until asked for; load it on first reveal.
    if (!showPassword && savedHidden && existing) {
      try {
        setPassword(await api.revealHostPassword(existing.id))
        setRevealed(true)
      } catch (e) {
        toast.error('Could not load the saved password', { description: errorMessage(e) })
        return
      }
    }
    setShowPassword((v) => !v)
  }

  const save = async () => {
    setBusy(true)
    try {
      let groupId = groupChoice === NO_GROUP ? '' : groupChoice
      if (groupChoice === NEW_GROUP) {
        if (!newGroup.trim()) throw new Error('Enter a name for the new group')
        groupId = (await api.saveGroup({ id: '', name: newGroup, createdAt: 0 })).id
      }
      const setPw = (!ssh || host.authMethod === 'password') && passwordTouched
      const saved = await api.saveHost({ host: { ...host, groupId }, password: setPw ? password : '', setPassword: setPw })
      if (ssh && forward !== forwarding) useSettings.getState().setLocal(await api.setForwardKeys(saved.id, forward))
      await loadHosts()
      toast.success(existing ? 'Host updated' : 'Host added')
      editHost(null)
    } catch (e) {
      toast.error('Could not save host', { description: errorMessage(e) })
    } finally {
      setBusy(false)
    }
  }

  const methods: { v: AuthMethod; label: string; icon: typeof LockIcon }[] = [
    { v: 'password', label: 'Password', icon: LockIcon },
    { v: 'key', label: 'SSH key', icon: KeyRoundIcon },
    { v: 'agent', label: 'SSH agent', icon: UsersIcon },
  ]

  return (
    <Dialog open={open} onOpenChange={(o) => !o && editHost(null)}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{existing ? 'Edit host' : 'New host'}</DialogTitle>
          <DialogDescription>Credentials are encrypted and stored only on this device.</DialogDescription>
        </DialogHeader>
        <div className="grid max-h-[65vh] grid-cols-6 gap-3 overflow-y-auto px-0.5 pb-1">
          <div className="col-span-6 grid grid-cols-3 gap-2" role="radiogroup" aria-label="Protocol">
            {PROTOCOLS.map((p) => (
              <button
                key={p.v}
                role="radio"
                aria-checked={protocol === p.v}
                onClick={() => setProtocol(p.v)}
                className={cn(
                  'flex flex-col items-center gap-0.5 rounded-md border py-1.5 transition-colors',
                  protocol === p.v ? 'border-primary bg-primary/10 text-foreground' : 'text-muted-foreground hover:bg-accent',
                )}
              >
                <span className="flex items-center gap-1.5 text-[13px] font-medium">
                  <p.icon className="size-3.5" /> {p.label}
                </span>
                <span className="text-[11px] text-muted-foreground">{p.hint}</span>
              </button>
            ))}
          </div>
          <Field label="Address" className="col-span-4">
            <Input autoFocus placeholder="192.168.1.10 or server.example.com" value={host.hostname} onChange={(e) => set({ hostname: e.target.value })} />
          </Field>
          <Field label="Port" className="col-span-2">
            <Input type="number" min={1} max={65535} value={host.port} onChange={(e) => set({ port: Number(e.target.value) })} />
          </Field>
          <Field label="Label" className="col-span-3">
            <Input placeholder={host.hostname || 'My server'} value={host.label} onChange={(e) => set({ label: e.target.value })} />
          </Field>
          <Field label="Username" className="col-span-3">
            <Input
              placeholder={protocol === 'rdp' ? 'Administrator' : protocol === 'vnc' ? 'Optional' : 'root'}
              value={host.username}
              onChange={(e) => set({ username: e.target.value })}
            />
          </Field>
          <Field label="Group" className="col-span-6">
            <div className="flex gap-2">
              <Select value={groupChoice} onValueChange={setGroupChoice}>
                <SelectTrigger className="w-48">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value={NO_GROUP}>No group</SelectItem>
                  {groups.map((g) => (
                    <SelectItem key={g.id} value={g.id}>
                      {g.name}
                    </SelectItem>
                  ))}
                  <SelectItem value={NEW_GROUP}>+ New group…</SelectItem>
                </SelectContent>
              </Select>
              {groupChoice === NEW_GROUP && <Input placeholder="Group name" value={newGroup} onChange={(e) => setNewGroup(e.target.value)} />}
            </div>
          </Field>

          {!ssh && (
            <Field
              label="Tunnel through"
              className="col-span-6"
              hint={
                host.viaHostId
                  ? `The ${protocol.toUpperCase()} connection goes through this SSH server, so the desktop doesn't need to be reachable from here.`
                  : 'Connect straight to the address, or pick an SSH server to tunnel through.'
              }
            >
              <Select value={host.viaHostId || DIRECT} onValueChange={(v) => set({ viaHostId: v === DIRECT ? '' : v })}>
                <SelectTrigger className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value={DIRECT}>Connect directly</SelectItem>
                  {viaMissing && <SelectItem value={host.viaHostId}>Deleted host</SelectItem>}
                  {viaHosts.map((h) => (
                    <SelectItem key={h.id} value={h.id}>
                      {h.label} · {h.username ? `${h.username}@` : ''}
                      {h.hostname}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </Field>
          )}
          {ssh && (
            <div className="col-span-6 flex flex-col gap-2">
              <Label>Authentication</Label>
              <div className="grid grid-cols-3 gap-2">
                {methods.map((m) => (
                  <button
                    key={m.v}
                    onClick={() => set({ authMethod: m.v })}
                    className={cn(
                      'flex items-center justify-center gap-2 rounded-md border py-2 text-[13px] transition-colors',
                      host.authMethod === m.v ? 'border-primary bg-primary/10 text-foreground' : 'text-muted-foreground hover:bg-accent',
                    )}
                  >
                    <m.icon className="size-4" /> {m.label}
                  </button>
                ))}
              </div>
            </div>
          )}
          {(!ssh || host.authMethod === 'password') && (
            <Field
              label={protocol === 'vnc' ? 'VNC password' : 'Password'}
              className="col-span-6"
              hint={
                savedHidden
                  ? 'A password is saved. Type to replace it, or leave blank to keep it.'
                  : existing?.hasPassword && !passwordTouched
                    ? 'This is the saved password. Edit it to change it.'
                    : protocol === 'rdp'
                      ? 'Optional. Handed to the RDP app when it opens; otherwise it asks.'
                      : protocol === 'vnc'
                        ? 'Optional. Sent when the server asks for it; otherwise you are asked.'
                        : 'Leave blank to be asked when connecting.'
              }
            >
              <div className="flex gap-2">
                <div className="relative flex-1">
                  <Input
                    type={showPassword ? 'text' : 'password'}
                    placeholder={savedHidden ? '••••••••' : 'Password'}
                    value={password}
                    autoComplete="off"
                    spellCheck={false}
                    className="pr-9"
                    onChange={(e) => {
                      setPassword(e.target.value)
                      setPasswordTouched(true)
                    }}
                  />
                  <button
                    type="button"
                    tabIndex={-1}
                    onClick={toggleShowPassword}
                    aria-label={showPassword ? 'Hide password' : 'Show password'}
                    className="absolute top-1/2 right-2.5 -translate-y-1/2 text-muted-foreground hover:text-foreground"
                  >
                    {showPassword ? <EyeOffIcon className="size-3.5" /> : <EyeIcon className="size-3.5" />}
                  </button>
                </div>
                {existing?.hasPassword && (
                  <Button
                    variant="outline"
                    onClick={() => {
                      setPassword('')
                      setPasswordTouched(true)
                      setRevealed(false)
                    }}
                  >
                    Forget
                  </Button>
                )}
              </div>
            </Field>
          )}
          {ssh && host.authMethod === 'key' && (
            <Field label="Key" className="col-span-6">
              {keys.length === 0 ? (
                <div className="flex items-center justify-between rounded-md border border-dashed px-3 py-2 text-xs text-muted-foreground">
                  Your keychain is empty.
                  <Button
                    variant="link"
                    size="xs"
                    onClick={() => {
                      editHost(null)
                      setPage('keychain')
                    }}
                  >
                    Generate or import a key
                  </Button>
                </div>
              ) : (
                <Select value={host.keyId} onValueChange={(keyId) => set({ keyId })}>
                  <SelectTrigger className="w-full">
                    <SelectValue placeholder="Choose a key" />
                  </SelectTrigger>
                  <SelectContent>
                    {keys.map((k) => (
                      <SelectItem key={k.id} value={k.id}>
                        {k.name} · {k.type.replace('ssh-', '')}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              )}
            </Field>
          )}
          {ssh && host.authMethod === 'agent' && (
            <div className={cn('col-span-6 rounded-md px-3 py-2 text-xs', hasAgent ? 'bg-muted/50 text-muted-foreground' : 'bg-warning/10 text-warning')}>
              {hasAgent
                ? 'Keys loaded in your running SSH agent (ssh-add -l) are offered to the server.'
                : 'No SSH agent was detected (SSH_AUTH_SOCK is not set). Agent authentication will fail until one is running.'}
            </div>
          )}
          {ssh && (
            <>
              <Field label="Startup command" className="col-span-6" hint="Optional. Typed into the shell right after connecting.">
                <Textarea
                  rows={2}
                  className="font-mono text-[12px]"
                  placeholder="cd /var/www"
                  value={host.startupCommand}
                  onChange={(e) => set({ startupCommand: e.target.value })}
                />
              </Field>
              <div className="col-span-6 flex items-start justify-between gap-4 rounded-md border px-3 py-2.5">
                <div className="flex flex-col gap-0.5">
                  <Label htmlFor="forward-keys">Forward my SSH keys</Label>
                  <span className="text-xs text-muted-foreground">
                    Lets git and ssh on this server log in with your keychain's keys, e.g. to pull a private repo. Keys never leave Remvyn, and you approve each use.
                    Applies to new connections, on this device only.
                  </span>
                </div>
                <Switch id="forward-keys" checked={forward} onCheckedChange={setForward} />
              </div>
            </>
          )}
          <Field label="Color" className="col-span-6">
            <div className="flex gap-2">
              {HOST_COLORS.map((c) => (
                <button
                  key={c || 'none'}
                  onClick={() => set({ color: c })}
                  aria-label={c ? `Color ${c}` : 'No color'}
                  className={cn(
                    'size-6 rounded-full border-2 transition-transform hover:scale-110',
                    host.color === c ? 'border-foreground' : 'border-transparent',
                    !c && 'bg-muted',
                  )}
                  style={c ? { background: c } : undefined}
                />
              ))}
            </div>
          </Field>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => editHost(null)}>
            Cancel
          </Button>
          <Button disabled={busy || !host.hostname.trim() || (ssh && host.authMethod === 'key' && !host.keyId)} onClick={save}>
            {existing ? 'Save' : 'Add host'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function Field({ label, hint, className, children }: { label: string; hint?: string; className?: string; children: React.ReactNode }) {
  return (
    <div className={cn('flex flex-col gap-1.5', className)}>
      <Label>{label}</Label>
      {children}
      {hint && <p className="text-[11.5px] text-muted-foreground">{hint}</p>}
    </div>
  )
}
