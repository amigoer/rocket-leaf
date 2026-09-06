import { useCallback } from "react";
import type { Binding, Destination } from "@/api/models";
import * as routingApi from "@/api/routing";
import { useBrokerData, type BrokerData } from "@/hooks/useBrokerData";

export interface SolaceRoutingSnapshot {
  /** The Message VPN's topic endpoints, whose names are their subscriptions. */
  endpoints: Destination[];
  /** Every topic subscription on every queue, as canonical bindings. */
  subscriptions: Binding[];
}

/**
 * Every topic endpoint and every topic subscription, as one snapshot.
 *
 * Read together because they are two halves of one answer. "Where does a
 * publication land" is answered by the subscriptions that match it plus the
 * topic endpoint named after it, and reading the two a moment apart would let
 * the page describe a topology that never existed at any single instant.
 *
 * It is several requests inside the driver rather than one: SEMP has no
 * collection of every subscription in a Message VPN - they live under the
 * queue that carries them and nowhere else - so the driver fans out over the
 * queues and folds the answers.
 */
export function useSolaceRouting(): BrokerData<SolaceRoutingSnapshot> {
  const load = useCallback(async (connID: number): Promise<SolaceRoutingSnapshot> => {
    const [endpoints, subscriptions] = await Promise.all([
      routingApi.getExchanges(connID),
      routingApi.getBindings(connID),
    ]);
    return { endpoints, subscriptions };
  }, []);
  return useBrokerData(load);
}
