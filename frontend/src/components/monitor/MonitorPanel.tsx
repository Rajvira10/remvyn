import {
  AlertTriangleIcon,
  ArrowDownIcon,
  ArrowUpIcon,
  BoxIcon,
  CpuIcon,
  HardDriveIcon,
  ListIcon,
  MemoryStickIcon,
  MoreHorizontalIcon,
  NetworkIcon,
  PlayIcon,
  RotateCwIcon,
  SquareIcon,
  WrenchIcon,
} from 'lucide-react'
import { useState, type ReactNode } from 'react'
import { toast } from 'sonner'
import { Spinner } from '@/components/common/Spinner'
import { Button } from '@/components/ui/button'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { Input } from '@/components/ui/input'
import { api, errorMessage } from '@/lib/api'
import type { Container, Metrics, MonitorStatus } from '@/lib/types'
import { cn, formatBytes } from '@/lib/utils'
import { type LivePoint, useLiveMetrics } from '@/stores/monitor'
import { Meter, Sparkline, formatRate, formatUptime, levelColor, pct } from './charts'

function Section({ icon: Icon, title, right, children }: { icon: typeof CpuIcon; title: string; right?: ReactNode; children: ReactNode }) {
  return (
    <section className="rounded-lg border bg-card/60 p-3">
      <div className="mb-2 flex items-center gap-1.5 text-[11px] font-semibold tracking-wide text-muted-foreground uppercase">
        <Icon className="size-3.5" /> {title}
        <span className="ml-auto font-normal tracking-normal normal-case">{right}</span>
      </div>
      {children}
    </section>
  )
}

export function StatusLine({ status, metrics }: { status?: MonitorStatus; metrics?: Metrics }) {
  if (!status || status.state === 'connecting' || (status.state === 'live' && !metrics)) {
    return (
      <div className="flex items-center gap-2 text-xs text-muted-foreground">
        <Spinner className="size-3.5" /> Collecting metrics…
      </div>
    )
  }
  if (status.state === 'error') {
    return (
      <div className="flex items-start gap-2 rounded-md bg-danger/10 px-2.5 py-2 text-xs text-danger">
        <AlertTriangleIcon className="mt-0.5 size-3.5 shrink-0" />
        <span className="break-words">{status.error}</span>
      </div>
    )
  }
  return (
    <div className="flex items-center gap-1.5 text-[11px] text-muted-foreground">
      <span className="relative flex size-2">
        <span className="absolute inline-flex size-full animate-ping rounded-full bg-success opacity-50" />
        <span className="relative inline-flex size-2 rounded-full bg-success" />
      </span>
      Live{status.source === 'session' ? ' · via this terminal’s connection' : status.source === 'direct' ? ' · direct connection' : ''}
    </div>
  )
}

function ContainerRow({ hostId, c }: { hostId: string; c: Container }) {
  const [busy, setBusy] = useState(false)
  const running = c.state === 'running'
  const act = async (action: 'start' | 'stop' | 'restart') => {
    setBusy(true)
    try {
      await api.containerAction(hostId, c.name || c.id, action)
      toast.success(`${c.name}: ${action === 'stop' ? 'stopped' : action === 'start' ? 'started' : 'restarted'}`)
    } catch (e) {
      toast.error(`Could not ${action} ${c.name}`, { description: errorMessage(e) })
    } finally {
      setBusy(false)
    }
  }
  return (
    <div className="group flex items-center gap-2 py-1.5 text-xs">
      <span className={cn('size-1.5 shrink-0 rounded-full', running ? 'bg-success' : c.state === 'exited' || c.state === 'dead' ? 'bg-danger' : 'bg-warning')} />
      <div className="min-w-0 flex-1">
        <div className="truncate font-medium" title={c.name}>
          {c.name}
        </div>
        <div className="truncate text-[10.5px] text-muted-foreground" title={`${c.image} · ${c.status}`}>
          {c.image} · {c.status}
        </div>
      </div>
      {running && (
        <div className="text-right text-[10.5px] text-muted-foreground tabular-nums">
          <div>{pct(c.cpuPct, 1)} CPU</div>
          <div>{c.memUsage ? c.memUsage.split('/')[0].trim() : pct(c.memPct, 1)}</div>
        </div>
      )}
      {busy ? (
        <Spinner className="size-3.5" />
      ) : (
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="ghost" size="icon-xs" className="opacity-60 group-hover:opacity-100" aria-label={`Actions for ${c.name}`}>
              <MoreHorizontalIcon />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            {running ? (
              <>
                <DropdownMenuItem onSelect={() => void act('restart')}>
                  <RotateCwIcon /> Restart
                </DropdownMenuItem>
                <DropdownMenuItem onSelect={() => void act('stop')}>
                  <SquareIcon /> Stop
                </DropdownMenuItem>
              </>
            ) : (
              <DropdownMenuItem onSelect={() => void act('start')}>
                <PlayIcon /> Start
              </DropdownMenuItem>
            )}
          </DropdownMenuContent>
        </DropdownMenu>
      )}
    </div>
  )
}

