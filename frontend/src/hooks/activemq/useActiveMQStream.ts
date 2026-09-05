import { useCallback, useEffect, useRef, useState } from "react";
import * as activemqApi from "@/api/activemq";
import type { ActiveMQSubscribeInput } from "@/api/activemq";
import type { LiveMessage } from "@/api/models";
import { useConnectionScope } from "@/mq/ConnectionScope";
import { formatErrorMessage } from "@/lib/utils";

/** How often the renderer drains the buffer the driver is filling. */
const POLL_MS = 400;

/** How many messages the panel holds. The driver's ring bounds its own. */
const KEEP = 500;

export interface ActiveMQStream {
  /** Newest last, the order the stream appends in. */
  messages: LiveMessage[];
  /**
   * The topics the running stream attached to, so a panel that remounted onto
   * an existing one shows what it is watching rather than its own empty box.
   */
  topics: string[];
  running: boolean;
  /**
   * False once the AMQP session behind the stream dropped. The subscription
   * survives - the panel can restart it - so this separates "nothing is being
   * published" from "we stopped listening", which are the same empty page.
   */
  live: boolean;
  received: number;
  /**
   * Messages lost because the buffer was full. A running total, and the reason
   * it is shown at all: a stream quietly losing messages and a quiet stream
   * look identical without it.
   */
  dropped: number;
  error: string | null;
  start: (input: ActiveMQSubscribeInput) => void;
  stop: () => void;
  clear: () => void;
}

/**
 * A live view of one or more topics, drained on a timer.
 *
 * A poll rather than a push, because the buffer it drains is on the Go side:
 * the broker pushes into a bounded ring per subscription and this drains it by
 * sequence. Sending every message across the bridge would put a busy topic's
 * whole firehose through it for a panel showing the last few hundred lines.
 *
 * The stream outlives the panel, deliberately. Sending on one page and
 * watching it arrive on this one is the ordinary way to use both, and a
 * subscription stopped on unmount would always be gone before the message
 * existed. Nothing here is stored, so coming back could not recover it. The
 * lifetime is the user's instead: a stream runs until they stop it or the
 * connection closes, and closing the connection detaches it on the broker.
 */
export function useActiveMQStream(): ActiveMQStream {
  const { id: connID } = useConnectionScope();
  const [messages, setMessages] = useState<LiveMessage[]>([]);
  const [topics, setTopics] = useState<string[]>([]);
  const [running, setRunning] = useState(false);
  const [live, setLive] = useState(true);
  const [received, setReceived] = useState(0);
  const [dropped, setDropped] = useState(0);
  const [error, setError] = useState<string | null>(null);

  // Refs rather than state: the poll loop reads them every tick and must not
  // be torn down and rebuilt for a cursor that moves several times a second.
  const subscription = useRef<string | null>(null);
  const cursor = useRef(0);
  const connection = useRef(connID);
  connection.current = connID;

  const stop = useCallback(() => {
    const id = subscription.current;
    subscription.current = null;
    setRunning(false);
    setTopics([]);
    if (id == null || connection.current === 0) return;
    // Fire and forget: the panel has already stopped reading, and a failed
    // detach is the broker's to notice when the connection closes.
    void activemqApi.stopSubscription(connection.current, id).catch(() => {});
  }, []);

  const start = useCallback((input: ActiveMQSubscribeInput) => {
    const id = connection.current;
    if (id === 0) return;

    // One stream per panel. Starting a second without stopping the first would
    // leave a receiver attached to the broker with nothing reading it.
    const previous = subscription.current;
    if (previous != null) {
      subscription.current = null;
      void activemqApi.stopSubscription(id, previous).catch(() => {});
    }

    setError(null);
    setMessages([]);
    setReceived(0);
    setDropped(0);
    setLive(true);
    cursor.current = 0;

    void activemqApi
      .startSubscription(id, input)
      .then((started) => {
        subscription.current = started.id;
        setTopics((started.filters ?? []).map((filter) => filter?.pattern ?? ""));
        setRunning(true);
      })
      .catch((cause: unknown) => {
        setError(formatErrorMessage(cause));
        setRunning(false);
      });
  }, []);

  const clear = useCallback(() => setMessages([]), []);

  useEffect(() => {
    if (!running) return;

    let cancelled = false;
    const timer = window.setInterval(() => {
      const id = subscription.current;
      const conn = connection.current;
      if (id == null || conn === 0) return;

      void activemqApi
        .pollSubscription(conn, id, cursor.current, KEEP)
        .then((batch) => {
          if (cancelled) return;
          cursor.current = batch.cursor;
          setReceived(batch.received);
          setDropped(batch.dropped);
          setLive(batch.live);
          const arrived = (batch.messages ?? []).filter(
            (message): message is LiveMessage => message != null,
          );
          if (arrived.length === 0) return;
          // Trimmed here as well as in the driver: the ring bounds what the Go
          // side holds between polls, and this bounds what one panel keeps
          // across a session that may run for hours.
          setMessages((held) => [...held, ...arrived].slice(-KEEP));
        })
        .catch((cause: unknown) => {
          if (cancelled) return;
          setError(formatErrorMessage(cause));
          setRunning(false);
        });
    }, POLL_MS);

    return () => {
      cancelled = true;
      window.clearInterval(timer);
    };
  }, [running]);

  /* On mount, adopt whatever is already running on this connection. */
  useEffect(() => {
    const id = connection.current;
    if (id === 0 || subscription.current != null) return;

    let cancelled = false;
    void activemqApi
      .subscriptions(id)
      .then((existing) => {
        const adopted = existing[0];
        if (cancelled || adopted == null) return;
        subscription.current = adopted.id;
        cursor.current = 0;
        setTopics((adopted.filters ?? []).map((filter) => filter?.pattern ?? ""));
        setRunning(true);
      })
      .catch(() => {
        // A connection that cannot list its streams has none to adopt, and the
        // panel is perfectly usable starting a new one.
      });
    return () => {
      cancelled = true;
    };
  }, [connID]);

  return { messages, topics, running, live, received, dropped, error, start, stop, clear };
}
