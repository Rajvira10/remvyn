import { create } from 'zustand'
import { api } from '@/lib/api'
import type { ForwardRule, ForwardStatus, Group, Host, Key, KnownHost, Snippet } from '@/lib/types'

interface VaultState {
  hosts: Host[]
  groups: Group[]
  keys: Key[]
  snippets: Snippet[]
  forwards: ForwardRule[]
  statuses: Record<string, ForwardStatus>
  knownHosts: KnownHost[]
  loaded: boolean

  loadAll: () => Promise<void>
  loadHosts: () => Promise<void>
  loadKeys: () => Promise<void>
  loadSnippets: () => Promise<void>
  loadForwards: () => Promise<void>
  loadKnownHosts: () => Promise<void>
  setStatus: (s: ForwardStatus) => void
}

export const useVault = create<VaultState>((set, get) => ({
  hosts: [],
  groups: [],
  keys: [],
  snippets: [],
  forwards: [],
  statuses: {},
  knownHosts: [],
  loaded: false,

  loadAll: async () => {
    await Promise.all([get().loadHosts(), get().loadKeys(), get().loadSnippets(), get().loadForwards(), get().loadKnownHosts()])
    set({ loaded: true })
  },
  loadHosts: async () => {
    const [hosts, groups] = await Promise.all([api.hosts(), api.groups()])
    set({ hosts, groups })
  },
  loadKeys: async () => set({ keys: await api.keys() }),
  loadSnippets: async () => set({ snippets: await api.snippets() }),
  loadForwards: async () => {
    const [forwards, sts] = await Promise.all([api.forwards(), api.forwardStatuses()])
    const statuses: Record<string, ForwardStatus> = {}
    for (const s of sts) statuses[s.ruleId] = s
    set({ forwards, statuses })
  },
  loadKnownHosts: async () => set({ knownHosts: await api.knownHosts() }),
  setStatus: (s) => set((st) => ({ statuses: { ...st.statuses, [s.ruleId]: s } })),
}))

export function hostById(id: string): Host | undefined {
  return useVault.getState().hosts.find((h) => h.id === id)
}

