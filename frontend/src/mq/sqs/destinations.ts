/**
 * SQS's view of a canonical destination.
 *
 * The keys are a contract with internal/driver/sqs/destination.go.
 *
 * The three counts are the part worth understanding, and no single figure
 * replaces them. `visible` is what a consumer would be handed right now,
 * `inFlight` is what has been handed out and not yet deleted, and `delayed` is
 * what is not due yet. A queue whose whole depth is in flight has a consumer
 * that is not finishing; one whose whole depth is visible has no consumer at
 * all; and one whose whole depth is delayed is working exactly as intended.
 *
 * Every one of them is approximate, and that is the service rather than this
 * app: SQS is distributed and reports what its servers last agreed on, so two
 * reads a second apart can disagree with no message having moved.
 */
import type { Destination } from "@bindings/model/models";

const AttrURL = "url";
const AttrARN = "arn";
const AttrFIFO = "fifo";
const AttrVisible = "visible";
const AttrInFlight = "inFlight";
const AttrDelayed = "delayed";
const AttrVisibilityTimeout = "visibilityTimeoutSec";
const AttrDelaySeconds = "delaySec";
const AttrRetentionSec = "retentionSec";
const AttrMaxMessageBytes = "maxMessageBytes";
const AttrReceiveWaitSec = "receiveWaitSec";
const AttrCreatedAt = "createdAt";
const AttrModifiedAt = "modifiedAt";
const AttrDeadLetterQueue = "deadLetterQueue";
const AttrMaxReceiveCount = "maxReceiveCount";
const AttrEncrypted = "encrypted";
const AttrContentDedup = "contentBasedDeduplication";
const AttrDedupScope = "deduplicationScope";
const AttrThroughputLimit = "fifoThroughputLimit";

export interface SqsQueue {
  name: string;
  /** The URL every API call addresses it by. The name is only for reading. */
  url: string | null;
  arn: string | null;

  /** Everything the queue holds: visible, in flight and delayed together. */
  depth: number | null;
  /** Available to a consumer now. */
  visible: number | null;
  /** Handed out and not yet deleted, so hidden until the timeout expires. */
  inFlight: number | null;
  /** Accepted and not due yet. */
  delayed: number | null;

  /** Ordering, deduplication and a mandatory group id on every send. */
  fifo: boolean;
  /** How long a received message stays hidden, in seconds. */
  visibilityTimeoutSec: number | null;
  /** A delay applied to every send that does not set its own. */
  delaySec: number | null;
  /** How long SQS keeps an undeleted message, in seconds. */
  retentionSec: number | null;
  maxMessageBytes: number | null;
  /** How long a receive waits for a message. Zero is a short poll. */
  receiveWaitSec: number | null;

  createdAtMs: number | null;
  modifiedAtMs: number | null;

  /** The queue this one's failures are redriven into, by redrive policy. */
  deadLetterQueue: string | null;
  /** How many receives before a message is moved there. */
  maxReceiveCount: number | null;

  /** Server-side encryption of either kind. Which key is a KMS question. */
  encrypted: boolean;
  /** FIFO-only, and absent rather than false on a standard queue. */
  contentBasedDeduplication: boolean | null;
  deduplicationScope: string | null;
  fifoThroughputLimit: string | null;
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

/** SQS timestamps are unix seconds; the app formats milliseconds. */
function millis(row: Destination, key: string): number | null {
  const seconds = number(row, key);
  return seconds == null ? null : seconds * 1000;
}

function flag(row: Destination, key: string): boolean | null {
  const value = text(row, key);
  return value == null ? null : value === "true";
}

/** UnknownMetric is -1 on the wire, and it means "no such figure here". */
function metric(value: number): number | null {
  return value < 0 ? null : value;
}

export function queue(row: Destination): SqsQueue {
  return {
    name: row.ref.name,
    url: text(row, AttrURL),
    arn: text(row, AttrARN),

    depth: metric(Number(row.depth)),
    visible: number(row, AttrVisible),
    inFlight: number(row, AttrInFlight),
    delayed: number(row, AttrDelayed),

    fifo: flag(row, AttrFIFO) === true,
    visibilityTimeoutSec: number(row, AttrVisibilityTimeout),
    delaySec: number(row, AttrDelaySeconds),
    retentionSec: number(row, AttrRetentionSec),
    maxMessageBytes: number(row, AttrMaxMessageBytes),
    receiveWaitSec: number(row, AttrReceiveWaitSec),

    createdAtMs: millis(row, AttrCreatedAt),
    modifiedAtMs: millis(row, AttrModifiedAt),

    deadLetterQueue: text(row, AttrDeadLetterQueue),
    maxReceiveCount: number(row, AttrMaxReceiveCount),

    encrypted: flag(row, AttrEncrypted) === true,
    contentBasedDeduplication: flag(row, AttrContentDedup),
    deduplicationScope: text(row, AttrDedupScope),
    fifoThroughputLimit: text(row, AttrThroughputLimit),
  };
}

/**
 * Whether everything this queue holds is in the hands of a consumer.
 *
 * The state to point at rather than the raw number: a queue with a hundred
 * in flight and nothing visible looks idle on a board that shows only the
 * available count, and what it actually means is that something took those
 * hundred messages and has not finished any of them.
 */
export function stalledInFlight(entry: SqsQueue): boolean {
  return (entry.inFlight ?? 0) > 0 && (entry.visible ?? 0) === 0 && (entry.delayed ?? 0) === 0;
}
