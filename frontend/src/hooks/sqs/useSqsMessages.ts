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
 * Browsing one queue.
 *
 * Not useBrokerData, and here the difference is more than a preference. A
 * browse goes through ReceiveMessage - the same call a consumer makes - so
 * every run hides what it read from real consumers for as long as it takes to
 * hand it back, and raises each message's receive count. A hook that polled
 * would do that every thirty seconds for a page nobody is looking at.
 *
 * There are no filters. SQS has no server-side selector of any kind, so
 * narrowing would mean receiving everything and discarding most of it - which
 * would hide far more messages, for far longer, than the page showed.
 */
export function useSqsBrowse(): BrowseState & {
  run: (queue: string, limit: number) => Promise<void>;
} {
  const { id: connID } = useConnectionScope();
  const [messages, setMessages] = useState<MessageItem[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [searched, setSearched] = useState(false);

  const run = useCallback(
    async (queue: string, limit: number) => {
      if (queue.trim() === "") return;
      setLoading(true);
      setError(null);
      try {
        setMessages(await fetchLatestMessages(connID, queue.trim(), limit));
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
