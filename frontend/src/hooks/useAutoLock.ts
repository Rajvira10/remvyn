import { useEffect } from 'react'
import { api } from '@/lib/api'
import { useLock } from '@/stores/lock'
import { useSettings } from '@/stores/settings'

/** Locks Remvyn after the configured idle time (only with a master password). */
export function useAutoLock() {
  const protectedVault = useLock((s) => s.status?.protected && !s.status.locked)
  const minutes = useSettings((s) => s.settings?.autoLockMinutes ?? 0)

  useEffect(() => {
    if (!protectedVault || minutes <= 0) return
    let last = Date.now()
    const touch = () => {
      last = Date.now()
    }
    const opts = { capture: true, passive: true }
    const kinds = ['keydown', 'mousedown', 'mousemove', 'wheel', 'touchstart'] as const
    kinds.forEach((k) => window.addEventListener(k, touch, opts))
    const id = window.setInterval(() => {
      if (Date.now() - last >= minutes * 60_000) void api.lock().catch(() => {})
    }, 15_000)
    return () => {
      kinds.forEach((k) => window.removeEventListener(k, touch, opts))
      window.clearInterval(id)
    }
  }, [protectedVault, minutes])
}
