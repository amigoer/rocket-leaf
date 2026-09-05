/**
 * NSQ's view of a connected consumer.
 *
 * The keys are a contract with internal/driver/nsq/clients.go.
 *
 * Consumers only, and the page has to say so. There is no connection list in
 * NSQ: a client appears in the stats of the channel it subscribed to and
 * nowhere else, so a connection that has not subscribed yet is invisible and a
 * producer is invisible always.
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

export interface NsqClient {
  /** The broker's own identifier: the peer, and the daemon it reached. */
  id: string;
  clientId: string;
  peer: string;
  /** The nsqd holding this connection. One consumer process holds one per daemon. */
  node: string;

  topic: string;
  channel: string;

  /** What this consumer told nsqd it will accept. Zero means it is asking for nothing. */
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
    clientId: row.clientName,
    peer: row.peerPort > 0 ? `${row.peerHost}:${row.peerPort}` : row.peerHost,
    node: text(row, AttrNode) || row.node,

    topic: text(row, AttrTopic),
    channel: text(row, AttrChannel),

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
 */
export function askingForNothing(entry: NsqClient): boolean {
  return entry.ready === 0;
}
