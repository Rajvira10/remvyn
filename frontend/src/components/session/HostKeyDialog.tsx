import { ShieldAlertIcon, ShieldQuestionIcon } from 'lucide-react'
import { useEffect, useState } from 'react'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { api, events, onEvent } from '@/lib/api'
import type { HostKeyPrompt } from '@/lib/types'
import { useVault } from '@/stores/vault'

/** Queues host-key prompts from the backend and asks the user to decide. */
export function HostKeyDialog() {
  const [queue, setQueue] = useState<HostKeyPrompt[]>([])
  useEffect(() => {
    const offPrompt = onEvent<HostKeyPrompt>(events.HOSTKEY_PROMPT, (p) => setQueue((q) => [...q, p]))
    const offResolved = onEvent<string>(events.HOSTKEY_RESOLVED, (id) => setQueue((q) => q.filter((p) => p.id !== id)))
    return () => {
      offPrompt()
      offResolved()
    }
  }, [])
  const current = queue[0]
  const answer = async (accept: boolean) => {
    if (!current) return
    setQueue((q) => q.slice(1))
    await api.resolveHostKey(current.id, accept)
    if (accept) void useVault.getState().loadKnownHosts()
  }
  return (
    <Dialog open={!!current} onOpenChange={(o) => !o && void answer(false)}>
      <DialogContent className="sm:max-w-lg" onPointerDownOutside={(e) => e.preventDefault()}>
        {current && (
          <>
            <DialogHeader>
              <div className="flex items-center gap-3">
                {current.changed ? <ShieldAlertIcon className="size-7 text-danger" /> : <ShieldQuestionIcon className="size-7 text-warning" />}
                <DialogTitle>{current.changed ? 'Warning: host key has changed' : 'Verify host key'}</DialogTitle>
              </div>
              <DialogDescription>
                {current.changed
                  ? `The key presented by ${current.host} is different from the one you trusted before. This can mean someone is intercepting the connection (man-in-the-middle), or the server was reinstalled. Only continue if you know why the key changed.`
                  : `This is the first connection to ${current.host}. Compare the fingerprint with the one your server administrator gave you (ssh-keygen -lf /etc/ssh/ssh_host_*_key.pub).`}
              </DialogDescription>
            </DialogHeader>
            <div className="flex flex-col gap-2 rounded-lg border bg-muted/40 p-3 text-xs">
              <div className="flex justify-between gap-4">
                <span className="text-muted-foreground">Host</span>
                <span className="font-mono">{current.host}</span>
              </div>
              <div className="flex justify-between gap-4">
                <span className="text-muted-foreground">Key type</span>
                <span className="font-mono">{current.keyType}</span>
              </div>
              <div className="flex flex-col gap-1">
                <span className="text-muted-foreground">{current.changed ? 'New fingerprint' : 'Fingerprint'}</span>
                <span className="font-mono break-all text-foreground" data-selectable>
                  {current.fingerprint}
                </span>
              </div>
              {current.oldFingerprint && (
                <div className="flex flex-col gap-1">
                  <span className="text-muted-foreground">Previously trusted</span>
                  <span className="font-mono break-all text-muted-foreground line-through" data-selectable>
                    {current.oldFingerprint}
                  </span>
                </div>
              )}
            </div>
            <DialogFooter>
              <Button variant="outline" onClick={() => void answer(false)}>
                {current.changed ? 'Disconnect' : 'Cancel'}
              </Button>
              <Button variant={current.changed ? 'destructive' : 'default'} onClick={() => void answer(true)}>
                {current.changed ? 'Trust new key and connect' : 'Trust and connect'}
              </Button>
            </DialogFooter>
          </>
        )}
      </DialogContent>
    </Dialog>
  )
}
