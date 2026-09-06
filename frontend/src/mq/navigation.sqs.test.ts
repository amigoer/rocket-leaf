import { describe, expect, it } from "vitest";
import { Capability } from "@bindings/model/models";
import { navAvailability } from "./navigation";
import { PROTOCOLS } from "@/design/data/protocols";
import type { CapabilityState } from "./capabilities";

/**
 * The sidebar a real Amazon SQS connection draws.
 *
 * SQS is the family with the least that can vary. It is one managed service
 * with one feature set: a credential that cannot do something fails the call
 * rather than narrowing what the connection reports, so there is no degraded
 * state anywhere and nothing here is conditional.
 *
 * The list below is what `capabilities()` in internal/driver/sqs/conn.go
 * declares. A Go test asserts the driver still declares exactly this, so the
 * two halves cannot drift apart without one of them failing.
 */
const SQS_CAPABILITIES: Capability[] = [
  Capability.CapDestinationList,
  Capability.CapDestinationCreate,
  Capability.CapDestinationUpdate,
  Capability.CapDestinationDelete,
  Capability.CapDestinationPurge,
  Capability.CapMessageQuery,
  Capability.CapPublish,
  Capability.CapDelayedDelivery,
  Capability.CapDeadLetterTopology,
];

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

/** Every page the SQS sidebar is built from, in the order it draws them. */
const drawn = PROTOCOLS.sqs.nav.flatMap((group) => group.items.map((item) => item.id));

describe("the sidebar an SQS connection draws", () => {
  /*
   * The pages SQS has no concept of, asserted twice: absent from the sidebar,
   * and with no capability declared that would bring them back. Either alone
   * would let one return by the other route.
   *
   * Consumers is the one worth naming. SQS has no subscription of any kind - a
   * consumer is whoever calls ReceiveMessage, and the service keeps no record
   * of who that was - so the page would list an empty set forever rather than
   * occasionally. Cluster and clients follow from AWS running the service:
   * there is no node, no session and no process to show. Access control is
   * IAM's, one service further out, and a page editing half of it would claim
   * to control access it cannot see.
   */
  it("draws none of the pages SQS has no concept of", () => {
    const absent = [
      "consumers",
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

    const nav = navAvailability(state(SQS_CAPABILITIES), true);
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
    const nav = navAvailability(state(SQS_CAPABILITIES), true);
    for (const page of drawn) {
      expect(nav.visible(page), page).toBe(true);
      expect(nav.disabled(page), page).toBe(false);
    }
  });

  /*
   * Alerts is the entry this family nearly lost.
   *
   * It used to hang on cluster metrics alone, and SQS declares none - there is
   * no node to attribute a figure to. Every number it reports belongs to a
   * queue, and its alert rules read the queue listing, so gating on a
   * capability it can never declare would have hidden a page that works.
   */
  it("reaches alerts without any cluster capability", () => {
    const nav = navAvailability(state(SQS_CAPABILITIES), true);

    expect(SQS_CAPABILITIES).not.toContain(Capability.CapClusterMetrics);
    expect(SQS_CAPABILITIES).not.toContain(Capability.CapClusterTopology);
    expect(nav.visible("alerts")).toBe(true);
    expect(nav.disabled("alerts")).toBe(false);
  });

  /*
   * The messages page is drawn, and it carries a caveat rather than a degraded
   * reason - which is the difference between a page that works with a
   * consequence and a page that cannot be opened. Browsing goes through
   * ReceiveMessage, the same call a consumer makes.
   */
  it("draws the messages page enabled, with its caveat intact", () => {
    const nav = navAvailability(
      state(SQS_CAPABILITIES, {}, { [Capability.CapMessageQuery]: "mq.sqs.caveat.receiveHides" }),
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
