import { BotIcon, KeyRoundIcon } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { api, errorMessage, events, onEvent } from '@/lib/api'
import type { AgentDecision, KeyUseRequest } from '@/lib/types'

/** Asks before a server uses one of the keychain's keys over agent forwarding. */
export function KeyUseDialog() {
  const [queue, setQueue] = useState<KeyUseRequest[]>([])
  const denyRef = useRef<HTMLButtonElement>(null)

  useEffect(() => {
    void api.pendingKeyUses().then((list) => setQueue((q) => [...q, ...list.filter((r) => !q.some((x) => x.id === r.id))]))
    const offReq = onEvent<KeyUseRequest>(events.KEYUSE_REQUEST, (r) => setQueue((q) => [...q, r]))
    const offDone = onEvent<string>(events.KEYUSE_DONE, (id) => setQueue((q) => q.filter((r) => r.id !== id)))
    return () => {
      offReq()
      offDone()
    }
  }, [])

  const current = queue[0]
  const now = useNow(!!current)
  const left = current ? Math.max(0, Math.round((current.expiresAt - now) / 1000)) : 0

  const answer = async (decision: AgentDecision) => {
    if (!current) return
    setQueue((q) => q.slice(1))
    try {
      await api.resolveKeyUse(current.id, decision)
    } catch (e) {
      toast.error('Could not answer the request', { description: errorMessage(e) })
    }
  }

  return (
    <Dialog open={!!current} onOpenChange={(o) => !o && void answer('deny')}>
      <DialogContent
        className="sm:max-w-lg"
        onPointerDownOutside={(e) => e.preventDefault()}
        onOpenAutoFocus={(e) => {
          // Focus Deny so a stray Enter can never approve.
          e.preventDefault()
          denyRef.current?.focus()
        }}
      >
        {current && (
          <>
            <DialogHeader>
              <div className="flex items-center gap-3">
                <div className="flex size-9 shrink-0 items-center justify-center rounded-lg bg-primary/15 text-primary">
                  {current.ai ? <BotIcon className="size-5" /> : <KeyRoundIcon className="size-5" />}
                </div>
                <div className="min-w-0">
                  <DialogTitle>
                    {current.ai ? `${current.source} on ${current.hostLabel}` : current.hostLabel} wants to use your key “{current.keyName}”
                  </DialogTitle>
                  <DialogDescription className="font-mono">{current.remote}</DialogDescription>
                </div>
              </div>
            </DialogHeader>
            <div className="flex flex-col gap-1 rounded-lg border bg-muted/40 p-3 text-xs">
              <span className="text-muted-foreground">Key fingerprint</span>
              <span className="font-mono break-all" data-selectable>
                {current.fingerprint}
              </span>
            </div>
            <p className="text-xs text-muted-foreground">
              {current.ai
                ? 'A command run by the AI agent on this server is trying to log in somewhere (for example git or ssh) with your key. Only allow it if you asked for that.'
                : 'Something on this server, usually git or ssh, is trying to log in somewhere with your key. The key itself never leaves Remvyn. If you didn’t just run such a command, deny it.'}
            </p>
            <DialogFooter className="items-center">
              <span className="mr-auto text-xs text-muted-foreground tabular-nums">
                {queue.length > 1 && `${queue.length - 1} more waiting · `}
                Denied in {left}s
              </span>
              <Button ref={denyRef} variant="outline" onClick={() => void answer('deny')}>
                Deny
              </Button>
              <Button variant="outline" onClick={() => void answer('session')}>
                Allow for this connection
              </Button>
              <Button onClick={() => void answer('once')}>Allow once</Button>
            </DialogFooter>
          </>
        )}
      </DialogContent>
    </Dialog>
  )
}

function useNow(active: boolean) {
  const [now, setNow] = useState(Date.now)
  useEffect(() => {
    if (!active) return
    setNow(Date.now())
    const t = setInterval(() => setNow(Date.now()), 1000)
    return () => clearInterval(t)
  }, [active])
  return now
}
