import type { LucideIcon } from 'lucide-react'
import type { ReactNode } from 'react'
import { cn } from '@/lib/utils'

export function EmptyState({
  icon: Icon,
  title,
  children,
  className,
}: {
  icon: LucideIcon
  title: string
  children?: ReactNode
  className?: string
}) {
  return (
    <div className={cn('flex h-full flex-col items-center justify-center gap-3 p-8 text-center', className)}>
      <div className="flex size-11 items-center justify-center rounded-xl border bg-muted/50">
        <Icon className="size-5 text-muted-foreground" />
      </div>
      <div className="text-[14px] font-medium">{title}</div>
      {children && <div className="max-w-md text-[13px] leading-relaxed text-muted-foreground">{children}</div>}
    </div>
  )
}
