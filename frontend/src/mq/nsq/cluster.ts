/**
 * NSQ's view of a canonical node, for both tiers.
 *
 * The keys are a contract with internal/driver/nsq/cluster.go.
 *
 * Two kinds of row read through here and they carry different fields, which is
 * the shape of the family rather than untidiness: an nsqd holds messages and
 * reports what it is holding; an nsqlookupd holds nothing and reports what it
 * would tell a consumer. The reader for each takes only what its tier has.
 */
import type { Node } from "@bindings/model/models";

const AttrHostname = "hostname";
const AttrBroadcastAddress = "broadcastAddress";
const AttrTCPPort = "tcpPort";
const AttrHTTPPort = "httpPort";
const AttrHealth = "health";
const AttrTopicCount = "topicCount";
const AttrChannelCount = "channelCount";
const AttrClientCount = "clientCount";
const AttrDepth = "depth";
const AttrHeapInUse = "heapInUseBytes";
const AttrHeapObjects = "heapObjects";
const AttrGCRuns = "gcTotalRuns";
const AttrProducerCount = "producerCount";
const AttrDirectoryTopics = "directoryTopics";
const AttrNodes = "nodes";

export interface NsqNode {
  name: string;
  address: string;
  version: string;
  status: string;

  hostname: string;
  /** What this daemon tells nsqlookupd to advertise it as. */
  broadcastAddress: string;
  tcpPort: string;
  httpPort: string;

  /** nsqd's own verdict on itself: "OK", or the error that broke it. */
  health: string;
  topics: number | null;
  channels: number | null;
  clients: number | null;
  depth: number | null;

  heapInUse: number | null;
  heapObjects: number | null;
  gcRuns: number | null;
}

export interface NsqDirectoryNode {
  address: string;
  version: string;
  /** How many nsqd have registered with this lookupd. */
  producers: number | null;
  /** How many topic names it knows, which is not the same as how many exist. */
  topics: number | null;
  /**
   * The daemons it would hand a consumer, spelled as each broadcast itself.
   * An address only the cluster can resolve is a misconfiguration, and this is
   * the one place it shows.
   */
  advertises: string[];
}

function text(node: Node, key: string): string {
  return node.attributes?.[key] ?? "";
}

function number(node: Node, key: string): number | null {
  const value = text(node, key);
  if (value === "") return null;
  const parsed = Number(value);
  return Number.isFinite(parsed) ? parsed : null;
}

export function node(entry: Node): NsqNode {
  return {
    name: entry.name,
    address: entry.address,
    version: entry.version,
    status: entry.status,

    hostname: text(entry, AttrHostname),
    broadcastAddress: text(entry, AttrBroadcastAddress),
    tcpPort: text(entry, AttrTCPPort),
    httpPort: text(entry, AttrHTTPPort),

    health: text(entry, AttrHealth),
    topics: number(entry, AttrTopicCount),
    channels: number(entry, AttrChannelCount),
    clients: number(entry, AttrClientCount),
    depth: number(entry, AttrDepth),

    heapInUse: number(entry, AttrHeapInUse),
    heapObjects: number(entry, AttrHeapObjects),
    gcRuns: number(entry, AttrGCRuns),
  };
}

export function directoryNode(entry: Node): NsqDirectoryNode {
  const advertised = text(entry, AttrNodes);
  return {
    address: entry.address,
    version: entry.version,
    producers: number(entry, AttrProducerCount),
    topics: number(entry, AttrDirectoryTopics),
    advertises: advertised === "" ? [] : advertised.split(","),
  };
}

/**
 * Whether a daemon is advertising itself at an address this app could not use.
 *
 * The failure worth naming on this page: nsqlookupd hands a consumer whatever
 * each nsqd broadcast about itself, so a daemon reachable here as
 * 127.0.0.1:4151 and broadcasting a container hostname sends every consumer
 * that uses discovery somewhere they cannot reach.
 */
export function advertisesSomethingElse(
  directory: NsqDirectoryNode[],
  nodes: NsqNode[],
): string[] {
  const reachable = new Set(nodes.map((entry) => entry.address));
  const advertised = new Set(directory.flatMap((entry) => entry.advertises));
  return [...advertised].filter((address) => !reachable.has(address)).sort();
}
