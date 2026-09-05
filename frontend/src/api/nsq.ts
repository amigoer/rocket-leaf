import { NSQService } from "@bindings/bridge";
import type { NSQPublishInput, NSQPublishResult } from "@bindings/bridge/models";
import { present, required } from "./client";

export type { NSQPublishInput };

/**
 * The NSQ-only half of the surface.
 *
 * Reading topics is not here: a topic is a destination, and api/topic.ts
 * already answers the whole read side. What this carries is what the canonical
 * services cannot express - a create with no vocabulary but a name, and a
 * pause, which no other family in this app has.
 */

/**
 * Declare a topic on every nsqd in the connection.
 *
 * Not through the canonical create: that one collects a broker address, two
 * queue counts and a permission string, and an NSQ topic has none of them. It
 * is created everywhere rather than on one daemon because a producer that
 * connected to another would otherwise auto-create it later with none of its
 * channels.
 */
export const createTopic = (connID: number, name: string): Promise<void> =>
  NSQService.CreateTopic(connID, name);

/**
 * Delete a topic, its channels and its registration in the discovery tier.
 *
 * The last part is not decoration: nsqd forgets a deleted topic and
 * nsqlookupd does not, so a delete that stopped at nsqd leaves the name where
 * a consumer looking it up still finds it.
 */
export const removeTopic = (connID: number, name: string): Promise<void> =>
  NSQService.RemoveTopic(connID, name);

/**
 * Discard everything the topic and every channel under it are holding.
 *
 * The channels are the point. nsqd copies each message into every channel as
 * it arrives, and emptying the topic alone touches only its own queue - which
 * on a topic anything is consuming is already zero.
 */
export const emptyTopic = (connID: number, name: string): Promise<void> =>
  NSQService.EmptyTopic(connID, name);

/**
 * Stop or resume delivery into a topic's channels.
 *
 * Publishing carries on while a topic is paused: what stops is the copy into
 * each channel, so the messages pile up in the topic itself rather than being
 * refused.
 */
export const setTopicPaused = (connID: number, name: string, paused: boolean): Promise<void> =>
  NSQService.SetTopicPaused(connID, name, paused);

/**
 * Declare a channel on every nsqd carrying its topic.
 *
 * There is no position to start it from. What it gets is whatever nsqd is
 * still holding: nothing, on a topic that already has a channel, because the
 * copies were made as the messages arrived; and the topic's own queue, on one
 * that had no channel to copy into.
 */
export const createChannel = (connID: number, topic: string, channel: string): Promise<void> =>
  NSQService.CreateChannel(connID, { topic, channel });

/** Delete a channel and its backlog, in the discovery tier as well as on nsqd. */
export const removeChannel = (connID: number, topic: string, channel: string): Promise<void> =>
  NSQService.RemoveChannel(connID, { topic, channel });

/**
 * Discard one channel's backlog.
 *
 * Not the same as emptying the topic, which reaches every channel under it:
 * this leaves the others holding their own copies.
 */
export const emptyChannel = (connID: number, topic: string, channel: string): Promise<void> =>
  NSQService.EmptyChannel(connID, { topic, channel });

/**
 * Stop or resume delivery to one channel's consumers.
 *
 * Its consumers stay connected and receive nothing; the other channels under
 * the topic keep running, which is what separates this from pausing the topic.
 */
export const setChannelPaused = (
  connID: number,
  topic: string,
  channel: string,
  paused: boolean,
): Promise<void> => NSQService.SetChannelPaused(connID, { topic, channel }, paused);

/**
 * Send one body, or the same body several times, through one nsqd.
 *
 * Which daemon matters more than it looks: the message is held by the nsqd
 * that took it, and a consumer connected to a different one sees it only if it
 * also finds this daemon through nsqlookupd.
 */
export const publish = (connID: number, input: NSQPublishInput): Promise<NSQPublishResult> =>
  NSQService.Publish(connID, input).then(required);

/** The daemons the send console can address, as the profile names them. */
export const nodes = (connID: number): Promise<string[]> =>
  NSQService.Nodes(connID).then(present);
