import { CheckIcon, CopyIcon } from 'lucide-react'
import { useState } from 'react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Tooltip } from '@/components/ui/tooltip'
import { copyText } from '@/lib/api'

export function CopyButton({
  text,
  label,
  tooltip,
  size = 'xs',
}: {
  text: string | (() => string | Promise<string>)
  label?: string
  tooltip?: string
  size?: 'xs' | 'sm' | 'icon-xs' | 'icon-sm'
}) {
  const [done, setDone] = useState(false)
  const onClick = async () => {
    const value = typeof text === 'function' ? await text() : text
    if (await copyText(value)) {
      setDone(true)
      setTimeout(() => setDone(false), 1200)
    } else {
      toast.error('Could not access the clipboard')
    }
  }
  const icon = done ? <CheckIcon className="text-success" /> : <CopyIcon />
  const btn = (
    <Button variant="ghost" size={label ? size : size === 'sm' ? 'icon-sm' : 'icon-xs'} onClick={onClick} aria-label={tooltip ?? label ?? 'Copy'}>
      {icon}
      {label}
    </Button>
  )
  return tooltip ? <Tooltip content={tooltip}>{btn}</Tooltip> : btn
}
