import { useEffect, useLayoutEffect, useRef } from 'react'
import { isMac } from '@/lib/utils'
import { isLocked } from '@/stores/lock'

export interface Hotkey {
  /**
   * e.g. "app+t", "app+shift+]", "escape".
   * "app" is ⌘ on macOS and Ctrl+Shift elsewhere, so plain Ctrl+key keeps
   * reaching the remote shell.
   */
  combo: string
  handler: (e: KeyboardEvent) => void
  /** Fire even when focus is in an input/textarea (including the terminal). */
  allowInInputs?: boolean
}

export function matchesCombo(e: KeyboardEvent, combo: string): boolean {
  const parts = combo.toLowerCase().split('+')
  const key = parts[parts.length - 1]
  const app = parts.includes('app')
  let wantMeta = false
  let wantCtrl = false
  let wantShift = parts.includes('shift')
  if (app) {
    if (isMac) wantMeta = true
    else {
      wantCtrl = true
      wantShift = true
    }
  }
  if (e.metaKey !== wantMeta || e.ctrlKey !== wantCtrl || e.shiftKey !== wantShift) return false
  if (e.altKey !== parts.includes('alt')) return false
  const k = e.key.toLowerCase()
  return (
    k === key ||
    e.code.toLowerCase() === `key${key}` ||
    e.code.toLowerCase() === `digit${key}` ||
    (key === ']' && e.code === 'BracketRight') ||
    (key === '[' && e.code === 'BracketLeft') ||
    (key === ',' && e.code === 'Comma') ||
    (key === '=' && e.code === 'Equal') ||
    (key === '-' && e.code === 'Minus')
  )
}

function inEditable(target: EventTarget | null): boolean {
  const el = target as HTMLElement | null
  if (!el) return false
  return el.isContentEditable || ['INPUT', 'TEXTAREA', 'SELECT'].includes(el.tagName)
}

export function useHotkeys(keys: Hotkey[], enabled = true) {
  const ref = useRef(keys)
  useLayoutEffect(() => {
    ref.current = keys
  })
  useEffect(() => {
    if (!enabled) return
    const onKey = (e: KeyboardEvent) => {
      if (isLocked()) return
      for (const hk of ref.current) {
        if (!matchesCombo(e, hk.combo)) continue
        if (!hk.allowInInputs && inEditable(e.target)) continue
        e.preventDefault()
        e.stopPropagation()
        hk.handler(e)
        return
      }
    }
    window.addEventListener('keydown', onKey, true)
    return () => window.removeEventListener('keydown', onKey, true)
  }, [enabled])
}
