import { EyeIcon, EyeOffIcon } from 'lucide-react'
import { useState } from 'react'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { cn } from '@/lib/utils'

/** Rough strength score, 0–4. */
export function passwordScore(pw: string): number {
  if (!pw) return 0
  let score = 0
  if (pw.length >= 8) score++
  if (pw.length >= 12) score++
  if (pw.length >= 16) score++
  const classes = [/[a-z]/, /[A-Z]/, /\d/, /[^A-Za-z0-9]/].filter((r) => r.test(pw)).length
  if (classes >= 3 || (pw.includes(' ') && pw.length >= 14)) score++
  if (/^(.)\1+$/.test(pw) || /^(password|12345678|qwerty)/i.test(pw)) score = 0
  return Math.min(score, 4)
}

const LABELS = ['Too weak', 'Weak', 'Okay', 'Good', 'Strong']
const COLORS = ['var(--danger)', 'var(--danger)', 'var(--warning)', 'var(--success)', 'var(--success)']

export function StrengthMeter({ password }: { password: string }) {
  const score = passwordScore(password)
  if (!password) return null
  return (
    <div className="flex items-center gap-2">
      <div className="flex flex-1 gap-1">
        {[0, 1, 2, 3].map((i) => (
          <div key={i} className="h-1 flex-1 rounded-full bg-muted" style={i < score ? { background: COLORS[score] } : undefined} />
        ))}
      </div>
      <span className="w-14 text-right text-[11px] text-muted-foreground">{LABELS[score]}</span>
    </div>
  )
}

export function PasswordInput({
  id,
  label,
  value,
  onChange,
  autoFocus,
  autoComplete = 'new-password',
  invalid,
}: {
  id: string
  label: string
  value: string
  onChange: (v: string) => void
  autoFocus?: boolean
  autoComplete?: string
  invalid?: boolean
}) {
  const [show, setShow] = useState(false)
  return (
    <div className="flex flex-col gap-1.5">
      <Label htmlFor={id}>{label}</Label>
      <div className="relative">
        <Input
          id={id}
          type={show ? 'text' : 'password'}
          value={value}
          onChange={(e) => onChange(e.target.value)}
          autoFocus={autoFocus}
          autoComplete={autoComplete}
          spellCheck={false}
          aria-invalid={invalid}
          className={cn('pr-9', invalid && 'border-danger')}
        />
        <button
          type="button"
          tabIndex={-1}
          onClick={() => setShow((v) => !v)}
          aria-label={show ? 'Hide password' : 'Show password'}
          className="absolute top-1/2 right-2.5 -translate-y-1/2 text-muted-foreground hover:text-foreground"
        >
          {show ? <EyeOffIcon className="size-3.5" /> : <EyeIcon className="size-3.5" />}
        </button>
      </div>
    </div>
  )
}

/** New password + confirmation with a strength meter. Returns an error or "". */
export function newPasswordError(pw: string, confirm: string, min: number): string {
  if (pw.length < min) return `Use at least ${min} characters.`
  if (pw !== confirm) return 'The passwords don’t match.'
  return ''
}
