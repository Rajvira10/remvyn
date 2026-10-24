import { DownloadIcon, FileUpIcon, KeyRoundIcon, MoreHorizontalIcon, PencilIcon, PlusIcon, SendIcon, ShieldCheckIcon, Trash2Icon } from 'lucide-react'
import { useEffect, useState } from 'react'
import { toast } from 'sonner'
import { ConfirmDialog } from '@/components/common/ConfirmDialog'
import { CopyButton } from '@/components/common/CopyButton'
import { EmptyState } from '@/components/common/EmptyState'
import { PromptDialog } from '@/components/common/PromptDialog'
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
import { Textarea } from '@/components/ui/textarea'
import { api, emptyConnect, errorCode, errorMessage } from '@/lib/api'
import type { Key, KeyGenerateInput } from '@/lib/types'
import { cn, formatDate } from '@/lib/utils'
import { useVault } from '@/stores/vault'

export function KeychainPage() {
  const { keys, loadKeys } = useVault()
  const [generating, setGenerating] = useState(false)
  const [importing, setImporting] = useState(false)
  const [renaming, setRenaming] = useState<Key | null>(null)
  const [deleting, setDeleting] = useState<Key | null>(null)
  const [installing, setInstalling] = useState<Key | null>(null)

  const exportKey = async (k: Key) => {
    try {
      const path = await api.exportPrivateKey(k.id)
      if (path) toast.success('Private key exported', { description: `${path} (permissions 600)` })
    } catch (e) {
      toast.error('Export failed', { description: errorMessage(e) })
    }
  }

  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className="flex h-14 shrink-0 items-center gap-3 border-b px-5">
        <h1 className="text-[15px] font-semibold">Keychain</h1>
        <span className="text-xs text-muted-foreground">Private keys are stored encrypted and never leave this device.</span>
        <div className="ml-auto flex gap-2">
          <Button variant="ghost" size="sm" onClick={() => setImporting(true)}>
            <FileUpIcon /> Import key
          </Button>
          <Button size="sm" onClick={() => setGenerating(true)}>
            <PlusIcon /> Generate key
          </Button>
        </div>
      </div>
      <div className="min-h-0 flex-1 overflow-auto p-5">
        {keys.length === 0 ? (
          <EmptyState icon={KeyRoundIcon} title="No keys yet">
            Generate a new Ed25519 key, or import an existing private key (OpenSSH or PEM).
            <div className="mt-4 flex justify-center gap-2">
              <Button variant="outline" size="sm" onClick={() => setImporting(true)}>
                <FileUpIcon /> Import
              </Button>
              <Button size="sm" onClick={() => setGenerating(true)}>
                <PlusIcon /> Generate
              </Button>
            </div>
          </EmptyState>
        ) : (
          <div className="grid grid-cols-[repeat(auto-fill,minmax(340px,1fr))] gap-3">
            {keys.map((k) => (
              <div key={k.id} className="flex flex-col gap-3 rounded-xl border bg-card p-4">
                <div className="flex items-start gap-3">
                  <div className="flex size-9 items-center justify-center rounded-lg bg-primary/15">
                    <KeyRoundIcon className="size-4 text-primary" />
                  </div>
                  <div className="min-w-0 flex-1">
                    <div className="truncate font-medium">{k.name}</div>
                    <div className="mt-1 flex flex-wrap gap-1.5">
                      <Badge variant="secondary">{k.type.replace('ssh-', '').replace('ecdsa-sha2-', 'ecdsa ')}</Badge>
                      {k.bits > 0 && <Badge variant="outline">{k.bits} bit</Badge>}
                      {k.hasPassphrase && (
                        <Badge variant="outline">
                          <ShieldCheckIcon /> passphrase
                        </Badge>
                      )}
                    </div>
                  </div>
                  <DropdownMenu>
                    <DropdownMenuTrigger asChild>
                      <Button variant="ghost" size="icon-xs" aria-label="Key actions">
                        <MoreHorizontalIcon />
                      </Button>
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align="end">
                      <DropdownMenuItem onSelect={() => setInstalling(k)}>
                        <SendIcon /> Install on host…
                      </DropdownMenuItem>
                      <DropdownMenuItem onSelect={() => void exportKey(k)}>
                        <DownloadIcon /> Export private key…
                      </DropdownMenuItem>
                      <DropdownMenuItem onSelect={() => setRenaming(k)}>
                        <PencilIcon /> Rename
                      </DropdownMenuItem>
                      <DropdownMenuSeparator />
                      <DropdownMenuItem variant="destructive" onSelect={() => setDeleting(k)}>
                        <Trash2Icon /> Delete
                      </DropdownMenuItem>
                    </DropdownMenuContent>
                  </DropdownMenu>
                </div>
                <div className="font-mono text-[11px] break-all text-muted-foreground" data-selectable>
                  {k.fingerprint}
                </div>
                <div className="flex items-center gap-2 rounded-md bg-muted/50 py-1 pr-1 pl-2">
                  <span className="flex-1 truncate font-mono text-[11px]" title={k.publicKey}>
                    {k.publicKey}
                  </span>
                  <CopyButton text={`${k.publicKey} ${k.name.replace(/\s+/g, '-')}`} label="Copy public key" />
                </div>
                <div className="text-[11px] text-muted-foreground">Added {formatDate(k.createdAt)}</div>
              </div>
            ))}
          </div>
        )}
      </div>

      <GenerateKeyDialog open={generating} onOpenChange={setGenerating} onDone={loadKeys} />
      <ImportKeyDialog open={importing} onOpenChange={setImporting} onDone={loadKeys} />
      <InstallKeyDialog keyItem={installing} onOpenChange={(o) => !o && setInstalling(null)} />
      <PromptDialog
        open={!!renaming}
        onOpenChange={(o) => !o && setRenaming(null)}
        title="Rename key"
        initial={renaming?.name}
        onSubmit={async (name) => {
          if (!renaming) return
          try {
            await api.renameKey(renaming.id, name)
            await loadKeys()
          } catch (e) {
            toast.error(errorMessage(e))
            throw e
          }
        }}
      />
      <ConfirmDialog
        open={!!deleting}
        onOpenChange={(o) => !o && setDeleting(null)}
        title={`Delete “${deleting?.name}”?`}
        description="The private key is permanently removed from Flux. Export it first if you still need it."
        confirmLabel="Delete key"
        destructive
        onConfirm={async () => {
          if (!deleting) return
          try {
            await api.deleteKey(deleting.id)
            await loadKeys()
          } catch (e) {
            toast.error('Cannot delete key', { description: errorMessage(e) })
            throw e
          }
        }}
      />
    </div>
  )
}

