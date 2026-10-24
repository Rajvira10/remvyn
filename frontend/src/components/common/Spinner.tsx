import { Loader2Icon } from 'lucide-react'
import { cn } from '@/lib/utils'

export function Spinner({ className }: { className?: string }) {
  return <Loader2Icon className={cn('size-4 animate-spin text-muted-foreground', className)} />
}

export function LoadingBlock({ label = 'Loading…' }: { label?: string }) {
  return (
    <div className="flex h-full items-center justify-center gap-2 text-[13px] text-muted-foreground">
      <Spinner /> {label}
    </div>
  )
}
