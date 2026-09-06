/**
 * Solace's view of a canonical message.
 *
 * The keys are a contract with internal/driver/solace/message.go.
 *
 * The canonical shape is RocketMQ's and most of it has no counterpart here.
 * There is no tag, no key index, no store host - and, unlike every other
 * family the app browses, no body. SEMP's message collection is metadata and
 * nothing else: an id, a spooled time, two sizes, a redelivery count and a few
 * flags. The broker's own manager shows a payload by opening a browser flow
 * over the messaging protocol, which is a wire client this app does not have.
 *
 * So the sizes below are not decoration. They are what the list shows in the
 * column where every other family shows a preview, and they are the only way
 * to tell a message with a payload from one carrying only properties.
 */
import type { MessageItem } from "@bindings/model/models";

const PropAttachmentSize = "attachmentSize";
const PropContentSize = "contentSize";
const PropRedelivery = "redeliveryCount";
const PropUndelivered = "undelivered";
const PropDmqEligible = "dmqEligible";
const PropPartitionKey = "partitionKey";
const PropPublisherID = "publisherId";
const PropReplicationID = "replicationGroupMsgId";
const PropReplication = "replicationState";
const PropSpooledTime = "spooledTime";

/** Properties this file reads, so anything else can be shown as-is. */
const KNOWN: ReadonlySet<string> = new Set([
  PropAttachmentSize,
  PropContentSize,
  PropRedelivery,
  PropUndelivered,
  PropDmqEligible,
  PropPartitionKey,
  PropPublisherID,
  PropReplicationID,
  PropReplication,
  PropSpooledTime,
]);

function property(message: MessageItem, key: string): string | null {
  const value = message.properties?.[key];
  return value == null || value === "" ? null : value;
}

function numberProperty(message: MessageItem, key: string): number | null {
  const value = property(message, key);
  if (value == null) return null;
  const parsed = Number(value);
  return Number.isFinite(parsed) ? parsed : null;
}

function flagProperty(message: MessageItem, key: string): boolean {
  return property(message, key) === "true";
}

/** The queue's own sequence number. It is not unique across queues. */
export const messageIdOf = (message: MessageItem): string => message.messageId;

/** The binary payload's size. Zero means the message carries only properties. */
export const attachmentSizeOf = (message: MessageItem): number | null =>
  numberProperty(message, PropAttachmentSize);

/** The structured-container payload's size, which most messages leave at zero. */
export const contentSizeOf = (message: MessageItem): number | null =>
  numberProperty(message, PropContentSize);

/** How many times delivery has been attempted and not acknowledged. */
export const redeliveryCountOf = (message: MessageItem): number | null =>
  numberProperty(message, PropRedelivery);

/** True while nothing has taken it yet, which is the ordinary state of a backlog. */
export const undelivered = (message: MessageItem): boolean =>
  flagProperty(message, PropUndelivered);

/**
 * Whether this message would be moved rather than discarded when it is given
 * up on.
 *
 * Worth showing, and easy to be surprised by: the flag is set by the
 * publisher, most clients leave it off, and a queue that respects it then
 * discards quietly instead of dead-lettering. A queue with respectDmqEligible
 * turned off ignores this and moves everything.
 */
export const dmqEligible = (message: MessageItem): boolean =>
  flagProperty(message, PropDmqEligible);

/** What the message was hashed on for a partitioned queue, if anything. */
export const partitionKeyOf = (message: MessageItem): string | null =>
  property(message, PropPartitionKey);

/** The broker-wide id, which unlike the message id survives replication. */
export const replicationIdOf = (message: MessageItem): string | null =>
  property(message, PropReplicationID);

export const replicationStateOf = (message: MessageItem): string | null =>
  property(message, PropReplication);

export const publisherIdOf = (message: MessageItem): string | null =>
  property(message, PropPublisherID);

/** Whatever the driver sent that this file does not name, for the detail panel. */
export function otherProperties(message: MessageItem): [string, string][] {
  return Object.entries(message.properties ?? {})
    .flatMap<[string, string]>(([key, value]) =>
      KNOWN.has(key) || value == null || value === "" ? [] : [[key, value]],
    )
    .sort(([left], [right]) => left.localeCompare(right));
}
