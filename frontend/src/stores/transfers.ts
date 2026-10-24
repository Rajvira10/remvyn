import { create } from 'zustand'
import { api } from '@/lib/api'
import type { Transfer } from '@/lib/types'

interface TransfersState {
  byId: Record<string, Transfer>
  apply: (t: Transfer) => void
  load: () => Promise<void>
  clearFinished: () => Promise<void>
}

export const useTransfers = create<TransfersState>((set) => ({
  byId: {},
  apply: (t) => set((s) => ({ byId: { ...s.byId, [t.id]: t } })),
  load: async () => {
    const list = await api.transfers()
    const byId: Record<string, Transfer> = {}
    for (const t of list) byId[t.id] = t
    set({ byId })
  },
  clearFinished: async () => {
    await api.clearTransfers()
    set((s) => {
      const byId: Record<string, Transfer> = {}
      for (const t of Object.values(s.byId)) if (t.state === 'running' || t.state === 'queued') byId[t.id] = t
      return { byId }
    })
  },
}))
