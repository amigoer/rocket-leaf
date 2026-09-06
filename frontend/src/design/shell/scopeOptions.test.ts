import { describe, expect, it } from "vitest";
import { scopeKeys, scopeOptions } from "./scopeOptions";

/*
 * What the switcher's popover offers for a query.
 *
 * The case that matters is a name the cluster has never carried: a RocketMQ
 * namespace is a prefix rather than an object, so the one you are about to
 * create the first topic in is invisible to the listing and still perfectly
 * usable. It has to stay reachable, and it must not be offered twice.
 */
const scope = (name: string) => ({ name, destinations: 0, subscriptions: 0 });

describe("what the popover offers", () => {
  it("offers a typed name the cluster has never carried", () => {
    const options = scopeOptions([scope("orders"), scope("audit")], "billing");
    expect(options.matched).toHaveLength(0);
    expect(options.typed).toBe("billing");
  });

  // Otherwise the same namespace appears twice, once as itself and once as
  // "switch to ...", which reads as two different destinations.
  it("does not offer a typed name the listing already holds", () => {
    const options = scopeOptions([scope("orders"), scope("audit")], "orders");
    expect(options.matched.map((entry) => entry.name)).toEqual(["orders"]);
    expect(options.typed).toBe("");
  });

  it("keeps a partial query as both a filter and an offer", () => {
    const options = scopeOptions([scope("orders"), scope("audit")], "ord");
    expect(options.matched.map((entry) => entry.name)).toEqual(["orders"]);
    expect(options.typed).toBe("ord");
  });

  it("offers nothing extra for an empty query", () => {
    const options = scopeOptions([scope("orders")], "   ");
    expect(options.matched.map((entry) => entry.name)).toEqual(["orders"]);
    expect(options.typed).toBe("");
  });
});

/*
 * The switcher's copy, which is RocketMQ's until a family says otherwise.
 *
 * "All namespaces" is true of an unscoped RocketMQ connection and false of an
 * unnamed Solace profile, which is resolved to a single Message VPN at dial
 * time - so the shared line cannot be the only one. What this pins is that a
 * family's own key is preferred and the shared one is still the fallback, so a
 * family overriding one line does not have to override the other nine.
 */
describe("the switcher's wording", () => {
  it("prefers the family's own line and falls back to the shared one", () => {
    expect(scopeKeys("solace", "unscoped")).toEqual([
      "mq.solace.scope.unscoped",
      "shell.scope.unscoped",
    ]);
    expect(scopeKeys("rocketmq", "label")).toEqual([
      "mq.rocketmq.scope.label",
      "shell.scope.label",
    ]);
  });

  // Nothing connected is not a family, and a key of "mq..scope.x" would
  // resolve to nothing at all rather than to the shared line.
  it("asks only for the shared line when there is no family", () => {
    expect(scopeKeys(undefined, "unscoped")).toEqual(["shell.scope.unscoped"]);
    expect(scopeKeys("", "unscoped")).toEqual(["shell.scope.unscoped"]);
  });
});
