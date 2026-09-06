/**
 * Service Bus's view of a canonical destination.
 *
 * The keys are a contract with internal/driver/azureservicebus/destination.go.
 *
 * One shape for two objects, because a queue and a topic are the same thing to
 * create, configure and delete. `kind` is what tells them apart, and almost
 * everything below is nullable for exactly that reason: a topic has no lock
 * duration, no delivery limit and no depth, because it holds no messages at
 * all - a send is copied into every subscription whose rules let it through
 * and discarded if none do.
 *
 * `depth` is null in a second case that is not the same one: against the
 * Service Bus emulator no entity reports a message count, so a queue's depth
 * is unknown there and a number anywhere else. The driver degrades the
 * capability with a reason rather than reporting a zero.
 */
import type { Destination } from "@bindings/model/models";

const AttrEntityType = "entityType";
const AttrStatus = "status";
const AttrLockDurationSec = "lockDurationSec";
const AttrMaxDeliveryCount = "maxDeliveryCount";
const AttrTTLSec = "ttlSec";
const AttrAutoDeleteOnIdleSec = "autoDeleteOnIdleSec";
const AttrMaxSizeMB = "maxSizeMb";
const AttrRequiresSession = "requiresSession";
const AttrRequiresDuplicateDetect = "requiresDuplicateDetection";
const AttrDeadLetterOnExpiry = "deadLetterOnExpiry";
const AttrPartitioned = "partitioned";
const AttrForwardTo = "forwardTo";
const AttrForwardDeadLettersTo = "forwardDeadLettersTo";
const AttrDeadLetterCount = "deadLetterCount";
const AttrScheduledCount = "scheduledCount";
const AttrSubscriptionNames = "subscriptionNames";

export type EntityKind = "queue" | "topic";

export interface ServiceBusEntity {
  name: string;
  /** Which of the two this is. Nothing else on the row means the same thing. */
  kind: EntityKind;

  /**
   * Active messages waiting to be delivered. Null on a topic, which holds
   * none, and null against the emulator, which reports none.
   */
  depth: number | null;
  /** What has been dead-lettered. Null wherever depth is. */
  deadLetterCount: number | null;
  /** Scheduled for later, so no consumer has been offered it yet. */
  scheduledCount: number | null;

  /** How many subscriptions read a topic. Null on a queue, which has none. */
  subscribers: number | null;
  /** Their names, capped by the driver; `subscribers` is the true count. */
  subscriptionNames: string[];

  /** Active, Disabled, SendDisabled or ReceiveDisabled. */
  status: string | null;
  /** How long a receiver holds a message before it is offered again. */
  lockDurationSec: number | null;
  /** Deliveries before a message is moved to the dead-letter queue. */
  maxDeliveryCount: number | null;
  ttlSec: number | null;
  autoDeleteOnIdleSec: number | null;
  maxSizeMb: number | null;

  requiresSession: boolean;
  requiresDuplicateDetection: boolean;
  deadLetterOnExpiry: boolean;
  partitioned: boolean;

  /** Another entity every message is forwarded to on arrival. */
  forwardTo: string | null;
  /** Another entity this one's dead letters are forwarded to. */
  forwardDeadLettersTo: string | null;
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

function flag(row: Destination, key: string): boolean {
  return text(row, key) === "true";
}

function list(row: Destination, key: string): string[] {
  const value = text(row, key);
  return value == null ? [] : value.split(",").filter((part) => part !== "");
}

export function entity(row: Destination): ServiceBusEntity {
  const kind: EntityKind = text(row, AttrEntityType) === "topic" ? "topic" : "queue";
  return {
    name: row.ref.name,
    kind,

    // The driver reports an unknown depth as -1, which is what a topic always
    // has and what every entity has against the emulator.
    depth: row.depth < 0 ? null : row.depth,
    deadLetterCount: number(row, AttrDeadLetterCount),
    scheduledCount: number(row, AttrScheduledCount),

    subscribers: kind === "topic" ? row.subscribers : null,
    subscriptionNames: list(row, AttrSubscriptionNames),

    status: text(row, AttrStatus),
    lockDurationSec: number(row, AttrLockDurationSec),
    maxDeliveryCount: number(row, AttrMaxDeliveryCount),
    ttlSec: number(row, AttrTTLSec),
    autoDeleteOnIdleSec: number(row, AttrAutoDeleteOnIdleSec),
    maxSizeMb: number(row, AttrMaxSizeMB),

    requiresSession: flag(row, AttrRequiresSession),
    requiresDuplicateDetection: flag(row, AttrRequiresDuplicateDetect),
    deadLetterOnExpiry: flag(row, AttrDeadLetterOnExpiry),
    partitioned: flag(row, AttrPartitioned),

    forwardTo: text(row, AttrForwardTo),
    forwardDeadLettersTo: text(row, AttrForwardDeadLettersTo),
  };
}

/**
 * Whether everything sent to this topic is being thrown away.
 *
 * The state to point at rather than the raw zero. A topic with no subscription
 * accepts every send, reports success, and discards the message - there is no
 * backlog anywhere afterwards and nothing else on any board says so.
 *
 * A queue is never in this state: it holds what is sent to it whether or not
 * anything is reading.
 */
export function discardsEverything(row: ServiceBusEntity): boolean {
  return row.kind === "topic" && row.subscribers === 0;
}

/** Whether this entity is refusing sends, receives, or both. */
export function isDisabled(row: ServiceBusEntity): boolean {
  return row.status != null && row.status !== "Active";
}
