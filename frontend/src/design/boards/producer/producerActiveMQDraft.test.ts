import { describe, expect, it } from "vitest";
import {
  emptyActiveMQProducerDraft,
  parseHeaders,
  toPublishInput,
} from "./producerActiveMQDraft";

describe("the activemq header block", () => {
  it("reads one key/value pair per line", () => {
    expect(parseHeaders("tenant: acme\nattempt: 3")).toEqual({
      headers: { tenant: "acme", attempt: "3" },
    });
  });

  it("ignores blank lines rather than making an empty header", () => {
    expect(parseHeaders("\n  \ntenant: acme\n")).toEqual({ headers: { tenant: "acme" } });
  });

  /*
   * A line with no colon is the one mistake this form can make. Sent as-is it
   * becomes a property whose name is the whole line, which both brokers accept
   * without complaint and nobody ever finds again.
   */
  it("reports a line that is not a pair", () => {
    expect(parseHeaders("tenant acme")).toEqual({ badLine: "tenant acme" });
    expect(parseHeaders(": acme")).toEqual({ badLine: ": acme" });
  });

  it("keeps a colon inside the value", () => {
    expect(parseHeaders("url: https://example.com:8161")).toEqual({
      headers: { url: "https://example.com:8161" },
    });
  });
});

describe("an activemq send", () => {
  it("is not ready without a destination", () => {
    expect(toPublishInput(emptyActiveMQProducerDraft())).toBeNull();
  });

  it("is not ready while a header line is malformed", () => {
    const draft = { ...emptyActiveMQProducerDraft(), destination: "ORDERS", headers: "oops" };
    expect(toPublishInput(draft)).toBeNull();
  });

  it("trims the destination and the JMS headers", () => {
    const input = toPublishInput({
      ...emptyActiveMQProducerDraft(),
      destination: "  ORDERS  ",
      correlationId: " abc ",
      replyTo: " REPLIES ",
      jmsType: " order ",
    });
    expect(input).toMatchObject({
      destination: "ORDERS",
      correlationId: "abc",
      replyTo: "REPLIES",
      jmsType: "order",
    });
  });

  it("sends once when the count was left at zero", () => {
    const input = toPublishInput({
      ...emptyActiveMQProducerDraft(),
      destination: "ORDERS",
      count: 0,
    });
    expect(input?.count).toBe(1);
  });
});
