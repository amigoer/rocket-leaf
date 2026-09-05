import { MQKind } from "@bindings/model/models";

export type AlertRuleKey =
  | "brokerOffline"
  | "groupOffline"
  | "groupLag"
  | "diskUsage"
  | "dlqGrowth"
  | "resourceAlarm"
  | "nodePartition"
  | "memoryUsage"
  | "queueBacklog"
  | "queueNoConsumer"
  | "flowControl"
  // Kafka's three degrees of partition trouble. Separate keys because they are
  // separate switches: a cluster mid-reassignment is under-replicated on
  // purpose and its operator wants that one off, not all three.
  | "partitionUnderReplicated"
  | "partitionOffline"
  | "partitionLeaderless"
  /*
   * Pulsar's own, and it has no counterpart on any other family: past its
   * unacknowledged limit the broker stops delivering to a subscription
   * entirely. From the backlog alone that is indistinguishable from a slow
   * consumer, and it is fixed by acknowledging or raising a limit rather than
   * by looking at the consumer - so it is a separate switch and a separate
   * alert.
   */
  | "subscriptionBlocked"
  /*
   * NATS's three, none of which any other family here can raise.
   *
   * A JetStream stream is a Raft group rather than a set of partitions, so
   * Kafka's three partition rules say the wrong thing about it in the wrong
   * words - "3 partitions neither readable nor writable" describes nothing a
   * stream can be. Two keys rather than one for the reason Kafka has three: a
   * stream that has just been given another replica is behind on purpose while
   * it catches up, and the operator doing that wants that switch off without
   * losing the leaderless alarm.
   *
   * A slow consumer is NATS dropping a subscriber that fell behind, which is
   * neither a blocked publisher nor a growing queue - the two shapes the other
   * families take - because the messages are simply gone.
   */
  | "streamNoLeader"
  | "streamUnderReplicated"
  | "slowConsumer";

export type AlertRulePrefs = Record<AlertRuleKey, boolean>;

/** Every rule, in the order a list of them reads best: worst first. */
export const ALERT_RULE_KEYS: readonly AlertRuleKey[] = [
  "brokerOffline",
  "streamNoLeader",
  "subscriptionBlocked",
  "resourceAlarm",
  "nodePartition",
  "partitionLeaderless",
  "partitionOffline",
  "partitionUnderReplicated",
  "streamUnderReplicated",
  "groupOffline",
  "queueNoConsumer",
  "groupLag",
  "queueBacklog",
  "diskUsage",
  "memoryUsage",
  "flowControl",
  "slowConsumer",
  "dlqGrowth",
];

/*
 * Which rules a family can actually raise.
 *
 * The switches are stored for every rule regardless, because a window can hold
 * connections to two families at once and a toggle is a preference rather than
 * a fact about a broker. What this decides is which ones are worth showing:
 * offering "consumer group has no instances" against RabbitMQ would be
 * offering a switch for something that cannot happen.
 */
const RULES_BY_KIND: Partial<Record<MQKind, readonly AlertRuleKey[]>> = {
  [MQKind.KindKafka]: [
    "brokerOffline",
    "partitionLeaderless",
    "partitionOffline",
    "partitionUnderReplicated",
    "groupOffline",
    "groupLag",
  ],
  /*
   * No brokerOffline, no diskUsage and no dlqGrowth, and all three absences
   * are deliberate.
   * Pulsar's active-broker listing stops listing a broker that has gone rather
   * than reporting it offline, so the driver never marks one - a switch for it
   * would be a switch for something that cannot fire. The messages are
   * BookKeeper's, so no broker reports a disk figure at all; memory is the
   * watermark that actually stops one on this family.
   *
   * dlqGrowth is the one left out for cost rather than for meaning. A Pulsar
   * dead-letter topic is found by walking a namespace and reading every
   * topic's depth, which is the same request-per-topic walk the subscription
   * rules already pay for - offering it here would double the sweep against
   * every open connection, every minute, for a page that answers the same
   * question on demand.
   */
  [MQKind.KindPulsar]: [
    "subscriptionBlocked",
    "groupOffline",
    "groupLag",
    "memoryUsage",
  ],
  /*
   * Nothing new, and one deliberate absence. Everything this family can raise
   * is a question another family already asks - a queue nobody is draining, a
   * store filling up, a consumer the broker has marked slow.
   *
   * groupOffline is one of two left out. A subscription with nothing attached
   * is a fault on Kafka and Pulsar and is the normal resting state here: a
   * durable subscription exists precisely so the broker can hold messages for
   * a client that is not connected, so the rule would fire on every healthy
   * deployment.
   *
   * slowConsumer is the other, and for a subtler reason. Classic does flag a
   * subscriber falling behind - but the sentence behind that key reads "the
   * server has disconnected N clients since it started", which is NATS's
   * meaning. The flag is shown on the subscriptions board instead, worded for
   * what it is here.
   */
  [MQKind.KindActiveMQ]: [
    "queueNoConsumer",
    "queueBacklog",
    "diskUsage",
    "memoryUsage",
    "dlqGrowth",
  ],
  [MQKind.KindRabbitMQ]: [
    "brokerOffline",
    "resourceAlarm",
    "nodePartition",
    "queueNoConsumer",
    "queueBacklog",
    "diskUsage",
    "memoryUsage",
    "flowControl",
  ],
  /*
   * Redis has no partitions, no dead-letter topic and no disk figure, so the
   * rules it can raise are its own: memory against its own cap, a background
   * save that failed, and a consumer group holding work with nothing attached
   * to finish it.
   */
  /*
   * No brokerOffline, and that is the same absence Pulsar has: a NATS server
   * that has gone stops answering the fan-out rather than reporting itself
   * down, so no row is ever marked offline and a switch for it would be a
   * switch for something that cannot fire. No disk figure exists anywhere in
   * NATS either - JetStream reports an account's usage against its limit,
   * which is a different question and one the accounts page answers.
   */
  [MQKind.KindNATS]: [
    "streamNoLeader",
    "groupOffline",
    "streamUnderReplicated",
    "groupLag",
    "slowConsumer",
  ],
  [MQKind.KindRedisStream]: [
    "brokerOffline",
    "resourceAlarm",
    "memoryUsage",
    "groupOffline",
    "groupLag",
  ],
};

