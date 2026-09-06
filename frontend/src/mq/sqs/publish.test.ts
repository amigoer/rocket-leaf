import { describe, expect, it } from "vitest";
import {
  MAX_COUNT,
  MAX_DELAY_SEC,
  emptySqsProducerDraft,
  sendProblem,
  sendWarning,
  targetsFifo,
  toPublishInput,
} from "./publish";

const standard = () => ({ ...emptySqsProducerDraft(), queue: "orders", body: "hello" });
const fifo = () => ({
  ...emptySqsProducerDraft(),
  queue: "orders.fifo",
  body: "hello",
  groupId: "acme",
});

/**
 * The rules the console enforces, and why each one is here rather than left to
 * the service: every SQS refusal names an attribute, and half of them name a
 * field the form did not draw.
 */
describe("the SQS send console's rules", () => {
  it("needs a queue and a body", () => {
    expect(sendProblem(emptySqsProducerDraft())).toBe("board.sqs.producer.queueRequired");
    expect(sendProblem({ ...emptySqsProducerDraft(), queue: "orders" })).toBe(
      "board.sqs.producer.bodyRequired",
    );
    expect(sendProblem(standard())).toBeNull();
  });

  it("holds the repeat count to what this console is for", () => {
    expect(sendProblem({ ...standard(), count: 0 })).toBe("board.sqs.producer.countRange");
    expect(sendProblem({ ...standard(), count: MAX_COUNT })).toBeNull();
    expect(sendProblem({ ...standard(), count: MAX_COUNT + 1 })).toBe(
      "board.sqs.producer.countRange",
    );
  });

  it("holds the delay to the service's own ceiling", () => {
    expect(sendProblem({ ...standard(), delaySec: MAX_DELAY_SEC })).toBeNull();
    expect(sendProblem({ ...standard(), delaySec: MAX_DELAY_SEC + 1 })).toBe(
      "board.sqs.producer.delayRange",
    );
    expect(sendProblem({ ...standard(), delaySec: -1 })).toBe("board.sqs.producer.delayRange");
  });

  /*
   * The group id is the field this console exists to get right. SQS refuses a
   * FIFO send without one and a standard send with one, and names
   * MessageGroupId in both answers - so the service's own message sends half
   * of its readers to the wrong place.
   */
  it("reads FIFO off the queue name and requires a group id there", () => {
    expect(targetsFifo(fifo())).toBe(true);
    expect(targetsFifo(standard())).toBe(false);

    expect(sendProblem({ ...fifo(), groupId: "" })).toBe("board.sqs.producer.groupRequired");
    expect(sendProblem(fifo())).toBeNull();
    expect(sendProblem({ ...standard(), groupId: "acme" })).toBe(
      "board.sqs.producer.groupOnStandard",
    );
  });

  // A FIFO queue's delay is a queue setting. Sending anyway would deliver the
  // messages immediately under a report that they had been held back.
  it("refuses a per-message delay on a FIFO queue", () => {
    expect(sendProblem({ ...fifo(), delaySec: 30 })).toBe("board.sqs.producer.fifoNoDelay");
    expect(sendProblem({ ...standard(), delaySec: 30 })).toBeNull();
  });

  it("refuses an attribute value with no name to send it under", () => {
    expect(
      sendProblem({ ...standard(), attributes: [{ name: "", value: "acme" }] }),
    ).toBe("board.sqs.producer.attributeNameRequired");
    // An empty row is one somebody added and has not filled in.
    expect(sendProblem({ ...standard(), attributes: [{ name: "", value: "" }] })).toBeNull();
  });

  // SQS deduplicates a FIFO send on the body for five minutes, so ten copies
  // arrive as one unless each carries its own id. The driver appends an index;
  // the console says so rather than letting it surprise somebody.
  it("warns that a FIFO repeat is deduplicated per copy", () => {
    expect(sendWarning({ ...fifo(), count: 5 })).toBe("board.sqs.producer.fifoRepeatNote");
    expect(sendWarning(fifo())).toBeNull();
    expect(sendWarning({ ...standard(), count: 5 })).toBeNull();
  });
});

describe("what the SQS console submits", () => {
  it("sends nothing while the draft is incomplete", () => {
    expect(toPublishInput(emptySqsProducerDraft())).toBeNull();
  });

  it("trims what was typed and drops the unnamed attribute rows", () => {
    const input = toPublishInput({
      ...standard(),
      queue: "  orders  ",
      attributes: [
        { name: "  tenant  ", value: "acme" },
        { name: "  ", value: "" },
      ],
    });
    expect(input?.queue).toBe("orders");
    expect(input?.attributes).toEqual({ tenant: "acme" });
  });

  /*
   * Neither FIFO field means anything on a standard queue, and sending one
   * would have SQS refuse the whole batch by naming an attribute the form
   * stopped showing the moment the queue changed.
   */
  it("drops the FIFO fields when the queue is not a FIFO one", () => {
    const input = toPublishInput({
      ...standard(),
      // Left behind by a queue the user picked and then changed.
      groupId: "",
      deduplicationId: "left-over",
    });
    expect(input?.groupId).toBe("");
    expect(input?.deduplicationId).toBe("");
  });

  it("carries the FIFO fields when the queue is one", () => {
    const input = toPublishInput({ ...fifo(), deduplicationId: "order-1" });
    expect(input?.groupId).toBe("acme");
    expect(input?.deduplicationId).toBe("order-1");
  });
});
