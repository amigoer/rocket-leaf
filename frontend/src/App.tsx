import { useState, useCallback } from 'react'
import type React from 'react'
import { Toaster, toast } from 'sonner'
import { TitleBar } from '@/components/TitleBar'
import { IconSidebar, type NavId } from '@/components/IconSidebar'
import { ConnectionGate } from '@/components/ConnectionGate'
import { ConnectionManagement } from '@/components/ConnectionManagement'
import { TopicList } from '@/components/TopicList'
import { PlaceholderView } from '@/components/PlaceholderView'
import { useConnections } from '@/hooks/useConnections'
import { useTopics } from '@/hooks/useTopics'
import * as connectionApi from '@/api/connection'
import { ConnectionStatus } from '../bindings/rocket-leaf/internal/model/models.js'
import { cn } from '@/lib/utils'

function App(): React.ReactElement {
  const [activeNav, setActiveNav] = useState<NavId>('topics')
  const [connectingId, setConnectingId] = useState<number | null>(null)

  const { list: connections, loading: connectionsLoading, error: connectionsError, refresh: refreshConnections } = useConnections()
  const { list: topics, loading: topicsLoading, error: topicsError, refresh: refreshTopics } = useTopics()

  const hasConnected = connections.some((c) => c.status === ConnectionStatus.StatusOnline)

  const handleConnect = useCallback(async (id: number) => {
    setConnectingId(id)
    try {
      await connectionApi.connect(id)
      await refreshConnections()
      toast.success('连接成功')
    } catch (e) {
      await refreshConnections()
      toast.error(e instanceof Error ? e.message : String(e))
    } finally {
      setConnectingId(null)
    }
  }, [refreshConnections])

  const handleOpenConnections = useCallback(() => {
    setActiveNav('connections')
  }, [])

  const renderContent = () => {
    if (!hasConnected && activeNav !== 'connections' && activeNav !== 'settings') {
      return <ConnectionGate onAddConnection={handleOpenConnections} hasConnections={connections.length > 0} />
    }
    switch (activeNav) {
      case 'connections':
        return (
          <ConnectionManagement
            list={connections}
            loading={connectionsLoading}
            error={connectionsError}
            onRefresh={refreshConnections}
            onConnect={handleConnect}
            connectingId={connectingId}
          />
        )
      case 'topics':
        return <TopicList list={topics} loading={topicsLoading} error={topicsError} onRefresh={refreshTopics} />
      case 'consumers':
        return <PlaceholderView title="消费者组" description="消费者组列表与消费进度" />
      case 'messages':
        return <PlaceholderView title="消息" description="按 Topic / Key / MessageId 查询与发送" />
      case 'cluster':
        return <PlaceholderView title="集群" description="集群状态与 TPS 监控" />
      case 'settings':
        return <PlaceholderView title="设置" description="软件设置" />
      default:
        return null
    }
  }

  return (
    <div className={cn('flex h-screen flex-col bg-background text-foreground')}>
      <TitleBar />
      <div className="flex min-h-0 flex-1">
        <IconSidebar
          active={activeNav}
          onSelect={setActiveNav}
          disabledIds={!hasConnected ? ['topics', 'consumers', 'messages', 'cluster'] : []}
        />
        <main className="min-w-0 flex-1 overflow-hidden border-border/40">
          {renderContent()}
        </main>
      </div>
      <Toaster position="bottom-right" closeButton style={{ '--width': '260px' } as React.CSSProperties} />
    </div>
  )
}

export default App
