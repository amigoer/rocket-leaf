import { useCallback } from "react";
import type { DeadLetterQueue } from "@bindings/model/models";
import * as pubsubApi from "@/api/googlepubsub";
import { useBrokerData, type BrokerData } from "@/hooks/useBrokerData";

/**
 * The topics subscriptions give up into.
 *
 * Nothing marks a dead-letter topic in Pub/Sub - it is an ordinary topic that
 * some subscription's policy points at - so this is a walk backwards through
 * the topology rather than a lookup, and the driver does the walking. The
 * policy is on the subscription, so every source names two things.
 */
export function useGooglePubSubDeadLetters(): BrokerData<DeadLetterQueue[]> {
  const load = useCallback((connID: number) => pubsubApi.deadLetterQueues(connID), []);
  return useBrokerData(load);
}
