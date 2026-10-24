import { FolderOpenIcon, MonitorIcon, MoonIcon, SunIcon } from 'lucide-react'
import { useEffect, useState } from 'react'
import { toast } from 'sonner'
import { ConfirmDialog } from '@/components/common/ConfirmDialog'
import { LoadingBlock } from '@/components/common/Spinner'
import { ProBadge } from '@/components/monitor/ProGate'
import { SecuritySection } from '@/components/settings/SecuritySection'
import { Field, Section } from '@/components/settings/Section'
import { SyncSection } from '@/components/settings/SyncSection'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { api, errorMessage } from '@/lib/api'
import type { Settings, Theme } from '@/lib/types'
import { TERMINAL_THEMES } from '@/lib/terminalThemes'
import { cn } from '@/lib/utils'
import { useSessions } from '@/stores/sessions'
import { useSettings } from '@/stores/settings'
import { useVault } from '@/stores/vault'

/** Number input that commits on blur/Enter so intermediate values aren't validated. */
function NumField({ value, onCommit, min, max, className }: { value: number; onCommit: (n: number) => void; min: number; max: number; className?: string }) {
  const [text, setText] = useState(String(value))
  useEffect(() => setText(String(value)), [value])
  const commit = () => {
    const n = Number(text)
    if (text === '' || !Number.isFinite(n) || n < min || n > max) {
      toast.error(`Enter a number between ${min} and ${max}`)
      setText(String(value))
      return
    }
    if (n !== value) onCommit(n)
  }
  return (
    <Input
      type="number"
      min={min}
      max={max}
      className={className}
      value={text}
      onChange={(e) => setText(e.target.value)}
      onBlur={commit}
      onKeyDown={(e) => e.key === 'Enter' && (e.target as HTMLInputElement).blur()}
    />
  )
}

