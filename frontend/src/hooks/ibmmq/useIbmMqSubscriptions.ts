import { useCallback } from "react";
import type { Subscription } from "@/api/models";
import * as consumerApi from "@/api/consumer";
import { useBrokerData, type BrokerData } from "@/hooks/useBrokerData";

/**
 * Every subscription the queue manager holds.
 *
 * One request per board here, three inside the driver: the definitions, their
 * runtime status, and the queue listing the backlog comes from - because a
 * subscription stores nothing itself and what it is owed sits on the queue it
 * delivers to.
 */
export function useIbmMqSubscriptions(): BrokerData<Subscription[]> {
  const load = useCallback((connID: number) => consumerApi.getConsumerGroups(connID), []);
  return useBrokerData(load);
}