function GenerateKeyDialog({ open, onOpenChange, onDone }: { open: boolean; onOpenChange: (o: boolean) => void; onDone: () => Promise<void> }) {
  const [form, setForm] = useState<KeyGenerateInput>({ name: '', type: 'ed25519', bits: 0, passphrase: '' })
  const [busy, setBusy] = useState(false)
  useEffect(() => {
    if (open) setForm({ name: '', type: 'ed25519', bits: 0, passphrase: '' })
  }, [open])
  const types: { v: KeyGenerateInput['type']; label: string; hint: string }[] = [
    { v: 'ed25519', label: 'Ed25519', hint: 'Recommended' },
    { v: 'ecdsa', label: 'ECDSA', hint: 'P-256/384/521' },
    { v: 'rsa', label: 'RSA', hint: 'Legacy servers' },
  ]
  const submit = async () => {
    setBusy(true)
    try {
      const k = await api.generateKey(form)
      await onDone()
      toast.success(`Generated ${k.name}`, { description: k.fingerprint })
      onOpenChange(false)
    } catch (e) {
      toast.error('Could not generate key', { description: errorMessage(e) })
    } finally {
      setBusy(false)
    }
  }
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Generate key</DialogTitle>
          <DialogDescription>Creates a new key pair. Copy or install the public key on your servers.</DialogDescription>
        </DialogHeader>
        <div className="flex flex-col gap-3">
          <div className="flex flex-col gap-1.5">
            <Label>Name</Label>
            <Input autoFocus placeholder="work-laptop" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} />
          </div>
          <div className="grid grid-cols-3 gap-2">
            {types.map((t) => (
              <button
                key={t.v}
                onClick={() => setForm({ ...form, type: t.v, bits: 0 })}
                className={cn('rounded-md border p-2 text-left', form.type === t.v ? 'border-primary bg-primary/10' : 'hover:bg-accent')}
              >
                <div className="text-[13px] font-medium">{t.label}</div>
                <div className="text-[11px] text-muted-foreground">{t.hint}</div>
              </button>
            ))}
          </div>
          {form.type !== 'ed25519' && (
            <div className="flex flex-col gap-1.5">
              <Label>Key size</Label>
              <Select value={String(form.bits || (form.type === 'rsa' ? 4096 : 256))} onValueChange={(v) => setForm({ ...form, bits: Number(v) })}>
                <SelectTrigger className="w-40">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {(form.type === 'rsa' ? [2048, 3072, 4096] : [256, 384, 521]).map((b) => (
                    <SelectItem key={b} value={String(b)}>
                      {b} bits
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          )}
          <div className="flex flex-col gap-1.5">
            <Label>Passphrase (optional)</Label>
            <Input type="password" value={form.passphrase} onChange={(e) => setForm({ ...form, passphrase: e.target.value })} />
            <p className="text-[11.5px] text-muted-foreground">Encrypts the key file when exported. Flux stores it so you aren't asked on every connection.</p>
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button disabled={busy || !form.name.trim()} onClick={submit}>
            {busy ? 'Generating…' : 'Generate'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function ImportKeyDialog({ open, onOpenChange, onDone }: { open: boolean; onOpenChange: (o: boolean) => void; onDone: () => Promise<void> }) {
  const [name, setName] = useState('')
  const [pem, setPem] = useState('')
  const [passphrase, setPassphrase] = useState('')
  const [needsPass, setNeedsPass] = useState(false)
  const [busy, setBusy] = useState(false)
  useEffect(() => {
    if (open) {
      setName('')
      setPem('')
      setPassphrase('')
      setNeedsPass(false)
    }
  }, [open])
  const chooseFile = async () => {
    try {
      const text = await api.readKeyFile()
      if (text) setPem(text)
    } catch (e) {
      toast.error(errorMessage(e))
    }
  }
  const submit = async () => {
    setBusy(true)
    try {
      const k = await api.importKey({ name, privateKey: pem, passphrase })
      await onDone()
      toast.success(`Imported ${k.name}`, { description: k.fingerprint })
      onOpenChange(false)
    } catch (e) {
      if (errorCode(e) === 'passphrase_required') {
        setNeedsPass(true)
        if (passphrase) toast.error('Incorrect passphrase')
      } else toast.error('Could not import key', { description: errorMessage(e) })
    } finally {
      setBusy(false)
    }
  }
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>Import key</DialogTitle>
          <DialogDescription>Paste a private key (OpenSSH, PKCS#1/#8 or EC PEM) or choose a file.</DialogDescription>
        </DialogHeader>
        <div className="flex flex-col gap-3">
          <div className="flex flex-col gap-1.5">
            <Label>Name</Label>
            <Input placeholder="Detected from the key if empty" value={name} onChange={(e) => setName(e.target.value)} />
          </div>
          <div className="flex flex-col gap-1.5">
            <div className="flex items-center justify-between">
              <Label>Private key</Label>
              <Button variant="link" size="xs" onClick={chooseFile}>
                Choose file…
              </Button>
            </div>
            <Textarea
              rows={7}
              className="font-mono text-[11px]"
              placeholder="-----BEGIN OPENSSH PRIVATE KEY-----"
              value={pem}
              onChange={(e) => setPem(e.target.value)}
            />
          </div>
          {needsPass && (
            <div className="flex flex-col gap-1.5">
              <Label>Passphrase</Label>
              <Input type="password" autoFocus value={passphrase} onChange={(e) => setPassphrase(e.target.value)} onKeyDown={(e) => e.key === 'Enter' && submit()} />
              <p className="text-[11.5px] text-muted-foreground">This key is encrypted. The passphrase is stored encrypted alongside it.</p>
            </div>
          )}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button disabled={busy || !pem.trim()} onClick={submit}>
            Import
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function InstallKeyDialog({ keyItem, onOpenChange }: { keyItem: Key | null; onOpenChange: (o: boolean) => void }) {
  const hosts = useVault((s) => s.hosts)
  const [hostId, setHostId] = useState('')
  const [password, setPassword] = useState('')
  const [busy, setBusy] = useState(false)
  const host = hosts.find((h) => h.id === hostId)
  useEffect(() => {
    if (keyItem) {
      setHostId('')
      setPassword('')
    }
  }, [keyItem])
  const needsPassword = host?.authMethod === 'password' && !host.hasPassword
  const submit = async () => {
    if (!keyItem || !host) return
    setBusy(true)
    try {
      await api.installPublicKey(host.id, keyItem.id, { ...emptyConnect(), password })
      toast.success(`Installed ${keyItem.name} on ${host.label}`, { description: 'You can now switch the host to key authentication.' })
      onOpenChange(false)
    } catch (e) {
      toast.error('Could not install key', { description: errorMessage(e) })
    } finally {
      setBusy(false)
    }
  }
  return (
    <Dialog open={!!keyItem} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Install public key</DialogTitle>
          <DialogDescription>
            Appends {keyItem?.name} to ~/.ssh/authorized_keys on the server (like ssh-copy-id), connecting with the host's current credentials.
          </DialogDescription>
        </DialogHeader>
        <div className="flex flex-col gap-3">
          <Select value={hostId} onValueChange={setHostId}>
            <SelectTrigger className="w-full">
              <SelectValue placeholder="Choose a host" />
            </SelectTrigger>
            <SelectContent>
              {hosts.map((h) => (
                <SelectItem key={h.id} value={h.id}>
                  {h.label} · {h.username ? h.username + '@' : ''}
                  {h.hostname}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          {needsPassword && (
            <div className="flex flex-col gap-1.5">
              <Label>Password for {host?.label}</Label>
              <Input type="password" value={password} onChange={(e) => setPassword(e.target.value)} />
            </div>
          )}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button disabled={busy || !host || (needsPassword && !password)} onClick={submit}>
            {busy ? 'Installing…' : 'Install'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
