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
  | "slowConsumer"
  /*
   * NSQ's own, and no other family here can raise it. A paused topic or
   * channel keeps accepting publishes and delivers nothing, so every figure
   * except the backlog stays healthy while it grows - and a pause is an
   * operator action rather than a fault, which is exactly why it is the kind
   * that gets left on. Neither of the backlog rules describes it: nothing is
   * wrong with the consumers, and there may not be any.
   */
  | "deliveryPaused"
  /*
   * Pub/Sub's two, and no other family here can raise either. Both describe
   * something only a fan-out service does: accept a message, report success,
   * and have nowhere to put it.
   *
   * A topic with no subscription discards every publish on arrival, and
   * nothing downstream records it - there is no backlog to notice it by and
   * the publisher is told the send succeeded. A subscription whose topic has
   * been deleted is the mirror image: it will never receive another message,
   * still holds what it had not delivered, and is still billed for it while
   * every other figure about it looks ordinary.
   *
   * Neither is queueNoConsumer, whose sentence reads "N waiting, no consumer
   * attached": there is no N here, and on the second there is no consumer the
   * alert is about.
   */
  | "topicUnsubscribed"
  | "subscriptionOrphaned"
  /*
   * Service Bus's two, and no other family here can raise either.
   *
   * A subscription with no rules receives nothing. It is not
   * subscriptionOrphaned - its topic is perfectly alive - and it is not
   * queueNoConsumer, whose sentence reads "N waiting, no consumer attached":
   * there is no N, because nothing can arrive to wait. The state is reached by
   * deleting the $Default rule and not replacing it, and every figure about
   * the subscription goes on looking healthy afterwards.
   *
   * An entity that is disabled, or has sends or receives switched off, is the
   * quietest fault this app can show. A send to a SendDisabled queue is
   * refused at the client and leaves no mark on any board; a ReceiveDisabled
   * one fills up while reporting nothing unusual. Only Service Bus has a
   * per-entity status this can read.
   */
  | "subscriptionUnroutable"
  | "entityDisabled"
  /*
   * Kinesis's two, and no other family here can raise either.
   *
   * Every operation that changes a stream is asynchronous, and while one is
   * running every other call that names the stream is refused - as "resource
   * in use", which describes nothing anybody did. A resize is a normal minute
   * in that state; a stream still in it an hour later is a stuck operation,
   * and no other page in this app says so.
   *
   * A registration is the same shape on a different object, and it is a
   * separate switch because it is a separate fault with a separate fix: a
   * consumer that never went ACTIVE is one an application is failing to
   * subscribe to, and an operator resizing a region wants the first alert off
   * without losing this one.
   *
   * Neither is brokerOffline, which needs a node, and neither is
   * topicUnsubscribed - which on this family would fire on nearly every
   * healthy stream, because the ordinary way to read one registers nothing.
   */
  | "streamNotActive"
  | "consumerNotActive";

export type AlertRulePrefs = Record<AlertRuleKey, boolean>;

