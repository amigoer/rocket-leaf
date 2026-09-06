import { describe, expect, it } from "vitest";
import { MAX_COUNT, emptyIbmMqProducerDraft, sendProblem, toPublishInput } from "./publish";

const filled = () => ({
  ...emptyIbmMqProducerDraft(),
  queue: "APP.ORDERS",
  body: "hello",
});

/*
 * The rules are the queue manager's rather than this app's, and they are
 * checked here for the reason the object-name rule is: mqweb refuses a
 * malformed correlation identifier by naming a hex string, which says nothing
 * about which field it came from.
 */
describe("an IBM MQ send", () => {
  it("needs a queue and a body", () => {
    expect(sendProblem(filled())).toBeNull();
    expect(sendProblem({ ...filled(), queue: "  " })).toBe("queueRequired");
    expect(sendProblem({ ...filled(), body: "" })).toBe("bodyRequired");
  });

  it("takes a correlation id of exactly 24 bytes, or none at all", () => {
    expect(sendProblem({ ...filled(), correlationId: "" })).toBeNull();
    expect(sendProblem({ ...filled(), correlationId: "ab".repeat(24) })).toBeNull();
    expect(sendProblem({ ...filled(), correlationId: "ab".repeat(23) })).toBe(
      "correlationIdLength",
    );
    expect(sendProblem({ ...filled(), correlationId: "z".repeat(48) })).toBe("correlationIdHex");
  });

  // Each copy is its own HTTP request, so the cap is a real one rather than a
  // batch size somebody could raise.
  it("caps how many copies one send makes", () => {
    expect(sendProblem({ ...filled(), count: String(MAX_COUNT) })).toBeNull();
    expect(sendProblem({ ...filled(), count: String(MAX_COUNT + 1) })).toBe("countRange");
    expect(sendProblem({ ...filled(), count: "0" })).toBe("countRange");
    expect(sendProblem({ ...filled(), count: "" })).toBe("countRange");
  });

  it("submits nothing while the draft would be refused", () => {
    expect(toPublishInput({ ...filled(), queue: "" })).toBeNull();
  });

  // Blank expiry is MQ's own unlimited rather than an expiry of zero, which
  // would discard the message the moment it arrived.
  it("sends no expiry when the field is blank", () => {
    const input = toPublishInput(filled());
    expect(input?.expirySeconds).toBe(0);
    expect(toPublishInput({ ...filled(), expirySeconds: "600" })?.expirySeconds).toBe(600);
  });

  it("trims the queue and the correlation id", () => {
    const input = toPublishInput({
      ...filled(),
      queue: "  APP.ORDERS  ",
      correlationId: `  ${"ab".repeat(24)}  `,
    });
    expect(input?.queue).toBe("APP.ORDERS");
    expect(input?.correlationId).toBe("ab".repeat(24));
  });
});
