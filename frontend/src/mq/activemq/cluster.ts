/**
 * ActiveMQ's view of a canonical node and connection.
 *
 * The keys are a contract with internal/driver/activemq/cluster.go and
 * clients.go.
 *
 * Half of these are filled by one product only, and the board leaves the other
 * blank rather than zeroing it. Classic reports a store and a temp percentage
 * and no journal type; Artemis reports a journal type, an HA policy and
 * whether it is a backup, and no temp store at all.
 */
import type { ClientConnection, Node } from "@bindings/model/models";

const AttrProduct = "product";
const AttrUptime = "uptime";
const AttrNodeID = "nodeId";
const AttrPersistence = "persistenceEnabled";
const AttrDataDirectory = "dataDirectory";
const AttrStorePercent = "storePercent";
const AttrTempPercent = "tempPercent";
const AttrMemoryPercent = "memoryPercent";
const AttrMemoryLimit = "memoryLimit";
const AttrTotalMessages = "totalMessages";
const AttrTotalEnqueued = "totalEnqueued";
const AttrTotalDequeued = "totalDequeued";
const AttrConnections = "connections";
const AttrConsumerCount = "consumerCount";
const AttrProducerCount = "producerCount";
const AttrAcceptors = "acceptors";
const AttrClustered = "clustered";
const AttrHAPolicy = "haPolicy";
const AttrBackup = "backup";
const AttrJournalType = "journalType";
const AttrSecurity = "securityEnabled";
const AttrBridge = "bridge";

const AttrSessions = "sessions";
const AttrConnector = "connector";
const AttrRemoteAddr = "remoteAddress";
const AttrCreated = "created";
const AttrBlocked = "blocked";
const AttrSlow = "slow";

export interface ActiveMQNode {
  name: string;
  address: string;
  version: string;
  status: string;
  product: "classic" | "artemis";
  /**
   * True for a broker this one forwards to rather than the one being read.
   * Its state is unknown by construction: the link is declared here and the
   * broker at the other end answers on its own console.
   */
  bridge: boolean;

  uptime: string | null;
  nodeId: string | null;
  persistent: boolean | null;
  dataDirectory: string | null;

  diskUsage: number | null;
  storePercent: number | null;
  tempPercent: number | null;
  memoryPercent: number | null;
  memoryLimit: number | null;

  totalMessages: number | null;
  totalEnqueued: number | null;
  totalDequeued: number | null;
  connections: number | null;
  consumers: number | null;
  producers: number | null;

  acceptors: string[] | null;
  clustered: boolean | null;
  haPolicy: string | null;
  backup: boolean | null;
  journalType: string | null;
  securityEnabled: boolean | null;
}

function nodeText(row: Node, key: string): string | null {
  const value = row.attributes?.[key];
  return value == null || value === "" ? null : value;
}

function nodeNumber(row: Node, key: string): number | null {
  const value = nodeText(row, key);
  if (value == null) return null;
  const parsed = Number(value);
  return Number.isFinite(parsed) ? parsed : null;
}

function nodeBool(row: Node, key: string): boolean | null {
  const value = nodeText(row, key);
  return value == null ? null : value === "true";
}

function metric(value: number): number | null {
  return value < 0 ? null : value;
}

export function node(row: Node): ActiveMQNode {
  const acceptors = nodeText(row, AttrAcceptors);
  return {
    name: row.name,
    address: row.address,
    version: row.version,
    status: row.status,
    product: (nodeText(row, AttrProduct) as "classic" | "artemis") ?? "classic",
    bridge: nodeBool(row, AttrBridge) === true,

    uptime: nodeText(row, AttrUptime),
    nodeId: nodeText(row, AttrNodeID),
    persistent: nodeBool(row, AttrPersistence),
    dataDirectory: nodeText(row, AttrDataDirectory),

    diskUsage: metric(row.diskUsage),
    storePercent: nodeNumber(row, AttrStorePercent),
    tempPercent: nodeNumber(row, AttrTempPercent),
    memoryPercent: nodeNumber(row, AttrMemoryPercent),
    memoryLimit: nodeNumber(row, AttrMemoryLimit),

    totalMessages: nodeNumber(row, AttrTotalMessages),
    totalEnqueued: nodeNumber(row, AttrTotalEnqueued),
    totalDequeued: nodeNumber(row, AttrTotalDequeued),
    connections: nodeNumber(row, AttrConnections),
    consumers: nodeNumber(row, AttrConsumerCount),
    producers: nodeNumber(row, AttrProducerCount),

    acceptors: acceptors == null ? null : acceptors.split(","),
    clustered: nodeBool(row, AttrClustered),
    haPolicy: nodeText(row, AttrHAPolicy),
    backup: nodeBool(row, AttrBackup),
    journalType: nodeText(row, AttrJournalType),
    securityEnabled: nodeBool(row, AttrSecurity),
  };
}

export interface ActiveMQConnection {
  /** The broker's own id, which is what a close names. */
  name: string;
  clientName: string;
  user: string;
  peer: string;
  peerHost: string;
  peerPort: number;
  /** AMQP, CORE, STOMP… read off the connection class or the connector. */
  protocol: string;
  state: string;
  /** JMS sessions, which are not channels - see the board. */
  sessions: number;
  product: "classic" | "artemis";
  connector: string | null;
  created: string | null;
  blocked: boolean | null;
  slow: boolean | null;
}

export function connection(row: ClientConnection): ActiveMQConnection {
  const attributes = row.attributes ?? {};
  const text = (key: string): string | null => {
    const value = attributes[key];
    return value == null || value === "" ? null : value;
  };
  const bool = (key: string): boolean | null => {
    const value = text(key);
    return value == null ? null : value === "true";
  };

  return {
    name: row.name,
    clientName: row.clientName,
    user: row.user,
    peer: text(AttrRemoteAddr) ?? `${row.peerHost}:${row.peerPort}`,
    peerHost: row.peerHost,
    peerPort: row.peerPort,
    protocol: row.protocol,
    state: row.state,
    sessions: Number(text(AttrSessions) ?? row.channels),
    product: (text(AttrProduct) as "classic" | "artemis") ?? "classic",
    connector: text(AttrConnector),
    created: text(AttrCreated),
    blocked: bool(AttrBlocked),
    slow: bool(AttrSlow),
  };
}
