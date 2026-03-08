import { useMemo } from 'react'
import { Leaf, Sun, Moon, Monitor } from 'lucide-react'
import { cn } from '@/lib/utils'
import { useTheme } from '@/hooks/useTheme'

function isMac(): boolean {
  if (typeof navigator === 'undefined') return false
  return navigator.platform === 'MacIntel' || /Mac|Darwin/.test(navigator.userAgent)
}

export function TitleBar() {
  const mac = useMemo(isMac, [])
  const { mode, effectiveDark, cycleTheme } = useTheme()

  const ThemeIcon = mode === 'system' ? Monitor : effectiveDark ? Moon : Sun
  const themeLabel = mode === 'system' ? '跟随系统' : mode === 'light' ? '浅色' : '深色'

  return (
    <header
      className={cn(
        'flex h-10 shrink-0 select-none items-center gap-2 border-b border-border/50 bg-background px-3',
        '[--wails-draggable:drag]',
        mac && 'pl-[72px]'
      )}
    >
      <Leaf className="h-5 w-5 shrink-0 text-muted-foreground" aria-hidden />
      <span className="min-w-0 flex-1 truncate text-sm font-medium text-foreground">Rocket-Leaf</span>
      <button
        type="button"
        onClick={cycleTheme}
        title={`主题：${themeLabel}（点击切换）`}
        className={cn(
          'flex h-8 w-8 shrink-0 items-center justify-center rounded-md text-muted-foreground hover:bg-accent hover:text-foreground',
          '[--wails-draggable:no-drag]'
        )}
        aria-label={`切换主题，当前：${themeLabel}`}
      >
        <ThemeIcon className="h-4 w-4" />
      </button>
    </header>
  )
}
