/**
 * Solace's view of a connected client.
 *
 * The keys are a contract with internal/driver/solace/clients.go.
 *
 * Two fields carry more than their names suggest. `internal` marks the broker
 * talking to itself - the message bus, the REST listener's own session - which
 * is listed rather than hidden because those hold real resources, and marked
 * because a reader counting applications has to be able to leave them out.
 * `slowSubscriber` is the broker's own verdict rather than a threshold this
 * app applied: it is set when a client cannot keep up with what is being
 * pushed to it, and it is the field that explains an egress rate that has
 * stopped climbing.
 */
import type { ClientConnection } from "@bindings/model/models";

const AttrPlatform = "platform";
const AttrSoftwareVersion = "softwareVersion";
const AttrClientProfile = "clientProfile";
const AttrACLProfile = "aclProfile";
const AttrDescription = "description";
const AttrUptimeSec = "uptimeSec";
const AttrSlowSubscriber = "slowSubscriber";
const AttrTLSDowngraded = "tlsDowngraded";
const AttrInternal = "internal";

export interface SolaceClient {
  /** The broker's own name for the session, and the key. */
  name: string;
  address: string;
  username: string;
  msgVpn: string;
  /** The client library and the machine it is running on, as it reported them. */
  platform: string | null;
  softwareVersion: string | null;
  clientProfile: string | null;
  aclProfile: string | null;
  description: string | null;
  uptimeSec: number | null;
  /** The broker's verdict that this client cannot keep up. */
  slowSubscriber: boolean;
  /** A TLS session the broker allowed to fall back to plain text. */
  tlsDowngraded: boolean;
  /** The broker talking to itself rather than an application. */
  internal: boolean;
  /** Endpoints it has bound to, which are flows rather than channels. */
  binds: number;
  recvBytes: number;
  sendBytes: number;
}

function attribute(client: ClientConnection, key: string): string | null {
  const value = client.attributes?.[key];
  return value == null || value === "" ? null : value;
}

function flag(client: ClientConnection, key: string): boolean {
  return client.attributes?.[key] === "true";
}

export function client(row: ClientConnection): SolaceClient {
  const uptime = attribute(row, AttrUptimeSec);
  const parsed = uptime == null ? Number.NaN : Number(uptime);
  return {
    name: row.name,
    address: row.peerPort > 0 ? `${row.peerHost}:${row.peerPort}` : row.peerHost,
    username: row.user,
    msgVpn: row.namespace,
    platform: attribute(row, AttrPlatform),
    softwareVersion: attribute(row, AttrSoftwareVersion),
    clientProfile: attribute(row, AttrClientProfile),
    aclProfile: attribute(row, AttrACLProfile),
    description: attribute(row, AttrDescription),
    uptimeSec: Number.isFinite(parsed) ? parsed : null,
    slowSubscriber: flag(row, AttrSlowSubscriber),
    tlsDowngraded: flag(row, AttrTLSDowngraded),
    internal: flag(row, AttrInternal),
    binds: row.channels,
    recvBytes: row.recvBytes,
    sendBytes: row.sendBytes,
  };
}

/** The applications, which is the list without the broker's own sessions. */
export function applications(clients: readonly SolaceClient[]): SolaceClient[] {
  return clients.filter((entry) => !entry.internal);
}
