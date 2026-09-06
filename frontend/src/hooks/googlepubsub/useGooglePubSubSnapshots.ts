import { useCallback } from "react";
import * as pubsubApi from "@/api/googlepubsub";
import { useBrokerData, type BrokerData } from "@/hooks/useBrokerData";

/**
 * The restore points the project holds.
 *
 * A snapshot is one subscription's acknowledgement state at a moment, and it
 * belongs to the topic rather than to the subscription it came from - so any
 * subscription on the same topic can be sought to it. Until it is deleted or
 * expires, the topic keeps everything the snapshot could restore.
 */
export function useGooglePubSubSnapshots(): BrokerData<pubsubApi.GooglePubSubSnapshot[]> {
  const load = useCallback((connID: number) => pubsubApi.listSnapshots(connID), []);
  return useBrokerData(load);
}
