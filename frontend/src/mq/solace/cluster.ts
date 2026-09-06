/**
 * Solace's view of the broker.
 *
 * The keys are a contract with internal/driver/solace/cluster.go.
 *
 * The pair worth reading twice is the spool. A Message VPN reports its usage
 * in bytes and its cap in megabytes, on the same object, with names three
 * letters apart - so the two are carried under names that say which is which
 * and are only ever compared through the percentage the driver already scaled.
 *
 * There is no broker name anywhere in SEMP v2, so a node's name is the address
 * the profile dialled. That is a fact about the API rather than a placeholder.
 */
import type { ClusterOverview, Node } from "@bindings/model/models";

const AttrVersion = "version";
const AttrRedundancy = "redundancyEnabled";
const AttrSpoolMaxMb = "spoolMaxMb";
const AttrSpoolUsedBytes = "spoolUsedBytes";
const AttrSpoolMsgCount = "spoolMsgCount";
const AttrClientCount = "clientCount";
const AttrQueueCount = "queueCount";
const AttrEndpointCount = "topicEndpointCount";
const AttrVpnState = "msgVpnState";
const AttrMsgVpn = "msgVpn";
const AttrBrokerSpoolMaxMb = "brokerSpoolMaxMb";
const AttrMaxConnections = "maxConnections";

export interface SolaceBroker {
  address: string;
  version: string | null;
  /** The Message VPN this connection reads, which is what every board is scoped to. */
  msgVpn: string;
  /** "up", "disabled" or the broker's own word. */
  msgVpnState: string | null;
  online: boolean;
  rateIn: number | null;
  rateOut: number | null;
  /** Already scaled by the driver: the two raw figures are in different units. */
  spoolPercent: number | null;
  spoolUsedBytes: number | null;
  spoolMaxMb: number | null;
  brokerSpoolMaxMb: number | null;
  spoolMsgCount: number | null;
  maxConnections: number | null;
  redundancyEnabled: boolean;
}

export interface SolaceOverview {
  msgVpn: string;
  version: string | null;
  queues: number | null;
  topicEndpoints: number | null;
  clients: number | null;
  spoolPercent: number | null;
  spoolUsedBytes: number | null;
  spoolMaxMb: number | null;
  spoolMsgCount: number | null;
  msgVpnState: string | null;
}

type Attributed = { attributes?: Record<string, string | undefined> };

function attribute(source: Attributed, key: string): string | null {
  const value = source.attributes?.[key];
  return value == null || value === "" ? null : value;
}

function numberAttribute(source: Attributed, key: string): number | null {
  const value = attribute(source, key);
  if (value == null) return null;
  const parsed = Number(value);
  return Number.isFinite(parsed) ? parsed : null;
}

/** UnknownMetric is -1 on the wire, and it means "no such figure here". */
function metric(value: number): number | null {
  return value < 0 ? null : value;
}

export function broker(node: Node): SolaceBroker {
  return {
    address: node.address,
    version: node.version === "" ? null : node.version,
    msgVpn: attribute(node, AttrMsgVpn) ?? node.cluster,
    msgVpnState: attribute(node, AttrVpnState),
    online: node.status === "online",
    rateIn: metric(node.rateIn),
    rateOut: metric(node.rateOut),
    spoolPercent: metric(node.diskUsage),
    spoolUsedBytes: numberAttribute(node, AttrSpoolUsedBytes),
    spoolMaxMb: numberAttribute(node, AttrSpoolMaxMb),
    brokerSpoolMaxMb: numberAttribute(node, AttrBrokerSpoolMaxMb),
    spoolMsgCount: numberAttribute(node, AttrSpoolMsgCount),
    maxConnections: numberAttribute(node, AttrMaxConnections),
    redundancyEnabled: node.attributes?.[AttrRedundancy] === "true",
  };
}

export function overview(view: ClusterOverview): SolaceOverview {
  return {
    msgVpn: attribute(view, AttrMsgVpn) ?? view.name,
    version: attribute(view, AttrVersion),
    queues: numberAttribute(view, AttrQueueCount) ?? metric(view.destinations),
    topicEndpoints: numberAttribute(view, AttrEndpointCount) ?? metric(view.subscriptions),
    clients: numberAttribute(view, AttrClientCount),
    spoolPercent: metric(view.avgDiskUsage),
    spoolUsedBytes: numberAttribute(view, AttrSpoolUsedBytes),
    spoolMaxMb: numberAttribute(view, AttrSpoolMaxMb),
    spoolMsgCount: numberAttribute(view, AttrSpoolMsgCount),
    msgVpnState: attribute(view, AttrVpnState),
  };
}

/** Whether the Message VPN this connection reads is actually serving. */
export function msgVpnServing(state: string | null): boolean {
  return state === "up";
}
