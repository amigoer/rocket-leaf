import { useCallback } from "react";
import * as nsqApi from "@/api/nsq";
import { useBrokerData, type BrokerData } from "@/hooks/useBrokerData";

/**
 * The daemons this connection can address, as the profile names them.
 *
 * Read off the open connection rather than from nsqlookupd, and the difference
 * matters for a send: the discovery tier reports whatever address each nsqd
 * broadcast about itself, which is not necessarily one this machine can reach.
 */
export function useNsqNodes(): BrokerData<string[]> {
  const load = useCallback((connID: number) => nsqApi.nodes(connID), []);
  return useBrokerData(load);
}
