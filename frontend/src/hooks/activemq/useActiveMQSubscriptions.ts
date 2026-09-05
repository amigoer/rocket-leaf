import { useCallback } from "react";
import type { Subscription } from "@/api/models";
import * as consumerApi from "@/api/consumer";
import { useBrokerData, type BrokerData } from "@/hooks/useBrokerData";

/**
 * Every durable subscription on the connected broker.
 *
 * It goes through the canonical subscription API rather than an ActiveMQ one:
 * a durable subscription is a subscription, one request answers the whole
 * list, and what only this family has - the client id, the JMS selector,
 * whether the subscriber is attached right now - rides in the attribute map.
 *
 * Both active and inactive ones are listed. An inactive durable subscription
 * is a subscriber whose client is not connected, which is the state durability
 * exists for and exactly what somebody comes to this page to look at.
 */
export function useActiveMQSubscriptions(): BrokerData<Subscription[]> {
  const load = useCallback((connID: number) => consumerApi.getConsumerGroups(connID), []);
  return useBrokerData(load);
}
