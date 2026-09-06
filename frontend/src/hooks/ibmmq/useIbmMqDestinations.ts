import { useCallback } from "react";
import type { Destination } from "@/api/models";
import * as topicApi from "@/api/topic";
import { useBrokerData, type BrokerData } from "@/hooks/useBrokerData";

/**
 * Every queue and topic the connection's queue manager holds.
 *
 * One request per board here, three inside the driver: the queues come from a
 * REST resource in one call, the topics from MQSC, and the count of what is
 * subscribed to each topic from a second MQSC call that is inverted rather
 * than asked per topic.
 */
export function useIbmMqDestinations(): BrokerData<Destination[]> {
  const load = useCallback((connID: number) => topicApi.getTopics(connID), []);
  return useBrokerData(load);
}
