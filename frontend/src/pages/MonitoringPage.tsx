import {
  ActivityIcon,
  ArrowDownIcon,
  ArrowLeftIcon,
  ArrowUpIcon,
  BellIcon,
  BellRingIcon,
  BoxIcon,
  GlobeIcon,
  LayoutDashboardIcon,
  PencilIcon,
  PlusIcon,
  ServerIcon,
  Trash2Icon,
  WrenchIcon,
  ZapIcon,
} from 'lucide-react'
import { useCallback, useEffect, useMemo, useState } from 'react'
import { toast } from 'sonner'
import { ConfirmDialog } from '@/components/common/ConfirmDialog'
import { EmptyState } from '@/components/common/EmptyState'
import { HostAvatar } from '@/components/hosts/HostAvatar'
import { HistoryChart, Meter, formatRate, pct } from '@/components/monitor/charts'
import { MonitorPanel, StatusLine } from '@/components/monitor/MonitorPanel'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { api, errorMessage, events, onEvent } from '@/lib/api'
import type { AlertEvent, AlertMetric, AlertRule, Host, MetricPoint, Settings, UptimeCheck, UptimeResult, UptimeSummary } from '@/lib/types'
import { cn, relativeTime } from '@/lib/utils'
import { useMonitor } from '@/stores/monitor'
import { useSessions } from '@/stores/sessions'
import { useSettings } from '@/stores/settings'
import { useVault } from '@/stores/vault'

export function MonitoringPage() {
  const [detail, setDetail] = useState<string | null>(null)
  const host = useVault((s) => s.hosts.find((h) => h.id === detail))
  const openAlerts = useMonitor((s) => Object.keys(s.openAlerts).length)
  if (detail && host) return <HostDetail host={host} onBack={() => setDetail(null)} />

  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className="flex h-14 shrink-0 items-center gap-3 border-b px-5">
        <h1 className="text-[15px] font-semibold">Monitoring</h1>
      </div>
      <Tabs defaultValue="dashboard" className="min-h-0 flex-1">
        <TabsList className="px-4">
          <TabsTrigger value="dashboard">
            <LayoutDashboardIcon /> Dashboard
          </TabsTrigger>
          <TabsTrigger value="alerts">
            <BellIcon /> Alerts
            {openAlerts > 0 && <span className="rounded-full bg-danger px-1.5 text-[10px] leading-4 text-white">{openAlerts}</span>}
          </TabsTrigger>
          <TabsTrigger value="uptime">
            <GlobeIcon /> Uptime
          </TabsTrigger>
        </TabsList>
        <TabsContent value="dashboard" className="overflow-auto">
          <Dashboard onOpen={setDetail} />
        </TabsContent>
        <TabsContent value="alerts" className="overflow-auto">
          <AlertsTab />
        </TabsContent>
        <TabsContent value="uptime" className="overflow-auto">
          <UptimeTab />
        </TabsContent>
      </Tabs>
    </div>
  )
}

// ---------------------------------------------------------------- dashboard

async function saveMonitored(ids: string[]) {
  const st = useSettings.getState().settings
  if (!st) return
  try {
    await useSettings.getState().save({ ...st, monitorHostIds: ids } as Settings)
  } catch (e) {
    toast.error('Could not update monitored servers', { description: errorMessage(e) })
  }
}

