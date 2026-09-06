import { IBMMQService } from "@bindings/bridge";
import type { IBMMQDestinationInput } from "@bindings/bridge/models";

export type { IBMMQDestinationInput };

/**
 * The IBM MQ-only half of the surface.
 *
 * Reading queues and topics is not here: both are destinations, and
 * api/topic.ts already answers the whole read side. What this carries is what
 * the canonical services cannot express - a create whose fields depend on
 * which of the two objects is being made, where TopicService.Create collects a
 * broker address, two queue counts and a permission string.
 */

/** Which queue manager this connection speaks to. */
export const queueManager = (connID: number): Promise<string> => IBMMQService.QueueManager(connID);

/**
 * Declare a queue or a topic on the queue manager.
 *
 * The kind decides everything else: a queue takes a type and a maximum depth,
 * a topic takes the string publishers name. A topic without one would be an
 * object nothing ever publishes through.
 */
export const createDestination = (connID: number, input: IBMMQDestinationInput): Promise<void> =>
  IBMMQService.CreateDestination(connID, input);

/**
 * Delete a queue or a topic.
 *
 * Without purge the queue manager refuses a queue that holds messages, which
 * is the check worth keeping; with it the messages go too and there is no
 * undo. A queue an application has open is refused either way.
 */
export const removeDestination = (connID: number, name: string, purge: boolean): Promise<void> =>
  IBMMQService.RemoveDestination(connID, name, purge);
