import { useCallback } from "react";
import type { MessageItem } from "@/api/models";
import * as messageApi from "@/api/message";
import { useBrokerData, type BrokerData } from "@/hooks/useBrokerData";

/**
 * One destination's messages, browsed rather than consumed.
 *
 * Browsing is a management operation on both products, so this takes nothing
 * off the destination - unlike RabbitMQ, where the same page carries a caveat
 * because basic.get alters the queue even when what it read is put back.
 *
 * Refreshing is manual. A browse of a deep destination is not cheap on either
 * broker, and a page that re-read it every few seconds would be a load nobody
 * asked for on a queue nobody was watching change.
 */
export function useActiveMQMessages(
  destination: string | null,
  limit: number,
): BrokerData<MessageItem[]> {
  const load = useCallback(
    async (connID: number) =>
      destination == null ? [] : messageApi.fetchLatestMessages(connID, destination, limit),
    [destination, limit],
  );
  return useBrokerData(load, { enabled: destination != null, refreshMs: null });
}
