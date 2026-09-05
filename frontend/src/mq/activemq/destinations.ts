/**
 * ActiveMQ's view of a canonical destination.
 *
 * The keys are a contract with internal/driver/activemq/attributes.go.
 *
 * Almost every reader returns null where the broker did not answer, and here
 * that is load-bearing rather than tidiness: one MQKind covers two products
 * that report different things, so a column is empty on Classic and filled on
 * Artemis or the other way round. A reader that turned an absence into 0 would
 * print a figure the broker never said, on half the connections.
 */
import type { Destination } from "@bindings/model/models";

const AttrProduct = "product";
const AttrKind = "kind";
const AttrConsumerCount = "consumerCount";
const AttrProducerCount = "producerCount";
const AttrEnqueueCount = "enqueueCount";
const AttrDequeueCount = "dequeueCount";
const AttrDispatchCount = "dispatchCount";
const AttrExpiredCount = "expiredCount";
const AttrInFlightCount = "inFlightCount";
const AttrScheduledCount = "scheduledCount";
const AttrMemoryUsage = "memoryUsage";
const AttrMemoryPercent = "memoryPercent";
const AttrMessageSize = "averageMessageSize";
const AttrPaused = "paused";
const AttrDurable = "durable";
const AttrInternal = "internal";
const AttrTemporary = "temporary";
const AttrAutoCreated = "autoCreated";
const AttrDeadLetter = "deadLetterAddress";
const AttrExpiry = "expiryAddress";
const AttrIsDeadLetter = "isDeadLetter";
const AttrRoutingTypes = "routingTypes";
const AttrAddress = "address";
const AttrFilter = "filter";
const AttrQueueCount = "queueCount";
const AttrBrowseCap = "browseCap";

/** Which of the family's two brokers this row came from. */
export type Product = "classic" | "artemis";

/** The JMS distinction both products keep and store differently. */
export type DestinationKind = "queue" | "topic";

export interface ActiveMQDestination {
  name: string;
  product: Product;
  kind: DestinationKind;

  /** Messages held. Null only if the broker did not answer at all. */
  depth: number | null;
  /** Connected consumers on a queue; declared subscriptions on a topic. */
  subscribers: number | null;

  consumers: number | null;
  /** Classic only. Artemis counts producers per connection, not per address. */
  producers: number | null;
  enqueued: number | null;
  dequeued: number | null;
  /** Classic only: messages sent to consumers, which is not the same as taken. */
  dispatched: number | null;
  expired: number | null;
  /** Handed to a consumer and not yet acknowledged. */
  inFlight: number | null;
  /** Accepted and waiting for a delivery time. Artemis only. */
  scheduled: number | null;

  bytes: number | null;
  memoryPercent: number | null;
  averageMessageSize: number | null;

  paused: boolean | null;
  durable: boolean | null;
  internal: boolean | null;
  temporary: boolean | null;
  autoCreated: boolean | null;

  deadLetterAddress: string | null;
  expiryAddress: string | null;
  /** This destination is where undeliverable messages are sent. */
  isDeadLetter: boolean;

  /** Artemis only: the routing level above the queue, and its routing types. */
  address: string | null;
  routingTypes: string | null;
  filter: string | null;
  /** Artemis only: queues under an address - a topic's subscriptions. */
  queueCount: number | null;

  /**
   * How many messages one browse will return at most, where the product caps
   * it. Present on Classic, absent on Artemis, and the board says so rather
   * than silently showing a short page.
   */
  browseCap: number | null;
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

function bool(row: Destination, key: string): boolean | null {
  const value = text(row, key);
  return value == null ? null : value === "true";
}

/** UnknownMetric is -1 on the wire, and it means "no such figure here". */
function metric(value: number): number | null {
  return value < 0 ? null : value;
}

export function destination(row: Destination): ActiveMQDestination {
  return {
    name: row.ref.name,
    product: (text(row, AttrProduct) as Product) ?? "classic",
    kind: (text(row, AttrKind) as DestinationKind) ?? "queue",

    depth: metric(Number(row.depth)),
    subscribers: metric(row.subscribers),

    consumers: number(row, AttrConsumerCount),
    producers: number(row, AttrProducerCount),
    enqueued: number(row, AttrEnqueueCount),
    dequeued: number(row, AttrDequeueCount),
    dispatched: number(row, AttrDispatchCount),
    expired: number(row, AttrExpiredCount),
    inFlight: number(row, AttrInFlightCount),
    scheduled: number(row, AttrScheduledCount),

    bytes: number(row, AttrMemoryUsage),
    memoryPercent: number(row, AttrMemoryPercent),
    averageMessageSize: number(row, AttrMessageSize),

    paused: bool(row, AttrPaused),
    durable: bool(row, AttrDurable),
    internal: bool(row, AttrInternal),
    temporary: bool(row, AttrTemporary),
    autoCreated: bool(row, AttrAutoCreated),

    deadLetterAddress: text(row, AttrDeadLetter),
    expiryAddress: text(row, AttrExpiry),
    isDeadLetter: bool(row, AttrIsDeadLetter) === true,

    address: text(row, AttrAddress),
    routingTypes: text(row, AttrRoutingTypes),
    filter: text(row, AttrFilter),
    queueCount: number(row, AttrQueueCount),

    browseCap: number(row, AttrBrowseCap),
  };
}

/**
 * Whether a browse of this destination will have been cut short.
 *
 * Classic stops at maxBrowsePageSize however deep the destination is, and the
 * limit is not readable over JMX - so this compares the cap against the depth
 * rather than against what came back. It is the difference between a page
 * that looks complete and one that says it is not.
 */
export function browseWillBeCapped(entry: ActiveMQDestination): boolean {
  return entry.browseCap != null && entry.depth != null && entry.depth > entry.browseCap;
}
