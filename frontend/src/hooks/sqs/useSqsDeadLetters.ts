import { useCallback } from "react";
import type { DeadLetterQueue } from "@bindings/model/models";
import * as sqsApi from "@/api/sqs";
import { useBrokerData, type BrokerData } from "@/hooks/useBrokerData";

/**
 * The queues other queues redrive into.
 *
 * Nothing marks a dead-letter queue in SQS - it is an ordinary queue another
 * queue's redrive policy points at - so this is a walk backwards through the
 * topology rather than a lookup, and the driver does the walking.
 */
export function useSqsDeadLetters(): BrokerData<DeadLetterQueue[]> {
  const load = useCallback((connID: number) => sqsApi.deadLetterQueues(connID), []);
  return useBrokerData(load);
}
