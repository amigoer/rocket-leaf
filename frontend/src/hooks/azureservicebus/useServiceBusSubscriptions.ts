import { useCallback } from "react";
import type { Subscription } from "@/api/models";
import * as consumerApi from "@/api/consumer";
import { useBrokerData, type BrokerData } from "@/hooks/useBrokerData";

/**
 * Every subscription in the connection's namespace, across every topic.
 *
 * One request per board here, several inside the driver: the management API
 * lists one topic's subscriptions at a time and has no call that lists them
 * all, and each row's rule names are a further call - both of which the driver
 * fans out and folds before they reach this hook.
 */
export function useServiceBusSubscriptions(): BrokerData<Subscription[]> {
  const load = useCallback((connID: number) => consumerApi.getConsumerGroups(connID), []);
  return useBrokerData(load);
}
