import type { Connection as ConnectionProfile } from "@/api/models";
import { MQKind } from "@/api/connection";
import { PROTOCOLS, type ProtocolId } from "./protocols";

/**
 * A stored connection profile as the shell draws it.
 *
 * The canvas drew six sample rows; these are the real ones, mapped from what
 * Go keeps in connections.json. Two fields the canvas drew have no source yet:
 * `latency` needs a live connection, and the status is whatever the last
 * connection attempt left behind rather than a current probe.
 */
export type ConnectionStatus = "online" | "offline" | "failed";

export type Connection = {
  /** The profile id as a string: the shell keys tabs and sessions by it. */
  key: string;
  id: number;
  name: string;
  /** null for a broker family the design has no boards for. */
  protocol: ProtocolId | null;
  /**
   * The stored kind, which is not the protocol id: Redis Stream is "redis" to
   * the design shell and "redis-stream" on disk. It is carried because the
   * i18n bundle is keyed by kind, so anything that wants a family's own
   * wording - the scope switcher's, today - needs this rather than the id.
   */
  kind: MQKind;
  protocolLabel: string;
  address: string;
  /**
   * What the connection is scoped to inside that address, when it is scoped to
   * anything: a RocketMQ namespace today. It is shown beside the address
   * because every page then draws short names, and a reader with no marker
   * would have no way to tell a namespaced connection from an unscoped one.
   */
  scope?: string;
  status: ConnectionStatus;
  latency?: string;
  lastUsed: string;
  isDefault: boolean;
  remark: string;
};

/**
 * The families the design draws boards for. A profile of any other kind is
 * still listed and still editable -- it just has no page to open.
 */
const PROTOCOL_BY_KIND: Partial<Record<MQKind, ProtocolId>> = {
  [MQKind.KindRocketMQ]: "rocketmq",
  [MQKind.KindKafka]: "kafka",
  [MQKind.KindRabbitMQ]: "rabbitmq",
  [MQKind.KindPulsar]: "pulsar",
  [MQKind.KindRedisStream]: "redis",
  [MQKind.KindMQTT]: "mqtt",
  [MQKind.KindNATS]: "nats",
  [MQKind.KindActiveMQ]: "activemq",
  [MQKind.KindNSQ]: "nsq",
  [MQKind.KindSQS]: "sqs",
  [MQKind.KindGooglePubSub]: "google-pubsub",
  [MQKind.KindAzureServiceBus]: "azure-servicebus",
  [MQKind.KindKinesis]: "kinesis",
  [MQKind.KindIBMMQ]: "ibmmq",
  [MQKind.KindSolace]: "solace",
};

export function protocolOfKind(kind: MQKind): ProtocolId | null {
  return PROTOCOL_BY_KIND[kind] ?? null;
}

/**
 * What the address column shows, which is not always an address.
 *
 * Most families here are reached by dialling something, so the profile's
 * endpoints field is what identifies it. Three of the four hosted ones are not:
 * SQS and Kinesis are reached by naming a region and signing a request, Pub/Sub
 * by naming a project and authenticating, and all three leave endpoints
 * deliberately empty - so a row that printed it would leave the column blank on
 * a perfectly good connection, and three of them would look identical.
 *
 * What goes there instead is whichever field says where the objects are, which
 * is the same field each form makes required.
 *
 * Service Bus is the hosted family that breaks that pattern, and it belongs
 * here for the opposite reason: its namespace *is* an address, and the field
 * takes three spellings of one - a bare host, a host:port for an emulator, and
 * the sb:// URL out of a connection string. The row shows the host the driver
 * actually dials rather than whichever of the three was typed, which is what
 * internal/driver/azureservicebus's namespaceOf does with the same value.
 *
 * IBM MQ is here for a third reason: its address is real and it is not the
 * whole of what identifies the connection. One mqweb server can front several
 * queue managers, they share nothing, and two profiles pointed at two of them
 * would otherwise print the same row. So the queue manager is appended as the
 * path segment it actually is in every REST call the driver makes.
 *
 * Solace is here for the same reason and not quite the same one. A Message VPN
 * is also a path segment on a broker that hosts several, so two profiles would
 * print the same row - but unlike a queue manager it is a scope, and the
 * sidebar re-points it without the profile being edited. That makes the row the
 * only place a user can see which VPN a connection is on while looking at the
 * list.
 */
