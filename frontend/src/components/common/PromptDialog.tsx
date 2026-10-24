import { useEffect, useState } from 'react'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'

export function PromptDialog({
  open,
  onOpenChange,
  title,
  description,
  initial = '',
  placeholder,
  confirmLabel = 'Save',
  onSubmit,
}: {
  open: boolean
  onOpenChange: (o: boolean) => void
  title: string
  description?: string
  initial?: string
  placeholder?: string
  confirmLabel?: string
  onSubmit: (value: string) => Promise<void> | void
}) {
  const [value, setValue] = useState(initial)
  const [busy, setBusy] = useState(false)
  useEffect(() => {
    if (open) setValue(initial)
  }, [open, initial])
  const submit = async () => {
    if (!value.trim()) return
    setBusy(true)
    try {
      await onSubmit(value.trim())
      onOpenChange(false)
    } finally {
      setBusy(false)
    }
  }
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          {description && <DialogDescription>{description}</DialogDescription>}
        </DialogHeader>
        <Input
          autoFocus
          value={value}
          placeholder={placeholder}
          onChange={(e) => setValue(e.target.value)}
          onKeyDown={(e) => e.key === 'Enter' && submit()}
        />
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button disabled={busy || !value.trim()} onClick={submit}>
            {confirmLabel}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
