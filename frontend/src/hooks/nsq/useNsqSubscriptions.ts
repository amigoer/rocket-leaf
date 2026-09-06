import { useCallback } from "react";
import type { Subscription } from "@/api/models";
import * as consumerApi from "@/api/consumer";
import { useBrokerData, type BrokerData } from "@/hooks/useBrokerData";

/**
 * Every channel on the connected cluster, folded to one row per topic and
 * name.
 *
 * The same request the topics board makes - nsqd reports channels inside
 * /stats rather than on an endpoint of their own - so the driver reads it once
 * and shapes it twice.
 */
export function useNsqSubscriptions(): BrokerData<Subscription[]> {
  const load = useCallback((connID: number) => consumerApi.getConsumerGroups(connID), []);
  return useBrokerData(load);
}
