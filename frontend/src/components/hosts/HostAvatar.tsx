import { cn, initials } from '@/lib/utils'

export function HostAvatar({ label, color, className }: { label: string; color: string; className?: string }) {
  return (
    <div
      className={cn('flex size-9 shrink-0 items-center justify-center rounded-lg text-[12px] font-semibold text-white', !color && 'bg-muted text-foreground', className)}
      style={color ? { background: color } : undefined}
    >
      {initials(label)}
    </div>
  )
}
