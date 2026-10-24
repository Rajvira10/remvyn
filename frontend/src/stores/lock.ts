import { create } from 'zustand'
import { api } from '@/lib/api'
import type { LockStatus, SyncStatus } from '@/lib/types'

interface LockState {
  status: LockStatus | null
  sync: SyncStatus | null
  load: () => Promise<void>
  setStatus: (s: LockStatus) => void
  setSync: (s: SyncStatus) => void
}

/** Master-password lock and GitHub sync state. */
export const useLock = create<LockState>((set) => ({
  status: null,
  sync: null,
  load: async () => {
    const [status, sync] = await Promise.all([api.lockStatus(), api.syncStatus()])
    set({ status, sync })
  },
  setStatus: (status) => set({ status }),
  setSync: (sync) => set({ sync }),
}))

/** True while Flux is locked; app shortcuts are ignored then. */
export const isLocked = () => useLock.getState().status?.locked ?? false
