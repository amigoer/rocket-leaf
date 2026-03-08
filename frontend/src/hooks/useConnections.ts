import { useState, useEffect, useCallback } from 'react'
import type { Connection } from '../../bindings/rocket-leaf/internal/model/models.js'
import * as connectionApi from '@/api/connection'

export function useConnections() {
  const [list, setList] = useState<(Connection | null)[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const refresh = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      const data = await connectionApi.getConnections()
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

  return { list: list.filter(Boolean) as Connection[], loading, error, refresh }
}
