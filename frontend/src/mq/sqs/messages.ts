/**
 * SQS's view of a canonical message.
 *
 * The keys are a contract with internal/driver/sqs/message.go.
 *
 * The canonical shape is RocketMQ's and several of its fields have no
 * counterpart here, which is why they are read through this file rather than
 * off the record directly. There is no tag, no partition, no offset and no
 * host of any kind: a message arrives over HTTPS from whoever signed the
 * request. What SQS has instead is a receive count, and it is the field to
 * read twice - it counts every receive including this app's browse, and the
 * redrive policy compares it against maxReceiveCount.
 */
import type { MessageItem } from "@bindings/model/models";

const PropReceiveCount = "approximateReceiveCount";
const PropFirstReceivedAt = "approximateFirstReceiveTimestamp";
const PropSenderID = "senderId";
const PropGroupID = "messageGroupId";
const PropDeduplicationID = "messageDeduplicationId";
const PropSequenceNumber = "sequenceNumber";
const PropBodyMD5 = "md5OfBody";

/** Producer-set attributes carry this prefix, so they stay apart from SQS's. */
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

/** The id SQS assigned on send. It addresses nothing: no call takes one. */
export const messageIdOf = (message: MessageItem): string => message.messageId;

/** When the producer's send was accepted, in epoch milliseconds. */
export const sentAtMs = (message: MessageItem): number | null =>
  message.storeTimestamp > 0 ? message.storeTimestamp : null;

/**
 * How many times this message has been received.
 *
 * Worth its own reader because of what it is compared against: a queue with a
 * redrive policy moves a message to its dead-letter queue once this passes
 * maxReceiveCount, and a browse on this page counts towards it.
 */
export const receiveCount = (message: MessageItem): number | null =>
  numberProperty(message, PropReceiveCount);

export const firstReceivedAtMs = (message: MessageItem): number | null =>
  numberProperty(message, PropFirstReceivedAt);

/** The AWS principal that sent it, which is as close to a producer as SQS gets. */
export const senderId = (message: MessageItem): string | null =>
  property(message, PropSenderID);

/** FIFO only: what this message is ordered against. */
export const groupId = (message: MessageItem): string | null => property(message, PropGroupID);

/** FIFO only: what SQS deduplicated it by inside the five-minute window. */
export const deduplicationId = (message: MessageItem): string | null =>
  property(message, PropDeduplicationID);

/** FIFO only: the position SQS assigned within the group. */
export const sequenceNumber = (message: MessageItem): string | null =>
  property(message, PropSequenceNumber);

export const bodyMd5 = (message: MessageItem): string | null => property(message, PropBodyMD5);

/** The attributes the producer set, with the prefix stripped back off. */
export function producerAttributes(message: MessageItem): [string, string][] {
  return Object.entries(message.properties ?? {})
    .filter(([key]) => key.startsWith(AttributePrefix))
    .map(([key, value]) => [key.slice(AttributePrefix.length), value] as [string, string])
    .sort(([a], [b]) => a.localeCompare(b));
}
