import type { ITheme } from '@xterm/xterm'
import { CheckIcon, MinusIcon, PaletteIcon, PlusIcon, SearchIcon } from 'lucide-react'
import { Popover } from 'radix-ui'
import { useEffect, useMemo, useRef, useState } from 'react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Tooltip } from '@/components/ui/tooltip'
import { errorMessage } from '@/lib/api'
import { TERMINAL_THEMES, type TerminalThemeDef } from '@/lib/terminalThemes'
import type { Settings } from '@/lib/types'
import { cn } from '@/lib/utils'
import { useSettings } from '@/stores/settings'

/** Applies a settings change right away and saves it in the background. */
async function change(patch: Partial<Settings>) {
  const { settings, setLocal, save } = useSettings.getState()
  if (!settings) return
  const next = { ...settings, ...patch }
  setLocal(next)
  try {
    await save(next)
  } catch (e) {
    setLocal(settings)
    toast.error('Could not change the terminal style', { description: errorMessage(e) })
  }
}

/** Steps to the previous or next color scheme. */
export function cycleTerminalTheme(dir: 1 | -1) {
  const cur = useSettings.getState().settings?.terminalTheme
  const i = TERMINAL_THEMES.findIndex((t) => t.id === cur)
  const next = TERMINAL_THEMES[(i + dir + TERMINAL_THEMES.length) % TERMINAL_THEMES.length]
  void change({ terminalTheme: next.id })
  toast(next.name, { id: 'terminal-theme', duration: 1200 })
}

export function changeFontSize(delta: number) {
  const size = useSettings.getState().settings?.terminalFontSize ?? 13
  const next = Math.max(8, Math.min(32, size + delta))
  if (next !== size) void change({ terminalFontSize: next })
}

function Swatch({ t }: { t: TerminalThemeDef }) {
  const c = t.theme
  return (
    <span className="flex h-6 w-16 shrink-0 items-center gap-[3px] rounded border px-1.5" style={{ background: c.background }}>
      <span className="mr-0.5 text-[10px] font-semibold" style={{ color: c.foreground }}>
        Aa
      </span>
      {[c.red, c.green, c.yellow, c.blue, c.magenta].map((col, i) => (
        <span key={i} className="size-1.5 rounded-full" style={{ background: col }} />
      ))}
    </span>
  )
}