function Services({ m }: { m: Metrics }) {
  const [showAll, setShowAll] = useState(false)
  const [q, setQ] = useState('')
  const s = m.services
  if (!s.available) return <div className="text-xs text-muted-foreground">{s.error || 'Waiting for service data…'}</div>
  const failed = s.items.filter((x) => x.active === 'failed')
  const list = showAll ? s.items.filter((x) => !q || x.name.toLowerCase().includes(q.toLowerCase())) : failed
  return (
    <div className="flex flex-col gap-1.5">
      {failed.length === 0 && !showAll && <div className="text-xs text-success">All services healthy</div>}
      {showAll && <Input value={q} onChange={(e) => setQ(e.target.value)} placeholder="Filter services" className="h-7 text-xs" />}
      <div className={cn('flex flex-col', showAll && 'max-h-64 overflow-y-auto')}>
        {list.map((x) => (
          <div key={x.name} className="flex items-center gap-2 py-1 text-xs" title={x.description}>
            <span className={cn('size-1.5 shrink-0 rounded-full', x.active === 'failed' ? 'bg-danger' : x.sub === 'running' ? 'bg-success' : 'bg-muted-foreground/40')} />
            <span className={cn('min-w-0 flex-1 truncate', x.active === 'failed' && 'font-medium text-danger')}>{x.name}</span>
            <span className="text-[10.5px] text-muted-foreground">{x.active === 'failed' ? 'failed' : x.sub}</span>
          </div>
        ))}
      </div>
      <button className="self-start text-[11px] text-primary hover:underline" onClick={() => setShowAll((v) => !v)}>
        {showAll ? 'Show failed only' : `Show all ${s.total} services`}
      </button>
    </div>
  )
}

