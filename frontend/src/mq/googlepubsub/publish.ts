/**
 * What the Pub/Sub send console collects, and the rules it enforces.
 *
 * Separate from the form component so the rules can be tested without
 * rendering. Each catches something Pub/Sub would either refuse with a message
 * naming nothing useful, or accept and quietly get wrong:
 *
 *   - A message with neither a body nor an attribute is refused by the
 *     service, and its refusal names no field at all.
 *   - An attribute with a value and no name cannot be sent, and one with
 *     neither is a row somebody added and has not filled in yet.
 *   - A count outside this app's own cap, mirrored from the driver.
 *
 * The warning is the one that matters most here and no other family has it: a
 * topic stores nothing, so a send to a topic with no subscription is accepted,
 * acknowledged and discarded with nothing anywhere recording that it happened.
 */
import type { GooglePubSubPublishInput } from "@bindings/bridge/models";

/**
 * How many copies one send may carry.
 *
 * This app's cap rather than the service's, mirrored from
 * internal/driver/googlepubsub/publish.go: a send console is for producing a
 * handful by hand, and every message costs a request Google bills for.
 */
export const MAX_COUNT = 1000;

export interface PubSubProducerDraft {
  topic: string;
  body: string;
  count: number;
  /** Only has an effect on a subscription created with message ordering on. */
  orderingKey: string;
  /** The publisher's own attributes, and what a subscription filter selects on. */
  attributes: { name: string; value: string }[];
}

export function emptyPubSubProducerDraft(): PubSubProducerDraft {
  return { topic: "", body: "", count: 1, orderingKey: "", attributes: [] };
}

function namedAttributes(draft: PubSubProducerDraft): Record<string, string> {
  const attributes: Record<string, string> = {};
  for (const row of draft.attributes) {
    const name = row.name.trim();
    if (name !== "") attributes[name] = row.value;
  }
  return attributes;
}

/** Why the draft cannot be sent, as an i18n key, or null when it can. */
export function sendProblem(draft: PubSubProducerDraft): string | null {
  if (draft.topic.trim() === "") return "board.google-pubsub.producer.topicRequired";
  if (draft.count < 1 || draft.count > MAX_COUNT) return "board.google-pubsub.producer.countRange";
  if (draft.attributes.some((row) => row.name.trim() === "" && row.value !== "")) {
    return "board.google-pubsub.producer.attributeNameRequired";
  }
  // The service refuses a message carrying neither, and says so by naming
  // nothing. A message with only attributes is fine and is how a filtered
  // subscription is exercised without a payload.
  if (draft.body === "" && Object.keys(namedAttributes(draft)).length === 0) {
    return "board.google-pubsub.producer.emptyMessage";
  }
  return null;
}

/**
 * What the console warns about but will still send, as an i18n key.
 *
 * `subscribers` is how many subscriptions the chosen topic has. Zero is the
 * one worth stopping at: the publish is accepted, reported as sent, and thrown
 * away, and no board anywhere afterwards records that it happened.
 */
export function sendWarning(draft: PubSubProducerDraft, subscribers: number | null): string | null {
  if (draft.topic.trim() !== "" && subscribers === 0) {
    return "board.google-pubsub.producer.noSubscriberWarning";
  }
  return null;
}

/** The input to send, or null when the draft is not yet whole. */
export function toPublishInput(draft: PubSubProducerDraft): GooglePubSubPublishInput | null {
  if (sendProblem(draft) != null) return null;
  return {
    topic: draft.topic.trim(),
    body: draft.body,
    count: draft.count,
    attributes: namedAttributes(draft),
    orderingKey: draft.orderingKey.trim(),
  };
}
