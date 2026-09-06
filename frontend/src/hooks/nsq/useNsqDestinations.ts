import { useCallback } from "react";
import type { Destination } from "@/api/models";
import * as topicApi from "@/api/topic";
import { useBrokerData, type BrokerData } from "@/hooks/useBrokerData";

/**
 * Every topic on the connected cluster, folded to one row per name.
 *
 * One request per nsqd rather than one for the cluster, because there is no
 * endpoint that answers for the cluster: a topic exists on the daemon it was
 * created on, and the driver adds the copies up before they reach here.
 */
export function useNsqDestinations(): BrokerData<Destination[]> {
  const load = useCallback((connID: number) => topicApi.getTopics(connID), []);
  return useBrokerData(load);
}
