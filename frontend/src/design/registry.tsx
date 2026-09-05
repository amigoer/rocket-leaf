import type { JSX } from "react";
import type { PageId, ProtocolId } from "@/design/data/protocols";
import { PROTOCOLS, labelOf } from "@/design/data/protocols";

import { OverviewRocketMQ } from "./boards/overview/OverviewRocketMQ";
import { OverviewKafka } from "./boards/overview/OverviewKafka";
import { OverviewRabbitMQ } from "./boards/overview/OverviewRabbitMQ";
import { OverviewPulsar } from "./boards/overview/OverviewPulsar";
import { OverviewRedis } from "./boards/overview/OverviewRedis";
import { OverviewMqtt } from "./boards/overview/OverviewMqtt";
import { OverviewNats } from "./boards/overview/OverviewNats";

import { TopicsRocketMQ } from "./boards/topics/TopicsRocketMQ";
import { TopicsKafka } from "./boards/topics/TopicsKafka";
import { TopicsPulsar } from "./boards/topics/TopicsPulsar";
import { QueuesRabbitMQ } from "./boards/topics/QueuesRabbitMQ";
import { ExchangesRabbitMQ } from "./boards/topics/ExchangesRabbitMQ";
import { VhostsRabbitMQ } from "./boards/vhosts/VhostsRabbitMQ";
import { NamespacesPulsar } from "./boards/vhosts/NamespacesPulsar";
import { AccountsNats } from "./boards/vhosts/AccountsNats";
import { UsersRabbitMQ } from "./boards/acl/UsersRabbitMQ";
import { PoliciesRabbitMQ } from "./boards/policies/PoliciesRabbitMQ";
import { DefinitionsRabbitMQ } from "./boards/definitions/DefinitionsRabbitMQ";
import { ReplicationRabbitMQ } from "./boards/replication/ReplicationRabbitMQ";
import { TopicsMqtt } from "./boards/topics/TopicsMqtt";
import { StreamsRedis } from "./boards/topics/StreamsRedis";
import { StreamsNats } from "./boards/topics/StreamsNats";

import { ConsumersRocketMQ } from "./boards/consumers/ConsumersRocketMQ";
import { ConsumersKafka } from "./boards/consumers/ConsumersKafka";
import { SubscriptionsPulsar } from "./boards/consumers/SubscriptionsPulsar";
import { ConsumersRedis } from "./boards/consumers/ConsumersRedis";
import { ConsumersNats } from "./boards/consumers/ConsumersNats";
import { ClientsMqtt } from "./boards/consumers/ClientsMqtt";
import { ClientsRedis } from "./boards/consumers/ClientsRedis";
import { ClientsNats } from "./boards/consumers/ClientsNats";
import { ChannelsRabbitMQ } from "./boards/consumers/ChannelsRabbitMQ";

import { MessagesRocketMQ } from "./boards/messages/MessagesRocketMQ";
import { MessagesKafka } from "./boards/messages/MessagesKafka";
import { MessagesPulsar } from "./boards/messages/MessagesPulsar";
import { MessagesRabbitMQ } from "./boards/messages/MessagesRabbitMQ";
import { MessagesRedis } from "./boards/messages/MessagesRedis";
import { MessagesNats } from "./boards/messages/MessagesNats";

import { DlqRocketMQ } from "./boards/dlq/DlqRocketMQ";
import { DlqRabbitMQ } from "./boards/dlq/DlqRabbitMQ";
import { DlqPulsar } from "./boards/dlq/DlqPulsar";
import { PelRedis } from "./boards/dlq/PelRedis";

