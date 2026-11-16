import {
  AlertTriangleIcon,
  BanIcon,
  BotIcon,
  CheckCircle2Icon,
  CheckIcon,
  ChevronRightIcon,
  CircleXIcon,
  Loader2Icon,
  ServerIcon,
  Trash2Icon,
} from 'lucide-react'
import { useEffect, useState } from 'react'
import { toast } from 'sonner'
import { ConfirmDialog } from '@/components/common/ConfirmDialog'
import { CopyButton } from '@/components/common/CopyButton'
import { EmptyState } from '@/components/common/EmptyState'
import { HostAvatar } from '@/components/hosts/HostAvatar'
import { Field, Section } from '@/components/settings/Section'
import { Button } from '@/components/ui/button'
import { Switch } from '@/components/ui/switch'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { api, errorMessage, events, onEvent } from '@/lib/api'
import type { AgentLogEntry, AgentMode, Host, LocalAgentStatus, MCPClient } from '@/lib/types'
import { cn, relativeTime } from '@/lib/utils'
import { useAgent } from '@/stores/agent'
import { useSettings } from '@/stores/settings'
import { useSSHHosts } from '@/stores/vault'

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
  const logCount = useAgent((s) => s.log.length)
  // Agents run commands, so only SSH hosts can be opened to them.
  const hosts = useSSHHosts()
  const enabled = !!settings?.agentEnabled
  const [tab, setTab] = useState('setup')

  const setEnabled = async (on: boolean) => {
    try {
      useSettings.getState().setLocal(await api.setAgentEnabled(on))
      if (!on) toast.success('AI agent access is off', { description: 'Running actions were stopped.' })
    } catch (e) {
      toast.error('Could not change agent access', { description: errorMessage(e) })
    }
  }

  return (
    <Tabs value={tab} onValueChange={setTab} className="h-full min-h-0">
      <div className="flex h-14 shrink-0 items-center gap-3 border-b px-5">
        <h1 className="text-[15px] font-semibold">AI agents</h1>
        <span className="truncate text-xs text-muted-foreground">AI tools work on your servers through Remvyn and never see a key or password.</span>
      </div>
      <TabsList className="px-3">
        <TabsTrigger value="setup">Setup</TabsTrigger>
        <TabsTrigger value="activity">
          Activity
          {logCount > 0 && <span className="rounded bg-muted px-1 text-[10.5px] tabular-nums text-muted-foreground">{logCount}</span>}
        </TabsTrigger>
      </TabsList>
      <TabsContent value="setup" className="overflow-auto">
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
                    ? `${status.running} action${status.running === 1 ? '' : 's'} running.`
                    : 'Agents can use the servers you open below.'
                  : 'Off: agents get no access.'}
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

          <ConnectSection />
          <HostsSection hosts={hosts} modes={settings?.agentHosts ?? {}} enabled={enabled} />
          <KeysSection forwardToAI={!!settings?.forwardKeysToAI} />
        </div>
      </TabsContent>
      <TabsContent value="activity" className="flex flex-col">
        <ActivityTab />
      </TabsContent>
    </Tabs>
  )
}

/** Adds Remvyn's MCP server to the AI tools on this computer. */
function ConnectSection() {
  const [clients, setClients] = useState<MCPClient[] | null>(null)
  const [busy, setBusy] = useState<string | null>(null)
  const [manual, setManual] = useState<string | null>(null)
  const [showAll, setShowAll] = useState(false)
  const load = () =>
    api
      .mcpClients()
      .then(setClients)
      .catch(() => setClients([]))
  useEffect(() => {
    void load()
  }, [])

  const act = async (c: MCPClient, remove: boolean) => {
    setBusy(c.id)
    try {
      if (remove) await api.disconnectMCPClient(c.id)
      else await api.connectMCPClient(c.id)
      toast.success(remove ? `Removed from ${c.name}` : `Added to ${c.name}`, {
        description: remove ? undefined : `Restart ${c.name} (or start a new session) to pick it up.`,
      })
    } catch (e) {
      toast.error(`Could not change ${c.name}`, { description: errorMessage(e) })
    } finally {
      setBusy(null)
      void load()
    }
  }

  const found = (clients ?? []).filter((c) => c.installed || c.connected)
  const others = (clients ?? []).filter((c) => !c.installed && !c.connected)
  const shown = showAll ? [...found, ...others] : found

  return (
    <Section title="Connect your AI tools" description="Works with any MCP client. Remvyn needs to be open while the agent works.">
      {clients === null ? (
        <div className="px-4 py-4 text-xs text-muted-foreground">Looking for AI tools…</div>
      ) : (
        <>
          {shown.length === 0 && <div className="px-4 py-4 text-xs text-muted-foreground">No AI tools found on this computer.</div>}
          {shown.map((c) => (
            <ClientRow
              key={c.id}
              c={c}
              busy={busy === c.id}
              manualOpen={manual === c.id}
              onManual={() => setManual(manual === c.id ? null : c.id)}
              onAct={(remove) => void act(c, remove)}
            />
          ))}
          {others.length > 0 && (
            <button
              onClick={() => setShowAll((v) => !v)}
              className="flex w-full items-center gap-1 px-4 py-2 text-left text-xs text-muted-foreground hover:text-foreground"
            >
              <ChevronRightIcon className={cn('size-3.5 transition-transform', showAll && 'rotate-90')} />
              {showAll ? 'Hide tools not on this computer' : `${others.length} more (${others.map((c) => c.name).join(', ')})`}
            </button>
          )}
        </>
      )}
    </Section>
  )
}

