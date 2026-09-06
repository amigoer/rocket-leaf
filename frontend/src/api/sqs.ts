import { SQSService } from "@bindings/bridge";
import type {
  SQSPublishInput,
  SQSPublishResult,
  SQSQueueInput,
} from "@bindings/bridge/models";
import type { DeadLetterQueue } from "@bindings/model/models";
import { present, required } from "./client";

export type { SQSPublishInput, SQSPublishResult, SQSQueueInput };

/**
 * The SQS-only half of the surface.
 *
 * Reading queues is not here: a queue is a destination, and api/topic.ts
 * already answers the whole read side. What this carries is what the canonical
 * services cannot express - a create whose fields are durations and a redrive
 * policy, where TopicService.Create collects a broker address, two queue
 * counts and a permission string.
 */

/**
 * Declare a queue in the connection's region.
 *
 * Whether it is FIFO is decided by the name: SQS requires the .fifo suffix on
 * a FIFO queue and refuses it on a standard one, so the flag and the name have
 * to agree and a mismatch is refused before the request is sent.
 */
export const createQueue = (connID: number, input: SQSQueueInput): Promise<void> =>
  SQSService.CreateQueue(connID, input);

/**
 * Change an existing queue's settings.
 *
 * Only what the form sends is written. SQS replaces exactly the attributes it
 * is given, so a field left at zero keeps whatever is stored rather than
 * resetting it - which is what lets an edit change one setting alone.
 */
export const updateQueue = (connID: number, input: SQSQueueInput): Promise<void> =>
  SQSService.UpdateQueue(connID, input);

/**
 * Delete a queue and everything in it.
 *
 * There is no undo, and the name is refused for 60 seconds afterwards: a queue
 * recreated straight away fails, which is the service rather than the app.
 */
export const removeQueue = (connID: number, name: string): Promise<void> =>
  SQSService.RemoveQueue(connID, name);

/**
 * Discard everything the queue holds.
 *
 * The call returning is not the queue being empty. SQS purges asynchronously,
 * may also delete anything sent in the following minute, and may go on
 * delivering what was sent just before it for about as long. One purge per
 * queue per minute is allowed.
 */
export const purgeQueue = (connID: number, name: string): Promise<void> =>
  SQSService.PurgeQueue(connID, name);

/**
 * Send one body, or the same body several times, to one queue.
 *
 * Batched in tens, which is the service's own maximum, and a batch's entries
 * succeed and fail individually - so a send that stopped partway reports how
 * many the service took before it did.
 */
export const publish = (connID: number, input: SQSPublishInput): Promise<SQSPublishResult> =>
  SQSService.Publish(connID, input).then(required);

/**
 * Find the queues other queues redrive into, and what points at each.
 *
 * A walk backwards rather than a lookup: nothing marks a dead-letter queue in
 * SQS, it is an ordinary queue with another queue's redrive policy aimed at
 * it. The walk starts from what the connection's queue prefix let through, so
 * a dead-letter queue every one of whose sources sits outside that prefix is
 * not found - widening the prefix is what makes it appear.
 */
export const deadLetterQueues = (connID: number): Promise<DeadLetterQueue[]> =>
  SQSService.DeadLetterQueues(connID).then(present);
