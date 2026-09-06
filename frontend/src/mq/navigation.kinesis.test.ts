import { describe, expect, it } from "vitest";
import { Capability } from "@bindings/model/models";
import { navAvailability } from "./navigation";
import { PROTOCOLS } from "@/design/data/protocols";
import type { CapabilityState } from "./capabilities";

/**
 * The sidebar a real Amazon Kinesis connection draws.
 *
 * Like SQS, this is one managed service with one feature set: a credential
 * that cannot do something fails the call rather than narrowing what the
 * connection reports. So nothing here is conditional except the backlog, which
 * is degraded on every connection there is - a stream records that a consumer
 * is registered and nothing about where it has read to.
 *
 * The list below is what `capabilities()` in internal/driver/kinesis/conn.go
 * declares. A Go test asserts the driver still declares exactly this, so the
 * two halves cannot drift apart without one of them failing.
 */
const KINESIS_CAPABILITIES: Capability[] = [
  Capability.CapDestinationList,
  Capability.CapDestinationCreate,
  Capability.CapDestinationUpdate,
  Capability.CapDestinationDelete,
  Capability.CapShards,
  Capability.CapMessageQuery,
  Capability.CapMessageByID,
  Capability.CapPublish,
  Capability.CapSubscriptionList,
  Capability.CapSubscriptionCreate,
  Capability.CapSubscriptionDelete,
];

/** Degraded on every Kinesis connection, rather than on some endpoints. */
const KINESIS_DEGRADED: Partial<Record<Capability, string>> = {
  [Capability.CapSubscriptionLag]: "mq.kinesis.degraded.positionInDynamo",
};

function state(
  supported: Capability[],
  degraded: Partial<Record<Capability, string>> = {},
  caveats: Partial<Record<Capability, string>> = {},
): CapabilityState {
  return {
    has: (capability) => supported.includes(capability),
    degradedReason: (capability) => degraded[capability],
    caveat: (capability) => caveats[capability],
    loading: false,
  };
}

/** Every page the Kinesis sidebar is built from, in the order it draws them. */
const drawn = PROTOCOLS.kinesis.nav.flatMap((group) => group.items.map((item) => item.id));

describe("the sidebar a Kinesis connection draws", () => {
  /*
   * The page no other family has, asserted from both sides: the sidebar draws
   * it, and it is reachable only through the capability that exists for it.
   *
   * Not CapPartitions. That capability is answered by DestinationStats, whose
   * page is a read range per partition number, and this driver declares
   * neither - a shard has a name, a slice of the hash space and a parent it
   * was split from, and a page built for a number would drop all three.
   */
  it("draws the shards page, and reaches it only through CapShards", () => {
    expect(drawn).toContain("shards");
    expect(KINESIS_CAPABILITIES).toContain(Capability.CapShards);
    expect(KINESIS_CAPABILITIES).not.toContain(Capability.CapPartitions);

    const nav = navAvailability(state(KINESIS_CAPABILITIES, KINESIS_DEGRADED), true);
    expect(nav.visible("shards")).toBe(true);
    expect(nav.disabled("shards")).toBe(false);

    // Without it the entry is not drawn at all, which is what keeps the page
    // out of every other family's sidebar.
    const without = navAvailability(
      state(KINESIS_CAPABILITIES.filter((c) => c !== Capability.CapShards)),
      true,
    );
    expect(without.visible("shards")).toBe(false);
  });

  /*
   * The pages Kinesis has no concept of, asserted twice: absent from the
   * sidebar, and with no capability declared that would bring them back.
   *
   * Dead letters is the one worth naming. Nothing in Kinesis is ever moved
   * aside: a record stays where it was written until retention expires,
   * whether anybody read it or not, so there is no dead-letter store to read
   * and no topology pointing at one. Cluster and clients follow from AWS
   * running the service. Access control is IAM's, one service further out.
   */
  it("draws none of the pages Kinesis has no concept of", () => {
    const absent = [
      "dlq",
      "clients",
      "cluster",
      "acl",
      "exchanges",
      "vhosts",
      "policies",
      "quotas",
      "subscribe",
    ];
    for (const page of absent) {
      expect(drawn).not.toContain(page);
    }

    const nav = navAvailability(state(KINESIS_CAPABILITIES, KINESIS_DEGRADED), true);
    for (const page of absent) {
      expect(nav.visible(page), page).toBe(false);
    }
  });

  /*
   * Every entry the sidebar draws has to be reachable from the capabilities
   * the driver declares. An entry drawn with nothing behind it opens onto a
   * page that fails when it asks for data, which reads as a broken app rather
   * than as a service that cannot answer.
   */
  it("draws only entries the declared capabilities can reach", () => {
    const nav = navAvailability(state(KINESIS_CAPABILITIES, KINESIS_DEGRADED), true);
    for (const page of drawn) {
      expect(nav.visible(page), page).toBe(true);
      expect(nav.disabled(page), page).toBe(false);
    }
  });

  /*
   * The consumers page is drawn and usable even though the backlog is
   * degraded, and that pair is the whole point of the middle state: the
   * registrations are real objects that can be listed, created and removed,
   * and only the number that would go beside them is missing.
   */
  it("draws consumers enabled while the backlog stays degraded", () => {
    const nav = navAvailability(state(KINESIS_CAPABILITIES, KINESIS_DEGRADED), true);

    expect(nav.visible("consumers")).toBe(true);
    expect(nav.disabled("consumers")).toBe(false);
    expect(KINESIS_CAPABILITIES).not.toContain(Capability.CapSubscriptionLag);
    expect(KINESIS_DEGRADED[Capability.CapSubscriptionLag]).toBeDefined();
  });

  /*
   * Alerts, which hangs on the destination listing rather than on cluster
   * metrics: Kinesis declares none - there is no node to attribute a figure to
   * - and gating on one would hide a page that works.
   */
  it("reaches alerts without any cluster capability", () => {
    const nav = navAvailability(state(KINESIS_CAPABILITIES, KINESIS_DEGRADED), true);

    expect(KINESIS_CAPABILITIES).not.toContain(Capability.CapClusterMetrics);
    expect(KINESIS_CAPABILITIES).not.toContain(Capability.CapClusterTopology);
    expect(nav.visible("alerts")).toBe(true);
    expect(nav.disabled("alerts")).toBe(false);
  });

  /*
   * The messages page is drawn, and it carries a caveat rather than a degraded
   * reason - the difference between a page that works with a consequence and
   * one that cannot be opened. The consequence is not SQS's: reading takes
   * nothing, it spends the shard's read allowance.
   */
  it("draws the messages page enabled, with its caveat intact", () => {
    const nav = navAvailability(
      state(KINESIS_CAPABILITIES, KINESIS_DEGRADED, {
        [Capability.CapMessageQuery]: "mq.kinesis.caveat.readQuota",
      }),
      true,
    );

    expect(nav.visible("messages")).toBe(true);
    expect(nav.disabled("messages")).toBe(false);
    expect(nav.reason("messages")).toBeUndefined();
  });

  // Before the connection answers, nothing is known: hiding pages that would
  // come back reads worse than showing them and finding out.
  it("draws every entry while the capabilities are still loading", () => {
    const nav = navAvailability({ ...state([]), loading: true }, true);
    for (const page of drawn) {
      expect(nav.visible(page), page).toBe(true);
    }
  });
});
