/**
 * ActiveMQ's view of a canonical subscription.
 *
 * The keys are a contract with internal/driver/activemq/subscription.go.
 *
 * A durable subscription is the same idea in both products and is stored in
 * unrelated places: Artemis binds a queue to a multicast address, Classic
 * registers a consumer against a topic under a client id. The driver reduces
 * both, and what survives here is the vocabulary a reader needs - which topic,
 * which client, whether anything is attached right now.
 */
import type { Subscription } from "@bindings/model/models";

const AttrProduct = "product";
const AttrClientID = "clientId";
const AttrSubscriptionName = "subscriptionName";
const AttrSelector = "selector";
const AttrActive = "active";
const AttrPendingAck = "pendingAck";
const AttrDispatched = "dispatched";
const AttrConsumed = "consumed";
const AttrPrefetch = "prefetchSize";
const AttrSlow = "slowConsumer";
const AttrDurable = "durable";
const AttrDeadLetter = "deadLetterAddress";

export interface ActiveMQSubscription {
  /** The canonical ref: a queue name on Artemis, client|name on Classic. */
  name: string;
  topic: string;
  product: "classic" | "artemis";

  /** Classic only - Artemis identifies a subscription by its queue's name. */
  clientId: string | null;
  /** The half of the name that is the subscription's own. */
  subscriptionName: string | null;

  /** Messages the broker is still holding for this subscriber. */
  backlog: number | null;
  /** Consumers attached right now. Zero is the resting state, not a fault. */
  members: number;
  /** Handed over and not yet acknowledged - in flight rather than still owed. */
  pendingAck: number | null;
  dispatched: number | null;
  consumed: number | null;
  prefetch: number | null;

  selector: string | null;
  durable: boolean | null;
  active: boolean;
  /** Classic marks a consumer that is falling behind what is dispatched to it. */
  slow: boolean | null;
  deadLetterAddress: string | null;

  status: string;
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

function bool(row: Subscription, key: string): boolean | null {
  const value = text(row, key);
  return value == null ? null : value === "true";
}

function metric(value: number): number | null {
  return value < 0 ? null : value;
}

export function subscription(row: Subscription): ActiveMQSubscription {
  return {
    name: row.ref.name,
    // The ref's namespace, not the attribute: the driver puts the topic in
    // both, and the ref is the one the canonical model guarantees.
    topic: row.ref.namespace,
    product: (text(row, AttrProduct) as "classic" | "artemis") ?? "classic",

    clientId: text(row, AttrClientID),
    subscriptionName: text(row, AttrSubscriptionName),

    backlog: metric(Number(row.backlog)),
    members: row.members,
    pendingAck: number(row, AttrPendingAck),
    dispatched: number(row, AttrDispatched),
    consumed: number(row, AttrConsumed),
    prefetch: number(row, AttrPrefetch),

    selector: text(row, AttrSelector),
    durable: bool(row, AttrDurable),
    active: bool(row, AttrActive) === true,
    slow: bool(row, AttrSlow),
    deadLetterAddress: text(row, AttrDeadLetter),

    status: row.status,
  };
}
