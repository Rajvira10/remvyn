import { ShieldCheckIcon, Trash2Icon } from 'lucide-react'
import { useState } from 'react'
import { ConfirmDialog } from '@/components/common/ConfirmDialog'
import { EmptyState } from '@/components/common/EmptyState'
import { Button } from '@/components/ui/button'
import { api } from '@/lib/api'
import type { KnownHost } from '@/lib/types'
import { formatDate } from '@/lib/utils'
import { useVault } from '@/stores/vault'

export function KnownHostsPage() {
  const { knownHosts, loadKnownHosts } = useVault()
  const [forgetting, setForgetting] = useState<KnownHost | null>(null)
  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className="flex h-14 shrink-0 items-center gap-3 border-b px-5">
        <h1 className="text-[15px] font-semibold">Known hosts</h1>
        <span className="text-xs text-muted-foreground">Server keys you have trusted. Remvyn warns you if a server's key changes.</span>
      </div>
      <div className="min-h-0 flex-1 overflow-auto p-5">
        {knownHosts.length === 0 ? (
          <EmptyState icon={ShieldCheckIcon} title="No trusted hosts yet">
            The first time you connect to a server, Remvyn shows its key fingerprint and asks you to confirm it.
          </EmptyState>
        ) : (
          <table className="w-full text-[13px]">
            <thead className="text-[11px] tracking-wide text-muted-foreground uppercase">
              <tr className="border-b">
                <th className="py-2 text-left font-medium">Host</th>
                <th className="text-left font-medium">Key type</th>
                <th className="text-left font-medium">Fingerprint</th>
                <th className="text-left font-medium">Trusted</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {knownHosts.map((k) => (
                <tr key={k.host + k.keyType} className="border-b border-border/60">
                  <td className="py-2 font-mono text-[12px]">{k.host}</td>
                  <td className="text-xs text-muted-foreground">{k.keyType}</td>
                  <td className="font-mono text-[11.5px] break-all text-muted-foreground" data-selectable>
                    {k.fingerprint}
                  </td>
                  <td className="text-xs whitespace-nowrap text-muted-foreground">{formatDate(k.addedAt)}</td>
                  <td className="text-right">
                    <Button variant="ghost" size="xs" onClick={() => setForgetting(k)}>
                      <Trash2Icon /> Forget
                    </Button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
      <ConfirmDialog
        open={!!forgetting}
        onOpenChange={(o) => !o && setForgetting(null)}
        title={`Forget ${forgetting?.host}?`}
        description="You will be asked to verify the server's key again on the next connection."
        confirmLabel="Forget"
        onConfirm={async () => {
          if (!forgetting) return
          await api.forgetKnownHost(forgetting.host)
          await loadKnownHosts()
        }}
      />
    </div>
  )
}
