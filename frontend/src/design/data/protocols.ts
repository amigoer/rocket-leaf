/**
 * Protocol registry for the design shell. The nav shape per protocol is taken
 * from the canvas sidebars (11a RocketMQ, 13a Kafka, 11b RabbitMQ, 11c Pulsar,
 * 11d Redis, 11e MQTT) — the same page slot is labelled with each protocol's
 * own noun, which is what makes the shell readable across six brokers.
 *
 * The canvas drew the icons as Unicode symbols, whose design sizes are
 * unrelated: ⌂ inked 5.5px wide where ⇄ inked 11px, and off darwin each fell
 * back to whatever the system had. lucide draws them on one grid instead. A
 * page slot carries the same icon across protocols even where the label does
 * not (topics is Topic / 队列 / Stream), because the slot is what the shell
 * navigates -- the exceptions are RabbitMQ's exchanges and MQTT's subscribe
 * and clients, which have no counterpart elsewhere.
 */
import {
  BellRing,
  Boxes,
  Cable,
  FileJson,
  Gauge,
  House,
  Layers,
  Mail,
  Plug,
  Radio,
  ScrollText,
  Send,
  Server,
  Shield,
  Split,
  TriangleAlert,
  Users,
  Waypoints,
  type LucideIcon,
} from "lucide-react";

export type ProtocolId =
  | "rocketmq"
  | "kafka"
  | "rabbitmq"
  | "pulsar"
  | "redis"
  | "mqtt"
  | "nats"
  | "activemq"
  | "nsq"
  | "sqs"
  | "google-pubsub"
  | "azure-servicebus"
  | "kinesis"
  | "ibmmq"
  | "solace";

export type PageId =
  | "overview"
  | "topics"
  /* Only Kinesis has them, and the sidebar is where that shows. A shard is
     not a partition number: it has an id, a hash key range, a read quota of
     its own and - because shards are split and merged rather than resized - a
     parent and an end. None of that fits a count on the streams page. */
  | "shards"
  /* Only IBM MQ has them, and the sidebar is where that shows. A channel is
     not a client connection and not a queue: it is a named, configured object
     that says how a client or another queue manager may reach this one, and it
     has a type, an address, a status and a running instance count of its own.
     Nothing on the queues page could carry that. */
  | "channels"
  | "exchanges"
  | "vhosts"
  | "policies"
  | "definitions"
  | "replication"
  | "quotas"
  | "consumers"
  | "messages"
  | "dlq"
  | "producer"
  | "subscribe"
  | "clients"
  | "cluster"
  | "alerts"
  | "acl";

/**
 * `label` is a translation key, not text: the sidebar and the palette resolve
 * it at render so a language change relabels the nav without rebuilding it.
 */
export type NavEntry = { id: PageId; icon: LucideIcon; label: string };
export type NavGroup = { label?: string; items: NavEntry[] };

export type Protocol = {
  id: ProtocolId;
  /** Display name used in connection rows and page subtitles. */
  name: string;
  /** Monospace badge from the 3h capability matrix. */
  badge: string;
  /** Badge palette class from tokens.css (`pb pRMQ` …). */
  badgeClass: string;
  nav: NavGroup[];
};

const BROWSE = "shell.nav.browse";
const OPS = "shell.nav.ops";

