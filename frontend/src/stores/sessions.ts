import { create } from 'zustand'
import { api } from '@/lib/api'
import { uid } from '@/lib/utils'
import { hostById } from './vault'

export type TabKind = 'terminal' | 'local' | 'sftp'
export type TabState = 'connecting' | 'auth' | 'connected' | 'closed' | 'error'

/** Panes side by side ('row') or stacked ('col'). */
export type SplitDir = 'row' | 'col'
/** A binary split tree of pane keys. ratio is the share of the first child. */
export type Layout = { pane: string } | { dir: SplitDir; ratio: number; a: Layout; b: Layout }

/**
 * One session: a top-level tab, or a pane split off one. Panes are stored
 * alongside tabs (with parent set) so they render flat and never remount
 * when the layout changes.
 */
export interface Tab {
  key: string
  kind: TabKind
  hostId: string
  title: string
  color: string
  /** Backend session ID for the current attempt (changes on reconnect). */
  sessionId: string
  attempt: number
  state: TabState
  remote?: string
  error?: string
  errorCode?: string
  exitCode?: number
  /** Set on panes: the key of the tab they belong to. */
  parent?: string
  /** Top-level tabs only: how the tab is split. Absent means a single pane. */
  layout?: Layout
  /** Top-level tabs only: the focused pane when split. */
  focus?: string
}

export interface Rect {
  x: number
  y: number
  w: number
  h: number
}

interface SessionsState {
  tabs: Tab[]
  active: string // 'vault' or a top-level tab key
  open: (hostId: string, kind: TabKind) => string
  openLocal: () => string
  update: (key: string, patch: Partial<Tab>) => void
  /** Closes a whole tab, with all its panes. */
  close: (key: string) => void
  /** Closes one pane; closing the last pane closes the tab. */
  closePane: (key: string) => void
  reconnect: (key: string) => void
  activate: (key: string) => void
  /** Activates the tab containing a pane and focuses the pane. */
  show: (key: string) => void
  move: (delta: number) => void
  /** Splits the focused pane of the active tab with a new session to the same place. */
  split: (dir: SplitDir) => void
  focusPane: (key: string) => void
  focusDirection: (dir: 'left' | 'right' | 'up' | 'down') => void
  setRatio: (rootKey: string, path: string, ratio: number) => void
}

export const isTerminal = (t: Tab) => t.kind === 'terminal' || t.kind === 'local'

export const topTabs = (tabs: Tab[]) => tabs.filter((t) => !t.parent)

/** Panes of a top-level tab, in layout order. */
export function paneKeys(t: Tab): string[] {
  if (!t.layout) return [t.key]
  const out: string[] = []
  const walk = (n: Layout): void => {
    if ('pane' in n) out.push(n.pane)
    else {
      walk(n.a)
      walk(n.b)
    }
  }
  walk(t.layout)
  return out
}

/** The focused pane of a top-level tab. */
export const focusedPane = (t: Tab) => (t.layout && t.focus ? t.focus : t.key)

function replacePane(n: Layout, key: string, with_: Layout): Layout {
  if ('pane' in n) return n.pane === key ? with_ : n
  return { ...n, a: replacePane(n.a, key, with_), b: replacePane(n.b, key, with_) }
}

function removePane(n: Layout, key: string): Layout | null {
  if ('pane' in n) return n.pane === key ? null : n
  const a = removePane(n.a, key)
  const b = removePane(n.b, key)
  if (!a) return b
  if (!b) return a
  return { ...n, a, b }
}

function withRatio(n: Layout, path: string, ratio: number): Layout {
  if ('pane' in n) return n
  if (path === '') return { ...n, ratio }
  return path[0] === 'a' ? { ...n, a: withRatio(n.a, path.slice(1), ratio) } : { ...n, b: withRatio(n.b, path.slice(1), ratio) }
}

/** Where each pane sits, as fractions of the tab area. */
export function paneRects(layout: Layout | undefined, rootKey: string): Record<string, Rect> {
  const out: Record<string, Rect> = {}
  if (!layout) {
    out[rootKey] = { x: 0, y: 0, w: 1, h: 1 }
    return out
  }
  splitRects(layout, { x: 0, y: 0, w: 1, h: 1 }, '', (n, r) => {
    if ('pane' in n) out[n.pane] = r
  })
  return out
}

/** Calls fn for every node with its rect and path ('a'/'b' steps from the root). */
export function splitRects(n: Layout, r: Rect, path: string, fn: (n: Layout, r: Rect, path: string) => void) {
  fn(n, r, path)
  if ('pane' in n) return
  if (n.dir === 'row') {
    const w = r.w * n.ratio
    splitRects(n.a, { ...r, w }, path + 'a', fn)
    splitRects(n.b, { ...r, x: r.x + w, w: r.w - w }, path + 'b', fn)
  } else {
    const h = r.h * n.ratio
    splitRects(n.a, { ...r, h }, path + 'a', fn)
    splitRects(n.b, { ...r, y: r.y + h, h: r.h - h }, path + 'b', fn)
  }
}

function release(t: Tab) {
  if (t.state === 'connecting' || t.state === 'auth') void api.cancelConnect(t.sessionId)
  if (isTerminal(t)) void api.closeTerminal(t.sessionId)
  else void api.closeSFTP(t.sessionId)
}

function newTab(hostId: string, kind: TabKind): Tab {
  const h = kind === 'local' ? undefined : hostById(hostId)
  return {
    key: uid(),
    kind,
    hostId,
    title: kind === 'local' ? 'Local' : (h?.label ?? 'Session'),
    color: h?.color ?? '',
    sessionId: uid(),
    attempt: 0,
    state: 'connecting',
  }
}

