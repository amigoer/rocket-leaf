/**
 * What is wrong with a Google Pub/Sub project right now.
 *
 * RocketMQ's rules, which every family without its own falls back to, read a
 * broker ordinal, a commit log's disk percentage, a consumer group's instance
 * count and a dead-letter topic named after a group. Pub/Sub has none of
 * those: Google runs the service so there is no node and no disk figure, and a
 * subscription's backlog is a Cloud Monitoring metric this connection cannot
 * read. A connection left on the defaults would run five rules that all read
 * zero and could never fire - an alerts page that looks armed and is not.
 *
 * Two rules, and neither has a counterpart on any other family here, because
 * both describe something only a fan-out service can do: accept a message,
 * report success, and have nowhere to put it.
 *
 *   - A topic with no subscription discards every publish on arrival. Nothing
 *     downstream records it, there is no backlog to notice it by, and the
 *     publisher is told the send succeeded.
 *   - A subscription whose topic has been deleted will never receive another
 *     message. It still holds what it had not delivered and is still billed
 *     for it, and every other figure about it looks ordinary.
 *
 * Five rules a reader might expect are absent, and each for a reason about the
 * service:
 *
 *   - brokerOffline cannot fire. There is no node: a credential that stops
 *     working fails the whole read rather than marking a row down.
 *   - groupLag and queueBacklog have nothing to read. The backlog is
 *     num_undelivered_messages, a Cloud Monitoring metric, and inventing one
 *     would mean pulling the backlog to count it.
 *   - groupOffline cannot fire either. Pub/Sub keeps no record of who is
 *     pulling a subscription, so "nothing is consuming this" is a claim the
 *     service cannot support.
 *   - dlqGrowth has no depth to compare. A dead-letter topic is a topic and
 *     holds nothing countable; what is worth saying about one is that nothing
 *     subscribes to it, which the first rule below already covers.
 *   - diskUsage has nothing to read. Google reports no storage figure at all.
 */
import type { AlertFacts, AlertThresholds, DerivedAlert } from "@/lib/alertDerive";
import type { AlertRulePrefs } from "@/lib/alertRules";
import { topic } from "./topics";
import { subscription } from "./subscriptions";

export function deriveGooglePubSubAlerts(
  facts: AlertFacts,
  rules: AlertRulePrefs,
  _thresholds: AlertThresholds,
): DerivedAlert[] {
  const out: DerivedAlert[] = [];

  if (rules.topicUnsubscribed) {
    for (const entry of facts.destinations.map(topic)) {
      if (entry.subscribers !== 0) continue;
      out.push({
        key: `topic-unsubscribed-${entry.name}`,
        ruleKey: "topicUnsubscribed",
        // A warning rather than critical: a topic created ahead of its
        // subscription is a normal minute in a deploy, and one that stays
        // this way is what the reader has to decide about.
        severity: "warn",
        params: { topic: entry.name },
      });
    }
  }

  if (rules.subscriptionOrphaned) {
    for (const entry of facts.consumerGroups.map(subscription)) {
      if (!entry.orphaned) continue;
      out.push({
        key: `subscription-orphaned-${entry.name}`,
        ruleKey: "subscriptionOrphaned",
        severity: "warn",
        params: { subscription: entry.name },
      });
    }
  }

  return out;
}
