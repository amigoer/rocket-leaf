import { useCallback } from "react";
import type { DeadLetterQueue } from "@bindings/model/models";
import * as activemqApi from "@/api/activemq";
import { useBrokerData, type BrokerData } from "@/hooks/useBrokerData";

/**
 * The destinations dead letters land in.
 *
 * Found by walking the declarations backwards rather than by looking up a
 * name: neither product keeps a list of its dead-letter destinations. Artemis
 * records a dead-letter address on each queue, so the set is what those point
 * at; Classic marks the destination that receives them and says nothing about
 * what fed it.
 */
export function useActiveMQDeadLetters(): BrokerData<DeadLetterQueue[]> {
  const load = useCallback((connID: number) => activemqApi.deadLetterQueues(connID), []);
  return useBrokerData(load);
}
