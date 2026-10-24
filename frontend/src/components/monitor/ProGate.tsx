import { CheckIcon, SparklesIcon } from 'lucide-react'
import type { ReactNode } from 'react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { errorMessage } from '@/lib/api'
import type { Settings } from '@/lib/types'
import { cn } from '@/lib/utils'
import { useSettings } from '@/stores/settings'

export const PRO_FEATURES = [
  'Historical metrics, kept for up to a year',
  'Custom alerts with desktop notifications',
  'Uptime monitoring for websites and ports',
  'Multi-server dashboard with background monitoring',
]

export function useIsPro(): boolean {
  return useSettings((s) => s.settings?.proEnabled ?? false)
}

export async function setPro(on: boolean) {
  const st = useSettings.getState().settings
  if (!st) return
  try {
    await useSettings.getState().save({ ...st, proEnabled: on } as Settings)
    if (on) toast.success('Flux Pro is on', { description: 'History, alerts, uptime checks and background monitoring are unlocked.' })
  } catch (e) {
    toast.error('Could not change Flux Pro', { description: errorMessage(e) })
  }
}

export function ProBadge({ className }: { className?: string }) {
  return (
    <span className={cn('inline-flex items-center gap-0.5 rounded bg-gradient-to-r from-violet-500/20 to-fuchsia-500/20 px-1.5 py-px text-[10px] font-semibold text-violet-300', className)}>
      <SparklesIcon className="size-2.5" /> PRO
    </span>
  )
}

/** Shows children for Pro users, and an upgrade card otherwise. */
export function ProGate({ feature, description, children, compact }: { feature: string; description: string; children: ReactNode; compact?: boolean }) {
  const pro = useIsPro()
  if (pro) return <>{children}</>
  if (compact) {
    return (
      <div className="flex items-center gap-3 rounded-lg border border-dashed border-violet-500/30 bg-violet-500/5 px-3 py-2.5 text-xs">
        <SparklesIcon className="size-4 shrink-0 text-violet-400" />
        <div className="flex-1">
          <span className="font-medium text-foreground">{feature}</span> <span className="text-muted-foreground">{description}</span>
        </div>
        <Button size="xs" variant="outline" onClick={() => void setPro(true)}>
          Upgrade
        </Button>
      </div>
    )
  }
  return (
    <div className="mx-auto my-10 flex max-w-md flex-col items-center gap-4 rounded-2xl border border-violet-500/25 bg-gradient-to-b from-violet-500/10 to-transparent p-8 text-center">
      <div className="flex size-11 items-center justify-center rounded-xl bg-violet-500/15">
        <SparklesIcon className="size-5 text-violet-400" />
      </div>
      <div>
        <div className="flex items-center justify-center gap-2 text-[16px] font-semibold">
          {feature} <ProBadge />
        </div>
        <p className="mt-1.5 text-[13px] leading-relaxed text-muted-foreground">{description}</p>
      </div>
      <ul className="flex flex-col gap-1.5 text-left text-[13px]">
        {PRO_FEATURES.map((f) => (
          <li key={f} className="flex items-center gap-2">
            <CheckIcon className="size-3.5 text-violet-400" /> {f}
          </li>
        ))}
      </ul>
      <Button onClick={() => void setPro(true)} className="bg-violet-600 text-white hover:bg-violet-600/90">
        <SparklesIcon /> Upgrade to Flux Pro
      </Button>
      <p className="text-[11px] text-muted-foreground">Payments aren’t connected yet. This unlocks Pro on this computer so you can try it.</p>
    </div>
  )
}
