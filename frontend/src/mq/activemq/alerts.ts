/**
 * What is wrong with an ActiveMQ broker right now.
 *
 * RocketMQ's rules, which every family without its own falls back to, read a
 * broker ordinal, a commit log's disk percentage and a dead-letter topic named
 * after a consumer group. ActiveMQ reports none of those, so a connection left
 * on the default rules would run five rules that all read zero and could never
 * fire - an alerts page that looks armed and is not.
 *
 * Every rule here is an existing key rather than a new one, because everything
 * this family can raise is something another family already asks: a queue
 * nobody is draining, a backlog past a threshold, a store filling up, a
 * consumer the broker has marked slow, a dead-letter queue growing.
 *
 * One rule is deliberately absent that a reader might expect. groupOffline -
 * a subscription with nothing attached - is a fault on Kafka and Pulsar and is
 * the normal resting state here: a durable subscription exists precisely so
 * the broker can hold messages for a client that is not connected. Raising it
 * would fire on every healthy deployment.
 */
import type { AlertFacts, AlertThresholds, DerivedAlert } from "@/lib/alertDerive";
import type { AlertRulePrefs } from "@/lib/alertRules";
import { destination } from "./destinations";
import { node } from "./cluster";
import { subscription } from "./subscriptions";

export function deriveActiveMQAlerts(
  facts: AlertFacts,
  rules: AlertRulePrefs,
  thresholds: AlertThresholds,
): DerivedAlert[] {
  const out: DerivedAlert[] = [];
  const { lag: lagThreshold, disk: diskThreshold } = thresholds;

  for (const row of facts.destinations) {
    const entry = destination(row);
    // A dead-letter destination is measured by the dlqGrowth rule below and
    // must not also fire the two ordinary queue rules: every dead-letter queue
    // has a backlog and no consumer, which is what it is for.
    if (entry.isDeadLetter) {
      if (rules.dlqGrowth && (entry.depth ?? 0) > 0) {
        out.push({
          key: `dlq-${entry.name}`,
          ruleKey: "dlqGrowth",
          severity: "warn",
          params: { topic: entry.name, count: entry.depth ?? 0 },
        });
      }
      continue;
    }

    // Only queues. A topic with a depth and no consumer is a topic with
    // durable subscribers that are not attached, which is normal - the
    // subscriptions page is where that is read.
    if (entry.kind !== "queue") continue;

    const depth = entry.depth ?? 0;
    if (rules.queueNoConsumer && depth > 0 && (entry.consumers ?? 0) === 0) {
      out.push({
        key: `queue-no-consumer-${entry.name}`,
        ruleKey: "queueNoConsumer",
        severity: "warn",
        params: { queue: entry.name, count: depth },
      });
    } else if (rules.queueBacklog && lagThreshold > 0 && depth >= lagThreshold) {
      // Else rather than as well: a queue nobody is draining is already the
      // stronger statement about the same queue, and firing both would put
      // the same problem on the page twice.
      out.push({
        key: `queue-backlog-${entry.name}`,
        ruleKey: "queueBacklog",
        severity: "warn",
        params: { queue: entry.name, count: depth },
      });
    }
  }

  for (const row of facts.consumerGroups) {
    const entry = subscription(row);
    // Classic marks a consumer that is falling behind what is dispatched to
    // it, which is the one subscription state here that is a problem rather
    // than a resting state.
    if (rules.slowConsumer && entry.slow === true) {
      out.push({
        key: `slow-${entry.name}`,
        ruleKey: "slowConsumer",
        severity: "warn",
        params: { client: entry.subscriptionName ?? entry.name, count: entry.backlog ?? 0 },
      });
    }
  }

  for (const row of facts.nodes) {
    const broker = node(row);
    // A bridged broker answers on its own console and reports nothing here, so
    // reading its figures would be reading zeros.
    if (broker.bridge) continue;

    if (rules.diskUsage && diskThreshold > 0 && (broker.diskUsage ?? 0) >= diskThreshold) {
      out.push({
        key: `disk-${broker.name}`,
        ruleKey: "diskUsage",
        severity: (broker.diskUsage ?? 0) >= 95 ? "crit" : "warn",
        params: { broker: broker.name, percent: broker.diskUsage ?? 0 },
      });
    }
    // Memory is the watermark that actually stops a broker on this family:
    // past it, producers are blocked rather than slowed.
    if (rules.memoryUsage && (broker.memoryPercent ?? 0) >= 90) {
      out.push({
        key: `memory-${broker.name}`,
        ruleKey: "memoryUsage",
        severity: (broker.memoryPercent ?? 0) >= 98 ? "crit" : "warn",
        params: { broker: broker.name, percent: broker.memoryPercent ?? 0 },
      });
    }
  }

  return out;
}
