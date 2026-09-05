import { NSQService } from "@bindings/bridge";

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
