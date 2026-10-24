import { useEffect, useState } from 'react'
import { useSettings } from '@/stores/settings'

function systemDark() {
  return window.matchMedia?.('(prefers-color-scheme: dark)').matches ?? true
}

/** Returns the effective theme and keeps the <html> class in sync. */
export function useResolvedTheme(): 'dark' | 'light' {
  const pref = useSettings((s) => s.settings?.theme ?? 'dark')
  const [sys, setSys] = useState(systemDark())
  useEffect(() => {
    const mq = window.matchMedia?.('(prefers-color-scheme: dark)')
    if (!mq) return
    const fn = () => setSys(mq.matches)
    mq.addEventListener('change', fn)
    return () => mq.removeEventListener('change', fn)
  }, [])
  const resolved = pref === 'system' ? (sys ? 'dark' : 'light') : pref
  useEffect(() => {
    document.documentElement.classList.toggle('dark', resolved === 'dark')
    document.documentElement.style.colorScheme = resolved
  }, [resolved])
  return resolved
}
