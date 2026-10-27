import { ExternalLinkIcon, RefreshCwIcon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { api } from '@/lib/api'
import { cn, formatBytes } from '@/lib/utils'
import { useSessions } from '@/stores/sessions'
import { useUpdate } from '@/stores/update'

/** Shows the new release and runs download → install → restart. */
export function UpdateDialog() {
  const { status, dialogOpen, setDialogOpen, install } = useUpdate()
  const openTabs = useSessions((s) => s.tabs.length)
  if (!status) return null

  const { latest, state } = status
  const working = state === 'downloading' || state === 'installing' || state === 'restarting'
  const pct = status.total > 0 ? Math.min(100, (status.downloaded / status.total) * 100) : 0

  let title = 'Remvyn is up to date'
  if (working) title = state === 'restarting' ? 'Restarting Remvyn…' : `Updating to ${latest?.version}…`
  else if (status.available) title = `Remvyn ${latest?.version} is available`

  return (
    <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>
            You have {status.current}
            {latest?.publishedAt && status.available && <> · released {new Date(latest.publishedAt).toLocaleDateString()}</>}
          </DialogDescription>
        </DialogHeader>

        {status.available && latest?.notes && (
          <div className="max-h-64 overflow-auto rounded-lg border bg-muted/40 p-3 text-xs leading-relaxed whitespace-pre-wrap text-muted-foreground">{plainNotes(latest.notes)}</div>
        )}

        {working && (
          <div className="flex flex-col gap-1.5">
            <div className="h-1.5 overflow-hidden rounded-full bg-muted">
              <div
                className={cn('h-full rounded-full bg-primary transition-[width]', state !== 'downloading' && 'animate-pulse')}
                style={{ width: `${state === 'downloading' ? pct : 100}%` }}
              />
            </div>
            <div className="text-xs text-muted-foreground tabular-nums">
              {state === 'downloading' && `Downloading ${formatBytes(status.downloaded)} of ${formatBytes(status.total)}`}
              {state === 'installing' && 'Installing…'}
              {state === 'restarting' && 'Remvyn will open again in a moment.'}
            </div>
          </div>
        )}

        {state === 'error' && status.error && <div className="rounded-lg border border-danger/40 bg-danger/10 p-3 text-xs text-danger">{status.error}</div>}

        {status.available && !working && status.manual && <div className="rounded-lg border p-3 text-xs text-muted-foreground">{status.manual}</div>}

        {status.available && !working && !status.manual && openTabs > 0 && (
          <p className="text-xs text-muted-foreground">
            Remvyn restarts to finish the update; your {openTabs === 1 ? 'open session' : `${openTabs} open sessions`} will be disconnected.
          </p>
        )}

        <DialogFooter>
          {status.available && !working ? (
            <>
              <Button variant="outline" onClick={() => setDialogOpen(false)}>
                Later
              </Button>
              {status.manual ? (
                <Button onClick={() => void api.openReleasePage()}>
                  <ExternalLinkIcon /> Open release page
                </Button>
              ) : (
                <Button onClick={() => void install()}>
                  <RefreshCwIcon /> {state === 'error' ? 'Try again' : 'Update and restart'}
                </Button>
              )}
            </>
          ) : (
            <Button variant="outline" disabled={state === 'restarting'} onClick={() => setDialogOpen(false)}>
              {working ? 'Hide' : 'Close'}
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

/** Release notes are Markdown; show them as plain text without the markup. */
function plainNotes(md: string): string {
  return md
    .replace(/\r\n/g, '\n')
    .replace(/^#{1,6}\s+/gm, '')
    .replace(/^(\s*)[-*]\s+/gm, '$1• ')
    .replace(/\*\*(.+?)\*\*/g, '$1')
    .replace(/`([^`\n]+)`/g, '$1')
    .replace(/\[([^\]\n]+)\]\([^)\s]+\)/g, '$1')
    .trim()
}