function ClientRow({
  c,
  busy,
  manualOpen,
  onManual,
  onAct,
}: {
  c: MCPClient
  busy: boolean
  manualOpen: boolean
  onManual: () => void
  onAct: (remove: boolean) => void
}) {
  const stale = c.connected && !c.current
  return (
    <div>
      <div className="flex items-center gap-3 px-4 py-2.5">
        <div className="flex size-8 shrink-0 items-center justify-center rounded-lg bg-muted text-[12px] font-semibold text-muted-foreground">
          {c.name[0]}
        </div>
        <div className="min-w-0 flex-1">
          <div className="text-[13px] font-medium">{c.name}</div>
          <div className={cn('flex items-center gap-1 text-[11.5px]', stale ? 'text-warning' : c.connected ? 'text-success' : 'text-muted-foreground')}>
            {c.connected && !stale && <CheckIcon className="size-3" />}
            {stale ? 'Points at another copy of Remvyn' : c.connected ? 'Added' : c.installed ? 'Not added' : 'Not found on this computer'}
          </div>
        </div>
        <Button variant="ghost" size="xs" onClick={onManual} className="text-muted-foreground">
          {manualOpen ? 'Hide' : 'By hand'}
        </Button>
        {c.connected && !stale ? (
          <Button variant="ghost" size="xs" disabled={busy} onClick={() => onAct(true)}>
            {busy ? <Loader2Icon className="animate-spin" /> : null} Remove
          </Button>
        ) : (
          <Button variant={stale ? 'outline' : 'default'} size="xs" disabled={busy} onClick={() => onAct(false)} className="min-w-16">
            {busy ? <Loader2Icon className="animate-spin" /> : null} {stale ? 'Update' : 'Add'}
          </Button>
        )}
      </div>
      {manualOpen && (
        <div className="flex flex-col gap-1.5 px-4 pb-3">
          <p className="text-xs text-muted-foreground">
            {c.manualKind === 'command' ? (
              'Run in a terminal:'
            ) : (
              <>
                Add to <span data-selectable className="font-mono">{c.configPath}</span>:
              </>
            )}
          </p>
          <div className="flex items-start gap-2 rounded-lg border bg-muted/40 py-2 pr-1.5 pl-3">
            <pre data-selectable className="min-w-0 flex-1 overflow-x-auto py-0.5 font-mono text-[12px] leading-relaxed whitespace-pre-wrap break-all">
              {c.manual}
            </pre>
            <CopyButton text={c.manual} tooltip="Copy" />
          </div>
        </div>
      )}
    </div>
  )
}

