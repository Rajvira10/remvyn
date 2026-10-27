import { toast } from 'sonner'
import { create } from 'zustand'
import { api, errorMessage } from '@/lib/api'
import type { UpdateStatus } from '@/lib/types'

interface UpdateStore {
  status: UpdateStatus | null
  dialogOpen: boolean
  /** Version already announced with a toast, so it isn't repeated every check. */
  announced: string
  load: () => Promise<void>
  apply: (s: UpdateStatus) => void
  check: () => Promise<UpdateStatus | null>
  install: () => Promise<void>
  setDialogOpen: (o: boolean) => void
}

export const useUpdate = create<UpdateStore>((set, get) => ({
  status: null,
  dialogOpen: false,
  announced: '',
  load: async () => set({ status: await api.updateStatus() }),
  apply: (status) => {
    set({ status })
    const v = status.latest?.version
    if (status.state === 'available' && v && v !== get().announced && !get().dialogOpen) {
      set({ announced: v })
      toast(`Remvyn ${v} is available`, {
        description: `You have ${status.current}.`,
        duration: 15000,
        action: { label: 'View update', onClick: () => set({ dialogOpen: true }) },
      })
    }
  },
  check: async () => {
    try {
      const status = await api.checkForUpdate()
      // A manual check opens the dialog itself; don't toast on top of it.
      if (status.latest) set({ announced: status.latest.version })
      set({ status })
      return status
    } catch (e) {
      toast.error('Could not check for updates', { description: errorMessage(e) })
      return null
    }
  },
  install: async () => {
    try {
      await api.installUpdate()
    } catch (e) {
      toast.error('Update failed', { description: errorMessage(e) })
    }
  },
  setDialogOpen: (dialogOpen) => set({ dialogOpen }),
}))
