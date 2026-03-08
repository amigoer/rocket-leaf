import { useState, useEffect, useCallback } from 'react'

const STORAGE_KEY = 'rocket-leaf-theme'
export type ThemeMode = 'light' | 'dark' | 'system'

function getSystemDark(): boolean {
  if (typeof window === 'undefined') return false
  return window.matchMedia('(prefers-color-scheme: dark)').matches
}

function loadStored(): ThemeMode {
  if (typeof localStorage === 'undefined') return 'system'
  const v = localStorage.getItem(STORAGE_KEY)
  if (v === 'light' || v === 'dark' || v === 'system') return v
  return 'system'
}

function applyTheme(mode: ThemeMode) {
  const dark = mode === 'system' ? getSystemDark() : mode === 'dark'
  document.documentElement.classList.toggle('dark', dark)
}

export function useTheme() {
  const [mode, setModeState] = useState<ThemeMode>(loadStored)
  const [effectiveDark, setEffectiveDark] = useState(() =>
    mode === 'system' ? getSystemDark() : mode === 'dark'
  )

  useEffect(() => {
    applyTheme(mode)
    setEffectiveDark(mode === 'system' ? getSystemDark() : mode === 'dark')
  }, [mode])

  useEffect(() => {
    if (mode !== 'system') return
    const mq = window.matchMedia('(prefers-color-scheme: dark)')
    const handler = () => {
      const dark = mq.matches
      document.documentElement.classList.toggle('dark', dark)
      setEffectiveDark(dark)
    }
    mq.addEventListener('change', handler)
    return () => mq.removeEventListener('change', handler)
  }, [mode])

  const setTheme = useCallback((next: ThemeMode) => {
    setModeState(next)
    localStorage.setItem(STORAGE_KEY, next)
    applyTheme(next)
    setEffectiveDark(next === 'system' ? getSystemDark() : next === 'dark')
  }, [])

  const cycleTheme = useCallback(() => {
    const next: ThemeMode = mode === 'system' ? 'light' : mode === 'light' ? 'dark' : 'system'
    setTheme(next)
  }, [mode, setTheme])

  return { mode, effectiveDark, setTheme, cycleTheme }
}
