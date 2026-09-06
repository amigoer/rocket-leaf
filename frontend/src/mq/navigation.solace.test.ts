import { describe, expect, it } from "vitest";
import { Capability } from "@bindings/model/models";
import { navAvailability } from "./navigation";
import { PROTOCOLS } from "@/design/data/protocols";
import type { CapabilityState } from "./capabilities";

/**
 * The sidebar a real Solace connection draws.
 *
 * Two ports answer this family and they are not the same interface: SEMP
 * manages the broker on 8080 and carries no message data at all, and the REST
 * messaging interface on another port is what a send goes through. So the
 * conditional half is real rather than theoretical - a Message VPN that does
 * not serve REST, or a client username it refuses, leaves every board except
 * the send console.
 *
 * The list below is what `capabilities()` in internal/driver/solace/conn.go
 * declares. A Go test asserts the driver still declares exactly this, so the
 * two halves cannot drift apart without one of them failing.
 */
const SOLACE_CAPABILITIES: Capability[] = [
  Capability.CapDestinationList,
  Capability.CapDestinationCreate,
  Capability.CapDestinationDelete,
  Capability.CapConnectionScope,
  Capability.CapMessageQuery,
  Capability.CapMessageByID,
  Capability.CapPublish,
  Capability.CapDeadLetterTopology,
  Capability.CapRouting,
  Capability.CapRoutingAdmin,
  Capability.CapClusterTopology,
  Capability.CapClusterMetrics,
  Capability.CapClientInspect,
];

/** What the REST messaging interface answers, and what goes with it. */
const MESSAGING: Capability[] = [Capability.CapPublish];

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

/** Every page the Solace sidebar is built from, in the order it draws them. */
const drawn = PROTOCOLS.solace.nav.flatMap((group) => group.items.map((item) => item.id));

