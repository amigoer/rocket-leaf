/**
 * Kinesis's view of a canonical subscription.
 *
 * The keys are a contract with internal/driver/kinesis/subscription.go.
 *
 * A "subscription" here is a registered enhanced fan-out consumer: a real
 * object with a name and an ARN, created and removed on its own. It is the
 * only reader a stream knows anything about. A classic consumer - the KCL, a
 * Lambda event source, anything calling GetRecords - registers nothing and
 * keeps its position in a DynamoDB table this connection never sees, so it
 * cannot appear on the page and its absence is the service's answer rather
 * than a gap in this app.
 *
 * Which is also why there is no backlog. A registered consumer carries no
 * position at all, so there is nothing to subtract from anything.
 */
import type { Subscription } from "@bindings/model/models";

const AttrConsumerARN = "consumerArn";
const AttrConsumerStatus = "consumerStatus";
const AttrConsumerSince = "createdAt";
const AttrStream = "stream";

/** Only an ACTIVE consumer can be subscribed to. */
export type ConsumerStatus = "CREATING" | "DELETING" | "ACTIVE";

export interface KinesisConsumer {
  name: string;
  /** The stream it is registered on. A name is unique only within one. */
  stream: string;
  /** What an application actually subscribes with. */
  arn: string | null;
  status: ConsumerStatus | null;
  registeredAtMs: number | null;
}

function attribute(row: Subscription, key: string): string | null {
  const value = row.attributes?.[key];
  return value == null || value === "" ? null : value;
}

export function consumer(row: Subscription): KinesisConsumer {
  const since = attribute(row, AttrConsumerSince);
  const parsed = since == null ? Number.NaN : Number(since);
  return {
    name: row.ref.name,
    stream: attribute(row, AttrStream) ?? row.ref.namespace,
    arn: attribute(row, AttrConsumerARN),
    status: (attribute(row, AttrConsumerStatus) as ConsumerStatus | null) ?? null,
    registeredAtMs: Number.isFinite(parsed) ? parsed : null,
  };
}

/**
 * Whether the registration is still settling.
 *
 * Registering and deregistering are both asynchronous, and a consumer that is
 * not ACTIVE cannot be subscribed to - so an application pointed at it now
 * would fail with an error about the consumer rather than about the stream.
 */
export function settling(entry: KinesisConsumer): boolean {
  return entry.status != null && entry.status !== "ACTIVE";
}