/** Every rule, in the order a list of them reads best: worst first. */
export const ALERT_RULE_KEYS: readonly AlertRuleKey[] = [
  "brokerOffline",
  "streamNoLeader",
  "deliveryPaused",
  "topicUnsubscribed",
  "subscriptionOrphaned",
  "subscriptionUnroutable",
  "entityDisabled",
  "streamNotActive",
  "consumerNotActive",
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
  /*
   * No brokerOffline, no disk figure and no dead letters.
   *
   * A daemon that has stopped answering fails the whole read rather than
   * appearing as an offline row: the node list is the profile's own addresses
   * and every figure elsewhere is a sum over them, so a connection cannot
   * half-succeed and a switch for it would be a switch for something that
   * cannot fire. nsqd reports no disk figure of any kind - the overflow file
   * sits wherever --data-path points and the daemon never looks at it - and it
   * moves nothing aside when a consumer gives up, so there is no dead-letter
   * queue to watch.
   *
   * groupOffline is narrowed rather than excluded, unlike ActiveMQ's: it fires
   * only on a channel that has a backlog and nothing attached, because a
   * channel holding messages for a consumer that is not connected is what a
   * channel is for.
   */
  [MQKind.KindNSQ]: [
    "deliveryPaused",
    "groupOffline",
    "queueNoConsumer",
    "groupLag",
  ],
  /*
   * Two, and every other rule is absent because the service cannot answer it.
   *
   * There is no node, so brokerOffline cannot fire: a credential that stops
   * working fails the whole read rather than marking a row down. There is no
   * storage figure anywhere, so diskUsage has nothing to read - a queue is
   * billed by request rather than by what it holds. And SQS keeps no record of
   * who reads a queue, so every rule about a consumer - groupOffline,
   * groupLag, queueNoConsumer - would be asserting something the service
   * cannot support.
   *
   * What is left reads the queue listing, which is the whole of what SQS
   * reports. dlqGrowth is affordable here in a way it was not on Pulsar: a
   * dead-letter queue is one another queue's redrive policy points at, and
   * every source row in the same listing already carries that target.
   */
  [MQKind.KindSQS]: ["dlqGrowth", "queueBacklog"],
  /*
   * Two, and every other rule is absent because the service cannot answer it.
   *
   * There is no node, so brokerOffline cannot fire. There is no storage figure
   * anywhere, so diskUsage has nothing to read. Every rule about a backlog -
   * groupLag, queueBacklog, dlqGrowth - would need num_undelivered_messages,
   * which is a Cloud Monitoring metric under a different API. And Pub/Sub
   * keeps no record of who is pulling a subscription, so groupOffline and
   * queueNoConsumer would be asserting something the service cannot support.
   *
   * What is left costs nothing the sweep was not already paying for: the topic
   * listing already carries every topic's subscription count, and the
   * subscription listing already says whose topic has been deleted.
   */
  [MQKind.KindGooglePubSub]: ["topicUnsubscribed", "subscriptionOrphaned"],
  /*
   * Four, and the absences are the service rather than the driver.
   *
   * There is no node, so brokerOffline cannot fire, and no storage figure
   * anywhere, so diskUsage has nothing to read. Nothing registers as a
   * consumer - a queue is read by whoever opens a receiver on it - so
   * groupOffline and queueNoConsumer would assert something the service
   * cannot support.
   *
   * The backlog rules are absent for a different reason and it is worth
   * separating: Service Bus does report an active message count, and a real
   * namespace answers it. What it cannot do is tell a rule apart from an
   * endpoint that reports no counts at all, which the emulator is - so a
   * backlog alert would fire on the emulator rather than on a backlog. The
   * depths stay on the boards, which draw a dash for the difference.
   *
   * What is left costs nothing the sweep was not already paying for: the
   * entity listing carries every topic's subscription count, every entity's
   * status and its dead-letter count, and the subscription listing carries
   * the rules that decide whether anything can reach one.
   */
  [MQKind.KindAzureServiceBus]: [
    "topicUnsubscribed",
    "subscriptionUnroutable",
    "entityDisabled",
    "dlqGrowth",
  ],
  /*
   * Two, and the absences here are sharper than any other family's: two of
   * the rules a reader would expect would not merely stay quiet, they would
   * fire on healthy streams.
   *
   * topicUnsubscribed is the dangerous one. A registered consumer is the
   * enhanced fan-out kind, and the ordinary way to read a Kinesis stream -
   * the KCL, a Lambda event source, a plain GetRecords loop - registers
   * nothing at all. So "no consumers" is the normal state of a stream three
   * applications are reading, and the rule would report every one of them.
   * queueNoConsumer is the same mistake with a number attached, and there is
   * no number: nothing counts what a stream holds.
   *
   * The rest are absent because there is nothing to read. brokerOffline needs
   * a node and AWS shows none; groupLag and queueBacklog need a position, and
   * no Kinesis connection can report one; diskUsage needs a storage figure,
   * and the service publishes none; dlqGrowth needs a dead-letter store, and
   * nothing here is ever moved aside.
   *
   * What is left costs nothing the sweep was not already paying for: the
   * stream listing carries every status, and the consumer listing carries
   * every registration's.
   */
  [MQKind.KindKinesis]: ["streamNotActive", "consumerNotActive"],
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
  "deliveryPaused",
  "topicUnsubscribed",
  "entityDisabled",
  "streamNotActive",
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
  MQKind.KindNSQ,
  MQKind.KindSQS,
  MQKind.KindGooglePubSub,
  MQKind.KindAzureServiceBus,
  MQKind.KindKinesis,
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
  deliveryPaused: true,
  topicUnsubscribed: true,
  subscriptionOrphaned: true,
  subscriptionUnroutable: true,
  entityDisabled: true,
  streamNotActive: true,
  consumerNotActive: true,
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
