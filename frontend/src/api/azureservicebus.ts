import { AzureServiceBusService } from "@bindings/bridge";
import type {
  AzureServiceBusEntityInput,
  AzureServiceBusSendInput,
  AzureServiceBusSendResult,
  AzureServiceBusSubscriptionInput,
} from "@bindings/bridge/models";
import { required } from "./client";

export type {
  AzureServiceBusEntityInput,
  AzureServiceBusSendInput,
  AzureServiceBusSendResult,
  AzureServiceBusSubscriptionInput,
};

/**
 * The Service Bus-only half of the surface.
 *
 * Reading entities is not here: a queue and a topic are both destinations, and
 * api/topic.ts already answers the whole read side. What this carries is what
 * the canonical services cannot express - a create whose fields are a delivery
 * contract, where TopicService.Create collects a broker address, two queue
 * counts and a permission string.
 */

/** Declare a queue or a topic in the connection's namespace. */
export const createEntity = (connID: number, input: AzureServiceBusEntityInput): Promise<void> =>
  AzureServiceBusService.CreateEntity(connID, input);

/**
 * Change an existing entity's settings.
 *
 * Sessions, duplicate detection and partitioning are fixed at creation and the
 * service refuses them here; so is the kind, and the driver refuses that
 * rather than replacing the entity with the other sort.
 */
export const updateEntity = (connID: number, input: AzureServiceBusEntityInput): Promise<void> =>
  AzureServiceBusService.UpdateEntity(connID, input);

/**
 * Delete a queue or a topic, and everything it was holding.
 *
 * Its dead letters go with it: a $DeadLetterQueue is a sub-entity rather than
 * a queue that survives its parent. A topic takes every subscription on it,
 * and their backlogs with them.
 */
export const removeEntity = (connID: number, name: string): Promise<void> =>
  AzureServiceBusService.RemoveEntity(connID, name);

/** Declare a subscription on a topic. */
export const createSubscription = (
  connID: number,
  input: AzureServiceBusSubscriptionInput,
): Promise<void> => AzureServiceBusService.CreateSubscription(connID, input);

/**
 * Change what a subscription lets be changed.
 *
 * The topic and sessions are fixed at creation, and so is what reaches it:
 * a rule is an object with a name, and the routing page is where those live.
 */
export const updateSubscription = (
  connID: number,
  input: AzureServiceBusSubscriptionInput,
): Promise<void> => AzureServiceBusService.UpdateSubscription(connID, input);

/**
 * Delete a subscription and everything it had not delivered.
 *
 * Nothing is handed back to the topic: a copy that reached this subscription
 * was never the topic's again.
 */
export const removeSubscription = (connID: number, topic: string, name: string): Promise<void> =>
  AzureServiceBusService.RemoveSubscription(connID, topic, name);

/**
 * Send one message, or the same one several times, to a queue or a topic.
 *
 * Accepted is not delivered. A queue holds what is sent to it; a topic holds
 * nothing, copying the message into every subscription whose rules let it
 * through and discarding it if none do - and reporting success either way.
 */
export const send = (
  connID: number,
  input: AzureServiceBusSendInput,
): Promise<AzureServiceBusSendResult> =>
  AzureServiceBusService.Send(connID, input).then(required);

/** Take back scheduled messages that have not been enqueued yet. */
export const cancelScheduled = (
  connID: number,
  entity: string,
  sequences: number[],
): Promise<void> => AzureServiceBusService.CancelScheduled(connID, entity, sequences);
