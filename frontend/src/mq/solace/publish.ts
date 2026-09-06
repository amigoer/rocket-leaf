/**
 * What the Solace send console collects, and what it may submit.
 *
 * Kept apart from the board so the rules can be tested without rendering it,
 * the way every other family's are. The rules are the broker's: a delivery
 * mode is one of three words it names in its own refusal, and a destination is
 * either a queue by name or a topic to be matched - which are two different
 * paths on the interface and two different things to do.
 */
import type { SolacePublishInput } from "@/api/solace";

/** More than this in one send is a long wait: each copy is its own request. */
export const MAX_COUNT = 100;

/** Where a send goes. Not derived from the name: the two are different acts. */
export const TARGETS = ["queue", "topic"] as const;
export type SendTarget = (typeof TARGETS)[number];

/**
 * The three the broker takes, spelled as it spells them in the message it
 * refuses a fourth with.
 */
export const DELIVERY_MODES = ["persistent", "non-persistent", "direct"] as const;

/** Anything goes on the way in here, unlike IBM MQ's character-only interface. */
export const CONTENT_TYPES = [
  "text/plain;charset=utf-8",
  "application/json",
  "application/octet-stream",
] as const;

export interface SolaceProducerDraft {
  target: SendTarget;
  destination: string;
  body: string;
  contentType: string;
  deliveryMode: string;
  timeToLiveMs: string;
  dmqEligible: boolean;
  correlationId: string;
  replyTo: string;
  count: string;
}

export function emptySolaceProducerDraft(): SolaceProducerDraft {
  return {
    target: "queue",
    destination: "",
    body: "",
    contentType: CONTENT_TYPES[0],
    // Persistent, which is the opposite of IBM MQ's console default and right
    // for the opposite reason: a direct message to a queue with nothing bound
    // is still spooled, but a non-persistent one is gone at the next restart
    // and a console user testing a queue wants to find their message there.
    deliveryMode: "persistent",
    timeToLiveMs: "",
    // Off, matching the broker. It is offered because the broker's default is
    // what makes a queue with a dead message queue discard quietly.
    dmqEligible: false,
    correlationId: "",
    replyTo: "",
    count: "1",
  };
}

/** What is stopping this send, or null when nothing is. */
export type SendProblem =
  | "destinationRequired"
  | "bodyRequired"
  | "deliveryMode"
  | "ttlRange"
  | "countRange";

export function sendProblem(draft: SolaceProducerDraft): SendProblem | null {
  if (draft.destination.trim() === "") return "destinationRequired";
  if (draft.body === "") return "bodyRequired";
  if (!DELIVERY_MODES.includes(draft.deliveryMode as (typeof DELIVERY_MODES)[number])) {
    return "deliveryMode";
  }

  const ttl = draft.timeToLiveMs.trim();
  if (ttl !== "") {
    const parsed = Number.parseInt(ttl, 10);
    if (Number.isNaN(parsed) || parsed < 0) return "ttlRange";
  }

  const count = Number.parseInt(draft.count, 10);
  if (Number.isNaN(count) || count < 1 || count > MAX_COUNT) return "countRange";
  return null;
}

/** The submission, or null when the draft would be refused. */
export function toPublishInput(draft: SolaceProducerDraft): SolacePublishInput | null {
  if (sendProblem(draft) != null) return null;
  const ttl = Number.parseInt(draft.timeToLiveMs, 10);
  return {
    target: draft.target,
    destination: draft.destination.trim(),
    body: draft.body,
    contentType: draft.contentType,
    deliveryMode: draft.deliveryMode,
    timeToLiveMs: Number.isNaN(ttl) || ttl < 0 ? 0 : ttl,
    dmqEligible: draft.dmqEligible,
    correlationId: draft.correlationId.trim(),
    replyTo: draft.replyTo.trim(),
    properties: {},
    count: Number.parseInt(draft.count, 10),
  };
}
