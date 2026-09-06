import { describe, expect, it } from "vitest";
import {
  MAX_QUEUE_NAME,
  queueNameProblem,
  reservedName,
  sharesConsumers,
  submittableQueueName,
} from "./names";

/*
 * The rule is the broker's, and it is checked here for one reason: SEMP
 * refuses a bad name by quoting the regular expression it failed, which is
 * accurate and unreadable.
 */
describe("a Solace queue name", () => {
  it("takes the topic-shaped names this family actually uses", () => {
    for (const name of ["orders", "mqstudio/seed/orders", "app.orders", "orders eu", "q_1"]) {
      expect(queueNameProblem(name), name).toBeNull();
      expect(submittableQueueName(name)).toBe(name);
    }
  });

  it("refuses only the seven characters the broker's own pattern excludes", () => {
    for (const name of ["orders*", "orders?", "orders'", "a<b", "a>b", "a&b", "a;b"]) {
      expect(queueNameProblem(name), name).toBe("characters");
      expect(submittableQueueName(name), name).toBeNull();
    }
  });

  it("stops at two hundred characters", () => {
    expect(queueNameProblem("q".repeat(MAX_QUEUE_NAME))).toBeNull();
    expect(queueNameProblem("q".repeat(MAX_QUEUE_NAME + 1))).toBe("tooLong");
  });

  it("treats a blank name as empty rather than as characters", () => {
    expect(queueNameProblem("   ")).toBe("empty");
  });

  // A leading # is the broker's own namespace: #DEAD_MSG_QUEUE and the MQTT
  // and replication queues all live there, and a create is either refused or
  // taken and then treated as internal.
  it("knows the broker's own prefix", () => {
    expect(reservedName("#DEAD_MSG_QUEUE")).toBe(true);
    expect(reservedName("  #MQTT/x")).toBe(true);
    expect(reservedName("orders#1")).toBe(false);
  });
});

/*
 * The access type is the create setting most likely to be regretted:
 * exclusive is the broker's default and hands every message to one consumer
 * while the others wait as standbys, which looks exactly like a broken
 * fan-out.
 */
describe("a Solace access type", () => {
  it("says which one shares the queue between consumers", () => {
    expect(sharesConsumers("non-exclusive")).toBe(true);
    expect(sharesConsumers("exclusive")).toBe(false);
  });
});
