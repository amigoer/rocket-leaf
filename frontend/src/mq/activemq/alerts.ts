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
 * dead-letter queue growing.
 *
 * Two rules are deliberately absent that a reader might expect.
 *
 * groupOffline - a subscription with nothing attached - is a fault on Kafka
 * and Pulsar and is the normal resting state here: a durable subscription
 * exists precisely so the broker can hold messages for a client that is not
 * connected, so the rule would fire on every healthy deployment.
 *
 * slowConsumer is absent for a subtler reason. Classic does flag a subscriber
 * that is falling behind, and the rule key exists - but the sentence behind it
 * reads "the server has disconnected N clients since it started", which is
 * NATS's meaning and not this one. Raising it would print something untrue.
 * The flag is on the subscriptions board instead, where it can be worded for
 * what it is.
 */
import type { AlertFacts, AlertThresholds, DerivedAlert } from "@/lib/alertDerive";
import type { AlertRulePrefs } from "@/lib/alertRules";
import { destination } from "./destinations";
import { node } from "./cluster";

/**
 * Where a broker starts blocking producers rather than slowing them.
 *
 * A constant rather than a setting: the disk threshold is the operator's
 * judgement about headroom, and this is the point at which the broker itself
 * stops accepting - which is not a preference.
 */
const MEMORY_WATERMARK = 90;

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
          // group and count are what alerts.detail.dlqGrowth interpolates.
          // The "group" here is the destination, because that is what gave up
          // on the message rather than a consumer group - this family has none.
          params: { group: entry.name, count: entry.depth ?? 0 },
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
        params: { queue: entry.name, lag: depth },
      });
    } else if (rules.queueBacklog && lagThreshold > 0 && depth >= lagThreshold) {
      // Else rather than as well: a queue nobody is draining is already the
      // stronger statement about the same queue, and firing both would put
      // the same problem on the page twice.
      out.push({
        key: `queue-backlog-${entry.name}`,
        ruleKey: "queueBacklog",
        severity: "warn",
        params: { queue: entry.name, lag: depth, threshold: lagThreshold },
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
        // usage and threshold are what alerts.detail.diskUsage interpolates.
        params: {
          broker: broker.name,
          usage: broker.diskUsage ?? 0,
          threshold: diskThreshold,
        },
      });
    }
    // Memory is the watermark that actually stops a broker on this family:
    // past it, producers are blocked rather than slowed.
    if (rules.memoryUsage && (broker.memoryPercent ?? 0) >= MEMORY_WATERMARK) {
      out.push({
        key: `memory-${broker.name}`,
        ruleKey: "memoryUsage",
        severity: (broker.memoryPercent ?? 0) >= 98 ? "crit" : "warn",
        params: {
          broker: broker.name,
          usage: broker.memoryPercent ?? 0,
          // The memory rule has a fixed watermark rather than a setting: past
          // it a broker blocks producers, which is not a preference.
          threshold: MEMORY_WATERMARK,
        },
      });
    }
  }

  return out;
}
