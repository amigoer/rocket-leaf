/**
 * Pub/Sub's view of a canonical message.
 *
 * The keys are a contract with internal/driver/googlepubsub/message.go.
 *
 * The canonical shape is RocketMQ's and several of its fields have no
 * counterpart here, which is why they are read through this file rather than
 * off the record directly. There is no tag, no partition, no offset and no
 * host of any kind: a message arrives over HTTPS from whoever authenticated
 * the publish. What Pub/Sub has instead is a delivery attempt, and it is the
 * field to read twice - it counts every delivery including this app's browse,
 * and a dead-letter policy compares it against maxDeliveryAttempts.
 *
 * `topic` on a record is the subscription it was read from, not a topic. That
 * is not a mistake in the mapping: a Pub/Sub topic holds nothing, so there is
 * no such thing as browsing one.
 */
import type { MessageItem } from "@bindings/model/models";

const PropDeliveryAttempt = "deliveryAttempt";
const PropOrderingKey = "orderingKey";
const PropSubscription = "subscription";

/** Publisher-set attributes carry this prefix, so they stay apart from the rest. */
const AttributePrefix = "attr.";

function property(message: MessageItem, key: string): string | null {
  const value = message.properties?.[key];
  return value == null || value === "" ? null : value;
}

function numberProperty(message: MessageItem, key: string): number | null {
  const raw = property(message, key);
  if (raw == null) return null;
  const parsed = Number(raw);
  return Number.isFinite(parsed) ? parsed : null;
}

/** The id Pub/Sub assigned on publish. It addresses nothing: no call takes one. */
export const messageIdOf = (message: MessageItem): string => message.messageId;

/** When the publish was accepted, in epoch milliseconds. */
export const publishedAtMs = (message: MessageItem): number | null =>
  message.storeTimestamp > 0 ? message.storeTimestamp : null;

/**
 * How many times this message has been delivered.
 *
 * Worth its own reader because of what it is compared against: a subscription
 * with a dead-letter policy moves a message to its dead-letter topic once this
 * passes maxDeliveryAttempts, and a browse on this page counts towards it.
 * Null on a subscription with no such policy - Pub/Sub does not track it then.
 */
export const deliveryAttempt = (message: MessageItem): number | null =>
  numberProperty(message, PropDeliveryAttempt);

/** What the message is ordered against, on a subscription with ordering on. */
export const orderingKey = (message: MessageItem): string | null =>
  property(message, PropOrderingKey);

/** The subscription it was read from, which is what "topic" holds here. */
export const readFrom = (message: MessageItem): string =>
  property(message, PropSubscription) ?? message.topic;

/** The attributes the publisher set, with the prefix stripped back off. */
export function publisherAttributes(message: MessageItem): [string, string][] {
  return Object.entries(message.properties ?? {})
    .filter(([key]) => key.startsWith(AttributePrefix))
    .map(([key, value]) => [key.slice(AttributePrefix.length), value] as [string, string])
    .sort(([a], [b]) => a.localeCompare(b));
}