export const useSessions = create<SessionsState>((set, get) => ({
  tabs: [],
  active: 'vault',

  open: (hostId, kind) => {
    const tab = newTab(hostId, kind)
    set((s) => ({ tabs: [...s.tabs, tab], active: tab.key }))
    return tab.key
  },

  openLocal: () => get().open('', 'local'),

  update: (key, patch) => set((s) => ({ tabs: s.tabs.map((t) => (t.key === key ? { ...t, ...patch } : t)) })),

  close: (key) => {
    const s = get()
    const t = s.tabs.find((x) => x.key === key)
    if (!t) return
    const root = t.parent ? s.tabs.find((x) => x.key === t.parent)! : t
    const gone = new Set(paneKeys(root))
    s.tabs.filter((x) => gone.has(x.key)).forEach(release)
    const top = topTabs(s.tabs)
    const idx = top.findIndex((x) => x.key === root.key)
    const tabs = s.tabs.filter((x) => !gone.has(x.key))
    const rest = top.filter((x) => x.key !== root.key)
    let active = s.active
    if (active === root.key) active = rest[idx]?.key ?? rest[idx - 1]?.key ?? 'vault'
    set({ tabs, active })
  },

  closePane: (key) => {
    const s = get()
    const p = s.tabs.find((x) => x.key === key)
    if (!p) return
    const root = p.parent ? s.tabs.find((x) => x.key === p.parent)! : p
    const layout = root.layout && removePane(root.layout, key)
    if (!layout) return get().close(root.key)
    release(p)
    const single = 'pane' in layout
    const first = paneKeys({ ...root, layout })[0]
    if (p.key !== root.key) {
      const focus = root.focus === key ? first : root.focus
      set({
        tabs: s.tabs
          .filter((x) => x.key !== key)
          .map((x) => (x.key === root.key ? { ...x, layout: single ? undefined : layout, focus: single ? undefined : focus } : x)),
      })
      return
    }
    // The tab's own pane is closing: the first remaining pane takes its place.
    const focus = root.focus === key ? first : root.focus
    const heir: Tab = { ...s.tabs.find((x) => x.key === first)!, parent: undefined, layout: single ? undefined : layout, focus: single ? undefined : focus }
    const tabs: Tab[] = []
    for (const x of s.tabs) {
      if (x.key === first) continue
      if (x.key === root.key) tabs.push(heir)
      else tabs.push(x.parent === root.key ? { ...x, parent: first } : x)
    }
    set({ tabs, active: s.active === root.key ? first : s.active })
  },

  reconnect: (key) => {
    const t = get().tabs.find((x) => x.key === key)
    if (!t) return
    release(t)
    get().update(key, { sessionId: uid(), attempt: t.attempt + 1, state: 'connecting', error: undefined, errorCode: undefined, exitCode: undefined })
  },

  activate: (active) => set({ active }),

  show: (key) => {
    const t = get().tabs.find((x) => x.key === key)
    if (!t) return
    set({ active: t.parent ?? t.key })
    get().focusPane(key)
  },

  move: (delta) => {
    const s = get()
    const order = ['vault', ...topTabs(s.tabs).map((t) => t.key)]
    const i = order.indexOf(s.active)
    set({ active: order[(i + delta + order.length) % order.length] })
  },

  split: (dir) => {
    const s = get()
    const root = s.tabs.find((t) => t.key === s.active)
    if (!root || !isTerminal(root)) return
    const from = s.tabs.find((t) => t.key === focusedPane(root)) ?? root
    const pane: Tab = { ...newTab(from.hostId, from.kind), parent: root.key }
    const layout = replacePane(root.layout ?? { pane: root.key }, from.key, { dir, ratio: 0.5, a: { pane: from.key }, b: { pane: pane.key } })
    set({ tabs: [...s.tabs.map((t) => (t.key === root.key ? { ...t, layout, focus: pane.key } : t)), pane] })
  },

  focusPane: (key) => {
    const s = get()
    const p = s.tabs.find((x) => x.key === key)
    const rootKey = p?.parent ?? key
    const root = s.tabs.find((x) => x.key === rootKey)
    if (!root?.layout || root.focus === key) return
    get().update(rootKey, { focus: key })
  },

  focusDirection: (dir) => {
    const s = get()
    const root = s.tabs.find((t) => t.key === s.active)
    if (!root?.layout) return
    const rects = paneRects(root.layout, root.key)
    const cur = rects[focusedPane(root)]
    if (!cur) return
    const cx = cur.x + cur.w / 2
    const cy = cur.y + cur.h / 2
    const eps = 1e-6
    let best = ''
    let bestDist = Infinity
    for (const [key, r] of Object.entries(rects)) {
      const ok =
        dir === 'left' ? r.x + r.w <= cur.x + eps : dir === 'right' ? r.x >= cur.x + cur.w - eps : dir === 'up' ? r.y + r.h <= cur.y + eps : r.y >= cur.y + cur.h - eps
      if (!ok) continue
      const d = Math.hypot(r.x + r.w / 2 - cx, r.y + r.h / 2 - cy)
      if (d < bestDist) {
        best = key
        bestDist = d
      }
    }
    if (best) get().focusPane(best)
  },

  setRatio: (rootKey, path, ratio) => {
    const root = get().tabs.find((t) => t.key === rootKey)
    if (!root?.layout) return
    get().update(rootKey, { layout: withRatio(root.layout, path, Math.min(0.85, Math.max(0.15, ratio))) })
  },
}))
