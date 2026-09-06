import { useCallback } from "react";
import type { Destination } from "@/api/models";
import * as topicApi from "@/api/topic";
import { useBrokerData, type BrokerData } from "@/hooks/useBrokerData";

/**
 * Every queue and topic the connection's namespace holds, narrowed by the
 * profile's prefix.
 *
 * One request per board here, several inside the driver: the management API
 * lists queues and topics separately, and the figure a topic row exists to
 * show - how many subscriptions read it - is a further call per topic, which
 * the driver fans out and folds before it reaches this hook.
 */
export function useServiceBusEntities(): BrokerData<Destination[]> {
  const load = useCallback((connID: number) => topicApi.getTopics(connID), []);
  return useBrokerData(load);
}
