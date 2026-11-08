import { create } from 'zustand'
import { api } from '@/lib/api'
import type { VaultProfile } from '@/lib/types'

/** The vaults on this computer, for the switcher in the title bar. */
interface VaultsState {
  list: VaultProfile[]
  load: () => Promise<void>
}

export const useVaults = create<VaultsState>((set) => ({
  list: [],
  load: async () => {
    try {
      set({ list: await api.listVaults() })
    } catch {
      // Keeps the last list; Settings → Vaults shows the error.
    }
  },
}))
