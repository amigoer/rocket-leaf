import { ActiveMQService } from "@bindings/bridge";
import type { ActiveMQMoveInput } from "@bindings/bridge/models";

export type { ActiveMQMoveInput };

/**
 * The ActiveMQ-only half of the surface.
 *
 * Reading destinations is not here: a queue and a topic are both destinations,
 * and api/topic.ts already answers the whole read side. What this carries is
 * what the canonical services cannot express - operations whose shape is JMS's
 * rather than the shared vocabulary's.
 */

/**
 * Empty a destination. There is no undo.
 *
 * On an Artemis topic this reaches every subscription under the address rather
 * than the address itself, which holds nothing of its own.
 */
export const purgeQueue = (connID: number, name: string): Promise<void> =>
  ActiveMQService.PurgeQueue(connID, name);

/**
 * Drain one destination into another, reporting the broker's own count.
 *
 * The count is what separates a move that matched nothing from one that moved
 * everything, and both are otherwise a successful call.
 */
export const moveMessages = (connID: number, input: ActiveMQMoveInput): Promise<number> =>
  ActiveMQService.MoveMessages(connID, input);

/**
 * Declare a queue or a topic.
 *
 * Not through the canonical create: that one collects a broker address, a read
 * queue count and a permission string, and a JMS destination has none of them.
 * What it does have is the one thing that shape cannot carry - whether it is a
 * queue or a topic, which is not inferable from the name.
 */
export const createDestination = (
  connID: number,
  name: string,
  topic: boolean,
): Promise<void> => ActiveMQService.CreateDestination(connID, { name, topic });

/** Delete a destination and everything it holds. */
export const removeDestination = (connID: number, name: string): Promise<void> =>
  ActiveMQService.RemoveDestination(connID, name);
