import { describe, expect, it } from "vitest";
import {
  MAX_COUNT,
  emptyPubSubProducerDraft,
  sendProblem,
  sendWarning,
  toPublishInput,
  type PubSubProducerDraft,
} from "./publish";

const filled = (over: Partial<PubSubProducerDraft> = {}): PubSubProducerDraft => ({
  ...emptyPubSubProducerDraft(),
  topic: "orders",
  body: "hello",
  ...over,
});

describe("what the Pub/Sub send console will send", () => {
  it("accepts an ordinary send", () => {
    expect(sendProblem(filled())).toBeNull();
    expect(toPublishInput(filled())?.topic).toBe("orders");
  });

  it("needs a topic", () => {
    expect(sendProblem(filled({ topic: "  " }))).toBe("board.google-pubsub.producer.topicRequired");
  });

  /*
   * The service refuses a message with neither a body nor an attribute and
   * names no field in the refusal. A message carrying only attributes is fine,
   * and is how a filtered subscription is exercised without a payload.
   */
  it("refuses a message with neither a body nor an attribute", () => {
    expect(sendProblem(filled({ body: "" }))).toBe("board.google-pubsub.producer.emptyMessage");
    expect(
      sendProblem(filled({ body: "", attributes: [{ name: "kind", value: "signal" }] })),
    ).toBeNull();
  });

  it("refuses an attribute with a value and no name", () => {
    expect(sendProblem(filled({ attributes: [{ name: " ", value: "x" }] }))).toBe(
      "board.google-pubsub.producer.attributeNameRequired",
    );
    // A wholly blank row is one somebody added and has not filled in yet.
    expect(sendProblem(filled({ attributes: [{ name: "", value: "" }] }))).toBeNull();
  });

  it("holds the count to this app's own cap", () => {
    expect(sendProblem(filled({ count: 0 }))).toBe("board.google-pubsub.producer.countRange");
    expect(sendProblem(filled({ count: MAX_COUNT + 1 }))).toBe(
      "board.google-pubsub.producer.countRange",
    );
    expect(sendProblem(filled({ count: MAX_COUNT }))).toBeNull();
  });

  it("drops blank attribute rows and trims what it sends", () => {
    const input = toPublishInput(
      filled({
        topic: "  orders  ",
        orderingKey: "  customer-1  ",
        attributes: [
          { name: " kind ", value: "order" },
          { name: "", value: "" },
        ],
      }),
    );
    expect(input?.topic).toBe("orders");
    expect(input?.orderingKey).toBe("customer-1");
    expect(input?.attributes).toEqual({ kind: "order" });
  });

  /*
   * The warning no other family here has. A topic stores nothing: with no
   * subscription the publish is accepted, reported as sent, and discarded, and
   * there is no backlog anywhere afterwards to notice it by.
   */
  it("warns when the chosen topic has no subscription", () => {
    expect(sendWarning(filled(), 0)).toBe("board.google-pubsub.producer.noSubscriberWarning");
    expect(sendWarning(filled(), 2)).toBeNull();
    // Unknown is not zero: before the topic list has loaded there is nothing
    // to warn about.
    expect(sendWarning(filled(), null)).toBeNull();
  });
});
