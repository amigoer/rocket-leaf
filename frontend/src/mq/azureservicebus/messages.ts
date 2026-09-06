/**
 * Service Bus's view of a canonical message.
 *
 * The keys are a contract with internal/driver/azureservicebus/message.go.
 *
 * The canonical shape is RocketMQ's, and two of its fields land better here
 * than on either hosted family before this one. `queueOffset` carries the
 * sequence number, which is a real offset: assigned in order, unique within
 * its entity, and what a browse resumes from. `tags` carries the subject,
 * which Service Bus also calls the label - not an approximation, because a
 * correlation filter matches on it by that name.
 *
 * `state` is what a peek shows and a receive could not. A scheduled message is
 * held back until its enqueue time and a deferred one has been set aside by
 * sequence number; no consumer is offered either, and a browse here sees both.
 */
import type { MessageItem } from "@bindings/model/models";

const PropState = "state";
const PropSequenceNumber = "sequenceNumber";
const PropDeliveryCount = "deliveryCount";
const PropScheduledEnqueueTime = "scheduledEnqueueTime";
const PropExpiresAt = "expiresAt";
const PropSessionID = "sessionId";
const PropPartitionKey = "partitionKey";
const PropCorrelationID = "correlationId";
const PropContentType = "contentType";
const PropDeadLetterReason = "deadLetterReason";
const PropDeadLetterDesc = "deadLetterErrorDescription";
const PropDeadLetterSource = "deadLetterSource";

/** Sender-set properties carry this prefix, so they stay apart from the rest. */
const AttributePrefix = "prop.";

export type MessageState = "active" | "deferred" | "scheduled";

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

/**
 * Which of the three states this message is in.
 *
 * The two that are not active are the reason this page exists as a peek: a
 * consumer would be offered neither, so nothing else in the app would show
 * that a scheduled message is waiting or that a deferred one was set aside.
 */
export function state(message: MessageItem): MessageState {
  const raw = property(message, PropState);
  return raw === "scheduled" || raw === "deferred" ? raw : "active";
}

/** The sequence number, which is what addresses a message here. */
export const sequenceNumber = (message: MessageItem): number | null =>
  numberProperty(message, PropSequenceNumber);

/**
 * How many times it has been handed out.
 *
 * Worth its own reader because of what it is compared against: a message is
 * moved to the dead-letter queue once this passes the entity's delivery limit.
 * A peek does not move it, which is the whole point of browsing this way.
 */
export const deliveryCount = (message: MessageItem): number | null =>
  numberProperty(message, PropDeliveryCount);

/** When a scheduled message will be enqueued, if it is one. */
export const scheduledFor = (message: MessageItem): string | null =>
  property(message, PropScheduledEnqueueTime);

export const expiresAt = (message: MessageItem): string | null =>
  property(message, PropExpiresAt);

/** What delivery is ordered against: a session, or failing that a partition. */
export const groupingKey = (message: MessageItem): string | null =>
  property(message, PropSessionID) ?? property(message, PropPartitionKey);

export const correlationId = (message: MessageItem): string | null =>
  property(message, PropCorrelationID);

export const contentType = (message: MessageItem): string | null =>
  property(message, PropContentType);

/** Why this one was given up on, on a dead letter. */
export const deadLetterReason = (message: MessageItem): string | null =>
  property(message, PropDeadLetterReason);

export const deadLetterDescription = (message: MessageItem): string | null =>
  property(message, PropDeadLetterDesc);

/** Which entity a forwarded dead letter originally came from. */
export const deadLetterSource = (message: MessageItem): string | null =>
  property(message, PropDeadLetterSource);

/** The sender's own properties, which is what a SQL rule selects on. */
export function senderProperties(message: MessageItem): [string, string][] {
  return Object.entries(message.properties ?? {})
    .filter(([key]) => key.startsWith(AttributePrefix))
    .map(([key, value]) => [key.slice(AttributePrefix.length), value] as [string, string])
    .sort(([a], [b]) => a.localeCompare(b));
}

/** Where the next page of a browse starts, or null when there is no next. */
export function nextSequence(messages: MessageItem[]): number | null {
  let highest: number | null = null;
  for (const message of messages) {
    const sequence = sequenceNumber(message);
    if (sequence != null && (highest == null || sequence > highest)) highest = sequence;
  }
  return highest == null ? null : highest + 1;
}
