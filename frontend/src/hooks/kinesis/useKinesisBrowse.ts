import { useCallback, useState } from "react";
import type { MessageItem } from "@/api/models";
import { browseMessages } from "@/api/message";
import { FILTER_SHARD_ID } from "@/mq/kinesis/messages";
import { useConnectionScope } from "@/mq/ConnectionScope";
import { formatErrorMessage } from "@/lib/utils";

export interface BrowseState {
  messages: MessageItem[];
  loading: boolean;
  error: string | null;
  /** True once a browse has been run, so the empty state can say which it is. */
  searched: boolean;
}

export interface BrowseRequest {
  stream: string;
  /** Empty reads every shard, which is what a browse of a stream means. */
  shard: string;
  limit: number;
  /** Epoch milliseconds. Zero starts at the oldest record still kept. */
  startTimeMs: number;
}

/**
 * Browsing one stream.
 *
 * Not useBrokerData, and the reason is not the one SQS has. Reading a Kinesis
 * stream takes nothing: no record is hidden, consumed or marked, and any
 * number of readers can read the same one until retention expires. What a read
 * does spend is the shard's own allowance - five GetRecords a second and two
 * megabytes a second, shared with every classic consumer on it - so a hook
 * that polled would be taking read capacity from a running application for a
 * page nobody is looking at.
 */
export function useKinesisBrowse(): BrowseState & {
  run: (request: BrowseRequest) => Promise<void>;
} {
  const { id: connID } = useConnectionScope();
  const [messages, setMessages] = useState<MessageItem[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [searched, setSearched] = useState(false);

  const run = useCallback(
    async (request: BrowseRequest) => {
      const stream = request.stream.trim();
      if (stream === "") return;
      setLoading(true);
      setError(null);
      try {
        const shard = request.shard.trim();
        setMessages(
          await browseMessages(
            connID,
            stream,
            request.limit,
            shard === "" ? {} : { [FILTER_SHARD_ID]: shard },
            { startTimeMs: request.startTimeMs },
          ),
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
