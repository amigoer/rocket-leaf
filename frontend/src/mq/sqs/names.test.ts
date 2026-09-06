import { describe, expect, it } from "vitest";
import { isFifoName, nameProblem, submittableName, withFifo } from "./names";

/**
 * The name rule, checked here rather than left to the service.
 *
 * SQS refuses a bad name as InvalidParameterValue naming "QueueName" and no
 * character, and refuses a FIFO mismatch by naming the FifoQueue attribute -
 * a field the form never drew. Neither answer tells the person typing what to
 * change.
 */
describe("an SQS queue name", () => {
  it("accepts letters, digits, hyphens and underscores", () => {
    expect(nameProblem("orders")).toBeNull();
    expect(nameProblem("team-orders_v2")).toBeNull();
    expect(submittableName("  orders  ")).toBe("orders");
  });

  it("refuses an empty name", () => {
    expect(nameProblem("")).toBe("empty");
    expect(nameProblem("   ")).toBe("empty");
    expect(submittableName("")).toBeNull();
  });

  it("refuses anything past eighty characters, suffix included", () => {
    expect(nameProblem("a".repeat(80))).toBeNull();
    expect(nameProblem("a".repeat(81))).toBe("tooLong");
    // The suffix counts towards the eighty rather than sitting outside it.
    expect(nameProblem("a".repeat(76) + ".fifo")).toBe("tooLong");
  });

  /*
   * The dot is the case worth pinning. It is legal in exactly one place - the
   * .fifo suffix - and every other family in this app allows it freely, so a
   * name copied across from one of them is the likeliest way to get this
   * wrong.
   */
  it("allows a dot only in the .fifo suffix", () => {
    expect(nameProblem("orders.fifo")).toBeNull();
    expect(nameProblem("orders.created")).toBe("charset");
    expect(nameProblem("orders.created.fifo")).toBe("charset");
    expect(nameProblem(".fifo")).toBe("empty");
  });

  it("reads FIFO off the name, which is where SQS keeps it", () => {
    expect(isFifoName("orders.fifo")).toBe(true);
    expect(isFifoName("orders-fifo")).toBe(false);
  });

  // A switch that did not change the name would be a control that silently
  // did nothing: whether a queue is FIFO is fixed at creation and spelled in
  // its name.
  it("adds and removes the suffix the switch decides", () => {
    expect(withFifo("orders", true)).toBe("orders.fifo");
    expect(withFifo("orders.fifo", true)).toBe("orders.fifo");
    expect(withFifo("orders.fifo", false)).toBe("orders");
    expect(withFifo("orders", false)).toBe("orders");
  });
});
