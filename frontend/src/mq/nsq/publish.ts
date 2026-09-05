/**
 * What the NSQ send console collects, and the rules it enforces.
 *
 * Separate from the form component so the rules can be tested without
 * rendering. There are three, and each catches something nsqd would either
 * refuse with a message that names no field or accept and quietly get wrong:
 *
 *   - An empty body is MSG_EMPTY, which says nothing about which field is
 *     blank on a form with four of them.
 *   - A delay past nsqd's --max-req-timeout is INVALID_DEFER. The limit is one
 *     hour by default and this driver cannot read the deployment's value -
 *     /info does not report it - so the console warns at the default and lets
 *     the daemon have the last word.
 *   - A repeat count above what one call should carry. A batch goes through
 *     /mpub, and a send console is for producing a handful by hand.
 */
import type { NSQPublishInput } from "@bindings/bridge/models";

/** nsqd's own cap on a repeat, mirrored from internal/driver/nsq/publish.go. */
export const MAX_COUNT = 1000;

/** nsqd's --max-req-timeout default, in seconds. */
export const DEFAULT_MAX_DELAY_SEC = 3600;

export interface NsqProducerDraft {
  topic: string;
  body: string;
  count: number;
  delaySec: number;
  /** host:port of the nsqd to publish through. Empty means the first. */
  node: string;
}

export function emptyNsqProducerDraft(): NsqProducerDraft {
  return { topic: "", body: "", count: 1, delaySec: 0, node: "" };
}

/** Why the draft cannot be sent, as an i18n key, or null when it can. */
export function sendProblem(draft: NsqProducerDraft): string | null {
  if (draft.topic.trim() === "") return "board.nsq.producer.topicRequired";
  if (draft.body === "") return "board.nsq.producer.bodyRequired";
  if (draft.count < 1 || draft.count > MAX_COUNT) return "board.nsq.producer.countRange";
  if (draft.delaySec < 0 || draft.delaySec > DEFAULT_MAX_DELAY_SEC) {
    return "board.nsq.producer.delayRange";
  }
  return null;
}

/** The input to send, or null when the draft is not yet whole. */
export function toPublishInput(draft: NsqProducerDraft): NSQPublishInput | null {
  if (sendProblem(draft) != null) return null;
  return {
    topic: draft.topic.trim(),
    body: draft.body,
    count: draft.count,
    delaySec: draft.delaySec,
    node: draft.node.trim(),
  };
}

/**
 * Whether this send will go out one message at a time rather than as a batch.
 *
 * Worth showing, because it is the difference between one round trip and a
 * hundred: /mpub ignores a defer - confirmed against 1.3.0, where the messages
 * arrive immediately - and cannot carry a body with a newline in it, since
 * newline is what separates the messages inside one.
 */
export function sendsOneAtATime(draft: NsqProducerDraft): boolean {
  return draft.count > 1 && (draft.delaySec > 0 || draft.body.includes("\n"));
}
