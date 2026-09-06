/**
 * IBM MQ's view of a canonical message.
 *
 * The keys are a contract with internal/driver/ibmmq/message.go.
 *
 * The canonical shape is RocketMQ's and a good deal of it has no counterpart
 * here. There is no tag, no key index and no store host; more surprisingly
 * there is no put time either, and that is the interface rather than the
 * broker: mqweb returns the identifiers, the persistence and the expiry in
 * response headers and does not return PutDate or PutTime at all. The driver
 * leaves the timestamp empty rather than filling it with the clock on this
 * machine, so this file has nothing to read for it.
 *
 * `bodyUnavailable` is the field worth reading twice. The messaging interface
 * carries character data and nothing else, so a message stored in any other
 * format is listed with its identifier and refused when opened - which is the
 * ordinary state of every dead letter. Its value is the format that stopped
 * it, and that is the useful half.
 */
import type { MessageItem } from "@bindings/model/models";

const PropFormat = "format";
const PropCorrelationID = "correlationId";
const PropPersistence = "persistence";
const PropExpiry = "expiry";
const PropReplyToQueue = "replyToQueue";
const PropReplyToQmgr = "replyToQueueManager";
const PropBodyUnavailable = "bodyUnavailable";

/** Descriptor fields this app knows about, so the rest can be shown as-is. */
const DESCRIPTOR: ReadonlySet<string> = new Set([
  PropFormat,
  PropCorrelationID,
  PropPersistence,
  PropExpiry,
  PropReplyToQueue,
  PropReplyToQmgr,
  PropBodyUnavailable,
]);

function property(message: MessageItem, key: string): string | null {
  const value = message.properties?.[key];
  return value == null || value === "" ? null : value;
}

/** MQ's own 24-byte MsgId, spelled as 48 hexadecimal characters. */
export const messageIdOf = (message: MessageItem): string => message.messageId;

/** How the queue manager stored it: MQSTR for text, MQDEAD for a dead letter. */
export const formatOf = (message: MessageItem): string | null =>
  property(message, PropFormat);

/** Set by whoever wants a reply matched to a request. Often all zeros. */
export const correlationIdOf = (message: MessageItem): string | null =>
  property(message, PropCorrelationID);

export const persistenceOf = (message: MessageItem): string | null =>
  property(message, PropPersistence);

/** "unlimited", or tenths of a second left. It is the broker's own spelling. */
export const expiryOf = (message: MessageItem): string | null =>
  property(message, PropExpiry);

export const replyToOf = (message: MessageItem): string | null =>
  property(message, PropReplyToQueue);

export const replyToQmgrOf = (message: MessageItem): string | null =>
  property(message, PropReplyToQmgr);

/**
 * The format that stopped the server returning this body, or null when it
 * returned one.
 *
 * A row with this set is not a failure to hide: the message is on the queue,
 * its identifier is real, and the only thing missing is the payload.
 */
export const bodyUnavailableAs = (message: MessageItem): string | null =>
  property(message, PropBodyUnavailable);

/**
 * Whatever an application put on the message itself.
 *
 * They arrive under their own names, so the descriptor fields above are
 * subtracted rather than the application ones being listed - a driver that
 * enumerated them would have to be edited every time MQ grew a header.
 */
export function userProperties(message: MessageItem): [string, string][] {
  const entries: [string, string][] = [];
  for (const [key, value] of Object.entries(message.properties ?? {})) {
    if (DESCRIPTOR.has(key) || value == null || value === "") continue;
    entries.push([key, value]);
  }
  return entries.sort(([left], [right]) => left.localeCompare(right));
}
