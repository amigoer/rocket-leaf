/**
 * Kinesis's view of a canonical destination.
 *
 * The keys are a contract with internal/driver/kinesis/destination.go.
 *
 * `openShards` is the field worth reading twice. It is the number of shards
 * taking writes, and it is not the number of shards the stream has: a shard
 * closed by a split or a merge still exists and still holds its records until
 * retention expires. What that count cannot say - which shards, which slice of
 * the hash space each owns, and which parent each was cut from - is why this
 * family has a shards page rather than a partitions column.
 *
 * `consumers` counts registered fan-out consumers, which are the only readers
 * the stream itself knows about. A classic consumer keeps its position in a
 * DynamoDB table this connection never sees, so a stream being read hard by
 * three applications can report zero. That is the service's own answer.
 */
import type { Destination } from "@bindings/model/models";

const AttrARN = "arn";
const AttrStatus = "status";
const AttrMode = "streamMode";
const AttrRetentionHours = "retentionHours";
const AttrOpenShards = "openShards";
const AttrConsumers = "consumers";
const AttrCreatedAt = "createdAt";
const AttrEncryption = "encryption";
const AttrKeyID = "kmsKeyId";
const AttrShardMetrics = "shardLevelMetrics";

/** What the service says a stream is doing. Only ACTIVE accepts a change. */
export type StreamStatus = "CREATING" | "DELETING" | "ACTIVE" | "UPDATING";

/** Whether the shard count is the operator's to set. */
export type StreamMode = "PROVISIONED" | "ON_DEMAND";

export interface KinesisStream {
  name: string;
  arn: string | null;
  status: StreamStatus | null;
  mode: StreamMode | null;

  /** Shards taking writes. Closed parents are not counted. */
  openShards: number | null;
  /** Registered fan-out consumers. A classic consumer registers nothing. */
  consumers: number | null;
  /** How long a record is kept, whether or not anybody read it. */
  retentionHours: number | null;

  createdAtMs: number | null;
  /** NONE or KMS. Changing it is a KMS decision this app does not make. */
  encryption: string | null;
  kmsKeyId: string | null;
  /** Per-shard CloudWatch metrics the stream publishes, if any. */
  shardMetrics: string[];
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

/** UnknownMetric is -1 on the wire, and it means "no such figure here". */
function metric(value: number): number | null {
  return value < 0 ? null : value;
}

export function stream(row: Destination): KinesisStream {
  const metrics = text(row, AttrShardMetrics);
  return {
    name: row.ref.name,
    arn: text(row, AttrARN),
    status: (text(row, AttrStatus) as StreamStatus | null) ?? null,
    mode: (text(row, AttrMode) as StreamMode | null) ?? null,

    // The canonical field and the attribute say the same thing; the attribute
    // is read first so a driver that stops filling one is visible here rather
    // than showing a plausible zero.
    openShards: number(row, AttrOpenShards) ?? metric(row.partitions),
    consumers: number(row, AttrConsumers) ?? metric(row.subscribers),
    retentionHours: number(row, AttrRetentionHours),

    createdAtMs: number(row, AttrCreatedAt),
    encryption: text(row, AttrEncryption),
    kmsKeyId: text(row, AttrKeyID),
    shardMetrics: metrics == null ? [] : metrics.split(","),
  };
}

/**
 * Whether an operation on this stream would be refused right now.
 *
 * Every call that names a stream is refused while it is not ACTIVE, and the
 * service's own message says ResourceInUseException rather than "it is still
 * resizing" - so the board says which state it is in instead.
 */
export function settling(entry: KinesisStream): boolean {
  return entry.status != null && entry.status !== "ACTIVE";
}

/** Hours as the service stores them, read back as something human. */
export function retention(hours: number | null): string | null {
  if (hours == null) return null;
  if (hours < 24) return `${hours}h`;
  const days = hours / 24;
  return Number.isInteger(days) ? `${days}d` : `${hours}h`;
}
