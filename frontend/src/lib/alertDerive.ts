/**
 * The alert rules, as a pure function of one connection's settled snapshot.
 *
 * Shared by the alerts page, which reads the single connection its tab is
 * scoped to, and by the notification centre, which fans out across every open
 * one. The rules cannot live in either: two copies drift the moment a
 * threshold moves.
 *
 * What is wrong with a broker is a question each family answers in its own
 * vocabulary, so the rules themselves live beside the readers that know it -
 * `@/mq/<family>/alerts`. This file is the shape they share and the dispatch
 * between them. It used to be one rule set reading RocketMQ attribute keys,
 * which meant a RabbitMQ connection was measured for a broker ordinal and a
 * commit log it does not have, and quietly produced no alerts at all.
 *
 * Nothing here is localised. A record outlives the language it fired in -- the
 * centre keeps resolved alerts to draw them as recovered -- so what is stored
 * is the rule and its numbers, and the words are chosen at render.
 */
import { MQKind } from "@bindings/model/models";
import type { ClientConnection } from "@bindings/model/models";
import type { Destination, Node, Subscription } from "@/api/models";
import type { AlertRuleKey, AlertRulePrefs } from "@/lib/alertRules";
import { deriveRocketMQAlerts } from "@/mq/rocketmq/alerts";
import { deriveRabbitMQAlerts } from "@/mq/rabbitmq/alerts";
import { deriveKafkaAlerts } from "@/mq/kafka/alerts";
import { deriveMqttAlerts } from "@/mq/mqtt/alerts";
import { derivePulsarAlerts } from "@/mq/pulsar/alerts";
import { deriveRedisAlerts } from "@/mq/redis/alerts";
import { deriveNatsAlerts } from "@/mq/nats/alerts";
import { deriveActiveMQAlerts } from "@/mq/activemq/alerts";
import { deriveNsqAlerts } from "@/mq/nsq/alerts";
import { deriveSqsAlerts } from "@/mq/sqs/alerts";
import { deriveGooglePubSubAlerts } from "@/mq/googlepubsub/alerts";
import { deriveAzureServiceBusAlerts } from "@/mq/azureservicebus/alerts";
import { deriveKinesisAlerts } from "@/mq/kinesis/alerts";

export type AlertSeverity = "crit" | "warn" | "info";

/** What the rules read. Every list comes from one settled poll of a connection. */
export interface AlertFacts {
  nodes: readonly Node[];
  consumerGroups: readonly Subscription[];
  /** Queues and topics. Empty for families whose rules do not read them. */
  destinations: readonly Destination[];
  /** Client connections, for the families that report flow control on one. */
  connections: readonly ClientConnection[];
}

export interface AlertThresholds {
  /** Backlog that counts as a lag alert. Zero disables the backlog rules. */
  lag: number;
  /** Disk percentage that counts as a water-level alert. Zero disables it. */
  disk: number;
}

export interface DerivedAlert {
  /** Stable across polls, so an alert that keeps firing keeps its record. */
  key: string;
  ruleKey: AlertRuleKey;
  severity: AlertSeverity;
  /** Interpolated into `alerts.rule.*` and `alerts.detail.*` at render. */
  params: Readonly<Record<string, string | number>>;
  /** What the broker last said about when this started, when it says anything. */
  since?: string;
}

function severityWeight(severity: AlertSeverity): number {
  return severity === "crit" ? 3 : severity === "warn" ? 2 : 1;
}

/** An empty snapshot, for a connection nothing has been read from yet. */
export const NO_FACTS: AlertFacts = {
  nodes: [],
  consumerGroups: [],
  destinations: [],
  connections: [],
};

/** Every rule the prefs leave enabled, worst first. */
export function deriveAlerts(
  kind: MQKind | undefined,
  facts: AlertFacts,
  rules: AlertRulePrefs,
  thresholds: AlertThresholds,
): DerivedAlert[] {
  const derived =
    kind === MQKind.KindRabbitMQ
      ? deriveRabbitMQAlerts(facts, rules, thresholds)
      : kind === MQKind.KindKafka
        ? deriveKafkaAlerts(facts, rules, thresholds)
        : kind === MQKind.KindMQTT
          ? deriveMqttAlerts(facts, rules, thresholds)
          : kind === MQKind.KindPulsar
            ? derivePulsarAlerts(facts, rules, thresholds)
            : kind === MQKind.KindRedisStream
              ? deriveRedisAlerts(facts, rules, thresholds)
              : kind === MQKind.KindNATS
                ? deriveNatsAlerts(facts, rules, thresholds)
                : kind === MQKind.KindActiveMQ
                  ? deriveActiveMQAlerts(facts, rules, thresholds)
                  : kind === MQKind.KindNSQ
                    ? deriveNsqAlerts(facts, rules, thresholds)
                    : kind === MQKind.KindSQS
                      ? deriveSqsAlerts(facts, rules, thresholds)
                      : kind === MQKind.KindGooglePubSub
                        ? deriveGooglePubSubAlerts(facts, rules, thresholds)
                        : kind === MQKind.KindAzureServiceBus
                          ? deriveAzureServiceBusAlerts(facts, rules, thresholds)
                          : kind === MQKind.KindKinesis
                            ? deriveKinesisAlerts(facts, rules, thresholds)
                            : /* Every other family is read with RocketMQ's rules, which is
                     what they were before this dispatch existed. A family whose
                     vocabulary they do not fit reports nothing rather than
                     something wrong, and gets its own rules when it gets its
                     own driver. */
                  deriveRocketMQAlerts(facts, rules, thresholds);

  return derived.sort(
    (left, right) => severityWeight(right.severity) - severityWeight(left.severity),
  );
}
