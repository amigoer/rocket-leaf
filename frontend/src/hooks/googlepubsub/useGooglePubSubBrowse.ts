import { useCallback, useState } from "react";
import type { MessageItem } from "@/api/models";
import { fetchLatestMessages } from "@/api/message";
import { useConnectionScope } from "@/mq/ConnectionScope";
import { formatErrorMessage } from "@/lib/utils";

export interface BrowseState {
  messages: MessageItem[];
  loading: boolean;
  error: string | null;
  /** True once a browse has been run, so the empty state can say which it is. */
  searched: boolean;
}

/**
 * Browsing one subscription.
 *
 * Not useBrokerData, and here the difference is more than a preference. A
 * browse goes through Pull - the same call a consumer makes - so every run
 * holds what it read away from real consumers until the driver hands it back,
 * and raises each message's delivery attempt for good. A hook that polled
 * would do that every thirty seconds for a page nobody is looking at.
 *
 * There are no filters. A subscription's filter is fixed at creation and there
 * is no per-request selector of any kind, so narrowing would mean pulling
 * everything and discarding most of it - which would hide far more messages,
 * for far longer, than the page showed.
 */
export function useGooglePubSubBrowse(): BrowseState & {
  run: (subscription: string, limit: number) => Promise<void>;
} {
  const { id: connID } = useConnectionScope();
  const [messages, setMessages] = useState<MessageItem[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [searched, setSearched] = useState(false);

  const run = useCallback(
    async (subscription: string, limit: number) => {
      if (subscription.trim() === "") return;
      setLoading(true);
      setError(null);
      try {
        // The canonical query takes a "topic"; on this family that argument
        // is a subscription name, because a topic holds nothing to read.
        setMessages(await fetchLatestMessages(connID, subscription.trim(), limit));
      } catch (browseError) {
        setError(formatErrorMessage(browseError));
        setMessages([]);
      } finally {
        setLoading(false);
        setSearched(true);
      }
    },
    [connID],
  );

  return { messages, loading, error, searched, run };
}
