import { LayoutGrid, Users, Mail, BarChart3, Settings } from 'lucide-react'
import { cn } from '@/lib/utils'

export type NavId = 'topics' | 'consumers' | 'messages' | 'cluster' | 'connections' | 'settings'

const NAV: { id: NavId; icon: React.ElementType; label: string }[] = [
  { id: 'topics', icon: LayoutGrid, label: 'Topic' },
  { id: 'consumers', icon: Users, label: '消费者组' },
  { id: 'messages', icon: Mail, label: '消息' },
  { id: 'cluster', icon: BarChart3, label: '集群' },
  { id: 'connections', icon: Settings, label: '连接管理' },
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
  return (
    <aside className="flex w-14 shrink-0 flex-col border-r border-border/40 bg-muted/30">
      <nav className="flex flex-col gap-0.5 p-1.5">
        {NAV.map(({ id, icon: Icon, label }) => {
          const disabled = disabledIds.includes(id)
          return (
          <button
            key={id}
            type="button"
            disabled={disabled}
            onClick={() => onSelect(id)}
            title={label}
            className={cn(
              'flex h-10 w-10 items-center justify-center rounded-md text-muted-foreground transition-colors',
              active === id ? 'bg-accent text-accent-foreground' : 'hover:bg-accent',
              disabled && 'pointer-events-none opacity-50'
            )}
            aria-label={label}
            aria-current={active === id ? 'page' : undefined}
          >
            <Icon className="h-5 w-5" />
          </button>
          )
        })}
      </nav>
    </aside>
  )
}

