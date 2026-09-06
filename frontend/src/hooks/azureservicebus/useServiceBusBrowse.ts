import { useCallback, useState } from "react";
import type { MessageItem } from "@/api/models";
import { browseMessages } from "@/api/message";
import { useConnectionScope } from "@/mq/ConnectionScope";
import { formatErrorMessage } from "@/lib/utils";

export interface BrowseTarget {
  /** The queue, or the topic a subscription belongs to. */
  entity: string;
  /** Empty when the entity is a queue and is read directly. */
  subscription: string;
  /** Read the entity's $DeadLetterQueue rather than the entity itself. */
  deadLetters: boolean;
  /** Sequence number to start from. Zero is the beginning. */
  fromSequence: number;
  limit: number;
}

export interface BrowseState {
  messages: MessageItem[];
  loading: boolean;
  error: string | null;
  /** True once a browse has been run, so the empty state can say which it is. */
  searched: boolean;
}

/**
 * Browsing one queue, subscription or dead-letter store.
 *
 * Not useBrokerData, and the reason is the opposite of the two hosted families
 * before it. Theirs could not poll because their only read was a consumer's
 * read; a peek takes nothing at all, so polling would be safe - it is simply
 * not what a browse is for. A page that re-read every thirty seconds would
 * scroll under the reader while they were looking at a message.
 *
 * The sequence number is what makes a browse repeatable. A receiver keeps a
 * cursor that advances with every peek, so a second call with no starting
 * position returns what follows the first rather than the same page.
 */
export function useServiceBusBrowse(): BrowseState & {
  run: (target: BrowseTarget) => Promise<void>;
} {
  const { id: connID } = useConnectionScope();
  const [messages, setMessages] = useState<MessageItem[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [searched, setSearched] = useState(false);

  const run = useCallback(
    async (target: BrowseTarget) => {
      const entity = target.entity.trim();
      if (entity === "") return;
      setLoading(true);
      setError(null);
      try {
        setMessages(
          await browseMessages(connID, entity, target.limit, {
            subscription: target.subscription.trim(),
            deadLetters: target.deadLetters ? "true" : "",
            fromSequence: String(target.fromSequence),
          }),
        );
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
