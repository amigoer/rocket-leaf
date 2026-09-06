import { useCallback } from "react";
import type { Binding, Destination } from "@/api/models";
import * as routingApi from "@/api/routing";
import { useBrokerData, type BrokerData } from "@/hooks/useBrokerData";

export interface RoutingSnapshot {
  /** The namespace's topics, which are what a message is routed by. */
  topics: Destination[];
  /** Every rule in the namespace, as canonical bindings. */
  rules: Binding[];
}

/**
 * Every topic and every rule, as one snapshot.
 *
 * Read together because neither is useful alone: a topic's row is a count of
 * the rules leaving it, and a rule is meaningless without the topic it starts
 * from. Reading them a moment apart would let a row count rules that have gone.
 *
 * It is several requests inside the driver rather than one - the management
 * API lists a topic's subscriptions one topic at a time and a subscription's
 * rules one subscription at a time - which the driver fans out and folds.
 */
export function useServiceBusRouting(): BrokerData<RoutingSnapshot> {
  const load = useCallback(async (connID: number): Promise<RoutingSnapshot> => {
    const [topics, rules] = await Promise.all([
      routingApi.getExchanges(connID),
      routingApi.getBindings(connID),
    ]);
    return { topics, rules };
  }, []);
  return useBrokerData(load);
}
