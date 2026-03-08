import { Plus, WifiOff } from 'lucide-react'
import { cn } from '@/lib/utils'

type Props = {
  onAddConnection: () => void
  hasConnections: boolean
}

export function ConnectionGate({ onAddConnection, hasConnections }: Props) {
  return (
    <div className="flex h-full flex-col items-center justify-center gap-6 px-6 text-center">
      <div className="flex h-14 w-14 items-center justify-center rounded-md border border-border/50 bg-muted/50">
        <WifiOff className="h-7 w-7 text-muted-foreground" />
      </div>
      <div className="space-y-1">
        <h2 className="text-lg font-medium text-foreground">
          {hasConnections ? '请先连接集群' : '暂无连接配置'}
        </h2>
        <p className="max-w-sm text-sm text-muted-foreground">
          {hasConnections
            ? '在左侧选择「连接管理」添加或选择连接，然后点击连接。'
            : '点击下方按钮添加 NameServer 连接配置。'}
        </p>
      </div>
      <button
        type="button"
        onClick={onAddConnection}
        className={cn(
          'inline-flex items-center gap-2 rounded-md border border-border/50 bg-background px-4 py-2 text-sm font-medium text-foreground hover:bg-accent'
        )}
      >
        <Plus className="h-4 w-4" />
        {hasConnections ? '连接管理' : '添加连接'}
      </button>
    </div>
  )
}