/** Toolbar popover for switching color scheme, font size and cursor without opening Settings. */
export function AppearancePicker({ onPreview }: { onPreview: (theme: ITheme | null) => void }) {
  const settings = useSettings((s) => s.settings)
  const [open, setOpen] = useState(false)
  const [query, setQuery] = useState('')
  const [active, setActive] = useState(-1)
  const listRef = useRef<HTMLDivElement>(null)

  const themes = useMemo(() => {
    const q = query.trim().toLowerCase()
    const list = q ? TERMINAL_THEMES.filter((t) => t.name.toLowerCase().includes(q)) : TERMINAL_THEMES
    // Dark schemes first, then light, keeping the original order inside each group.
    return [...list.filter((t) => t.dark), ...list.filter((t) => !t.dark)]
  }, [query])

  useEffect(() => {
    if (!open) {
      onPreview(null)
      setQuery('')
      setActive(-1)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open])

  useEffect(() => {
    if (active >= 0) listRef.current?.querySelector(`[data-index="${active}"]`)?.scrollIntoView({ block: 'nearest' })
  }, [active])

  const move = (dir: 1 | -1) => {
    if (themes.length === 0) return
    const i = active < 0 ? Math.max(0, themes.findIndex((t) => t.id === settings?.terminalTheme)) : (active + dir + themes.length) % themes.length
    setActive(i)
    onPreview(themes[i].theme)
  }

  const pick = (t: TerminalThemeDef) => {
    void change({ terminalTheme: t.id })
    onPreview(null)
  }

  if (!settings) return null
  const firstLight = themes.findIndex((t) => !t.dark)

  return (
    <Popover.Root open={open} onOpenChange={setOpen}>
      <Tooltip content="Terminal style">
        <Popover.Trigger asChild>
          <Button variant={open ? 'secondary' : 'ghost'} size="icon-xs" aria-label="Terminal style">
            <PaletteIcon />
          </Button>
        </Popover.Trigger>
      </Tooltip>
      <Popover.Portal>
        <Popover.Content
          align="end"
          sideOffset={6}
          onOpenAutoFocus={(e) => {
            e.preventDefault()
            ;(e.currentTarget as HTMLElement).querySelector('input')?.focus()
          }}
          className="z-50 flex w-80 flex-col overflow-hidden rounded-lg border bg-popover text-popover-foreground shadow-xl data-[state=open]:animate-in data-[state=open]:fade-in-0 data-[state=open]:zoom-in-95"
        >
          <div className="flex flex-col gap-2 border-b p-2.5">
            <div className="flex items-center justify-between text-xs">
              <span className="text-muted-foreground">Font size</span>
              <div className="flex items-center gap-1">
                <Button variant="outline" size="icon-xs" aria-label="Smaller text" onClick={() => changeFontSize(-1)} disabled={settings.terminalFontSize <= 8}>
                  <MinusIcon />
                </Button>
                <span className="w-7 text-center tabular-nums">{settings.terminalFontSize}</span>
                <Button variant="outline" size="icon-xs" aria-label="Larger text" onClick={() => changeFontSize(1)} disabled={settings.terminalFontSize >= 32}>
                  <PlusIcon />
                </Button>
              </div>
            </div>
            <div className="flex items-center justify-between text-xs">
              <span className="text-muted-foreground">Cursor</span>
              <div className="flex rounded-md border p-0.5">
                {(['block', 'bar', 'underline'] as const).map((v) => (
                  <button
                    key={v}
                    onClick={() => void change({ cursorStyle: v })}
                    className={cn('rounded px-2 py-0.5 capitalize', settings.cursorStyle === v ? 'bg-accent text-foreground' : 'text-muted-foreground hover:text-foreground')}
                  >
                    {v}
                  </button>
                ))}
              </div>
            </div>
          </div>
          <div className="flex items-center gap-1.5 border-b px-2.5">
            <SearchIcon className="size-3.5 text-muted-foreground" />
            <input
              value={query}
              onChange={(e) => {
                setQuery(e.target.value)
                setActive(-1)
                onPreview(null)
              }}
              onKeyDown={(e) => {
                if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
                  e.preventDefault()
                  move(e.key === 'ArrowDown' ? 1 : -1)
                } else if (e.key === 'Enter') {
                  const t = themes[active] ?? (themes.length === 1 ? themes[0] : undefined)
                  if (t) pick(t)
                }
              }}
              placeholder={`Search ${TERMINAL_THEMES.length} color schemes`}
              spellCheck={false}
              className="h-8 flex-1 bg-transparent text-xs outline-none"
            />
          </div>
          <div ref={listRef} className="max-h-80 overflow-y-auto p-1" onMouseLeave={() => active < 0 && onPreview(null)}>
            {themes.length === 0 && <div className="px-2 py-3 text-center text-xs text-muted-foreground">No color schemes match</div>}
            {themes.map((t, i) => (
              <div key={t.id}>
                {(i === 0 || i === firstLight) && (
                  <div className="px-2 pt-1.5 pb-1 text-[10px] font-medium tracking-wide text-muted-foreground uppercase">{t.dark ? 'Dark' : 'Light'}</div>
                )}
                <button
                  data-index={i}
                  onClick={() => pick(t)}
                  onMouseEnter={() => {
                    setActive(-1)
                    onPreview(t.theme)
                  }}
                  className={cn('flex w-full items-center gap-2.5 rounded-md px-2 py-1 text-left text-xs hover:bg-accent', active === i && 'bg-accent')}
                >
                  <Swatch t={t} />
                  <span className="flex-1 truncate">{t.name}</span>
                  {settings.terminalTheme === t.id && <CheckIcon className="size-3.5 text-primary" />}
                </button>
              </div>
            ))}
          </div>
          <div className="border-t px-2.5 py-1.5 text-[10.5px] text-muted-foreground">Hover to preview, click to apply to all terminals.</div>
        </Popover.Content>
      </Popover.Portal>
    </Popover.Root>
  )
}