export const PROTOCOLS: Record<ProtocolId, Protocol> = {
  rocketmq: {
    id: "rocketmq",
    name: "RocketMQ",
    badge: "RMQ 4/5",
    badgeClass: "pRMQ",
    nav: [
      { items: [{ id: "overview", icon: House, label: "shell.nav.rocketmq.overview" }] },
      {
        label: BROWSE,
        items: [
          { id: "topics", icon: Layers, label: "shell.nav.rocketmq.topics" },
          { id: "consumers", icon: Users, label: "shell.nav.rocketmq.consumers" },
          { id: "messages", icon: Mail, label: "shell.nav.rocketmq.messages" },
          { id: "dlq", icon: TriangleAlert, label: "shell.nav.rocketmq.dlq" },
          { id: "producer", icon: Send, label: "shell.nav.rocketmq.producer" },
        ],
      },
      {
        label: OPS,
        items: [
          { id: "cluster", icon: Server, label: "shell.nav.rocketmq.cluster" },
          { id: "alerts", icon: BellRing, label: "shell.nav.rocketmq.alerts" },
          { id: "acl", icon: Shield, label: "shell.nav.rocketmq.acl" },
        ],
      },
    ],
  },
  kafka: {
    id: "kafka",
    name: "Kafka",
    badge: "KAFKA",
    badgeClass: "pKFK",
    nav: [
      { items: [{ id: "overview", icon: House, label: "shell.nav.kafka.overview" }] },
      {
        label: BROWSE,
        items: [
          { id: "topics", icon: Layers, label: "shell.nav.kafka.topics" },
          { id: "consumers", icon: Users, label: "shell.nav.kafka.consumers" },
          { id: "messages", icon: Mail, label: "shell.nav.kafka.messages" },
          { id: "producer", icon: Send, label: "shell.nav.kafka.producer" },
        ],
      },
      {
        label: OPS,
        items: [
          { id: "cluster", icon: Server, label: "shell.nav.kafka.cluster" },
          { id: "acl", icon: Shield, label: "shell.nav.kafka.acl" },
          { id: "quotas", icon: Gauge, label: "shell.nav.kafka.quotas" },
          { id: "alerts", icon: BellRing, label: "shell.nav.kafka.alerts" },
        ],
      },
    ],
  },
  rabbitmq: {
    id: "rabbitmq",
    name: "RabbitMQ",
    badge: "RABBIT",
    badgeClass: "pAMQ",
    nav: [
      { items: [{ id: "overview", icon: House, label: "shell.nav.rabbitmq.overview" }] },
      {
        label: BROWSE,
        items: [
          { id: "topics", icon: Layers, label: "shell.nav.rabbitmq.topics" },
          { id: "exchanges", icon: Waypoints, label: "shell.nav.rabbitmq.exchanges" },
          { id: "consumers", icon: Users, label: "shell.nav.rabbitmq.consumers" },
          { id: "messages", icon: Mail, label: "shell.nav.rabbitmq.messages" },
          { id: "dlq", icon: TriangleAlert, label: "shell.nav.rabbitmq.dlq" },
          { id: "producer", icon: Send, label: "shell.nav.rabbitmq.producer" },
        ],
      },
      {
        label: OPS,
        items: [
          { id: "cluster", icon: Server, label: "shell.nav.rabbitmq.cluster" },
          { id: "vhosts", icon: Boxes, label: "shell.nav.rabbitmq.vhosts" },
          { id: "policies", icon: ScrollText, label: "shell.nav.rabbitmq.policies" },
          { id: "replication", icon: Cable, label: "shell.nav.rabbitmq.replication" },
          { id: "definitions", icon: FileJson, label: "shell.nav.rabbitmq.definitions" },
          { id: "alerts", icon: BellRing, label: "shell.nav.rabbitmq.alerts" },
          { id: "acl", icon: Shield, label: "shell.nav.rabbitmq.acl" },
        ],
      },
    ],
  },
  pulsar: {
    id: "pulsar",
    name: "Pulsar",
    badge: "PULSAR",
    badgeClass: "pPLS",
    nav: [
      { items: [{ id: "overview", icon: House, label: "shell.nav.pulsar.overview" }] },
      {
        label: BROWSE,
        items: [
          /* Not an optional organising page the way a RabbitMQ vhost list is:
             a Pulsar topic is addressed as tenant/namespace/name, so this is
             where the topics page's scope comes from. */
          { id: "vhosts", icon: Boxes, label: "shell.nav.pulsar.vhosts" },
          { id: "topics", icon: Layers, label: "shell.nav.pulsar.topics" },
          { id: "consumers", icon: Users, label: "shell.nav.pulsar.consumers" },
          { id: "messages", icon: Mail, label: "shell.nav.pulsar.messages" },
          { id: "dlq", icon: TriangleAlert, label: "shell.nav.pulsar.dlq" },
          { id: "producer", icon: Send, label: "shell.nav.pulsar.producer" },
        ],
      },
      {
        label: OPS,
        items: [
          { id: "cluster", icon: Server, label: "shell.nav.pulsar.cluster" },
          { id: "alerts", icon: BellRing, label: "shell.nav.pulsar.alerts" },
          { id: "acl", icon: Shield, label: "shell.nav.pulsar.acl" },
        ],
      },
    ],
  },
  redis: {
    id: "redis",
    name: "Redis Stream",
    badge: "REDIS",
    badgeClass: "pRDS",
    nav: [
      { items: [{ id: "overview", icon: House, label: "shell.nav.redis.overview" }] },
      {
        label: BROWSE,
        items: [
          { id: "topics", icon: Layers, label: "shell.nav.redis.topics" },
          { id: "consumers", icon: Users, label: "shell.nav.redis.consumers" },
          { id: "messages", icon: Mail, label: "shell.nav.redis.messages" },
          { id: "dlq", icon: TriangleAlert, label: "shell.nav.redis.dlq" },
          { id: "producer", icon: Send, label: "shell.nav.redis.producer" },
        ],
      },
      {
        label: OPS,
        items: [
          { id: "cluster", icon: Server, label: "shell.nav.redis.cluster" },
          { id: "clients", icon: Plug, label: "shell.nav.redis.clients" },
          { id: "alerts", icon: BellRing, label: "shell.nav.redis.alerts" },
          { id: "acl", icon: Shield, label: "shell.nav.redis.acl" },
        ],
      },
    ],
  },
  mqtt: {
    id: "mqtt",
    name: "MQTT",
    badge: "MQTT",
    badgeClass: "pMQT",
    nav: [
      { items: [{ id: "overview", icon: House, label: "shell.nav.mqtt.overview" }] },
      {
        label: BROWSE,
        items: [
          { id: "topics", icon: Layers, label: "shell.nav.mqtt.topics" },
          { id: "subscribe", icon: Radio, label: "shell.nav.mqtt.subscribe" },
          { id: "producer", icon: Send, label: "shell.nav.mqtt.producer" },
          { id: "clients", icon: Plug, label: "shell.nav.mqtt.clients" },
        ],
      },
      {
        label: OPS,
        items: [
          { id: "cluster", icon: Server, label: "shell.nav.mqtt.cluster" },
          { id: "alerts", icon: BellRing, label: "shell.nav.mqtt.alerts" },
        ],
      },
    ],
  },
  nats: {
    id: "nats",
    name: "NATS",
    badge: "NATS 2.x",
    badgeClass: "pNAT",
    /* One entry for now, and the rest arrive with the capabilities that make
       them answerable. navigation.nats.test.ts pins the rule: an entry the
       declared capabilities cannot reach is drawn and then fails when it is
       opened, which reads as a broken app rather than as a broker that cannot
       answer. Overview needs none, because it renders whatever the connection
       does report. */
    nav: [
      { items: [{ id: "overview", icon: House, label: "shell.nav.nats.overview" }] },
      {
        label: BROWSE,
        items: [
          { id: "topics", icon: Layers, label: "shell.nav.nats.topics" },
          { id: "consumers", icon: Users, label: "shell.nav.nats.consumers" },
          { id: "messages", icon: Mail, label: "shell.nav.nats.messages" },
          { id: "producer", icon: Send, label: "shell.nav.nats.producer" },
          /* Core NATS delivers to whoever is listening and keeps nothing,
             so this is not the messages page with a filter on it: there is
             no history to page back through and nothing to re-read. */
          { id: "subscribe", icon: Radio, label: "shell.nav.nats.subscribe" },
        ],
      },
      {
        label: OPS,
        items: [
          { id: "cluster", icon: Server, label: "shell.nav.nats.cluster" },
          { id: "clients", icon: Plug, label: "shell.nav.nats.clients" },
          /* Accounts, which is NATS's isolation boundary and its only one:
             not a label on a subject but a wall between two of them. */
          { id: "vhosts", icon: Boxes, label: "shell.nav.nats.vhosts" },
          { id: "alerts", icon: BellRing, label: "shell.nav.nats.alerts" },
        ],
      },
    ],
  },
  activemq: {
    id: "activemq",
    name: "ActiveMQ",
    badge: "AMQ",
    badgeClass: "pAMQ",
    /* One MQKind, two products, one nav. Classic and Artemis disagree about
       almost everything underneath - Artemis routes through an address and
       stores in a queue, Classic addresses a destination directly - but a user
       reading the sidebar is not choosing a product, so the difference is the
       driver's to absorb.

       Queues and topics share the topics slot rather than splitting into two.
       Their management surface is identical - same attributes, same
       operations, both browsable - and what separates them is delivery, which
       the subscriptions page already shows: a queue has no named subscribers
       and a topic does. RabbitMQ splits queues from exchanges because an
       exchange holds no messages at all; a JMS topic does. */
    nav: [
      { items: [{ id: "overview", icon: House, label: "shell.nav.activemq.overview" }] },
      {
        label: BROWSE,
        items: [
          { id: "topics", icon: Layers, label: "shell.nav.activemq.topics" },
          { id: "consumers", icon: Users, label: "shell.nav.activemq.consumers" },
          { id: "messages", icon: Mail, label: "shell.nav.activemq.messages" },
          /* The first family since RabbitMQ with a real broker-side dead
             letter queue and a retry that puts a message back where it came
             from, rather than a convention a client library follows. */
          { id: "dlq", icon: TriangleAlert, label: "shell.nav.activemq.dlq" },
          { id: "producer", icon: Send, label: "shell.nav.activemq.producer" },
          /* Not the messages page with a filter on it. That one browses what
             the broker is holding and takes nothing; this attaches a
             subscriber, and what arrives is a copy of what is published while
             it listens. Topics only - a JMS consumer consumes, so attaching
             one to a queue would take its messages. */
          { id: "subscribe", icon: Radio, label: "shell.nav.activemq.subscribe" },
        ],
      },
      {
        label: OPS,
        items: [
          { id: "cluster", icon: Server, label: "shell.nav.activemq.cluster" },
          { id: "clients", icon: Plug, label: "shell.nav.activemq.clients" },
          { id: "alerts", icon: BellRing, label: "shell.nav.activemq.alerts" },
        ],
      },
    ],
  },
  nsq: {
    id: "nsq",
    name: "NSQ",
    badge: "NSQ",
    badgeClass: "pNSQ",
    /* Seven entries, and the four that are missing are the point. There is no
       messages page, no dead letters and no access page, because nsqd keeps no
       message it has already handed out, moves nothing aside when a consumer
       gives up, and authenticates nobody over HTTP.

       Topics and channels take the topics and consumers slots. A channel is
       the durable consumer-group equivalent - every channel under a topic gets
       a copy of every message, and its depth is the backlog - so it belongs in
       the slot every other family puts its groups in rather than in one of its
       own. */
    nav: [
      { items: [{ id: "overview", icon: House, label: "shell.nav.nsq.overview" }] },
      {
        label: BROWSE,
        items: [
          { id: "topics", icon: Layers, label: "shell.nav.nsq.topics" },
          { id: "consumers", icon: Users, label: "shell.nav.nsq.consumers" },
          { id: "producer", icon: Send, label: "shell.nav.nsq.producer" },
        ],
      },
      {
        label: OPS,
        items: [
          { id: "cluster", icon: Server, label: "shell.nav.nsq.cluster" },
          { id: "clients", icon: Plug, label: "shell.nav.nsq.clients" },
          { id: "alerts", icon: BellRing, label: "shell.nav.nsq.alerts" },
        ],
      },
    ],
  },
  sqs: {
    id: "sqs",
    name: "Amazon SQS",
    badge: "SQS",
    badgeClass: "pSQS",
    /* Five entries, and the six that are missing are the point. There is no
       consumers page, because SQS has no subscription of any kind: a consumer
       is whoever calls ReceiveMessage, and the service keeps no record of who
       that was. There is no cluster page and no clients page, because AWS runs
       the service and shows no node or session. There is no access page,
       because who may call what is IAM's, one service further out.

       Queues take the topics slot, which is every family's destination list.
       There is no second object here to put anywhere else. */
    nav: [
      { items: [{ id: "overview", icon: House, label: "shell.nav.sqs.overview" }] },
      {
        label: BROWSE,
        items: [
          { id: "topics", icon: Layers, label: "shell.nav.sqs.topics" },
          { id: "messages", icon: Mail, label: "shell.nav.sqs.messages" },
          /* An ordinary queue another queue's redrive policy points at, found
             by walking the topology rather than named after a group - because
             there are no groups to name one after. */
          { id: "dlq", icon: TriangleAlert, label: "shell.nav.sqs.dlq" },
          { id: "producer", icon: Send, label: "shell.nav.sqs.producer" },
        ],
      },
      {
        label: OPS,
        items: [{ id: "alerts", icon: BellRing, label: "shell.nav.sqs.alerts" }],
      },
    ],
  },
  "google-pubsub": {
    id: "google-pubsub",
    name: "Google Pub/Sub",
    badge: "PUB/SUB",
    badgeClass: "pGPS",
    /* Six entries, and the one that came back is the point. SQS, the other
       family with no address, draws no consumers page because it has no
       subscription of any kind; a Pub/Sub subscription is a real object -
       created, listed and deleted on its own - and it is where the whole of
       the delivery configuration lives, so it takes the consumers slot.

       Topics take the topics slot, and a topic here holds nothing: it fans a
       publish out to whatever subscriptions exist at that moment and discards
       it if none do. That is why the topics board leads with a subscription
       count rather than a depth, and why a topic with none is the fault the
       alerts page raises.

       There is no cluster page and no clients page, because Google runs the
       service and shows no node or session. There is no access page, because
       who may call what is IAM's, one service further out. */
    nav: [
      { items: [{ id: "overview", icon: House, label: "shell.nav.google-pubsub.overview" }] },
      {
        label: BROWSE,
        items: [
          { id: "topics", icon: Layers, label: "shell.nav.google-pubsub.topics" },
          { id: "consumers", icon: Users, label: "shell.nav.google-pubsub.consumers" },
          { id: "messages", icon: Mail, label: "shell.nav.google-pubsub.messages" },
          /* An ordinary topic a subscription's dead-letter policy points at.
             The policy sits on the subscription rather than on the topic,
             which is why each source names both. */
          { id: "dlq", icon: TriangleAlert, label: "shell.nav.google-pubsub.dlq" },
          { id: "producer", icon: Send, label: "shell.nav.google-pubsub.producer" },
        ],
      },
      {
        label: OPS,
        items: [{ id: "alerts", icon: BellRing, label: "shell.nav.google-pubsub.alerts" }],
      },
    ],
  },
  "azure-servicebus": {
    id: "azure-servicebus",
    name: "Azure Service Bus",
    badge: "SB",
    badgeClass: "pASB",
    /* Eight entries, and the new one is the point. Every other family's
       routing lives on the object that holds the messages: a Pub/Sub
       subscription carries a filter as a field, an SQS queue carries none at
       all. A Service Bus subscription carries *rules* - separate objects,
       created and deleted on their own, each a filter and optionally an
       action that rewrites the message on the way in - so which messages
       reach which subscription is a topology rather than a setting, and it
       takes the same slot RabbitMQ's exchanges do.

       Queues and topics share the topics slot. They are the same kind of
       thing to create, configure and delete, and what separates them is
       whether anything has to exist before a message can be read - which the
       subscriptions page is where to see.

       There is no cluster page and no clients page, because Microsoft runs
       the service and shows no node or session. There is no access page,
       because who may call what is a shared access policy on the namespace,
       which is what this connection authenticated with rather than something
       it can enumerate. */
    nav: [
      { items: [{ id: "overview", icon: House, label: "shell.nav.azure-servicebus.overview" }] },
      {
        label: BROWSE,
        items: [
          { id: "topics", icon: Layers, label: "shell.nav.azure-servicebus.topics" },
          { id: "consumers", icon: Users, label: "shell.nav.azure-servicebus.consumers" },
          { id: "exchanges", icon: Waypoints, label: "shell.nav.azure-servicebus.exchanges" },
          { id: "messages", icon: Mail, label: "shell.nav.azure-servicebus.messages" },
          /* Not a queue something else points at: every queue and every
             subscription has a $DeadLetterQueue of its own that the broker
             names, so this reads a sub-entity rather than walking a topology. */
          { id: "dlq", icon: TriangleAlert, label: "shell.nav.azure-servicebus.dlq" },
          { id: "producer", icon: Send, label: "shell.nav.azure-servicebus.producer" },
        ],
      },
      {
        label: OPS,
        items: [{ id: "alerts", icon: BellRing, label: "shell.nav.azure-servicebus.alerts" }],
      },
    ],
  },
  kinesis: {
    id: "kinesis",
    name: "Amazon Kinesis",
    badge: "KDS",
    badgeClass: "pKDS",
    /* Seven entries, and the second one in Browse is the family's own. Every
       other partitioned family here reports a count and nothing else, which
       the streams page can carry in a column. A shard cannot be carried that
       way: it is named, it owns a range of the hash space, it is split and
       merged rather than resized, and a shard closed by a split still holds
       its records until retention expires. So the detail gets a page, and the
       streams page keeps the count.

       Consumers are the enhanced fan-out kind, which are the only readers a
       stream knows about at all - a classic consumer keeps its position in a
       DynamoDB table this connection never sees.

       There is no cluster page, because AWS runs the service and shows no
       node. There is no dead-letter page, because nothing is ever moved: a
       record stays where it was written until retention expires, whether or
       not anybody read it. And there is no access page, because who may call
       what is IAM's, one service further out. */
    nav: [
      { items: [{ id: "overview", icon: House, label: "shell.nav.kinesis.overview" }] },
      {
        label: BROWSE,
        items: [
          { id: "topics", icon: Layers, label: "shell.nav.kinesis.topics" },
          { id: "shards", icon: Split, label: "shell.nav.kinesis.shards" },
          { id: "consumers", icon: Users, label: "shell.nav.kinesis.consumers" },
          { id: "messages", icon: Mail, label: "shell.nav.kinesis.messages" },
          { id: "producer", icon: Send, label: "shell.nav.kinesis.producer" },
        ],
      },
      {
        label: OPS,
        items: [{ id: "alerts", icon: BellRing, label: "shell.nav.kinesis.alerts" }],
      },
    ],
  },
  ibmmq: {
    id: "ibmmq",
    name: "IBM MQ",
    badge: "IMQ",
    badgeClass: "pIMQ",
    /* Seven entries, and the second one in Browse is the family's own. A
       channel is how anything reaches a queue manager at all - a client
       application, another queue manager, a cluster - and it is a configured
       object with a name, a type, a connection name and a status, not a
       transport session the broker happens to be holding. The clients page
       could not carry it and the queues page has nowhere to put it.

       There is no cluster page: an MQ cluster is a set of queue managers that
       publish to each other's repositories, and this connection speaks to one
       of them. There is no access page either - authority records are per
       object and per principal, which is a page of its own rather than a
       column somewhere. Dead letters are here because the queue manager names
       one queue for them and every queue may name its own. */
    nav: [
      { items: [{ id: "overview", icon: House, label: "shell.nav.ibmmq.overview" }] },
      {
        label: BROWSE,
        items: [
          { id: "topics", icon: Layers, label: "shell.nav.ibmmq.topics" },
          { id: "channels", icon: Cable, label: "shell.nav.ibmmq.channels" },
          { id: "consumers", icon: Users, label: "shell.nav.ibmmq.consumers" },
          { id: "messages", icon: Mail, label: "shell.nav.ibmmq.messages" },
          { id: "producer", icon: Send, label: "shell.nav.ibmmq.producer" },
        ],
      },
      {
        label: OPS,
        items: [
          { id: "dlq", icon: TriangleAlert, label: "shell.nav.ibmmq.dlq" },
          { id: "alerts", icon: BellRing, label: "shell.nav.ibmmq.alerts" },
        ],
      },
    ],
  },
  solace: {
    id: "solace",
    name: "Solace PubSub+",
    badge: "SOL",
    badgeClass: "pSOL",
    /* Nine entries, and the exchanges slot is the one worth explaining. A
       Solace queue does not have to be published to by name: it carries topic
       subscriptions, and whatever is published to a matching topic lands on
       it. That is a routing topology rather than a setting on the queue - the
       same page RabbitMQ's exchanges and Service Bus's rules are drawn on -
       and it is where topic endpoints belong too, because a topic endpoint's
       routing is its own name.

       There is no consumers entry: this family has no consumer group. What
       reads a queue is a client bound to it, and that is the clients page.

       The cluster page is a broker page: one appliance, its version and what
       it is holding. A redundancy pair here shares one virtual router and
       only one half is ever active, so there is no list of nodes to draw. */
    nav: [
      { items: [{ id: "overview", icon: House, label: "shell.nav.solace.overview" }] },
      {
        label: BROWSE,
        items: [
          { id: "topics", icon: Layers, label: "shell.nav.solace.topics" },
          { id: "exchanges", icon: Split, label: "shell.nav.solace.exchanges" },
          { id: "messages", icon: Mail, label: "shell.nav.solace.messages" },
          { id: "producer", icon: Send, label: "shell.nav.solace.producer" },
          { id: "clients", icon: Plug, label: "shell.nav.solace.clients" },
        ],
      },
      {
        label: OPS,
        items: [
          { id: "dlq", icon: TriangleAlert, label: "shell.nav.solace.dlq" },
          { id: "cluster", icon: Server, label: "shell.nav.solace.cluster" },
          { id: "alerts", icon: BellRing, label: "shell.nav.solace.alerts" },
        ],
      },
    ],
  },
};

