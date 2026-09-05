/**
 * What the ActiveMQ send console collects, and the one rule it enforces.
 *
 * Separate from the form component so the rule can be tested without
 * rendering: a header line that is not a key/value pair is the only way to get
 * this form wrong, and it fails on the broker rather than in the field unless
 * something checks it here.
 */
import type { ActiveMQPublishInput } from "@/api/activemq";

export interface ActiveMQProducerDraft {
  destination: string;
  body: string;
  persistent: boolean;
  priority: number;
  correlationId: string;
  replyTo: string;
  jmsType: string;
  /** One `key: value` per line, which is how every send console here takes them. */
  headers: string;
  count: number;
}

export function emptyActiveMQProducerDraft(): ActiveMQProducerDraft {
  return {
    destination: "",
    body: "",
    persistent: true,
    // 4 is the JMS default and the value a producer that said nothing gets.
    priority: 4,
    correlationId: "",
    replyTo: "",
    jmsType: "",
    headers: "",
    count: 1,
  };
}

/**
 * Parses the header block, or reports the first line that is not a pair.
 *
 * A line with no colon is the mistake worth catching: sent as-is it becomes a
 * property whose name is the whole line, which the broker accepts and nobody
 * ever finds again.
 */
export function parseHeaders(
  block: string,
): { headers: Record<string, string> } | { badLine: string } {
  const headers: Record<string, string> = {};
  for (const raw of block.split("\n")) {
    const line = raw.trim();
    if (line === "") continue;
    const separator = line.indexOf(":");
    if (separator <= 0) return { badLine: line };
    const key = line.slice(0, separator).trim();
    if (key === "") return { badLine: line };
    headers[key] = line.slice(separator + 1).trim();
  }
  return { headers };
}

/** The input to send, or null when the draft is not yet whole. */
export function toPublishInput(draft: ActiveMQProducerDraft): ActiveMQPublishInput | null {
  const destination = draft.destination.trim();
  if (destination === "") return null;
  const parsed = parseHeaders(draft.headers);
  if ("badLine" in parsed) return null;

  return {
    destination,
    body: draft.body,
    persistent: draft.persistent,
    priority: draft.priority,
    correlationId: draft.correlationId.trim(),
    replyTo: draft.replyTo.trim(),
    jmsType: draft.jmsType.trim(),
    headers: parsed.headers,
    count: draft.count > 0 ? draft.count : 1,
  };
}