import { Producer } from "./boards/producer/Producer";
import { ProducerKafka } from "./boards/producer/ProducerKafka";
import { ProducerMqtt } from "./boards/producer/ProducerMqtt";
import { ProducerPulsar } from "./boards/producer/ProducerPulsar";
import { ProducerRabbitMQ } from "./boards/producer/ProducerRabbitMQ";
import { ProducerRedis } from "./boards/producer/ProducerRedis";
import { ProducerNats } from "./boards/producer/ProducerNats";
import { Alerts } from "./boards/alerts/Alerts";
import { Acl } from "./boards/acl/Acl";
import { AclKafka } from "./boards/acl/AclKafka";
import { TokensPulsar } from "./boards/acl/TokensPulsar";
import { AclRedis } from "./boards/acl/AclRedis";
import { QuotasKafka } from "./boards/quotas/QuotasKafka";

import { ClusterRocketMQ } from "./boards/cluster/ClusterRocketMQ";
import { BrokersKafka } from "./boards/cluster/BrokersKafka";
import { BrokersPulsar } from "./boards/cluster/BrokersPulsar";
import { NodesRabbitMQ } from "./boards/cluster/NodesRabbitMQ";
import { NodeRedis } from "./boards/cluster/NodeRedis";
import { NodesMqtt } from "./boards/cluster/NodesMqtt";
import { ServersNats } from "./boards/cluster/ServersNats";

import { MqttWorkbench } from "./boards/mqtt/MqttWorkbench";
import { NatsWorkbench } from "./boards/nats/NatsWorkbench";
import { DestinationsActiveMQ } from "./boards/topics/DestinationsActiveMQ";
import { SubscriptionsActiveMQ } from "./boards/consumers/SubscriptionsActiveMQ";
import { MessagesActiveMQ } from "./boards/messages/MessagesActiveMQ";
import { DlqActiveMQ } from "./boards/dlq/DlqActiveMQ";
import { NotDesigned } from "./boards/misc/NotDesigned";

/**
 * What one page should arrive looking at, when it was reached from another.
 *
 * A board reads it once, on mount, so a later edit by the user is never
 * overwritten by where they came from.
 */
export interface BoardFocus {
  topic?: string;
  group?: string;
  /** Open the page's create dialog on arrival. */
  create?: boolean;
}

/** What a board may ask the shell to do, for the few that need to. */
export interface BoardNav {
  /** The alerts page sends the reader to where the thresholds are set. */
  onOpenAlertSettings?: () => void;
  /** Move to another page in this tab, optionally naming what to open on. */
  onOpenPage?: (page: PageId, focus?: BoardFocus) => void;
  /** Set when this page was reached through `onOpenPage`. */
  focus?: BoardFocus;
}

export interface BoardProps {
  nav?: BoardNav;
}

/**
 * (page, protocol) -> board. Every cell is its own component: the canvas draws
 * each protocol's page separately and the differences are semantic, not
 * cosmetic, so nothing is shared beyond the primitive layer.
 */
const BOARDS: Partial<
  Record<PageId, Partial<Record<ProtocolId, (props: BoardProps) => JSX.Element>>>
