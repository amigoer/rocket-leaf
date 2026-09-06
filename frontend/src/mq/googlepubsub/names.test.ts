import { describe, expect, it } from "vitest";
import { nameProblem, submittableName } from "./names";

/**
 * The rules are the service's, and each one below refuses something a person
 * would otherwise type and get an INVALID_ARGUMENT for - naming "Resource
 * name" and no character, which says nothing about which clause was broken.
 */
describe("what Pub/Sub accepts as a name", () => {
  it("accepts an ordinary name", () => {
    expect(nameProblem("orders")).toBeNull();
    expect(nameProblem("orders-dead-letters")).toBeNull();
    expect(nameProblem("team.orders_v2~1+a%20")).toBeNull();
  });

  it("refuses a name shorter than three characters", () => {
    expect(nameProblem("")).toBe("empty");
    expect(nameProblem("ab")).toBe("tooShort");
    expect(nameProblem("abc")).toBeNull();
  });

  it("refuses a name longer than 255 characters", () => {
    expect(nameProblem("a".repeat(255))).toBeNull();
    expect(nameProblem("a".repeat(256))).toBe("tooLong");
  });

  // A digit or a hyphen first is refused by the service, and a slash would
  // address something else entirely: every call takes a full resource path
  // and the name is its last segment.
  it("refuses a name that does not start with a letter", () => {
    expect(nameProblem("1orders")).toBe("charset");
    expect(nameProblem("-orders")).toBe("charset");
    expect(nameProblem("team/orders")).toBe("charset");
  });

  // Reserved by Google for its own resources, in any case, and nothing in the
  // service's own refusal says so.
  it("refuses the prefix Google keeps for itself", () => {
    expect(nameProblem("goog-orders")).toBe("reserved");
    expect(nameProblem("GoOgle-orders")).toBe("reserved");
    expect(nameProblem("googol")).toBe("reserved");
    expect(nameProblem("go-orders")).toBeNull();
  });

  it("submits the trimmed name, or nothing at all", () => {
    expect(submittableName("  orders  ")).toBe("orders");
    expect(submittableName("  ab  ")).toBeNull();
  });
});
