import { BotIcon } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { api, errorMessage } from '@/lib/api'
import type { AgentDecision } from '@/lib/types'
import { useAgent } from '@/stores/agent'

const GRANT_MINUTES = 30

const detailLabel: Record<string, string> = {
  run_command: 'Command',
  read_file: 'File',
  write_file: 'File',
  list_dir: 'Directory',
}

/** Asks the user to allow an AI agent's action on a host in "ask" mode. */
export function AgentApprovalDialog() {
  const approvals = useAgent((s) => s.approvals)
  const current = approvals[0]
  const denyRef = useRef<HTMLButtonElement>(null)
  const now = useNow(!!current)

  const answer = async (decision: AgentDecision) => {
    if (!current) return
    useAgent.getState().removeApproval(current.id)
    try {
      await api.resolveAgentApproval(current.id, decision)
    } catch (e) {
      toast.error('Could not answer the request', { description: errorMessage(e) })
    }
  }

  const left = current ? Math.max(0, Math.round((current.expiresAt - now) / 1000)) : 0

  return (
    <Dialog open={!!current} onOpenChange={(o) => !o && void answer('deny')}>
      <DialogContent
        className="sm:max-w-xl"
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
                  <BotIcon className="size-5" />
                </div>
                <div className="min-w-0">
                  <DialogTitle>
                    {current.client} wants to {current.title}
                  </DialogTitle>
                  <DialogDescription>
                    on <span className="font-medium text-foreground">{current.hostLabel}</span> · <span className="font-mono">{current.remote}</span>
                  </DialogDescription>
                </div>
              </div>
            </DialogHeader>
            <div className="flex min-w-0 flex-col gap-1.5">
              <span className="text-xs text-muted-foreground">{detailLabel[current.tool] ?? 'Details'}</span>
              <pre
                data-selectable
                className="max-h-56 overflow-auto rounded-lg border bg-muted/40 p-3 font-mono text-[12.5px] leading-relaxed break-all whitespace-pre-wrap text-foreground"
              >
                {current.detail}
              </pre>
            </div>
            {current.preview && (
              <div className="flex min-w-0 flex-col gap-1.5">
                <span className="text-xs text-muted-foreground">New content</span>
                <pre
                  data-selectable
                  className="max-h-48 overflow-auto rounded-lg border bg-muted/40 p-3 font-mono text-[12px] leading-relaxed break-all whitespace-pre-wrap text-muted-foreground"
                >
                  {current.preview}
                </pre>
              </div>
            )}
            <p className="text-xs text-muted-foreground">
              Remvyn connects with the saved credentials; {current.client} only sees the result. Check what will run before allowing it.
            </p>
            <DialogFooter className="items-center">
              <span className="mr-auto text-xs text-muted-foreground tabular-nums">
                {approvals.length > 1 && `${approvals.length - 1} more waiting · `}
                Expires in {Math.floor(left / 60)}:{String(left % 60).padStart(2, '0')}
              </span>
              <Button ref={denyRef} variant="outline" onClick={() => void answer('deny')}>
                Deny
              </Button>
              <Button variant="outline" onClick={() => void answer('session')}>
                Allow for {GRANT_MINUTES} min
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
