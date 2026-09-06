import { KinesisService } from "@bindings/bridge";
import type {
  KinesisPublishInput,
  KinesisPublishResult,
  KinesisStreamInput,
} from "@bindings/bridge/models";
import type { Shard } from "@bindings/model/models";
import { present, required } from "./client";

export type { KinesisPublishInput, KinesisPublishResult, KinesisStreamInput };

/**
 * The Kinesis-only half of the surface.
 *
 * Reading streams is not here: a stream is a destination, and api/topic.ts
 * already answers the whole read side. What this carries is what the canonical
 * services cannot express - a create whose fields are a capacity mode and a
 * retention, where TopicService.Create collects a broker address, two queue
 * counts and a permission string.
 */

/**
 * Declare a stream in the connection's region.
 *
 * The capacity mode decides whether the shard count is used at all: AWS picks
 * an on-demand stream's capacity, and CreateStream refuses a count beside it.
 */
export const createStream = (connID: number, input: KinesisStreamInput): Promise<void> =>
  KinesisService.CreateStream(connID, input);

/**
 * Change an existing stream's capacity mode, shard count or retention.
 *
 * Three separate asynchronous operations on the service, each refused while
 * the stream is settling from the last, so this takes seconds rather than
 * milliseconds when more than one of them changed.
 */
export const updateStream = (connID: number, input: KinesisStreamInput): Promise<void> =>
  KinesisService.UpdateStream(connID, input);

/**
 * Delete a stream and every record in it.
 *
 * There is no undo. A stream with registered consumers is refused rather than
 * cascaded - deregister them on the consumers page first.
 */
export const removeStream = (connID: number, name: string): Promise<void> =>
  KinesisService.RemoveStream(connID, name);

/**
 * One stream's shards, open and closed.
 *
 * Closed ones are included: a shard split or merged still holds its records
 * until retention expires, so leaving it out would hide both the records and
 * the reason a stream lists more shards than its open count.
 */
export const shards = async (connID: number, stream: string): Promise<Shard[]> =>
  present(await KinesisService.Shards(connID, stream));

/**
 * Send one record, or the same body several times.
 *
 * Every record needs a partition key: the service hashes it to choose a shard,
 * and there is no default. An explicit hash key overrides that hash, which is
 * the only way to aim a record at a shard by name.
 */
export const publish = (
  connID: number,
  input: KinesisPublishInput,
): Promise<KinesisPublishResult> =>
  KinesisService.Publish(connID, input).then(required);

/**
 * Register an enhanced fan-out consumer on a stream.
 *
 * Registering reserves the name and the dedicated two megabytes a second of
 * read throughput; an application then subscribes against the ARN. A stream
 * takes at most twenty, which is a service quota rather than a setting.
 */
export const registerConsumer = (connID: number, stream: string, name: string): Promise<void> =>
  KinesisService.RegisterConsumer(connID, stream, name);

/**
 * Remove a registration.
 *
 * It frees the name and stops the throughput being reserved. It does not stop
 * anything reading: a classic consumer registered nothing and is unaffected.
 */
export const deregisterConsumer = (connID: number, stream: string, name: string): Promise<void> =>
  KinesisService.DeregisterConsumer(connID, stream, name);
