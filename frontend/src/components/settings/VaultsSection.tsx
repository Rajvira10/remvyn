import { CheckIcon, PencilIcon, PlusIcon, Trash2Icon } from 'lucide-react'
import { useEffect, useState } from 'react'
import { toast } from 'sonner'
import { ConfirmDialog } from '@/components/common/ConfirmDialog'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { api, errorMessage } from '@/lib/api'
import type { VaultProfile } from '@/lib/types'
import { useLock } from '@/stores/lock'
import { reopenPage } from '@/stores/ui'
import { Field, Section } from './Section'

/** Reload the window on Settings so every store reads the newly opened vault. */
function reloadIntoSettings() {
  reopenPage('settings')
  window.location.reload()
}

function NameDialog({
  open,
  onOpenChange,
  title,
  description,
  initial,
  confirmLabel,
  onSubmit,
}: {
  open: boolean
  onOpenChange: (o: boolean) => void
  title: string
  description: string
  initial: string
  confirmLabel: string
  onSubmit: (name: string) => Promise<void>
}) {
  const [name, setName] = useState(initial)
  const [busy, setBusy] = useState(false)
  const submit = async () => {
    setBusy(true)
    try {
      await onSubmit(name)
      onOpenChange(false)
    } catch (e) {
      toast.error(errorMessage(e))
    } finally {
      setBusy(false)
    }
  }
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>{description}</DialogDescription>
        </DialogHeader>
        <form
          className="grid gap-1.5"
          onSubmit={(e) => {
            e.preventDefault()
            void submit()
          }}
        >
          <Label htmlFor="vault-name">Name</Label>
          <Input id="vault-name" value={name} onChange={(e) => setName(e.target.value)} placeholder="Work" maxLength={60} autoFocus />
          <DialogFooter className="mt-3">
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              Cancel
            </Button>
            <Button type="submit" disabled={busy || !name.trim()}>
              {confirmLabel}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

export function VaultsSection() {
  const sync = useLock((s) => s.sync)
  const [vaults, setVaults] = useState<VaultProfile[] | null>(null)
  const [creating, setCreating] = useState(false)
  const [renaming, setRenaming] = useState<VaultProfile | null>(null)
  const [deleting, setDeleting] = useState<VaultProfile | null>(null)
  const [busy, setBusy] = useState(false)

  const load = async () => {
    try {
      setVaults(await api.listVaults())
    } catch (e) {
      toast.error('Could not list vaults', { description: errorMessage(e) })
    }
  }
  // Reload when sync connects or disconnects, so the open vault's repository shows.
  useEffect(() => {
    void load()
  }, [sync?.repo])

  if (!vaults) return null

  const open = async (v: VaultProfile) => {
    setBusy(true)
    try {
      await api.switchVault(v.id)
      reloadIntoSettings()
    } catch (e) {
      toast.error('Could not open that vault', {
        description: errorMessage(e),
      })
      setBusy(false)
    }
  }

  return (
    <Section
      title="Vaults"
      description="Each vault has its own hosts, keys, passwords, snippets and master password, and syncs with its own GitHub repository. One GitHub sign-in works for all of them."
    >
      {vaults.map((v) => (
        <Field
          key={v.id}
          label={
            <span className="flex items-center gap-2">
              {v.name}
              {v.current && (
                <Badge variant="success">
                  <CheckIcon /> Open
                </Badge>
              )}
            </span>
          }
          hint={v.repo ? <span className="font-mono">{v.repo}</span> : 'Not synced'}
        >
          <div className="flex gap-1">
            <Button variant="ghost" size="icon-sm" aria-label={`Rename ${v.name}`} onClick={() => setRenaming(v)}>
              <PencilIcon />
            </Button>
            {!v.current && v.id !== 'default' && (
              <Button variant="ghost" size="icon-sm" aria-label={`Delete ${v.name}`} onClick={() => setDeleting(v)}>
                <Trash2Icon />
              </Button>
            )}
            {!v.current && (
              <Button variant="outline" size="sm" onClick={() => void open(v)} disabled={busy}>
                Open
              </Button>
            )}
          </div>
        </Field>
      ))}
      <Field label="New vault" hint="Starts empty. Pick its own repository in Sync below; you stay signed in to GitHub.">
        <Button size="sm" onClick={() => setCreating(true)} disabled={busy}>
          <PlusIcon /> New vault…
        </Button>
      </Field>

      <NameDialog
        key={`new-${creating}`}
        open={creating}
        onOpenChange={setCreating}
        title="New vault"
        description="Remvyn closes this vault (open terminals and forwards stop) and opens the new one. Switch back any time here."
        initial=""
        confirmLabel="Create and open"
        onSubmit={async (name) => {
          await api.createVault(name)
          reloadIntoSettings()
        }}
      />
      <NameDialog
        key={`rename-${renaming?.id}`}
        open={!!renaming}
        onOpenChange={(o) => !o && setRenaming(null)}
        title="Rename vault"
        description="The name is only shown on this computer."
        initial={renaming?.name ?? ''}
        confirmLabel="Rename"
        onSubmit={async (name) => {
          await api.renameVault(renaming!.id, name)
          await load()
        }}
      />
      <ConfirmDialog
        open={!!deleting}
        onOpenChange={(o) => !o && setDeleting(null)}
        title={`Delete “${deleting?.name}” from this computer?`}
        description={
          deleting?.repo
            ? `Its hosts, keys and passwords are erased here. The encrypted copy in ${deleting.repo} stays on GitHub, so you can join it again with its master password.`
            : 'Its hosts, keys and passwords are erased. It isn’t synced, so this can’t be undone.'
        }
        confirmLabel="Delete"
        destructive
        onConfirm={async () => {
          try {
            await api.deleteVault(deleting!.id)
            await load()
          } catch (e) {
            toast.error('Could not delete the vault', {
              description: errorMessage(e),
            })
          }
        }}
      />
    </Section>
  )
}
