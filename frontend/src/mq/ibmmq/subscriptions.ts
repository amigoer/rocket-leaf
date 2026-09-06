/**
 * IBM MQ's view of a canonical subscription.
 *
 * The keys are a contract with internal/driver/ibmmq/subscription.go.
 *
 * A subscription here registers interest in a topic string and names a queue
 * to deliver to. That queue is where the messages actually sit, so the backlog
 * on the canonical row belongs to it rather than to the subscription - and the
 * two numbers below can disagree in a way worth showing: `messagesReceived` is
 * how many publications the subscription has ever taken, and the backlog is
 * how many of them are still waiting.
 *
 * `attached` is the field that reads wrong if it is read as "online". An
 * administrative subscription - one an operator defined - has nothing attached
 * by design: the publications land on its destination queue and whichever
 * application reads that queue is the consumer. So the row's status looks at
 * whether anything is draining the queue, not at whether a subscriber is
 * connected.
 */
import type { Subscription } from "@bindings/model/models";

const AttrTopicString = "topicString";
const AttrDestination = "destination";
const AttrDestinationQM = "destinationQueueManager";
const AttrDurable = "durable";
const AttrType = "subscriptionType";
const AttrUser = "user";
const AttrSelector = "selector";
const AttrID = "subscriptionId";
const AttrMessages = "messagesReceived";
const AttrLastMessage = "lastMessageAt";
const AttrAttached = "attached";
const AttrReaders = "queueReaders";

/**
 * Who made it. An admin subscription was defined by an operator and outlives
 * every application; an api one was created by a program and usually goes with
 * it; a proxy one belongs to another queue manager in a publish/subscribe
 * cluster.
 */
export type SubscriptionKind = "admin" | "api" | "proxy" | string;

export interface IbmMqSubscription {
  name: string;
  topicString: string | null;
  /** The queue publications are delivered to, which holds the backlog. */
  destination: string | null;
  destinationQueueManager: string | null;
  durable: boolean;
  kind: SubscriptionKind | null;
  user: string | null;
  selector: string | null;
  subscriptionId: string | null;

  /** How many publications it has ever received. A total, not a rate. */
  messagesReceived: number | null;
  lastMessageAt: string | null;
  /** Whether a subscriber is connected. Usually false, and usually fine. */
  attached: boolean;
  /** Applications reading the destination queue: the real consumer count. */
  queueReaders: number | null;
  /** What is still waiting on the destination queue. */
  backlog: number | null;
}

function attribute(row: Subscription, key: string): string | null {
  const value = row.attributes?.[key];
  return value == null || value === "" ? null : value;
}

function number(row: Subscription, key: string): number | null {
  const value = attribute(row, key);
  if (value == null) return null;
  const parsed = Number(value);
  return Number.isFinite(parsed) ? parsed : null;
}

/** UnknownMetric is -1 on the wire, and it means "no such figure here". */
function metric(value: number): number | null {
  return value < 0 ? null : value;
}

export function subscription(row: Subscription): IbmMqSubscription {
  return {
    name: row.ref.name,
    topicString: attribute(row, AttrTopicString),
    destination: attribute(row, AttrDestination),
    destinationQueueManager: attribute(row, AttrDestinationQM),
    durable: attribute(row, AttrDurable) === "yes",
    kind: attribute(row, AttrType),
    user: attribute(row, AttrUser),
    selector: attribute(row, AttrSelector),
    subscriptionId: attribute(row, AttrID),

    messagesReceived: number(row, AttrMessages),
    lastMessageAt: attribute(row, AttrLastMessage),
    attached: attribute(row, AttrAttached) === "true",
    queueReaders: number(row, AttrReaders),
    backlog: metric(row.backlog),
  };
}

/**
 * A subscription collecting publications nothing is reading.
 *
 * The case worth flagging, and it is not "nothing attached": an administrative
 * subscription has nothing attached by design, and it is only a problem when
 * its destination queue is filling up with nobody at the other end.
 */
export function unread(entry: IbmMqSubscription): boolean {
  return (entry.backlog ?? 0) > 0 && !entry.attached && (entry.queueReaders ?? 0) === 0;
}
