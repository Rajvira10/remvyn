import {
  CheckIcon,
  CloudIcon,
  ExternalLinkIcon,
  LoaderIcon,
  LockIcon,
  PlusIcon,
  RefreshCwIcon,
  TriangleAlertIcon,
} from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { toast } from 'sonner'
import { BrowserOpenURL } from '../../../wailsjs/runtime/runtime'
import { ConfirmDialog } from '@/components/common/ConfirmDialog'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { api, copyText, errorCode, errorMessage } from '@/lib/api'
import type { DeviceCode, RepoInfo, SyncRepo, SyncStatus, VaultProfile } from '@/lib/types'
import { cn, relativeTime } from '@/lib/utils'
import { useLock } from '@/stores/lock'
import { useVault } from '@/stores/vault'
import { Field, Section } from './Section'
import { newPasswordError, PasswordInput, StrengthMeter } from './PasswordFields'

/** GitHub's mark (lucide no longer ships brand icons). */
function GithubIcon({ className }: { className?: string }) {
  return (
    <svg viewBox="0 0 16 16" fill="currentColor" aria-hidden className={cn('size-4', className)}>
      <path d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82.64-.18 1.32-.27 2-.27.68 0 1.36.09 2 .27 1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.013 8.013 0 0016 8c0-4.42-3.58-8-8-8z" />
    </svg>
  )
}

function Step({ n, children }: { n: number; children: React.ReactNode }) {
  return (
    <li className="flex gap-2.5">
      <span className="flex size-5 shrink-0 items-center justify-center rounded-full bg-muted text-[11px] font-semibold">{n}</span>
      <div className="min-w-0 flex-1 pt-px">{children}</div>
    </li>
  )
}

function LinkButton({ href, children }: { href: string; children: React.ReactNode }) {
  return (
    <Button type="button" variant="outline" size="xs" onClick={() => BrowserOpenURL(href)}>
      {children} <ExternalLinkIcon />
    </Button>
  )
}

