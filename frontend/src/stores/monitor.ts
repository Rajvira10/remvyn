import { useEffect } from 'react'
import { create } from 'zustand'
import { api } from '@/lib/api'
import type { AlertEvent, Metrics, MonitorStatus } from '@/lib/types'

/** A recent live sample kept in memory for sparklines (free tier). */
export interface LivePoint {
  at: number
  cpu: number
  mem: number
  rx: number
  tx: number
}

const LIVE_POINTS = 90

interface MonitorState {
  latest: Record<string, Metrics>
  live: Record<string, LivePoint[]>
  status: Record<string, MonitorStatus>
  /** Open (unresolved) alerts by event ID, for badges. */
  openAlerts: Record<number, AlertEvent>
  applySample: (m: Metrics) => void
  applyStatus: (s: MonitorStatus) => void
  applyAlert: (e: AlertEvent) => void
  loadAlerts: () => Promise<void>
}

export const useMonitor = create<MonitorState>((set) => ({
  latest: {},
  live: {},
  status: {},
  openAlerts: {},
  applySample: (m) =>
    set((s) => {
      const pts = [...(s.live[m.hostId] ?? []), { at: m.at, cpu: m.cpuPct, mem: m.memPct, rx: m.rxRate, tx: m.txRate }]
      return { latest: { ...s.latest, [m.hostId]: m }, live: { ...s.live, [m.hostId]: pts.slice(-LIVE_POINTS) } }
    }),
  applyStatus: (st) => set((s) => ({ status: { ...s.status, [st.hostId]: st } })),
  applyAlert: (e) =>
    set((s) => {
      const open = { ...s.openAlerts }
      if (e.resolvedAt) delete open[e.id]
      else open[e.id] = e
      return { openAlerts: open }
    }),
  loadAlerts: async () => {
    const events = await api.alertEvents()
    const open: Record<number, AlertEvent> = {}
    for (const e of events) if (!e.resolvedAt) open[e.id] = e
    set({ openAlerts: open })
  },
}))

/** Streams live metrics for a host while the calling component is mounted and active. */
export function useLiveMetrics(hostId: string | undefined, active = true) {
  useEffect(() => {
    if (!hostId || !active) return
    void api.monitorWatch(hostId).then((st) => useMonitor.getState().applyStatus(st)).catch(() => {})
    void api.monitorLatest(hostId).then((m) => m && useMonitor.getState().applySample(m))
    return () => void api.monitorUnwatch(hostId)
  }, [hostId, active])
  const metrics = useMonitor((s) => (hostId ? s.latest[hostId] : undefined))
  const status = useMonitor((s) => (hostId ? s.status[hostId] : undefined))
  const live = useMonitor((s) => (hostId ? s.live[hostId] : undefined))
  return { metrics, status, live }
}
