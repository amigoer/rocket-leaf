import { SolaceService } from "@bindings/bridge";
import { present, required } from "./client";
import type { DeadLetterQueue } from "@bindings/model/models";
import type {
  SolacePublishInput,
  SolacePublishResult,
  SolaceQueueInput,
} from "@bindings/bridge/models";

export type { SolacePublishInput, SolacePublishResult, SolaceQueueInput };

/**
 * The Solace-only half of the surface.
 *
 * Reading queues is not here: a queue is a destination, and api/topic.ts
 * already answers the whole read side. What this carries is what the canonical
 * services cannot express - a create whose fields decide how every consumer
 * that ever binds to the queue behaves, where TopicService.Create collects a
 * broker address, two queue counts and a permission string.
 */

/**
 * Which Message VPN this connection reads.
 *
 * Worth asking for on a board rather than reading off the profile: the sidebar
 * re-points a connection at another VPN without editing it, so the profile and
 * the live connection can disagree until the next reload.
 */
export const msgVpn = (connID: number): Promise<string> => SolaceService.MsgVPN(connID);

/**
 * Declare a queue in this connection's Message VPN.
 *
 * The access type is the field that matters most: exclusive hands every
 * message to one consumer and keeps the rest waiting, non-exclusive shares.
 */
export const createQueue = (connID: number, input: SolaceQueueInput): Promise<void> =>
  SolaceService.CreateQueue(connID, input);

/**
 * Delete a queue and whatever it was holding.
 *
 * There is no purge flag because SEMP has no precondition to ask for: it
 * deletes a full queue as readily as an empty one, and the messages are gone
 * rather than moved. The board's confirmation is the only thing in the way.
 */
export const removeQueue = (connID: number, name: string): Promise<void> =>
  SolaceService.RemoveQueue(connID, name);

/**
 * Send one body, or the same body several times.
 *
 * To a queue by name or to a topic to be matched, which are two different acts
 * rather than one field with two spellings: a topic send lands on every queue
 * whose subscriptions match and on nothing at all when none do.
 *
 * The result carries no message id, and that is the interface: the broker
 * answers a send it took with an empty body and no identifier of any kind.
 */
export const publish = (
  connID: number,
  input: SolacePublishInput,
): Promise<SolacePublishResult> => SolaceService.Publish(connID, input).then(required);

/**
 * The queues something else dead-letters into.
 *
 * Found by walking every endpoint's pointer backwards: nothing marks a dead
 * message queue on this family. An entry whose depth is unknown is one the
 * Message VPN does not hold - the ordinary state of the pointer every endpoint
 * ships with, and what makes a message given up on disappear rather than move.
 */
export const deadMsgQueues = async (connID: number): Promise<DeadLetterQueue[]> =>
  present(await SolaceService.DeadMsgQueues(connID));
