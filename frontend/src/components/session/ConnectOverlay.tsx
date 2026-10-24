import { AlertTriangleIcon, KeyRoundIcon, LockIcon, PlugZapIcon, RotateCwIcon, XIcon } from 'lucide-react'
import { useState } from 'react'
import { Spinner } from '@/components/common/Spinner'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import type { Tab } from '@/stores/sessions'
import { useUI } from '@/stores/ui'
import { useSessions } from '@/stores/sessions'
import type { Host } from '@/lib/types'

export interface Credentials {
  password: string
  savePassword: boolean
  passphrase: string
}

/** Connection status, credential prompts and errors shown over a session. */
export function ConnectOverlay({
  tab,
  host,
  onSubmitCredentials,
  onCancel,
}: {
  tab: Tab
  host: Host | undefined
  onSubmitCredentials: (c: Credentials) => void
  onCancel: () => void
}) {
  const { reconnect, close, activate } = useSessions()
  const editHost = useUI((s) => s.editHost)
  const target = host ? `${host.username ? host.username + '@' : ''}${host.hostname}${host.port !== 22 ? ':' + host.port : ''}` : ''

  if (tab.state === 'connected') return null

  return (
    <div className="absolute inset-0 z-10 flex items-center justify-center bg-background/85 backdrop-blur-sm">
      <div className="w-full max-w-sm rounded-xl border bg-card p-6 shadow-2xl">
        {tab.state === 'connecting' && (
          <div className="flex flex-col items-center gap-3 text-center">
            <Spinner className="size-6" />
            <div className="font-medium">Connecting to {tab.title}</div>
            <div className="font-mono text-xs text-muted-foreground">{target}</div>
            <Button variant="outline" size="sm" onClick={onCancel}>
              Cancel
            </Button>
          </div>
        )}
        {tab.state === 'auth' && <CredentialForm tab={tab} host={host} onSubmit={onSubmitCredentials} onCancel={() => close(tab.key)} />}
        {(tab.state === 'error' || tab.state === 'closed') && (
          <div className="flex flex-col items-center gap-3 text-center">
            {tab.state === 'error' ? <AlertTriangleIcon className="size-7 text-danger" /> : <PlugZapIcon className="size-7 text-muted-foreground" />}
            <div className="font-medium">{tab.state === 'error' ? 'Connection failed' : 'Session ended'}</div>
            <div className="max-w-xs text-xs break-words text-muted-foreground" data-selectable>
              {tab.state === 'error'
                ? tab.error
                : tab.error
                  ? tab.error
                  : `The remote shell exited${tab.exitCode !== undefined && tab.exitCode >= 0 ? ` with status ${tab.exitCode}` : ''}.`}
            </div>
            <div className="mt-1 flex gap-2">
              <Button size="sm" onClick={() => reconnect(tab.key)}>
                <RotateCwIcon /> Reconnect
              </Button>
              {tab.state === 'error' && host && (
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => {
                    activate('vault')
                    editHost(host.id)
                  }}
                >
                  Edit host
                </Button>
              )}
              <Button variant="ghost" size="sm" onClick={() => close(tab.key)}>
                <XIcon /> Close
              </Button>
            </div>
          </div>
        )}
      </div>
    </div>
  )
}

function CredentialForm({
  tab,
  host,
  onSubmit,
  onCancel,
}: {
  tab: Tab
  host: Host | undefined
  onSubmit: (c: Credentials) => void
  onCancel: () => void
}) {
  const passphrase = tab.errorCode === 'passphrase_required'
  const [value, setValue] = useState('')
  const [save, setSave] = useState(true)
  const failed = tab.errorCode === 'auth_failed'
  const submit = () => {
    if (!value) return
    onSubmit(passphrase ? { password: '', savePassword: false, passphrase: value } : { password: value, savePassword: save, passphrase: '' })
  }
  return (
    <div className="flex flex-col gap-4">
      <div className="flex items-center gap-3">
        <div className="flex size-9 items-center justify-center rounded-lg bg-primary/15">
          {passphrase ? <KeyRoundIcon className="size-4 text-primary" /> : <LockIcon className="size-4 text-primary" />}
        </div>
        <div>
          <div className="font-medium">{passphrase ? 'Key passphrase' : 'Password'}</div>
          <div className="font-mono text-xs text-muted-foreground">
            {host?.username || 'user'}@{host?.hostname}
          </div>
        </div>
      </div>
      {failed && <div className="rounded-md bg-danger/10 px-2 py-1.5 text-xs text-danger">{tab.error}</div>}
      <div className="flex flex-col gap-1.5">
        <Label htmlFor={`cred-${tab.key}`}>{passphrase ? 'Passphrase for the selected key' : 'Password'}</Label>
        <Input
          id={`cred-${tab.key}`}
          type="password"
          autoFocus
          value={value}
          onChange={(e) => setValue(e.target.value)}
          onKeyDown={(e) => e.key === 'Enter' && submit()}
        />
      </div>
      {!passphrase && (
        <Label className="flex items-center gap-2 font-normal">
          <Checkbox checked={save} onCheckedChange={(c) => setSave(c === true)} />
          Save password (encrypted on this device)
        </Label>
      )}
      <div className="flex justify-end gap-2">
        <Button variant="ghost" size="sm" onClick={onCancel}>
          Cancel
        </Button>
        <Button size="sm" disabled={!value} onClick={submit}>
          Connect
        </Button>
      </div>
    </div>
  )
}
