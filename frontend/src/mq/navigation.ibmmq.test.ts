import { describe, expect, it } from "vitest";
import { Capability } from "@bindings/model/models";
import { navAvailability } from "./navigation";
import { PROTOCOLS } from "@/design/data/protocols";
import type { CapabilityState } from "./capabilities";

/**
 * The sidebar a real IBM MQ connection draws.
 *
 * Two interfaces answer this family and they authorise separately, so the
 * conditional half is real here rather than theoretical: an mqweb account
 * mapped only to MQWebAdmin reaches every board except the two that touch
 * messages, and IBM's own developer image ships exactly that account.
 *
 * The list below is what `capabilities()` in internal/driver/ibmmq/conn.go
 * declares. A Go test asserts the driver still declares exactly this, so the
 * two halves cannot drift apart without one of them failing.
 */
const IBMMQ_CAPABILITIES: Capability[] = [
  Capability.CapDestinationList,
  Capability.CapDestinationCreate,
  Capability.CapDestinationDelete,
  Capability.CapChannels,
  Capability.CapMessageQuery,
  Capability.CapMessageByID,
  Capability.CapPublish,
  Capability.CapDeadLetterTopology,
  Capability.CapSubscriptionList,
  Capability.CapSubscriptionLag,
];

/** What the messaging interface answers, and what goes with it when it cannot. */
const MESSAGING: Capability[] = [
  Capability.CapMessageQuery,
  Capability.CapMessageByID,
  Capability.CapPublish,
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

/** Every page the IBM MQ sidebar is built from, in the order it draws them. */
const drawn = PROTOCOLS.ibmmq.nav.flatMap((group) => group.items.map((item) => item.id));

describe("the sidebar an IBM MQ connection draws", () => {
  /*
   * The page no other family has, asserted from both sides: the sidebar draws
   * it, and it is reachable only through the capability that exists for it.
   *
   * Not CapClientInspect. That capability's page lists the connections open
   * right now; a channel is the definition they have to come through, it is
   * there with nothing connected, and one of them carries many connections at
   * once - so a page built for connections would be empty on a queue manager
   * whose applications are all idle.
   */
  it("draws the channels page, and reaches it only through CapChannels", () => {
    expect(drawn).toContain("channels");
    expect(IBMMQ_CAPABILITIES).toContain(Capability.CapChannels);
    expect(IBMMQ_CAPABILITIES).not.toContain(Capability.CapClientInspect);

    const nav = navAvailability(state(IBMMQ_CAPABILITIES), true);
    expect(nav.visible("channels")).toBe(true);
    expect(nav.disabled("channels")).toBe(false);

    // Without it the entry is not drawn at all, which is what keeps the page
    // out of every other family's sidebar.
    const without = navAvailability(
      state(IBMMQ_CAPABILITIES.filter((c) => c !== Capability.CapChannels)),
      true,
    );
    expect(without.visible("channels")).toBe(false);
  });

  /*
   * The pages IBM MQ has no concept of, asserted twice: absent from the
   * sidebar, and with no capability declared that would bring them back.
   *
   * Cluster is the one worth naming. An MQ cluster is a set of queue managers
   * publishing to each other's repositories, and this connection speaks to one
   * of them - so a topology board would have exactly one invented row on it.
   * Access control is per object and per principal, which is a page of its own
   * rather than a column this driver could fill from a listing.
   */
  it("draws none of the pages IBM MQ has no concept of", () => {
    const absent = [
      "clients",
      "cluster",
      "acl",
      "exchanges",
      "vhosts",
      "policies",
      "quotas",
      "shards",
      "subscribe",
    ];
    for (const page of absent) {
      expect(drawn).not.toContain(page);
    }

    const nav = navAvailability(state(IBMMQ_CAPABILITIES), true);
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
    const nav = navAvailability(state(IBMMQ_CAPABILITIES), true);
    for (const page of drawn) {
      expect(nav.visible(page), page).toBe(true);
      expect(nav.disabled(page), page).toBe(false);
    }
  });

  /*
   * The dead-letter page, reached through the topology capability rather than
   * through CapDLQ. Nothing on a queue manager is marked as a dead-letter
   * queue: what makes one is the queue manager's DEADQ attribute or another
   * queue's backout queue pointing at it, which is a walk backwards rather
   * than a lookup.
   */
  it("reaches dead letters through the topology capability", () => {
    const nav = navAvailability(state(IBMMQ_CAPABILITIES), true);

    expect(drawn).toContain("dlq");
    expect(IBMMQ_CAPABILITIES).toContain(Capability.CapDeadLetterTopology);
    expect(IBMMQ_CAPABILITIES).not.toContain(Capability.CapDLQ);
    expect(nav.visible("dlq")).toBe(true);
    expect(nav.disabled("dlq")).toBe(false);
  });

  /*
   * The half that is genuinely conditional on this family, and the reason the
   * connection probes at all: mqweb authorises its two REST interfaces
   * against two roles, and a credential holding only the administrative one
   * reaches every board except messages and send.
   */
  it("keeps every administrative page when the messaging interface refuses the credential", () => {
    const administrative = IBMMQ_CAPABILITIES.filter((c) => !MESSAGING.includes(c));
    const degraded = Object.fromEntries(
      MESSAGING.map((c) => [c, "mq.ibmmq.degraded.messagingForbidden"]),
    ) as Partial<Record<Capability, string>>;

    const nav = navAvailability(state(administrative, degraded), true);

    for (const page of ["topics", "channels", "consumers", "dlq", "alerts", "overview"]) {
      expect(nav.visible(page), page).toBe(true);
      expect(nav.disabled(page), page).toBe(false);
    }
    // Drawn and explained rather than gone: the family has the concept and
    // this credential cannot use it, which is what the middle state is for.
    for (const page of ["messages", "producer"]) {
      expect(nav.visible(page), page).toBe(true);
      expect(nav.disabled(page), page).toBe(true);
      expect(nav.reason(page), page).toBe("mq.ibmmq.degraded.messagingForbidden");
    }
  });

  /*
   * The messages page carries a caveat rather than a degraded reason on a
   * working connection - the difference between a page that works with a
   * consequence and one that cannot be opened. The consequence is not SQS's
   * or Pub/Sub's: a browse here takes nothing at all.
   */
  it("draws the messages page enabled, with its caveat intact", () => {
    const nav = navAvailability(
      state(IBMMQ_CAPABILITIES, {}, {
        [Capability.CapMessageQuery]: "mq.ibmmq.caveat.browseCharacterOnly",
      }),
      true,
    );

    expect(nav.visible("messages")).toBe(true);
    expect(nav.disabled("messages")).toBe(false);
    expect(nav.reason("messages")).toBeUndefined();
  });

  /*
   * Alerts, which hangs on the destination listing rather than on cluster
   * metrics: IBM MQ declares none - there is one queue manager and no node to
   * attribute a figure to - and gating on one would hide a page that works.
   */
  it("reaches alerts without any cluster capability", () => {
    const nav = navAvailability(state(IBMMQ_CAPABILITIES), true);

    expect(IBMMQ_CAPABILITIES).not.toContain(Capability.CapClusterMetrics);
    expect(IBMMQ_CAPABILITIES).not.toContain(Capability.CapClusterTopology);
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
