import { useCallback } from "react";
import type { Subscription } from "@/api/models";
import * as consumerApi from "@/api/consumer";
import { useBrokerData, type BrokerData } from "@/hooks/useBrokerData";

/**
 * Every subscription the connection's project holds, narrowed by the profile's
 * prefix.
 *
 * One request, unlike the topics board: everything a subscription reports is
 * on the subscription itself. What is not on it is the backlog, which is a
 * Cloud Monitoring metric and the reason the lag capability is degraded.
 */
export function useGooglePubSubSubscriptions(): BrokerData<Subscription[]> {
  const load = useCallback((connID: number) => consumerApi.getConsumerGroups(connID), []);
  return useBrokerData(load);
}
