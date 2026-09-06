import { describe, expect, it } from "vitest";
import { isEphemeral, nameProblem, submittableName } from "./names";

/**
 * nsqd's own rule, pinned against what a running 1.3.0 actually accepts.
 *
 * The refusal is worth catching here rather than at the daemon: a bad
 * character comes back as INVALID_REQUEST because it broke the URL before
 * nsqd read it, and a long name as INVALID_TOPIC, and neither answer says
 * which part of the name was wrong.
 */
describe("an nsq topic or channel name", () => {
  it("accepts the character set nsqd accepts", () => {
    for (const name of ["orders", "MQS.SEED.orders", "a_b-c.d", "ORDERS9"]) {
      expect(nameProblem(name), name).toBeNull();
    }
  });

  it("refuses everything outside it", () => {
    for (const name of ["a/b", "a b", "a:b", "orders!", "主题"]) {
      expect(nameProblem(name), name).toBe("charset");
    }
  });

  it("refuses an empty name, before and after trimming", () => {
    expect(nameProblem("")).toBe("empty");
    expect(nameProblem("   ")).toBe("empty");
  });

  // Sixty-four exactly, confirmed against 1.3.0: 64 answers 200 and 65
  // answers INVALID_TOPIC.
  it("stops at sixty-four characters", () => {
    expect(nameProblem("a".repeat(64))).toBeNull();
    expect(nameProblem("a".repeat(65))).toBe("tooLong");
  });

  // The suffix is inside the limit rather than extra, which is the half a
  // reimplementation gets wrong: 64 characters plus #ephemeral is refused.
  it("counts the ephemeral suffix towards the limit", () => {
    expect(nameProblem("a".repeat(64) + "#ephemeral")).toBe("tooLong");
    expect(nameProblem("a".repeat(54) + "#ephemeral")).toBeNull();
  });

  it("accepts the ephemeral suffix and nothing else containing a hash", () => {
    expect(nameProblem("orders#ephemeral")).toBeNull();
    expect(nameProblem("orders#temp")).toBe("charset");
    expect(nameProblem("orders#ephemeral#ephemeral")).toBe("charset");
  });

  it("submits the trimmed name, and nothing at all when the name is unusable", () => {
    expect(submittableName("  orders  ")).toBe("orders");
    expect(submittableName("  a/b  ")).toBeNull();
  });

  // An ephemeral topic keeps nothing on disk and is deleted when its last
  // client goes, so the form warns rather than letting someone find out.
  it("reports the suffix that makes an object disappear", () => {
    expect(isEphemeral("orders#ephemeral")).toBe(true);
    expect(isEphemeral("  orders#ephemeral  ")).toBe(true);
    expect(isEphemeral("orders")).toBe(false);
  });
});
