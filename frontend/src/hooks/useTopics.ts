import { useState, useEffect, useCallback } from 'react'
import type { TopicItem } from '../../bindings/rocket-leaf/internal/model/models.js'
import * as topicApi from '@/api/topic'

export function useTopics() {
  const [list, setList] = useState<(TopicItem | null)[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const refresh = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      const data = await topicApi.getTopics()
      setList(data)
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
      setList([])
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    refresh()
  }, [refresh])

  return { list: list.filter(Boolean) as TopicItem[], loading, error, refresh }
}
