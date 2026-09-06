import { describe, expect, it } from "vitest";
import { Capability } from "@bindings/model/models";
import { navAvailability } from "./navigation";
import { PROTOCOLS } from "@/design/data/protocols";
import type { CapabilityState } from "./capabilities";

/**
 * The sidebar a real Google Pub/Sub connection draws.
 *
 * Pub/Sub is a managed service with one feature set: a credential that cannot
 * do something fails the call rather than narrowing what the connection
 * reports. So nothing here is conditional on the endpoint, and the one thing
 * that is always narrowed is narrowed for every connection alike - the
 * subscription backlog, which lives in Cloud Monitoring rather than in this
 * API at all.
 *
 * The list below is what `capabilities()` in
 * internal/driver/googlepubsub/conn.go declares. A Go test asserts the driver
 * still declares exactly this, so the two halves cannot drift apart without
 * one of them failing.
 */
const PUBSUB_CAPABILITIES: Capability[] = [
  Capability.CapDestinationList,
  Capability.CapDestinationCreate,
  Capability.CapDestinationUpdate,
  Capability.CapDestinationDelete,
  Capability.CapSubscriptionList,
  Capability.CapSubscriptionCreate,
  Capability.CapSubscriptionDelete,
  Capability.CapSubscriptionPosition,
  Capability.CapOffsetReset,
  Capability.CapMessageQuery,
  Capability.CapPublish,
  Capability.CapDeadLetterTopology,
];

/** The one capability every Pub/Sub connection reports as degraded. */
const DEGRADED: Partial<Record<Capability, string>> = {
  [Capability.CapSubscriptionLag]: "mq.google-pubsub.degraded.lagInMonitoring",
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

/** Every page the Pub/Sub sidebar is built from, in the order it draws them. */
const drawn = PROTOCOLS["google-pubsub"].nav.flatMap((group) =>
  group.items.map((item) => item.id),
);

describe("the sidebar a Google Pub/Sub connection draws", () => {
  /*
   * The pages Pub/Sub has no concept of, asserted twice: absent from the
   * sidebar, and with no capability declared that would bring them back.
   * Either alone would let one return by the other route.
   *
   * Cluster and clients follow from Google running the service: there is no
   * node, no session and no process to show. Access control is IAM's, one
   * service further out, and a page editing half of it would claim to control
   * access it cannot see. There is no exchange, no vhost, no policy and no
   * quota anywhere in the service, and no live subscribe page - what a
   * streaming pull delivers is what an ordinary consumer would have had, which
   * is the browse page's caveat rather than a second kind of read.
   */
  it("draws none of the pages Pub/Sub has no concept of", () => {
    const absent = [
      "clients",
      "cluster",
      "acl",
      "exchanges",
      "vhosts",
      "policies",
      "quotas",
      "subscribe",
      "definitions",
      "replication",
    ];
    for (const page of absent) {
      expect(drawn).not.toContain(page);
    }

    const nav = navAvailability(state(PUBSUB_CAPABILITIES, DEGRADED), true);
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
    const nav = navAvailability(state(PUBSUB_CAPABILITIES, DEGRADED), true);
    for (const page of drawn) {
      expect(nav.visible(page), page).toBe(true);
      expect(nav.disabled(page), page).toBe(false);
    }
  });

  /*
   * The consumers page is the one this family adds over the other addressless
   * one, and it survives the backlog being unreadable.
   *
   * The subscriptions page is gated on the listing capability rather than on
   * lag, which is what keeps it reachable: every Pub/Sub connection degrades
   * the backlog, and gating on it would hide a page whose whole other half -
   * creating, describing and deleting subscriptions - works perfectly.
   */
  it("reaches the subscriptions page although the backlog never is", () => {
    const nav = navAvailability(state(PUBSUB_CAPABILITIES, DEGRADED), true);

    expect(PUBSUB_CAPABILITIES).not.toContain(Capability.CapSubscriptionLag);
    expect(drawn).toContain("consumers");
    expect(nav.visible("consumers")).toBe(true);
    expect(nav.disabled("consumers")).toBe(false);
  });

  /*
   * Alerts is the entry this family would otherwise lose.
   *
   * It hangs on cluster metrics or a destination listing, and Pub/Sub declares
   * no cluster capability at all - Google runs the service and shows no node.
   * Its alert rules read the topic and subscription listings, so gating on a
   * metric it can never declare would have hidden a page that works.
   */
  it("reaches alerts without any cluster capability", () => {
    const nav = navAvailability(state(PUBSUB_CAPABILITIES, DEGRADED), true);

    expect(PUBSUB_CAPABILITIES).not.toContain(Capability.CapClusterMetrics);
    expect(PUBSUB_CAPABILITIES).not.toContain(Capability.CapClusterTopology);
    expect(nav.visible("alerts")).toBe(true);
    expect(nav.disabled("alerts")).toBe(false);
  });

  /*
   * The messages page is drawn, and it carries a caveat rather than a degraded
   * reason - which is the difference between a page that works with a
   * consequence and a page that cannot be opened. Browsing goes through Pull,
   * the same call a consumer makes.
   */
  it("draws the messages page enabled, with its caveat intact", () => {
    const nav = navAvailability(
      state(PUBSUB_CAPABILITIES, DEGRADED, {
        [Capability.CapMessageQuery]: "mq.google-pubsub.caveat.pullDelivers",
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
