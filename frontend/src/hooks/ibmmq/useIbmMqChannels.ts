import { useCallback } from "react";
import type { Channel } from "@bindings/model/models";
import * as ibmmqApi from "@/api/ibmmq";
import { useBrokerData, type BrokerData } from "@/hooks/useBrokerData";

/**
 * Every channel the queue manager defines, running or not.
 *
 * Two MQSC calls inside the driver - the definitions and the status of what is
 * running - folded into one row per definition before they reach here.
 */
export function useIbmMqChannels(): BrokerData<Channel[]> {
  const load = useCallback((connID: number) => ibmmqApi.channels(connID), []);
  return useBrokerData(load);
}
