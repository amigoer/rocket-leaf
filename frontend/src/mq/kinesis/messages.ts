/**
 * Kinesis's view of a canonical message.
 *
 * The keys are a contract with internal/driver/kinesis/message.go.
 *
 * The canonical shape is RocketMQ's and most of it has no counterpart here.
 * A record has no tag, no key index, no host and no id of its own: the sender
 * sets a partition key, which is not unique, and the service assigns a
 * sequence number, which is unique only within its shard. So the handle that
 * addresses one record is the pair, and this file is where it is taken apart.
 *
 * The partition index and offset the canonical model offers are deliberately
 * unused. A shard id is a name rather than a number, and a sequence number is
 * a 56-digit value that does not fit the offset field at all.
 */
import type { MessageItem } from "@bindings/model/models";

const PropShardID = "shardId";
const PropSequenceNumber = "sequenceNumber";
const PropPartitionKey = "partitionKey";
const PropEncryption = "encryptionType";

/** The filter key the driver's browse understands. */
export const FILTER_SHARD_ID = "shardId";

function property(message: MessageItem, key: string): string | null {
  const value = message.properties?.[key];
  return value == null || value === "" ? null : value;
}

/** The pair that addresses this record: "<shard id>:<sequence number>". */
export const recordIdOf = (message: MessageItem): string => message.messageId;

/** Which shard holds it. Without this the sequence number addresses nothing. */
export const shardIdOf = (message: MessageItem): string | null =>
  property(message, PropShardID);

/** Unique within its shard, and only there. */
export const sequenceNumberOf = (message: MessageItem): string | null =>
  property(message, PropSequenceNumber);

/**
 * The sender's own key, and the only thing it decided: Kinesis hashes it into
 * the 128-bit key space and the shard whose range covers the hash takes the
 * record. It is not unique and nothing indexes it.
 */
export const partitionKeyOf = (message: MessageItem): string | null =>
  property(message, PropPartitionKey);

/** NONE or KMS, as the record was stored. */
export const encryptionOf = (message: MessageItem): string | null =>
  property(message, PropEncryption);

/** When the service accepted the record, in epoch milliseconds. */
export const arrivedAtMs = (message: MessageItem): number | null =>
  message.storeTimestamp > 0 ? message.storeTimestamp : null;
