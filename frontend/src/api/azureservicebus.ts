import { AzureServiceBusService } from "@bindings/bridge";
import type { AzureServiceBusEntityInput } from "@bindings/bridge/models";

export type { AzureServiceBusEntityInput };

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
