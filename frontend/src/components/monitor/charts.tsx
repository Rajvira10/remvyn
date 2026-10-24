import { useMemo, useRef, useState } from 'react'
import { formatBytes } from '@/lib/utils'
import { cn } from '@/lib/utils'

export function formatRate(bps: number): string {
  if (!Number.isFinite(bps) || bps < 0) return '—'
  return `${formatBytes(Math.round(bps))}/s`
}

export function formatUptime(sec: number): string {
  if (!sec) return '—'
  const d = Math.floor(sec / 86400)
  const h = Math.floor((sec % 86400) / 3600)
  const m = Math.floor((sec % 3600) / 60)
  if (d > 0) return `${d}d ${h}h`
  if (h > 0) return `${h}h ${m}m`
  return `${m}m`
}

export function pct(v: number, digits = 0): string {
  return `${(Number.isFinite(v) ? v : 0).toFixed(digits)}%`
}

/** Color for a utilisation percentage. */
export function levelColor(v: number): string {
  if (v >= 90) return 'var(--danger)'
  if (v >= 75) return 'var(--warning)'
  return 'var(--primary)'
}

/** Thin progress bar. */
export function Meter({ value, className, color }: { value: number; className?: string; color?: string }) {
  const v = Math.max(0, Math.min(100, value || 0))
  return (
    <div className={cn('h-1.5 w-full overflow-hidden rounded-full bg-muted', className)}>
      <div className="h-full rounded-full transition-[width] duration-500" style={{ width: `${v}%`, background: color ?? levelColor(v) }} />
    </div>
  )
}

/** Small area sparkline. Values are plotted against max (default: data max, or 100 for percentages). */
export function Sparkline({
  values,
  max,
  color = 'var(--primary)',
  className,
  height = 32,
}: {
  values: number[]
  max?: number
  color?: string
  className?: string
  height?: number
}) {
  const w = 100
  const h = height
  const path = useMemo(() => {
    if (values.length < 2) return null
    const top = max ?? Math.max(1, ...values)
    const step = w / (values.length - 1)
    const pts = values.map((v, i) => [i * step, h - (Math.min(v, top) / top) * (h - 2) - 1] as const)
    const line = pts.map(([x, y], i) => `${i ? 'L' : 'M'}${x.toFixed(2)},${y.toFixed(2)}`).join(' ')
    return { line, area: `${line} L${w},${h} L0,${h} Z` }
  }, [values, max, h])
  return (
    <svg viewBox={`0 0 ${w} ${h}`} preserveAspectRatio="none" className={cn('w-full', className)} style={{ height }} aria-hidden>
      {path && (
        <>
          <path d={path.area} fill={color} opacity={0.14} />
          <path d={path.line} fill="none" stroke={color} strokeWidth={1.5} vectorEffect="non-scaling-stroke" />
        </>
      )}
    </svg>
  )
}

export interface Series {
  label: string
  color: string
  values: number[]
}

/**
 * Time-series chart with a y axis, time labels and a hover readout.
 * Gaps longer than gapMs (for example while Remvyn was closed) break the line.
 */
export function HistoryChart({
  times,
  series,
  max,
  format,
  height = 140,
  gapMs,
}: {
  times: number[]
  series: Series[]
  max?: number
  format: (v: number) => string
  height?: number
  gapMs?: number
}) {
  const ref = useRef<SVGSVGElement>(null)
  const [hover, setHover] = useState<number | null>(null)
  const W = 600
  const H = height
  const padL = 44
  const padB = 18
  const plotW = W - padL - 4
  const plotH = H - padB - 6

  const top = useMemo(() => {
    if (max) return max
    const m = Math.max(0, ...series.flatMap((s) => s.values))
    if (m <= 0) return 1
    const mag = 10 ** Math.floor(Math.log10(m))
    return Math.ceil(m / mag) * mag
  }, [series, max])

  const t0 = times[0] ?? 0
  const t1 = times[times.length - 1] ?? 1
  const x = (t: number) => padL + (t1 === t0 ? plotW : ((t - t0) / (t1 - t0)) * plotW)
  const y = (v: number) => 6 + plotH - (Math.min(v, top) / top) * plotH

  const paths = useMemo(
    () =>
      series.map((s) => {
        let d = ''
        s.values.forEach((v, i) => {
          const gap = i > 0 && gapMs !== undefined && times[i] - times[i - 1] > gapMs
          d += `${i === 0 || gap ? 'M' : 'L'}${x(times[i]).toFixed(1)},${y(v).toFixed(1)} `
        })
        return d
      }),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [series, times, top, gapMs],
  )

  const fmtTime = (t: number) => {
    const span = t1 - t0
    const d = new Date(t)
    if (span > 2 * 86400_000) return d.toLocaleDateString(undefined, { month: 'short', day: 'numeric' })
    return d.toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' })
  }

  const onMove = (e: React.MouseEvent) => {
    const rect = ref.current?.getBoundingClientRect()
    if (!rect || times.length === 0) return
    const px = ((e.clientX - rect.left) / rect.width) * W
    const t = t0 + ((px - padL) / plotW) * (t1 - t0)
    let best = 0
    for (let i = 1; i < times.length; i++) if (Math.abs(times[i] - t) < Math.abs(times[best] - t)) best = i
    setHover(best)
  }

  if (times.length === 0) {
    return (
      <div className="flex items-center justify-center rounded-lg border border-dashed text-xs text-muted-foreground" style={{ height }}>
        No data for this period yet
      </div>
    )
  }

  return (
    <div className="relative">
      <svg ref={ref} viewBox={`0 0 ${W} ${H}`} className="w-full" style={{ height }} onMouseMove={onMove} onMouseLeave={() => setHover(null)}>
        {[0, 0.5, 1].map((f) => (
          <g key={f}>
            <line x1={padL} x2={W - 4} y1={y(top * f)} y2={y(top * f)} stroke="var(--border)" strokeDasharray={f ? '3 3' : undefined} />
            <text x={padL - 6} y={y(top * f) + 3} textAnchor="end" fontSize={10} fill="var(--muted-foreground)">
              {format(top * f)}
            </text>
          </g>
        ))}
        {[t0, (t0 + t1) / 2, t1].map((t, i) => (
          <text key={i} x={x(t)} y={H - 4} textAnchor={i === 0 ? 'start' : i === 2 ? 'end' : 'middle'} fontSize={10} fill="var(--muted-foreground)">
            {fmtTime(t)}
          </text>
        ))}
        {paths.map((d, i) => (
          <path key={series[i].label} d={d} fill="none" stroke={series[i].color} strokeWidth={1.6} strokeLinejoin="round" />
        ))}
        {hover !== null && <line x1={x(times[hover])} x2={x(times[hover])} y1={6} y2={6 + plotH} stroke="var(--muted-foreground)" strokeOpacity={0.5} />}
      </svg>
      {hover !== null && (
        <div className="pointer-events-none absolute top-1 right-1 rounded-md border bg-popover px-2 py-1 text-[11px] shadow-md">
          <div className="text-muted-foreground">{new Date(times[hover]).toLocaleString()}</div>
          {series.map((s) => (
            <div key={s.label} className="flex items-center gap-1.5">
              <span className="size-2 rounded-full" style={{ background: s.color }} />
              {s.label} <span className="ml-auto pl-3 font-medium tabular-nums">{format(s.values[hover])}</span>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
