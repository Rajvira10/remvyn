import { EyeIcon, EyeOffIcon, KeyRoundIcon, LockIcon, UsersIcon } from 'lucide-react'
import { useEffect, useState } from 'react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'
import { api, errorMessage } from '@/lib/api'
import type { AuthMethod, Host } from '@/lib/types'
import { HOST_COLORS, cn } from '@/lib/utils'
import { useSettings } from '@/stores/settings'
import { useUI } from '@/stores/ui'
import { useVault } from '@/stores/vault'

const NO_GROUP = '__none__'
const NEW_GROUP = '__new__'

function blank(): Host {
  return {
    id: '',
    label: '',
    groupId: '',
    hostname: '',
    port: 22,
    username: '',
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
  }, [open, existing])

  const set = (patch: Partial<Host>) => setHost((h) => ({ ...h, ...patch }))

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
      const setPw = host.authMethod === 'password' && passwordTouched
      await api.saveHost({ host: { ...host, groupId }, password: setPw ? password : '', setPassword: setPw })
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
            <Input placeholder="root" value={host.username} onChange={(e) => set({ username: e.target.value })} />
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
          {host.authMethod === 'password' && (
            <Field
              label="Password"
              className="col-span-6"
              hint={
                savedHidden
                  ? 'A password is saved. Type to replace it, or leave blank to keep it.'
                  : existing?.hasPassword && !passwordTouched
                    ? 'This is the saved password. Edit it to change it.'
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
          {host.authMethod === 'key' && (
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
          {host.authMethod === 'agent' && (
            <div className={cn('col-span-6 rounded-md px-3 py-2 text-xs', hasAgent ? 'bg-muted/50 text-muted-foreground' : 'bg-warning/10 text-warning')}>
              {hasAgent
                ? 'Keys loaded in your running SSH agent (ssh-add -l) are offered to the server.'
                : 'No SSH agent was detected (SSH_AUTH_SOCK is not set). Agent authentication will fail until one is running.'}
            </div>
          )}
          <Field label="Startup command" className="col-span-6" hint="Optional. Typed into the shell right after connecting.">
            <Textarea
              rows={2}
              className="font-mono text-[12px]"
              placeholder="cd /var/www && tmux attach"
              value={host.startupCommand}
              onChange={(e) => set({ startupCommand: e.target.value })}
            />
          </Field>
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
          <Button disabled={busy || !host.hostname.trim() || (host.authMethod === 'key' && !host.keyId)} onClick={save}>
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