/** Forwarded keys for agents, and the local SSH agent for apps on this computer. */
function KeysSection({ forwardToAI }: { forwardToAI: boolean }) {
  const [status, setStatus] = useState<LocalAgentStatus | null>(null)
  const [showSetup, setShowSetup] = useState(false)
  useEffect(() => {
    void api
      .localAgentStatus()
      .then(setStatus)
      .catch(() => {})
    return onEvent<LocalAgentStatus>(events.LOCAL_AGENT_STATUS, setStatus)
  }, [])
  const toggleLocal = async (on: boolean) => {
    try {
      useSettings.getState().setLocal(await api.setLocalKeyAgent(on))
      setStatus(await api.localAgentStatus())
      if (on) setShowSetup(true)
    } catch (e) {
      toast.error('Could not change the setting', { description: errorMessage(e) })
    }
  }
  const sock = status?.socketPath ?? ''
  const sshConfig = `Host github.com\n  IdentityAgent "${sock}"`
  const env = `export SSH_AUTH_SOCK=${shellQuote(sock)}`
  return (
    <Section title="SSH keys" description="Your keys never leave Remvyn, and you're asked each time one is used.">
      <Field label="Agents can use forwarded keys" hint="On servers with key forwarding on, e.g. for git pull from a private repo.">
        <Switch
          checked={forwardToAI}
          aria-label="Agents can use forwarded keys"
          onCheckedChange={(on) =>
            void api
              .setForwardKeysToAI(on)
              .then((s) => useSettings.getState().setLocal(s))
              .catch((e) => toast.error('Could not change the setting', { description: errorMessage(e) }))
          }
        />
      </Field>
      {status?.supported && (
        <div>
          <Field label="Apps on this computer can use my keys" hint="git, ssh and AI agents on this computer, e.g. to git push.">
            <Switch checked={status.enabled} aria-label="Apps on this computer can use my keys" onCheckedChange={(on) => void toggleLocal(on)} />
          </Field>
          {status.enabled && status.listenError && (
            <div className="mx-4 mb-3 flex items-start gap-2 rounded-lg border border-warning/40 bg-warning/10 px-3 py-2 text-xs text-warning">
              <AlertTriangleIcon className="mt-0.5 size-3.5 shrink-0" />
              <span>The SSH agent couldn't start: {status.listenError}</span>
            </div>
          )}
          {status.enabled && (
            <div className="px-4 pb-3">
              <button onClick={() => setShowSetup((v) => !v)} className="flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground">
                <ChevronRightIcon className={cn('size-3.5 transition-transform', showSetup && 'rotate-90')} /> How to use it
              </button>
              {showSetup && (
                <div className="mt-2 flex flex-col gap-2">
                  <p className="text-xs text-muted-foreground">
                    Add to <span className="font-mono">~/.ssh/config</span>:
                  </p>
                  <div className="flex items-start gap-2 rounded-lg border bg-muted/40 py-2 pr-1.5 pl-3">
                    <pre data-selectable className="min-w-0 flex-1 overflow-x-auto font-mono text-[12px] leading-relaxed">
                      {sshConfig}
                    </pre>
                    <CopyButton text={sshConfig} tooltip="Copy" />
                  </div>
                  <p className="text-xs text-muted-foreground">Or, for one shell:</p>
                  <div className="flex items-start gap-2 rounded-lg border bg-muted/40 py-2 pr-1.5 pl-3">
                    <code data-selectable className="min-w-0 flex-1 py-0.5 font-mono text-[12px] leading-relaxed break-all">
                      {env}
                    </code>
                    <CopyButton text={env} tooltip="Copy" />
                  </div>
                </div>
              )}
            </div>
          )}
        </div>
      )}
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
          <span className="text-foreground">Ask first</span>: you approve each action. <span className="text-foreground">Always allow</span>: no prompts.
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

function ActivityTab() {
  const { log, logLoaded, loadLog } = useAgent()
  const [open, setOpen] = useState<string | null>(null)
  const [clearing, setClearing] = useState(false)
  useEffect(() => {
    loadLog().catch(() => {})
  }, [loadLog])
  return (
    <>
      <div className="flex h-10 shrink-0 items-center justify-between gap-3 border-b px-5 text-xs text-muted-foreground">
        <span>Everything agents did, newest first. Stored encrypted.</span>
        {log.length > 0 && (
          <Button variant="ghost" size="xs" onClick={() => setClearing(true)}>
            <Trash2Icon /> Clear
          </Button>
        )}
      </div>
      <div className="min-h-0 flex-1 overflow-auto">
        {log.length === 0 ? (
          <div className="px-4 py-10 text-center text-xs text-muted-foreground">{logLoaded ? 'No agent activity yet.' : 'Loading…'}</div>
        ) : (
          <div className="divide-y">
            {log.map((e) => (
              <LogRow key={e.id} e={e} open={open === e.id} onToggle={() => setOpen(open === e.id ? null : e.id)} />
            ))}
          </div>
        )}
      </div>
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
    </>
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
      <button onClick={onToggle} className="flex w-full items-center gap-2.5 px-5 py-2 text-left hover:bg-accent/40">
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
        <div className="flex flex-col gap-2 px-5 pb-3">
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
