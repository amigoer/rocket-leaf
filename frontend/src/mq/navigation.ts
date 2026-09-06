/**
 * Navigation derived from what the connection can do.
 *
 * The sidebar used to be a constant and the disabled set a hardcoded list of
 * RocketMQ page ids. That works while every family answers every question and
 * breaks the moment one does not: MQTT has no destinations to list and no
 * groups to inspect, and an app that renders those entries greyed out reads as
 * broken rather than as honest about the broker.
 */
import { Capability } from "@bindings/model/models";
import type { CapabilityState } from "./capabilities";

/**
 * The capability a page needs to be worth showing at all.
 *
 * A list means any one of them will do: two families can answer the same page
 * by different means, and dead letters is where that first showed up. RocketMQ
 * has a dead-letter topic per consumer group and reads it like any other;
 * RabbitMQ has ordinary queues that something else dead-letters into, found by
 * walking the topology. Neither can answer the page the other's way, and both
 * answer it.
 */
const requires: Record<string, Capability | Capability[]> = {
  topics: Capability.CapDestinationList,
  // Only Kinesis, and deliberately not CapPartitions. That capability's page
  // is a read range per partition number; a shard has a name, a slice of the
  // hash space, and a parent it was split from, and a family that reported a
  // count would have nothing to draw here.
  shards: Capability.CapShards,
  // Only IBM MQ, and deliberately not CapClientInspect. That capability's page
  // lists the connections open right now; a channel is the definition they
  // have to come through, it is there with nothing connected, and one of them
  // carries many connections at once.
  channels: Capability.CapChannels,
  consumers: Capability.CapSubscriptionList,
  // Three families draw a routing page and only one of them has exchanges.
  // A Service Bus rule decides which of a topic's messages reach one
  // subscription; a Solace queue's topic subscriptions decide whether it
  // receives anything at all, because a publisher on that broker never names
  // a queue. All three are a topology rather than a setting on the reader,
  // and a family with none of them must not draw the entry.
  exchanges: Capability.CapRouting,
  messages: Capability.CapMessageQuery,
  // A live subscription is not a message query: there is nothing stored to
  // query, so the page needs its own capability rather than borrowing one
  // that promises history. MQTT, NATS and ActiveMQ draw it - and the last of
  // those only when its AMQP acceptor is reachable, because JMX cannot push.
  subscribe: Capability.CapLiveStream,
  // Three means for ten families, because none can answer the page
  // another's way. RocketMQ reads a dead-letter topic per consumer group and
  // Service Bus reads the $DeadLetterQueue the broker gives every entity;
  // RabbitMQ, Pulsar, ActiveMQ, SQS, Pub/Sub, IBM MQ and Solace all go looking
  // for what something else dead-letters into - through the topology, by the
  // naming convention the client libraries use, by the dead-letter address a
  // queue declares, by a queue's redrive policy, by a subscription's, by the
  // queue manager's own DEADQ plus every queue's backout queue, and by the
  // deadMsgQueue every endpoint carries; Redis moves nothing at all and keeps,
  // per group, a record of every delivery it has not had acknowledged.
  dlq: [
    Capability.CapDLQ,
    Capability.CapDeadLetterTopology,
    Capability.CapPendingEntries,
  ],
  vhosts: Capability.CapNamespaceList,
  policies: Capability.CapPolicyList,
  definitions: Capability.CapDefinitionsExport,
  replication: Capability.CapReplication,
  // Only Kafka throttles by identity rather than by destination.
  quotas: Capability.CapQuotaList,
  // Two, because a family whose message is an ordered set of named fields
  // cannot be sent through a signature built for a topic with a body.
  producer: [Capability.CapPublish, Capability.CapEntryPublish],
  // The page had no requirement at all, so every family listing it drew the
  // entry whether or not its driver could answer it.
  clients: Capability.CapClientInspect,
  cluster: Capability.CapClusterTopology,
  // Five, because five families answer this page by five different means.
  // RocketMQ has a credential pair carrying its own permissions; RabbitMQ has
  // users whose tags and per-vhost permissions are two systems on one name;
  // Kafka has rules attached to a principal it may not even store; Redis puts
  // the commands, the key patterns and the channel patterns all on the user;
  // Pulsar has grants and no principal store at all, for the reason below.
  // None can answer the page the others' way, and all five answer it.
  acl: [
    Capability.CapAccessControl,
    Capability.CapIdentityList,
    Capability.CapAccessDirectory,
    /* Pulsar has no principal store to enumerate: it authorises the subject of
       a token, and the cluster keeps no directory of them. What it has is
       grants, so the page is reachable through the permissions capability
       alone - without this entry a family that can read and write every grant
       on the cluster would have no page to draw them on. */
    Capability.CapIdentityPermissions,
    Capability.CapAclUsers,
  ],
  /*
   * Alerts needs no particular capability, only a connection with figures to
   * compare - which the connected check below already covers. What the two
   * entries settle is where those figures come from: most families report
   * cluster metrics, and a family with no cluster to report has none. That is
   * the four hosted ones, and IBM MQ - whose connection speaks to one queue
   * manager rather than to a cluster of them. Everything all five report
   * belongs to a queue, a topic or a stream, so their rules read the
   * destination listing, and gating on a metric none of them can ever declare
   * would hide a page that works.
   */
  alerts: [Capability.CapClusterMetrics, Capability.CapDestinationList],
};