const ROCKETMQ_RULES: readonly AlertRuleKey[] = [
  "brokerOffline",
  "groupOffline",
  "groupLag",
  "diskUsage",
  "dlqGrowth",
];

export function rulesFor(kind: MQKind | undefined): readonly AlertRuleKey[] {
  if (kind == null) return ALERT_RULE_KEYS;
  return RULES_BY_KIND[kind] ?? ROCKETMQ_RULES;
}

const STORAGE_KEY = "mq-studio:alert-rules";

/**
 * Rules that read facts.destinations, and the families that must therefore
 * have them swept.
 *
 * Two lists rather than one boolean expression in the sweep, because nothing
 * tied them together and both went stale: ActiveMQ shipped with three
 * destination rules and no destinations gathered, which is an alerts page that
 * is armed and cannot fire. The test in alertRules.test.ts holds them in step.
 *
 * The sweep is per family and hand-written for a reason - a destination
 * listing costs a request on some families and a walk on others - so this
 * names which families pay it rather than making every family pay.
 */
export const DESTINATION_RULES: readonly AlertRuleKey[] = [
  "queueNoConsumer",
  "queueBacklog",
  "dlqGrowth",
  "partitionLeaderless",
  "partitionOffline",
  "partitionUnderReplicated",
  "streamNoLeader",
  "streamUnderReplicated",
];

/**
 * The families with a rule list of their own.
 *
 * A family not here runs RocketMQ's rules, which read RocketMQ's facts the
 * RocketMQ way - so the destination pairing below is only a question for the
 * families that brought their own.
 */
export const KINDS_WITH_OWN_RULES: readonly MQKind[] = Object.keys(
  RULES_BY_KIND,
) as MQKind[];

/** The families whose sweep fetches a destination listing. */
export const KINDS_NEEDING_DESTINATIONS: readonly MQKind[] = [
  MQKind.KindRabbitMQ,
  MQKind.KindKafka,
  MQKind.KindNATS,
  MQKind.KindActiveMQ,
];

export const DEFAULT_ALERT_RULES: AlertRulePrefs = {
  brokerOffline: true,
  groupOffline: true,
  groupLag: true,
  diskUsage: true,
  dlqGrowth: true,
  resourceAlarm: true,
  nodePartition: true,
  memoryUsage: true,
  queueBacklog: true,
  queueNoConsumer: true,
  flowControl: true,
  subscriptionBlocked: true,
  partitionUnderReplicated: true,
  partitionOffline: true,
  partitionLeaderless: true,
  streamNoLeader: true,
  streamUnderReplicated: true,
  slowConsumer: true,
};

function read(): AlertRulePrefs {
  if (typeof window === "undefined") return { ...DEFAULT_ALERT_RULES };
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return { ...DEFAULT_ALERT_RULES };
    const parsed = JSON.parse(raw) as Partial<AlertRulePrefs>;
    return { ...DEFAULT_ALERT_RULES, ...parsed };
  } catch {
    return { ...DEFAULT_ALERT_RULES };
  }
}

/*
 * One snapshot for the whole window.
 *
 * The alerts page and the notification centre both read these, and a toggle on
 * the page has to reach the centre in the same tick -- two hooks each holding
 * their own useState copy would leave the bell counting rows the page had just
 * switched off. The identity is stable until a write, which is what
 * useSyncExternalStore needs to avoid an infinite re-render.
 */
let current: AlertRulePrefs = read();
const listeners = new Set<() => void>();

export function getAlertRules(): AlertRulePrefs {
  return current;
}

export function subscribeAlertRules(listener: () => void): () => void {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

function same(left: AlertRulePrefs, right: AlertRulePrefs): boolean {
  return (Object.keys(DEFAULT_ALERT_RULES) as AlertRuleKey[]).every(
    (key) => left[key] === right[key],
  );
}

/**
 * Re-reads storage, keeping the cached identity when nothing changed.
 *
 * Something outside this module may have written the key -- another window, or
 * a restore -- so this is a real read; holding the identity when the content
 * matches is what keeps `getAlertRules` usable as a store snapshot.
 */
export function loadAlertRules(): AlertRulePrefs {
  const fresh = read();
  if (!same(fresh, current)) current = fresh;
  return current;
}

export function saveAlertRules(rules: AlertRulePrefs): void {
  current = rules;
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(rules));
  } catch {
    // ignore quota / private mode
  }
  for (const listener of listeners) listener();
}
