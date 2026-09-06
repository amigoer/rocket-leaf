import { useCallback } from "react";
import type { DeadLetterQueue } from "@bindings/model/models";
import * as ibmmqApi from "@/api/ibmmq";
import { useBrokerData, type BrokerData } from "@/hooks/useBrokerData";

/**
 * The queues something else dead-letters into.
 *
 * One request, and the driver does the walking: the queue listing already
 * carries every queue's backout queue, so finding the targets costs nothing
 * beyond reading the queue manager's own DEADQ.
 */
export function useIbmMqDeadLetters(): BrokerData<DeadLetterQueue[]> {
  const load = useCallback((connID: number) => ibmmqApi.deadLetterQueues(connID), []);
  return useBrokerData(load);
}
