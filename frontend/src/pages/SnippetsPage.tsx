import { CodeIcon, MoreHorizontalIcon, PencilIcon, PlayIcon, PlusIcon, SearchIcon, Trash2Icon } from 'lucide-react'
import { useEffect, useState } from 'react'
import { toast } from 'sonner'
import { ConfirmDialog } from '@/components/common/ConfirmDialog'
import { CopyButton } from '@/components/common/CopyButton'
import { EmptyState } from '@/components/common/EmptyState'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { useShallow } from 'zustand/react/shallow'
import { api, errorMessage } from '@/lib/api'
import type { Snippet } from '@/lib/types'
import { isTerminal, useSessions } from '@/stores/sessions'
import { useVault } from '@/stores/vault'

const blank = (): Snippet => ({ id: '', name: '', command: '', description: '', createdAt: 0, updatedAt: 0 })

export function SnippetsPage() {
  const { snippets, loadSnippets } = useVault()
  const tabs = useSessions(useShallow((s) => s.tabs.filter((t) => isTerminal(t) && t.state === 'connected')))
  const [editing, setEditing] = useState<Snippet | null>(null)
  const [deleting, setDeleting] = useState<Snippet | null>(null)
  const [query, setQuery] = useState('')
  const q = query.toLowerCase()
  const list = snippets.filter((s) => !q || `${s.name} ${s.command} ${s.description}`.toLowerCase().includes(q))

  const run = (sn: Snippet, tabKey: string) => {
    const t = useSessions.getState().tabs.find((x) => x.key === tabKey)
    if (!t) return
    const cmd = sn.command.endsWith('\n') ? sn.command : sn.command + '\r'
    api
      .writeTerminal(t.sessionId, cmd)
      .then(() => useSessions.getState().show(t.key))
      .catch((e) => toast.error(errorMessage(e)))
  }

  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className="flex h-14 shrink-0 items-center gap-3 border-b px-5">
        <h1 className="text-[15px] font-semibold">Snippets</h1>
        <div className="relative ml-2 w-full max-w-xs">
          <SearchIcon className="pointer-events-none absolute top-1/2 left-2.5 size-3.5 -translate-y-1/2 text-muted-foreground" />
          <Input value={query} onChange={(e) => setQuery(e.target.value)} placeholder="Search snippets" className="h-8 pl-8" />
        </div>
        <Button size="sm" className="ml-auto" onClick={() => setEditing(blank())}>
          <PlusIcon /> New snippet
        </Button>
      </div>
      <div className="min-h-0 flex-1 overflow-auto p-5">
        {snippets.length === 0 ? (
          <EmptyState icon={CodeIcon} title="No snippets yet">
            Save commands you run often and send them to any open terminal from the Snippets menu.
          </EmptyState>
        ) : (
          <div className="grid grid-cols-[repeat(auto-fill,minmax(320px,1fr))] gap-3">
            {list.map((s) => (
              <div key={s.id} className="flex flex-col gap-2 rounded-xl border bg-card p-4">
                <div className="flex items-center gap-2">
                  <CodeIcon className="size-4 text-primary" />
                  <span className="flex-1 truncate font-medium">{s.name}</span>
                  <DropdownMenu>
                    <DropdownMenuTrigger asChild>
                      <Button variant="secondary" size="xs" disabled={tabs.length === 0} title={tabs.length === 0 ? 'Open a terminal first' : undefined}>
                        <PlayIcon /> Run
                      </Button>
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align="end">
                      <DropdownMenuLabel>Run in</DropdownMenuLabel>
                      {tabs.map((t) => (
                        <DropdownMenuItem key={t.key} onSelect={() => run(s, t.key)}>
                          {t.title}
                          <span className="ml-auto font-mono text-[11px] text-muted-foreground">{t.remote}</span>
                        </DropdownMenuItem>
                      ))}
                    </DropdownMenuContent>
                  </DropdownMenu>
                  <DropdownMenu>
                    <DropdownMenuTrigger asChild>
                      <Button variant="ghost" size="icon-xs" aria-label="Snippet actions">
                        <MoreHorizontalIcon />
                      </Button>
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align="end">
                      <DropdownMenuItem onSelect={() => setEditing(s)}>
                        <PencilIcon /> Edit
                      </DropdownMenuItem>
                      <DropdownMenuSeparator />
                      <DropdownMenuItem variant="destructive" onSelect={() => setDeleting(s)}>
                        <Trash2Icon /> Delete
                      </DropdownMenuItem>
                    </DropdownMenuContent>
                  </DropdownMenu>
                </div>
                {s.description && <div className="text-xs text-muted-foreground">{s.description}</div>}
                <div className="flex items-start gap-1 rounded-md bg-muted/50 py-1.5 pr-1 pl-2">
                  <pre className="max-h-28 flex-1 overflow-auto font-mono text-[11.5px] whitespace-pre-wrap" data-selectable>
                    {s.command}
                  </pre>
                  <CopyButton text={s.command} tooltip="Copy command" />
                </div>
              </div>
            ))}
          </div>
        )}
      </div>
      <SnippetDialog snippet={editing} onOpenChange={(o) => !o && setEditing(null)} onSaved={loadSnippets} />
      <ConfirmDialog
        open={!!deleting}
        onOpenChange={(o) => !o && setDeleting(null)}
        title={`Delete “${deleting?.name}”?`}
        description="This snippet will be removed."
        confirmLabel="Delete"
        destructive
        onConfirm={async () => {
          if (!deleting) return
          await api.deleteSnippet(deleting.id)
          await loadSnippets()
        }}
      />
    </div>
  )
}

function SnippetDialog({ snippet, onOpenChange, onSaved }: { snippet: Snippet | null; onOpenChange: (o: boolean) => void; onSaved: () => Promise<void> }) {
  const [s, setS] = useState<Snippet>(blank())
  useEffect(() => {
    if (snippet) setS(snippet)
  }, [snippet])
  const save = async () => {
    try {
      await api.saveSnippet(s)
      await onSaved()
      onOpenChange(false)
    } catch (e) {
      toast.error('Could not save snippet', { description: errorMessage(e) })
    }
  }
  return (
    <Dialog open={!!snippet} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{snippet?.id ? 'Edit snippet' : 'New snippet'}</DialogTitle>
        </DialogHeader>
        <div className="flex flex-col gap-3">
          <div className="flex flex-col gap-1.5">
            <Label>Name</Label>
            <Input autoFocus value={s.name} onChange={(e) => setS({ ...s, name: e.target.value })} placeholder="Disk usage" />
          </div>
          <div className="flex flex-col gap-1.5">
            <Label>Command</Label>
            <Textarea rows={5} className="font-mono text-[12px]" value={s.command} onChange={(e) => setS({ ...s, command: e.target.value })} placeholder="df -h && du -sh /var/log/*" />
            <p className="text-[11.5px] text-muted-foreground">Sent to the terminal followed by Enter. Multi-line snippets are sent line by line.</p>
          </div>
          <div className="flex flex-col gap-1.5">
            <Label>Description (optional)</Label>
            <Input value={s.description} onChange={(e) => setS({ ...s, description: e.target.value })} />
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button disabled={!s.name.trim() || !s.command.trim()} onClick={save}>
            Save
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