export const PROTOCOL_ORDER: ProtocolId[] = [
  "rocketmq",
  "kafka",
  "rabbitmq",
  "pulsar",
  "redis",
  "mqtt",
  "nats",
  "activemq",
  "nsq",
  "sqs",
  "google-pubsub",
  "azure-servicebus",
  "kinesis",
  "ibmmq",
  "solace",
];

/** Every page the protocol's sidebar can reach, flattened. */
export function pagesOf(protocol: ProtocolId): PageId[] {
  return PROTOCOLS[protocol].nav.flatMap((g) => g.items.map((i) => i.id));
}

export function labelOf(protocol: ProtocolId, page: PageId): string {
  for (const group of PROTOCOLS[protocol].nav) {
    for (const item of group.items) if (item.id === page) return item.label;
  }
  return page;
}

/**
 * Protocols whose boards read a real broker. The other two are drawn in the
 * picker so the shell shows where it is going, but they cannot be selected: a
 * board of invented figures beside a live cluster is worse than no board.
 *
 * Adding one here needs a driver, a form in
 * boards/connections/connectionDraft.ts, and boards that read the endpoint.
 */
const READY: ReadonlySet<ProtocolId> = new Set<ProtocolId>([
  "rocketmq",
  "rabbitmq",
  "kafka",
  "pulsar",
  "redis",
  "mqtt",
  "nats",
  "activemq",
  "nsq",
  "sqs",
  "google-pubsub",
  "azure-servicebus",
  "kinesis",
  "ibmmq",
  "solace",
]);

