import { useCallback, useState } from "react";
import type { MessageItem } from "@/api/models";
import { browseMessages } from "@/api/message";
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
 * Not useBrokerData, and not for the reason SQS has. A SEMP browse takes
 * nothing at all - the queue's depth, its spool usage and its delivery
 * counters are identical afterwards - so polling would harm nothing. What it
 * would be is pointless traffic: the list is metadata, it changes only when
 * the queue does, and a page that refreshed itself on a timer would be reading
 * a management API on behalf of a tab nobody is looking at.
 */
export function useSolaceBrowse(): BrowseState & {
  run: (queue: string, limit: number) => Promise<void>;
} {
  const { id: connID } = useConnectionScope();
  const [messages, setMessages] = useState<MessageItem[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [searched, setSearched] = useState(false);

  const run = useCallback(
    async (queue: string, limit: number) => {
      const name = queue.trim();
      if (name === "") return;
      setLoading(true);
      setError(null);
      try {
        setMessages(await browseMessages(connID, name, limit, {}));
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