/**
 * Pages that stand on their own: the shell and the landing page.
 *
 * Alerts is deliberately not here. Its rules are broker-agnostic numeric
 * comparisons, but it has nothing to compare until a connection reports
 * metrics.
 */
const alwaysAvailable = new Set(["home", "connections", "settings", "github"]);

export interface NavAvailability {
  /** False when the family has no such concept; the entry is not drawn. */
  visible: (id: string) => boolean;
  /** True when the entry is drawn but cannot be used yet. */
  disabled: (id: string) => boolean;
  /** Set when the endpoint reports why it cannot do this. */
  reason: (id: string) => string | undefined;
}

/**
 * Works out what to draw.
 *
 * Being offline disables nothing. Every board renders its own "not connected"
 * state, which says more than a dead sidebar does, and a nav that goes inert
 * the moment a broker drops takes away the one thing still worth doing -
 * looking at the other pages to see how far the outage reaches.
 *
 * What the sidebar does gate on is the endpoint's answer once it has one: a
 * capability it reports a reason for is drawn disabled with that reason, and
 * one the family has no concept of is not drawn at all.
 */
export function navAvailability(
  capabilities: CapabilityState,
  connected: boolean,
): NavAvailability {
  const asked = (id: string): Capability[] => {
    const wanted = requires[id];
    if (wanted == null) return [];
    return Array.isArray(wanted) ? wanted : [wanted];
  };
  const known = (id: string) =>
    asked(id).find(
      (capability) =>
        capabilities.has(capability) ||
        capabilities.degradedReason(capability) !== undefined,
    );
  // Before the endpoint answers, nothing is known; hiding pages that would
  // come back reads worse than showing them and finding out.
  const unknown = !connected || capabilities.loading;

  return {
    visible: (id) => {
      if (alwaysAvailable.has(id)) return true;
      if (asked(id).length === 0 || unknown) return true;
      return known(id) !== undefined;
    },
    disabled: (id) => {
      if (alwaysAvailable.has(id)) return false;
      if (asked(id).length === 0 || unknown) return false;
      // Usable if any one of the capabilities is plainly supported; degraded
      // on all of them is what disables the entry.
      return !asked(id).some((capability) => capabilities.has(capability));
    },
    reason: (id) => {
      const capability = known(id);
      return capability ? capabilities.degradedReason(capability) : undefined;
    },
  };
}