function Dashboard({ onOpen }: { onOpen: (id: string) => void }) {
  const hosts = useVault((s) => s.hosts)
  const monitoredIds = useSettings((s) => s.settings?.monitorHostIds)
  const interval = useSettings((s) => s.settings?.monitorIntervalSec ?? 30)
  const [choosing, setChoosing] = useState(false)
  const monitored = useMemo(() => hosts.filter((h) => monitoredIds?.includes(h.id)), [hosts, monitoredIds])
  const others = useMemo(() => hosts.filter((h) => !monitoredIds?.includes(h.id)), [hosts, monitoredIds])

  // Load the current state for background-monitored hosts.
  useEffect(() => {
    void api.monitorStatuses().then((sts) => sts.forEach((s) => useMonitor.getState().applyStatus(s)))
    for (const h of monitored) void api.monitorLatest(h.id).then((m) => m && useMonitor.getState().applySample(m))
  }, [monitored])

  if (hosts.length === 0) {
    return (
      <EmptyState icon={ServerIcon} title="No hosts to monitor" className="h-auto py-20">
        Add a host in Hosts first. Remvyn reads CPU, memory, disk, network, Docker and services over SSH; nothing needs to be installed on the server.
      </EmptyState>
    )
  }

  return (
    <div className="flex flex-col gap-6 p-5">
      <section>
        <div className="mb-3 flex items-center gap-2">
          <h2 className="text-[11.5px] font-semibold tracking-wide text-muted-foreground uppercase">Monitored servers</h2>
          <span className="text-[11px] text-muted-foreground">checked every {interval}s while Remvyn is open</span>
          <Button size="sm" variant="outline" className="ml-auto" onClick={() => setChoosing(true)}>
            <PlusIcon /> Choose servers
          </Button>
        </div>
        {monitored.length === 0 ? (
          <button
            onClick={() => setChoosing(true)}
            className="flex w-full flex-col items-center gap-1 rounded-xl border border-dashed py-8 text-[13px] text-muted-foreground hover:bg-accent/30"
          >
            <PlusIcon className="size-5" />
            Choose servers to monitor in the background
          </button>
        ) : (
          <div className="grid grid-cols-[repeat(auto-fill,minmax(280px,1fr))] gap-3">
            {monitored.map((h) => (
              <ServerCard key={h.id} host={h} onOpen={() => onOpen(h.id)} />
            ))}
          </div>
        )}
      </section>

      <section>
        <div className="mb-2 flex items-center gap-2">
          <h2 className="text-[11.5px] font-semibold tracking-wide text-muted-foreground uppercase">Other hosts</h2>
          <span className="text-[11px] text-muted-foreground">live metrics while you look</span>
        </div>
        <div className="divide-y overflow-hidden rounded-xl border">
          {others.map((h) => (
            <div key={h.id} className="flex items-center gap-3 px-3 py-2">
              <HostAvatar label={h.label} color={h.color} className="size-7 text-[10px]" />
              <div className="min-w-0 flex-1">
                <div className="truncate text-[13px] font-medium">{h.label}</div>
                <div className="truncate font-mono text-[11px] text-muted-foreground">
                  {h.username ? `${h.username}@` : ''}
                  {h.hostname}
                </div>
              </div>
              <Button size="xs" variant="outline" onClick={() => onOpen(h.id)}>
                <ActivityIcon /> Live metrics
              </Button>
            </div>
          ))}
          {others.length === 0 && <div className="px-3 py-3 text-xs text-muted-foreground">All hosts are monitored.</div>}
        </div>
      </section>
      <ChooseServersDialog open={choosing} onOpenChange={setChoosing} />
    </div>
  )
}

function ServerCard({ host, onOpen }: { host: Host; onOpen: () => void }) {
  const m = useMonitor((s) => s.latest[host.id])
  const st = useMonitor((s) => s.status[host.id])
  const alerts = useMonitor((s) => Object.values(s.openAlerts).filter((a) => a.subjectId === host.id).length)
  const stopped = m?.docker.available ? m.docker.containers.filter((c) => c.state !== 'running').length : 0
  return (
    <button onClick={onOpen} className="flex flex-col gap-2.5 rounded-xl border bg-card p-3 text-left transition-colors hover:border-primary/40">
      <div className="flex items-center gap-2.5">
        <HostAvatar label={host.label} color={host.color} className="size-8 text-[11px]" />
        <div className="min-w-0 flex-1">
          <div className="truncate text-[13.5px] font-medium">{host.label}</div>
          <div className="truncate text-[11px] text-muted-foreground">{m ? m.hostname : host.hostname}</div>
        </div>
        {alerts > 0 && (
          <Badge variant="danger" className="gap-1 text-[10px]">
            <BellRingIcon className="size-3" /> {alerts}
          </Badge>
        )}
      </div>
      {st?.state === 'error' || !m ? (
        <StatusLine status={st} metrics={m} />
      ) : (
        <>
          {(['CPU', 'Memory', 'Disk'] as const).map((k) => {
            const v = k === 'CPU' ? m.cpuPct : k === 'Memory' ? m.memPct : m.diskPct
            return (
              <div key={k} className="flex items-center gap-2 text-[11.5px]">
                <span className="w-14 text-muted-foreground">{k}</span>
                <Meter value={v} className="flex-1" />
                <span className="w-9 text-right tabular-nums">{pct(v)}</span>
              </div>
            )
          })}
          <div className="flex items-center gap-3 text-[11px] text-muted-foreground tabular-nums">
            <span className="flex items-center gap-0.5">
              <ArrowDownIcon className="size-3" /> {formatRate(m.rxRate)}
            </span>
            <span className="flex items-center gap-0.5">
              <ArrowUpIcon className="size-3" /> {formatRate(m.txRate)}
            </span>
            {m.docker.available && (
              <span className={cn('ml-auto flex items-center gap-1', stopped > 0 && 'text-danger')}>
                <BoxIcon className="size-3" /> {m.docker.containers.length - stopped}/{m.docker.containers.length}
              </span>
            )}
            {m.services.available && (
              <span className={cn('flex items-center gap-1', !m.docker.available && 'ml-auto', m.services.failed > 0 && 'text-danger')}>
                <WrenchIcon className="size-3" /> {m.services.failed} failed
              </span>
            )}
          </div>
        </>
      )}
    </button>
  )
}

