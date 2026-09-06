/**
 * What is wrong with an SQS region right now.
 *
 * RocketMQ's rules, which every family without its own falls back to, read a
 * broker ordinal, a commit log's disk percentage, a consumer group's instance
 * count and a dead-letter topic named after a group. SQS has none of those:
 * AWS runs the service so there is no node and no disk figure, and there are
 * no consumer groups at all. A connection left on the defaults would run five
 * rules that all read zero and could never fire - an alerts page that looks
 * armed and is not.
 *
 * Two rules, and both read the queue listing, because that listing is the
 * whole of what SQS reports. Everything else is CloudWatch, which is a
 * different API under a different permission.
 *
 * Three rules a reader might expect are absent, and each for a reason about
 * the service:
 *
 *   - brokerOffline cannot fire. There is no node: a credential that stops
 *     working fails the whole read rather than marking a row down.
 *   - groupOffline and queueNoConsumer cannot fire either. SQS keeps no
 *     record of who reads a queue - a consumer is whoever calls
 *     ReceiveMessage - so "nothing is consuming this" is a claim the service
 *     cannot support, and a rule that assumed it would fire on every healthy
 *     queue between two polls.
 *   - diskUsage has nothing to read. AWS reports no storage figure at all;
 *     a queue is billed by request rather than by what it holds.
 */
import type { AlertFacts, AlertThresholds, DerivedAlert } from "@/lib/alertDerive";
import type { AlertRulePrefs } from "@/lib/alertRules";
import { queue } from "./destinations";

export function deriveSqsAlerts(
  facts: AlertFacts,
  rules: AlertRulePrefs,
  thresholds: AlertThresholds,
): DerivedAlert[] {
  const out: DerivedAlert[] = [];
  const { lag: lagThreshold } = thresholds;

  const queues = facts.destinations.map(queue);

  /*
   * Which queues are dead-letter queues, taken from the listing rather than
   * from a second walk.
   *
   * Nothing marks one in SQS: it is an ordinary queue that another queue's
   * redrive policy points at, and every source row already carries that
   * target. So the set costs no request the sweep was not already making,
   * which is what keeps this rule affordable where Pulsar's had to be left
   * out.
   */
  const deadLetterTargets = new Set(
    queues.flatMap((entry) => (entry.deadLetterQueue != null ? [entry.deadLetterQueue] : [])),
  );

  for (const entry of queues) {
    /*
     * A dead-letter queue with anything in it.
     *
     * Worth a rule of its own rather than folding into the backlog: these are
     * messages a consumer gave up on, so any depth at all is a fault where a
     * working queue's backlog is only a fault past a threshold. There is
     * nothing draining it either - a redrive is an operator's decision - so
     * it does not clear on its own.
     */
    if (rules.dlqGrowth && deadLetterTargets.has(entry.name) && (entry.visible ?? 0) > 0) {
      out.push({
        key: `dlq-${entry.name}`,
        ruleKey: "dlqGrowth",
        severity: "warn",
        // group and count are what alerts.detail.dlqGrowth interpolates. The
        // "group" is the queue, because that is what the messages were given
        // up on into - this family has no consumer groups to name.
        params: { group: entry.name, count: entry.visible ?? 0 },
      });
      continue;
    }

    /*
     * A backlog past the threshold, counted on what is available rather than
     * on everything the queue holds.
     *
     * In-flight messages are with a consumer and delayed ones are not due, so
     * counting either would fire on a queue that is working exactly as it was
     * asked to.
     */
    if (rules.queueBacklog && lagThreshold > 0 && (entry.visible ?? 0) >= lagThreshold) {
      out.push({
        key: `queue-backlog-${entry.name}`,
        ruleKey: "queueBacklog",
        severity: "warn",
        params: { queue: entry.name, lag: entry.visible ?? 0, threshold: lagThreshold },
      });
    }
  }

  return out;
}