function addressOf(profile: ConnectionProfile): string {
  if (profile.kind === MQKind.KindSQS) return profile.options?.region ?? "";
  if (profile.kind === MQKind.KindKinesis) return profile.options?.region ?? "";
  if (profile.kind === MQKind.KindGooglePubSub) return profile.options?.projectId ?? "";
  if (profile.kind === MQKind.KindAzureServiceBus) return namespaceOf(profile.endpoints);
  if (profile.kind === MQKind.KindIBMMQ) return queueManagerAddress(profile);
  if (profile.kind === MQKind.KindSolace) return msgVpnAddress(profile);
  return profile.endpoints;
}

/**
 * The mqweb address with the queue manager on it, when the profile names one.
 *
 * It may not: a profile that left the field blank is asking the driver to
 * discover the only queue manager the server fronts, and printing a guess here
 * would be printing something the profile does not say.
 */
function queueManagerAddress(profile: ConnectionProfile): string {
  const server = profile.endpoints.trim().replace(/\/+$/, "");
  const qmgr = (profile.options?.queueManager ?? "").trim();
  if (server === "" || qmgr === "") return server;
  return `${server}/${qmgr}`;
}

/**
 * The SEMP address with the Message VPN on it, when the profile names one.
 *
 * It may not, and blank means something different here than it does for IBM
 * MQ: the driver falls back to "default", which every broker ships. Printing
 * that fallback would still be printing something the profile does not say, so
 * the row shows the broker alone until the scope switcher writes a name.
 */
function msgVpnAddress(profile: ConnectionProfile): string {
  const broker = profile.endpoints.trim().replace(/\/+$/, "");
  const vpn = (profile.options?.msgVpn ?? "").trim();
  if (broker === "" || vpn === "") return broker;
  return `${broker}/${vpn}`;
}

/**
 * The host out of whatever a Service Bus endpoint field holds.
 *
 * Deliberately string work rather than `new URL`: the field's ordinary value
 * is a bare host, which URL refuses outright, and its emulator value carries a
 * port that has to survive.
 */
function namespaceOf(endpoints: string): string {
  const first = (endpoints.split(/[,;\n]/)[0] ?? "").trim().replace(/\/+$/, "");
  const scheme = first.indexOf("://");
  return scheme < 0 ? first : first.slice(scheme + 3);
}

/**
 * What the connection is scoped to, for the two families that carry one.
 *
 * The option key is the driver's own - RocketMQ spells it "namespace" and
 * Solace "msgVpn" - and it is read here rather than from the descriptor
 * because this runs on a list of stored profiles with nothing dialled.
 *
 * Empty means something different in each and neither is shown: a RocketMQ
 * connection with no namespace is reading the whole cluster, and a Solace
 * profile with no Message VPN is resolved to one at dial time. Printing a
 * marker for either would be printing something the profile does not say.
 */
function scopeOf(profile: ConnectionProfile): string | undefined {
  const option =
    profile.kind === MQKind.KindRocketMQ
      ? profile.options?.namespace
      : profile.kind === MQKind.KindSolace
        ? profile.options?.msgVpn
        : undefined;
  const scope = option ?? "";
  return scope === "" ? undefined : scope;
}

export function toShellConnection(profile: ConnectionProfile): Connection {
  const protocol = protocolOfKind(profile.kind);
  return {
    key: String(profile.id),
    id: profile.id,
    name: profile.name,
    protocol,
    kind: profile.kind,
    // A family with no board still names itself; the raw kind is all there is.
    protocolLabel: protocol != null ? PROTOCOLS[protocol].name : profile.kind,
    address: addressOf(profile),
    scope: scopeOf(profile),
    status: profile.status === "online" ? "online" : "offline",
    lastUsed: profile.lastCheck,
    isDefault: profile.isDefault,
    remark: profile.remark,
  };
}
