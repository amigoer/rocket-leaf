import { useState, useCallback } from 'react'
import type React from 'react'
import { Toaster, toast } from 'sonner'
import { TitleBar } from '@/components/TitleBar'
import { IconSidebar, type NavId } from '@/components/IconSidebar'
import { ConnectionGate } from '@/components/ConnectionGate'
import { ConnectionManagement } from '@/components/ConnectionManagement'
import { OverviewView } from '@/components/OverviewView'
import { TopicList } from '@/components/TopicList'
import { ConsumerGroupList } from '@/components/ConsumerGroupList'
import { MessageView } from '@/components/MessageView'
import { ClusterView } from '@/components/ClusterView'
import { PlaceholderView } from '@/components/PlaceholderView'
import { SettingsView } from '@/components/SettingsView'
import { useConnections } from '@/hooks/useConnections'
import { useTopics } from '@/hooks/useTopics'
import { useConsumerGroups } from '@/hooks/useConsumerGroups'
import * as connectionApi from '@/api/connection'
import { ConnectionStatus } from '../bindings/rocket-leaf/internal/model/models.js'
import { cn, formatErrorMessage } from '@/lib/utils'

function App(): React.ReactElement {
  const [activeNav, setActiveNav] = useState<NavId>('home')
  const [connectingId, setConnectingId] = useState<number | null>(null)
  const [disconnectingId, setDisconnectingId] = useState<number | null>(null)

  const { list: connections, loading: connectionsLoading, error: connectionsError, refresh: refreshConnections } = useConnections()
  const { list: topics, loading: topicsLoading, error: topicsError, refresh: refreshTopics } = useTopics()
  const { list: consumerGroups, loading: consumerGroupsLoading, error: consumerGroupsError, refresh: refreshConsumerGroups } = useConsumerGroups()

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
        await refreshConsumerGroups()
        setActiveNav('home')
        toast.success('连接成功')
      } catch (e) {
        await refreshConnections()
        toast.error(formatErrorMessage(e))
      } finally {
        setConnectingId(null)
      }
    },
    [connections, refreshConnections, refreshTopics, refreshConsumerGroups]
  )

  const handleDisconnect = useCallback(async (id: number) => {
    setDisconnectingId(id)
    try {
      await connectionApi.disconnect(id)
      await refreshConnections()
      toast.success('已断开连接')
    } catch (e) {
      await refreshConnections()
      toast.error(formatErrorMessage(e))
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
        await refreshConsumerGroups()
        setActiveNav('home')
        toast.success('已切换到该实例')
      } catch (e) {
        await refreshConnections()
        toast.error(formatErrorMessage(e))
      }
    },
    [refreshConnections, refreshTopics, refreshConsumerGroups]
  )

  const renderContent = () => {
    if (!hasConnected && activeNav !== 'connections' && activeNav !== 'settings') {
      return <ConnectionGate onAddConnection={handleOpenConnections} hasConnections={connections.length > 0} />
    }
    switch (activeNav) {
      case 'home':
        return (
          <OverviewView
            connections={connections}
            topicCount={topics.length}
            consumerGroupCount={consumerGroups.length}
            onSelectNav={setActiveNav}
          />
        )
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
        return (
          <ConsumerGroupList
            list={consumerGroups}
            loading={consumerGroupsLoading}
            error={consumerGroupsError}
            onRefresh={refreshConsumerGroups}
          />
        )
      case 'messages':
        return <MessageView />
      case 'cluster':
        return <ClusterView />
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
