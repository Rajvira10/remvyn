import {
  CopyIcon,
  ExternalLinkIcon,
  EyeIcon,
  EyeOffIcon,
  KeyRoundIcon,
  LockKeyholeIcon,
  PencilIcon,
  PlusIcon,
  SearchIcon,
  ShieldAlertIcon,
  StarIcon,
  StickyNoteIcon,
  Trash2Icon,
  UserIcon,
} from 'lucide-react'
import { useCallback, useEffect, useMemo, useState } from 'react'
import { toast } from 'sonner'
import { BrowserOpenURL } from '../../wailsjs/runtime/runtime'
import { ConfirmDialog } from '@/components/common/ConfirmDialog'
import { EmptyState } from '@/components/common/EmptyState'
import { ItemEditor, KIND_LABELS, blankItem } from '@/components/passwords/ItemEditor'
import { Button } from '@/components/ui/button'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { Input } from '@/components/ui/input'
import { Tooltip } from '@/components/ui/tooltip'
import { api, errorMessage } from '@/lib/api'
import type { TOTPCode, VaultItem, VaultItemKind, VaultSecret } from '@/lib/types'
import { cn } from '@/lib/utils'
import { useLock } from '@/stores/lock'
import { usePasswords } from '@/stores/passwords'
import { useUI } from '@/stores/ui'

/** How long revealed secrets stay on screen. */
const REVEAL_MS = 30_000

type Filter = 'all' | 'favorite' | VaultItemKind
const FILTERS: { v: Filter; label: string }[] = [
  { v: 'all', label: 'All' },
  { v: 'favorite', label: 'Favorites' },
  { v: 'login', label: 'Logins' },
  { v: 'token', label: 'Tokens' },
  { v: 'note', label: 'Notes' },
]

const KIND_ICONS = { login: KeyRoundIcon, token: LockKeyholeIcon, note: StickyNoteIcon }

