import { useState, useCallback } from 'react'
import type React from 'react'
import { Toaster, toast } from 'sonner'
import { TitleBar } from '@/components/TitleBar'
import { IconSidebar, type NavId } from '@/components/IconSidebar'
import { ConnectionGate } from '@/components/ConnectionGate'
import { ConnectionManagement } from '@/components/ConnectionManagement'
import { TopicList } from '@/components/TopicList'
import { PlaceholderView } from '@/components/PlaceholderView'
import { SettingsView } from '@/components/SettingsView'
import { useConnections } from '@/hooks/useConnections'
import { useTopics } from '@/hooks/useTopics'
import * as connectionApi from '@/api/connection'
import { ConnectionStatus } from '../bindings/rocket-leaf/internal/model/models.js'
import { cn } from '@/lib/utils'

function App(): React.ReactElement {
  const [activeNav, setActiveNav] = useState<NavId>('topics')
  const [connectingId, setConnectingId] = useState<number | null>(null)
  const [disconnectingId, setDisconnectingId] = useState<number | null>(null)

  const { list: connections, loading: connectionsLoading, error: connectionsError, refresh: refreshConnections } = useConnections()
  const { list: topics, loading: topicsLoading, error: topicsError, refresh: refreshTopics } = useTopics()

  const hasConnected = connections.some((c) => c.status === ConnectionStatus.StatusOnline)

  const handleConnect = useCallback(
    async (id: number) => {
      setConnectingId(id)
      try {
        const othersOnline = connections.filter(
          (c) => c.status === ConnectionStatus.StatusOnline && c.id !== id
        )
        for (const c of othersOnline) {
          await connectionApi.disconnect(c.id)
        }
        await connectionApi.connect(id)
        await connectionApi.setDefaultConnection(id)
        await refreshConnections()
        await refreshTopics()
        setActiveNav('topics')
        toast.success('连接成功')
      } catch (e) {
        await refreshConnections()
        toast.error(e instanceof Error ? e.message : String(e))
      } finally {
        setConnectingId(null)
      }
    },
    [connections, refreshConnections, refreshTopics]
  )

  const handleDisconnect = useCallback(async (id: number) => {
    setDisconnectingId(id)
    try {
      await connectionApi.disconnect(id)
      await refreshConnections()
      toast.success('已断开连接')
    } catch (e) {
      await refreshConnections()
      toast.error(e instanceof Error ? e.message : String(e))
    } finally {
      setDisconnectingId(null)
    }
  }, [refreshConnections])

  const handleOpenConnections = useCallback(() => {
    setActiveNav('connections')
  }, [])

  const handleSelectConnection = useCallback(
    async (id: number) => {
      try {
        await connectionApi.setDefaultConnection(id)
        await refreshConnections()
        await refreshTopics()
        setActiveNav('topics')
        toast.success('已切换到该实例')
      } catch (e) {
        await refreshConnections()
        toast.error(e instanceof Error ? e.message : String(e))
      }
    },
    [refreshConnections, refreshTopics]
  )

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
            onDisconnect={handleDisconnect}
            onSelectConnection={handleSelectConnection}
            connectingId={connectingId}
            disconnectingId={disconnectingId}
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
        return <SettingsView />
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