function ChooseServersDialog({ open, onOpenChange }: { open: boolean; onOpenChange: (o: boolean) => void }) {
  const hosts = useVault((s) => s.hosts)
  const current = useSettings((s) => s.settings?.monitorHostIds)
  const [sel, setSel] = useState<string[]>([])
  useEffect(() => {
    if (open) setSel(current ?? [])
  }, [open, current])
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Monitored servers</DialogTitle>
          <DialogDescription>
            These servers are checked in the background while Remvyn is open, building history and triggering alerts. Remvyn reuses an open terminal’s connection, or connects with the host’s saved credentials.
          </DialogDescription>
        </DialogHeader>
        <div className="max-h-72 overflow-y-auto rounded-md border p-1">
          {hosts.map((h) => (
            <label key={h.id} className="flex cursor-default items-center gap-2.5 rounded px-2 py-1.5 text-[13px] hover:bg-accent/50">
              <Checkbox checked={sel.includes(h.id)} onCheckedChange={(v) => setSel(v ? [...sel, h.id] : sel.filter((x) => x !== h.id))} />
              {h.label}
              {h.authMethod === 'password' && !h.hasPassword && <span className="text-[11px] text-warning">password not saved</span>}
              <span className="ml-auto font-mono text-[11px] text-muted-foreground">{h.hostname}</span>
            </label>
          ))}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button
            onClick={async () => {
              await saveMonitored(sel)
              onOpenChange(false)
            }}
          >
            Save
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

// ---------------------------------------------------------------- host detail

const RANGES = [
  { label: '1h', minutes: 60 },
  { label: '6h', minutes: 360 },
  { label: '24h', minutes: 1440 },
  { label: '7d', minutes: 10080 },
  { label: '30d', minutes: 43200 },
]

function HostDetail({ host, onBack }: { host: Host; onBack: () => void }) {
  const open = useSessions((s) => s.open)
  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className="flex h-14 shrink-0 items-center gap-3 border-b px-5">
        <Button variant="ghost" size="icon-sm" onClick={onBack} aria-label="Back">
          <ArrowLeftIcon />
        </Button>
        <HostAvatar label={host.label} color={host.color} className="size-7 text-[10px]" />
        <h1 className="text-[15px] font-semibold">{host.label}</h1>
        <Button size="sm" variant="outline" className="ml-auto" onClick={() => open(host.id, 'terminal')}>
          Open terminal
        </Button>
      </div>
      <div className="min-h-0 flex-1 overflow-auto p-5">
        <MonitorPanel hostId={host.id} variant="full" />
        <div className="mt-5">
          <HistorySection hostId={host.id} />
        </div>
      </div>
    </div>
  )
}

function HistorySection({ hostId }: { hostId: string }) {
  const [range, setRange] = useState(60)
  const [points, setPoints] = useState<MetricPoint[]>([])
  const load = useCallback(async () => {
    try {
      setPoints(await api.metricsHistory(hostId, range))
    } catch {
      setPoints([])
    }
  }, [hostId, range])
  useEffect(() => {
    void load()
    const t = setInterval(() => void load(), 60_000)
    return () => clearInterval(t)
  }, [load])

  const times = points.map((p) => p.at)
  const bucket = points.length > 1 ? points[1].at - points[0].at : 60_000
  const gap = Math.max(bucket * 3, 5 * 60_000)

  return (
    <section>
      <div className="mb-3 flex items-center gap-2">
        <h2 className="text-[11.5px] font-semibold tracking-wide text-muted-foreground uppercase">History</h2>
        <div className="ml-auto flex rounded-md border p-0.5">
          {RANGES.map((r) => (
            <button
              key={r.minutes}
              onClick={() => setRange(r.minutes)}
              className={cn('rounded px-2.5 py-0.5 text-xs', range === r.minutes ? 'bg-accent text-foreground' : 'text-muted-foreground hover:text-foreground')}
            >
              {r.label}
            </button>
          ))}
        </div>
      </div>
      <div className="grid grid-cols-1 gap-3 xl:grid-cols-2">
        <ChartCard title="CPU">
          <HistoryChart
            times={times}
            max={100}
            format={(v) => pct(v)}
            gapMs={gap}
            series={[
              { label: 'Average', color: 'var(--primary)', values: points.map((p) => p.cpu) },
              { label: 'Peak', color: 'var(--warning)', values: points.map((p) => p.cpuMax) },
            ]}
          />
        </ChartCard>
        <ChartCard title="Memory">
          <HistoryChart
            times={times}
            max={100}
            format={(v) => pct(v)}
            gapMs={gap}
            series={[
              { label: 'Memory', color: 'var(--success)', values: points.map((p) => p.mem) },
              { label: 'Swap', color: 'var(--warning)', values: points.map((p) => p.swap) },
            ]}
          />
        </ChartCard>
        <ChartCard title="Network">
          <HistoryChart
            times={times}
            format={formatRate}
            gapMs={gap}
            series={[
              { label: 'Received', color: 'var(--info)', values: points.map((p) => p.rx) },
              { label: 'Sent', color: '#a78bfa', values: points.map((p) => p.tx) },
            ]}
          />
        </ChartCard>
        <ChartCard title="Disk (fullest)">
          <HistoryChart times={times} max={100} format={(v) => pct(v)} gapMs={gap} series={[{ label: 'Used', color: 'var(--danger)', values: points.map((p) => p.disk) }]} />
        </ChartCard>
      </div>
      <p className="mt-2 text-[11px] text-muted-foreground">History is recorded while Remvyn is open and this server is monitored or being viewed. Gaps mean Remvyn was closed.</p>
    </section>
  )
}

