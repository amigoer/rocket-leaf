/**
 * IBM MQ's view of a canonical destination.
 *
 * The keys are a contract with internal/driver/ibmmq/destination.go.
 *
 * One list holds two kinds of object, and `kind` is what separates them. A
 * queue is a store an application opens by name; a topic is a place a
 * publication is matched against. They share a page because they share a
 * vocabulary from an application's side - both are opened by name - and
 * nothing else about them is alike, which is why almost every column below is
 * null for one of the two.
 *
 * `topicString` is the field worth reading twice. A topic object's name is not
 * what publishers use: they name a string, and the object is where that
 * string's settings are attached. A board that showed only the object name
 * could not tell a reader where to publish.
 */
import type { Destination } from "@bindings/model/models";

const AttrKind = "kind";
const AttrQueueType = "queueType";
const AttrDescription = "description";
const AttrMaxDepth = "maxDepth";
const AttrMaxMessageLength = "maxMessageLength";
const AttrInhibitGet = "inhibitGet";
const AttrInhibitPut = "inhibitPut";
const AttrTransmission = "transmissionQueue";
const AttrBackoutQueue = "backoutQueue";
const AttrBackoutThreshold = "backoutThreshold";
const AttrCluster = "cluster";
const AttrOpenInput = "openInput";
const AttrOpenOutput = "openOutput";
const AttrOldestAge = "oldestMessageAgeSec";
const AttrLastPut = "lastPut";
const AttrLastGet = "lastGet";
const AttrUncommitted = "uncommitted";
const AttrAltered = "altered";
const AttrDeadLetter = "deadLetterQueue";
const AttrTopicString = "topicString";
const AttrTopicType = "topicType";

/** What an application gets when it opens this name. */
export type IbmMqKind = "queue" | "topic";

/**
 * A local queue stores; an alias and a remote definition resolve somewhere
 * else; a model queue is a template a dynamic queue is copied from. Only the
 * first has a depth, which is why so many rows show a dash.
 */
export type IbmMqQueueType = "local" | "alias" | "remote" | "model" | "cluster";

export interface IbmMqDestination {
  name: string;
  kind: IbmMqKind;
  description: string | null;

  /** Queues only. */
  queueType: IbmMqQueueType | null;
  depth: number | null;
  maxDepth: number | null;
  maxMessageLength: number | null;
  /** Applications holding it open for input and for output. */
  openInput: number | null;
  openOutput: number | null;
  /** Written but not committed, which is why a depth can look wrong. */
  uncommitted: number | null;
  oldestMessageAgeSec: number | null;
  lastPut: string | null;
  lastGet: string | null;
  inhibitGet: boolean;
  inhibitPut: boolean;
  transmissionQueue: boolean;
  /** Where this queue's poison messages go, and after how many attempts. */
  backoutQueue: string | null;
  backoutThreshold: number | null;
  cluster: string | null;
  /** The one queue the queue manager itself names for what it cannot deliver. */
  deadLetterQueue: boolean;

  /** Topics only. */
  topicString: string | null;
  topicType: string | null;
  subscribers: number | null;

  altered: string | null;
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
  return row.attributes?.[key] === "true";
}

/** UnknownMetric is -1 on the wire, and it means "no such figure here". */
function metric(value: number): number | null {
  return value < 0 ? null : value;
}

export function destination(row: Destination): IbmMqDestination {
  const kind = (text(row, AttrKind) ?? "queue") as IbmMqKind;
  return {
    name: row.ref.name,
    kind,
    description: text(row, AttrDescription),

    queueType: kind === "queue" ? ((text(row, AttrQueueType) as IbmMqQueueType | null) ?? null) : null,
    depth: metric(row.depth),
    maxDepth: number(row, AttrMaxDepth),
    maxMessageLength: number(row, AttrMaxMessageLength),
    openInput: number(row, AttrOpenInput),
    openOutput: number(row, AttrOpenOutput),
    uncommitted: number(row, AttrUncommitted),
    oldestMessageAgeSec: number(row, AttrOldestAge),
    lastPut: text(row, AttrLastPut),
    lastGet: text(row, AttrLastGet),
    inhibitGet: flag(row, AttrInhibitGet),
    inhibitPut: flag(row, AttrInhibitPut),
    transmissionQueue: flag(row, AttrTransmission),
    backoutQueue: text(row, AttrBackoutQueue),
    backoutThreshold: number(row, AttrBackoutThreshold),
    cluster: text(row, AttrCluster),
    deadLetterQueue: flag(row, AttrDeadLetter),

    topicString: text(row, AttrTopicString),
    topicType: text(row, AttrTopicType),
    subscribers: kind === "topic" ? metric(row.subscribers) : null,

    altered: text(row, AttrAltered),
  };
}

/**
 * Whether this queue would refuse an application right now.
 *
 * Inhibiting is an ordinary operational step - a queue is inhibited while
 * something is being migrated off it - and it is also the commonest reason a
 * message ends up on the dead-letter queue, so a row that did not say so would
 * leave the reader hunting.
 */
export function inhibited(entry: IbmMqDestination): boolean {
  return entry.inhibitGet || entry.inhibitPut;
}

/** How full a queue is, or null where there is no depth to compare. */
export function fullness(entry: IbmMqDestination): number | null {
  if (entry.depth == null || entry.maxDepth == null || entry.maxDepth <= 0) return null;
  return entry.depth / entry.maxDepth;
}
