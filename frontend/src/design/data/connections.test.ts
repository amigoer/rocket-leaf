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
 * What the address column shows for the families whose row is not simply
 * their endpoints, which do not agree with each other.
 *
 * Every other protocol's row prints the profile's endpoints, and an SQS,
 * Kinesis or Pub/Sub profile's is deliberately empty - there is nothing to
 * dial. Printed as-is that leaves the column blank on a perfectly good
 * connection, and two connections to different regions, or different projects,
 * look identical.
 *
 * Service Bus is the hosted family that does dial something, so its row prints
 * endpoints like any other - normalised, because the field takes three
 * spellings of one namespace and the row should show the host the driver
 * reaches rather than whichever was typed.
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

  it("shows the region for Kinesis, the second family with no address", () => {
    const row = toShellConnection(
      profileOf({ kind: MQKind.KindKinesis, options: { region: "us-east-1" } }),
    );
    expect(row.address).toBe("us-east-1");
    expect(row.protocol).toBe("kinesis");
  });

  /*
   * IBM MQ has a real address and it is still not enough on its own: one mqweb
   * server can front several queue managers, they share nothing, and two
   * profiles pointed at two of them would otherwise print the same row.
   */
  it("shows the mqweb server with the queue manager for IBM MQ", () => {
    const row = toShellConnection(
      profileOf({
        kind: MQKind.KindIBMMQ,
        endpoints: "https://mq.example:9443",
        options: { queueManager: "QM1" },
      }),
    );
    expect(row.address).toBe("https://mq.example:9443/QM1");
    expect(row.protocol).toBe("ibmmq");
  });

  // A profile that named none is asking the driver to discover the only one
  // the server fronts, so the row prints what the profile says and no more.
  it("shows only the server when an IBM MQ profile names no queue manager", () => {
    const row = toShellConnection(
      profileOf({ kind: MQKind.KindIBMMQ, endpoints: "https://mq.example:9443" }),
    );
    expect(row.address).toBe("https://mq.example:9443");
  });

  it("shows the project for Pub/Sub, which has no address either", () => {
    const row = toShellConnection(
      profileOf({ kind: MQKind.KindGooglePubSub, options: { projectId: "orders-prod" } }),
    );
    expect(row.address).toBe("orders-prod");
    expect(row.protocol).toBe("google-pubsub");
  });

  it("shows the namespace for Service Bus, which is the hosted family with an address", () => {
    const row = toShellConnection(
      profileOf({
        kind: MQKind.KindAzureServiceBus,
        endpoints: "orders.servicebus.windows.net",
      }),
    );
    expect(row.address).toBe("orders.servicebus.windows.net");
    expect(row.protocol).toBe("azure-servicebus");
  });

  // The three spellings a user actually pastes, all naming one namespace. The
  // Go half is internal/driver/azureservicebus's TestNamespaceOfReadsWhateverWasPasted.
  it("normalises whatever was pasted into the Service Bus address field", () => {
    const address = (endpoints: string) =>
      toShellConnection(profileOf({ kind: MQKind.KindAzureServiceBus, endpoints })).address;

    expect(address("sb://orders.servicebus.windows.net/")).toBe("orders.servicebus.windows.net");
    expect(address("  orders.servicebus.windows.net  ")).toBe("orders.servicebus.windows.net");
    // The emulator's port is part of its address and has to survive.
    expect(address("localhost:5672")).toBe("localhost:5672");
  });

  it("still shows the endpoints for a family that dials one", () => {
    const row = toShellConnection(
      profileOf({ kind: MQKind.KindRocketMQ, endpoints: "10.0.0.1:9876" }),
    );
    expect(row.address).toBe("10.0.0.1:9876");
  });
});
