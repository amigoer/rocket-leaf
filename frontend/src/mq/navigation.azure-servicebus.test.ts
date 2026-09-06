import { describe, expect, it } from "vitest";
import { Capability } from "@bindings/model/models";
import { navAvailability } from "./navigation";
import { PROTOCOLS } from "@/design/data/protocols";
import type { CapabilityState } from "./capabilities";

/**
 * The sidebar a real Azure Service Bus connection draws.
 *
 * Service Bus is a managed service with one feature set: a credential that
 * cannot do something fails the call rather than narrowing what the connection
 * reports. One thing does vary, and it varies by endpoint rather than by
 * credential - the emulator reports no message counts at all, so a connection
 * to one degrades the subscription backlog and a connection to a real
 * namespace does not.
 *
 * The list below is what `capabilities()` in
 * internal/driver/azureservicebus/conn.go declares. A Go test asserts the
 * driver still declares exactly this, so the two halves cannot drift apart
 * without one of them failing.
 */
const SERVICEBUS_CAPABILITIES: Capability[] = [
  Capability.CapDestinationList,
  Capability.CapDestinationCreate,
  Capability.CapDestinationUpdate,
  Capability.CapDestinationDelete,
  Capability.CapSubscriptionList,
  Capability.CapSubscriptionCreate,
  Capability.CapSubscriptionDelete,
  Capability.CapSubscriptionLag,
  Capability.CapMessageQuery,
  Capability.CapPublish,
  Capability.CapDelayedDelivery,
  Capability.CapDLQ,
  Capability.CapMessageResend,
  Capability.CapRouting,
  Capability.CapRoutingAdmin,
];

/**
 * What an emulator connection reports instead of the backlog.
 *
 * Only an emulator: a real namespace answers the count, which is why this is a
 * narrowing in declare() rather than a family-wide gap.
 */
const EMULATOR_DEGRADED: Partial<Record<Capability, string>> = {
  [Capability.CapSubscriptionLag]: "mq.azure-servicebus.degraded.countsNotInEmulator",
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

/** Every page the Service Bus sidebar is built from, in the order it draws them. */
const drawn = PROTOCOLS["azure-servicebus"].nav.flatMap((group) =>
  group.items.map((item) => item.id),
);

describe("the sidebar an Azure Service Bus connection draws", () => {
  /*
   * The pages Service Bus has no concept of, asserted twice: absent from the
   * sidebar, and with no capability declared that would bring them back.
   * Either alone would let one return by the other route.
   *
   * Cluster and clients follow from Microsoft running the service: there is no
   * node, no session and no process to show. Access control is a shared access
   * policy on the namespace, which is what this connection authenticated with
   * rather than something it can enumerate. There is no vhost inside a
   * namespace, no policy applied by pattern, no quota this API can read, and
   * no live subscribe page - a receiver takes messages, which is the opposite
   * of what this family's browse is for.
   */
  it("draws none of the pages Service Bus has no concept of", () => {
    const absent = [
      "clients",
      "cluster",
      "acl",
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

    const nav = navAvailability(state(SERVICEBUS_CAPABILITIES), true);
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
    const nav = navAvailability(state(SERVICEBUS_CAPABILITIES), true);
    for (const page of drawn) {
      expect(nav.visible(page), page).toBe(true);
      expect(nav.disabled(page), page).toBe(false);
    }
  });

  /*
   * The routing page is what this family adds over the two hosted ones before
   * it, and it is the only page in the app RabbitMQ used to have to itself.
   *
   * It is here because a rule is an object rather than a field: it has a name,
   * several may sit on one subscription, and each is a filter plus an optional
   * action. Which messages reach which subscription is therefore a topology.
   */
  it("draws the routing page, which no other hosted family has", () => {
    const nav = navAvailability(state(SERVICEBUS_CAPABILITIES), true);

    expect(drawn).toContain("exchanges");
    expect(nav.visible("exchanges")).toBe(true);
    expect(nav.disabled("exchanges")).toBe(false);

    // And it goes when the capability does, rather than being drawn for a
    // connection that cannot read a rule.
    const without = navAvailability(
      state(SERVICEBUS_CAPABILITIES.filter((c) => c !== Capability.CapRouting)),
      true,
    );
    expect(without.visible("exchanges")).toBe(false);
  });

  /*
   * The subscriptions page survives the backlog being unreadable.
   *
   * It is gated on the listing capability rather than on lag, which is what
   * keeps it reachable against an emulator: every emulator connection degrades
   * the backlog, and gating on it would hide a page whose whole other half -
   * creating, describing and deleting subscriptions - works perfectly.
   */
  it("reaches the subscriptions page even where the backlog is not readable", () => {
    const emulator = navAvailability(
      state(
        SERVICEBUS_CAPABILITIES.filter((c) => c !== Capability.CapSubscriptionLag),
        EMULATOR_DEGRADED,
      ),
      true,
    );

    expect(drawn).toContain("consumers");
    expect(emulator.visible("consumers")).toBe(true);
    expect(emulator.disabled("consumers")).toBe(false);
  });

  /*
   * Alerts is the entry this family would otherwise lose.
   *
   * It hangs on cluster metrics or a destination listing, and Service Bus
   * declares no cluster capability at all - Microsoft runs the service and
   * shows no node. Its alert rules read the entity and subscription listings,
   * so gating on a metric it can never declare would have hidden a page that
   * works.
   */
  it("reaches alerts without any cluster capability", () => {
    const nav = navAvailability(state(SERVICEBUS_CAPABILITIES), true);

    expect(SERVICEBUS_CAPABILITIES).not.toContain(Capability.CapClusterMetrics);
    expect(SERVICEBUS_CAPABILITIES).not.toContain(Capability.CapClusterTopology);
    expect(nav.visible("alerts")).toBe(true);
    expect(nav.disabled("alerts")).toBe(false);
  });

  /*
   * The messages page is drawn enabled and carries no caveat, which is the one
   * thing this family does that neither hosted family before it could.
   *
   * SQS's browse hides what it read and Pub/Sub's raises a delivery attempt,
   * so both pages warn. A peek takes nothing, so there is nothing to warn
   * about - and the absence is asserted here as well as in Go, because the way
   * it would be lost is silent.
   */
  it("draws the messages page enabled and with nothing to warn about", () => {
    const capabilities = state(SERVICEBUS_CAPABILITIES);
    const nav = navAvailability(capabilities, true);

    expect(nav.visible("messages")).toBe(true);
    expect(nav.disabled("messages")).toBe(false);
    expect(nav.reason("messages")).toBeUndefined();
    // And nothing behind it either: the caveat a page would show comes off the
    // capability, and this family declares none for browsing.
    expect(capabilities.caveat(Capability.CapMessageQuery)).toBeUndefined();
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