export function SettingsPage() {
  const { settings, info, save } = useSettings()
  const [form, setForm] = useState<Settings | null>(settings)
  const [confirmWipe, setConfirmWipe] = useState(false)

  useEffect(() => setForm(settings), [settings])
  if (!form || !settings) return <LoadingBlock />

  // Settings apply immediately, like a native preferences window.
  const apply = async (patch: Partial<Settings>) => {
    const next = { ...form, ...patch }
    setForm(next)
    try {
      await save(next)
    } catch (e) {
      toast.error('Could not save setting', { description: errorMessage(e) })
      setForm(settings)
    }
  }

  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className="flex h-14 shrink-0 items-center border-b px-5">
        <h1 className="text-[15px] font-semibold">Settings</h1>
      </div>
      <div className="min-h-0 flex-1 overflow-auto">
        <div className="mx-auto flex max-w-3xl flex-col gap-5 p-6">
          {info?.initError && <div className="rounded-lg border border-danger/40 bg-danger/10 p-3 text-xs text-danger">Flux failed to initialise: {info.initError}</div>}

          <Section title="Appearance">
            <Field label="Theme">
              <div className="flex rounded-md border p-0.5">
                {([
                  ['dark', MoonIcon, 'Dark'],
                  ['light', SunIcon, 'Light'],
                  ['system', MonitorIcon, 'System'],
                ] as const).map(([v, Icon, label]) => (
                  <button
                    key={v}
                    onClick={() => void apply({ theme: v as Theme })}
                    className={cn('flex items-center gap-1.5 rounded px-2.5 py-1 text-xs', form.theme === v ? 'bg-accent text-foreground' : 'text-muted-foreground hover:text-foreground')}
                  >
                    <Icon className="size-3.5" /> {label}
                  </button>
                ))}
              </div>
            </Field>
          </Section>

          <Section title="Terminal">
            <Field label="Color scheme">
              <Select value={form.terminalTheme} onValueChange={(terminalTheme) => void apply({ terminalTheme })}>
                <SelectTrigger className="w-44">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {TERMINAL_THEMES.map((t) => (
                    <SelectItem key={t.id} value={t.id}>
                      <span className="flex items-center gap-2">
                        <span className="size-3 rounded-sm border" style={{ background: t.theme.background }} />
                        {t.name}
                      </span>
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </Field>
            <Field label="Font size">
              <NumField min={8} max={32} className="w-20" value={form.terminalFontSize} onCommit={(terminalFontSize) => void apply({ terminalFontSize })} />
            </Field>
            <div className="flex flex-col gap-1.5 px-4 py-3">
              <div className="text-[13px] font-medium">Font family</div>
              <Input
                className="font-mono text-[12px]"
                defaultValue={form.terminalFontFamily}
                onBlur={(e) => e.target.value !== form.terminalFontFamily && void apply({ terminalFontFamily: e.target.value })}
              />
            </div>
            <Field label="Cursor">
              <div className="flex items-center gap-3">
                <Select value={form.cursorStyle} onValueChange={(v) => void apply({ cursorStyle: v as Settings['cursorStyle'] })}>
                  <SelectTrigger className="w-32">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="block">Block</SelectItem>
                    <SelectItem value="bar">Bar</SelectItem>
                    <SelectItem value="underline">Underline</SelectItem>
                  </SelectContent>
                </Select>
                <label className="flex items-center gap-2 text-xs text-muted-foreground">
                  Blink <Switch checked={form.cursorBlink} onCheckedChange={(cursorBlink) => void apply({ cursorBlink })} />
                </label>
              </div>
            </Field>
            <Field label="Scrollback" hint="Lines kept in memory per terminal.">
              <NumField min={100} max={200000} className="w-28" value={form.scrollback} onCommit={(scrollback) => void apply({ scrollback })} />
            </Field>
            <Field label="Copy on select" hint="Selected text is copied to the clipboard automatically.">
              <Switch checked={form.copyOnSelect} onCheckedChange={(copyOnSelect) => void apply({ copyOnSelect })} />
            </Field>
            <Field label="Confirm before closing a connected tab">
              <Switch checked={form.confirmOnClose} onCheckedChange={(confirmOnClose) => void apply({ confirmOnClose })} />
            </Field>
          </Section>

          <Section title="Connection">
            <Field label="Connect timeout" hint="Applies to new connections.">
              <div className="flex items-center gap-2">
                <NumField min={3} max={300} className="w-20" value={form.connectTimeout} onCommit={(connectTimeout) => void apply({ connectTimeout })} />
                <span className="text-xs text-muted-foreground">sec</span>
              </div>
            </Field>
            <Field label="Keepalive interval" hint="Sends a keepalive and drops dead connections after 3 missed replies. 0 disables it.">
              <div className="flex items-center gap-2">
                <NumField min={0} max={3600} className="w-20" value={form.keepaliveSeconds} onCommit={(keepaliveSeconds) => void apply({ keepaliveSeconds })} />
                <span className="text-xs text-muted-foreground">sec</span>
              </div>
            </Field>
            <Field label="Show hidden files in SFTP by default">
              <Switch checked={form.showHiddenFiles} onCheckedChange={(showHiddenFiles) => void apply({ showHiddenFiles })} />
            </Field>
          </Section>

          <Section
            title="Flux Pro"
            description="Historical metrics, custom alerts, uptime monitoring and the multi-server dashboard."
          >
            <Field
              label={
                <span className="flex items-center gap-2">
                  Enable Flux Pro <ProBadge />
                </span>
              }
              hint="Payments aren’t connected yet. This switch unlocks Pro on this computer."
            >
              <Switch checked={form.proEnabled} onCheckedChange={(proEnabled) => void apply({ proEnabled })} />
            </Field>
          </Section>

          <Section title="Monitoring" description="Live metrics are free. Background monitoring, history and alerts need Flux Pro and run while Flux is open.">
            <Field label="Background check interval" hint="How often monitored servers are sampled when you aren’t looking at them.">
              <div className="flex items-center gap-2">
                <NumField min={5} max={3600} className="w-20" value={form.monitorIntervalSec} onCommit={(monitorIntervalSec) => void apply({ monitorIntervalSec })} />
                <span className="text-xs text-muted-foreground">sec</span>
              </div>
            </Field>
            <Field label="Keep history for" hint="Metrics, uptime results and resolved alerts older than this are deleted.">
              <div className="flex items-center gap-2">
                <NumField min={1} max={365} className="w-20" value={form.metricsRetentionDays} onCommit={(metricsRetentionDays) => void apply({ metricsRetentionDays })} />
                <span className="text-xs text-muted-foreground">days</span>
              </div>
            </Field>
            <Field label="Desktop notifications for alerts">
              <div className="flex items-center gap-3">
                <Button variant="ghost" size="xs" onClick={() => void api.sendTestNotification()}>
                  Send test
                </Button>
                <Switch checked={form.alertNotifications} onCheckedChange={(alertNotifications) => void apply({ alertNotifications })} />
              </div>
            </Field>
          </Section>

          <SecuritySection form={form} apply={apply} />

          <SyncSection />

          <Section title="Data">
            <Field label="Data folder" hint={<span className="font-mono">{info?.dataDir}</span>}>
              <Button variant="outline" size="sm" onClick={() => void api.revealDataDir()}>
                <FolderOpenIcon /> Open
              </Button>
            </Field>
            <Field label="Clear all data" hint="Disconnects every session and deletes hosts, keys, snippets, forwarding rules, known hosts and settings.">
              <Button variant="destructive" size="sm" onClick={() => setConfirmWipe(true)}>
                Clear data…
              </Button>
            </Field>
            <Field label="Version" hint={info?.platform}>
              <span className="font-mono text-xs text-muted-foreground">Flux {info?.version}</span>
            </Field>
          </Section>
        </div>
      </div>
      <ConfirmDialog
        open={confirmWipe}
        onOpenChange={setConfirmWipe}
        title="Delete all Flux data?"
        description="All sessions are disconnected. Hosts, saved passwords, private keys, snippets, forwarding rules and known hosts are permanently deleted. Export any keys you need first."
        confirmLabel="Delete everything"
        destructive
        onConfirm={async () => {
          try {
            const s = await api.clearAppData()
            useSettings.getState().setLocal(s)
            for (const t of useSessions.getState().tabs) useSessions.getState().close(t.key)
            await useVault.getState().loadAll()
            toast.success('All data cleared')
          } catch (e) {
            toast.error('Could not clear data', { description: errorMessage(e) })
          }
        }}
      />
    </div>
  )
}
