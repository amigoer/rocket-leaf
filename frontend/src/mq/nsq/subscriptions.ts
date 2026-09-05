/**
 * NSQ's view of a canonical subscription.
 *
 * The keys are a contract with internal/driver/nsq/subscription.go.
 *
 * A channel is this family's consumer group: every channel under a topic gets
 * a copy of every message, and its depth is the backlog. There is no offset
 * behind that depth, which is why nothing here reads or writes a position -
 * the only ways to change a channel's backlog are to consume it or to empty
 * it.
 */
import type { Subscription } from "@bindings/model/models";

const AttrTopic = "topic";
const AttrPaused = "paused";
const AttrInFlight = "inFlight";
const AttrDeferred = "deferred";
const AttrRequeued = "requeued";
const AttrTimedOut = "timedOut";
const AttrMessageCount = "messageCount";
const AttrBackendDepth = "backendDepth";
const AttrEphemeral = "ephemeral";
const AttrNodes = "nodes";

export interface NsqChannel {
  /** The topic is half the identity: the same name under two topics is two. */
  topic: string;
  name: string;

  /** Published and not yet finished. The only lag this family has. */
  backlog: number | null;
  /** The part of it that has spilled out of memory onto disk. */
  backendDepth: number | null;

  /** Consumers connected right now. Zero is normal: a channel is durable. */
  clients: number;

  /** Handed to a consumer and not yet finished, so not counted in the backlog. */
  inFlight: number | null;
  /** Waiting for a delivery time, and likewise outside the backlog. */
  deferred: number | null;
  /** Handed back by a consumer, or taken back when its timeout expired. */
  requeued: number | null;
  timedOut: number | null;
  /** Since the daemon started, not since the channel was created. */
  messages: number | null;

  /** Paused anywhere. Consumers stay connected and receive nothing. */
  paused: boolean;
  /** Deleted when its last consumer disconnects, backlog and all. */
  ephemeral: boolean;
  nodes: string[];
}

function text(row: Subscription, key: string): string | null {
  const value = row.attributes?.[key];
  return value == null || value === "" ? null : value;
}

function number(row: Subscription, key: string): number | null {
  const value = text(row, key);
  if (value == null) return null;
  const parsed = Number(value);
  return Number.isFinite(parsed) ? parsed : null;
}

/** UnknownMetric is -1 on the wire, and it means "no such figure here". */
function metric(value: number): number | null {
  return value < 0 ? null : value;
}

export function channel(row: Subscription): NsqChannel {
  return {
    topic: text(row, AttrTopic) ?? row.ref.namespace,
    name: row.ref.name,

    backlog: metric(Number(row.backlog)),
    backendDepth: number(row, AttrBackendDepth),

    clients: row.members,

    inFlight: number(row, AttrInFlight),
    deferred: number(row, AttrDeferred),
    requeued: number(row, AttrRequeued),
    timedOut: number(row, AttrTimedOut),
    messages: number(row, AttrMessageCount),

    paused: text(row, AttrPaused) === "true",
    ephemeral: text(row, AttrEphemeral) === "true",
    nodes: (text(row, AttrNodes) ?? "").split(",").filter((entry) => entry !== ""),
  };
}

/** A channel's identity as one string, for a list key and a selection. */
export function channelKey(entry: { topic: string; name: string }): string {
  return `${entry.topic}/${entry.name}`;
}

/**
 * Whether this channel's backlog is nobody's fault but the pause.
 *
 * The state worth naming on the board: the consumers are connected, they are
 * asking for messages, and nsqd is sending none. Every other explanation for a
 * backlog has a consumer to look at.
 */
export function stalledByPause(entry: NsqChannel): boolean {
  return entry.paused && (entry.backlog ?? 0) > 0;
}
