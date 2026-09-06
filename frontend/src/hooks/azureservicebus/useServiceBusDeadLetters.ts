import { useCallback, useMemo, useState } from "react";
import type { MessageItem } from "@/api/models";
import { queryDLQMessages, resendMessage } from "@/api/message";
import { useConnectionScope } from "@/mq/ConnectionScope";
import { formatErrorMessage } from "@/lib/utils";
import { useServiceBusEntities } from "./useServiceBusEntities";
import { useServiceBusSubscriptions } from "./useServiceBusSubscriptions";
import { entity as readEntity } from "@/mq/azureservicebus/entities";
import { subscription as readSubscription } from "@/mq/azureservicebus/subscriptions";

/** One thing with a dead-letter store of its own. */
export interface DeadLetterStore {
  /** "orders" for a queue, "events/worker" for a subscription. */
  path: string;
  kind: "queue" | "subscription";
  /** How many it holds, or null where the service reports no counts. */
  count: number | null;
}

export interface DeadLetterState {
  /** Every queue and subscription, each of which has a dead-letter store. */
  stores: DeadLetterStore[];
  loading: boolean;
  storesError: string | null;
  messages: MessageItem[];
  reading: boolean;
  error: string | null;
  searched: boolean;
}

/**
 * The dead letters of every queue and subscription.
 *
 * There is no listing to fetch and there could not be: a $DeadLetterQueue is
 * part of its entity rather than an object of its own, so it never appears in
 * any listing. What this does instead is take the entities and subscriptions
 * that are already loaded and say that each of them has one - which is true of
 * every single one, including those that have never failed a message.
 *
 * That is the whole difference from the two hosted families before this. Their
 * dead-letter pages walk a topology to find which ordinary queues or topics
 * something else points at, and a family with no such policy anywhere has an
 * empty page. Here the page can never be empty while the namespace has an
 * entity in it, and a store with nothing in it is the ordinary case.
 */
export function useServiceBusDeadLetters(): DeadLetterState & {
  read: (path: string, limit: number) => Promise<void>;
  resend: (path: string, sequence: number) => Promise<void>;
  refresh: () => Promise<void>;
} {
  const { id: connID } = useConnectionScope();
  const entities = useServiceBusEntities();
  const subscriptions = useServiceBusSubscriptions();
  const [messages, setMessages] = useState<MessageItem[]>([]);
  const [reading, setReading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [searched, setSearched] = useState(false);

  const stores = useMemo<DeadLetterStore[]>(() => {
    const queues = (entities.data ?? [])
      .map(readEntity)
      .filter((row) => row.kind === "queue")
      .map<DeadLetterStore>((row) => ({
        path: row.name,
        kind: "queue",
        count: row.deadLetterCount,
      }));
    const subs = (subscriptions.data ?? []).map(readSubscription).map<DeadLetterStore>((row) => ({
      path: `${row.topic}/${row.name}`,
      kind: "subscription",
      count: row.deadLetterCount,
    }));
    return [...queues, ...subs];
  }, [entities.data, subscriptions.data]);

  const read = useCallback(
    async (path: string, limit: number) => {
      if (path.trim() === "") return;
      setReading(true);
      setError(null);
      try {
        // The canonical query takes a "group"; on this family that argument is
        // an entity path, because a dead letter belongs to its entity rather
        // than to whoever was reading it.
        setMessages(await queryDLQMessages(connID, path.trim(), limit));
      } catch (readError) {
        setError(formatErrorMessage(readError));
        setMessages([]);
      } finally {
        setReading(false);
        setSearched(true);
      }
    },
    [connID],
  );

  const resend = useCallback(
    async (path: string, sequence: number) => {
      // consumerGroup carries the entity path and messageId the sequence
      // number; the other two arguments are RocketMQ's and have no
      // counterpart here.
      await resendMessage(connID, path, "", "", String(sequence));
      await read(path, 100);
    },
    [connID, read],
  );

  const refresh = useCallback(async () => {
    await Promise.all([entities.refresh(), subscriptions.refresh()]);
  }, [entities, subscriptions]);

  return {
    stores,
    loading: entities.loading || subscriptions.loading,
    storesError: entities.error ?? subscriptions.error,
    messages,
    reading,
    error,
    searched,
    read,
    resend,
    refresh,
  };
}
