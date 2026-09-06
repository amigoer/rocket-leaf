import { IBMMQService } from "@bindings/bridge";
import type {
  IBMMQDestinationInput,
  IBMMQPublishInput,
  IBMMQPublishResult,
} from "@bindings/bridge/models";
import type { Channel, DeadLetterQueue } from "@bindings/model/models";
import { present, required } from "./client";

export type { IBMMQDestinationInput, IBMMQPublishInput, IBMMQPublishResult };

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

/**
 * Every channel definition, with what is running folded in.
 *
 * Definitions rather than connections: a channel is listed whether or not
 * anything is using it, because it is what decides whether anything may.
 */
export const channels = async (connID: number): Promise<Channel[]> =>
  present(await IBMMQService.Channels(connID));

/**
 * Send one body, or the same body several times, to one queue.
 *
 * A queue, not a topic: the messaging REST API has no topic resource, so
 * publishing needs an MQ client and this console does not pretend otherwise.
 * Each copy is its own request, which is why the count is capped.
 */
export const publish = (
  connID: number,
  input: IBMMQPublishInput,
): Promise<IBMMQPublishResult> => IBMMQService.Publish(connID, input).then(required);

/**
 * The queues something else dead-letters into.
 *
 * Found by walking every queue's configuration backwards: nothing marks a
 * dead-letter queue here, and what makes one is the queue manager's DEADQ
 * attribute or another queue's backout queue pointing at it.
 */
export const deadLetterQueues = async (connID: number): Promise<DeadLetterQueue[]> =>
  present(await IBMMQService.DeadLetterQueues(connID));
