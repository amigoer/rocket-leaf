/**
 * NSQ's view of a connected client, in both the roles it can be in.
 *
 * The keys are a contract with internal/driver/nsq/clients.go.
 *
 * A consumer and a producer are reported by nsqd in different places and carry
 * different figures: a consumer has a channel and a ready count, a producer
 * has neither and instead reports what it has published per topic. Reading
 * them into one shape is fine as long as the page keeps them apart - a
 * producer with no ready count is not a stalled consumer.
 *
 * One kind of client is invisible and no page can fix it: anything publishing
 * over HTTP. /pub is a request rather than a connection, so nsqd has nothing
 * left to list once it has answered.
 */
import type { ClientConnection } from "@bindings/model/models";

const AttrTopic = "topic";
const AttrChannel = "channel";
const AttrReadyCount = "readyCount";
const AttrInFlight = "inFlight";
const AttrMessageCount = "messageCount";
const AttrFinishCount = "finishCount";
const AttrRequeued = "requeued";
const AttrUserAgent = "userAgent";
const AttrHostname = "hostname";
const AttrSnappy = "snappy";
const AttrNode = "node";
const AttrRole = "role";
const AttrProducerRole = "producer";
const AttrPublished = "published";

/** Which half of the picture a row is. The two carry different figures. */
export type NsqClientRole = "consumer" | "producer";

export interface NsqClient {
  /** The broker's own identifier: the peer, and the daemon it reached. */
  id: string;
  role: NsqClientRole;
  clientId: string;
  peer: string;
  /** The nsqd holding this connection. One consumer process holds one per daemon. */
  node: string;

  /** A consumer's channel; a producer's topics, which may be several. */
  topic: string;
  /** A consumer's only. A producer subscribes to nothing. */
  channel: string;
  /** A producer's only: "topic=count" per topic it has published to. */
  published: string;

  /**
   * What a consumer told nsqd it will accept. Null on a producer, which has no
   * such figure - printing a zero there would read as a stalled consumer.
   */
  ready: number | null;
  inFlight: number | null;
  messages: number | null;
  finished: number | null;
  requeued: number | null;

  userAgent: string;
  hostname: string;
  state: string;
  protocol: string;
  tls: boolean;
  snappy: boolean;
  connectedAtMs: number;
}

function text(row: ClientConnection, key: string): string {
  return row.attributes?.[key] ?? "";
}

function number(row: ClientConnection, key: string): number | null {
  const value = text(row, key);
  if (value === "") return null;
  const parsed = Number(value);
  return Number.isFinite(parsed) ? parsed : null;
}

export function client(row: ClientConnection): NsqClient {
  return {
    id: row.name,
    role: text(row, AttrRole) === AttrProducerRole ? "producer" : "consumer",
    clientId: row.clientName,
    peer: row.peerPort > 0 ? `${row.peerHost}:${row.peerPort}` : row.peerHost,
    node: text(row, AttrNode) || row.node,

    topic: text(row, AttrTopic),
    channel: text(row, AttrChannel),
    published: text(row, AttrPublished),

    ready: number(row, AttrReadyCount),
    inFlight: number(row, AttrInFlight),
    messages: number(row, AttrMessageCount),
    finished: number(row, AttrFinishCount),
    requeued: number(row, AttrRequeued),

    userAgent: text(row, AttrUserAgent),
    hostname: text(row, AttrHostname),
    state: row.state,
    protocol: row.protocol,
    tls: row.tls,
    snappy: text(row, AttrSnappy) === "true",
    connectedAtMs: row.connectedAtMs,
  };
}

/**
 * Whether this consumer is connected and asking for nothing.
 *
 * The state worth flagging: a ready count of zero means the client has told
 * nsqd not to send it anything, so its channel's backlog will not move however
 * healthy everything else looks. It is usually a consumer whose handler is
 * stuck, and nothing else in the app can see it.
 *
 * A producer is never this. It has no ready count at all, and reading its
 * absent one as a zero would flag every publisher on the cluster.
 */
export function askingForNothing(entry: NsqClient): boolean {
  return entry.role === "consumer" && entry.ready === 0;
}
