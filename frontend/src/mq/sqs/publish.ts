/**
 * What the SQS send console collects, and the rules it enforces.
 *
 * Separate from the form component so the rules can be tested without
 * rendering. Each catches something SQS would either refuse with a message
 * naming an attribute the form never drew, or accept and quietly get wrong:
 *
 *   - An empty body is InvalidParameterValue naming MessageBody, which says
 *     nothing about which field is blank on a form with several.
 *   - A FIFO queue with no group id is MissingParameter naming
 *     MessageGroupId. A standard queue with one is refused by the same name,
 *     so a user who filled the field in by habit would be told the field they
 *     typed is missing.
 *   - A per-message delay on a FIFO queue is refused outright: a FIFO queue's
 *     delay is a queue setting, and sending anyway would deliver the messages
 *     immediately under a report that they had been held back.
 *   - A delay past fifteen minutes, which is the service's own ceiling.
 */
import type { SQSPublishInput } from "@bindings/bridge/models";
import { isFifoName } from "./names";

/**
 * How many copies one send may carry.
 *
 * This app's cap rather than the service's, mirrored from
 * internal/driver/sqs/publish.go: a send console is for producing a handful by
 * hand, and every message costs a request AWS bills for.
 */
export const MAX_COUNT = 1000;

/** The longest SQS will hold a message back, in seconds. */
export const MAX_DELAY_SEC = 900;

export interface SqsProducerDraft {
  queue: string;
  body: string;
  count: number;
  delaySec: number;
  /** Required on a FIFO queue, refused on a standard one. */
  groupId: string;
  /** FIFO only. Blank lets the driver generate one per copy. */
  deduplicationId: string;
  /** The producer's own attributes, sent as SQS string attributes. */
  attributes: { name: string; value: string }[];
}

export function emptySqsProducerDraft(): SqsProducerDraft {
  return {
    queue: "",
    body: "",
    count: 1,
    delaySec: 0,
    groupId: "",
    deduplicationId: "",
    attributes: [],
  };
}

/** Whether the chosen queue orders its messages, which its name decides. */
export function targetsFifo(draft: SqsProducerDraft): boolean {
  return isFifoName(draft.queue);
}

/** Why the draft cannot be sent, as an i18n key, or null when it can. */
export function sendProblem(draft: SqsProducerDraft): string | null {
  if (draft.queue.trim() === "") return "board.sqs.producer.queueRequired";
  if (draft.body === "") return "board.sqs.producer.bodyRequired";
  if (draft.count < 1 || draft.count > MAX_COUNT) return "board.sqs.producer.countRange";
  if (draft.delaySec < 0 || draft.delaySec > MAX_DELAY_SEC) {
    return "board.sqs.producer.delayRange";
  }
  if (targetsFifo(draft)) {
    if (draft.groupId.trim() === "") return "board.sqs.producer.groupRequired";
    if (draft.delaySec > 0) return "board.sqs.producer.fifoNoDelay";
  } else if (draft.groupId.trim() !== "") {
    return "board.sqs.producer.groupOnStandard";
  }
  // An attribute with a value and no name cannot be sent, and one with
  // neither is a row somebody added and has not filled in yet.
  if (draft.attributes.some((row) => row.name.trim() === "" && row.value !== "")) {
    return "board.sqs.producer.attributeNameRequired";
  }
  return null;
}

/**
 * What the console warns about but will still send, as an i18n key.
 *
 * A repeat on a FIFO queue is worth saying out loud: SQS deduplicates on the
 * body inside a five-minute window unless each copy carries its own
 * deduplication id, so the driver appends an index to whatever was typed. A
 * user who set an explicit id and expected ten identical messages gets ten,
 * but not under the id they chose.
 */
export function sendWarning(draft: SqsProducerDraft): string | null {
  if (targetsFifo(draft) && draft.count > 1) return "board.sqs.producer.fifoRepeatNote";
  return null;
}

/** The input to send, or null when the draft is not yet whole. */
export function toPublishInput(draft: SqsProducerDraft): SQSPublishInput | null {
  if (sendProblem(draft) != null) return null;
  const attributes: Record<string, string> = {};
  for (const row of draft.attributes) {
    const name = row.name.trim();
    if (name !== "") attributes[name] = row.value;
  }
  const fifo = targetsFifo(draft);
  return {
    queue: draft.queue.trim(),
    body: draft.body,
    count: draft.count,
    delaySec: draft.delaySec,
    attributes,
    // Neither field means anything on a standard queue, and sending one would
    // have the service refuse the whole batch by an attribute name the form
    // stopped showing the moment the queue changed.
    groupId: fifo ? draft.groupId.trim() : "",
    deduplicationId: fifo ? draft.deduplicationId.trim() : "",
  };
}