export function PasswordsPage() {
  const { items, load } = usePasswords()
  const protectedVault = useLock((s) => s.status?.protected ?? false)
  const [query, setQuery] = useState('')
  const [filter, setFilter] = useState<Filter>('all')
  const [selectedId, setSelectedId] = useState<string | null>(null)
  const [editing, setEditing] = useState<VaultItem | null>(null)
  const [deleting, setDeleting] = useState<VaultItem | null>(null)

  useEffect(() => {
    void load().catch((e) => toast.error('Could not load passwords', { description: errorMessage(e) }))
  }, [load])

  const q = query.trim().toLowerCase()
  const list = useMemo(
    () =>
      items.filter(
        (it) =>
          (filter === 'all' || (filter === 'favorite' ? it.favorite : it.kind === filter)) &&
          (!q || `${it.name} ${it.username} ${it.url}`.toLowerCase().includes(q)),
      ),
    [items, filter, q],
  )
  const selected = items.find((it) => it.id === selectedId) ?? list[0] ?? null
  const closeEditor = useCallback((o: boolean) => !o && setEditing(null), [])

  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className="flex h-14 shrink-0 items-center gap-3 border-b px-5">
        <h1 className="text-[15px] font-semibold">Passwords</h1>
        <div className="relative ml-2 w-full max-w-xs">
          <SearchIcon className="pointer-events-none absolute top-1/2 left-2.5 size-3.5 -translate-y-1/2 text-muted-foreground" />
          <Input value={query} onChange={(e) => setQuery(e.target.value)} placeholder="Search passwords" className="h-8 pl-8" />
        </div>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button size="sm" className="ml-auto">
              <PlusIcon /> New item
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            {(Object.keys(KIND_LABELS) as VaultItemKind[]).map((k) => {
              const Icon = KIND_ICONS[k]
              return (
                <DropdownMenuItem key={k} onSelect={() => setEditing(blankItem(k))}>
                  <Icon /> {KIND_LABELS[k]}
                </DropdownMenuItem>
              )
            })}
          </DropdownMenuContent>
        </DropdownMenu>
      </div>

      {!protectedVault && (
        <div className="flex items-center gap-3 border-b bg-warning/10 px-5 py-2 text-[12.5px]">
          <ShieldAlertIcon className="size-4 shrink-0 text-warning" />
          <span className="flex-1">
            Passwords are encrypted, but without a master password the key is stored on this computer, so anyone who can use your account could open them.
          </span>
          <Button size="xs" variant="outline" onClick={() => useUI.getState().setPage('settings')}>
            Set a master password
          </Button>
        </div>
      )}

      {items.length === 0 ? (
        <EmptyState icon={LockKeyholeIcon} title="No passwords yet">
          Save email and website logins, API tokens and secure notes. Everything, including names, is encrypted with your vault key, synced only in encrypted form,
          and never shown to AI agents.
        </EmptyState>
      ) : (
        <div className="flex min-h-0 flex-1">
          <div className="flex w-[320px] shrink-0 flex-col border-r">
            <div className="flex gap-0.5 overflow-x-auto border-b px-2 py-2">
              {FILTERS.map((f) => (
                <button
                  key={f.v}
                  onClick={() => setFilter(f.v)}
                  className={cn(
                    'shrink-0 rounded-full px-2.5 py-0.5 text-xs transition-colors',
                    filter === f.v ? 'bg-primary/15 font-medium text-primary' : 'text-muted-foreground hover:bg-accent hover:text-foreground',
                  )}
                >
                  {f.label}
                </button>
              ))}
            </div>
            <div className="min-h-0 flex-1 overflow-y-auto p-1.5">
              {list.length === 0 && <div className="p-4 text-center text-xs text-muted-foreground">Nothing matches.</div>}
              {list.map((it) => (
                <button
                  key={it.id}
                  onClick={() => setSelectedId(it.id)}
                  className={cn(
                    'flex w-full items-center gap-2.5 rounded-md px-2 py-1.5 text-left transition-colors',
                    selected?.id === it.id ? 'bg-accent' : 'hover:bg-accent/50',
                  )}
                >
                  <ItemAvatar item={it} />
                  <div className="min-w-0 flex-1">
                    <div className="flex items-center gap-1 truncate text-[13px] font-medium">
                      <span className="truncate">{it.name}</span>
                      {it.favorite && <StarIcon className="size-3 shrink-0 fill-warning text-warning" />}
                    </div>
                    <div className="truncate text-[11.5px] text-muted-foreground">{it.username || KIND_LABELS[it.kind]}</div>
                  </div>
                </button>
              ))}
            </div>
          </div>
          <div className="min-w-0 flex-1 overflow-y-auto">
            {selected && <ItemDetail key={selected.id} item={selected} onEdit={() => setEditing(selected)} onDelete={() => setDeleting(selected)} />}
          </div>
        </div>
      )}

      <ItemEditor
        item={editing}
        onOpenChange={closeEditor}
        onSaved={(it) => {
          setSelectedId(it.id)
          void load()
        }}
      />
      <ConfirmDialog
        open={!!deleting}
        onOpenChange={(o) => !o && setDeleting(null)}
        title={`Delete “${deleting?.name}”?`}
        description="The item is removed from this computer and, if sync is on, from your other devices. This can't be undone."
        confirmLabel="Delete"
        destructive
        onConfirm={async () => {
          if (!deleting) return
          await api.deleteVaultItem(deleting.id)
          setSelectedId(null)
          await load()
        }}
      />
    </div>
  )
}

function ItemAvatar({ item, large }: { item: VaultItem; large?: boolean }) {
  const Icon = KIND_ICONS[item.kind]
  const letter = item.kind === 'login' ? (item.name.trim()[0] ?? '?').toUpperCase() : null
  return (
    <div
      className={cn(
        'flex shrink-0 items-center justify-center rounded-lg bg-primary/15 font-semibold text-primary',
        large ? 'size-11 text-base' : 'size-8 text-[13px]',
      )}
    >
      {letter ?? <Icon className={large ? 'size-5' : 'size-4'} />}
    </div>
  )
}

