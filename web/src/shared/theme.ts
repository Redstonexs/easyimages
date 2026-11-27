import { useEffect, useState } from 'react'

export type ThemeMode = 'light' | 'dark'

const STORAGE_KEY = 'easyimage-theme'

export function readInitialTheme(): ThemeMode {
  if (typeof window === 'undefined') return 'light'
  try {
    const stored = window.localStorage.getItem(STORAGE_KEY)
    if (stored === 'dark' || stored === 'light') return stored
  } catch {
    // Ignore storage restrictions.
  }
  return window.matchMedia?.('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
}

export function applyTheme(theme: ThemeMode): void {
  if (typeof document === 'undefined') return
  const root = document.documentElement
  const isDark = theme === 'dark'
  root.classList.toggle('dark', isDark)
  root.style.colorScheme = isDark ? 'dark' : 'light'
}

function transitionThemeChange(apply: () => void) {
  if (typeof document === 'undefined') {
    apply()
    return
  }
  const prefersReduced = window.matchMedia?.('(prefers-reduced-motion: reduce)').matches
  if (prefersReduced) {
    apply()
    return
  }
  const doc = document as Document & { startViewTransition?: (cb: () => void) => unknown }
  if (typeof doc.startViewTransition === 'function') {
    doc.startViewTransition(() => apply())
    return
  }
  const root = document.documentElement
  root.classList.add('theme-transitioning')
  apply()
  window.setTimeout(() => root.classList.remove('theme-transitioning'), 240)
}

export function useTheme() {
  const [theme, setThemeState] = useState<ThemeMode>(readInitialTheme)

  useEffect(() => {
    applyTheme(theme)
  }, [theme])

  // Follow OS theme changes if the user has not pinned a manual preference.
  useEffect(() => {
    const media = window.matchMedia?.('(prefers-color-scheme: dark)')
    if (!media) return
    const onChange = (event: MediaQueryListEvent) => {
      try {
        const stored = window.localStorage.getItem(STORAGE_KEY)
        if (stored === 'dark' || stored === 'light') return
      } catch {
        // Ignore storage restrictions.
      }
      setThemeState(event.matches ? 'dark' : 'light')
    }
    media.addEventListener('change', onChange)
    return () => media.removeEventListener('change', onChange)
  }, [])

  function setTheme(next: ThemeMode) {
    transitionThemeChange(() => {
      applyTheme(next)
      setThemeState(next)
    })
    try {
      window.localStorage.setItem(STORAGE_KEY, next)
    } catch {
      // Ignore storage quota / private-mode restrictions.
    }
  }

  function toggleTheme() {
    setTheme(theme === 'dark' ? 'light' : 'dark')
  }

  return { theme, setTheme, toggleTheme }
}
