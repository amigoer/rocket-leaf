/**
 * Solace's view of a canonical destination.
 *
 * The keys are a contract with internal/driver/solace/destination.go.
 *
 * Two fields here look like the same number and are not, which is the one
 * thing to read twice. `depth` is how many messages the queue is holding right
 * now, and it does not come from a field at all - the driver reads the message
 * collection's own count, because SEMP has no depth on the queue object.
 * `spooledTotal` is the queue's spooledMsgCount: a lifetime statistic that
 * counts every message ever spooled, that clearStats resets to zero on a full
 * queue, and that a drained queue keeps reporting at its high-water mark. It
 * is carried so the detail panel can show it under a name that says what it
 * is, and never as the depth.
 */
import type { Destination } from "@bindings/model/models";

const AttrAccessType = "accessType";
const AttrPermission = "permission";
const AttrOwner = "owner";
const AttrDurable = "durable";
const AttrIngress = "ingressEnabled";
const AttrEgress = "egressEnabled";
const AttrSpoolUsage = "spoolUsageBytes";
const AttrMaxSpool = "maxSpoolUsageMb";
const AttrMaxMsgSize = "maxMsgSizeBytes";
const AttrPartitions = "partitionCount";
const AttrVirtualRouter = "virtualRouter";
const AttrByManagement = "createdByManagement";
const AttrDeadMsgQueue = "deadMsgQueue";
const AttrMaxRedelivery = "maxRedeliveryCount";
const AttrRespectTtl = "respectTtlEnabled";
const AttrMaxTtl = "maxTtlSec";
const AttrRespectDmq = "respectDmqEligibleEnabled";
const AttrRedelivered = "redeliveredMsgCount";
const AttrUnacked = "txUnackedMsgCount";
const AttrToDmqTtl = "ttlExpiredToDmqMsgCount";
const AttrToDmqRetry = "maxRedeliveryToDmqMsgCount";
const AttrSpooledTotal = "spooledMsgCountTotal";

/**
 * Exclusive hands every message to one consumer and keeps the rest waiting as
 * standbys; non-exclusive shares the queue between all of them. It is the
 * setting most likely to be wrong on a queue somebody expected to fan out.
 */
export type SolaceAccessType = "exclusive" | "non-exclusive";

export interface SolaceDestination {
  name: string;
  /** Messages held right now, read from the message collection's count. */
  depth: number | null;
  /** Consumers bound to it right now, read from the flow collection's count. */
  boundConsumers: number | null;
  rateIn: number | null;
  rateOut: number | null;

  accessType: SolaceAccessType | null;
  permission: string | null;
  owner: string | null;
  durable: boolean;
  ingressEnabled: boolean;
  egressEnabled: boolean;

  spoolUsageBytes: number | null;
  maxSpoolUsageMb: number | null;
  maxMsgSizeBytes: number | null;
  partitionCount: number | null;
  virtualRouter: string | null;
  createdByManagement: boolean;

  /** Where this queue's undelivered messages go, and after how many attempts. */
  deadMsgQueue: string | null;
  maxRedeliveryCount: number | null;
  respectTtlEnabled: boolean;
  maxTtlSec: number | null;
  /** Off means every message qualifies, whatever the publisher marked. */
  respectDmqEligibleEnabled: boolean;

  redeliveredMsgCount: number | null;
  unackedMsgCount: number | null;
  ttlExpiredToDmq: number | null;
  redeliveryExceededToDmq: number | null;
  /** A lifetime statistic. Never the depth — see the note above. */
  spooledTotal: number | null;
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

export function destination(row: Destination): SolaceDestination {
  return {
    name: row.ref.name,
    depth: metric(row.depth),
    boundConsumers: metric(row.subscribers),
    rateIn: metric(row.rateIn),
    rateOut: metric(row.rateOut),

    accessType: text(row, AttrAccessType) as SolaceAccessType | null,
    permission: text(row, AttrPermission),
    owner: text(row, AttrOwner),
    durable: flag(row, AttrDurable),
    ingressEnabled: flag(row, AttrIngress),
    egressEnabled: flag(row, AttrEgress),

    spoolUsageBytes: number(row, AttrSpoolUsage),
    maxSpoolUsageMb: number(row, AttrMaxSpool),
    maxMsgSizeBytes: number(row, AttrMaxMsgSize),
    partitionCount: number(row, AttrPartitions),
    virtualRouter: text(row, AttrVirtualRouter),
    createdByManagement: flag(row, AttrByManagement),

    deadMsgQueue: text(row, AttrDeadMsgQueue),
    maxRedeliveryCount: number(row, AttrMaxRedelivery),
    respectTtlEnabled: flag(row, AttrRespectTtl),
    maxTtlSec: number(row, AttrMaxTtl),
    respectDmqEligibleEnabled: flag(row, AttrRespectDmq),

    redeliveredMsgCount: number(row, AttrRedelivered),
    unackedMsgCount: number(row, AttrUnacked),
    ttlExpiredToDmq: number(row, AttrToDmqTtl),
    redeliveryExceededToDmq: number(row, AttrToDmqRetry),
    spooledTotal: number(row, AttrSpooledTotal),
  };
}

/**
 * Whether this queue would refuse a message or a consumer right now.
 *
 * Disabling ingress or egress is an ordinary operational step - a queue is put
 * on hold while something is migrated off it - and it is also the commonest
 * reason a queue that looks healthy is not moving, so a row that did not say
 * so would leave the reader hunting.
 */
export function halted(entry: SolaceDestination): boolean {
  return !entry.ingressEnabled || !entry.egressEnabled;
}

/**
 * How full a queue's spool quota is, or null where there is nothing to
 * compare. The quota is in megabytes and the usage is in bytes, which is the
 * broker's own pair of units rather than a mistake here.
 */
export function fullness(entry: SolaceDestination): number | null {
  if (entry.spoolUsageBytes == null || entry.maxSpoolUsageMb == null) return null;
  if (entry.maxSpoolUsageMb <= 0) return null;
  return entry.spoolUsageBytes / (entry.maxSpoolUsageMb * 1024 * 1024);
}

/** Whether anything is set up to take this queue's undelivered messages. */
export function hasDeadMsgQueue(entry: SolaceDestination): boolean {
  return entry.deadMsgQueue != null && entry.deadMsgQueue !== "";
}
