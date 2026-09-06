import { useCallback } from "react";
import type { Subscription } from "@/api/models";
import * as consumerApi from "@/api/consumer";
import { useBrokerData, type BrokerData } from "@/hooks/useBrokerData";

/**
 * Every registered consumer in the connection's region.
 *
 * One request per board here, one per stream inside the driver: Kinesis lists
 * a single stream's consumers at a time and has no account-wide call, so the
 * driver fans the listing out and folds it before it reaches this hook.
 */
export function useKinesisConsumers(): BrokerData<Subscription[]> {
  const load = useCallback((connID: number) => consumerApi.getConsumerGroups(connID), []);
  return useBrokerData(load);
}
