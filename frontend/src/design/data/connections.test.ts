import { describe, expect, it } from "vitest";
import { MQKind } from "@bindings/model/models";
import { protocolOfKind, toShellConnection } from "./connections";
import { PROTOCOL_ORDER, isProtocolReady } from "./protocols";
import type { Connection as ConnectionProfile } from "@/api/models";

/**
 * Every protocol the picker draws has to map back from a stored kind.
 *
 * This is a round trip with two halves in two files, and nothing tied them
 * together: NewConnectionDialog writes a profile whose kind comes from
 * connectionDraft, and the connections list reads it back through
 * protocolOfKind. A protocol missing from the second half still creates a
 * connection - and the row comes back with no icon, its label falls back to
 * the raw kind string, and it cannot be double-clicked to open a tab, because
 * all three are gated on the protocol being known.
 *
 * That is exactly what shipped for ActiveMQ until somebody opened the app and
 * looked at the row, which is the failure this test exists to make loud.
 */
describe("the kind-to-protocol map", () => {
  it("resolves every protocol the picker can create a connection for", () => {
    const reachable = new Set(
      Object.values(MQKind)
        .map((kind) => protocolOfKind(kind as MQKind))
        .filter((protocol) => protocol != null),
    );
    const missing = PROTOCOL_ORDER.filter(
      (protocol) => isProtocolReady(protocol) && !reachable.has(protocol),
    );
    expect(missing).toEqual([]);
  });

  /*
   * And the other direction: a kind that maps to a protocol the shell does not
   * draw would send the list looking for boards that do not exist.
   */
  it("maps no kind to a protocol the shell cannot draw", () => {
    const drawn = new Set<string>(PROTOCOL_ORDER);
    for (const kind of Object.values(MQKind)) {
      const protocol = protocolOfKind(kind as MQKind);
      if (protocol != null) expect(drawn.has(protocol), `${kind} → ${protocol}`).toBe(true);
    }
  });
});

/**
 * What the address column shows for a family that has no address.
 *
 * Every other protocol's row prints the profile's endpoints, and an SQS
 * profile's is deliberately empty - there is nothing to dial. Printed as-is
 * that leaves the column blank on a perfectly good connection, and two SQS
 * connections to different regions look identical.
 */
describe("the address a connection row shows", () => {
  const profileOf = (extra: Partial<ConnectionProfile>) =>
    ({
      id: 1,
      name: "orders",
      group: "",
      endpoints: "",
      timeoutSec: 5,
      status: "offline",
      lastCheck: "",
      isDefault: false,
      remark: "",
      options: {},
      ...extra,
    }) as unknown as ConnectionProfile;

  it("shows the region for SQS, which has no address at all", () => {
    const row = toShellConnection(
      profileOf({ kind: MQKind.KindSQS, options: { region: "eu-west-1" } }),
    );
    expect(row.address).toBe("eu-west-1");
    expect(row.protocol).toBe("sqs");
  });

  it("still shows the endpoints for a family that dials one", () => {
    const row = toShellConnection(
      profileOf({ kind: MQKind.KindRocketMQ, endpoints: "10.0.0.1:9876" }),
    );
    expect(row.address).toBe("10.0.0.1:9876");
  });
});
