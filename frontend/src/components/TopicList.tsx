import { RefreshCw } from 'lucide-react'
import { cn } from '@/lib/utils'
import type { TopicItem } from '../../bindings/rocket-leaf/internal/model/models.js'

type Props = {
  list: TopicItem[]
  loading: boolean
  error: string | null
  onRefresh: () => void
}

export function TopicList({ list, loading, error, onRefresh }: Props) {
  return (
    <div className="flex h-full flex-col">
      <div className="flex shrink-0 items-center justify-between border-b border-border/40 px-4 py-3">
        <h1 className="text-sm font-medium text-foreground">Topic</h1>
        <button
          type="button"
          onClick={onRefresh}
          disabled={loading}
          className={cn(
            'inline-flex items-center gap-1.5 rounded-md border border-border/50 px-3 py-1.5 text-sm font-medium hover:bg-accent disabled:opacity-50'
          )}
        >
          <RefreshCw className={cn('h-4 w-4', loading && 'animate-spin')} />
          刷新
        </button>
      </div>
      <div className="flex-1 overflow-y-auto scroll-thin p-4">
        {loading && list.length === 0 ? (
          <div className="flex items-center justify-center py-12 text-muted-foreground">加载中…</div>
        ) : error ? (
          <div className="rounded-md border border-border/50 bg-muted/50 px-4 py-3 text-sm text-muted-foreground">
            {error}
          </div>
        ) : list.length === 0 ? (
          <p className="text-sm text-muted-foreground">暂无 Topic</p>
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
                    R{t.readQueue ?? 0} / W{t.writeQueue ?? 0}
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
