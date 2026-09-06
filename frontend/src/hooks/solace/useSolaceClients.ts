import { useCallback } from "react";
import type { ClientConnection } from "@bindings/model/models";
import * as solaceApi from "@/api/solace";
import { useBrokerData, type BrokerData } from "@/hooks/useBrokerData";

/**
 * What is connected to this Message VPN.
 *
 * One request: SEMP lists a VPN's clients with everything worth knowing about
 * each of them, which is unusual - most families here need a second call per
 * connection to say anything beyond an address.
 */
export function useSolaceClients(): BrokerData<ClientConnection[]> {
  const load = useCallback((connID: number) => solaceApi.clients(connID), []);
  return useBrokerData(load);
}
