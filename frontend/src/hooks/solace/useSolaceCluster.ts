import { useCallback } from "react";
import type { Node } from "@/api/models";
import * as clusterApi from "@/api/cluster";
import { useBrokerData, type BrokerData } from "@/hooks/useBrokerData";

/**
 * The broker, which is one row.
 *
 * Through the canonical cluster service rather than a Solace one, because
 * nothing about the request is this family's: a node is a node. What is this
 * family's is that there is exactly one - a redundancy pair shares a virtual
 * router and only one half answers - and that is the driver's business rather
 * than this hook's.
 */
export function useSolaceBroker(): BrokerData<Node[]> {
  const load = useCallback((connID: number) => clusterApi.getBrokers(connID), []);
  return useBrokerData(load);
}
