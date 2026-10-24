import { KeyRoundIcon, LockIcon, ShieldCheckIcon } from 'lucide-react'
import { useState } from 'react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Kbd } from '@/components/ui/kbd'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Tooltip } from '@/components/ui/tooltip'
import { api, errorCode, errorMessage } from '@/lib/api'
import type { Settings } from '@/lib/types'
import { appKey } from '@/lib/utils'
import { useLock } from '@/stores/lock'
import { Field, Section } from './Section'
import { newPasswordError, PasswordInput, StrengthMeter } from './PasswordFields'

type Mode = 'set' | 'change' | 'remove'

const AUTO_LOCK = [
  [0, 'Never'],
  [1, 'After 1 minute'],
  [5, 'After 5 minutes'],
  [15, 'After 15 minutes'],
  [30, 'After 30 minutes'],
  [60, 'After 1 hour'],
  [240, 'After 4 hours'],
] as const

function PasswordDialog({ mode, onClose }: { mode: Mode | null; onClose: () => void }) {
  const status = useLock((s) => s.status)
  const [current, setCurrent] = useState('')
  const [next, setNext] = useState('')
  const [confirm, setConfirm] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const min = status?.minLength ?? 8

  const reset = () => {
    setCurrent('')
    setNext('')
    setConfirm('')
    setError('')
  }
  const close = () => {
    reset()
    onClose()
  }

  const submit = async () => {
    if (mode !== 'remove') {
      const err = newPasswordError(next, confirm, min)
      if (err) return setError(err)
    }
    setBusy(true)
    setError('')
    try {
      const st = mode === 'remove' ? await api.removeMasterPassword(current) : await api.setMasterPassword(current, next)
      useLock.getState().setStatus(st)
      toast.success(mode === 'set' ? 'Master password set' : mode === 'change' ? 'Master password changed' : 'Master password removed')
      close()
    } catch (e) {
      setError(errorCode(e) === 'wrong_password' ? 'Your current password is not correct.' : errorMessage(e))
    } finally {
      setBusy(false)
    }
  }

  const titles: Record<Mode, string> = { set: 'Set a master password', change: 'Change master password', remove: 'Remove master password' }
  return (
    <Dialog open={mode !== null} onOpenChange={(o) => !o && close()}>
      <DialogContent>
        {mode && (
          <form
            className="flex flex-col gap-4"
            onSubmit={(e) => {
              e.preventDefault()
              void submit()
            }}
          >
            <DialogHeader>
              <DialogTitle>{titles[mode]}</DialogTitle>
              <DialogDescription>
                {mode === 'set' && 'Flux will ask for it when it starts and after it auto-locks. It also encrypts your vault for sync.'}
                {mode === 'change' && (status?.syncConnected
                  ? 'The synced vault on GitHub is updated too. Your other devices keep syncing; they use the new password the next time they join.'
                  : 'Your saved passwords and keys stay as they are; only the password that unlocks them changes.')}
                {mode === 'remove' && 'Flux will open without a password, and anyone using this computer can see your saved passwords and keys.'}
              </DialogDescription>
            </DialogHeader>
            {mode !== 'set' && (
              <PasswordInput id="mp-current" label="Current password" value={current} onChange={setCurrent} autoFocus autoComplete="current-password" />
            )}
            {mode !== 'remove' && (
              <>
                <div className="flex flex-col gap-2">
                  <PasswordInput id="mp-new" label="New password" value={next} onChange={(v) => { setNext(v); setError('') }} autoFocus={mode === 'set'} />
                  <StrengthMeter password={next} />
                </div>
                <PasswordInput id="mp-confirm" label="Confirm new password" value={confirm} onChange={(v) => { setConfirm(v); setError('') }} />
                <p className="rounded-md bg-warning/10 px-3 py-2 text-xs leading-relaxed text-warning">
                  There’s no way to recover a forgotten master password. Use a passphrase you’ll remember, or keep it in a password manager.
                </p>
              </>
            )}
            {error && <p className="text-xs text-danger">{error}</p>}
            <DialogFooter>
              <Button type="button" variant="outline" onClick={close}>
                Cancel
              </Button>
              <Button type="submit" variant={mode === 'remove' ? 'destructive' : 'default'} disabled={busy || (mode !== 'set' && !current)}>
                {busy ? 'Saving…' : mode === 'remove' ? 'Remove password' : 'Save password'}
              </Button>
            </DialogFooter>
          </form>
        )}
      </DialogContent>
    </Dialog>
  )
}

export function SecuritySection({ form, apply }: { form: Settings; apply: (patch: Partial<Settings>) => Promise<void> }) {
  const status = useLock((s) => s.status)
  const [mode, setMode] = useState<Mode | null>(null)
  const protectedVault = !!status?.protected

  return (
    <Section title="Security">
      <Field
        label={
          <span className="flex items-center gap-1.5">
            Master password
            {protectedVault ? <ShieldCheckIcon className="size-3.5 text-success" /> : null}
          </span>
        }
        hint={
          protectedVault
            ? 'Required when Flux starts and after it locks.'
            : 'Not set. Anyone who can use this computer can open Flux and read your saved passwords.'
        }
      >
        {protectedVault ? (
          <div className="flex gap-2">
            <Button variant="outline" size="sm" onClick={() => setMode('change')}>
              Change…
            </Button>
            <Tooltip content={status?.syncConnected ? 'Sync needs a master password. Disconnect sync first.' : undefined}>
              <span>
                <Button variant="ghost" size="sm" disabled={status?.syncConnected} onClick={() => setMode('remove')}>
                  Remove
                </Button>
              </span>
            </Tooltip>
          </div>
        ) : (
          <Button size="sm" onClick={() => setMode('set')}>
            <KeyRoundIcon /> Set password…
          </Button>
        )}
      </Field>
      {protectedVault && (
        <>
          <Field label="Auto-lock" hint="Lock Flux when you haven’t used it for a while. Open terminals keep running.">
            <Select value={String(form.autoLockMinutes)} onValueChange={(v) => void apply({ autoLockMinutes: Number(v) })}>
              <SelectTrigger className="w-40">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {AUTO_LOCK.map(([v, label]) => (
                  <SelectItem key={v} value={String(v)}>
                    {label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </Field>
          <Field label="Lock now" hint={<span>Or press <Kbd>{appKey}L</Kbd> anywhere.</span>}>
            <Button variant="outline" size="sm" onClick={() => void api.lock()}>
              <LockIcon /> Lock
            </Button>
          </Field>
        </>
      )}
      <div className="flex gap-3 px-4 py-3 text-xs leading-relaxed text-muted-foreground">
        <LockIcon className="mt-0.5 size-4 shrink-0 text-success" />
        <div className="flex flex-col gap-2">
          <p>
            Passwords, private keys and key passphrases are encrypted with AES-256-GCM before being written to the local database.{' '}
            {protectedVault
              ? 'The encryption key is itself encrypted with a key derived from your master password (Argon2id), and is only in memory while Flux is unlocked.'
              : 'The encryption key is stored in a separate file readable only by your user account.'}{' '}
            Host names and other non-secret details are not encrypted.
          </p>
          <p>
            Server host keys are verified on every connection. New servers require you to confirm their fingerprint, and a changed key is flagged as a possible
            man-in-the-middle attack.
          </p>
        </div>
      </div>
      <PasswordDialog mode={mode} onClose={() => setMode(null)} />
    </Section>
  )
}