> = {
  overview: {
    rocketmq: OverviewRocketMQ,
    kafka: OverviewKafka,
    rabbitmq: OverviewRabbitMQ,
    pulsar: OverviewPulsar,
    redis: OverviewRedis,
    mqtt: OverviewMqtt,
    nats: OverviewNats,
  },
  topics: {
    rocketmq: TopicsRocketMQ,
    kafka: TopicsKafka,
    rabbitmq: QueuesRabbitMQ,
    pulsar: TopicsPulsar,
    redis: StreamsRedis,
    mqtt: TopicsMqtt,
    nats: StreamsNats,
    activemq: DestinationsActiveMQ,
  },
  exchanges: { rabbitmq: ExchangesRabbitMQ },
  vhosts: { rabbitmq: VhostsRabbitMQ, pulsar: NamespacesPulsar, nats: AccountsNats },
  policies: { rabbitmq: PoliciesRabbitMQ },
  definitions: { rabbitmq: DefinitionsRabbitMQ },
  replication: { rabbitmq: ReplicationRabbitMQ },
  quotas: { kafka: QuotasKafka },
  consumers: {
    rocketmq: ConsumersRocketMQ,
    kafka: ConsumersKafka,
    rabbitmq: ChannelsRabbitMQ,
    pulsar: SubscriptionsPulsar,
    redis: ConsumersRedis,
    nats: ConsumersNats,
    activemq: SubscriptionsActiveMQ,
  },
  subscribe: { mqtt: MqttWorkbench, nats: NatsWorkbench },
  clients: { mqtt: ClientsMqtt, redis: ClientsRedis, nats: ClientsNats },
  messages: {
    rocketmq: MessagesRocketMQ,
    kafka: MessagesKafka,
    rabbitmq: MessagesRabbitMQ,
    pulsar: MessagesPulsar,
    redis: MessagesRedis,
    nats: MessagesNats,
    activemq: MessagesActiveMQ,
  },
  dlq: {
    rocketmq: DlqRocketMQ,
    rabbitmq: DlqRabbitMQ,
    pulsar: DlqPulsar,
    redis: PelRedis,
    activemq: DlqActiveMQ,
  },
  cluster: {
    rocketmq: ClusterRocketMQ,
    kafka: BrokersKafka,
    rabbitmq: NodesRabbitMQ,
    pulsar: BrokersPulsar,
    redis: NodeRedis,
    mqtt: NodesMqtt,
    nats: ServersNats,
  },
};

export function renderBoard(
  protocol: ProtocolId,
  page: PageId,
  nav?: BoardNav,
): JSX.Element {
  /* The send console is per family, not shared: RabbitMQ's collects an
     exchange, a routing key, headers and AMQP properties, Redis's collects an
     ordered list of named fields and an optional entry id, and the shared one
     collects a topic, tags, keys and a delay level - RocketMQ's vocabulary, of
     which only the body means anything to any of them. */
  if (page === "producer") {
    if (protocol === "rabbitmq") return <ProducerRabbitMQ />;
    if (protocol === "kafka") return <ProducerKafka />;
    /* Pulsar's own too: the shared console collects tags and a RocketMQ delay
       level, and this family has no tag at all - what a RocketMQ producer puts
       in one, a Pulsar producer puts in a property. */
    if (protocol === "pulsar") return <ProducerPulsar />;
    if (protocol === "redis") return <ProducerRedis />;
    if (protocol === "mqtt") return <ProducerMqtt />;
    /* NATS's own too: the shared console collects a topic, tags, keys and a
       delay level, and this family has none of those - what a NATS publish
       needs instead is headers, the choice between a core send and a stored
       one, and a reply timeout. */
    if (protocol === "nats") return <ProducerNats />;
    return <Producer protocol={protocol} nav={nav} />;
  }
  /* Alerts is one board for every family: the rules are numeric comparisons
     over a cluster snapshot, with nothing protocol-specific to draw. */
  if (page === "alerts") return <Alerts onOpenSettings={nav?.onOpenAlertSettings} />;
  /* Access control is per family, and deliberately so: each speaks its own
     model. RocketMQ has a credential pair carrying its own permissions;
     RabbitMQ has users whose tags gate the management API and whose
     per-virtual-host permissions gate AMQP, which are two systems on one name. */
  if (page === "acl") {
    if (protocol === "rocketmq") return <Acl />;
    if (protocol === "rabbitmq") return <UsersRabbitMQ />;
    if (protocol === "kafka") return <AclKafka />;
    /* Pulsar has no users at all: it authorises the subject of a token and
       keeps no directory of them, so the page lists grants rather than
       accounts and is named for what it is. */
    if (protocol === "pulsar") return <TokensPulsar />;
    if (protocol === "redis") return <AclRedis />;
  }

  const Board = BOARDS[page]?.[protocol];
  if (Board) return <Board nav={nav} />;

  return (
    <NotDesigned labelKey={labelOf(protocol, page)} protocolName={PROTOCOLS[protocol].name} />
  );
}
