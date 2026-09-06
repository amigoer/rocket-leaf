/**
 * Kinesis's shards, as the shards page reads them.
 *
 * The Go half is internal/model/shard.go, and unlike every other family's
 * extras these arrive as a typed model rather than through the attribute map:
 * a shard is not a variation on a canonical object, it is an object the
 * canonical vocabulary has no entry for.
 *
 * The hash keys are strings on the wire and stay strings here. They are
 * 128-bit unsigned integers, so Number would round the low digits away - and
 * those are precisely the digits that decide which of two neighbouring shards
 * a partition key lands on. Comparing them is done with BigInt.
 */
import type { Shard } from "@bindings/model/models";

export type { Shard };

/** The whole 128-bit key space a stream's open shards divide between them. */
const KEY_SPACE = (1n << 128n) - 1n;

function big(value: string): bigint | null {
  try {
    return value === "" ? null : BigInt(value);
  } catch {
    return null;
  }
}

/**
 * What share of the key space a shard covers, as a fraction between 0 and 1.
 *
 * It is the closest thing a stream has to "how much of the traffic should land
 * here": records are placed by hashing the partition key, so an even spread of
 * keys lands in proportion to these widths. A stream split unevenly has one
 * shard doing most of the work and a count that says nothing about it.
 */
export function keyShare(shard: Shard): number | null {
  const start = big(shard.startHashKey);
  const end = big(shard.endHashKey);
  if (start == null || end == null || end < start) return null;
  // Scaled before the divide: bigint division truncates, so the ratio has to
  // be taken in integer parts-per-million and converted afterwards.
  const width = end - start + 1n;
  return Number((width * 1_000_000n) / (KEY_SPACE + 1n)) / 1_000_000;
}

/** A shard's children, by the two ways a shard can be somebody's parent. */
export function childrenOf(shards: Shard[], id: string): Shard[] {
  return shards.filter(
    (shard) => shard.parentId === id || shard.adjacentParentId === id,
  );
}

/** The parents a shard names: one after a split, two after a merge. */
export function parentsOf(shard: Shard): string[] {
  return [shard.parentId, shard.adjacentParentId].filter((id) => id !== "");
}

/**
 * How many of a listing's shards still take writes.
 *
 * The same number the streams page shows as the open shard count, computed
 * from the listing rather than read off the stream - which is what makes the
 * two comparable, and the difference between them the closed shards nobody
 * has drained.
 */
export function openCount(shards: Shard[]): number {
  return shards.filter((shard) => !shard.closed).length;
}
