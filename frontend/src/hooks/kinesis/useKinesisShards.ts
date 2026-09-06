import { useCallback, useEffect, useState } from "react";
import * as kinesisApi from "@/api/kinesis";
import type { Shard } from "@/mq/kinesis/shards";
import { useConnectionScope } from "@/mq/ConnectionScope";

export interface ShardsState {
  shards: Shard[];
  loading: boolean;
  error: string | null;
  refresh: () => Promise<void>;
}

/**
 * One stream's shards.
 *
 * Per stream rather than for the whole region, because that is how the service
 * answers: ListShards takes one stream, and a page that loaded every stream's
 * shards would be one request per stream to fill a table nobody asked for.
 */
export function useKinesisShards(stream: string | null): ShardsState {
  const { id: connID, online } = useConnectionScope();
  const [shards, setShards] = useState<Shard[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async () => {
    if (connID === 0 || !online || stream == null || stream === "") {
      setShards([]);
      setError(null);
      return;
    }
    setLoading(true);
    setError(null);
    try {
      setShards(await kinesisApi.shards(connID, stream));
    } catch (loadError) {
      setShards([]);
      setError(loadError instanceof Error ? loadError.message : String(loadError));
    } finally {
      setLoading(false);
    }
  }, [connID, online, stream]);

  useEffect(() => {
    void load();
  }, [load]);

  return { shards, loading, error, refresh: load };
}
