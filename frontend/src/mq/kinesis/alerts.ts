/**
 * What is wrong with a Kinesis region right now.
 *
 * RocketMQ's rules, which every family without its own falls back to, read a
 * broker ordinal, a commit log's disk percentage, a consumer group's instance
 * count and a dead-letter topic named after a group. Kinesis has none of
 * those: AWS runs the service, so there is no node and no disk figure; nothing
 * is ever moved aside, so there is no dead-letter anything; and no reader's
 * position exists anywhere the API can reach. A connection left on the
 * defaults would run five rules that all read zero and could never fire.
 *
 * Two rules, and both describe the same thing about two different objects: an
 * operation that has not finished. They are separate switches because they are
 * separate faults with separate fixes - one is a stream operation an operator
 * started, the other a registration an application is waiting on - and because
 * a region being resized on purpose wants the first one off without losing the
 * second.
 *
 * Two rules a reader might expect are absent, and both would be actively
 * wrong here rather than merely quiet:
 *
 *   - topicUnsubscribed would fire on almost every healthy stream. A
 *     registered consumer is the enhanced fan-out kind, and the ordinary way
 *     to read a Kinesis stream registers nothing at all - so "no consumers"
 *     is the normal state of a stream three applications are reading.
 *   - queueNoConsumer is the same mistake with a number attached, and there
 *     is no number: nothing in the service counts what a stream is holding.
 *
 * Three more are absent for the plainer reason that there is nothing to read:
 * groupLag and queueBacklog need a position, which no Kinesis connection can
 * report; diskUsage needs a storage figure, and AWS publishes none.
 */
import type { AlertFacts, AlertThresholds, DerivedAlert } from "@/lib/alertDerive";
import type { AlertRulePrefs } from "@/lib/alertRules";
import { stream } from "./destinations";
import { consumer } from "./subscriptions";

export function deriveKinesisAlerts(
  facts: AlertFacts,
  rules: AlertRulePrefs,
  _thresholds: AlertThresholds,
): DerivedAlert[] {
  const out: DerivedAlert[] = [];

  if (rules.streamNotActive) {
    for (const entry of facts.destinations.map(stream)) {
      if (entry.status == null || entry.status === "ACTIVE") continue;
      out.push({
        key: `stream-not-active-${entry.name}`,
        ruleKey: "streamNotActive",
        // A warning rather than critical: a stream mid-resize is a normal
        // minute, and one that stays this way is what the reader has to
        // decide about. Nothing distinguishes the two from one snapshot.
        severity: "warn",
        params: { stream: entry.name, status: entry.status },
      });
    }
  }

  if (rules.consumerNotActive) {
    for (const entry of facts.consumerGroups.map(consumer)) {
      if (entry.status == null || entry.status === "ACTIVE") continue;
      out.push({
        key: `consumer-not-active-${entry.stream}-${entry.name}`,
        ruleKey: "consumerNotActive",
        severity: "warn",
        params: { consumer: entry.name, stream: entry.stream, status: entry.status },
      });
    }
  }

  return out;
}
