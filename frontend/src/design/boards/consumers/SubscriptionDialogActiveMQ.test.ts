import { describe, expect, it } from "vitest";
import { classicSubscriptionRef } from "./SubscriptionDialogActiveMQ";

/**
 * Classic identifies a durable subscription by a pair and the canonical ref
 * carries one string, so the two halves are joined here. Getting the join
 * wrong produces a name the broker accepts on creation and cannot find again.
 */
describe("a classic subscription ref", () => {
  it("joins the client id and the name with a bar", () => {
    expect(classicSubscriptionRef("orders-app", "nightly")).toBe("orders-app|nightly");
  });

  it("trims both halves, because a padded name is a different name", () => {
    expect(classicSubscriptionRef("  orders-app ", " nightly  ")).toBe("orders-app|nightly");
  });

  it("is not whole until both halves are given", () => {
    expect(classicSubscriptionRef("", "nightly")).toBeNull();
    expect(classicSubscriptionRef("orders-app", "")).toBeNull();
    expect(classicSubscriptionRef("   ", "  ")).toBeNull();
  });

  /*
   * The separator inside a client id would make a ref that cannot be split
   * back out: the driver would read the client as everything before the first
   * bar and destroy a subscription nobody asked about. A slash or a colon is
   * fine and common - which is why the separator is a bar in the first place.
   */
  it("refuses a client id containing the separator", () => {
    expect(classicSubscriptionRef("orders|app", "nightly")).toBeNull();
    expect(classicSubscriptionRef("orders/app:1", "nightly")).toBe("orders/app:1|nightly");
  });
});
