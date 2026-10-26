import * as React from 'react'
import { ContextMenu as ContextMenuPrimitive } from 'radix-ui'
import { ChevronRightIcon } from 'lucide-react'
import { cn } from '@/lib/utils'

const ContextMenu = ContextMenuPrimitive.Root
const ContextMenuTrigger = ContextMenuPrimitive.Trigger
const ContextMenuSub = ContextMenuPrimitive.Sub

const contentCls =
  'z-50 max-h-(--radix-context-menu-content-available-height) min-w-[12rem] overflow-y-auto rounded-lg border bg-popover p-1 text-popover-foreground shadow-xl data-[state=open]:animate-in data-[state=open]:fade-in-0 data-[state=open]:zoom-in-95'

// Keep menus clear of the window edges so the last items are never cut off.
const COLLISION_PADDING = 8

function ContextMenuContent({ className, ...props }: React.ComponentProps<typeof ContextMenuPrimitive.Content>) {
  return (
    <ContextMenuPrimitive.Portal>
      <ContextMenuPrimitive.Content collisionPadding={COLLISION_PADDING} className={cn(contentCls, className)} {...props} />
    </ContextMenuPrimitive.Portal>
  )
}

const itemCls =
  "relative flex cursor-default items-center gap-2 rounded-md px-2 py-[5px] text-[13px] outline-none select-none focus:bg-accent focus:text-accent-foreground data-[disabled]:pointer-events-none data-[disabled]:opacity-50 [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4 [&_svg]:text-muted-foreground"

function ContextMenuItem({
  className,
  variant,
  ...props
}: React.ComponentProps<typeof ContextMenuPrimitive.Item> & { variant?: 'destructive' }) {
  return (
    <ContextMenuPrimitive.Item
      className={cn(itemCls, variant === 'destructive' && 'text-destructive focus:bg-destructive/10 focus:text-destructive [&_svg]:text-destructive', className)}
      {...props}
    />
  )
}

function ContextMenuSubTrigger({ className, children, ...props }: React.ComponentProps<typeof ContextMenuPrimitive.SubTrigger>) {
  return (
    <ContextMenuPrimitive.SubTrigger className={cn(itemCls, 'data-[state=open]:bg-accent', className)} {...props}>
      {children}
      <ChevronRightIcon className="ml-auto size-3.5" />
    </ContextMenuPrimitive.SubTrigger>
  )
}

function ContextMenuSubContent({ className, ...props }: React.ComponentProps<typeof ContextMenuPrimitive.SubContent>) {
  return (
    <ContextMenuPrimitive.Portal>
      <ContextMenuPrimitive.SubContent collisionPadding={COLLISION_PADDING} className={cn(contentCls, className)} {...props} />
    </ContextMenuPrimitive.Portal>
  )
}

function ContextMenuLabel({ className, ...props }: React.ComponentProps<typeof ContextMenuPrimitive.Label>) {
  return <ContextMenuPrimitive.Label className={cn('px-2 py-1.5 text-[11px] font-semibold uppercase tracking-wide text-muted-foreground', className)} {...props} />
}

function ContextMenuSeparator({ className, ...props }: React.ComponentProps<typeof ContextMenuPrimitive.Separator>) {
  return <ContextMenuPrimitive.Separator className={cn('-mx-1 my-0.5 h-px bg-border', className)} {...props} />
}

function ContextMenuShortcut({ className, ...props }: React.ComponentProps<'span'>) {
  return <span className={cn('ml-auto pl-4 text-xs tracking-wide text-muted-foreground', className)} {...props} />
}

export {
  ContextMenu,
  ContextMenuTrigger,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuSub,
  ContextMenuSubTrigger,
  ContextMenuSubContent,
  ContextMenuLabel,
  ContextMenuSeparator,
  ContextMenuShortcut,
}
