import { Leaf } from 'lucide-react'
import { cn } from '@/lib/utils'

export function TitleBar() {
  return (
    <header
      className={cn(
        'flex h-10 shrink-0 items-center gap-2 border-b border-border/50 bg-background px-3',
        '[--wails-draggable:drag]'
      )}
    >
      <Leaf className="h-5 w-5 text-muted-foreground" aria-hidden />
      <span className="text-sm font-medium text-foreground">Rocket-Leaf</span>
    </header>
  )
}
