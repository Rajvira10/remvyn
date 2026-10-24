import { ArrowDownIcon, ArrowUpIcon, CheckCircle2Icon, ChevronDownIcon, ChevronUpIcon, XCircleIcon, XIcon } from 'lucide-react'
import { useState } from 'react'
import { Button } from '@/components/ui/button'
import { api } from '@/lib/api'
import { cn, formatBytes } from '@/lib/utils'
import { useTransfers } from '@/stores/transfers'

export function TransfersPanel({ sessionId }: { sessionId: string }) {
  const byId = useTransfers((s) => s.byId)
  const clearFinished = useTransfers((s) => s.clearFinished)
  const [open, setOpen] = useState(true)
  const list = Object.values(byId)
    .filter((t) => t.sessionId === sessionId)
    .sort((a, b) => b.startedAt - a.startedAt)
  if (list.length === 0) return null
  const running = list.filter((t) => t.state === 'running').length

  return (
    <div className="shrink-0 border-t bg-panel">
      <div className="flex h-8 items-center gap-2 px-3 text-xs">
        <button className="flex items-center gap-1 font-medium" onClick={() => setOpen((o) => !o)}>
          {open ? <ChevronDownIcon className="size-3.5" /> : <ChevronUpIcon className="size-3.5" />}
          Transfers
        </button>
        <span className="text-muted-foreground">{running > 0 ? `${running} in progress` : 'all finished'}</span>
        <Button variant="ghost" size="xs" className="ml-auto" onClick={() => void clearFinished()} disabled={running === list.length}>
          Clear finished
        </Button>
      </div>
      {open && (
        <div className="max-h-44 overflow-auto px-3 pb-2">
          {list.map((t) => {
            const pct = t.bytesTotal > 0 ? Math.min(100, (t.bytesDone / t.bytesTotal) * 100) : t.state === 'done' ? 100 : 0
            return (
              <div key={t.id} className="flex items-center gap-3 py-1 text-xs">
                {t.direction === 'upload' ? <ArrowUpIcon className="size-3.5 shrink-0 text-info" /> : <ArrowDownIcon className="size-3.5 shrink-0 text-success" />}
                <span className="w-48 shrink-0 truncate" title={`${t.source} → ${t.dest}`}>
                  {t.name}
                </span>
                <div className="h-1.5 min-w-20 flex-1 overflow-hidden rounded-full bg-muted">
                  <div
                    className={cn('h-full rounded-full transition-[width]', t.state === 'failed' ? 'bg-danger' : t.state === 'cancelled' ? 'bg-muted-foreground' : 'bg-primary')}
                    style={{ width: `${pct}%` }}
                  />
                </div>
                <span className="w-40 shrink-0 text-right text-muted-foreground tabular-nums">
                  {formatBytes(t.bytesDone)} / {formatBytes(t.bytesTotal)}
                  {t.filesTotal > 1 && ` · ${t.filesDone}/${t.filesTotal}`}
                </span>
                <span className="flex w-20 shrink-0 justify-end">
                  {t.state === 'running' && (
                    <Button variant="ghost" size="icon-xs" onClick={() => void api.cancelTransfer(t.id)} aria-label="Cancel transfer">
                      <XIcon />
                    </Button>
                  )}
                  {t.state === 'done' && <CheckCircle2Icon className="size-4 text-success" />}
                  {t.state === 'failed' && (
                    <span title={t.error} className="flex items-center gap-1 text-danger">
                      <XCircleIcon className="size-4" /> failed
                    </span>
                  )}
                  {t.state === 'cancelled' && <span className="text-muted-foreground">cancelled</span>}
                </span>
              </div>
            )
          })}
        </div>
      )}
    </div>
  )
}
