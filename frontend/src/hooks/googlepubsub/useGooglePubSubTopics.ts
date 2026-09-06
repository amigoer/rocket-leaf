import { useCallback } from "react";
import type { Destination } from "@/api/models";
import * as topicApi from "@/api/topic";
import { useBrokerData, type BrokerData } from "@/hooks/useBrokerData";

/**
 * Every topic the connection's project holds, narrowed by the profile's prefix.
 *
 * One request per board here, several inside the driver: ListTopics answers
 * with topics and nothing else, and the figure this board exists to show - how
 * many subscriptions read each one - is a second call per topic, which the
 * driver fans out and folds before it reaches this hook.
 */
export function useGooglePubSubTopics(): BrokerData<Destination[]> {
  const load = useCallback((connID: number) => topicApi.getTopics(connID), []);
  return useBrokerData(load);
}
