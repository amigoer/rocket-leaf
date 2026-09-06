import { useCallback } from "react";
import type { Destination } from "@/api/models";
import * as topicApi from "@/api/topic";
import { useBrokerData, type BrokerData } from "@/hooks/useBrokerData";

/**
 * Every stream the connection's region holds, narrowed by the profile's prefix.
 *
 * One request per board here, several inside the driver: ListStreams answers
 * with names and every figure a row shows is a second call per stream, which
 * the driver fans out and folds before it reaches this hook.
 */
export function useKinesisDestinations(): BrokerData<Destination[]> {
  const load = useCallback((connID: number) => topicApi.getTopics(connID), []);
  return useBrokerData(load);
}
