import { create } from 'zustand'
import { api } from '@/lib/api'

interface HistoryState {
  /** Commands typed in Remvyn, per host, most recent first. */
  local: Record<string, string[]>
  /** The server's own shell history, read once per host while Remvyn runs. */
  remote: Record<string, string[]>
  load: (hostId: string) => Promise<void>
  loadRemote: (hostId: string, sessionId: string) => Promise<void>
  add: (hostId: string, cmd: string) => void
  forget: (hostId: string, cmd: string) => void
  clear: () => Promise<void>
}

export const useCommandHistory = create<HistoryState>((set, get) => ({
  local: {},
  remote: {},
  load: async (hostId) => {
    if (get().local[hostId]) return
    try {
      const cmds = await api.commandHistory(hostId)
      set((s) => ({ local: { ...s.local, [hostId]: cmds } }))
    } catch {
      // Locked or unavailable: try again on the next connect.
    }
  },
  loadRemote: async (hostId, sessionId) => {
    if (get().remote[hostId]) return
    set((s) => ({ remote: { ...s.remote, [hostId]: [] } }))
    try {
      const cmds = await api.remoteShellHistory(sessionId)
      set((s) => ({ remote: { ...s.remote, [hostId]: cmds } }))
    } catch {
      // No readable history file, or the session ended.
    }
  },
  add: (hostId, cmd) => {
    set((s) => ({ local: { ...s.local, [hostId]: [cmd, ...(s.local[hostId] ?? []).filter((c) => c !== cmd)] } }))
    void api.addCommand(hostId, cmd).catch(() => {})
  },
  forget: (hostId, cmd) => {
    // The server's history file is left alone; the command is hidden until Remvyn restarts.
    set((s) => ({
      local: { ...s.local, [hostId]: (s.local[hostId] ?? []).filter((c) => c !== cmd) },
      remote: { ...s.remote, [hostId]: (s.remote[hostId] ?? []).filter((c) => c !== cmd) },
    }))
    void api.forgetCommand(hostId, cmd).catch(() => {})
  },
  clear: async () => {
    await api.clearCommandHistory()
    set((s) => ({ local: Object.fromEntries(Object.keys(s.local).map((k) => [k, []])) }))
  },
}))

/** The most recent command for a host that extends what has been typed. */
export function suggestCommand(hostId: string, typed: string): string | undefined {
  if (!typed.trim()) return undefined
  const { local, remote } = useCommandHistory.getState()
  const match = (c: string) => c.length > typed.length && c.startsWith(typed)
  return local[hostId]?.find(match) ?? remote[hostId]?.find(match)
}

/**
 * Past commands for the suggestion list: those starting with typed first,
 * then those containing it, most recent first within each. With nothing
 * typed, the most recent commands.
 */
export function listCommands(hostId: string, typed: string, limit: number): string[] {
  const { local, remote } = useCommandHistory.getState()
  const all = [...(local[hostId] ?? []), ...(remote[hostId] ?? [])]
  const needle = typed.trim()
  const prefix: string[] = []
  const contains: string[] = []
  const seen = new Set<string>()
  for (const c of all) {
    if (seen.has(c) || c === typed) continue
    seen.add(c)
    if (c.startsWith(typed)) prefix.push(c)
    else if (needle && c.includes(needle)) contains.push(c)
    if (prefix.length >= limit) break
  }
  return [...prefix, ...contains].slice(0, limit)
}