/** Live server metrics. "side" is the narrow panel beside a terminal; "full" is the wide monitoring view. */
export function MonitorPanel({ hostId, variant = 'side', active = true }: { hostId: string; variant?: 'side' | 'full'; active?: boolean }) {
  const { metrics: m, status, live } = useLiveMetrics(hostId, active)
  const pts: LivePoint[] = live ?? []
  const full = variant === 'full'

  return (
    <div className={cn('flex flex-col gap-2.5', full ? '' : 'p-3')}>
      <div className="flex flex-col gap-1">
        {m && (
          <div className="flex items-baseline gap-2">
            <span className="truncate text-[13px] font-semibold">{m.hostname}</span>
            <span className="truncate text-[11px] text-muted-foreground">
              up {formatUptime(m.uptimeSec)} · {m.kernel}
            </span>
          </div>
        )}
        <StatusLine status={status} metrics={m} />
      </div>
      {m && (
        <div className={cn('grid gap-2.5', full ? 'grid-cols-2 xl:grid-cols-4' : 'grid-cols-1')}>
          <Section icon={CpuIcon} title="CPU" right={`${m.cpuCount} core${m.cpuCount === 1 ? '' : 's'}`}>
            <div className="flex items-end justify-between">
              <span className="text-2xl font-semibold tabular-nums" style={{ color: levelColor(m.cpuPct) }}>
                {pct(m.cpuPct)}
              </span>
              <span className="text-[11px] text-muted-foreground tabular-nums" title="Load average (1, 5, 15 min)">
                load {m.load1.toFixed(2)} · {m.load5.toFixed(2)} · {m.load15.toFixed(2)}
              </span>
            </div>
            <Sparkline values={pts.map((p) => p.cpu)} max={100} className="mt-1.5" />
          </Section>
          <Section icon={MemoryStickIcon} title="Memory" right={`${formatBytes(m.memUsed)} / ${formatBytes(m.memTotal)}`}>
            <div className="text-2xl font-semibold tabular-nums" style={{ color: levelColor(m.memPct) }}>
              {pct(m.memPct)}
            </div>
            <Meter value={m.memPct} className="mt-2" />
            <div className="mt-2 flex justify-between text-[11px] text-muted-foreground">
              <span>Swap</span>
              <span className="tabular-nums">{m.swapTotal ? `${formatBytes(m.swapUsed)} / ${formatBytes(m.swapTotal)} (${pct(m.swapPct)})` : 'none'}</span>
            </div>
          </Section>
          <Section icon={NetworkIcon} title="Network">
            <div className="flex gap-4 text-[13px] font-medium tabular-nums">
              <span className="flex items-center gap-1 text-info">
                <ArrowDownIcon className="size-3.5" /> {formatRate(m.rxRate)}
              </span>
              <span className="flex items-center gap-1 text-violet-400">
                <ArrowUpIcon className="size-3.5" /> {formatRate(m.txRate)}
              </span>
            </div>
            <div className="relative mt-1.5">
              <Sparkline values={pts.map((p) => p.rx)} color="var(--info)" />
              <Sparkline values={pts.map((p) => p.tx)} max={Math.max(1, ...pts.map((p) => p.rx), ...pts.map((p) => p.tx))} color="#a78bfa" className="absolute inset-0" />
            </div>
          </Section>
          <Section icon={HardDriveIcon} title="Disks">
            <div className="flex flex-col gap-2">
              {m.disks.length === 0 && <div className="text-xs text-muted-foreground">No disks reported</div>}
              {m.disks.map((d) => (
                <div key={d.mount}>
                  <div className="mb-1 flex justify-between gap-2 text-[11.5px]">
                    <span className="truncate font-mono" title={`${d.fs} on ${d.mount}`}>
                      {d.mount}
                    </span>
                    <span className="shrink-0 text-muted-foreground tabular-nums">
                      {formatBytes(d.used)} / {formatBytes(d.total)} · <span style={{ color: levelColor(d.usedPct) }}>{pct(d.usedPct)}</span>
                    </span>
                  </div>
                  <Meter value={d.usedPct} />
                </div>
              ))}
            </div>
          </Section>
        </div>
      )}
      {m && (
        <div className={cn('grid gap-2.5', full ? 'grid-cols-1 lg:grid-cols-3' : 'grid-cols-1')}>
          <Section
            icon={BoxIcon}
            title="Containers"
            right={m.docker.available ? `${m.docker.containers.filter((c) => c.state === 'running').length} / ${m.docker.containers.length} running` : undefined}
          >
            {!m.docker.updatedAt ? (
              <div className="text-xs text-muted-foreground">Checking for Docker…</div>
            ) : !m.docker.available ? (
              <div className="text-xs text-muted-foreground">{m.docker.error}</div>
            ) : m.docker.containers.length === 0 ? (
              <div className="text-xs text-muted-foreground">No containers</div>
            ) : (
              <div className={cn('divide-y', full && 'max-h-80 overflow-y-auto')}>
                {m.docker.containers.map((c) => (
                  <ContainerRow key={c.id} hostId={hostId} c={c} />
                ))}
              </div>
            )}
          </Section>
          <Section
            icon={WrenchIcon}
            title="Services"
            right={m.services.available ? <span className={cn(m.services.failed > 0 && 'font-medium text-danger')}>{m.services.running} running · {m.services.failed} failed</span> : undefined}
          >
            {!m.services.updatedAt ? <div className="text-xs text-muted-foreground">Checking services…</div> : <Services m={m} />}
          </Section>
          <Section icon={ListIcon} title="Top processes">
            <div className="flex flex-col">
              {m.processes.length === 0 && <div className="text-xs text-muted-foreground">Not available</div>}
              {m.processes.slice(0, full ? 8 : 5).map((p) => (
                <div key={p.pid} className="flex items-center gap-2 py-0.5 text-xs" title={`PID ${p.pid} · ${p.user}`}>
                  <span className="min-w-0 flex-1 truncate font-mono">{p.command}</span>
                  <span className="w-12 text-right text-muted-foreground tabular-nums">{pct(p.cpuPct, 1)}</span>
                  <span className="w-12 text-right text-muted-foreground tabular-nums">{pct(p.memPct, 1)}</span>
                </div>
              ))}
            </div>
          </Section>
        </div>
      )}
    </div>
  )
}