/**
 * Which of the three families a protocol belongs to.
 *
 * The same split the roadmap and the README already sort drivers into, and the
 * distinction a reader is actually making: whether they run the broker
 * themselves, rent it from a cloud, or bought it.
 *
 * A Record rather than three arrays, so a protocol added to the union without
 * a family stops the build here instead of quietly vanishing from the picker.
 */
export type ProtocolGroupId = "self-hosted" | "hosted" | "enterprise";

const GROUP_OF: Record<ProtocolId, ProtocolGroupId> = {
  rocketmq: "self-hosted",
  kafka: "self-hosted",
  rabbitmq: "self-hosted",
  pulsar: "self-hosted",
  redis: "self-hosted",
  mqtt: "self-hosted",
  nats: "self-hosted",
  activemq: "self-hosted",
  nsq: "self-hosted",
  sqs: "hosted",
  "google-pubsub": "hosted",
  "azure-servicebus": "hosted",
  kinesis: "hosted",
  ibmmq: "enterprise",
  solace: "enterprise",
};

/** `label` is a translation key, resolved at render like every other one. */
export const PROTOCOL_GROUPS: { id: ProtocolGroupId; label: string }[] = [
  { id: "self-hosted", label: "page.connections.group.selfHosted" },
  { id: "hosted", label: "page.connections.group.hosted" },
  { id: "enterprise", label: "page.connections.group.enterprise" },
];

/** The protocols in one family, in the order the picker lists them. */
export function protocolsIn(group: ProtocolGroupId): ProtocolId[] {
  return PROTOCOL_ORDER.filter((protocol) => GROUP_OF[protocol] === group);
}

export function groupOf(protocol: ProtocolId): ProtocolGroupId {
  return GROUP_OF[protocol];
}

export function isProtocolReady(protocol: ProtocolId): boolean {
  return READY.has(protocol);
}
