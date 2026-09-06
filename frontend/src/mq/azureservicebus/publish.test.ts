import { describe, expect, it } from "vitest";
import {
  MAX_COUNT,
  emptyServiceBusProducerDraft,
  sendProblem,
  sendWarning,
  toSendInput,
} from "./publish";

const filled = () => ({
  ...emptyServiceBusProducerDraft(),
  entity: "orders",
  body: "hello",
});

describe("what the Service Bus send console will send", () => {
  it("needs an entity and nothing else", () => {
    expect(sendProblem(emptyServiceBusProducerDraft())).toBe(
      "board.azure-servicebus.producer.entityRequired",
    );
    expect(sendProblem(filled())).toBeNull();
  });

  /*
   * A Service Bus message may be empty, unlike a Pub/Sub one - and it is not a
   * degenerate case here: a message with properties and no body is exactly how
   * a subscription filtering on those properties is exercised.
   */
  it("sends a message with no body at all", () => {
    const draft = { ...filled(), body: "" };
    expect(sendProblem(draft)).toBeNull();
    expect(toSendInput(draft)?.body).toBe("");
  });

  it("holds the count to this app's own cap", () => {
    expect(sendProblem({ ...filled(), count: 0 })).toBe(
      "board.azure-servicebus.producer.countRange",
    );
    expect(sendProblem({ ...filled(), count: MAX_COUNT + 1 })).toBe(
      "board.azure-servicebus.producer.countRange",
    );
    expect(sendProblem({ ...filled(), count: MAX_COUNT })).toBeNull();
  });

  it("refuses a property with a value and no name", () => {
    expect(
      sendProblem({ ...filled(), properties: [{ name: "", value: "red" }] }),
    ).toBe("board.azure-servicebus.producer.propertyNameRequired");
    // A wholly empty row is one somebody added and has not filled in.
    expect(sendProblem({ ...filled(), properties: [{ name: "", value: "" }] })).toBeNull();
  });

  it("refuses a negative delay", () => {
    expect(sendProblem({ ...filled(), delaySec: -1 })).toBe(
      "board.azure-servicebus.producer.delayNegative",
    );
    expect(sendProblem({ ...filled(), delaySec: 3600 })).toBeNull();
  });

  it("carries the fields a subscription's rules select on", () => {
    const input = toSendInput({
      ...filled(),
      subject: "  order  ",
      correlationId: " abc ",
      sessionId: " customer-1 ",
      delaySec: 60,
      properties: [
        { name: " colour ", value: "red" },
        { name: "", value: "" },
      ],
    });
    expect(input).not.toBeNull();
    // The subject is what a correlation filter matches by name, and the
    // properties are what a SQL filter reads - so both have to survive.
    expect(input?.subject).toBe("order");
    expect(input?.correlationId).toBe("abc");
    expect(input?.sessionId).toBe("customer-1");
    expect(input?.delaySec).toBe(60);
    expect(input?.properties).toEqual({ colour: "red" });
  });

  /*
   * The one state no queue can be in. A topic with no subscription accepts
   * every send, reports success, and discards the message - and there is no
   * backlog anywhere afterwards to notice it by.
   */
  it("warns about a topic nothing subscribes to, and still sends", () => {
    expect(sendWarning(filled(), 0)).toBe("board.azure-servicebus.producer.noSubscriberWarning");
    expect(sendProblem(filled())).toBeNull();
    expect(sendWarning(filled(), 2)).toBeNull();
    // Null is a queue, or an entity the listing did not include: "not loaded"
    // must not read as "nothing subscribes to it".
    expect(sendWarning(filled(), null)).toBeNull();
  });
});