function ItemDetail({ item, onEdit, onDelete }: { item: VaultItem; onEdit: () => void; onDelete: () => void }) {
  const [secret, setSecret] = useState<VaultSecret | null>(null)
  const [revealed, setRevealed] = useState(false)

  // Decrypted secrets stay on screen for 30 seconds, then are dropped.
  useEffect(() => {
    if (!revealed) return
    const t = setTimeout(() => {
      setRevealed(false)
      setSecret(null)
    }, REVEAL_MS)
    return () => clearTimeout(t)
  }, [revealed])

  const reveal = async () => {
    if (revealed) {
      setRevealed(false)
      setSecret(null)
      return
    }
    try {
      setSecret(await api.revealVaultItem(item.id))
      setRevealed(true)
    } catch (e) {
      toast.error('Could not decrypt this item', { description: errorMessage(e) })
    }
  }

  const copy = async (field: string, what: string, secretValue = true) => {
    try {
      await api.copyVaultItemValue(item.id, field)
      toast.success(`${what} copied`, { description: secretValue ? 'Cleared from the clipboard in 30 seconds.' : undefined, duration: 2000 })
    } catch (e) {
      toast.error('Could not copy', { description: errorMessage(e) })
    }
  }

  const toggleFavorite = async () => {
    try {
      await api.setVaultItemFavorite(item.id, !item.favorite)
      await usePasswords.getState().load()
    } catch (e) {
      toast.error(errorMessage(e))
    }
  }

  const secretLabel = item.kind === 'token' ? 'Token' : 'Password'
  const masked = '••••••••••••'
  const hasSecretParts = item.hasPassword || item.hasNotes || item.fieldCount > 0

  return (
    <div className="mx-auto flex max-w-2xl flex-col gap-5 p-6">
      <div className="flex items-center gap-3">
        <ItemAvatar item={item} large />
        <div className="min-w-0 flex-1">
          <h2 className="truncate text-lg font-semibold" data-selectable>
            {item.name}
          </h2>
          <div className="text-xs text-muted-foreground">{KIND_LABELS[item.kind]}</div>
        </div>
        <Tooltip content={item.favorite ? 'Remove from favorites' : 'Add to favorites'}>
          <Button variant="ghost" size="icon-sm" aria-label={item.favorite ? 'Remove from favorites' : 'Add to favorites'} onClick={() => void toggleFavorite()}>
            <StarIcon className={cn(item.favorite && 'fill-warning text-warning')} />
          </Button>
        </Tooltip>
        {hasSecretParts && (
          <Button variant="outline" size="sm" onClick={() => void reveal()}>
            {revealed ? <EyeOffIcon /> : <EyeIcon />} {revealed ? 'Hide' : 'Reveal'}
          </Button>
        )}
        <Button variant="outline" size="sm" onClick={onEdit}>
          <PencilIcon /> Edit
        </Button>
        <Tooltip content="Delete">
          <Button variant="ghost" size="icon-sm" aria-label="Delete" onClick={onDelete}>
            <Trash2Icon />
          </Button>
        </Tooltip>
      </div>

      <div className="divide-y overflow-hidden rounded-xl border">
        {item.username && (
          <Row label={item.kind === 'login' ? 'Username' : 'Account'} onCopy={() => void copy('username', 'Username', false)}>
            <span className="flex items-center gap-1.5" data-selectable>
              <UserIcon className="size-3.5 text-muted-foreground" />
              {item.username}
            </span>
          </Row>
        )}
        {item.hasPassword && (
          <Row label={secretLabel} onCopy={() => void copy('password', secretLabel)}>
            <span className="font-mono text-[12.5px] break-all" data-selectable={revealed || undefined}>
              {revealed && secret ? secret.password : masked}
            </span>
          </Row>
        )}
        {item.hasTotp && <TotpRow item={item} onCopy={() => void copy('totp', 'Code')} />}
        {item.url && (
          <Row label="Website" onCopy={() => void copy('url', 'Website', false)}>
            <button className="flex min-w-0 items-center gap-1.5 text-primary hover:underline" onClick={() => BrowserOpenURL(withScheme(item.url))}>
              <span className="truncate">{item.url}</span>
              <ExternalLinkIcon className="size-3 shrink-0" />
            </button>
          </Row>
        )}
        {item.fieldCount > 0 &&
          (revealed && secret ? (
            secret.fields.map((f, i) => (
              <Row key={i} label={f.name} onCopy={() => void copy(`field:${i}`, f.name, f.hidden)}>
                <span className="font-mono text-[12.5px] break-all" data-selectable>
                  {f.value}
                </span>
              </Row>
            ))
          ) : (
            <Row label="Extra fields">
              <span className="text-muted-foreground">
                {item.fieldCount} hidden {item.fieldCount === 1 ? 'field' : 'fields'} · Reveal to see
              </span>
            </Row>
          ))}
        {item.hasNotes && (
          <Row label={item.kind === 'note' ? 'Note' : 'Notes'}>
            {revealed && secret ? (
              <pre className="font-mono text-[12px] leading-relaxed whitespace-pre-wrap" data-selectable>
                {secret.notes}
              </pre>
            ) : (
              <span className="text-muted-foreground">Hidden · Reveal to see</span>
            )}
          </Row>
        )}
      </div>
      <div className="text-[11px] text-muted-foreground">
        Updated {new Date(item.updatedAt).toLocaleString()} · Created {new Date(item.createdAt).toLocaleDateString()}
      </div>
    </div>
  )
}

