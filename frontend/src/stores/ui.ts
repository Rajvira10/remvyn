import { create } from 'zustand'

export type VaultPage = 'hosts' | 'monitoring' | 'passwords' | 'keychain' | 'forwarding' | 'snippets' | 'knownhosts' | 'agents' | 'settings'

interface UIState {
  page: VaultPage
  paletteOpen: boolean
  shortcutsOpen: boolean
  editingHostId: string | null // '' = new host
  /** Whether the metrics panel is shown beside terminals (remembered). */
  monitorOpen: boolean
  setMonitorOpen: (o: boolean) => void
  setPage: (p: VaultPage) => void
  setPaletteOpen: (o: boolean) => void
  setShortcutsOpen: (o: boolean) => void
  editHost: (id: string | null) => void
}

const REOPEN_KEY = 'remvyn.reopenPage'

/** Open page after the next window reload (switching vaults reloads). */
export function reopenPage(page: VaultPage) {
  sessionStorage.setItem(REOPEN_KEY, page)
}

const initialPage = (sessionStorage.getItem(REOPEN_KEY) as VaultPage | null) ?? 'hosts'
sessionStorage.removeItem(REOPEN_KEY)

export const useUI = create<UIState>((set) => ({
  page: initialPage,
  paletteOpen: false,
  shortcutsOpen: false,
  editingHostId: null,
  monitorOpen: localStorage.getItem('remvyn.monitorOpen') === '1',
  setMonitorOpen: (monitorOpen) => {
    localStorage.setItem('remvyn.monitorOpen', monitorOpen ? '1' : '0')
    set({ monitorOpen })
  },
  setPage: (page) => set({ page }),
  setPaletteOpen: (paletteOpen) => set({ paletteOpen }),
  setShortcutsOpen: (shortcutsOpen) => set({ shortcutsOpen }),
  editHost: (editingHostId) => set({ editingHostId }),
}))
