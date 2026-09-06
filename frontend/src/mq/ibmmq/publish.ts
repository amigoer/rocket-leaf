/**
 * What the IBM MQ send console collects, and what it may submit.
 *
 * Kept apart from the board so the rules can be tested without rendering it,
 * the way every other family's are. The rules are MQ's rather than this app's:
 * a correlation identifier is 24 bytes spelled as 48 hexadecimal characters,
 * and a destination is a queue because the messaging REST API has no topic
 * resource at all.
 */
import type { IBMMQPublishInput } from "@/api/ibmmq";

/** More than this in one send is a long wait: each copy is its own request. */
export const MAX_COUNT = 100;

/** The two character types the interface accepts. Anything else is refused. */
export const CONTENT_TYPES = ["text/plain;charset=utf-8", "application/json"] as const;

export interface IbmMqProducerDraft {
  queue: string;
  body: string;
  contentType: string;
  correlationId: string;
  persistent: boolean;
  expirySeconds: string;
  count: string;
}

export function emptyIbmMqProducerDraft(): IbmMqProducerDraft {
  return {
    queue: "",
    body: "",
    contentType: CONTENT_TYPES[0],
    correlationId: "",
    // Non-persistent is the queue manager's own default for a new queue, and
    // it is the honest default for a console: a message somebody is testing
    // with should not outlive a restart unless they said so.
    persistent: false,
    expirySeconds: "",
    count: "1",
  };
}

/** What is stopping this send, or null when nothing is. */
export type SendProblem =
  | "queueRequired"
  | "bodyRequired"
  | "correlationIdLength"
  | "correlationIdHex"
  | "countRange";

export function sendProblem(draft: IbmMqProducerDraft): SendProblem | null {
  if (draft.queue.trim() === "") return "queueRequired";
  if (draft.body === "") return "bodyRequired";

  const correlation = draft.correlationId.trim();
  if (correlation !== "") {
    if (correlation.length !== 48) return "correlationIdLength";
    if (!/^[0-9a-fA-F]+$/.test(correlation)) return "correlationIdHex";
  }

  const count = Number.parseInt(draft.count, 10);
  if (Number.isNaN(count) || count < 1 || count > MAX_COUNT) return "countRange";
  return null;
}

/** The submission, or null when the draft would be refused. */
export function toPublishInput(draft: IbmMqProducerDraft): IBMMQPublishInput | null {
  if (sendProblem(draft) != null) return null;
  const expiry = Number.parseInt(draft.expirySeconds, 10);
  return {
    queue: draft.queue.trim(),
    body: draft.body,
    contentType: draft.contentType,
    correlationId: draft.correlationId.trim(),
    persistent: draft.persistent,
    expirySeconds: Number.isNaN(expiry) || expiry < 0 ? 0 : expiry,
    properties: {},
    count: Number.parseInt(draft.count, 10),
  };
}
