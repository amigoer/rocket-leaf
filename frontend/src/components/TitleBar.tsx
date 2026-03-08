import { useMemo, useState, useCallback, useEffect } from 'react'
import { Sun, Moon, Monitor, Minus, Square, SquareMinus, X } from 'lucide-react'
import { cn } from '@/lib/utils'
import { useTheme } from '@/hooks/useTheme'

import logoUrl from '@/assets/logo.png'
import { Window } from '@wailsio/runtime'

function isMac(): boolean {
  if (typeof navigator === 'undefined') return false
  return navigator.platform === 'MacIntel' || /Mac|Darwin/.test(navigator.userAgent)
}

export function TitleBar() {
  const mac = useMemo(isMac, [])
  const [isMaximised, setIsMaximised] = useState(false)

  const refreshMaximised = useCallback(async () => {
    try {
      const max = await Window.IsMaximised()
      setIsMaximised(max)
    } catch {
      setIsMaximised(false)
    }
  }, [])

  useEffect(() => {
    refreshMaximised()
  }, [refreshMaximised])

  const { mode, effectiveDark, cycleTheme } = useTheme()

  const ThemeIcon = mode === 'system' ? Monitor : effectiveDark ? Moon : Sun
  const themeLabel = mode === 'system' ? '跟随系统' : mode === 'light' ? '浅色' : '深色'

  const handleMinimise = useCallback(() => {
    Window.Minimise().catch(() => { })
  }, [])
  const handleToggleMaximise = useCallback(() => {
    Window.ToggleMaximise()
      .then(refreshMaximised)
      .catch(() => { })
  }, [refreshMaximised])
  const handleClose = useCallback(() => {
    Window.Close().catch(() => { })
  }, [])

  const btnClass =
    'flex h-8 w-8 shrink-0 items-center justify-center rounded-md text-muted-foreground hover:bg-accent hover:text-foreground [--wails-draggable:no-drag]'
  const closeBtnClass =
    'flex h-8 w-8 shrink-0 items-center justify-center rounded-md text-muted-foreground hover:bg-destructive/15 hover:text-destructive [--wails-draggable:no-drag]'

  return (
    <header
      className={cn(
        'flex h-10 shrink-0 select-none items-center gap-2 border-b border-border/50 bg-background px-3',
        '[--wails-draggable:drag]',
        mac && 'pl-[72px]'
      )}
    >
      <img src={logoUrl} alt="" className="h-9 w-9 shrink-0 object-contain" aria-hidden />
      <span className="min-w-0 flex-1 truncate text-sm font-medium text-foreground">Rocket-Leaf</span>
      {!mac && (
        <div className="flex shrink-0 items-center gap-0.5">
          <button type="button" onClick={handleMinimise} className={btnClass} title="最小化" aria-label="最小化">
            <Minus className="h-4 w-4" />
          </button>
          <button
            type="button"
            onClick={handleToggleMaximise}
            className={btnClass}
            title={isMaximised ? '还原' : '最大化'}
            aria-label={isMaximised ? '还原' : '最大化'}
          >
            {isMaximised ? <SquareMinus className="h-4 w-4" /> : <Square className="h-4 w-4" />}
          </button>
          <button type="button" onClick={handleClose} className={closeBtnClass} title="关闭" aria-label="关闭">
            <X className="h-4 w-4" />
          </button>
        </div>
      )}
      <button
        type="button"
        onClick={cycleTheme}
        title={`主题：${themeLabel}（点击切换）`}
        className={cn(btnClass)}
        aria-label={`切换主题，当前：${themeLabel}`}
      >
        <ThemeIcon className="h-4 w-4" />
      </button>
    </header>
  )
}
