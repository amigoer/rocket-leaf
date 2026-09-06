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
 * Not useBrokerData, and not for the reason SQS has. An MQ browse takes
 * nothing - the queue's depth is the same afterwards and the messages stay in
 * order - so polling would not be dangerous. What it would be is expensive:
 * the driver spends one request listing the identifiers and one more per
 * message, so a page that refreshed itself would be making fifty round trips
 * at whoever left the tab open.
 */
export function useIbmMqBrowse(): BrowseState & {
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
