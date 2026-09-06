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
};

export function protocolOfKind(kind: MQKind): ProtocolId | null {
  return PROTOCOL_BY_KIND[kind] ?? null;
}

/**
 * What the address column shows, which is not always an address.
 *
 * Most families here are reached by dialling something, so the profile's
 * endpoints field is what identifies it. Two of the three hosted ones are not:
 * SQS is reached by naming a region and signing a request, Pub/Sub by naming a
 * project and authenticating, and both leave endpoints deliberately empty - so
 * a row that printed it would leave the column blank on a perfectly good
 * connection, and two of them would look identical.
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
 */
function addressOf(profile: ConnectionProfile): string {
  if (profile.kind === MQKind.KindSQS) return profile.options?.region ?? "";
  if (profile.kind === MQKind.KindGooglePubSub) return profile.options?.projectId ?? "";
  if (profile.kind === MQKind.KindAzureServiceBus) return namespaceOf(profile.endpoints);
  return profile.endpoints;
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

/** Only RocketMQ scopes a connection by name today. */
function scopeOf(profile: ConnectionProfile): string | undefined {
  if (profile.kind !== MQKind.KindRocketMQ) return undefined;
  const namespace = profile.options?.namespace ?? "";
  return namespace === "" ? undefined : namespace;
}

export function toShellConnection(profile: ConnectionProfile): Connection {
  const protocol = protocolOfKind(profile.kind);
  return {
    key: String(profile.id),
    id: profile.id,
    name: profile.name,
    protocol,
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
