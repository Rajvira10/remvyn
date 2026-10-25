import { create } from 'zustand'
import { api } from '@/lib/api'
import type { AgentApproval, AgentLogEntry, AgentStatus } from '@/lib/types'

interface AgentState {
  status: AgentStatus | null
  approvals: AgentApproval[]
  log: AgentLogEntry[]
  logLoaded: boolean
  load: () => Promise<void>
  loadLog: () => Promise<void>
  setStatus: (s: AgentStatus) => void
  addApproval: (a: AgentApproval) => void
  removeApproval: (id: string) => void
  addLog: (e: AgentLogEntry) => void
  clearLog: () => void
}

const LOG_LIMIT = 300

export const useAgent = create<AgentState>((set) => ({
  status: null,
  approvals: [],
  log: [],
  logLoaded: false,
  load: async () => {
    const [status, approvals] = await Promise.all([api.agentStatus(), api.pendingAgentApprovals()])
    set({ status, approvals })
  },
  loadLog: async () => {
    const log = await api.agentLog(LOG_LIMIT)
    set({ log, logLoaded: true })
  },
  setStatus: (status) => set({ status }),
  addApproval: (a) => set((s) => (s.approvals.some((x) => x.id === a.id) ? s : { approvals: [...s.approvals, a] })),
  removeApproval: (id) => set((s) => ({ approvals: s.approvals.filter((a) => a.id !== id) })),
  addLog: (e) => set((s) => ({ log: [e, ...s.log].slice(0, LOG_LIMIT) })),
  clearLog: () => set({ log: [] }),
}))
