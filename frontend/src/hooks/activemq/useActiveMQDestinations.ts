import { useCallback } from "react";
import type { Destination } from "@/api/models";
import * as topicApi from "@/api/topic";
import { useBrokerData, type BrokerData } from "@/hooks/useBrokerData";

/**
 * Every queue and topic on the connected broker.
 *
 * One request for both, because both are destinations: the driver reduces
 * Classic's two MBean trees and Artemis's address-and-queue pair to the same
 * shape before either reaches here.
 *
 * Internal destinations are excluded. Classic publishes an advisory topic per
 * destination per event, which on a broker with twenty queues is several
 * hundred rows nobody declared.
 */
export function useActiveMQDestinations(): BrokerData<Destination[]> {
  const load = useCallback((connID: number) => topicApi.getTopics(connID), []);
  return useBrokerData(load);
}
