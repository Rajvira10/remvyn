import {
  AlertTriangleIcon,
  BanIcon,
  BotIcon,
  CheckCircle2Icon,
  ChevronRightIcon,
  CircleXIcon,
  ServerIcon,
  Trash2Icon,
} from 'lucide-react'
import { useEffect, useState } from 'react'
import { toast } from 'sonner'
import { ConfirmDialog } from '@/components/common/ConfirmDialog'
import { CopyButton } from '@/components/common/CopyButton'
import { EmptyState } from '@/components/common/EmptyState'
import { HostAvatar } from '@/components/hosts/HostAvatar'
import { Section } from '@/components/settings/Section'
import { Button } from '@/components/ui/button'
import { Switch } from '@/components/ui/switch'
import { api, errorMessage } from '@/lib/api'
import type { AgentLogEntry, AgentMode, Host } from '@/lib/types'
import { cn, relativeTime } from '@/lib/utils'
import { useAgent } from '@/stores/agent'
import { useSettings } from '@/stores/settings'
import { useVault } from '@/stores/vault'

/** Quotes a path for a POSIX shell when it needs it. */
function shellQuote(s: string) {
  if (/^[\w@%+=:,./-]+$/.test(s)) return s
  // Windows paths: double quotes work in cmd, PowerShell and Git Bash.
  if (/^[A-Za-z]:\\/.test(s)) return `"${s}"`
  return `'${s.replace(/'/g, `'\\''`)}'`
}

const toolLabel: Record<string, string> = {
  run_command: 'Command',
  read_file: 'Read',
  write_file: 'Write',
  list_dir: 'List',
}

export function AgentsPage() {
  const settings = useSettings((s) => s.settings)
  const status = useAgent((s) => s.status)
  const hosts = useVault((s) => s.hosts)
  const enabled = !!settings?.agentEnabled

  const setEnabled = async (on: boolean) => {
    try {
      useSettings.getState().setLocal(await api.setAgentEnabled(on))
      if (!on) toast.success('AI agent access is off', { description: 'Running actions were stopped.' })
    } catch (e) {
      toast.error('Could not change agent access', { description: errorMessage(e) })
    }
  }

  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className="flex h-14 shrink-0 items-center gap-3 border-b px-5">
        <h1 className="text-[15px] font-semibold">AI agents</h1>
        <span className="truncate text-xs text-muted-foreground">
          Let Claude Code work on your servers through Remvyn, without ever seeing a key or password.
        </span>
      </div>
      <div className="min-h-0 flex-1 overflow-auto">
        <div className="mx-auto flex max-w-3xl flex-col gap-5 p-5">
          <section className="flex items-center gap-4 rounded-xl border bg-card px-4 py-3.5">
            <div className={cn('flex size-10 shrink-0 items-center justify-center rounded-lg', enabled ? 'bg-primary/15 text-primary' : 'bg-muted text-muted-foreground')}>
              <BotIcon className="size-5" />
            </div>
            <div className="min-w-0 flex-1">
              <div className="text-[13.5px] font-semibold">Allow AI agents</div>
              <div className="mt-0.5 text-xs text-muted-foreground">
                {enabled
                  ? status?.running
                    ? `${status.running} action${status.running === 1 ? '' : 's'} running. Turning this off stops them immediately.`
                    : 'Agents can use the servers you open below. Turning this off stops everything immediately.'
                  : 'Off: agents get no access to anything.'}
              </div>
            </div>
            <Switch checked={enabled} onCheckedChange={(on) => void setEnabled(on)} aria-label="Allow AI agents" />
          </section>

          {status && !status.listening && status.listenError && (
            <div className="flex items-start gap-2 rounded-lg border border-warning/40 bg-warning/10 px-3 py-2 text-xs text-warning">
              <AlertTriangleIcon className="mt-0.5 size-3.5 shrink-0" />
              <span>Agents can't reach Remvyn: {status.listenError}</span>
            </div>
          )}

          <SetupSection command={status?.command ?? []} />
          <HostsSection hosts={hosts} modes={settings?.agentHosts ?? {}} enabled={enabled} />
          <ActivitySection />
        </div>
      </div>
    </div>
  )
}

function SetupSection({ command }: { command: string[] }) {
  const [exe = 'remvyn', ...args] = command
  const claudeCmd = `claude mcp add --scope user remvyn -- ${[exe, ...args].map(shellQuote).join(' ')}`
  const json = JSON.stringify({ mcpServers: { remvyn: { command: exe, args } } }, null, 2)
  const [showJson, setShowJson] = useState(false)
  return (
    <Section title="Connect Claude Code" description="Run this once in a terminal. Remvyn has to be open for agents to reach your servers.">
      <div className="flex flex-col gap-3 px-4 py-3">
        <div className="flex items-start gap-2 rounded-lg border bg-muted/40 py-2 pr-1.5 pl-3">
          <code data-selectable className="min-w-0 flex-1 py-0.5 font-mono text-[12px] leading-relaxed break-all">
            {claudeCmd}
          </code>
          <CopyButton text={claudeCmd} tooltip="Copy command" />
        </div>
        <p className="text-xs text-muted-foreground">
          Then ask Claude something like <span className="text-foreground">“check disk space on my prod server”</span>. It sees only the servers you
          open below, and Remvyn runs every action with the credentials in your vault.
        </p>
        <button onClick={() => setShowJson((v) => !v)} className="flex items-center gap-1 self-start text-xs text-muted-foreground hover:text-foreground">
          <ChevronRightIcon className={cn('size-3.5 transition-transform', showJson && 'rotate-90')} /> Other MCP clients (Cursor, Codex, Claude Desktop…)
        </button>
        {showJson && (
          <div className="flex items-start gap-2 rounded-lg border bg-muted/40 py-2 pr-1.5 pl-3">
            <pre data-selectable className="min-w-0 flex-1 overflow-x-auto font-mono text-[12px] leading-relaxed">
              {json}
            </pre>
            <CopyButton text={json} tooltip="Copy JSON" />
          </div>
        )}
      </div>
    </Section>
  )
}

const MODES: { v: AgentMode | ''; label: string }[] = [
  { v: '', label: 'No access' },
  { v: 'ask', label: 'Ask first' },
  { v: 'auto', label: 'Always allow' },
]

function HostsSection({ hosts, modes, enabled }: { hosts: Host[]; modes: Record<string, AgentMode>; enabled: boolean }) {
  const allGrants = useAgent((s) => s.status?.grants)
  // Re-render every 30 seconds so expired "allowed until" labels go away.
  const [now, setNow] = useState(Date.now)
  useEffect(() => {
    const t = setInterval(() => setNow(Date.now()), 30_000)
    return () => clearInterval(t)
  }, [])
  const grants = (allGrants ?? []).filter((g) => g.until > now)
  const setMode = async (hostId: string, mode: AgentMode | '') => {
    try {
      useSettings.getState().setLocal(await api.setAgentHostMode(hostId, mode))
    } catch (e) {
      toast.error('Could not change access', { description: errorMessage(e) })
    }
  }
  const opened = hosts.filter((h) => modes[h.id]).length
  return (
    <Section
      title="Servers"
      description={
        <>
          Choose what agents may use. <span className="text-foreground">Ask first</span> shows each command in Remvyn for you to approve;{' '}
          <span className="text-foreground">Always allow</span> runs anything without asking.
        </>
      }
    >
      {hosts.length === 0 ? (
        <div className="px-4 py-6">
          <EmptyState icon={ServerIcon} title="No hosts yet">
            Add a host first, then open it to agents here.
          </EmptyState>
        </div>
      ) : (
        <>
          {hosts.map((h) => {
            const mode = modes[h.id] ?? ''
            const grant = grants.find((g) => g.hostId === h.id)
            return (
              <div key={h.id} className={cn('flex items-center gap-3 px-4 py-2.5', !enabled && 'opacity-60')}>
                <HostAvatar label={h.label} color={h.color} className="size-8 text-[11px]" />
                <div className="min-w-0 flex-1">
                  <div className="truncate text-[13px] font-medium">{h.label}</div>
                  <div className="truncate font-mono text-[11.5px] text-muted-foreground">
                    {h.username ? `${h.username}@` : ''}
                    {h.hostname}
                    {h.port !== 22 ? `:${h.port}` : ''}
                    {grant && mode === 'ask' && (
                      <span className="ml-2 font-sans text-success">· allowed until {new Date(grant.until).toLocaleTimeString([], { hour: 'numeric', minute: '2-digit' })}</span>
                    )}
                  </div>
                </div>
                <div role="radiogroup" aria-label={`Agent access for ${h.label}`} className="flex shrink-0 rounded-md border p-0.5">
                  {MODES.map((m) => (
                    <button
                      key={m.v || 'off'}
                      role="radio"
                      aria-checked={mode === m.v}
                      onClick={() => mode !== m.v && void setMode(h.id, m.v)}
                      className={cn(
                        'rounded px-2.5 py-1 text-xs transition-colors',
                        mode === m.v
                          ? m.v === 'auto'
                            ? 'bg-warning/15 font-medium text-warning'
                            : m.v === 'ask'
                              ? 'bg-primary/15 font-medium text-primary'
                              : 'bg-accent font-medium text-foreground'
                          : 'text-muted-foreground hover:text-foreground',
                      )}
                    >
                      {m.label}
                    </button>
                  ))}
                </div>
              </div>
            )
          })}
          {(opened > 0 || grants.length > 0) && (
            <div className="flex items-center justify-between gap-3 px-4 py-2.5 text-xs text-muted-foreground">
              <span>
                {opened} of {hosts.length} open to agents
                {!enabled && ' (agent access is off)'}
              </span>
              {grants.length > 0 && (
                <Button variant="ghost" size="xs" onClick={() => void api.revokeAgentGrants()}>
                  Ask again everywhere
                </Button>
              )}
            </div>
          )}
        </>
      )}
    </Section>
  )
}

function ActivitySection() {
  const { log, logLoaded, loadLog } = useAgent()
  const [open, setOpen] = useState<string | null>(null)
  const [clearing, setClearing] = useState(false)
  useEffect(() => {
    loadLog().catch(() => {})
  }, [loadLog])
  return (
    <Section title="Activity" description="Everything agents did, newest first. Commands and output are stored encrypted, like your passwords.">
      {log.length === 0 ? (
        <div className="px-4 py-6 text-center text-xs text-muted-foreground">{logLoaded ? 'No agent activity yet.' : 'Loading…'}</div>
      ) : (
        <>
          {log.map((e) => (
            <LogRow key={e.id} e={e} open={open === e.id} onToggle={() => setOpen(open === e.id ? null : e.id)} />
          ))}
          <div className="flex justify-end px-4 py-2">
            <Button variant="ghost" size="xs" onClick={() => setClearing(true)}>
              <Trash2Icon /> Clear activity
            </Button>
          </div>
        </>
      )}
      <ConfirmDialog
        open={clearing}
        onOpenChange={setClearing}
        title="Clear agent activity?"
        description="The record of what agents did on your servers will be deleted from this device."
        confirmLabel="Clear"
        destructive
        onConfirm={async () => {
          await api.clearAgentLog()
          useAgent.getState().clearLog()
        }}
      />
    </Section>
  )
}

function LogRow({ e, open, onToggle }: { e: AgentLogEntry; open: boolean; onToggle: () => void }) {
  const icon =
    e.status === 'ok' ? (
      <CheckCircle2Icon className="size-3.5 text-success" />
    ) : e.status === 'denied' ? (
      <BanIcon className="size-3.5 text-muted-foreground" />
    ) : (
      <CircleXIcon className="size-3.5 text-danger" />
    )
  const statusText =
    e.status === 'denied'
      ? 'denied'
      : e.exitCode !== undefined && e.exitCode !== null
        ? `exit ${e.exitCode}`
        : e.status === 'error'
          ? 'error'
          : ''
  return (
    <div>
      <button onClick={onToggle} className="flex w-full items-center gap-2.5 px-4 py-2 text-left hover:bg-accent/40">
        {icon}
        <span className="w-14 shrink-0 text-[11px] text-muted-foreground">{toolLabel[e.tool] ?? e.tool}</span>
        <span className="min-w-0 flex-1 truncate font-mono text-[12px]">{e.summary || '(details unavailable)'}</span>
        <span className="shrink-0 text-[11px] text-muted-foreground">{e.hostLabel}</span>
        <span className="w-12 shrink-0 text-right text-[11px] text-muted-foreground tabular-nums">{statusText}</span>
        <span className="w-16 shrink-0 text-right text-[11px] text-muted-foreground" title={new Date(e.at).toLocaleString()}>
          {relativeTime(e.at)}
        </span>
      </button>
      {open && (
        <div className="flex flex-col gap-2 px-4 pb-3">
          <div className="flex flex-wrap gap-x-4 gap-y-1 text-[11px] text-muted-foreground">
            <span>{e.client}</span>
            <span>{new Date(e.at).toLocaleString()}</span>
            <span>{e.durationMs < 1000 ? `${e.durationMs} ms` : `${(e.durationMs / 1000).toFixed(1)} s`}</span>
            <span>
              {e.approval === 'auto'
                ? 'ran without asking'
                : e.approval === 'once'
                  ? 'you allowed it'
                  : e.approval === 'session'
                    ? 'allowed for 30 min'
                    : 'not allowed'}
            </span>
          </div>
          <pre data-selectable className="max-h-72 overflow-auto rounded-lg border bg-muted/40 p-3 font-mono text-[11.5px] leading-relaxed break-all whitespace-pre-wrap">
            {e.output || '(no output)'}
          </pre>
        </div>
      )}
    </div>
  )
}
