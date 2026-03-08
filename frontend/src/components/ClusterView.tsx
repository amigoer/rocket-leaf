import { useState, useCallback, useEffect } from 'react'
import { toast } from 'sonner'
import { RefreshCw, Server, Loader2, X } from 'lucide-react'
import { cn, formatErrorMessage } from '@/lib/utils'
import type { BrokerNode, ClusterInfo } from '../../bindings/rocket-leaf/internal/model/models.js'
import { BrokerRole, NodeStatus } from '../../bindings/rocket-leaf/internal/model/models.js'
import * as clusterApi from '@/api/cluster'

const MIN_SPIN_MS = 400

export function ClusterView() {
  const [loading, setLoading] = useState(true)
  const [refreshing, setRefreshing] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [info, setInfo] = useState<ClusterInfo | null>(null)
  const [selectedBroker, setSelectedBroker] = useState<BrokerNode | null>(null)
  const [detail, setDetail] = useState<BrokerNode | null>(null)
  const [detailLoading, setDetailLoading] = useState(false)

  const brokers = (info?.brokers ?? []).filter((b): b is BrokerNode => b != null)
  const isSpinning = loading || refreshing

  const load = useCallback(async () => {
    setError(null)
    try {
      const data = await clusterApi.getClusterInfo()
      setInfo(data ?? null)
    } catch (e) {
      setError(formatErrorMessage(e))
      setInfo(null)
    } finally {
      setLoading(false)
      setRefreshing(false)
    }
  }, [])

  useEffect(() => {
    load()
  }, [load])

  const handleRefresh = useCallback(() => {
    setRefreshing(true)
    load()
  }, [load])

  useEffect(() => {
    if (!refreshing && loading === false) {
      const t = setTimeout(() => setRefreshing(false), MIN_SPIN_MS)
      return () => clearTimeout(t)
    }
  }, [refreshing, loading])

  useEffect(() => {
    if (!selectedBroker?.address) {
      setDetail(null)
      return
    }
    let cancelled = false
    setDetailLoading(true)
    clusterApi
      .getBrokerDetail(selectedBroker.address)
      .then((data) => {
        if (!cancelled) setDetail(data ?? null)
      })
      .catch(() => {
        if (!cancelled) setDetail(null)
      })
      .finally(() => {
        if (!cancelled) setDetailLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [selectedBroker?.address])

  const statusLabel = (status: NodeStatus) => {
    switch (status) {
      case NodeStatus.NodeOnline:
        return '在线'
      case NodeStatus.NodeWarning:
        return '告警'
      case NodeStatus.NodeOffline:
        return '离线'
      default:
        return '—'
    }
  }

  const roleLabel = (role: BrokerRole) => {
    switch (role) {
      case BrokerRole.RoleMaster:
        return 'Master'
      case BrokerRole.RoleSlave:
        return 'Slave'
      default:
        return '—'
    }
  }

  return (
    <div className="flex h-full flex-col">
      <div className="flex shrink-0 items-center justify-between border-b border-border/40 px-4 py-3">
        <h1 className="text-sm font-medium text-foreground">集群</h1>
        <button
          type="button"
          onClick={handleRefresh}
          disabled={isSpinning}
          className="flex h-8 w-8 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-accent hover:text-foreground disabled:opacity-50"
          aria-label="刷新"
        >
          <RefreshCw className={cn('h-4 w-4', isSpinning && 'animate-spin')} />
        </button>
      </div>
      <div className="flex min-h-0 flex-1">
        <div className="flex-1 overflow-y-auto scroll-thin p-4">
          {loading && !info ? (
            <div className="flex items-center justify-center py-12 text-muted-foreground">
              <Loader2 className="h-6 w-6 animate-spin" />
            </div>
          ) : error ? (
            <div className="rounded-md border border-border/50 bg-muted/50 px-4 py-3 text-sm text-muted-foreground">
              {error}
            </div>
          ) : !info || brokers.length === 0 ? (
            <p className="text-sm text-muted-foreground">暂无集群数据，请先连接 NameServer</p>
          ) : (
            <div className="space-y-6">
              {/* 集群概览 */}
              <section>
                <h2 className="mb-2 text-xs font-medium text-muted-foreground">集群概览</h2>
                <div className="rounded-md border border-border/40 bg-card p-4">
                  <div className="flex flex-wrap items-center gap-4">
                    <div>
                      <p className="text-xs text-muted-foreground">集群名称</p>
                      <p className="font-medium text-foreground">{info.clusterName || '—'}</p>
                    </div>
                    <div>
                      <p className="text-xs text-muted-foreground">Broker</p>
                      <p className="font-medium tabular-nums text-foreground">
                        {info.onlineBrokers ?? 0} / {info.totalBrokers ?? 0} 在线
                      </p>
                    </div>
                    {(info.totalTopics ?? 0) > 0 && (
                      <div>
                        <p className="text-xs text-muted-foreground">Topic</p>
                        <p className="font-medium tabular-nums text-foreground">{info.totalTopics}</p>
                      </div>
                    )}
                    {(info.totalGroups ?? 0) > 0 && (
                      <div>
                        <p className="text-xs text-muted-foreground">消费者组</p>
                        <p className="font-medium tabular-nums text-foreground">{info.totalGroups}</p>
                      </div>
                    )}
                    {info.nameServers != null && info.nameServers.length > 0 && (
                      <div className="min-w-0 flex-1">
                        <p className="text-xs text-muted-foreground">NameServer</p>
                        <p className="truncate font-mono text-sm text-foreground">{info.nameServers.join(', ')}</p>
                      </div>
                    )}
                  </div>
                </div>
              </section>

              {/* Broker 列表 */}
              <section>
                <h2 className="mb-2 text-xs font-medium text-muted-foreground">Broker 列表</h2>
                <div className="rounded-md border border-border/40 bg-card overflow-hidden">
                  <div className="overflow-x-auto">
                    <table className="w-full min-w-[520px] text-sm">
                      <thead>
                        <tr className="border-b border-border/40 bg-muted/30">
                          <th className="px-3 py-2 text-left font-medium text-foreground">Broker</th>
                          <th className="px-3 py-2 text-left font-medium text-foreground">角色</th>
                          <th className="px-3 py-2 text-left font-medium text-foreground">地址</th>
                          <th className="px-3 py-2 text-left font-medium text-foreground">状态</th>
                          <th className="px-3 py-2 text-right font-medium text-foreground">Topic</th>
                          <th className="px-3 py-2 text-right font-medium text-foreground">消费组</th>
                          <th className="px-3 py-2 text-right font-medium text-foreground">TPS 入/出</th>
                        </tr>
                      </thead>
                      <tbody>
                        {brokers.map((b) => (
                          <tr
                            key={b.address ?? b.id}
                            onClick={() => setSelectedBroker(b)}
                            className={cn(
                              'cursor-pointer border-b border-border/30 transition-colors last:border-0 hover:bg-accent/50',
                              selectedBroker?.address === b.address && 'bg-accent/50'
                            )}
                          >
                            <td className="px-3 py-2 font-mono text-foreground">{b.brokerName ?? '—'}</td>
                            <td className="px-3 py-2">
                              <span
                                className={cn(
                                  'rounded px-1.5 py-0.5 text-xs',
                                  b.role === BrokerRole.RoleMaster ? 'bg-primary/10 text-primary' : 'bg-muted text-muted-foreground'
                                )}
                              >
                                {roleLabel(b.role)}
                              </span>
                            </td>
                            <td className="max-w-[160px] truncate px-3 py-2 font-mono text-muted-foreground" title={b.address ?? ''}>
                              {b.address ?? '—'}
                            </td>
                            <td className="px-3 py-2">
                              <span
                                className={cn(
                                  'rounded px-1.5 py-0.5 text-xs',
                                  b.status === NodeStatus.NodeOnline && 'bg-emerald-500/10 text-emerald-700 dark:text-emerald-400',
                                  b.status === NodeStatus.NodeWarning && 'bg-amber-500/10 text-amber-700 dark:text-amber-400',
                                  (b.status === NodeStatus.NodeOffline || !b.status) && 'bg-muted text-muted-foreground'
                                )}
                              >
                                {statusLabel(b.status)}
                              </span>
                            </td>
                            <td className="px-3 py-2 text-right tabular-nums text-muted-foreground">{b.topics ?? 0}</td>
                            <td className="px-3 py-2 text-right tabular-nums text-muted-foreground">{b.groups ?? 0}</td>
                            <td className="px-3 py-2 text-right tabular-nums text-muted-foreground">
                              {b.tpsIn ?? 0} / {b.tpsOut ?? 0}
                            </td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                </div>
              </section>
            </div>
          )}
        </div>

        {/* Broker 详情抽屉 */}
        {selectedBroker && (
          <div className="flex w-[360px] shrink-0 flex-col border-l border-border/40 bg-card">
            <div className="flex shrink-0 items-center justify-between border-b border-border/30 px-3 py-2.5">
              <span className="truncate text-sm font-medium text-foreground">Broker 详情</span>
              <button
                type="button"
                onClick={() => setSelectedBroker(null)}
                className="rounded-md p-1.5 text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
                aria-label="关闭"
              >
                <X className="h-4 w-4" />
              </button>
            </div>
            <div className="flex-1 overflow-y-auto scroll-thin p-3">
              {detailLoading ? (
                <div className="flex items-center justify-center py-8 text-muted-foreground">
                  <Loader2 className="h-6 w-6 animate-spin" />
                </div>
              ) : detail ? (
                <div className="space-y-4 text-sm">
                  <div className="space-y-1.5">
                    <p>
                      <span className="text-muted-foreground">名称：</span>
                      <span className="font-mono text-foreground">{detail.brokerName ?? '—'}</span>
                    </p>
                    <p>
                      <span className="text-muted-foreground">集群：</span>
                      <span className="text-foreground">{detail.cluster ?? '—'}</span>
                    </p>
                    <p>
                      <span className="text-muted-foreground">角色 / 状态：</span>
                      <span className="text-foreground">{roleLabel(detail.role)} / {statusLabel(detail.status)}</span>
                    </p>
                    <p>
                      <span className="text-muted-foreground">地址：</span>
                      <span className="break-all font-mono text-muted-foreground">{detail.address ?? '—'}</span>
                    </p>
                    <p>
                      <span className="text-muted-foreground">Topic / 消费组：</span>
                      <span className="text-foreground">{detail.topics ?? 0} / {detail.groups ?? 0}</span>
                    </p>
                    <p>
                      <span className="text-muted-foreground">TPS 入 / 出：</span>
                      <span className="tabular-nums text-foreground">{detail.tpsIn ?? 0} / {detail.tpsOut ?? 0}</span>
                    </p>
                    {(detail.commitLogDiskUsage ?? 0) > 0 && (
                      <p>
                        <span className="text-muted-foreground">CommitLog 磁盘：</span>
                        <span className="text-foreground">{detail.commitLogDiskUsage}%</span>
                      </p>
                    )}
                    {(detail.consumeQueueDiskUsage ?? 0) > 0 && (
                      <p>
                        <span className="text-muted-foreground">ConsumeQueue 磁盘：</span>
                        <span className="text-foreground">{detail.consumeQueueDiskUsage}%</span>
                      </p>
                    )}
                    {detail.lastUpdate && (
                      <p>
                        <span className="text-muted-foreground">更新时间：</span>
                        <span className="text-muted-foreground">{detail.lastUpdate}</span>
                      </p>
                    )}
                  </div>
                </div>
              ) : (
                <p className="text-sm text-muted-foreground">未获取到详情</p>
              )}
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