function ChartCard({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div className="rounded-lg border bg-card/60 p-3">
      <div className="mb-1 text-[11px] font-semibold tracking-wide text-muted-foreground uppercase">{title}</div>
      {children}
    </div>
  )
}

// ---------------------------------------------------------------- alerts

const METRICS: { v: AlertMetric; label: string; unit: string; threshold?: number; help: string }[] = [
  { v: 'cpu', label: 'CPU usage', unit: '%', threshold: 90, help: 'Average CPU across all cores' },
  { v: 'memory', label: 'Memory usage', unit: '%', threshold: 90, help: 'RAM in use, excluding cache' },
  { v: 'disk', label: 'Disk usage', unit: '%', threshold: 85, help: 'The fullest mounted disk' },
  { v: 'swap', label: 'Swap usage', unit: '%', threshold: 50, help: 'Swap space in use' },
  { v: 'load', label: 'Load average', unit: '', threshold: 4, help: '1-minute load average' },
  { v: 'services_failed', label: 'Failed services', unit: '', threshold: 0, help: 'systemd units in the failed state' },
  { v: 'containers_stopped', label: 'Stopped containers', unit: '', threshold: 0, help: 'Docker containers that are not running' },
  { v: 'host_unreachable', label: 'Server unreachable', unit: '', help: 'Remvyn can’t connect to or read from the server' },
  { v: 'uptime_down', label: 'Uptime check down', unit: '', help: 'A URL or port check is failing' },
]

const metricInfo = (m: AlertMetric) => METRICS.find((x) => x.v === m)!

const DURATIONS = [
  { v: 0, label: 'immediately' },
  { v: 60, label: 'for 1 minute' },
  { v: 300, label: 'for 5 minutes' },
  { v: 900, label: 'for 15 minutes' },
  { v: 3600, label: 'for 1 hour' },
]

function describeRule(r: AlertRule, hosts: Host[], checks: UptimeSummary[]) {
  const info = metricInfo(r.metric)
  const scope =
    r.metric === 'uptime_down'
      ? r.checkId
        ? (checks.find((c) => c.check.id === r.checkId)?.check.name ?? 'a deleted check')
        : 'any uptime check'
      : r.hostId
        ? (hosts.find((h) => h.id === r.hostId)?.label ?? 'a deleted host')
        : 'any monitored server'
  const dur = DURATIONS.find((d) => d.v === r.durationSec)?.label ?? `for ${r.durationSec}s`
  if (r.metric === 'host_unreachable' || r.metric === 'uptime_down') return `${info.label} · ${scope} · ${dur}`
  return `${info.label} ${r.op === '<' ? 'below' : 'above'} ${r.threshold}${info.unit} · ${scope} · ${dur}`
}

