import { create } from 'zustand'

interface EditorState {
  /** Number of files with unsaved changes, by session tab key. */
  unsaved: Record<string, number>
  setUnsaved: (tabKey: string, n: number) => void
}

export const useEditorState = create<EditorState>((set) => ({
  unsaved: {},
  setUnsaved: (tabKey, n) =>
    set((s) => {
      if ((s.unsaved[tabKey] ?? 0) === n) return s
      const unsaved = { ...s.unsaved }
      if (n) unsaved[tabKey] = n
      else delete unsaved[tabKey]
      return { unsaved }
    }),
}))
