import { ActiveMQService } from "@bindings/bridge";
import type {
  ActiveMQMoveInput,
  ActiveMQPublishInput,
} from "@bindings/bridge/models";
import type {
  ClientConnection,
  DeadLetterQueue,
  PublishResult,
} from "@bindings/model/models";
import { present, required } from "./client";

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

/**
 * Register a durable subscription on a topic.
 *
 * Not through the canonical create: that one carries a broker address, a
 * consume mode and a retry count, and carries no topic - which is the one
 * thing a durable subscription cannot be made without.
 */
export const createSubscription = (
  connID: number,
  topic: string,
  name: string,
  selector: string,
): Promise<void> => ActiveMQService.CreateSubscription(connID, { topic, name, selector });

/** Unsubscribe, discarding whatever the subscription was still owed. */
export const removeSubscription = (connID: number, name: string): Promise<void> =>
  ActiveMQService.RemoveSubscription(connID, name);

/**
 * The destinations dead letters land in, and what feeds them.
 *
 * The sources are filled on Artemis and empty on Classic, and that is the
 * broker rather than the app: Artemis records a dead-letter address on every
 * queue, so the topology can be walked backwards; Classic decides by a
 * broker-wide policy and keeps no record of where a dead letter came from.
 */
export const deadLetterQueues = (connID: number): Promise<DeadLetterQueue[]> =>
  ActiveMQService.DeadLetterQueues(connID).then(present);

/**
 * Send a dead-lettered destination's contents back where each message came
 * from, reporting the broker's own count.
 *
 * The whole destination, because that is the only form either product offers:
 * retryMessages() takes no arguments on Classic or on Artemis.
 */
export const retryDeadLetters = (connID: number, name: string): Promise<number> =>
  ActiveMQService.RetryDeadLetters(connID, name);

export type { ActiveMQPublishInput };

/**
 * Send one or more messages, reporting what the broker took.
 *
 * A management operation like everything else here, so the console works on a
 * broker with every wire acceptor switched off. What it cannot do is send a
 * body that is not text: both send operations take a String, and bytes would
 * need the optional AMQP tier.
 */
export const publish = (
  connID: number,
  input: ActiveMQPublishInput,
): Promise<PublishResult> => ActiveMQService.Publish(connID, input).then(required);

/** What is holding a socket open on the broker. */
export const connections = (connID: number): Promise<ClientConnection[]> =>
  ActiveMQService.Connections(connID).then(present);

/**
 * Disconnect one client, by the broker's own connection id.
 *
 * Not by address: one host can hold twenty connections, and closing by address
 * would take all of them.
 */
export const closeConnection = (connID: number, name: string): Promise<void> =>
  ActiveMQService.CloseConnection(connID, name);
