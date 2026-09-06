/**
 * Solace's view of a routing topology.
 *
 * The keys are a contract with internal/driver/solace/routing.go.
 *
 * There is no exchange between a topic and a queue on this family, so a
 * binding's source, its routing key and its handle are all the same string:
 * the topic the queue subscribes to. That is why the page shows one column
 * where RabbitMQ's shows three, and why adding one takes two fields rather
 * than four.
 */
import type { Binding, Destination } from "@bindings/model/models";

const ArgWildcard = "wildcard";
const ArgQueueDepth = "queueDepth";
const AttrEndpointTopic = "endpointTopic";
const AttrIngress = "ingressEnabled";
const AttrEgress = "egressEnabled";
const AttrAccessType = "accessType";
const AttrDeadMsgQueue = "deadMsgQueue";

export interface SolaceSubscription {
  /** The queue that receives what matches. */
  queue: string;
  /** The topic pattern it subscribes to, which is also its handle. */
  topic: string;
  /** True when it matches more than one topic. */
  wildcard: boolean;
  /** What the queue is holding, so a subscription filling one is visible here. */
  queueDepth: number | null;
}

export interface SolaceTopicEndpoint {
  /** The name, which is the topic it takes. There is no second field. */
  name: string;
  depth: number | null;
  accessType: string | null;
  deadMsgQueue: string | null;
  ingressEnabled: boolean;
  egressEnabled: boolean;
}

function argument(binding: Binding, key: string): string | null {
  const value = binding.arguments?.[key];
  return value == null || value === "" ? null : value;
}

function attribute(row: Destination, key: string): string | null {
  const value = row.attributes?.[key];
  return value == null || value === "" ? null : value;
}

/** UnknownMetric is -1 on the wire, and it means "no such figure here". */
function metric(value: number): number | null {
  return value < 0 ? null : value;
}

export function subscription(binding: Binding): SolaceSubscription {
  const depth = argument(binding, ArgQueueDepth);
  const parsed = depth == null ? Number.NaN : Number(depth);
  return {
    queue: binding.destination,
    topic: binding.propertiesKey !== "" ? binding.propertiesKey : binding.routingKey,
    wildcard: argument(binding, ArgWildcard) === "true",
    queueDepth: Number.isFinite(parsed) && parsed >= 0 ? parsed : null,
  };
}

export function topicEndpoint(row: Destination): SolaceTopicEndpoint {
  return {
    name: attribute(row, AttrEndpointTopic) ?? row.ref.name,
    depth: metric(row.depth),
    accessType: attribute(row, AttrAccessType),
    deadMsgQueue: attribute(row, AttrDeadMsgQueue),
    ingressEnabled: row.attributes?.[AttrIngress] === "true",
    egressEnabled: row.attributes?.[AttrEgress] === "true",
  };
}

/**
 * Whether a topic pattern is one the broker would accept as a subscription.
 *
 * The two wildcards are positional rather than free. "*" stands for one whole
 * level and is only legal as a whole level or at the end of one; ">" stands
 * for the rest of the topic and is only legal as the last character. A pattern
 * like "orders/*eu" looks like a glob and matches nothing at all, which is the
 * mistake this catches before it becomes a subscription nobody notices is
 * dead.
 */
export type TopicProblem = "empty" | "midLevelWildcard" | "trailingWildcardPlacement";

export function topicProblem(topic: string): TopicProblem | null {
  const trimmed = topic.trim();
  if (trimmed === "") return "empty";

  const gt = trimmed.indexOf(">");
  if (gt >= 0 && gt !== trimmed.length - 1) return "trailingWildcardPlacement";

  for (const level of trimmed.split("/")) {
    if (!level.includes("*")) continue;
    // A star is a whole level, or the tail of one: "a/*" and "a/pre*" are both
    // legal, "a/*post" and "a/p*t" are not.
    if (level !== "*" && level.indexOf("*") !== level.length - 1) return "midLevelWildcard";
  }
  return null;
}

/** The subscriptions pointing at one queue. */
export function forQueue(
  subscriptions: readonly SolaceSubscription[],
  queue: string,
): SolaceSubscription[] {
  return subscriptions.filter((entry) => entry.queue === queue);
}
