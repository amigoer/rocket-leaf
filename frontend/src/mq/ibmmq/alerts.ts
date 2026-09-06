/**
 * What is wrong with a queue manager right now.
 *
 * RocketMQ's rules, which every family without its own falls back to, read a
 * broker ordinal, a commit log's disk percentage, a consumer group's instance
 * count and a dead-letter topic named after a group. IBM MQ has none of those:
 * there is one queue manager rather than a cluster of brokers, the REST
 * interface publishes no storage figure at all, and a dead-letter queue is an
 * ordinary queue something else points at. A connection left on the defaults
 * would run five rules that all read zero and could never fire.
 *
 * Most of what is here is an existing key, because most of what this family
 * can raise is something another family already asks: a queue nobody is
 * draining, a backlog past a threshold, a dead-letter queue growing, an object
 * switched off. One rule is new, and it is the one no other family has a shape
 * for - see transmissionBacklog below.
 *
 * Three rules a reader might expect are absent.
 *
 * topicUnsubscribed would fire on nearly every healthy queue manager. A topic
 * object is where a topic string's settings live, not a thing publishers name:
 * they publish to strings, which may have no object at all, so an object with
 * no subscription is the ordinary state rather than a fault.
 *
 * diskUsage and memoryUsage need a figure the REST interface does not report.
 * The queue manager's log and its storage are readable from the machine it
 * runs on, and nothing this connection can call returns a percentage.
 *
 * groupLag is absent because it would double-count. A subscription's backlog
 * is the depth of the queue it delivers to, and that queue is on this page
 * already through the queue rules.
 */
import type { AlertFacts, AlertThresholds, DerivedAlert } from "@/lib/alertDerive";
import type { AlertRulePrefs } from "@/lib/alertRules";
import { destination, inhibited } from "./destinations";

export function deriveIbmMqAlerts(
  facts: AlertFacts,
  rules: AlertRulePrefs,
  thresholds: AlertThresholds,
): DerivedAlert[] {
  const out: DerivedAlert[] = [];
  const { lag: lagThreshold } = thresholds;

  for (const row of facts.destinations) {
    const entry = destination(row);
    // A topic stores nothing, so every depth rule below would read a dash.
    if (entry.kind !== "queue") continue;

    const depth = entry.depth ?? 0;

    /*
     * An inhibited queue, which is the commonest reason messages end up on the
     * dead-letter queue and leaves no other mark anywhere.
     *
     * It is reported whatever the depth: a put-inhibited queue is empty
     * precisely because nothing can reach it, and waiting for a backlog would
     * be waiting for the one symptom this fault cannot produce.
     */
    if (rules.entityDisabled && inhibited(entry)) {
      out.push({
        key: `inhibited-${entry.name}`,
        ruleKey: "entityDisabled",
        severity: "warn",
        // entity and status are what alerts.detail.entityDisabled interpolates.
        params: {
          entity: entry.name,
          status: entry.inhibitPut
            ? entry.inhibitGet
              ? "put and get inhibited"
              : "put inhibited"
            : "get inhibited",
        },
      });
    }

    // The queue manager's own dead-letter queue is measured by dlqGrowth and
    // must not also fire the two ordinary queue rules: a dead-letter queue has
    // a backlog and no consumer, which is what it is for.
    if (entry.deadLetterQueue) {
      if (rules.dlqGrowth && depth > 0) {
        out.push({
          key: `dlq-${entry.name}`,
          ruleKey: "dlqGrowth",
          severity: "warn",
          // group and count are what alerts.detail.dlqGrowth interpolates. The
          // "group" is the queue, because the queue manager gave up on the
          // message rather than a consumer group - this family has none.
          params: { group: entry.name, count: depth },
        });
      }
      continue;
    }

    /*
     * A transmission queue with anything on it, which is the rule no other
     * family has a shape for.
     *
     * A transmission queue exists to be drained by one channel. Anything
     * sitting on it is a message that has not left this queue manager - the
     * channel is stopped, retrying, or was never started - and the queue's own
     * consumer count says nothing, because the channel is not an application
     * holding it open in a way this page can see. It is separate from the
     * backlog rule because it needs no threshold: one message on a
     * transmission queue for any length of time is already the fault.
     */
    if (rules.transmissionBacklog && entry.transmissionQueue && depth > 0) {
      out.push({
        key: `transmission-${entry.name}`,
        ruleKey: "transmissionBacklog",
        severity: "warn",
        params: { queue: entry.name, count: depth },
      });
      continue;
    }

    if (rules.queueNoConsumer && depth > 0 && (entry.openInput ?? 0) === 0) {
      out.push({
        key: `queue-no-consumer-${entry.name}`,
        ruleKey: "queueNoConsumer",
        severity: "warn",
        params: { queue: entry.name, lag: depth },
      });
    } else if (rules.queueBacklog && lagThreshold > 0 && depth >= lagThreshold) {
      // Else rather than as well: a queue nobody is draining is already the
      // stronger statement about the same queue, and firing both would put the
      // same problem on the page twice.
      out.push({
        key: `queue-backlog-${entry.name}`,
        ruleKey: "queueBacklog",
        severity: "warn",
        params: { queue: entry.name, lag: depth, threshold: lagThreshold },
      });
    }
  }

  return out;
}
