import { describe, expect, it } from "vitest";
import { nameProblem, submittableName } from "./names";

/**
 * The rule is the service's own, and checking it here is not politeness: a
 * refused name comes back as a 400 whose detail is "SubCode=40000" and a
 * tracking id, naming neither the character nor the field.
 */
describe("what Service Bus will take as an entity name", () => {
  it("accepts the shapes the service documents", () => {
    for (const name of ["orders", "team.orders", "team-orders_2", "orders/retry", "a"]) {
      expect(nameProblem(name)).toBeNull();
    }
  });

  it("refuses a name that addresses a sub-entity", () => {
    // $DeadLetterQueue and $Transfer/$DeadLetterQueue are reached by suffixing
    // an entity path, so a name carrying one would silently read something
    // else - and the dead letters have a page that addresses them on purpose.
    expect(nameProblem("$DeadLetterQueue")).toBe("reserved");
    expect(nameProblem("orders/$DeadLetterQueue")).toBe("reserved");
  });

  it("refuses a leading, trailing or doubled separator", () => {
    expect(nameProblem("/orders")).toBe("separator");
    expect(nameProblem("orders/")).toBe("separator");
    expect(nameProblem("orders//retry")).toBe("separator");
  });

  it("refuses characters the service does not take", () => {
    expect(nameProblem("orders queue")).toBe("charset");
    expect(nameProblem("orders#1")).toBe("charset");
    expect(nameProblem("-orders")).toBe("charset");
  });

  it("holds the two scopes to their own length limits", () => {
    const long = "a".repeat(261);
    expect(nameProblem(long)).toBe("tooLong");
    expect(nameProblem("a".repeat(260))).toBeNull();
    // A subscription and a rule are far shorter, and they cannot be nested:
    // a slash in one addresses something that does not exist.
    expect(nameProblem("a".repeat(51), "child")).toBe("tooLong");
    expect(nameProblem("a".repeat(50), "child")).toBeNull();
    expect(nameProblem("worker/retry", "child")).toBe("charset");
  });

  it("trims what it submits and refuses what it will not", () => {
    expect(submittableName("  orders  ")).toBe("orders");
    expect(submittableName("   ")).toBeNull();
    expect(submittableName("$DeadLetterQueue")).toBeNull();
  });
});
