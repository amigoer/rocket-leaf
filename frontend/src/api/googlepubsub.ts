import { GooglePubSubService } from "@bindings/bridge";
import type {
  GooglePubSubPublishInput,
  GooglePubSubPublishResult,
  GooglePubSubSnapshot,
  GooglePubSubSubscriptionInput,
  GooglePubSubTopicInput,
} from "@bindings/bridge/models";
import { present, required } from "./client";

export type {
  GooglePubSubPublishInput,
  GooglePubSubPublishResult,
  GooglePubSubSnapshot,
  GooglePubSubSubscriptionInput,
  GooglePubSubTopicInput,
};

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

/**
 * Declare a subscription on a topic.
 *
 * The topic, the filter and message ordering are fixed at creation: a
 * subscription reads exactly one topic and the service refuses to change any
 * of the three afterwards.
 */
export const createSubscription = (
  connID: number,
  input: GooglePubSubSubscriptionInput,
): Promise<void> => GooglePubSubService.CreateSubscription(connID, input);

/**
 * Change what a subscription lets be changed.
 *
 * Only what the form sends is written. An empty dead-letter topic is how the
 * policy is removed - there is no separate call, and leaving the field out
 * keeps it instead.
 */
export const updateSubscription = (
  connID: number,
  input: GooglePubSubSubscriptionInput,
): Promise<void> => GooglePubSubService.UpdateSubscription(connID, input);

/**
 * Delete a subscription and everything it had not acknowledged.
 *
 * Those messages go with it. They were never the topic's to hand out again,
 * which is the whole point of the split between the two objects.
 */
export const removeSubscription = (connID: number, name: string): Promise<void> =>
  GooglePubSubService.RemoveSubscription(connID, name);

/** Every restore point in the project. */
export const listSnapshots = (connID: number): Promise<GooglePubSubSnapshot[]> =>
  GooglePubSubService.ListSnapshots(connID).then(present);

/** Take a restore point from one subscription. */
export const createSnapshot = (
  connID: number,
  name: string,
  subscription: string,
): Promise<void> => GooglePubSubService.CreateSnapshot(connID, name, subscription);

/**
 * Delete a restore point.
 *
 * Worth doing rather than waiting for the seven-day expiry: while it exists
 * the topic keeps every message the snapshot could restore.
 */
export const removeSnapshot = (connID: number, name: string): Promise<void> =>
  GooglePubSubService.RemoveSnapshot(connID, name);

/**
 * Move a subscription to a restore point.
 *
 * The other half of Seek - moving to a moment in time - goes through the
 * shared consumer API, because that is what every family's reset is.
 */
export const seekToSnapshot = (
  connID: number,
  subscription: string,
  snapshot: string,
): Promise<void> => GooglePubSubService.SeekToSnapshot(connID, subscription, snapshot);

/**
 * Send one body, or the same body several times, to one topic.
 *
 * Accepted is not delivered: a topic stores nothing, so the publish reaches
 * whatever subscriptions exist at that instant and is discarded if none do,
 * and the service reports success either way.
 */
export const publish = (
  connID: number,
  input: GooglePubSubPublishInput,
): Promise<GooglePubSubPublishResult> =>
  GooglePubSubService.Publish(connID, input).then(required);
