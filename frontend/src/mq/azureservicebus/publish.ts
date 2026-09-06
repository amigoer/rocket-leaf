/**
 * What the Service Bus send console collects, and the rules it enforces.
 *
 * Separate from the form component so the rules can be tested without
 * rendering. Each catches something the service would either refuse with a
 * message naming nothing useful, or accept and quietly get wrong:
 *
 *   - A property with a value and no name cannot be sent, and one with neither
 *     is a row somebody added and has not filled in yet.
 *   - A count outside this app's own cap, mirrored from the driver.
 *   - A session id on an entity that has no sessions is carried and ignored,
 *     which is worth saying rather than refusing.
 *
 * The warning is the one no other console here has in this shape: a topic
 * stores nothing, so a send to one with no subscription is accepted, reported
 * as sent, and discarded with no backlog anywhere to notice it by. A queue is
 * never in that state, which is why the check is about the entity's kind.
 */
import type { AzureServiceBusSendInput } from "@bindings/bridge/models";

/**
 * How many copies one send may carry.
 *
 * This app's cap rather than the service's, mirrored from
 * internal/driver/azureservicebus/publish.go: a send console is for producing
 * a handful by hand, and every message costs an operation Azure bills for.
 */
export const MAX_COUNT = 1000;

export interface ServiceBusProducerDraft {
  /** A queue or a topic. A subscription cannot be sent to. */
  entity: string;
  body: string;
  count: number;
  /** Also called the label, and what a correlation filter matches by name. */
  subject: string;
  correlationId: string;
  /** Orders delivery within a session; required on an entity with sessions on. */
  sessionId: string;
  /** Schedules the message instead of sending it now. Zero sends it now. */
  delaySec: number;
  /** The sender's own properties, and what a SQL rule selects on. */
  properties: { name: string; value: string }[];
}

export function emptyServiceBusProducerDraft(): ServiceBusProducerDraft {
  return {
    entity: "",
    body: "",
    count: 1,
    subject: "",
    correlationId: "",
    sessionId: "",
    delaySec: 0,
    properties: [],
  };
}

function namedProperties(draft: ServiceBusProducerDraft): Record<string, string> {
  const properties: Record<string, string> = {};
  for (const row of draft.properties) {
    const name = row.name.trim();
    if (name !== "") properties[name] = row.value;
  }
  return properties;
}

/** Why the draft cannot be sent, as an i18n key, or null when it can. */
export function sendProblem(draft: ServiceBusProducerDraft): string | null {
  if (draft.entity.trim() === "") return "board.azure-servicebus.producer.entityRequired";
  if (draft.count < 1 || draft.count > MAX_COUNT) {
    return "board.azure-servicebus.producer.countRange";
  }
  if (draft.properties.some((row) => row.name.trim() === "" && row.value !== "")) {
    return "board.azure-servicebus.producer.propertyNameRequired";
  }
  if (draft.delaySec < 0) return "board.azure-servicebus.producer.delayNegative";
  // A Service Bus message may be empty, unlike a Pub/Sub one, so nothing is
  // refused for having no body: an empty message with properties is exactly
  // how a filtered subscription is exercised without a payload.
  return null;
}

/**
 * What the console warns about but will still send, as an i18n key.
 *
 * `subscribers` is how many subscriptions the chosen topic has, and is null
 * for a queue or for an entity the listing did not include. Zero is the one
 * worth stopping at: the send is accepted, reported as sent, and thrown away,
 * and no board anywhere afterwards records that it happened.
 */
export function sendWarning(
  draft: ServiceBusProducerDraft,
  subscribers: number | null,
): string | null {
  if (draft.entity.trim() !== "" && subscribers === 0) {
    return "board.azure-servicebus.producer.noSubscriberWarning";
  }
  return null;
}

/** The input to send, or null when the draft is not yet whole. */
export function toSendInput(draft: ServiceBusProducerDraft): AzureServiceBusSendInput | null {
  if (sendProblem(draft) != null) return null;
  return {
    entity: draft.entity.trim(),
    body: draft.body,
    count: draft.count,
    subject: draft.subject.trim(),
    correlationId: draft.correlationId.trim(),
    contentType: "",
    sessionId: draft.sessionId.trim(),
    partitionKey: "",
    properties: namedProperties(draft),
    delaySec: draft.delaySec,
    ttlSec: 0,
  };
}
