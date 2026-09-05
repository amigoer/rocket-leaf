/**
 * ActiveMQ's view of a canonical message.
 *
 * The keys are a contract with internal/driver/activemq/message.go.
 *
 * The two products share no browse-result key at all - Classic answers with
 * JMS header names and Artemis with its own lower-case set - and the driver
 * reduces both before anything reaches here. What survives is JMS vocabulary,
 * because that is what both are underneath.
 */
import type { MessageItem } from "@bindings/model/models";

const AttrProduct = "product";
const AttrPriority = "priority";
const AttrPersistent = "persistent";
const AttrRedelivered = "redelivered";
const AttrCorrelation = "correlationId";
const AttrReplyTo = "replyTo";
const AttrJMSType = "jmsType";
const AttrExpiration = "expiration";
const AttrProtocol = "protocol";
const AttrLargeMessage = "largeMessage";
const AttrGroupID = "groupId";
const AttrGroupSeq = "groupSeq";
const AttrTruncated = "truncated";

/**
 * The keys the driver writes for its own use, which are not user properties.
 *
 * A message's own headers and what a producer set are two different things,
 * and the canonical model has one map for both - so the board has to know
 * which is which or it shows "product: classic" as though an application had
 * set it.
 */
const DRIVER_KEYS = new Set([
  AttrProduct,
  AttrPriority,
  AttrPersistent,
  AttrRedelivered,
  AttrCorrelation,
  AttrReplyTo,
  AttrJMSType,
  AttrExpiration,
  AttrProtocol,
  AttrLargeMessage,
  AttrGroupID,
  AttrGroupSeq,
  AttrTruncated,
]);

export interface ActiveMQMessage {
  id: string;
  body: string;
  timestampMs: number;
  storeTime: string;

  product: "classic" | "artemis";
  priority: number | null;
  persistent: boolean | null;
  redelivered: boolean | null;
  correlationId: string | null;
  replyTo: string | null;
  jmsType: string | null;
  /** Epoch millis at which the broker may discard it; 0 means never. */
  expiration: number | null;
  /** Artemis only: which wire protocol the message arrived on. */
  protocol: string | null;
  largeMessage: boolean | null;
  groupId: string | null;
  groupSeq: number | null;

  /**
   * True when the body shown is not the whole message. Artemis stores a large
   * message outside the journal and browse reports the flag instead of the
   * content, so an empty body here means "not included", not "empty".
   */
  truncated: boolean;

  /** What a producer set, with the driver's own keys removed. */
  properties: Record<string, string>;
}

function text(row: MessageItem, key: string): string | null {
  const value = row.properties?.[key];
  return value == null || value === "" ? null : value;
}

function number(row: MessageItem, key: string): number | null {
  const value = text(row, key);
  if (value == null) return null;
  const parsed = Number(value);
  return Number.isFinite(parsed) ? parsed : null;
}

function bool(row: MessageItem, key: string): boolean | null {
  const value = text(row, key);
  return value == null ? null : value === "true";
}

export function message(row: MessageItem): ActiveMQMessage {
  const properties: Record<string, string> = {};
  for (const [key, value] of Object.entries(row.properties ?? {})) {
    if (value != null && !DRIVER_KEYS.has(key)) properties[key] = value;
  }

  return {
    id: row.messageId,
    body: row.body,
    timestampMs: Number(row.storeTimestamp),
    storeTime: row.storeTime,

    product: (text(row, AttrProduct) as "classic" | "artemis") ?? "classic",
    priority: number(row, AttrPriority),
    persistent: bool(row, AttrPersistent),
    redelivered: bool(row, AttrRedelivered),
    correlationId: text(row, AttrCorrelation),
    replyTo: text(row, AttrReplyTo),
    jmsType: text(row, AttrJMSType),
    expiration: number(row, AttrExpiration),
    protocol: text(row, AttrProtocol),
    largeMessage: bool(row, AttrLargeMessage),
    groupId: text(row, AttrGroupID),
    groupSeq: number(row, AttrGroupSeq),

    truncated: bool(row, AttrTruncated) === true,
    properties,
  };
}

/** A one-line preview for the list, since a body can be a whole document. */
export function summary(entry: ActiveMQMessage, limit = 120): string {
  const flat = entry.body.replace(/\s+/g, " ").trim();
  return flat.length > limit ? `${flat.slice(0, limit)}…` : flat;
}
