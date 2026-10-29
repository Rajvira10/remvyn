import { EyeIcon, EyeOffIcon, LockIcon } from 'lucide-react'
import { Logo } from '@/components/common/Logo'
import { useEffect, useRef, useState } from 'react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { api, errorCode, errorMessage } from '@/lib/api'
import type { VaultProfile } from '@/lib/types'
import { useLock } from '@/stores/lock'
import { useSessions } from '@/stores/sessions'
import { useSettings } from '@/stores/settings'
import { useVault } from '@/stores/vault'

/** Full-window lock. Sessions stay open underneath but receive no input. */
export function LockScreen() {
  const status = useLock((s) => s.status)
  const sync = useLock((s) => s.sync)
  const [password, setPassword] = useState('')
  const [show, setShow] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [forgot, setForgot] = useState(false)
  const [vaults, setVaults] = useState<VaultProfile[]>([])
  const input = useRef<HTMLInputElement>(null)

  useEffect(() => {
    if (!status?.locked) return
    setPassword('')
    setError('')
    // Take focus away from any terminal.
    ;(document.activeElement as HTMLElement | null)?.blur()
    const id = setTimeout(() => input.current?.focus(), 30)
    api.listVaults().then(setVaults, () => setVaults([]))
    return () => clearTimeout(id)
  }, [status?.locked])

  if (!status?.locked) return null
  // With several vaults, say which one is locked (each has its own password).
  const current = vaults.length > 1 ? vaults.find((v) => v.current) : undefined
  const others = current ? vaults.filter((v) => !v.current) : []

  const unlock = async () => {
    if (!password || busy) return
    setBusy(true)
    setError('')
    try {
      await api.unlock(password)
      setPassword('')
      useLock.getState().setStatus(await api.lockStatus())
      await useVault.getState().loadAll()
    } catch (e) {
      setError(errorCode(e) === 'wrong_password' ? 'That password is not correct.' : errorMessage(e))
      setTimeout(() => input.current?.select(), 0)
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="fixed inset-0 z-[60] flex items-center justify-center bg-background" role="dialog" aria-modal aria-label="Remvyn is locked">
      <div className="titlebar-drag absolute inset-x-0 top-0 h-[50px]" />
      <form
        className="flex w-80 flex-col items-center gap-5"
        onSubmit={(e) => {
          e.preventDefault()
          void unlock()
        }}
      >
        <div className="relative flex size-14 items-center justify-center rounded-2xl border bg-card shadow-sm">
          <Logo className="size-6 text-primary" />
          <span className="absolute -right-1.5 -bottom-1.5 flex size-6 items-center justify-center rounded-full border bg-background">
            <LockIcon className="size-3 text-muted-foreground" />
          </span>
        </div>
        <div className="text-center">
          <h1 className="text-[16px] font-semibold">Remvyn is locked</h1>
          <p className="mt-1 text-xs text-muted-foreground">
            {current ? `Enter the master password for the “${current.name}” vault.` : 'Enter your master password to continue.'}
          </p>
        </div>
        <div className="flex w-full flex-col gap-2">
          <div className="relative">
            <input
              ref={input}
              type={show ? 'text' : 'password'}
              value={password}
              onChange={(e) => {
                setPassword(e.target.value)
                setError('')
              }}
              placeholder="Master password"
              autoComplete="current-password"
              spellCheck={false}
              aria-invalid={!!error}
              className="h-10 w-full rounded-lg border bg-card px-3 pr-10 text-[13px] outline-none focus-visible:ring-2 focus-visible:ring-ring/60 aria-invalid:border-danger"
            />
            <button
              type="button"
              tabIndex={-1}
              onClick={() => setShow((v) => !v)}
              aria-label={show ? 'Hide password' : 'Show password'}
              className="absolute top-1/2 right-2.5 -translate-y-1/2 text-muted-foreground hover:text-foreground"
            >
              {show ? <EyeOffIcon className="size-4" /> : <EyeIcon className="size-4" />}
            </button>
          </div>
          {error && <p className="text-xs text-danger">{error}</p>}
          <Button type="submit" disabled={!password || busy} className="h-9">
            {busy ? 'Unlocking…' : 'Unlock'}
          </Button>
        </div>
        <button type="button" className="text-xs text-muted-foreground hover:text-foreground" onClick={() => setForgot(true)}>
          Forgot your password?
        </button>
        {others.length > 0 && (
          <div className="flex flex-wrap items-center justify-center gap-1.5 text-xs text-muted-foreground">
            Open another vault:
            {others.map((v) => (
              <Button
                key={v.id}
                type="button"
                variant="outline"
                size="xs"
                onClick={async () => {
                  try {
                    await api.switchVault(v.id)
                    window.location.reload()
                  } catch (e) {
                    toast.error('Could not open that vault', { description: errorMessage(e) })
                  }
                }}
              >
                {v.name}
              </Button>
            ))}
          </div>
        )}
      </form>
      {forgot && (
        // Inline rather than a dialog: dialogs sit below the lock screen.
        <div className="absolute inset-0 flex items-center justify-center bg-black/50">
          <div className="flex w-[26rem] flex-col gap-3 rounded-xl border bg-popover p-5 shadow-2xl">
            <h2 className="text-[15px] font-semibold">Reset Remvyn?</h2>
            <p className="text-[13px] leading-relaxed text-muted-foreground">
              {sync?.connected
                ? 'Your master password can’t be recovered. Resetting erases everything on this computer and turns off sync. The copy on GitHub is kept, but joining it again still needs the password.'
                : 'Your master password can’t be recovered, so saved passwords and keys can’t be decrypted. Resetting erases all hosts, keys, snippets and settings on this computer so you can start over.'}
            </p>
            <div className="flex justify-end gap-2">
              <Button variant="outline" size="sm" onClick={() => setForgot(false)}>
                Cancel
              </Button>
              <Button
                variant="destructive"
                size="sm"
                onClick={async () => {
                  try {
                    const s = await api.resetVault()
                    useSettings.getState().setLocal(s)
                    for (const t of useSessions.getState().tabs) useSessions.getState().close(t.key)
                    await Promise.all([useLock.getState().load(), useVault.getState().loadAll()])
                    setForgot(false)
                    toast.success('Remvyn was reset')
                  } catch (e) {
                    toast.error('Could not reset', { description: errorMessage(e) })
                  }
                }}
              >
                Erase and reset
              </Button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
