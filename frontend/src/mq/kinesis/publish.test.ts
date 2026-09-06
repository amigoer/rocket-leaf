import { describe, expect, it } from "vitest";
import {
  MAX_COUNT,
  aimsAtAShard,
  emptyKinesisProducerDraft,
  sendProblem,
  sendWarning,
  toPublishInput,
} from "./publish";

const whole = () => ({
  ...emptyKinesisProducerDraft(),
  stream: "orders",
  body: "{}",
  partitionKey: "orders-1",
});

/**
 * The partition key is the field this console has and no other does, and it is
 * required rather than convenient: it is what the service hashes to choose a
 * shard, there is no default, and a send without one is refused by an
 * exception that names Data in the same breath.
 */
describe("what the Kinesis send console will not send", () => {
  it("accepts a whole draft", () => {
    expect(sendProblem(whole())).toBeNull();
    expect(toPublishInput(whole())).toEqual({
      stream: "orders",
      body: "{}",
      partitionKey: "orders-1",
      explicitHashKey: "",
      count: 1,
    });
  });

  it("asks for the three fields a record cannot be built without", () => {
    expect(sendProblem({ ...whole(), stream: "  " })).toBe(
      "board.kinesis.producer.streamRequired",
    );
    expect(sendProblem({ ...whole(), body: "" })).toBe("board.kinesis.producer.bodyRequired");
    expect(sendProblem({ ...whole(), partitionKey: " " })).toBe(
      "board.kinesis.producer.keyRequired",
    );
  });

  it("holds the repeat count to what the driver will take", () => {
    expect(sendProblem({ ...whole(), count: 0 })).toBe("board.kinesis.producer.countRange");
    expect(sendProblem({ ...whole(), count: MAX_COUNT })).toBeNull();
    expect(sendProblem({ ...whole(), count: MAX_COUNT + 1 })).toBe(
      "board.kinesis.producer.countRange",
    );
  });

  // A hash key is a 128-bit unsigned integer written in decimal. The service
  // refuses a bad one with a regular expression that names neither the field
  // nor the range it wanted.
  it("holds the explicit hash key to a decimal 128-bit number", () => {
    expect(sendProblem({ ...whole(), explicitHashKey: "0" })).toBeNull();
    expect(sendProblem({ ...whole(), explicitHashKey: (2n ** 127n).toString() })).toBeNull();
    expect(sendProblem({ ...whole(), explicitHashKey: "0x1f" })).toBe(
      "board.kinesis.producer.hashKeyInvalid",
    );
    expect(sendProblem({ ...whole(), explicitHashKey: "1".repeat(40) })).toBe(
      "board.kinesis.producer.hashKeyInvalid",
    );
  });

  // The body limit is on bytes rather than characters, and a form that counted
  // characters would let a multi-byte payload through to be refused.
  it("measures the body in bytes", () => {
    const oneMegabyteOfAscii = "a".repeat(1024 * 1024);
    expect(sendProblem({ ...whole(), body: oneMegabyteOfAscii })).toBeNull();
    expect(sendProblem({ ...whole(), body: `${oneMegabyteOfAscii}a` })).toBe(
      "board.kinesis.producer.bodyTooLarge",
    );
    // Half as many characters, the same number of bytes: still too large.
    expect(sendProblem({ ...whole(), body: "。".repeat(1024 * 350) })).toBe(
      "board.kinesis.producer.bodyTooLarge",
    );
  });
});

/**
 * An aimed repeat is accepted and worth a word: the hash key overrides the
 * partition key entirely, so every copy lands on one shard rather than being
 * spread the way a plain repeat is.
 */
describe("what the console warns about and still sends", () => {
  it("says an aimed repeat all goes to one shard", () => {
    expect(sendWarning(whole())).toBeNull();
    expect(sendWarning({ ...whole(), count: 10 })).toBeNull();
    expect(aimsAtAShard({ ...whole(), explicitHashKey: "42" })).toBe(true);
    expect(sendWarning({ ...whole(), explicitHashKey: "42", count: 10 })).toBe(
      "board.kinesis.producer.aimedRepeatNote",
    );
  });
});
