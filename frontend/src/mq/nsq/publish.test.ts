import { describe, expect, it } from "vitest";
import {
  DEFAULT_MAX_DELAY_SEC,
  MAX_COUNT,
  emptyNsqProducerDraft,
  sendProblem,
  sendsOneAtATime,
  toPublishInput,
} from "./publish";

const draft = (over: Partial<ReturnType<typeof emptyNsqProducerDraft>> = {}) => ({
  ...emptyNsqProducerDraft(),
  topic: "orders",
  body: "hello",
  ...over,
});

describe("the nsq send console's rules", () => {
  it("sends a filled-in draft", () => {
    expect(sendProblem(draft())).toBeNull();
    expect(toPublishInput(draft())).toEqual({
      topic: "orders",
      body: "hello",
      count: 1,
      delaySec: 0,
      node: "",
    });
  });

  it("trims the topic and the node, because a padded name is a different one", () => {
    const input = toPublishInput(draft({ topic: "  orders  ", node: "  127.0.0.1:4151 " }));
    expect(input?.topic).toBe("orders");
    expect(input?.node).toBe("127.0.0.1:4151");
  });

  // nsqd answers MSG_EMPTY, which names no field on a form with four of them.
  it("names the empty field rather than letting nsqd say MSG_EMPTY", () => {
    expect(sendProblem(draft({ body: "" }))).toBe("board.nsq.producer.bodyRequired");
    expect(sendProblem(draft({ topic: "   " }))).toBe("board.nsq.producer.topicRequired");
  });

  // The body is not trimmed: whitespace is a legitimate payload, and a body of
  // spaces is a message nsqd will take.
  it("keeps the body exactly as typed", () => {
    expect(toPublishInput(draft({ body: "  padded  " }))?.body).toBe("  padded  ");
  });

  it("holds the repeat count inside what one call carries", () => {
    expect(sendProblem(draft({ count: 0 }))).toBe("board.nsq.producer.countRange");
    expect(sendProblem(draft({ count: MAX_COUNT }))).toBeNull();
    expect(sendProblem(draft({ count: MAX_COUNT + 1 }))).toBe("board.nsq.producer.countRange");
  });

  // Confirmed against 1.3.0: defer=3600000 answers OK and 3600001 answers
  // INVALID_DEFER. The limit is a daemon flag this app cannot read, so the
  // console warns at the default rather than pretending to know.
  it("warns at nsqd's default delay ceiling", () => {
    expect(sendProblem(draft({ delaySec: DEFAULT_MAX_DELAY_SEC }))).toBeNull();
    expect(sendProblem(draft({ delaySec: DEFAULT_MAX_DELAY_SEC + 1 }))).toBe(
      "board.nsq.producer.delayRange",
    );
    expect(sendProblem(draft({ delaySec: -1 }))).toBe("board.nsq.producer.delayRange");
  });

  /*
   * The two cases a batch cannot express. /mpub ignores a defer outright, and
   * it separates messages with a newline - so a repeat of either kind goes one
   * message per request, which is worth telling the user before they ask for a
   * thousand.
   */
  it("reports a repeat that cannot go as one batch", () => {
    expect(sendsOneAtATime(draft({ count: 10 }))).toBe(false);
    expect(sendsOneAtATime(draft({ count: 10, delaySec: 30 }))).toBe(true);
    expect(sendsOneAtATime(draft({ count: 10, body: "two\nlines" }))).toBe(true);
    // One message is one request whatever it contains, so there is nothing to
    // warn about.
    expect(sendsOneAtATime(draft({ count: 1, delaySec: 30 }))).toBe(false);
  });
});
