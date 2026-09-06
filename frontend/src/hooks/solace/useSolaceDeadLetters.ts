import { useCallback } from "react";
import type { DeadLetterQueue } from "@bindings/model/models";
import * as solaceApi from "@/api/solace";
import { useBrokerData, type BrokerData } from "@/hooks/useBrokerData";

/**
 * The queues something else dead-letters into.
 *
 * One request from here; rather more inside the driver, because finding a dead
 * message queue means reading every queue and every topic endpoint in the
 * Message VPN and inverting their pointers. Nothing on a queue says it is one.
 */
export function useSolaceDeadLetters(): BrokerData<DeadLetterQueue[]> {
  const load = useCallback((connID: number) => solaceApi.deadMsgQueues(connID), []);
  return useBrokerData(load);
}
