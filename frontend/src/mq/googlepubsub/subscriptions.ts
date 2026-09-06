/**
 * Pub/Sub's view of a canonical subscription.
 *
 * The keys are a contract with internal/driver/googlepubsub/subscription.go.
 *
 * This is the object the family adds over the one before it. An SQS queue was
 * one thing: it held messages and a consumer was whoever asked for them. Here
 * the subscription is its own object - created, listed and deleted on its own,
 * outliving its topic, carrying the whole of the delivery configuration - and
 * two subscriptions on one topic each receive every message and each fall
 * behind separately.
 *
 * `backlog` is deliberately absent from this shape rather than nullable. The
 * figure exists as num_undelivered_messages, a Cloud Monitoring metric, and
 * there is no call anywhere in the Pub/Sub API that reports it - so the driver
 * degrades the capability with a reason and a board that drew a column would
 * be drawing dashes forever.
 */
import type { Subscription } from "@bindings/model/models";

const AttrPath = "path";
const AttrTopic = "topic";
const AttrAckDeadlineSec = "ackDeadlineSec";
const AttrRetentionSec = "retentionSec";
const AttrRetainAcked = "retainAcked";
const AttrExpirationSec = "expirationTtlSec";
const AttrFilter = "filter";
const AttrOrdering = "messageOrdering";
const AttrExactlyOnce = "exactlyOnce";
const AttrDetached = "detached";
const AttrState = "state";
const AttrDelivery = "delivery";
const AttrPushEndpoint = "pushEndpoint";
const AttrDeadLetterTopic = "deadLetterTopic";
const AttrMaxAttempts = "maxDeliveryAttempts";
const AttrRetryMinSec = "retryMinBackoffSec";
const AttrRetryMaxSec = "retryMaxBackoffSec";
const LabelPrefix = "label.";

/** The service's own marker for a subscription whose topic has been deleted. */
export const DELETED_TOPIC = "_deleted-topic_";

/** How a subscription is delivered. Only a pull one can be read from here. */
export type PubSubDelivery = "pull" | "push" | "bigquery" | "cloudStorage" | "bigtable";

export interface PubSubSubscription {
  name: string;
  path: string | null;

  /** The one topic it reads, or DELETED_TOPIC once that topic is gone. */
  topic: string;
  /** True when the topic is gone: nothing will ever arrive again. */
  orphaned: boolean;

  ackDeadlineSec: number | null;
  retentionSec: number | null;
  /** Keeps acknowledged messages, which is what lets a seek go back past them. */
  retainAcked: boolean;
  /** When the subscription itself is deleted for being idle, in seconds. */
  expirationTtlSec: number | null;

  /** Set at creation and never afterwards. Empty means every message. */
  filter: string | null;
  ordering: boolean;
  exactlyOnce: boolean;

  delivery: PubSubDelivery;
  pushEndpoint: string | null;

  /** The topic its give-ups go to, and after how many delivery attempts. */
  deadLetterTopic: string | null;
  maxDeliveryAttempts: number | null;

  retryMinBackoffSec: number | null;
  retryMaxBackoffSec: number | null;

  /** Detached by an operator: publishing carries on and delivery stops. */
  detached: boolean;
  /** ACTIVE, or RESOURCE_ERROR when the service cannot deliver at all. */
  state: string | null;

  labels: [string, string][];
}

function text(row: Subscription, key: string): string | null {
  const value = row.attributes?.[key];
  return value == null || value === "" ? null : value;
}

function number(row: Subscription, key: string): number | null {
  const value = text(row, key);
  if (value == null) return null;
  const parsed = Number(value);
  return Number.isFinite(parsed) ? parsed : null;
}

function flag(row: Subscription, key: string): boolean {
  return text(row, key) === "true";
}

export function subscription(row: Subscription): PubSubSubscription {
  const topic = text(row, AttrTopic) ?? "";
  return {
    name: row.ref.name,
    path: text(row, AttrPath),

    topic,
    orphaned: topic === DELETED_TOPIC,

    ackDeadlineSec: number(row, AttrAckDeadlineSec),
    retentionSec: number(row, AttrRetentionSec),
    retainAcked: flag(row, AttrRetainAcked),
    expirationTtlSec: number(row, AttrExpirationSec),

    filter: text(row, AttrFilter),
    ordering: flag(row, AttrOrdering),
    exactlyOnce: flag(row, AttrExactlyOnce),

    delivery: (text(row, AttrDelivery) ?? "pull") as PubSubDelivery,
    pushEndpoint: text(row, AttrPushEndpoint),

    deadLetterTopic: text(row, AttrDeadLetterTopic),
    maxDeliveryAttempts: number(row, AttrMaxAttempts),

    retryMinBackoffSec: number(row, AttrRetryMinSec),
    retryMaxBackoffSec: number(row, AttrRetryMaxSec),

    detached: flag(row, AttrDetached),
    state: text(row, AttrState),

    labels: Object.entries(row.attributes ?? {})
      .filter(([key]) => key.startsWith(LabelPrefix))
      .map(([key, value]) => [key.slice(LabelPrefix.length), value] as [string, string])
      .sort(([a], [b]) => a.localeCompare(b)),
  };
}

/**
 * Whether nothing will ever arrive on this subscription again.
 *
 * Two ways to get there and both are permanent without an operator: the topic
 * was deleted, or the subscription was detached. Neither is an error state and
 * neither recovers on its own, so a board showing only errors would show a
 * healthy row for a subscription that is finished.
 */
export function receivesNothing(entry: PubSubSubscription): boolean {
  return entry.orphaned || entry.detached;
}