function AlertsTab() {
  const hosts = useVault((s) => s.hosts)
  const [rules, setRules] = useState<AlertRule[]>([])
  const [eventsList, setEvents] = useState<AlertEvent[]>([])
  const [checks, setChecks] = useState<UptimeSummary[]>([])
  const [editing, setEditing] = useState<AlertRule | null>(null)
  const [deleting, setDeleting] = useState<AlertRule | null>(null)
  const monitoredCount = useSettings((s) => s.settings?.monitorHostIds.length ?? 0)

  const load = useCallback(async () => {
    const [r, e, c] = await Promise.all([api.alertRules(), api.alertEvents(), api.uptime()])
    setRules(r)
    setEvents(e)
    setChecks(c)
  }, [])
  useEffect(() => {
    void load()
    return onEvent(events.ALERT, () => void load())
  }, [load])

  const toggle = async (r: AlertRule, enabled: boolean) => {
    try {
      await api.saveAlertRule({ ...r, enabled })
      await load()
    } catch (e) {
      toast.error(errorMessage(e))
    }
  }

  return (
    <div className="flex flex-col gap-6 p-5">
      <section>
        <div className="mb-3 flex items-center gap-2">
          <h2 className="text-[11.5px] font-semibold tracking-wide text-muted-foreground uppercase">Rules</h2>
          <Button size="sm" variant="ghost" className="ml-auto" onClick={() => void api.sendTestNotification()}>
            <BellIcon /> Test notification
          </Button>
          <Button size="sm" onClick={() => setEditing(newRule())}>
            <PlusIcon /> New rule
          </Button>
        </div>
        {monitoredCount === 0 && (
          <div className="mb-3 rounded-md bg-warning/10 px-3 py-2 text-xs text-warning">
            Server rules only run for servers being monitored. Choose servers on the Dashboard to watch them in the background.
          </div>
        )}
        {rules.length === 0 ? (
          <div className="flex flex-col items-center gap-2 rounded-xl border border-dashed py-8 text-[13px] text-muted-foreground">
            No alert rules yet.
            <div className="flex flex-wrap justify-center gap-2">
              {(['cpu', 'disk', 'services_failed', 'host_unreachable'] as AlertMetric[]).map((m) => (
                <Button key={m} size="xs" variant="outline" onClick={() => setEditing(newRule(m))}>
                  <ZapIcon /> {metricInfo(m).label}
                </Button>
              ))}
            </div>
          </div>
        ) : (
          <div className="divide-y overflow-hidden rounded-xl border">
            {rules.map((r) => (
              <div key={r.id} className="flex items-center gap-3 px-3 py-2.5">
                <Switch checked={r.enabled} onCheckedChange={(v) => void toggle(r, v)} aria-label="Enabled" />
                <div className={cn('min-w-0 flex-1', !r.enabled && 'opacity-50')}>
                  <div className="text-[13px] font-medium">{r.name || metricInfo(r.metric).label}</div>
                  <div className="text-[11.5px] text-muted-foreground">{describeRule(r, hosts, checks)}</div>
                </div>
                <Button variant="ghost" size="icon-xs" onClick={() => setEditing(r)} aria-label="Edit rule">
                  <PencilIcon />
                </Button>
                <Button variant="ghost" size="icon-xs" onClick={() => setDeleting(r)} aria-label="Delete rule">
                  <Trash2Icon />
                </Button>
              </div>
            ))}
          </div>
        )}
      </section>

      <section>
        <h2 className="mb-3 text-[11.5px] font-semibold tracking-wide text-muted-foreground uppercase">Recent alerts</h2>
        {eventsList.length === 0 ? (
          <div className="rounded-xl border border-dashed py-6 text-center text-[13px] text-muted-foreground">Nothing has fired yet.</div>
        ) : (
          <div className="divide-y overflow-hidden rounded-xl border">
            {eventsList.map((e) => (
              <div key={e.id} className="flex items-start gap-3 px-3 py-2.5 text-[12.5px]">
                <span className={cn('mt-1.5 size-2 shrink-0 rounded-full', e.resolvedAt ? 'bg-muted-foreground/40' : 'animate-pulse bg-danger')} />
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-2">
                    <span className="font-medium">{e.ruleName}</span>
                    <span className="text-muted-foreground">· {e.subject}</span>
                    {!e.resolvedAt && (
                      <Badge variant="danger" className="text-[10px]">
                        Firing
                      </Badge>
                    )}
                  </div>
                  <div className="text-muted-foreground">{e.message}</div>
                </div>
                <div className="shrink-0 text-right text-[11px] text-muted-foreground">
                  <div title={new Date(e.startedAt).toLocaleString()}>{relativeTime(e.startedAt)}</div>
                  {e.resolvedAt > 0 && <div>lasted {formatDuration(e.resolvedAt - e.startedAt)}</div>}
                </div>
              </div>
            ))}
          </div>
        )}
      </section>

      <RuleDialog rule={editing} checks={checks} onOpenChange={(o) => !o && setEditing(null)} onSaved={load} />
      <ConfirmDialog
        open={!!deleting}
        onOpenChange={(o) => !o && setDeleting(null)}
        title="Delete this alert rule?"
        description={deleting ? describeRule(deleting, hosts, checks) : ''}
        confirmLabel="Delete"
        destructive
        onConfirm={async () => {
          if (!deleting) return
          await api.deleteAlertRule(deleting.id)
          await load()
        }}
      />
    </div>
  )
}

function formatDuration(ms: number) {
  const s = Math.round(ms / 1000)
  if (s < 60) return `${s}s`
  const m = Math.round(s / 60)
  if (m < 60) return `${m}m`
  const h = Math.floor(m / 60)
  return h < 24 ? `${h}h ${m % 60}m` : `${Math.floor(h / 24)}d ${h % 24}h`
}

function newRule(metric: AlertMetric = 'cpu'): AlertRule {
  const info = metricInfo(metric)
  return {
    id: '',
    name: '',
    metric,
    hostId: '',
    checkId: '',
    op: '>',
    threshold: info.threshold ?? 0,
    durationSec: metric === 'cpu' || metric === 'memory' || metric === 'load' ? 300 : metric === 'host_unreachable' ? 60 : 0,
    enabled: true,
    createdAt: 0,
  }
}