function Row({ label, children, onCopy }: { label: string; children: React.ReactNode; onCopy?: () => void }) {
  return (
    <div className="group flex items-start gap-3 px-4 py-3">
      <div className="w-28 shrink-0 pt-0.5 text-xs text-muted-foreground">{label}</div>
      <div className="min-w-0 flex-1 text-[13px]">{children}</div>
      {onCopy && (
        <Tooltip content={`Copy ${label.toLowerCase()}`}>
          <Button variant="ghost" size="icon-xs" aria-label={`Copy ${label.toLowerCase()}`} onClick={onCopy}>
            <CopyIcon />
          </Button>
        </Tooltip>
      )}
    </div>
  )
}

function TotpRow({ item, onCopy }: { item: VaultItem; onCopy: () => void }) {
  const [code, setCode] = useState<TOTPCode | null>(null)
  const [now, setNow] = useState(Date.now())
  const [error, setError] = useState('')

  useEffect(() => {
    const t = setInterval(() => setNow(Date.now()), 1000)
    return () => clearInterval(t)
  }, [])
  const expired = !code || now >= code.expiresAt
  useEffect(() => {
    if (!expired) return
    api
      .vaultItemTotp(item.id)
      .then((c) => {
        setCode(c)
        setError('')
      })
      .catch((e) => setError(errorMessage(e)))
  }, [expired, item.id])

  const left = code ? Math.max(0, Math.ceil((code.expiresAt - now) / 1000)) : 0
  const pretty = code ? code.code.replace(/^(\d{3})(\d{3})$/, '$1 $2') : '··· ···'
  return (
    <Row label="Two-factor code" onCopy={onCopy}>
      {error ? (
        <span className="text-danger">{error}</span>
      ) : (
        <span className="flex items-center gap-2.5">
          <span className="font-mono text-[15px] font-semibold tracking-wider tabular-nums">{pretty}</span>
          {code && (
            <span className={cn('text-[11px] tabular-nums', left <= 5 ? 'text-danger' : 'text-muted-foreground')}>
              {left}s
            </span>
          )}
        </span>
      )}
    </Row>
  )
}

/** Only http(s) links are opened; anything else is treated as a host name. */
function withScheme(url: string) {
  return /^https?:\/\//i.test(url) ? url : `https://${url}`
}
