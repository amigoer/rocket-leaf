import { useCallback } from "react";
import type { Destination } from "@/api/models";
import * as topicApi from "@/api/topic";
import { useBrokerData, type BrokerData } from "@/hooks/useBrokerData";

/**
 * Every queue in the connection's Message VPN.
 *
 * One request per board here and rather more inside the driver: the queues
 * come from one paged listing, and then each one's depth and bound consumer
 * count come from its own sub-collections. SEMP puts neither figure on the
 * queue object - what looks like a depth there is a lifetime statistic - so
 * this is the price of the numbers on this page being true.
 */
export function useSolaceDestinations(): BrokerData<Destination[]> {
  const load = useCallback((connID: number) => topicApi.getTopics(connID), []);
  return useBrokerData(load);
}
