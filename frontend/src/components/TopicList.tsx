import { useState, useRef, useCallback, useEffect } from 'react'
import { toast } from 'sonner'
import { RefreshCw, Plus, Search, X, Trash2, Loader2, AlertTriangle } from 'lucide-react'
import { cn } from '@/lib/utils'
import type { TopicItem } from '../../bindings/rocket-leaf/internal/model/models.js'
import { TopicPerm } from '../../bindings/rocket-leaf/internal/model/models.js'
import * as topicApi from '@/api/topic'

const TOOLTIP_DELAY_MS = 150
const MIN_SPIN_MS = 400

type Props = {
  list: TopicItem[]
  loading: boolean
  error: string | null
  onRefresh: () => void
}

const PERM_OPTIONS: { value: TopicPerm; label: string }[] = [
  { value: TopicPerm.PermRW, label: '读写 (RW)' },
  { value: TopicPerm.PermR, label: '只读 (R)' },
  { value: TopicPerm.PermW, label: '只写 (W)' },
  { value: TopicPerm.PermDeny, label: '拒绝 (DENY)' },
]

export function TopicList({ list, loading, error, onRefresh }: Props) {
  const [searchQuery, setSearchQuery] = useState('')
  const [showTooltip, setShowTooltip] = useState(false)
  const [refreshing, setRefreshing] = useState(false)
  const [selectedTopic, setSelectedTopic] = useState<string | null>(null)
  const [detail, setDetail] = useState<TopicItem | null>(null)
  const [detailLoading, setDetailLoading] = useState(false)
  const [detailError, setDetailError] = useState<string | null>(null)
  const [createOpen, setCreateOpen] = useState(false)
  const [createSubmitting, setCreateSubmitting] = useState(false)
  const [createError, setCreateError] = useState<string | null>(null)
  const [deleteConfirmTopic, setDeleteConfirmTopic] = useState<string | null>(null)
  const [deletingTopic, setDeletingTopic] = useState<string | null>(null)
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const spinEndRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  const isSpinning = loading || refreshing

  const filteredList = searchQuery.trim()
    ? list.filter((t) => (t.topic ?? '').toLowerCase().includes(searchQuery.trim().toLowerCase()))
    : list

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

  useEffect(() => {
    if (!selectedTopic) {
      setDetail(null)
      setDetailError(null)
      return
    }
    let cancelled = false
    setDetailLoading(true)
    setDetailError(null)
    topicApi
      .getTopicDetail(selectedTopic)
      .then((data) => {
        if (!cancelled) {
          setDetail(data ?? null)
          setDetailError(data ? null : '未获取到详情')
        }
      })
      .catch((e) => {
        if (!cancelled) {
          setDetailError(e instanceof Error ? e.message : String(e))
          setDetail(null)
        }
      })
      .finally(() => {
        if (!cancelled) setDetailLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [selectedTopic])

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

  const handleCreateSubmit = useCallback(
    async (e: React.FormEvent<HTMLFormElement>) => {
      e.preventDefault()
      const form = e.currentTarget
      const topic = (form.querySelector('[name="topic"]') as HTMLInputElement)?.value?.trim()
      const brokerAddr = (form.querySelector('[name="brokerAddr"]') as HTMLInputElement)?.value?.trim()
      const readQueue = Number((form.querySelector('[name="readQueue"]') as HTMLInputElement)?.value) || 4
      const writeQueue = Number((form.querySelector('[name="writeQueue"]') as HTMLInputElement)?.value) || 4
      const perm = (form.querySelector('[name="perm"]') as HTMLSelectElement)?.value ?? TopicPerm.PermRW
      if (!topic) {
        setCreateError('请输入 Topic 名称')
        return
      }
      if (!brokerAddr) {
        setCreateError('请输入 Broker 地址')
        return
      }
      setCreateSubmitting(true)
      setCreateError(null)
      try {
        await topicApi.createTopic(topic, brokerAddr, readQueue, writeQueue, perm)
        toast.success('创建成功')
        setCreateOpen(false)
        onRefresh()
      } catch (err) {
        setCreateError(err instanceof Error ? err.message : String(err))
      } finally {
        setCreateSubmitting(false)
      }
    },
    [onRefresh]
  )

  const handleDeleteConfirm = useCallback(
    async (topic: string) => {
      setDeletingTopic(topic)
      try {
        await topicApi.deleteTopic(topic, '')
        toast.success('已删除')
        setDeleteConfirmTopic(null)
        if (selectedTopic === topic) setSelectedTopic(null)
        onRefresh()
      } catch (err) {
        toast.error(err instanceof Error ? err.message : String(err))
      } finally {
        setDeletingTopic(null)
      }
    },
    [onRefresh, selectedTopic]
  )

  return (
    <div className="flex h-full flex-col">
      <div className="flex shrink-0 flex-col gap-3 border-b border-border/40 px-4 py-3">
        <div className="flex items-center justify-between gap-2">
          <h1 className="text-sm font-medium text-foreground">主题</h1>
          <div className="flex items-center gap-1">
            <button
              type="button"
              onClick={() => {
                setCreateError(null)
                setCreateOpen(true)
              }}
              className="flex items-center gap-1.5 rounded-md border border-border/40 bg-background px-2.5 py-1.5 text-xs text-foreground transition-colors hover:bg-accent"
            >
              <Plus className="h-3.5 w-3.5" />
              创建 Topic
            </button>
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
        </div>
        <div className="relative">
          <Search className="absolute left-2.5 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" aria-hidden />
          <input
            type="text"
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            placeholder="搜索 Topic…"
            className="w-full rounded-md border border-border/40 bg-background py-1.5 pl-8 pr-3 text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-primary/50"
            aria-label="搜索 Topic"
          />
        </div>
      </div>

      <div className="flex min-h-0 flex-1">
        <div className="flex-1 overflow-y-auto scroll-thin p-4">
          {loading && list.length === 0 ? (
            <div className="flex items-center justify-center py-12 text-muted-foreground">加载中…</div>
          ) : error ? (
            <div className="rounded-md border border-border/50 bg-muted/50 px-4 py-3 text-sm text-muted-foreground">
              {error}
            </div>
          ) : filteredList.length === 0 ? (
            <p className="text-sm text-muted-foreground">
              {searchQuery.trim() ? '无匹配 Topic' : '暂无主题'}
            </p>
          ) : (
            <ul className="space-y-1">
              {filteredList.map((t) => (
                <li
                  key={t.topic ?? ''}
                  onClick={() => setSelectedTopic(t.topic ?? null)}
                  className={cn(
                    'flex cursor-pointer items-center justify-between rounded-md border border-border/40 px-3 py-2 transition-colors hover:border-primary/50 hover:bg-accent/50',
                    selectedTopic === (t.topic ?? '') && 'border-primary/50 bg-accent/50'
                  )}
                >
                  <div className="min-w-0 flex-1">
                    <span className="text-sm font-medium text-foreground">{t.topic}</span>
                    <span className="ml-2 text-xs text-muted-foreground">
                      读 {t.readQueue ?? 0} / 写 {t.writeQueue ?? 0}
                    </span>
                  </div>
                  <button
                    type="button"
                    onClick={(e) => {
                      e.stopPropagation()
                      setDeleteConfirmTopic(t.topic ?? '')
                    }}
                    className="shrink-0 rounded-md p-1.5 text-muted-foreground transition-colors hover:bg-destructive/10 hover:text-destructive"
                    title="删除"
                    aria-label="删除"
                  >
                    <Trash2 className="h-4 w-4" />
                  </button>
                </li>
              ))}
            </ul>
          )}
        </div>

        {selectedTopic && (
          <div className="flex w-[360px] shrink-0 flex-col border-l border-border/40 bg-card">
            <div className="flex shrink-0 items-center justify-between border-b border-border/30 px-3 py-2.5">
              <span className="truncate text-sm font-medium text-foreground">Topic 详情</span>
              <button
                type="button"
                onClick={() => setSelectedTopic(null)}
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
              ) : detailError ? (
                <p className="text-sm text-destructive">{detailError}</p>
              ) : detail ? (
                <div className="space-y-4">
                  <div className="space-y-1.5 text-sm">
                    <p>
                      <span className="text-muted-foreground">名称：</span>
                      <span className="font-mono text-foreground">{detail.topic}</span>
                    </p>
                    {detail.cluster != null && detail.cluster !== '' && (
                      <p>
                        <span className="text-muted-foreground">集群：</span>
                        <span className="text-foreground">{detail.cluster}</span>
                      </p>
                    )}
                    <p>
                      <span className="text-muted-foreground">读队列 / 写队列：</span>
                      <span className="text-foreground">{detail.readQueue ?? 0} / {detail.writeQueue ?? 0}</span>
                    </p>
                    {detail.perm != null && detail.perm !== '' && (
                      <p>
                        <span className="text-muted-foreground">权限：</span>
                        <span className="text-foreground">{detail.perm}</span>
                      </p>
                    )}
                  </div>
                  {detail.routes != null && detail.routes.length > 0 && (
                    <div>
                      <h3 className="mb-2 text-xs font-medium text-muted-foreground">路由</h3>
                      <div className="overflow-x-auto rounded-md border border-border/40">
                        <table className="w-full min-w-[280px] text-xs">
                          <thead>
                            <tr className="border-b border-border/40 bg-muted/30">
                              <th className="px-2 py-1.5 text-left font-medium text-foreground">Broker</th>
                              <th className="px-2 py-1.5 text-left font-medium text-foreground">地址</th>
                              <th className="px-2 py-1.5 text-right font-medium text-foreground">读/写</th>
                              <th className="px-2 py-1.5 text-left font-medium text-foreground">权限</th>
                            </tr>
                          </thead>
                          <tbody>
                            {detail.routes.map((r, i) => (
                              <tr key={i} className="border-b border-border/30 last:border-0">
                                <td className="px-2 py-1.5 font-mono text-foreground">{r.broker ?? '-'}</td>
                                <td className="max-w-[120px] truncate px-2 py-1.5 font-mono text-muted-foreground" title={r.brokerAddr ?? ''}>
                                  {r.brokerAddr ?? '-'}
                                </td>
                                <td className="px-2 py-1.5 text-right text-muted-foreground">
                                  {r.readQueue ?? 0} / {r.writeQueue ?? 0}
                                </td>
                                <td className="px-2 py-1.5 text-muted-foreground">{r.perm ?? '-'}</td>
                              </tr>
                            ))}
                          </tbody>
                        </table>
                      </div>
                    </div>
                  )}
                </div>
              ) : null}
            </div>
          </div>
        )}
      </div>

      {createOpen && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40">
          <div className="w-full max-w-md rounded-md border border-border/50 bg-card p-4">
            <h2 className="text-sm font-medium text-card-foreground">创建 Topic</h2>
            {createError && (
              <div className="mt-2 rounded-md border border-destructive/50 bg-destructive/10 px-3 py-2 text-sm text-destructive">
                {createError}
              </div>
            )}
            <form onSubmit={handleCreateSubmit} className="mt-4 space-y-3">
              <div>
                <label id="create-topic-name-label" className="mb-1 block text-xs text-muted-foreground">Topic 名称</label>
                <input
                  id="create-topic-name"
                  name="topic"
                  type="text"
                  aria-labelledby="create-topic-name-label"
                  className="w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
                  placeholder="例如：my_topic"
                />
              </div>
              <div>
                <label id="create-topic-broker-label" className="mb-1 block text-xs text-muted-foreground">Broker 地址</label>
                <input
                  id="create-topic-broker"
                  name="brokerAddr"
                  type="text"
                  aria-labelledby="create-topic-broker-label"
                  className="w-full rounded-md border border-input bg-background px-3 py-2 text-sm font-mono"
                  placeholder="例如：192.168.1.1:10911"
                />
              </div>
              <div className="grid grid-cols-2 gap-3">
                <div>
                  <label id="create-topic-read-label" className="mb-1 block text-xs text-muted-foreground">读队列数</label>
                  <input
                    id="create-topic-read"
                    name="readQueue"
                    type="number"
                    min={1}
                    defaultValue={4}
                    aria-labelledby="create-topic-read-label"
                    className="w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
                  />
                </div>
                <div>
                  <label id="create-topic-write-label" className="mb-1 block text-xs text-muted-foreground">写队列数</label>
                  <input
                    id="create-topic-write"
                    name="writeQueue"
                    type="number"
                    min={1}
                    defaultValue={4}
                    aria-labelledby="create-topic-write-label"
                    className="w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
                  />
                </div>
              </div>
              <div>
                <label id="create-topic-perm-label" className="mb-1 block text-xs text-muted-foreground">权限</label>
                <select
                  id="create-topic-perm"
                  name="perm"
                  aria-labelledby="create-topic-perm-label"
                  className="w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
                  defaultValue={TopicPerm.PermRW}
                >
                  {PERM_OPTIONS.map((o) => (
                    <option key={o.value} value={o.value}>
                      {o.label}
                    </option>
                  ))}
                </select>
              </div>
              <div className="flex justify-end gap-2 pt-2">
                <button
                  type="button"
                  onClick={() => setCreateOpen(false)}
                  className="rounded-md border border-border/40 px-3 py-1.5 text-sm text-muted-foreground hover:bg-accent"
                >
                  取消
                </button>
                <button
                  type="submit"
                  disabled={createSubmitting}
                  className="rounded-md bg-primary px-3 py-1.5 text-sm text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
                >
                  {createSubmitting ? '创建中…' : '创建'}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {deleteConfirmTopic && (
        <div
          className="fixed inset-0 z-50 flex items-center justify-center bg-black/40"
          onClick={() => deletingTopic === null && setDeleteConfirmTopic(null)}
          role="dialog"
          aria-modal="true"
          aria-labelledby="delete-topic-title"
        >
          <div
            className="w-full max-w-sm rounded-md border border-border/50 bg-card p-4 shadow-sm"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="flex gap-3">
              <span className="flex h-9 w-9 shrink-0 items-center justify-center rounded-md bg-destructive/10 text-destructive">
                <AlertTriangle className="h-5 w-5" aria-hidden />
              </span>
              <div className="min-w-0 flex-1">
                <h2 id="delete-topic-title" className="text-sm font-medium text-card-foreground">
                  删除 Topic
                </h2>
                <p className="mt-1.5 text-sm text-muted-foreground">
                  确定删除 Topic「<span className="font-mono text-foreground">{deleteConfirmTopic}</span>」？此操作不可恢复。
                </p>
              </div>
            </div>
            <div className="mt-4 flex justify-end gap-2">
              <button
                type="button"
                onClick={() => setDeleteConfirmTopic(null)}
                disabled={deletingTopic !== null}
                className="rounded-md border border-border/50 px-3 py-1.5 text-sm text-muted-foreground hover:bg-accent disabled:opacity-50"
              >
                取消
              </button>
              <button
                type="button"
                onClick={() => handleDeleteConfirm(deleteConfirmTopic)}
                disabled={deletingTopic !== null}
                className="rounded-md bg-destructive px-3 py-1.5 text-sm text-destructive-foreground hover:bg-destructive/90 disabled:opacity-50"
              >
                {deletingTopic === deleteConfirmTopic ? (
                  <>
                    <Loader2 className="mr-1.5 inline h-4 w-4 animate-spin" aria-hidden />
                    删除中…
                  </>
                ) : (
                  '确定删除'
                )}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