/** One-time registration of the user's own GitHub App. */
function AppSetupDialog({ open, onOpenChange, status }: { open: boolean; onOpenChange: (o: boolean) => void; status: SyncStatus }) {
  const [clientId, setClientId] = useState(status.builtInApp ? '' : status.clientId)
  const [slug, setSlug] = useState(status.builtInApp ? '' : status.appSlug)
  const [busy, setBusy] = useState(false)
  const save = async () => {
    setBusy(true)
    try {
      useLock.getState().setSync(await api.syncSetApp(clientId, slug))
      toast.success('GitHub app saved')
      onOpenChange(false)
    } catch (e) {
      toast.error('Could not save', { description: errorMessage(e) })
    } finally {
      setBusy(false)
    }
  }
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>Set up the Remvyn GitHub app</DialogTitle>
          <DialogDescription>
            Sign-in uses a GitHub App that you register once. It only gets access to the one repository you choose, and Remvyn needs no server or secret.
          </DialogDescription>
        </DialogHeader>
        <ol className="flex flex-col gap-3 text-[13px]">
          <Step n={1}>
            <div className="flex flex-wrap items-center gap-2">
              Create a new GitHub App <LinkButton href="https://github.com/settings/apps/new">Open GitHub</LinkButton>
            </div>
          </Step>
          <Step n={2}>
            Give it any name and homepage URL. Under <b>Webhook</b>, untick <i>Active</i>.
          </Step>
          <Step n={3}>
            Tick <b>Enable Device Flow</b>, and untick <b>Expire user authorization tokens</b>.
          </Step>
          <Step n={4}>
            Under <b>Repository permissions</b>, set <b>Contents</b> to <i>Read and write</i>. Leave everything else as is.
          </Step>
          <Step n={5}>Create the app, then copy its Client ID and page address below.</Step>
        </ol>
        <div className="grid gap-3">
          <div className="grid gap-1.5">
            <Label htmlFor="gh-client">Client ID</Label>
            <Input id="gh-client" value={clientId} onChange={(e) => setClientId(e.target.value)} placeholder="Iv23li…" className="font-mono text-[12px]" />
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor="gh-slug">App page or name</Label>
            <Input id="gh-slug" value={slug} onChange={(e) => setSlug(e.target.value)} placeholder="https://github.com/apps/my-remvyn-sync" className="font-mono text-[12px]" />
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button onClick={() => void save()} disabled={busy || !clientId.trim() || !slug.trim()}>
            Save
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

type Stage = 'code' | 'repo' | 'password'

/** Sign in with GitHub, pick a repository, and create or join the vault. */
function ConnectDialog({ open, onOpenChange, relogin }: { open: boolean; onOpenChange: (o: boolean) => void; relogin: boolean }) {
  const sync = useLock((s) => s.sync)
  const lock = useLock((s) => s.status)
  const [stage, setStage] = useState<Stage>('code')
  const [code, setCode] = useState<DeviceCode | null>(null)
  const [codeError, setCodeError] = useState('')
  const [login, setLogin] = useState('')
  const [repos, setRepos] = useState<SyncRepo[] | null>(null)
  const [otherVaults, setOtherVaults] = useState<VaultProfile[]>([])
  const [installed, setInstalled] = useState(true)
  const [loadingRepos, setLoadingRepos] = useState(false)
  const [picked, setPicked] = useState('')
  const [info, setInfo] = useState<RepoInfo | null>(null)
  const [password, setPassword] = useState('')
  const [confirm, setConfirm] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const attempt = useRef(0)

  const loadRepos = async () => {
    setLoadingRepos(true)
    try {
      const [r, vaults] = await Promise.all([api.syncRepos(), api.listVaults().catch(() => [])])
      // Repositories already used by another vault on this computer.
      const others = vaults.filter((v) => !v.current && v.repo)
      setRepos(r.repos)
      setOtherVaults(others)
      setInstalled(r.installed)
      const free = r.repos.filter((x) => x.private && !others.some((v) => v.repo === x.fullName))
      const vault = free.find((x) => /remvyn|flux/i.test(x.fullName)) ?? (free.length === 1 ? free[0] : undefined)
      if (vault && !picked) setPicked(vault.fullName)
    } catch (e) {
      toast.error('Could not list repositories', { description: errorMessage(e) })
    } finally {
      setLoadingRepos(false)
    }
  }

  const begin = async () => {
    const my = ++attempt.current
    setCode(null)
    setCodeError('')
    try {
      const dc = await api.syncStartLogin()
      if (attempt.current !== my) return
      setCode(dc)
      const who = await api.syncFinishLogin()
      if (attempt.current !== my) return
      setLogin(who)
      if (relogin) {
        toast.success(`Signed in as ${who}`)
        useLock.getState().setSync(await api.syncStatus())
        onOpenChange(false)
        return
      }
      setStage('repo')
      void loadRepos()
    } catch (e) {
      if (attempt.current !== my || errorCode(e) === 'cancelled') return
      setCodeError(errorMessage(e))
    }
  }

  useEffect(() => {
    if (!open) return
    setStage('code')
    setRepos(null)
    setPicked('')
    setInfo(null)
    setPassword('')
    setConfirm('')
    setError('')
    // Already signed in: another vault on this computer shared its account.
    if (!relogin && sync?.signedIn) {
      setLogin(sync.login)
      setStage('repo')
      void loadRepos()
    } else {
      void begin()
    }
    return () => {
      attempt.current++
      void api.syncCancelLogin()
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open])

  const choose = async () => {
    setBusy(true)
    setError('')
    try {
      const i = await api.syncInspect(picked)
      if (!i.private) {
        setError('Choose a private repository. Everything is encrypted, but the vault shouldn’t be public.')
        return
      }
      setInfo(i)
      setStage('password')
    } catch (e) {
      setError(errorMessage(e))
    } finally {
      setBusy(false)
    }
  }

  const creating = !!info && !info.hasVault
  const needsNewPassword = creating && !lock?.protected
  const connect = async () => {
    if (needsNewPassword) {
      const err = newPasswordError(password, confirm, lock?.minLength ?? 8)
      if (err) return setError(err)
    }
    setBusy(true)
    setError('')
    try {
      const st = await api.syncConnect(picked, password)
      useLock.getState().setSync(st)
      useLock.getState().setStatus(await api.lockStatus())
      await useVault.getState().loadAll()
      if (st.error) toast.warning('Sync is on, but the first sync failed', { description: st.error })
      else toast.success(creating ? 'Sync is on. Your vault is now in GitHub.' : 'Sync is on. This computer joined your vault.')
      onOpenChange(false)
    } catch (e) {
      setError(errorCode(e) === 'wrong_password' ? (creating ? 'That isn’t your master password.' : 'That isn’t the master password for this vault.') : errorMessage(e))
    } finally {
      setBusy(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <GithubIcon className="size-4" /> {relogin ? 'Sign in to GitHub again' : 'Sync with GitHub'}
          </DialogTitle>
          <DialogDescription>
            {stage === 'code' && 'Approve Remvyn on github.com with this one-time code.'}
            {stage === 'repo' && `Signed in as ${login}. Choose the private repository that will hold your encrypted vault.`}
            {stage === 'password' && (info?.hasVault ? `${picked} already has a Remvyn vault.` : `Remvyn will create a new vault in ${picked}.`)}
          </DialogDescription>
        </DialogHeader>

        {stage === 'code' && (
          <div className="flex flex-col items-center gap-4 py-2">
            {codeError ? (
              <>
                <p className="text-center text-[13px] text-danger">{codeError}</p>
                <Button variant="outline" size="sm" onClick={() => void begin()}>
                  Try again
                </Button>
              </>
            ) : !code ? (
              <LoaderIcon className="size-5 animate-spin text-muted-foreground" />
            ) : (
              <>
                <div className="rounded-lg border bg-muted/40 px-5 py-3 font-mono text-2xl font-semibold tracking-[0.2em] select-all">{code.userCode}</div>
                <Button
                  onClick={async () => {
                    await copyText(code.userCode)
                    BrowserOpenURL(code.verificationUri)
                  }}
                >
                  Copy code and open GitHub <ExternalLinkIcon />
                </Button>
                <p className="flex items-center gap-2 text-xs text-muted-foreground">
                  <LoaderIcon className="size-3.5 animate-spin" /> Waiting for you to approve on GitHub…
                </p>
              </>
            )}
          </div>
        )}

        {stage === 'repo' && (
          <div className="flex flex-col gap-3">
            {repos === null ? (
              <div className="flex justify-center py-4">
                <LoaderIcon className="size-5 animate-spin text-muted-foreground" />
              </div>
            ) : (
              <>
                {repos.length > 0 && (
                  <div className="flex max-h-56 flex-col gap-1 overflow-y-auto rounded-lg border p-1" role="radiogroup" aria-label="Repository">
                    {repos.map((r) => {
                      const usedBy = otherVaults.find((v) => v.repo === r.fullName)
                      return (
                        <button
                          key={r.fullName}
                          role="radio"
                          aria-checked={picked === r.fullName}
                          disabled={!!usedBy}
                          title={usedBy ? `Already your “${usedBy.name}” vault; open it in Vaults above` : undefined}
                          onClick={() => setPicked(r.fullName)}
                          className={cn('flex items-center gap-2 rounded-md px-2.5 py-1.5 text-left text-[13px] hover:bg-accent disabled:pointer-events-none disabled:opacity-50', picked === r.fullName && 'bg-accent')}
                        >
                          <span className="flex-1 truncate font-mono text-[12px]">{r.fullName}</span>
                          {usedBy && <span className="truncate text-[11px] text-muted-foreground">vault “{usedBy.name}”</span>}
                          {!r.private && <span className="text-[11px] text-warning">public</span>}
                          {picked === r.fullName && <CheckIcon className="size-3.5 text-primary" />}
                        </button>
                      )
                    })}
                  </div>
                )}
                <div className="rounded-lg border border-dashed p-3 text-[12.5px]">
                  <p className="mb-2 text-muted-foreground">
                    {repos.length === 0
                      ? installed
                        ? 'Remvyn can’t see any repositories yet.'
                        : 'The Remvyn app isn’t installed on your account yet.'
                      : 'Don’t see the right one?'}
                  </p>
                  <ol className="flex flex-col gap-2">
                    <Step n={1}>
                      <div className="flex flex-wrap items-center gap-2">
                        Create an empty private repository
                        {sync?.newRepoUrl && (
                          <LinkButton href={sync.newRepoUrl}>
                            <PlusIcon /> New repository
                          </LinkButton>
                        )}
                      </div>
                    </Step>
                    <Step n={2}>
                      <div className="flex flex-wrap items-center gap-2">
                        Give Remvyn access to only that repository
                        {sync?.installUrl && <LinkButton href={sync.installUrl}>Choose repositories</LinkButton>}
                      </div>
                    </Step>
                    <Step n={3}>
                      <Button type="button" variant="ghost" size="xs" onClick={() => void loadRepos()} disabled={loadingRepos}>
                        <RefreshCwIcon className={cn(loadingRepos && 'animate-spin')} /> Check again
                      </Button>
                    </Step>
                  </ol>
                </div>
              </>
            )}
            {error && <p className="text-xs text-danger">{error}</p>}
            <DialogFooter>
              <Button variant="outline" onClick={() => onOpenChange(false)}>
                Cancel
              </Button>
              <Button onClick={() => void choose()} disabled={!picked || busy}>
                Continue
              </Button>
            </DialogFooter>
          </div>
        )}

        {stage === 'password' && info && (
          <form
            className="flex flex-col gap-3"
            onSubmit={(e) => {
              e.preventDefault()
              void connect()
            }}
          >
            {info.hasVault ? (
              <>
                <PasswordInput id="sync-pw" label="Master password from your other device" value={password} onChange={(v) => { setPassword(v); setError('') }} autoFocus autoComplete="current-password" />
                <p className="text-xs leading-relaxed text-muted-foreground">
                  Your hosts, keys and snippets on this computer are merged into the vault. This computer will use the same master password from now on.
                </p>
              </>
            ) : lock?.protected ? (
              <>
                <PasswordInput id="sync-pw" label="Your master password" value={password} onChange={(v) => { setPassword(v); setError('') }} autoFocus autoComplete="current-password" />
                <p className="text-xs leading-relaxed text-muted-foreground">
                  Everything is encrypted with it before upload; GitHub only stores ciphertext. Use the same password on your other devices.
                </p>
              </>
            ) : (
              <>
                <div className="flex flex-col gap-2">
                  <PasswordInput id="sync-pw" label="Create a master password" value={password} onChange={(v) => { setPassword(v); setError('') }} autoFocus />
                  <StrengthMeter password={password} />
                </div>
                <PasswordInput id="sync-pw2" label="Confirm master password" value={confirm} onChange={(v) => { setConfirm(v); setError('') }} />
                <p className="rounded-md bg-warning/10 px-3 py-2 text-xs leading-relaxed text-warning">
                  It encrypts the vault before upload, unlocks Remvyn on this computer, and lets your other devices join. It can’t be recovered if you forget it.
                </p>
              </>
            )}
            {error && <p className="text-xs text-danger">{error}</p>}
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setStage('repo')}>
                Back
              </Button>
              <Button type="submit" disabled={!password || busy}>
                {busy ? (info.hasVault ? 'Joining…' : 'Creating…') : info.hasVault ? 'Join vault' : 'Create vault and sync'}
              </Button>
            </DialogFooter>
          </form>
        )}
      </DialogContent>
    </Dialog>
  )
}

function stateLabel(s: SyncStatus): { text: string; tone: string } {
  switch (s.state) {
    case 'syncing':
      return { text: 'Syncing…', tone: 'text-muted-foreground' }
    case 'error':
      return { text: 'Sync failed', tone: 'text-danger' }
    case 'locked':
      return { text: 'Paused while Remvyn is locked', tone: 'text-muted-foreground' }
    default:
      return { text: s.lastSyncAt ? `Synced ${relativeTime(s.lastSyncAt)}` : 'Not synced yet', tone: 'text-success' }
  }
}

export function SyncSection() {
  const sync = useLock((s) => s.sync)
  const [setupOpen, setSetupOpen] = useState(false)
  const [connectOpen, setConnectOpen] = useState(false)
  const [relogin, setRelogin] = useState(false)
  const [confirmOff, setConfirmOff] = useState(false)
  const [busy, setBusy] = useState(false)
  const [, tick] = useState(0)

  // Keep "Synced 2m ago" fresh.
  useEffect(() => {
    const id = setInterval(() => tick((n) => n + 1), 30_000)
    return () => clearInterval(id)
  }, [])

  if (!sync) return null
  const label = stateLabel(sync)

  const syncNow = async () => {
    setBusy(true)
    try {
      useLock.getState().setSync(await api.syncNow())
    } catch (e) {
      useLock.getState().setSync(await api.syncStatus())
      if (errorCode(e) !== 'locked') toast.error('Sync failed', { description: errorMessage(e) })
    } finally {
      setBusy(false)
    }
  }

  return (
    <Section
      title="Sync"
      description="Keep hosts, keys, passwords, snippets, forwarding rules and known hosts in sync between your computers through a private GitHub repository. Everything is encrypted with your master password first."
    >
      {!sync.appConfigured ? (
        <Field label="GitHub app" hint="Sync signs in through a GitHub App you register once. It takes about two minutes.">
          <Button size="sm" onClick={() => setSetupOpen(true)}>
            Set up…
          </Button>
        </Field>
      ) : !sync.connected ? (
        <Field
          label={
            <span className="flex items-center gap-1.5">
              <CloudIcon className="size-3.5 text-muted-foreground" /> Not syncing
            </span>
          }
          hint="Sign in with GitHub, choose a private repository, and Remvyn does the rest."
        >
          <Button
            size="sm"
            onClick={() => {
              setRelogin(false)
              setConnectOpen(true)
            }}
          >
            <GithubIcon /> Sign in with GitHub
          </Button>
        </Field>
      ) : (
        <>
          <Field
            label={
              <span className="flex items-center gap-1.5">
                <GithubIcon className="size-3.5" />
                <button className="font-mono text-[12.5px] hover:underline" onClick={() => BrowserOpenURL(sync.repoUrl)}>
                  {sync.repo}
                </button>
              </span>
            }
            hint={sync.login ? `Signed in as ${sync.login}` : undefined}
          >
            <div className="flex gap-2">
              <Button variant="outline" size="sm" onClick={() => void syncNow()} disabled={busy || sync.state === 'syncing' || sync.state === 'locked'}>
                <RefreshCwIcon className={cn((busy || sync.state === 'syncing') && 'animate-spin')} /> Sync now
              </Button>
              <Button variant="ghost" size="sm" onClick={() => setConfirmOff(true)}>
                Disconnect
              </Button>
            </div>
          </Field>
          <div className="flex items-start gap-2.5 px-4 py-3 text-xs">
            {sync.state === 'error' ? (
              <TriangleAlertIcon className="mt-px size-3.5 shrink-0 text-danger" />
            ) : sync.state === 'locked' ? (
              <LockIcon className="mt-px size-3.5 shrink-0 text-muted-foreground" />
            ) : sync.state === 'syncing' ? (
              <LoaderIcon className="mt-px size-3.5 shrink-0 animate-spin text-muted-foreground" />
            ) : (
              <CheckIcon className="mt-px size-3.5 shrink-0 text-success" />
            )}
            <div className="flex min-w-0 flex-1 flex-col gap-1">
              <span className={label.tone}>{label.text}</span>
              {sync.state === 'error' && <span className="text-muted-foreground">{sync.error}</span>}
              {sync.state !== 'error' && <span className="text-muted-foreground">Remvyn syncs a few seconds after you change something, every minute, and when you switch back to it.</span>}
            </div>
            {sync.errorCode === 'github_auth' && (
              <Button
                size="xs"
                onClick={() => {
                  setRelogin(true)
                  setConnectOpen(true)
                }}
              >
                Sign in again
              </Button>
            )}
          </div>
        </>
      )}
      {sync.appConfigured && !sync.builtInApp && !sync.connected && (
        <Field label="GitHub app" hint={<span className="font-mono">{sync.appSlug || sync.clientId}</span>}>
          <Button variant="ghost" size="sm" onClick={() => setSetupOpen(true)}>
            Change…
          </Button>
        </Field>
      )}
      <AppSetupDialog key={String(setupOpen)} open={setupOpen} onOpenChange={setSetupOpen} status={sync} />
      <ConnectDialog open={connectOpen} onOpenChange={setConnectOpen} relogin={relogin} />
      <ConfirmDialog
        open={confirmOff}
        onOpenChange={setConfirmOff}
        title="Turn off sync on this computer?"
        description="Your data stays on this computer and in the GitHub repository; they just stop syncing. Your master password stays on."
        confirmLabel="Disconnect"
        onConfirm={async () => {
          try {
            useLock.getState().setSync(await api.syncDisconnect())
            useLock.getState().setStatus(await api.lockStatus())
          } catch (e) {
            toast.error('Could not disconnect', { description: errorMessage(e) })
          }
        }}
      />
    </Section>
  )
}
