import { useCallback } from "react";
import type { ClientConnection, ClusterView } from "@/api/models";
import * as clusterApi from "@/api/cluster";
import * as nsqApi from "@/api/nsq";
import { useBrokerData, type BrokerData } from "@/hooks/useBrokerData";

/**
 * The daemons, the counters over them, and the discovery tier beside them.
 *
 * One call for all three, which is what the canonical cluster view is for. The
 * directory is separate from the node list rather than folded into it because
 * the two answer different questions: the nodes are where messages are, and
 * the tier is what a consumer is told when it asks - and the two disagreeing
 * is the failure this page exists to make visible.
 */
export function useNsqCluster(): BrokerData<ClusterView> {
  const load = useCallback((connID: number) => clusterApi.getClusterView(connID), []);
  return useBrokerData(load);
}

/** One daemon's effective settings, as it reports them itself. */
export function useNsqNodeConfig(address: string | null) {
  const load = useCallback(
    async (connID: number) => (address == null ? null : clusterApi.getNodeConfig(connID, address)),
    [address],
  );
  return useBrokerData(load, { enabled: address != null, refreshMs: null });
}

/** Every consumer holding a subscription open on the cluster. */
export function useNsqConnections(): BrokerData<ClientConnection[]> {
  const load = useCallback((connID: number) => nsqApi.connections(connID), []);
  return useBrokerData(load);
}
