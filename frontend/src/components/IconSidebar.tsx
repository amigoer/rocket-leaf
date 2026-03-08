import { useState, useRef, useCallback } from 'react'
import { Home, LayoutGrid, Users, Mail, BarChart3, Server, Github, Settings } from 'lucide-react'
import { Browser } from '@wailsio/runtime'
import { cn } from '@/lib/utils'

const GITHUB_URL = 'https://github.com/codermast/rocket-leaf'

export type NavId = 'home' | 'topics' | 'consumers' | 'messages' | 'cluster' | 'connections' | 'settings' | 'github'

const TOOLTIP_DELAY_MS = 150

const MAIN_NAV: { id: NavId; icon: React.ElementType; label: string; href?: string }[] = [
  { id: 'home', icon: Home, label: '概览' },
  { id: 'topics', icon: LayoutGrid, label: '主题' },
  { id: 'consumers', icon: Users, label: '消费者组' },
  { id: 'messages', icon: Mail, label: '消息' },
  { id: 'cluster', icon: BarChart3, label: '集群' },
  { id: 'connections', icon: Server, label: '连接管理' },
]

const BOTTOM_NAV: { id: NavId; icon: React.ElementType; label: string; href?: string }[] = [
  { id: 'github', icon: Github, label: 'GitHub', href: GITHUB_URL },
  { id: 'settings', icon: Settings, label: '设置' },
]

export function IconSidebar({
  active,
  onSelect,
  disabledIds = [],
}: {
  active: NavId
  onSelect: (id: NavId) => void
  disabledIds?: NavId[]
}) {
  const [hoveredId, setHoveredId] = useState<NavId | null>(null)
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  const clearTimer = useCallback(() => {
    if (timerRef.current) {
      clearTimeout(timerRef.current)
      timerRef.current = null
    }
  }, [])

  const handleEnter = useCallback(
    (id: NavId) => {
      clearTimer()
      timerRef.current = setTimeout(() => setHoveredId(id), TOOLTIP_DELAY_MS)
    },
    [clearTimer]
  )
  const handleLeave = useCallback(() => {
    clearTimer()
    setHoveredId(null)
  }, [clearTimer])

  const renderItem = (item: (typeof MAIN_NAV)[0], isBottom = false) => {
    const { id, icon: Icon, label, href } = item
    const disabled = !isBottom && disabledIds.includes(id)
    const isLink = Boolean(href)
    const isActive = active === id

    const buttonClass = cn(
      'flex h-10 w-10 items-center justify-center rounded-md text-muted-foreground transition-[background-color,color] duration-200 ease-out',
      isActive ? 'bg-accent text-accent-foreground' : 'hover:bg-accent',
      disabled && 'pointer-events-none opacity-50'
    )

    const content = (
      <>
        <Icon className="h-5 w-5" />
        {hoveredId === id && (
          <span
            className="absolute left-full top-1/2 z-50 ml-2 -translate-y-1/2 whitespace-nowrap rounded-md border border-border/50 bg-card px-2 py-1.5 text-xs text-card-foreground shadow-sm"
            role="tooltip"
          >
            {label}
          </span>
        )}
      </>
    )

    const handleLinkClick = () => {
      if (!href) return
      Browser.OpenURL(href).catch(() => window.open(href, '_blank', 'noopener,noreferrer'))
    }

    return (
      <div key={id} className="relative">
        {isLink ? (
          <button
            type="button"
            onClick={handleLinkClick}
            onMouseEnter={() => handleEnter(id)}
            onMouseLeave={handleLeave}
            className={buttonClass}
            aria-label={label}
          >
            {content}
          </button>
        ) : (
          <button
            type="button"
            disabled={disabled}
            onClick={() => onSelect(id)}
            onMouseEnter={() => !disabled && handleEnter(id)}
            onMouseLeave={handleLeave}
            className={buttonClass}
            aria-label={label}
            aria-current={isActive ? 'page' : undefined}
          >
            {content}
          </button>
        )}
      </div>
    )
  }

  return (
    <aside className="flex w-14 shrink-0 flex-col border-r border-border/40 bg-muted/30 transition-[background-color,border-color] duration-200 ease-out">
      <nav className="flex flex-1 flex-col gap-0.5 p-1.5">
        {MAIN_NAV.map((item) => renderItem(item))}
      </nav>
      <nav className="flex flex-col gap-0.5 border-t border-border/40 p-1.5">
        {BOTTOM_NAV.map((item) => renderItem(item, true))}
      </nav>
    </aside>
  )
}

