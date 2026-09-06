import { describe, expect, it } from "vitest";
import { topicProblem } from "./routing";

/*
 * The wildcard rules, which are the reason this check exists at all.
 *
 * Solace's two wildcards look like a glob and are positional: "*" is one whole
 * level or the tail of one, and ">" is the rest of the topic and only ever the
 * last character. A pattern that breaks either is accepted by the broker as a
 * literal topic and then matches nothing - a subscription that is there, looks
 * right on every listing, and is dead.
 */
describe("a Solace subscription topic", () => {
  it("takes a plain topic and both wildcards where they are allowed", () => {
    for (const topic of [
      "orders/eu/created",
      "orders/>",
      "orders/*/created",
      "orders/eu*/created",
      "*",
      ">",
    ]) {
      expect(topicProblem(topic), topic).toBeNull();
    }
  });

  it("refuses a > anywhere but the end, where it means nothing", () => {
    expect(topicProblem("orders/>/created")).toBe("trailingWildcardPlacement");
    expect(topicProblem("or>ders")).toBe("trailingWildcardPlacement");
  });

  it("refuses a * inside a level, which looks like a glob and is not one", () => {
    expect(topicProblem("orders/*eu/created")).toBe("midLevelWildcard");
    expect(topicProblem("orders/e*u")).toBe("midLevelWildcard");
  });

  it("refuses an empty topic", () => {
    expect(topicProblem("   ")).toBe("empty");
  });
});
