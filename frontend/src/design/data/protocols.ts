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
  | "activemq";

export type PageId =
  | "overview"
  | "topics"
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
]);

export function isProtocolReady(protocol: ProtocolId): boolean {
  return READY.has(protocol);
}
