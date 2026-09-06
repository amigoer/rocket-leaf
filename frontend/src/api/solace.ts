import { SolaceService } from "@bindings/bridge";
import type { SolaceQueueInput } from "@bindings/bridge/models";

export type { SolaceQueueInput };

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