describe("the sidebar a Solace connection draws", () => {
  /*
   * The routing page, which this family has the strongest claim to of the
   * three that draw one: a Solace publisher never names a queue at all, so
   * what has subscribed is the whole of what decides where a message lands.
   */
  it("draws the routing page, and reaches it only through CapRouting", () => {
    expect(drawn).toContain("exchanges");
    expect(SOLACE_CAPABILITIES).toContain(Capability.CapRouting);

    const nav = navAvailability(state(SOLACE_CAPABILITIES), true);
    expect(nav.visible("exchanges")).toBe(true);
    expect(nav.disabled("exchanges")).toBe(false);

    const without = navAvailability(
      state(SOLACE_CAPABILITIES.filter((c) => c !== Capability.CapRouting)),
      true,
    );
    expect(without.visible("exchanges")).toBe(false);
  });

  /*
   * The pages Solace has no concept of, asserted twice: absent from the
   * sidebar, and with no capability declared that would bring them back.
   *
   * Consumers is the one worth naming. There is no consumer group anywhere in
   * this product: what reads a queue is a client bound to it, which is the
   * clients page - so a subscriptions board would have nothing to list.
   * Channels is IBM MQ's and has no counterpart: a Solace flow is per endpoint
   * rather than a configured object a connection has to come through.
   */
  it("draws none of the pages Solace has no concept of", () => {
    const absent = [
      "consumers",
      "channels",
      "acl",
      "vhosts",
      "policies",
      "quotas",
      "shards",
      "subscribe",
      "definitions",
      "replication",
    ];
    for (const page of absent) {
      expect(drawn).not.toContain(page);
    }
    expect(SOLACE_CAPABILITIES).not.toContain(Capability.CapSubscriptionList);
    expect(SOLACE_CAPABILITIES).not.toContain(Capability.CapChannels);

    const nav = navAvailability(state(SOLACE_CAPABILITIES), true);
    for (const page of absent) {
      expect(nav.visible(page), page).toBe(false);
    }
  });

  /*
   * Every entry the sidebar draws has to be reachable from the capabilities
   * the driver declares. An entry drawn with nothing behind it opens onto a
   * page that fails when it asks for data, which reads as a broken app rather
   * than as a server that cannot answer.
   */
  it("draws only entries the declared capabilities can reach", () => {
    const nav = navAvailability(state(SOLACE_CAPABILITIES), true);
    for (const page of drawn) {
      expect(nav.visible(page), page).toBe(true);
      expect(nav.disabled(page), page).toBe(false);
    }
  });

  /*
   * The dead-letter page, reached through the topology capability rather than
   * through CapDLQ. Nothing on this broker is marked as a dead message queue:
   * what makes one is another endpoint's deadMsgQueue pointer, which is a walk
   * backwards rather than a lookup.
   */
  it("reaches dead letters through the topology capability", () => {
    const nav = navAvailability(state(SOLACE_CAPABILITIES), true);

    expect(drawn).toContain("dlq");
    expect(SOLACE_CAPABILITIES).toContain(Capability.CapDeadLetterTopology);
    expect(SOLACE_CAPABILITIES).not.toContain(Capability.CapDLQ);
    expect(nav.visible("dlq")).toBe(true);
    expect(nav.disabled("dlq")).toBe(false);
  });

  /*
   * The scope switcher, which is what makes a Message VPN a selector rather
   * than part of the address. The shell draws it from the capability alone, so
   * a family that declared it without a ScopeOption on its descriptor would
   * draw a switch that refuses every switch - which the driver's own
   * conformance test covers from the other side.
   */
  it("carries a switchable scope", () => {
    expect(SOLACE_CAPABILITIES).toContain(Capability.CapConnectionScope);
  });

  /*
   * The half that is genuinely conditional on this family, and the reason the
   * connection probes at all: SEMP carries no message data, so a send goes to
   * a different port with a credential from a different directory. Everything
   * read from SEMP keeps working when that port does not answer.
   */
  it("keeps every semp page when the rest messaging interface is unreachable", () => {
    const readable = SOLACE_CAPABILITIES.filter((c) => !MESSAGING.includes(c));
    const degraded = Object.fromEntries(
      MESSAGING.map((c) => [c, "mq.solace.degraded.restUnreachable"]),
    ) as Partial<Record<Capability, string>>;

    const nav = navAvailability(state(readable, degraded), true);

    for (const page of ["topics", "exchanges", "messages", "clients", "dlq", "cluster", "alerts"]) {
      expect(nav.visible(page), page).toBe(true);
      expect(nav.disabled(page), page).toBe(false);
    }
    // Drawn and explained rather than gone: the family has the concept and
    // this endpoint cannot reach it, which is what the middle state is for.
    expect(nav.visible("producer")).toBe(true);
    expect(nav.disabled("producer")).toBe(true);
    expect(nav.reason("producer")).toBe("mq.solace.degraded.restUnreachable");
  });

  /*
   * The messages page carries a caveat rather than a degraded reason on a
   * working connection - the difference between a page that works with a
   * consequence and one that cannot be opened. The consequence is not SQS's or
   * Pub/Sub's: a browse here takes nothing at all, and what it cannot do is
   * show the message.
   */
  it("draws the messages page enabled, with its caveat intact", () => {
    const nav = navAvailability(
      state(SOLACE_CAPABILITIES, {}, {
        [Capability.CapMessageQuery]: "mq.solace.caveat.browseNoPayload",
      }),
      true,
    );

    expect(nav.visible("messages")).toBe(true);
    expect(nav.disabled("messages")).toBe(false);
    expect(nav.reason("messages")).toBeUndefined();
  });

  /*
   * Alerts, which this family reaches through cluster metrics rather than only
   * through the destination listing: unlike IBM MQ there is a broker row here,
   * and its spool percentage is one of the six rules.
   */
  it("reaches alerts through the cluster metrics it does declare", () => {
    const nav = navAvailability(state(SOLACE_CAPABILITIES), true);

    expect(SOLACE_CAPABILITIES).toContain(Capability.CapClusterMetrics);
    expect(nav.visible("alerts")).toBe(true);
    expect(nav.disabled("alerts")).toBe(false);
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
