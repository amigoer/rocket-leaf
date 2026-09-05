/**
 * What is wrong with an NSQ cluster right now.
 *
 * RocketMQ's rules, which every family without its own falls back to, read a
 * broker ordinal, a commit log's disk percentage and a dead-letter topic named
 * after a consumer group. NSQ reports none of those - it has no disk figure at
 * all and no dead letters anywhere - so a connection left on the default rules
 * would run five rules that all read zero and could never fire: an alerts page
 * that looks armed and is not.
 *
 * Three of the four rules here are existing keys, because a channel with a
 * backlog and nothing attached, a backlog past a threshold, and a topic
 * holding messages nothing has taken are all questions another family already
 * asks. The fourth is this family's own and had to be: a pause is invisible
 * everywhere else on the app, publishing carries on while it is in force, and
 * every other figure keeps looking healthy while the backlog grows.
 *
 * Two rules a reader might expect are absent.
 *
 * brokerOffline cannot fire. A daemon that has stopped answering fails the
 * whole read rather than appearing in the list as offline - the node list is
 * the profile's own addresses, and every figure elsewhere is a sum over them,
 * so a connection cannot half-succeed.
 *
 * A node whose health is not OK is left to the cluster board. nsqd's health is
 * one word it sets when a write to the overflow file fails, and no rule
 * sentence here describes that: resourceAlarm says the alarm blocks every
 * publisher, which is more than nsqd's flag actually claims.
 */
import type { AlertFacts, AlertThresholds, DerivedAlert } from "@/lib/alertDerive";
import type { AlertRulePrefs } from "@/lib/alertRules";
import { channel, channelKey } from "./subscriptions";
import { topic } from "./destinations";

export function deriveNsqAlerts(
  facts: AlertFacts,
  rules: AlertRulePrefs,
  thresholds: AlertThresholds,
): DerivedAlert[] {
  const out: DerivedAlert[] = [];
  const { lag: lagThreshold } = thresholds;

  for (const row of facts.consumerGroups) {
    const entry = channel(row);
    const name = channelKey(entry);
    const backlog = entry.backlog ?? 0;

    // A pause outranks the backlog rules on the same channel. It is the
    // explanation for the backlog, and firing both would put one problem on
    // the page twice with the weaker sentence second.
    if (rules.deliveryPaused && entry.paused && backlog > 0) {
      out.push({
        key: `channel-paused-${name}`,
        ruleKey: "deliveryPaused",
        severity: "warn",
        params: { target: name, lag: backlog },
      });
      continue;
    }

    if (rules.groupOffline && backlog > 0 && entry.clients === 0) {
      // Narrower than the rule's name, deliberately. A channel with no
      // consumer and nothing waiting is idle rather than broken - that is
      // what a durable channel is for - so this fires only where the messages
      // are piling up with nothing to take them.
      out.push({
        key: `channel-no-consumer-${name}`,
        ruleKey: "groupOffline",
        severity: "warn",
        params: { group: name, lag: backlog },
      });
    } else if (rules.groupLag && lagThreshold > 0 && backlog >= lagThreshold) {
      // Else rather than as well: a channel nobody is draining is already the
      // stronger statement about the same backlog.
      out.push({
        key: `channel-lag-${name}`,
        ruleKey: "groupLag",
        severity: "warn",
        params: { group: name, lag: backlog, threshold: lagThreshold },
      });
    }
  }

  for (const row of facts.destinations) {
    const entry = topic(row);
    const held = entry.topicDepth ?? 0;

    if (rules.deliveryPaused && entry.paused && held > 0) {
      out.push({
        key: `topic-paused-${entry.name}`,
        ruleKey: "deliveryPaused",
        severity: "warn",
        params: { target: entry.name, lag: held },
      });
      continue;
    }

    /*
     * A topic holding messages that reached no channel.
     *
     * The one state where a message can be lost by neglect rather than by
     * failure: nsqd copies a message into the channels that exist when it
     * arrives, so a topic nothing has ever subscribed to accumulates in its
     * own queue - and unless a channel is created, everything past the memory
     * queue is written to disk and read by nobody.
     */
    if (rules.queueNoConsumer && held > 0 && entry.channels.length === 0) {
      out.push({
        key: `topic-no-channel-${entry.name}`,
        ruleKey: "queueNoConsumer",
        severity: "warn",
        params: { queue: entry.name, lag: held },
      });
    }
  }

  return out;
}
