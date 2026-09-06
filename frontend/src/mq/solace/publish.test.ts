import { describe, expect, it } from "vitest";
import {
  MAX_COUNT,
  emptySolaceProducerDraft,
  sendProblem,
  toPublishInput,
} from "./publish";

const filled = () => ({
  ...emptySolaceProducerDraft(),
  destination: "  orders/eu  ",
  body: '{"order":1}',
});

/*
 * The console's rules, which are the broker's rather than this app's. They are
 * tested away from the board because that is the only way to pin what a send
 * would actually submit.
 */
describe("the Solace send draft", () => {
  it("submits a trimmed destination and the target as chosen", () => {
    const input = toPublishInput({ ...filled(), target: "topic" });
    expect(input).not.toBeNull();
    expect(input?.destination).toBe("orders/eu");
    expect(input?.target).toBe("topic");
  });

  /*
   * The target is a field rather than something guessed from the name, and
   * this is what pins that. A queue and a topic can be called the same thing -
   * "orders/eu" is an ordinary name for either - and the two sends do
   * different things: one names an endpoint, the other is matched against
   * every subscription in the Message VPN.
   */
  it("keeps the target apart from the name, which can be the same for both", () => {
    const asQueue = toPublishInput({ ...filled(), target: "queue" });
    const asTopic = toPublishInput({ ...filled(), target: "topic" });
    expect(asQueue?.destination).toBe(asTopic?.destination);
    expect(asQueue?.target).not.toBe(asTopic?.target);
  });

  it("refuses a delivery mode the broker does not have", () => {
    expect(sendProblem({ ...filled(), deliveryMode: "eventual" })).toBe("deliveryMode");
    for (const mode of ["persistent", "non-persistent", "direct"]) {
      expect(sendProblem({ ...filled(), deliveryMode: mode }), mode).toBeNull();
    }
  });

  it("refuses an empty destination and an empty body", () => {
    expect(sendProblem({ ...filled(), destination: "   " })).toBe("destinationRequired");
    expect(sendProblem({ ...filled(), body: "" })).toBe("bodyRequired");
  });

  it("refuses a negative time to live and takes an empty one as unlimited", () => {
    expect(sendProblem({ ...filled(), timeToLiveMs: "-1" })).toBe("ttlRange");
    expect(sendProblem({ ...filled(), timeToLiveMs: "" })).toBeNull();
    expect(toPublishInput({ ...filled(), timeToLiveMs: "" })?.timeToLiveMs).toBe(0);
  });

  it("caps the count, because each copy is its own request", () => {
    expect(sendProblem({ ...filled(), count: String(MAX_COUNT) })).toBeNull();
    expect(sendProblem({ ...filled(), count: String(MAX_COUNT + 1) })).toBe("countRange");
    expect(sendProblem({ ...filled(), count: "0" })).toBe("countRange");
  });

  /*
   * Dead-message eligibility starts off because that is the broker's default,
   * and it is the reason a queue configured with a dead message queue can
   * still discard quietly. The console offers it rather than hiding it.
   */
  it("starts with dead-message eligibility off, as the broker does", () => {
    expect(emptySolaceProducerDraft().dmqEligible).toBe(false);
    expect(toPublishInput({ ...filled(), dmqEligible: true })?.dmqEligible).toBe(true);
  });

  it("submits nothing at all while the draft would be refused", () => {
    expect(toPublishInput({ ...filled(), destination: "" })).toBeNull();
  });
});