const ANY = '__any__'

function RuleDialog({ rule, checks, onOpenChange, onSaved }: { rule: AlertRule | null; checks: UptimeSummary[]; onOpenChange: (o: boolean) => void; onSaved: () => Promise<void> }) {
  const hosts = useVault((s) => s.hosts)
  const [r, setR] = useState<AlertRule>(newRule())
  const [busy, setBusy] = useState(false)
  useEffect(() => {
    if (rule) setR(rule)
  }, [rule])
  const set = (p: Partial<AlertRule>) => setR((x) => ({ ...x, ...p }))
  const info = metricInfo(r.metric)
  const hasThreshold = r.metric !== 'host_unreachable' && r.metric !== 'uptime_down'

  const save = async () => {
    setBusy(true)
    try {
      await api.saveAlertRule(r)
      await onSaved()
      toast.success(rule?.id ? 'Rule updated' : 'Rule created')
      onOpenChange(false)
    } catch (e) {
      toast.error('Could not save rule', { description: errorMessage(e) })
    } finally {
      setBusy(false)
    }
  }

  return (
    <Dialog open={!!rule} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{rule?.id ? 'Edit alert rule' : 'New alert rule'}</DialogTitle>
          <DialogDescription>Remvyn notifies you when the condition holds for the chosen time, and again when it clears.</DialogDescription>
        </DialogHeader>
        <div className="flex flex-col gap-3">
          <div className="flex flex-col gap-1.5">
            <Label>Alert when</Label>
            <Select
              value={r.metric}
              onValueChange={(v) => {
                const m = v as AlertMetric
                const d = newRule(m)
                set({ metric: m, threshold: d.threshold, durationSec: d.durationSec, op: '>' })
              }}
            >
              <SelectTrigger className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {METRICS.map((m) => (
                  <SelectItem key={m.v} value={m.v}>
                    {m.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <p className="text-[11.5px] text-muted-foreground">{info.help}</p>
          </div>
          {hasThreshold && (
            <div className="flex items-end gap-2">
              <div className="flex flex-col gap-1.5">
                <Label>Is</Label>
                <Select value={r.op} onValueChange={(v) => set({ op: v as '>' | '<' })}>
                  <SelectTrigger className="w-32">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value=">">{info.unit === '%' || r.metric === 'load' ? 'above' : 'more than'}</SelectItem>
                    <SelectItem value="<">{info.unit === '%' || r.metric === 'load' ? 'below' : 'fewer than'}</SelectItem>
                  </SelectContent>
                </Select>
              </div>
              <div className="flex flex-1 items-center gap-2">
                <Input type="number" min={0} step={r.metric === 'load' ? 0.5 : 1} value={r.threshold} onChange={(e) => set({ threshold: Number(e.target.value) })} />
                <span className="w-4 text-xs text-muted-foreground">{info.unit}</span>
              </div>
            </div>
          )}
          <div className="flex flex-col gap-1.5">
            <Label>{r.metric === 'uptime_down' ? 'Check' : 'Server'}</Label>
            {r.metric === 'uptime_down' ? (
              <Select value={r.checkId || ANY} onValueChange={(v) => set({ checkId: v === ANY ? '' : v })}>
                <SelectTrigger className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value={ANY}>Any uptime check</SelectItem>
                  {checks.map((c) => (
                    <SelectItem key={c.check.id} value={c.check.id}>
                      {c.check.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            ) : (
              <Select value={r.hostId || ANY} onValueChange={(v) => set({ hostId: v === ANY ? '' : v })}>
                <SelectTrigger className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value={ANY}>Any monitored server</SelectItem>
                  {hosts.map((h) => (
                    <SelectItem key={h.id} value={h.id}>
                      {h.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            )}
          </div>
          <div className="flex flex-col gap-1.5">
            <Label>For</Label>
            <Select value={String(r.durationSec)} onValueChange={(v) => set({ durationSec: Number(v) })}>
              <SelectTrigger className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {DURATIONS.map((d) => (
                  <SelectItem key={d.v} value={String(d.v)}>
                    {d.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="flex flex-col gap-1.5">
            <Label>Name (optional)</Label>
            <Input value={r.name} placeholder={info.label} onChange={(e) => set({ name: e.target.value })} />
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button disabled={busy} onClick={save}>
            {rule?.id ? 'Save' : 'Create rule'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

// ---------------------------------------------------------------- uptime

function UptimeTab() {
  const [list, setList] = useState<UptimeSummary[]>([])
  const [editing, setEditing] = useState<UptimeCheck | null>(null)
  const [deleting, setDeleting] = useState<UptimeCheck | null>(null)
  const load = useCallback(async () => setList(await api.uptime()), [])
  useEffect(() => {
    void load()
    const off = onEvent<UptimeResult>(events.UPTIME_RESULT, () => void load())
    const t = setInterval(() => void load(), 30_000)
    return () => {
      off()
      clearInterval(t)
    }
  }, [load])

  return (
    <div className="p-5">
      <div className="mb-3 flex items-center gap-2">
        <h2 className="text-[11.5px] font-semibold tracking-wide text-muted-foreground uppercase">Checks</h2>
        <span className="text-[11px] text-muted-foreground">probed from this computer while Remvyn is open</span>
        <Button size="sm" className="ml-auto" onClick={() => setEditing(newCheck())}>
          <PlusIcon /> New check
        </Button>
      </div>
      {list.length === 0 ? (
        <EmptyState icon={GlobeIcon} title="No uptime checks" className="h-auto py-16">
          Add a website, API health endpoint or TCP port to check on a schedule.
        </EmptyState>
      ) : (
        <div className="flex flex-col gap-2.5">
          {list.map((s) => (
            <UptimeCard key={s.check.id} s={s} onEdit={() => setEditing(s.check)} onDelete={() => setDeleting(s.check)} onToggle={async (enabled) => {
              try {
                await api.saveUptimeCheck({ ...s.check, enabled })
                await load()
              } catch (e) {
                toast.error(errorMessage(e))
              }
            }} />
          ))}
        </div>
      )}
      <CheckDialog check={editing} onOpenChange={(o) => !o && setEditing(null)} onSaved={load} />
      <ConfirmDialog
        open={!!deleting}
        onOpenChange={(o) => !o && setDeleting(null)}
        title={`Delete “${deleting?.name}”?`}
        description="Its history and any alert rules for it are deleted too."
        confirmLabel="Delete"
        destructive
        onConfirm={async () => {
          if (!deleting) return
          await api.deleteUptimeCheck(deleting.id)
          await load()
        }}
      />
    </div>
  )
}

function ratio(v: number) {
  return v < 0 ? '—' : `${v >= 99.995 ? '100' : v.toFixed(2)}%`
}

function UptimeCard({ s, onEdit, onDelete, onToggle }: { s: UptimeSummary; onEdit: () => void; onDelete: () => void; onToggle: (enabled: boolean) => void }) {
  const c = s.check
  const state = !c.enabled ? 'paused' : s.up === null ? 'pending' : s.up ? 'up' : 'down'
  return (
    <div className="rounded-xl border bg-card p-3">
      <div className="flex items-center gap-3">
        <span
          className={cn('size-2.5 shrink-0 rounded-full', {
            'bg-success': state === 'up',
            'animate-pulse bg-danger': state === 'down',
            'bg-muted-foreground/40': state === 'paused' || state === 'pending',
          })}
        />
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2 text-[13.5px] font-medium">
            {c.name}
            <Badge variant="outline" className="text-[10px] uppercase">
              {c.kind}
            </Badge>
          </div>
          <div className="truncate font-mono text-[11.5px] text-muted-foreground">{c.target}</div>
        </div>
        <div className="hidden text-right text-[11px] text-muted-foreground sm:block">
          {state === 'down' ? (
            <div className="font-medium text-danger">Down {s.downSince ? `for ${formatDuration(Date.now() - s.downSince)}` : ''}</div>
          ) : state === 'up' ? (
            <div className="font-medium text-success">Up · {s.latencyMs} ms</div>
          ) : (
            <div>{state === 'paused' ? 'Paused' : 'Waiting for first check'}</div>
          )}
          <div>every {c.intervalSec >= 60 ? `${c.intervalSec / 60}m` : `${c.intervalSec}s`}{s.lastAt ? ` · checked ${relativeTime(s.lastAt)}` : ''}</div>
        </div>
        <Switch checked={c.enabled} onCheckedChange={onToggle} aria-label="Enabled" />
        <Button variant="ghost" size="icon-xs" onClick={onEdit} aria-label="Edit check">
          <PencilIcon />
        </Button>
        <Button variant="ghost" size="icon-xs" onClick={onDelete} aria-label="Delete check">
          <Trash2Icon />
        </Button>
      </div>
      {state === 'down' && s.lastError && <div className="mt-2 rounded-md bg-danger/10 px-2.5 py-1.5 text-xs text-danger">{s.lastError}</div>}
      <div className="mt-3 flex items-end gap-4">
        <div className="flex h-6 flex-1 items-end gap-px" aria-label="Recent checks">
          {Array.from({ length: Math.max(0, 60 - s.recent.length) }).map((_, i) => (
            <span key={`e${i}`} className="h-full flex-1 rounded-sm bg-muted" />
          ))}
          {s.recent.map((r) => (
            <span
              key={r.at}
              title={`${new Date(r.at).toLocaleString()} · ${r.ok ? `${r.latencyMs} ms` : r.error}`}
              className={cn('h-full flex-1 rounded-sm', r.ok ? 'bg-success/70' : 'bg-danger')}
            />
          ))}
        </div>
        <div className="flex gap-4 text-right text-[11px]">
          {(
            [
              ['24h', s.uptime24h],
              ['7d', s.uptime7d],
              ['30d', s.uptime30d],
            ] as const
          ).map(([l, v]) => (
            <div key={l}>
              <div className={cn('font-semibold tabular-nums', v >= 0 && v < 99 ? 'text-warning' : '')}>{ratio(v)}</div>
              <div className="text-muted-foreground">{l}</div>
            </div>
          ))}
        </div>
      </div>
    </div>
  )
}

function newCheck(): UptimeCheck {
  return { id: '', name: '', kind: 'http', target: '', intervalSec: 60, timeoutSec: 10, enabled: true, createdAt: 0 }
}

function CheckDialog({ check, onOpenChange, onSaved }: { check: UptimeCheck | null; onOpenChange: (o: boolean) => void; onSaved: () => Promise<void> }) {
  const [c, setC] = useState<UptimeCheck>(newCheck())
  const [busy, setBusy] = useState(false)
  const [test, setTest] = useState<UptimeResult | null>(null)
  useEffect(() => {
    if (check) {
      setC(check)
      setTest(null)
    }
  }, [check])
  const set = (p: Partial<UptimeCheck>) => setC((x) => ({ ...x, ...p }))

  const runTest = async () => {
    setBusy(true)
    try {
      setTest(await api.testUptimeCheck(c))
    } catch (e) {
      toast.error(errorMessage(e))
    } finally {
      setBusy(false)
    }
  }
  const save = async () => {
    setBusy(true)
    try {
      await api.saveUptimeCheck(c)
      await onSaved()
      toast.success(check?.id ? 'Check updated' : 'Check added')
      onOpenChange(false)
    } catch (e) {
      toast.error('Could not save check', { description: errorMessage(e) })
    } finally {
      setBusy(false)
    }
  }

  return (
    <Dialog open={!!check} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{check?.id ? 'Edit uptime check' : 'New uptime check'}</DialogTitle>
          <DialogDescription>HTTP checks pass when the response status is below 400. TCP checks pass when the port accepts a connection.</DialogDescription>
        </DialogHeader>
        <div className="flex flex-col gap-3">
          <div className="grid grid-cols-2 gap-2">
            {(
              [
                ['http', 'Website / API'],
                ['tcp', 'TCP port'],
              ] as const
            ).map(([k, label]) => (
              <button
                key={k}
                onClick={() => set({ kind: k })}
                className={cn('rounded-md border py-2 text-[13px]', c.kind === k ? 'border-primary bg-primary/10' : 'text-muted-foreground hover:bg-accent')}
              >
                {label}
              </button>
            ))}
          </div>
          <div className="flex flex-col gap-1.5">
            <Label>{c.kind === 'http' ? 'URL' : 'Host and port'}</Label>
            <Input autoFocus className="font-mono" placeholder={c.kind === 'http' ? 'https://example.com/health' : 'db.example.com:5432'} value={c.target} onChange={(e) => set({ target: e.target.value })} />
          </div>
          <div className="flex flex-col gap-1.5">
            <Label>Name</Label>
            <Input placeholder={c.target || 'Production API'} value={c.name} onChange={(e) => set({ name: e.target.value })} />
          </div>
          <div className="grid grid-cols-2 gap-3">
            <div className="flex flex-col gap-1.5">
              <Label>Check every</Label>
              <Select value={String(c.intervalSec)} onValueChange={(v) => set({ intervalSec: Number(v) })}>
                <SelectTrigger className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {[30, 60, 300, 900].map((v) => (
                    <SelectItem key={v} value={String(v)}>
                      {v < 60 ? `${v} seconds` : `${v / 60} minute${v === 60 ? '' : 's'}`}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="flex flex-col gap-1.5">
              <Label>Timeout</Label>
              <Select value={String(c.timeoutSec)} onValueChange={(v) => set({ timeoutSec: Number(v) })}>
                <SelectTrigger className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {[5, 10, 20, 30].map((v) => (
                    <SelectItem key={v} value={String(v)}>
                      {v} seconds
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          </div>
          {test && (
            <div className={cn('rounded-md px-3 py-2 text-xs', test.ok ? 'bg-success/10 text-success' : 'bg-danger/10 text-danger')}>
              {test.ok ? `Up · ${test.statusCode ? `HTTP ${test.statusCode} · ` : ''}${test.latencyMs} ms` : `Down · ${test.error}`}
            </div>
          )}
        </div>
        <DialogFooter>
          <Button variant="ghost" disabled={busy || !c.target.trim()} onClick={runTest} className="mr-auto">
            Test now
          </Button>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button disabled={busy || !c.target.trim()} onClick={save}>
            {check?.id ? 'Save' : 'Add check'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
