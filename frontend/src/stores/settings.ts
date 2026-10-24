import { create } from 'zustand'
import { api } from '@/lib/api'
import type { AppInfo, Settings } from '@/lib/types'

interface SettingsState {
  settings: Settings | null
  info: AppInfo | null
  load: () => Promise<void>
  save: (s: Settings) => Promise<Settings>
  setLocal: (s: Settings) => void
}

export const useSettings = create<SettingsState>((set) => ({
  settings: null,
  info: null,
  load: async () => {
    const [settings, info] = await Promise.all([api.settings(), api.appInfo()])
    set({ settings, info })
  },
  save: async (s) => {
    const saved = await api.updateSettings(s)
    set({ settings: saved })
    return saved
  },
  setLocal: (settings) => set({ settings }),
}))
