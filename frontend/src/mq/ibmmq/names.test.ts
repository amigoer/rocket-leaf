import { describe, expect, it } from "vitest";
import { MAX_OBJECT_NAME, objectNameProblem, reservedName, submittableObjectName } from "./names";

/*
 * The rule is the queue manager's, and it is checked here for one reason: the
 * command server refuses a bad name with a syntax error naming a character
 * position, which reads as a broken app rather than as a name that was never
 * allowed.
 */
describe("an IBM MQ object name", () => {
  it("takes letters, digits and the four punctuation characters MQ allows", () => {
    for (const name of ["APP.ORDERS", "app_orders", "a/b", "q%1", "Q1"]) {
      expect(objectNameProblem(name), name).toBeNull();
      expect(submittableObjectName(name)).toBe(name);
    }
  });

  it("refuses a space, which is the mistake somebody actually makes", () => {
    expect(objectNameProblem("APP ORDERS")).toBe("characters");
    expect(submittableObjectName("APP ORDERS")).toBeNull();
  });

  it("refuses the characters that look allowed and are not", () => {
    for (const name of ["app-orders", "app:orders", "app*", "app,orders"]) {
      expect(objectNameProblem(name), name).toBe("characters");
    }
  });

  it("stops at forty-eight characters", () => {
    expect(objectNameProblem("Q".repeat(MAX_OBJECT_NAME))).toBeNull();
    expect(objectNameProblem("Q".repeat(MAX_OBJECT_NAME + 1))).toBe("tooLong");
  });

  it("trims before judging, because a trailing space is not a name", () => {
    expect(objectNameProblem("  APP.ORDERS  ")).toBeNull();
    expect(submittableObjectName("  APP.ORDERS  ")).toBe("APP.ORDERS");
    expect(objectNameProblem("   ")).toBe("empty");
  });

  // SYSTEM. is reserved and the queue manager refuses to define anything under
  // it, so a form offering one would offer a create that always fails.
  it("knows the prefix the queue manager keeps for itself", () => {
    expect(reservedName("SYSTEM.DEFAULT.LOCAL.QUEUE")).toBe(true);
    expect(reservedName("system.mine")).toBe(true);
    expect(reservedName("APP.SYSTEM.ORDERS")).toBe(false);
  });
});
