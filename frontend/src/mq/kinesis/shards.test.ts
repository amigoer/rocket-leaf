import { describe, expect, it } from "vitest";
import { childrenOf, keyShare, openCount, parentsOf, type Shard } from "./shards";

/** A shard with only the fields a test cares about filled in. */
function shard(extra: Partial<Shard>): Shard {
  return {
    id: "shardId-000000000000",
    parentId: "",
    adjacentParentId: "",
    startHashKey: "",
    endHashKey: "",
    startSequence: "",
    endSequence: "",
    closed: false,
    ...extra,
  } as Shard;
}

/**
 * The hash keys are 128-bit unsigned integers, and this is the test that says
 * why they are strings.
 *
 * A stream's key space is 2^128, which is seventeen digits past what a double
 * can represent exactly - so Number() rounds the low end away, and the low end
 * is precisely what decides which of two neighbouring shards a partition key
 * lands on. Halving the space is the case that shows it: the two halves have
 * to come out at 50% each rather than at whatever the rounding left.
 */
describe("a shard's share of the key space", () => {
  const LAST = (2n ** 128n - 1n).toString();
  const MID = (2n ** 127n).toString();

  it("gives a whole stream's single shard the whole space", () => {
    expect(keyShare(shard({ startHashKey: "0", endHashKey: LAST }))).toBe(1);
  });

  it("splits a halved stream evenly, which rounding through a double does not", () => {
    const lower = keyShare(shard({ startHashKey: "0", endHashKey: (2n ** 127n - 1n).toString() }));
    const upper = keyShare(shard({ startHashKey: MID, endHashKey: LAST }));
    expect(lower).toBeCloseTo(0.5, 6);
    expect(upper).toBeCloseTo(0.5, 6);
  });

  it("has no answer for a shard with no range rather than an invented zero", () => {
    expect(keyShare(shard({}))).toBeNull();
    expect(keyShare(shard({ startHashKey: "10", endHashKey: "1" }))).toBeNull();
    expect(keyShare(shard({ startHashKey: "not a number", endHashKey: LAST }))).toBeNull();
  });
});

/**
 * Lineage, which is the half of a shard listing a count cannot hold.
 *
 * A split gives a child one parent; a merge gives it two, and the second is
 * the adjacent one. Both have to count as being somebody's child, or the page
 * would draw a merged shard as having appeared from nowhere.
 */
describe("shard lineage", () => {
  const parentA = shard({ id: "shardId-0", closed: true, endSequence: "49" });
  const parentB = shard({ id: "shardId-1", closed: true, endSequence: "50" });
  const merged = shard({ id: "shardId-2", parentId: "shardId-0", adjacentParentId: "shardId-1" });
  const shards = [parentA, parentB, merged];

  it("names both parents of a merge and the one parent of a split", () => {
    expect(parentsOf(merged)).toEqual(["shardId-0", "shardId-1"]);
    expect(parentsOf(shard({ parentId: "shardId-0" }))).toEqual(["shardId-0"]);
    expect(parentsOf(parentA)).toEqual([]);
  });

  it("finds a child through either parent field", () => {
    expect(childrenOf(shards, "shardId-0").map((entry) => entry.id)).toEqual(["shardId-2"]);
    expect(childrenOf(shards, "shardId-1").map((entry) => entry.id)).toEqual(["shardId-2"]);
    expect(childrenOf(shards, "shardId-2")).toEqual([]);
  });

  // The number the streams page shows, computed from the listing - which is
  // what makes the difference between them the closed shards nobody drained.
  it("counts only the shards still taking writes", () => {
    expect(openCount(shards)).toBe(1);
    expect(openCount([])).toBe(0);
  });
});
