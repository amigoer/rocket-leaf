/**
 * NSQ's view of a canonical destination.
 *
 * The keys are a contract with internal/driver/nsq/destination.go.
 *
 * The two depth readings are the ones worth understanding. `depth` is the
 * canonical figure - everything nsqd is holding on this topic's account - and
 * it is the sum of `topicDepth` and `channelDepth`, which are different
 * things. A message published to a topic with two channels is copied into
 * both, so a topic can hold more messages than were ever published to it; and
 * a topic depth above zero means nothing has been copied out at all, which in
 * practice means the topic is paused or has no channel yet.
 */
import type { Destination } from "@bindings/model/models";

const AttrPaused = "paused";
const AttrTopicDepth = "topicDepth";
const AttrChannelDepth = "channelDepth";
const AttrBackendDepth = "backendDepth";
const AttrMessageCount = "messageCount";
const AttrMessageBytes = "messageBytes";
const AttrInFlight = "inFlight";
const AttrDeferred = "deferred";
const AttrRequeued = "requeued";
const AttrTimedOut = "timedOut";
const AttrEphemeral = "ephemeral";
const AttrNodes = "nodes";
const AttrChannels = "channels";

export interface NsqTopic {
  name: string;

  /** Everything nsqd holds for this topic: the topic's own plus its channels'. */
  depth: number | null;
  /** Not yet copied into any channel. Non-zero means paused or unconsumed. */
  topicDepth: number | null;
  /** Published and not yet finished, summed over the channels. */
  channelDepth: number | null;
  /** The part that has spilled out of memory onto disk. */
  backendDepth: number | null;

  /** Channels, which are this family's consumer groups. */
  channels: string[];
  /** Since the daemon started, not since the topic was created. */
  messages: number | null;
  bytes: number | null;

  /** Handed to a consumer and not yet finished. */
  inFlight: number | null;
  /** Accepted and waiting for a delivery time. */
  deferred: number | null;
  requeued: number | null;
  timedOut: number | null;

  /** Paused anywhere. Delivery to channels stops; publishing does not. */
  paused: boolean;
  /** Exists only while something is connected; nsqd deletes it after. */
  ephemeral: boolean;
  /** Which nsqd are carrying it. A topic lives on the daemon it was made on. */
  nodes: string[];
}

function text(row: Destination, key: string): string | null {
  const value = row.attributes?.[key];
  return value == null || value === "" ? null : value;
}

function number(row: Destination, key: string): number | null {
  const value = text(row, key);
  if (value == null) return null;
  const parsed = Number(value);
  return Number.isFinite(parsed) ? parsed : null;
}

function list(row: Destination, key: string): string[] {
  const value = text(row, key);
  return value == null ? [] : value.split(",");
}

/** UnknownMetric is -1 on the wire, and it means "no such figure here". */
function metric(value: number): number | null {
  return value < 0 ? null : value;
}

export function topic(row: Destination): NsqTopic {
  return {
    name: row.ref.name,

    depth: metric(Number(row.depth)),
    topicDepth: number(row, AttrTopicDepth),
    channelDepth: number(row, AttrChannelDepth),
    backendDepth: number(row, AttrBackendDepth),

    channels: list(row, AttrChannels),
    messages: number(row, AttrMessageCount),
    bytes: number(row, AttrMessageBytes),

    inFlight: number(row, AttrInFlight),
    deferred: number(row, AttrDeferred),
    requeued: number(row, AttrRequeued),
    timedOut: number(row, AttrTimedOut),

    paused: text(row, AttrPaused) === "true",
    ephemeral: text(row, AttrEphemeral) === "true",
    nodes: list(row, AttrNodes),
  };
}

/**
 * Whether this topic is holding messages no channel has taken a copy of.
 *
 * The state to point at rather than the flag: a paused topic is one cause and
 * a topic with no channels is the other, and both look like a healthy empty
 * topic on a board that only adds up channel depths.
 */
export function holdingUndelivered(entry: NsqTopic): boolean {
  return entry.topicDepth != null && entry.topicDepth > 0;
}
