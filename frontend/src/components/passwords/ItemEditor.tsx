import { EyeIcon, EyeOffIcon, PlusIcon, RefreshCwIcon, Trash2Icon, WandSparklesIcon } from 'lucide-react'
import { useEffect, useState } from 'react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { Tooltip } from '@/components/ui/tooltip'
import { api, errorMessage } from '@/lib/api'
import { defaultGen, generatePassword, passwordStrength, type GenOptions } from '@/lib/passwordGen'
import type { VaultField, VaultItem, VaultItemKind, VaultSecret } from '@/lib/types'
import { cn } from '@/lib/utils'

export const KIND_LABELS: Record<VaultItemKind, string> = { login: 'Login', token: 'API token', note: 'Secure note' }

export const blankItem = (kind: VaultItemKind = 'login'): VaultItem => ({
  id: '',
  kind,
  name: '',
  username: '',
  url: '',
  favorite: false,
  hasPassword: false,
  hasTotp: false,
  hasNotes: false,
  fieldCount: 0,
  createdAt: 0,
  updatedAt: 0,
})

const blankSecret = (): VaultSecret => ({ password: '', totp: '', notes: '', fields: [] })

/** Creates or edits a vault item. `item` with an empty id creates one. */
export function ItemEditor({ item, onOpenChange, onSaved }: { item: VaultItem | null; onOpenChange: (o: boolean) => void; onSaved: (it: VaultItem) => void }) {
  const [it, setIt] = useState<VaultItem>(blankItem())
  const [sec, setSec] = useState<VaultSecret>(blankSecret())
  const [loading, setLoading] = useState(false)
  const [busy, setBusy] = useState(false)
  const [show, setShow] = useState(false)
  const [gen, setGen] = useState<GenOptions>(defaultGen)
  const [genOpen, setGenOpen] = useState(false)

  useEffect(() => {
    if (!item) return
    setIt(item)
    setShow(false)
    setGenOpen(false)
    if (!item.id) {
      const s = blankSecret()
      if (item.kind === 'login') s.password = generatePassword(defaultGen)
      setSec(s)
      return
    }
    // The secret is decrypted only while the editor is open.
    setLoading(true)
    api
      .revealVaultItem(item.id)
      .then(setSec)
      .catch((e) => {
        toast.error('Could not decrypt this item', { description: errorMessage(e) })
        onOpenChange(false)
      })
      .finally(() => setLoading(false))
  }, [item, onOpenChange])

  // Forget the decrypted secret when the dialog closes.
  useEffect(() => {
    if (!item) setSec(blankSecret())
  }, [item])

  const kind = it.kind
  const secretLabel = kind === 'token' ? 'Token' : 'Password'
  const strength = kind === 'login' ? passwordStrength(sec.password) : null
  const setField = (i: number, f: Partial<VaultField>) => setSec((s) => ({ ...s, fields: s.fields.map((x, j) => (j === i ? { ...x, ...f } : x)) }))
  const regenerate = (o: GenOptions = gen) => {
    setSec((s) => ({ ...s, password: generatePassword(o) }))
    setShow(true)
  }

  const save = async () => {
    setBusy(true)
    try {
      const saved = await api.saveVaultItem({ item: it, secret: sec })
      onSaved(saved)
      onOpenChange(false)
    } catch (e) {
      toast.error('Could not save', { description: errorMessage(e) })
    } finally {
      setBusy(false)
    }
  }

  return (
    <Dialog open={!!item} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{item?.id ? `Edit ${KIND_LABELS[kind].toLowerCase()}` : `New ${KIND_LABELS[kind].toLowerCase()}`}</DialogTitle>
        </DialogHeader>
        {!item?.id && (
          <div role="radiogroup" aria-label="Item type" className="flex self-start rounded-md border p-0.5">
            {(Object.keys(KIND_LABELS) as VaultItemKind[]).map((k) => (
              <button
                key={k}
                role="radio"
                aria-checked={kind === k}
                onClick={() => {
                  setIt({ ...it, kind: k })
                  if (k === 'login' && !sec.password) setSec({ ...sec, password: generatePassword(gen) })
                  if (k !== 'login' && kind === 'login' && !show) setSec({ ...sec, password: '' })
                }}
                className={cn('rounded px-2.5 py-1 text-xs transition-colors', kind === k ? 'bg-accent font-medium text-foreground' : 'text-muted-foreground hover:text-foreground')}
              >
                {KIND_LABELS[k]}
              </button>
            ))}
          </div>
        )}
        <div className={cn('flex flex-col gap-3', loading && 'pointer-events-none opacity-50')}>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="vi-name">Name</Label>
            <Input
              id="vi-name"
              autoFocus
              value={it.name}
              onChange={(e) => setIt({ ...it, name: e.target.value })}
              placeholder={kind === 'login' ? 'Gmail' : kind === 'token' ? 'GitHub personal token' : 'Recovery codes'}
            />
          </div>
          {kind !== 'note' && (
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="vi-user">{kind === 'login' ? 'Email or username' : 'Account (optional)'}</Label>
              <Input id="vi-user" value={it.username} autoComplete="off" spellCheck={false} onChange={(e) => setIt({ ...it, username: e.target.value })} />
            </div>
          )}
          {kind !== 'note' && (
            <div className="flex flex-col gap-1.5">
              <div className="flex items-center justify-between">
                <Label htmlFor="vi-pw">{secretLabel}</Label>
                {strength?.label && (
                  <span className={cn('text-[11px]', ['text-danger', 'text-warning', 'text-success', 'text-success'][strength.level])}>
                    {strength.label} · {strength.bits} bits
                  </span>
                )}
              </div>
              <div className="flex gap-1.5">
                <div className="relative flex-1">
                  <Input
                    id="vi-pw"
                    type={show ? 'text' : 'password'}
                    value={sec.password}
                    autoComplete="new-password"
                    spellCheck={false}
                    className="pr-9 font-mono text-[12.5px]"
                    onChange={(e) => setSec({ ...sec, password: e.target.value })}
                  />
                  <button
                    type="button"
                    tabIndex={-1}
                    onClick={() => setShow((v) => !v)}
                    aria-label={show ? `Hide ${secretLabel.toLowerCase()}` : `Show ${secretLabel.toLowerCase()}`}
                    className="absolute top-1/2 right-2.5 -translate-y-1/2 text-muted-foreground hover:text-foreground"
                  >
                    {show ? <EyeOffIcon className="size-3.5" /> : <EyeIcon className="size-3.5" />}
                  </button>
                </div>
                {kind === 'login' && (
                  <Tooltip content="Password generator">
                    <Button variant={genOpen ? 'secondary' : 'outline'} size="icon" aria-label="Password generator" onClick={() => setGenOpen((v) => !v)}>
                      <WandSparklesIcon />
                    </Button>
                  </Tooltip>
                )}
              </div>
              {kind === 'login' && genOpen && (
                <div className="flex flex-wrap items-center gap-x-4 gap-y-2 rounded-lg border bg-muted/30 px-3 py-2 text-xs">
                  <label className="flex items-center gap-2">
                    Length
                    <input
                      type="range"
                      min={8}
                      max={64}
                      value={gen.length}
                      onChange={(e) => {
                        const o = { ...gen, length: Number(e.target.value) }
                        setGen(o)
                        regenerate(o)
                      }}
                      className="w-28 accent-[var(--primary)]"
                    />
                    <span className="w-5 tabular-nums">{gen.length}</span>
                  </label>
                  <label className="flex items-center gap-1.5">
                    <Checkbox
                      checked={gen.digits}
                      onCheckedChange={(v) => {
                        const o = { ...gen, digits: !!v }
                        setGen(o)
                        regenerate(o)
                      }}
                    />
                    Numbers
                  </label>
                  <label className="flex items-center gap-1.5">
                    <Checkbox
                      checked={gen.symbols}
                      onCheckedChange={(v) => {
                        const o = { ...gen, symbols: !!v }
                        setGen(o)
                        regenerate(o)
                      }}
                    />
                    Symbols
                  </label>
                  <Button size="xs" variant="ghost" className="ml-auto" onClick={() => regenerate()}>
                    <RefreshCwIcon /> New
                  </Button>
                </div>
              )}
            </div>
          )}
          {kind !== 'note' && (
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="vi-url">Website (optional)</Label>
              <Input id="vi-url" value={it.url} spellCheck={false} placeholder="https://" onChange={(e) => setIt({ ...it, url: e.target.value })} />
            </div>
          )}
          {kind === 'login' && (
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="vi-totp">Two-factor secret (optional)</Label>
              <Input
                id="vi-totp"
                value={sec.totp}
                autoComplete="off"
                spellCheck={false}
                className="font-mono text-[12px]"
                placeholder="Setup key or otpauth:// link"
                onChange={(e) => setSec({ ...sec, totp: e.target.value })}
              />
              <p className="text-[11px] text-muted-foreground">
                When a site shows a QR code for an authenticator app, choose “can’t scan?” to get the key. Remvyn then shows the 6-digit codes.
              </p>
            </div>
          )}

          <div className="flex flex-col gap-1.5">
            <div className="flex items-center justify-between">
              <Label>Extra fields</Label>
              <Button size="xs" variant="ghost" onClick={() => setSec({ ...sec, fields: [...sec.fields, { name: '', value: '', hidden: true }] })}>
                <PlusIcon /> Add field
              </Button>
            </div>
            {sec.fields.map((f, i) => (
              <div key={i} className="flex items-center gap-1.5">
                <Input value={f.name} placeholder="Name" className="w-36" onChange={(e) => setField(i, { name: e.target.value })} />
                <Input
                  value={f.value}
                  placeholder="Value"
                  type={f.hidden && !show ? 'password' : 'text'}
                  autoComplete="off"
                  spellCheck={false}
                  className="flex-1 font-mono text-[12px]"
                  onChange={(e) => setField(i, { value: e.target.value })}
                />
                <Tooltip content={f.hidden ? 'Hidden in the item view' : 'Shown in the item view'}>
                  <Button variant="ghost" size="icon-sm" aria-label={f.hidden ? 'Show this field' : 'Hide this field'} onClick={() => setField(i, { hidden: !f.hidden })}>
                    {f.hidden ? <EyeOffIcon /> : <EyeIcon />}
                  </Button>
                </Tooltip>
                <Button variant="ghost" size="icon-sm" aria-label="Remove field" onClick={() => setSec({ ...sec, fields: sec.fields.filter((_, j) => j !== i) })}>
                  <Trash2Icon />
                </Button>
              </div>
            ))}
          </div>

          <div className="flex flex-col gap-1.5">
            <Label htmlFor="vi-notes">{kind === 'note' ? 'Note' : 'Notes (optional)'}</Label>
            <Textarea
              id="vi-notes"
              rows={kind === 'note' ? 8 : 3}
              value={sec.notes}
              spellCheck={false}
              className="font-mono text-[12px]"
              onChange={(e) => setSec({ ...sec, notes: e.target.value })}
            />
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button disabled={busy || loading || !(it.name.trim() || it.url.trim())} onClick={() => void save()}>
            Save
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
