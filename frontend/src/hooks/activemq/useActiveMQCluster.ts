import { useCallback } from "react";
import type { ClientConnection, Node } from "@/api/models";
import * as clusterApi from "@/api/cluster";
import * as activemqApi from "@/api/activemq";
import { useBrokerData, type BrokerData } from "@/hooks/useBrokerData";

/**
 * The broker, and the brokers it bridges to.
 *
 * A broker page more than a cluster page, and that is the family rather than
 * the driver: a JMS broker is a unit. Destinations live on the one that owns
 * them, clients connect to it, and clustering here is a bridge between brokers
 * rather than nodes sharing a namespace - so the ordinary deployment is one
 * row, and the extra rows are links this broker declares.
 */
export function useActiveMQNodes(): BrokerData<Node[]> {
  const load = useCallback((connID: number) => clusterApi.getBrokers(connID), []);
  return useBrokerData(load);
}

/** The broker's effective settings, as its own management tree reports them. */
export function useActiveMQNodeConfig(address: string | null) {
  const load = useCallback(
    async (connID: number) =>
      address == null ? null : clusterApi.getNodeConfig(connID, address),
    [address],
  );
  return useBrokerData(load, { enabled: address != null, refreshMs: null });
}

/** What is holding a socket open on the broker. */
export function useActiveMQConnections(): BrokerData<ClientConnection[]> {
  const load = useCallback((connID: number) => activemqApi.connections(connID), []);
  return useBrokerData(load);
}
