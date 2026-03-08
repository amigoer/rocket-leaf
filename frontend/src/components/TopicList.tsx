import { useState, useRef, useCallback, useEffect } from 'react'
import { toast } from 'sonner'
import { RefreshCw } from 'lucide-react'
import { cn } from '@/lib/utils'
import type { TopicItem } from '../../bindings/rocket-leaf/internal/model/models.js'

const TOOLTIP_DELAY_MS = 150
const MIN_SPIN_MS = 400

type Props = {
  list: TopicItem[]
  loading: boolean
  error: string | null
  onRefresh: () => void
}

export function TopicList({ list, loading, error, onRefresh }: Props) {
  const [showTooltip, setShowTooltip] = useState(false)
  const [refreshing, setRefreshing] = useState(false)
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const spinEndRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  const isSpinning = loading || refreshing

  useEffect(() => {
    if (!loading && refreshing) {
      if (error) toast.error(error)
      else toast.success('已刷新')
      if (spinEndRef.current) clearTimeout(spinEndRef.current)
      spinEndRef.current = setTimeout(() => {
        setRefreshing(false)
        spinEndRef.current = null
      }, MIN_SPIN_MS)
    }
    return () => {
      if (spinEndRef.current) clearTimeout(spinEndRef.current)
    }
  }, [loading, refreshing, error])

  const onEnter = useCallback(() => {
    if (timerRef.current) clearTimeout(timerRef.current)
    timerRef.current = setTimeout(() => setShowTooltip(true), TOOLTIP_DELAY_MS)
  }, [])
  const onLeave = useCallback(() => {
    if (timerRef.current) clearTimeout(timerRef.current)
    timerRef.current = null
    setShowTooltip(false)
  }, [])

  const handleRefresh = useCallback(() => {
    setRefreshing(true)
    onRefresh()
  }, [onRefresh])

  return (
    <div className="flex h-full flex-col">
      <div className="flex shrink-0 items-center justify-between border-b border-border/40 px-4 py-3">
        <h1 className="text-sm font-medium text-foreground">主题</h1>
        <div className="relative">
          <button
            type="button"
            onClick={handleRefresh}
            disabled={isSpinning}
            onMouseEnter={onEnter}
            onMouseLeave={onLeave}
            className={cn(
              'flex h-8 w-8 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-accent hover:text-foreground disabled:opacity-50'
            )}
            aria-label="刷新"
          >
            <RefreshCw className={cn('h-4 w-4', isSpinning && 'animate-spin')} />
          </button>
          {showTooltip && (
            <span
              className="absolute right-full top-1/2 mr-2 -translate-y-1/2 whitespace-nowrap rounded-md border border-border/50 bg-card px-2 py-1.5 text-xs text-card-foreground shadow-sm"
              role="tooltip"
            >
              刷新
            </span>
          )}
        </div>
      </div>
      <div className="flex-1 overflow-y-auto scroll-thin p-4">
        {loading && list.length === 0 ? (
          <div className="flex items-center justify-center py-12 text-muted-foreground">加载中…</div>
        ) : error ? (
          <div className="rounded-md border border-border/50 bg-muted/50 px-4 py-3 text-sm text-muted-foreground">
            {error}
          </div>
        ) : list.length === 0 ? (
          <p className="text-sm text-muted-foreground">暂无主题</p>
        ) : (
          <ul className="space-y-1">
            {list.map((t) => (
              <li
                key={t.topic ?? ''}
                className="flex items-center justify-between rounded-md border border-border/40 px-3 py-2"
              >
                <div>
                  <span className="text-sm font-medium text-foreground">{t.topic}</span>
                  <span className="ml-2 text-xs text-muted-foreground">
                    读 {t.readQueue ?? 0} / 写 {t.writeQueue ?? 0}
                  </span>
                </div>
              </li>
            ))}
          </ul>
        )}
      </div>
    </div>
  )
}
