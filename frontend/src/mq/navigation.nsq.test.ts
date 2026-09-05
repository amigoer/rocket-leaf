import { describe, expect, it } from "vitest";
import { Capability } from "@bindings/model/models";
import { navAvailability } from "./navigation";
import { PROTOCOLS } from "@/design/data/protocols";
import type { CapabilityState } from "./capabilities";

/**
 * The sidebar a real NSQ connection draws.
 *
 * NSQ is the family with the least that can be missing. One optional tier
 * exists - nsqlookupd, which a single-node deployment simply does not run -
 * and everything else is either there or the connection did not open: a daemon
 * that stopped answering fails the read rather than degrading a page.
 *
 * The list below is what `capabilities()` in internal/driver/nsq/conn.go
 * declares. A Go test asserts the driver still declares exactly this, so the
 * two halves cannot drift apart without one of them failing. It grows one
 * entry at a time: a capability with no port behind it fails conformance, so
 * each arrives in the commit that implements it.
 */
const NSQ_CAPABILITIES: Capability[] = [
  Capability.CapDestinationList,
  Capability.CapDestinationCreate,
  Capability.CapDestinationDelete,
  Capability.CapDestinationPurge,
  Capability.CapSubscriptionList,
  Capability.CapSubscriptionCreate,
  Capability.CapSubscriptionDelete,
  Capability.CapSubscriptionLag,
  Capability.CapPublish,
  Capability.CapDelayedDelivery,
  Capability.CapClusterTopology,
  Capability.CapClusterMetrics,
  Capability.CapNodeConfig,
  Capability.CapClientInspect,
  Capability.CapDirectory,
];

function state(
  supported: Capability[],
  degraded: Partial<Record<Capability, string>> = {},
): CapabilityState {
  return {
    has: (capability) => supported.includes(capability),
    degradedReason: (capability) => degraded[capability],
    caveat: () => undefined,
    loading: false,
  };
}

/** Every page the NSQ sidebar is built from, in the order it draws them. */
const drawn = PROTOCOLS.nsq.nav.flatMap((group) => group.items.map((item) => item.id));

describe("the sidebar an NSQ connection draws", () => {
  /*
   * The pages NSQ has no concept of, asserted twice: absent from the sidebar,
   * and with no capability declared that would bring them back. Either alone
   * would let one return by the other route.
   *
   * Messages is the one worth naming. nsqd hands a message to a consumer and
   * stops holding it - there is no stored log behind a depth, no id anything
   * indexes, and no call that reads one back - so a browse page here would be
   * permanently empty rather than occasionally so. Dead letters follows from
   * the same fact: nothing is moved aside when a consumer gives up, it is
   * dropped.
   */
  it("draws none of the pages NSQ has no concept of", () => {
    for (const absent of ["messages", "dlq", "acl", "exchanges", "vhosts", "policies", "quotas"]) {
      expect(drawn).not.toContain(absent);
    }

    const nav = navAvailability(state(NSQ_CAPABILITIES), true);
    for (const absent of ["messages", "dlq", "acl", "exchanges", "vhosts", "policies", "quotas"]) {
      expect(nav.visible(absent), absent).toBe(false);
    }
  });

  /*
   * Every entry the sidebar draws has to be reachable from the capabilities
   * the driver declares. An entry drawn with nothing behind it opens onto a
   * page that fails when it asks for data, which reads as a broken app rather
   * than as a broker that cannot answer.
   */
  it("draws only entries the declared capabilities can reach", () => {
    const nav = navAvailability(state(NSQ_CAPABILITIES), true);
    for (const page of drawn) {
      expect(nav.visible(page), page).toBe(true);
    }
  });

  /*
   * The one thing a connection can be without. nsqlookupd is a separate daemon
   * and a profile that names none has no discovery tier to describe - so the
   * cluster page stays, because the daemons are still there, and the reason
   * the tier is missing has to be readable rather than the page silently
   * showing one table instead of two.
   */
  it("keeps the cluster page when no discovery tier is configured", () => {
    const withoutDirectory = NSQ_CAPABILITIES.filter(
      (capability) => capability !== Capability.CapDirectory,
    );
    const nav = navAvailability(
      state(withoutDirectory, {
        [Capability.CapDirectory]: "mq.nsq.degraded.lookupdAbsent",
      }),
      true,
    );
    expect(nav.visible("cluster")).toBe(true);
    expect(nav.disabled("cluster")).toBe(false);
  });
});
