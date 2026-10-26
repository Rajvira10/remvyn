import { create } from 'zustand'
import { api, errorCode } from '@/lib/api'
import type { VaultItem } from '@/lib/types'

interface PasswordsState {
  items: VaultItem[]
  loaded: boolean
  load: () => Promise<void>
  /** Forgets everything decrypted (on lock). */
  clear: () => void
}

export const usePasswords = create<PasswordsState>((set) => ({
  items: [],
  loaded: false,
  load: async () => {
    try {
      set({ items: await api.vaultItems(), loaded: true })
    } catch (e) {
      if (errorCode(e) !== 'locked') throw e
      set({ items: [], loaded: false })
    }
  },
  clear: () => set({ items: [], loaded: false }),
}))
