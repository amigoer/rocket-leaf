/**
 * What is wrong with a Message VPN right now.
 *
 * RocketMQ's rules, which every family without its own falls back to, read a
 * broker ordinal, a commit log's disk percentage, a consumer group's instance
 * count and a dead-letter topic named after a group. Solace has none of those:
 * there is one broker rather than a cluster of them, and there is no consumer
 * group anywhere in the product. A connection left on the defaults would run
 * five rules that all read zero and could never fire.
 *
 * Most of what is here is a key another family already asks, which is the
 * honest answer for a family whose faults are ordinary: a queue nobody is
 * draining, a backlog past a threshold, a dead message queue growing, an
 * endpoint switched off, a spool filling up.
 *
 * One rule is this family's own and no other family has a shape for it - see
 * deadMsgQueueMissing below.
 *
 * Three rules a reader might expect are absent. brokerOffline needs a cluster
 * and there is one broker. groupOffline and groupLag need consumer groups,
 * which this product does not have: what reads a queue is a client bound to
 * it, and that is on the clients page. memoryUsage needs a figure SEMP does
 * not report for the broker at all.
 */
import type { AlertFacts, AlertThresholds, DerivedAlert } from "@/lib/alertDerive";
import type { AlertRulePrefs } from "@/lib/alertRules";
import { destination, halted } from "./destinations";

/** Past this the broker is close enough to full to be a critical rather than a warning. */
const CRITICAL_SPOOL = 95;

export function deriveSolaceAlerts(
  facts: AlertFacts,
  rules: AlertRulePrefs,
  thresholds: AlertThresholds,
): DerivedAlert[] {
  const out: DerivedAlert[] = [];
  const { lag: lagThreshold, disk: diskThreshold } = thresholds;

  const queues = facts.destinations.map(destination);
  const byName = new Set(queues.map((entry) => entry.name));
  // Which queues something else dead-letters into, built from the same listing
  // rather than a second call: every queue carries the pointer.
  const deadMsgQueues = new Set(
    queues.flatMap((entry) =>
      entry.deadMsgQueue != null && entry.deadMsgQueue !== "" ? [entry.deadMsgQueue] : [],
    ),
  );

  for (const entry of queues) {
    const depth = entry.depth ?? 0;

    /*
     * A queue that will give up on a message and has nowhere to put it.
     *
     * This is the rule no other family has, and it is the one worth having.
     * Every Solace endpoint ships pointing at "#DEAD_MSG_QUEUE" and no broker
     * creates a queue by that name, so a queue with a redelivery limit or a
     * TTL is configured to dead-letter and actually discards. Nothing else
     * anywhere reports it: the queue is healthy, the pointer is set, and the
     * messages are simply gone.
     *
     * Both conditions are needed. A queue that never gives up - no redelivery
     * limit and no TTL - never follows the pointer, so a missing target there
     * is a setting that has not been reached rather than a fault, and firing
     * on it would put every unconfigured queue on the page.
     */
    if (
      rules.deadMsgQueueMissing &&
      entry.deadMsgQueue != null &&
      entry.deadMsgQueue !== "" &&
      !byName.has(entry.deadMsgQueue) &&
      willGiveUp(entry.maxRedeliveryCount, entry.respectTtlEnabled, entry.maxTtlSec)
    ) {
      out.push({
        key: `dmq-missing-${entry.name}`,
        ruleKey: "deadMsgQueueMissing",
        severity: "warn",
        // queue and target are what alerts.detail.deadMsgQueueMissing
        // interpolates.
        params: { queue: entry.name, target: entry.deadMsgQueue },
      });
    }

    /*
     * An endpoint with ingress or egress switched off.
     *
     * Reported whatever the depth, for the reason IBM MQ reports an inhibited
     * queue: a queue that takes nothing is empty precisely because nothing can
     * reach it, and waiting for a backlog would be waiting for the one symptom
     * this fault cannot produce.
     */
    if (rules.entityDisabled && halted(entry)) {
      out.push({
        key: `halted-${entry.name}`,
        ruleKey: "entityDisabled",
        severity: "warn",
        params: {
          entity: entry.name,
          status: !entry.ingressEnabled
            ? entry.egressEnabled
              ? "ingress disabled"
              : "ingress and egress disabled"
            : "egress disabled",
        },
      });
    }

    // A dead message queue is measured by dlqGrowth and must not also fire the
    // two ordinary queue rules: having a backlog and no consumer is what it is
    // for.
    if (deadMsgQueues.has(entry.name)) {
      if (rules.dlqGrowth && depth > 0) {
        out.push({
          key: `dlq-${entry.name}`,
          ruleKey: "dlqGrowth",
          severity: "warn",
          // group and count are what alerts.detail.dlqGrowth interpolates. The
          // "group" is the queue, because the broker gave up on the message
          // rather than a consumer group - this family has none.
          params: { group: entry.name, count: depth },
        });
      }
      continue;
    }

    if (rules.queueNoConsumer && depth > 0 && (entry.boundConsumers ?? 0) === 0) {
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

  /*
   * The message spool, which is the one broker-wide figure worth a rule.
   *
   * It is the Message VPN's share rather than the disk, and past its quota the
   * broker stops accepting guaranteed messages for this VPN while every other
   * figure stays healthy. The driver has already scaled it: the two raw
   * numbers are in different units.
   */
  for (const node of facts.nodes) {
    const usage = node.diskUsage;
    if (rules.diskUsage && diskThreshold > 0 && usage >= diskThreshold) {
      out.push({
        key: `spool-${node.name}`,
        ruleKey: "diskUsage",
        severity: usage >= CRITICAL_SPOOL ? "crit" : "warn",
        // usage and threshold are what alerts.detail.diskUsage interpolates.
        params: { broker: node.name, usage, threshold: diskThreshold },
      });
    }
  }

  return out;
}

/**
 * Whether this queue will ever give up on a message.
 *
 * Two ways it can: a redelivery limit it exceeds, or a time to live it respects
 * and outlives. With neither, the pointer at a dead message queue is never
 * followed and its target's absence is not yet a fault.
 */
function willGiveUp(
  maxRedeliveryCount: number | null,
  respectTtl: boolean,
  maxTtlSec: number | null,
): boolean {
  if ((maxRedeliveryCount ?? 0) > 0) return true;
  return respectTtl && (maxTtlSec ?? 0) > 0;
}
