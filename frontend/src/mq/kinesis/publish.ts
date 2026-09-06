/**
 * What the Kinesis send console collects, and the rules it enforces.
 *
 * Separate from the form component so the rules can be tested without
 * rendering. Each catches something the service would either refuse with a
 * message naming a parameter the form never drew, or accept and quietly get
 * wrong:
 *
 *   - An empty body is ValidationException naming Data, on a form with
 *     several fields that could be blank.
 *   - A missing partition key is the same exception naming PartitionKey, and
 *     it is the field that decides where the record lands - there is no
 *     default and nothing to fall back to.
 *   - An explicit hash key that is not a decimal number is refused by a
 *     regular expression that names neither the field nor the range.
 *   - A repeat with an explicit hash key is accepted and puts every copy on
 *     one shard, which is what the hash key is for and worth saying out loud.
 */
import type { KinesisPublishInput } from "@bindings/bridge/models";

/**
 * How many copies one send may carry.
 *
 * This app's cap rather than the service's, mirrored from
 * internal/driver/kinesis/publish.go: a send console is for producing a
 * handful by hand, and a shard takes a thousand records a second.
 */
export const MAX_COUNT = 1000;

/** The largest payload one record may carry, which the service enforces. */
export const MAX_RECORD_BYTES = 1024 * 1024;

/** A hash key is a 128-bit unsigned integer, so at most 39 decimal digits. */
const MAX_HASH_KEY_DIGITS = 39;

export interface KinesisProducerDraft {
  stream: string;
  body: string;
  /** Required. What the service hashes to choose a shard. */
  partitionKey: string;
  /** Optional. Overrides that hash, which aims the record at one shard. */
  explicitHashKey: string;
  count: number;
}

export function emptyKinesisProducerDraft(): KinesisProducerDraft {
  return { stream: "", body: "", partitionKey: "", explicitHashKey: "", count: 1 };
}

/** Whether the send aims at a shard rather than letting the key place it. */
export function aimsAtAShard(draft: KinesisProducerDraft): boolean {
  return draft.explicitHashKey.trim() !== "";
}

/** Why the draft cannot be sent, as an i18n key, or null when it can. */
export function sendProblem(draft: KinesisProducerDraft): string | null {
  if (draft.stream.trim() === "") return "board.kinesis.producer.streamRequired";
  if (draft.body === "") return "board.kinesis.producer.bodyRequired";
  // Bytes rather than characters: the limit is on the payload, and a
  // multi-byte character counts for what it weighs.
  if (new TextEncoder().encode(draft.body).length > MAX_RECORD_BYTES) {
    return "board.kinesis.producer.bodyTooLarge";
  }
  if (draft.partitionKey.trim() === "") return "board.kinesis.producer.keyRequired";
  if (draft.count < 1 || draft.count > MAX_COUNT) return "board.kinesis.producer.countRange";
  const hashKey = draft.explicitHashKey.trim();
  if (hashKey !== "") {
    if (!/^\d+$/.test(hashKey) || hashKey.length > MAX_HASH_KEY_DIGITS) {
      return "board.kinesis.producer.hashKeyInvalid";
    }
  }
  return null;
}

/**
 * What the console warns about but will still send, as an i18n key.
 *
 * An aimed repeat is worth saying out loud: the hash key overrides the
 * partition key entirely, so every copy lands on one shard and the per-copy
 * key the driver would otherwise vary does nothing.
 */
export function sendWarning(draft: KinesisProducerDraft): string | null {
  if (aimsAtAShard(draft) && draft.count > 1) return "board.kinesis.producer.aimedRepeatNote";
  return null;
}

/** The input to send, or null when the draft is not yet whole. */
export function toPublishInput(draft: KinesisProducerDraft): KinesisPublishInput | null {
  if (sendProblem(draft) != null) return null;
  return {
    stream: draft.stream.trim(),
    body: draft.body,
    partitionKey: draft.partitionKey.trim(),
    explicitHashKey: draft.explicitHashKey.trim(),
    count: draft.count,
  };
}
