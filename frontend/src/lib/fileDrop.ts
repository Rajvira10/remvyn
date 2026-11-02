import { OnFileDrop } from '../../wailsjs/runtime/runtime'

/** Event fired on the drop target when files are dropped from the OS. */
export const FILE_DROP_EVENT = 'remvyn:file-drop'

/**
 * Wails takes a single file-drop callback, so one listener hands each drop to
 * the element under the cursor that opted in with data-file-drop (and the
 * --wails-drop-target CSS property, which also highlights it while dragging).
 */
export function listenForFileDrops() {
  if (!(window as unknown as { runtime?: unknown }).runtime) return
  OnFileDrop((x, y, dropped) => {
    // macOS also reports drags inside the page, with no files.
    const paths = (dropped ?? []).filter(Boolean)
    if (!paths.length) return
    const target = document.elementFromPoint(x, y)?.closest('[data-file-drop]')
    target?.dispatchEvent(new CustomEvent<string[]>(FILE_DROP_EVENT, { detail: paths }))
  }, true)
}
