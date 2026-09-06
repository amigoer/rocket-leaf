import { GooglePubSubService } from "@bindings/bridge";
import type { GooglePubSubTopicInput } from "@bindings/bridge/models";

export type { GooglePubSubTopicInput };

/**
 * The Pub/Sub-only half of the surface.
 *
 * Reading topics is not here: a topic is a destination, and api/topic.ts
 * already answers the whole read side. What this carries is what the canonical
 * services cannot express - a create whose fields are a retention and a set of
 * labels, where TopicService.Create collects a broker address, two queue
 * counts and a permission string.
 */

/** Declare a topic in the connection's project. */
export const createTopic = (connID: number, input: GooglePubSubTopicInput): Promise<void> =>
  GooglePubSubService.CreateTopic(connID, input);

/**
 * Change an existing topic's settings.
 *
 * Only what the form sends is written, except for labels: the update mask
 * names the field rather than one key, so the set is replaced wholesale and
 * the form has to send all of it.
 */
export const updateTopic = (connID: number, input: GooglePubSubTopicInput): Promise<void> =>
  GooglePubSubService.UpdateTopic(connID, input);

/**
 * Delete a topic.
 *
 * Its subscriptions survive it. They keep whatever they had not delivered,
 * report their topic as _deleted-topic_ from then on, and can never receive
 * another message - which is what the subscriptions board shows.
 */
export const removeTopic = (connID: number, name: string): Promise<void> =>
  GooglePubSubService.RemoveTopic(connID, name);
