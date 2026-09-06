import { useCallback } from "react";
import type { Destination } from "@/api/models";
import * as topicApi from "@/api/topic";
import { useBrokerData, type BrokerData } from "@/hooks/useBrokerData";

/**
 * Every queue the connection's region holds, narrowed by the profile's prefix.
 *
 * One request per board here, several inside the driver: ListQueues answers
 * with URLs and every figure a row shows is a second call per queue, which the
 * driver fans out and folds before it reaches this hook.
 */
export function useSqsDestinations(): BrokerData<Destination[]> {
  const load = useCallback((connID: number) => topicApi.getTopics(connID), []);
  return useBrokerData(load);
}
