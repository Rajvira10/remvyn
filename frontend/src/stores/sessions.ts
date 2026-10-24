import { create } from 'zustand'
import { api } from '@/lib/api'
import { uid } from '@/lib/utils'
import { hostById } from './vault'

export type TabKind = 'terminal' | 'sftp'
export type TabState = 'connecting' | 'auth' | 'connected' | 'closed' | 'error'

export interface Tab {
  key: string
  kind: TabKind
  hostId: string
  title: string
  color: string
  /** Backend session ID for the current attempt (changes on reconnect). */
  sessionId: string
  attempt: number
  state: TabState
  remote?: string
  error?: string
  errorCode?: string
  exitCode?: number
}

interface SessionsState {
  tabs: Tab[]
  active: string // 'vault' or a tab key
  open: (hostId: string, kind: TabKind) => string
  update: (key: string, patch: Partial<Tab>) => void
  close: (key: string) => void
  reconnect: (key: string) => void
  activate: (key: string) => void
  move: (delta: number) => void
}

function release(t: Tab) {
  if (t.state === 'connecting' || t.state === 'auth') void api.cancelConnect(t.sessionId)
  if (t.kind === 'terminal') void api.closeTerminal(t.sessionId)
  else void api.closeSFTP(t.sessionId)
}

export const useSessions = create<SessionsState>((set, get) => ({
  tabs: [],
  active: 'vault',

  open: (hostId, kind) => {
    const h = hostById(hostId)
    const key = uid()
    const tab: Tab = {
      key,
      kind,
      hostId,
      title: h?.label ?? 'Session',
      color: h?.color ?? '',
      sessionId: uid(),
      attempt: 0,
      state: 'connecting',
    }
    set((s) => ({ tabs: [...s.tabs, tab], active: key }))
    return key
  },

  update: (key, patch) => set((s) => ({ tabs: s.tabs.map((t) => (t.key === key ? { ...t, ...patch } : t)) })),

  close: (key) => {
    const s = get()
    const idx = s.tabs.findIndex((t) => t.key === key)
    if (idx < 0) return
    release(s.tabs[idx])
    const tabs = s.tabs.filter((t) => t.key !== key)
    let active = s.active
    if (active === key) active = tabs[idx]?.key ?? tabs[idx - 1]?.key ?? 'vault'
    set({ tabs, active })
  },

  reconnect: (key) => {
    const t = get().tabs.find((x) => x.key === key)
    if (!t) return
    release(t)
    get().update(key, { sessionId: uid(), attempt: t.attempt + 1, state: 'connecting', error: undefined, errorCode: undefined, exitCode: undefined })
  },

  activate: (active) => set({ active }),

  move: (delta) => {
    const s = get()
    const order = ['vault', ...s.tabs.map((t) => t.key)]
    const i = order.indexOf(s.active)
    set({ active: order[(i + delta + order.length) % order.length] })
  },
}))
