/**
 * What is wrong with an Azure Service Bus namespace right now.
 *
 * RocketMQ's rules, which every family without its own falls back to, read a
 * broker ordinal, a commit log's disk percentage and a consumer group's
 * instance count. Service Bus has none of those: Microsoft runs the service,
 * so there is no node and no disk figure, and nothing registers as a consumer.
 * A connection left on the defaults would run five rules that all read zero
 * and could never fire.
 *
 * Four rules, and two of them are this family's own.
 *
 *   - A topic with no subscription discards every send on arrival. It is the
 *     rule Pub/Sub brought, and it is the same fault: the sender is told the
 *     message was accepted, and there is no backlog anywhere to notice it by.
 *   - A subscription with no rules receives nothing. Nothing else in the app
 *     shows it: the subscription exists, reports itself Active, and has an
 *     empty backlog because nothing can arrive. It is reached by deleting the
 *     $Default rule and not replacing it, which is a normal minute in a deploy
 *     and a leak if it is left.
 *   - An entity that is disabled, or has sends or receives switched off, is
 *     the quietest of the four: a send to a SendDisabled queue is refused at
 *     the client with an error nothing on a board would show, and a
 *     ReceiveDisabled one fills up while looking healthy.
 *   - Anything in a dead-letter store. Every queue and subscription has one,
 *     so this reads a figure that is always there rather than a topology that
 *     may not be.
 *
 * The backlog rules are deliberately not here, and the reason is the emulator
 * rather than the service. Service Bus reports an active message count on
 * every entity and a real namespace answers it, but a page cannot tell "the
 * queue is empty" from "this endpoint reports no counts" without reading the
 * capability - and an alert that fires on the second would be an alert about
 * the emulator. Depths belong on the boards, which draw a dash for the
 * difference.
 */
import type { AlertFacts, AlertThresholds, DerivedAlert } from "@/lib/alertDerive";
import type { AlertRulePrefs } from "@/lib/alertRules";
import { entity } from "./entities";
import { subscription } from "./subscriptions";

export function deriveAzureServiceBusAlerts(
  facts: AlertFacts,
  rules: AlertRulePrefs,
  _thresholds: AlertThresholds,
): DerivedAlert[] {
  const out: DerivedAlert[] = [];

  for (const row of facts.destinations.map(entity)) {
    // A topic nothing reads. A queue is never in this state: it holds what is
    // sent to it whether or not anything is consuming.
    if (rules.topicUnsubscribed && row.kind === "topic" && row.subscribers === 0) {
      out.push({
        key: `topic-unsubscribed-${row.name}`,
        ruleKey: "topicUnsubscribed",
        // A warning rather than critical: a topic created ahead of its
        // subscription is a normal minute in a deploy, and one that stays
        // this way is what the reader has to decide about.
        severity: "warn",
        params: { topic: row.name },
      });
    }

    if (rules.entityDisabled && row.status != null && row.status !== "Active") {
      out.push({
        key: `entity-disabled-${row.name}`,
        ruleKey: "entityDisabled",
        severity: "warn",
        params: { entity: row.name, status: row.status },
      });
    }

    // Dead letters on a queue. Null rather than zero is an endpoint that
    // reports no counts, and must not read as an empty store.
    if (rules.dlqGrowth && row.deadLetterCount != null && row.deadLetterCount > 0) {
      out.push({
        key: `dlq-${row.name}`,
        ruleKey: "dlqGrowth",
        severity: "warn",
        // group and count are what alerts.detail.dlqGrowth interpolates. The
        // "group" is the entity, because a dead letter belongs to the entity
        // rather than to whoever was reading it.
        params: { group: row.name, count: row.deadLetterCount },
      });
    }
  }

  for (const row of facts.consumerGroups.map(subscription)) {
    const path = `${row.topic}/${row.name}`;

    if (rules.subscriptionUnroutable && row.ruleNames.length === 0) {
      out.push({
        key: `subscription-unroutable-${path}`,
        ruleKey: "subscriptionUnroutable",
        severity: "warn",
        params: { subscription: path },
      });
    }

    if (rules.entityDisabled && row.status != null && row.status !== "Active") {
      out.push({
        key: `entity-disabled-${path}`,
        ruleKey: "entityDisabled",
        severity: "warn",
        params: { entity: path, status: row.status },
      });
    }

    if (rules.dlqGrowth && row.deadLetterCount != null && row.deadLetterCount > 0) {
      out.push({
        key: `dlq-${path}`,
        ruleKey: "dlqGrowth",
        severity: "warn",
        params: { group: path, count: row.deadLetterCount },
      });
    }
  }

  return out;
}
