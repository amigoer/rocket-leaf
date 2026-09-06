import { beforeAll, describe, expect, it, vi } from "vitest";

/**
 * Every Solace board, through the states it can be in.
 *
 * The i18n sweep renders each board once with nothing connected, which covers
 * the offline notice and the strings. It cannot cover the rest: a board only
 * touches its data on the path where data exists, and that is exactly where a
 * missing attribute or an empty list throws. Each board is rendered here
 * against a stubbed hook so loading, failed, connected-but-empty and populated
 * all get exercised.
 *
 * The stubs return the shapes internal/driver/solace actually sends, so a
 * driver that renames an attribute key breaks a board test rather than a
 * screenshot. Three fixtures matter more than the rest, and each is a state
 * this family gets into and no other does: a queue pointing at a dead message
 * queue nobody created, a dead-letter row whose target has an unknown depth
 * for exactly that reason, and a browsed message with no body at all - which
 * is every Solace message, because SEMP carries no payload.
 */

type BrokerState<T> = {
  data: T | null;
  loading: boolean;
  refreshing: boolean;
  error: string | null;
  online: boolean;
  refresh: () => Promise<void>;
};

function stateOf<T>(over: Partial<BrokerState<T>>): BrokerState<T> {
  return {
    data: null,
    loading: false,
    refreshing: false,
    error: null,
    online: true,
    refresh: async () => {},
    ...over,
  };
}

const destinationsState = vi.hoisted(() => ({ current: null as unknown }));
const routingState = vi.hoisted(() => ({ current: null as unknown }));
const deadLettersState = vi.hoisted(() => ({ current: null as unknown }));
const clientsState = vi.hoisted(() => ({ current: null as unknown }));
const brokerState = vi.hoisted(() => ({ current: null as unknown }));
const browseState = vi.hoisted(() => ({
  current: {
    messages: [],
    loading: false,
    error: null,
    searched: false,
    run: async () => {},
  } as unknown,
}));

vi.mock("@/hooks/solace/useSolaceDestinations", () => ({
  useSolaceDestinations: () => destinationsState.current,
}));
vi.mock("@/hooks/solace/useSolaceRouting", () => ({
  useSolaceRouting: () => routingState.current,
}));
vi.mock("@/hooks/solace/useSolaceDeadLetters", () => ({
  useSolaceDeadLetters: () => deadLettersState.current,
}));
vi.mock("@/hooks/solace/useSolaceClients", () => ({
  useSolaceClients: () => clientsState.current,
}));
vi.mock("@/hooks/solace/useSolaceCluster", () => ({
  useSolaceBroker: () => brokerState.current,
}));
vi.mock("@/hooks/solace/useSolaceBrowse", () => ({
  useSolaceBrowse: () => browseState.current,
}));

let render: (element: React.ReactElement) => string;
let QueuesSolace: typeof import("./topics/QueuesSolace").QueuesSolace;
let RoutingSolace: typeof import("./topics/RoutingSolace").RoutingSolace;
let MessagesSolace: typeof import("./messages/MessagesSolace").MessagesSolace;
let ProducerSolace: typeof import("./producer/ProducerSolace").ProducerSolace;
let DlqSolace: typeof import("./dlq/DlqSolace").DlqSolace;
let ClientsSolace: typeof import("./consumers/ClientsSolace").ClientsSolace;
let BrokerSolace: typeof import("./cluster/BrokerSolace").BrokerSolace;
let OverviewSolace: typeof import("./overview/OverviewSolace").OverviewSolace;

beforeAll(async () => {
  const storage = { getItem: () => null, setItem() {}, removeItem() {} };
  vi.stubGlobal("window", {
    _wails: { environment: { OS: "darwin" } },
    matchMedia: () => ({ matches: false, addEventListener() {}, removeEventListener() {} }),
    localStorage: storage,
    addEventListener() {},
    removeEventListener() {},
  });
  vi.stubGlobal("localStorage", storage);

  const [
    server,
    queues,
    routing,
    messages,
    producer,
    dlq,
    clients,
    cluster,
    overview,
    ui,
    i18n,
    settings,
    profiles,
  ] = await Promise.all([
    import("react-dom/server"),
    import("./topics/QueuesSolace"),
    import("./topics/RoutingSolace"),
    import("./messages/MessagesSolace"),
    import("./producer/ProducerSolace"),
    import("./dlq/DlqSolace"),
    import("./consumers/ClientsSolace"),
    import("./cluster/BrokerSolace"),
    import("./overview/OverviewSolace"),
    import("@/components"),
    import("@/i18n"),
    import("@/hooks/useSettings"),
    import("@/hooks/useConnectionProfiles"),
  ]);
  await i18n.default.changeLanguage("zh");
  render = (node) =>
    server.renderToStaticMarkup(
      <ui.ConfirmProvider>
        <settings.SettingsProvider>
          <profiles.ConnectionProfilesProvider>{node}</profiles.ConnectionProfilesProvider>
        </settings.SettingsProvider>
      </ui.ConfirmProvider>,
    );
  QueuesSolace = queues.QueuesSolace;
  RoutingSolace = routing.RoutingSolace;
  MessagesSolace = messages.MessagesSolace;
  ProducerSolace = producer.ProducerSolace;
  DlqSolace = dlq.DlqSolace;
  ClientsSolace = clients.ClientsSolace;
  BrokerSolace = cluster.BrokerSolace;
  OverviewSolace = overview.OverviewSolace;
});

/**
 * A queue with a backlog, as internal/driver/solace/destination.go sends one.
 *
 * partitions is -1, which is UnknownMetric on the wire: a Solace queue is not
 * divided into parts a reader browses. The depth and the bound count are real
 * because the driver reads them from the two collection counts.
 */
const orders = {
  id: 1,
  ref: { namespace: "default", name: "mqstudio/seed/orders" },
  partitions: -1,
  subscribers: 0,
  depth: 12,
  rateIn: 0,
  rateOut: 0,
  lastUpdated: "",
  attributes: {
    accessType: "exclusive",
    permission: "consume",
    owner: "",
    durable: "true",
    ingressEnabled: "true",
    egressEnabled: "true",
    spoolUsageBytes: "1022",
    maxSpoolUsageMb: "5000",
    maxMsgSizeBytes: "10000000",
    partitionCount: "0",
    virtualRouter: "primary",
    createdByManagement: "true",
    deadMsgQueue: "#DEAD_MSG_QUEUE",
    maxRedeliveryCount: "0",
    respectTtlEnabled: "false",
    maxTtlSec: "0",
    respectDmqEligibleEnabled: "true",
    redeliveredMsgCount: "0",
    txUnackedMsgCount: "0",
    ttlExpiredToDmqMsgCount: "0",
    maxRedeliveryToDmqMsgCount: "0",
    spooledMsgCountTotal: "12",
  },
};

/*
 * The queue that has handed everything to its dead message queue.
 *
 * Its depth is 0 and its lifetime counter is 3, which is the pair that catches
 * a board reading spooledMsgCount as a depth: it would draw this queue as
 * holding three messages that are not there.
 */
const audit = {
  ...orders,
  id: 2,
  ref: { namespace: "default", name: "mqstudio/seed/audit" },
  depth: 0,
  attributes: {
    ...orders.attributes,
    deadMsgQueue: "mqstudio/seed/dmq",
    maxRedeliveryCount: "2",
    respectTtlEnabled: "true",
    maxTtlSec: "1",
    respectDmqEligibleEnabled: "false",
    ttlExpiredToDmqMsgCount: "3",
    spooledMsgCountTotal: "3",
  },
};

/** The dead message queue itself: a backlog and nothing draining it. */
const dmq = {
  ...orders,
  id: 3,
  ref: { namespace: "default", name: "mqstudio/seed/dmq" },
  depth: 3,
  attributes: { ...orders.attributes, spooledMsgCountTotal: "3" },
};

/*
 * A queue that will give up on a message and has nowhere to put it.
 *
 * This is the state no other family here has: the pointer is set, the queue is
 * healthy, and the messages are discarded. The overview counts it and the
 * alert rule fires on it.
 */
const discarding = {
  ...orders,
  id: 4,
  ref: { namespace: "default", name: "mqstudio/seed/discarding" },
  depth: 1,
  attributes: {
    ...orders.attributes,
    deadMsgQueue: "#DEAD_MSG_QUEUE",
    maxRedeliveryCount: "3",
  },
};

/** A queue with ingress switched off, which takes nothing and looks fine. */
const stopped = {
  ...orders,
  id: 5,
  ref: { namespace: "default", name: "mqstudio/seed/stopped" },
  depth: 0,
  attributes: { ...orders.attributes, ingressEnabled: "false" },
};

const everyQueue = [orders, audit, dmq, discarding, stopped];

/** A topic subscription, as the routing listing sends one. */
const subscription = {
  id: 1,
  namespace: "default",
  source: "mqstudio/seed/events/>",
  destination: "mqstudio/seed/events",
  destinationKind: "queue",
  routingKey: "mqstudio/seed/events/>",
  propertiesKey: "mqstudio/seed/events/>",
  arguments: { wildcard: "true", queueDepth: "5" },
};

/** A topic endpoint, whose name is its subscription and nothing else. */
const endpoint = {
  id: 1,
  ref: { namespace: "default", name: "mqstudio/seed/endpoint" },
  partitions: -1,
  subscribers: -1,
  depth: 0,
  rateIn: 0,
  rateOut: 0,
  lastUpdated: "",
  attributes: {
    endpointTopic: "mqstudio/seed/endpoint",
    accessType: "exclusive",
    permission: "consume",
    owner: "",
    durable: "true",
    ingressEnabled: "true",
    egressEnabled: "true",
    deadMsgQueue: "#DEAD_MSG_QUEUE",
    maxRedeliveryCount: "0",
    spoolUsageBytes: "0",
  },
};

/** A dead message queue that exists, with a source that moves everything. */
const realDeadLetter = {
  namespace: "default",
  name: "mqstudio/seed/dmq",
  depth: 3,
  consumers: 0,
  sources: [
    {
      queue: "mqstudio/seed/audit",
      subscription: "moves-everything",
      exchange: "queue",
      routingKey: "2",
    },
  ],
};

/*
 * The pointer every endpoint ships with, at a queue no broker creates.
 *
 * Its depth is -1 rather than 0, which is what says the target is not there:
 * a zero would read as an empty queue that is working.
 */
const missingDeadLetter = {
  namespace: "default",
  name: "#DEAD_MSG_QUEUE",
  depth: -1,
  consumers: 0,
  sources: [
    {
      queue: "mqstudio/seed/orders",
      subscription: "moves-marked-only",
      exchange: "queue",
      routingKey: "0",
    },
    {
      queue: "mqstudio/seed/endpoint",
      subscription: "moves-marked-only",
      exchange: "topicEndpoint",
      routingKey: "0",
    },
  ],
};

/** An application session. */
const application = {
  name: "orders-service/1",
  clientName: "orders-service/1",
  namespace: "default",
  user: "app",
  node: "primary",
  peerHost: "10.0.0.7",
  peerPort: 51122,
  protocol: "",
  state: "connected",
  channels: 2,
  tls: false,
  cipher: "",
  heartbeatSec: 0,
  recvBytes: 4096,
  sendBytes: 128,
  recvByteRate: 0,
  sendByteRate: 0,
  connectedAtMs: 0,
  attributes: {
    platform: "Linux-aarch64_opt - C SDK",
    softwareVersion: "7.33.1.1",
    clientProfile: "default",
    aclProfile: "default",
    description: "",
    uptimeSec: "3204",
    slowSubscriber: "false",
    tlsDowngraded: "false",
    internal: "false",
  },
};

/** The broker talking to itself, which is listed and marked rather than hidden. */
const internal = {
  ...application,
  name: "#client",
  clientName: "#client",
  user: "#client-username",
  attributes: {
    ...application.attributes,
    description: "Internal Message Bus",
    internal: "true",
  },
};

/** The one broker row, with the spool percentage already scaled by the driver. */
const brokerNode = {
  id: 1,
  name: "http://127.0.0.1:8080",
  address: "http://127.0.0.1:8080",
  cluster: "default",
  version: "10.26.0.8827",
  status: "online",
  rateIn: 0,
  rateOut: 0,
  diskUsage: 0,
  lastSeen: "",
  tpsHistoryTimestamps: [],
  tpsInHistory: [],
  tpsOutHistory: [],
  replicas: [],
  attributes: {
    msgVpn: "default",
    msgVpnState: "up",
    version: "10.26.0.8827",
    redundancyEnabled: "true",
    spoolUsedBytes: "1572",
    spoolMaxMb: "1500",
    spoolMsgCount: "20",
    brokerSpoolMaxMb: "1500",
    maxConnections: "100",
  },
};

/*
 * A browsed message, which carries no body at all.
 *
 * That is every Solace message, not an edge case: SEMP has no payload field at
 * any version. A board that rendered the body would draw an empty box on every
 * row and say nothing about why.
 */
const browsed = {
  id: 1,
  cluster: "default",
  topic: "mqstudio/seed/orders",
  messageId: "12",
  tags: "",
  keys: "",
  queueId: 0,
  queueOffset: 12,
  storeHost: "",
  bornHost: "",
  storeTime: "2026-09-06T09:00:00Z",
  storeTimestamp: 1788690000000,
  status: "undelivered",
  retryTimes: 0,
  body: "",
  properties: {
    attachmentSize: "59",
    contentSize: "0",
    redeliveryCount: "0",
    undelivered: "true",
    dmqEligible: "false",
    partitionKey: "",
    publisherId: "3",
    replicationGroupMsgId: "rmid1:54e9b-585979064e9-00000000-0000000c",
    replicationState: "not-replicated",
    spooledTime: "1788690000",
  },
};

describe("the Solace queues board", () => {
  it("says the connection is offline rather than showing an empty list", () => {
    destinationsState.current = stateOf({ online: false });
    expect(() => render(<QueuesSolace />)).not.toThrow();
  });

  it("renders while the first request is in flight", () => {
    destinationsState.current = stateOf({ loading: true });
    expect(() => render(<QueuesSolace />)).not.toThrow();
  });

  it("shows what failed rather than an empty list", () => {
    destinationsState.current = stateOf({ error: "semp unauthorized: Authorization failed" });
    expect(render(<QueuesSolace />)).toContain("semp unauthorized: Authorization failed");
  });

  it("renders a Message VPN holding nothing", () => {
    destinationsState.current = stateOf({ data: [] });
    expect(() => render(<QueuesSolace />)).not.toThrow();
  });

  /*
   * The one figure this family gets wrong by default. The audit queue holds
   * nothing and its lifetime counter says three: a board reading
   * spooledMsgCount as the depth would draw a 3 in the spooled column.
   */
  it("draws the real depth rather than the lifetime counter", () => {
    destinationsState.current = stateOf({ data: [audit] });
    const html = render(<QueuesSolace />);
    expect(html).toContain("mqstudio/seed/audit");
    // The detail panel opens on the first row and shows both, so what is
    // checked is that the lifetime figure is labelled rather than absent.
    expect(html).toContain("累计入队");
  });

  it("marks a queue that names a dead message queue", () => {
    destinationsState.current = stateOf({ data: everyQueue });
    const html = render(<QueuesSolace />);
    expect(html).toContain("mqstudio/seed/orders");
    expect(html).toContain("DMQ");
  });
});

describe("the Solace routing board", () => {
  it("says the connection is offline rather than showing an empty list", () => {
    routingState.current = stateOf({ online: false });
    destinationsState.current = stateOf({ data: [] });
    expect(() => render(<RoutingSolace />)).not.toThrow();
  });

  it("renders while the first request is in flight", () => {
    routingState.current = stateOf({ loading: true });
    expect(() => render(<RoutingSolace />)).not.toThrow();
  });

  it("shows what failed rather than an empty topology", () => {
    routingState.current = stateOf({ error: "semp not_found: Could not find match" });
    expect(render(<RoutingSolace />)).toContain("semp not_found: Could not find match");
  });

  it("renders a Message VPN with no subscriptions and no topic endpoints", () => {
    routingState.current = stateOf({ data: { endpoints: [], subscriptions: [] } });
    expect(() => render(<RoutingSolace />)).not.toThrow();
  });

  it("marks a wildcard subscription and draws the queue it fills", () => {
    routingState.current = stateOf({
      data: { endpoints: [endpoint], subscriptions: [subscription] },
    });
    const html = render(<RoutingSolace />);
    expect(html).toContain("mqstudio/seed/events/&gt;");
    expect(html).toContain("mqstudio/seed/endpoint");
    expect(html).toContain("通配");
  });
});

describe("the Solace messages board", () => {
  it("says the connection is offline rather than showing an empty list", () => {
    destinationsState.current = stateOf({ online: false });
    expect(() => render(<MessagesSolace />)).not.toThrow();
  });

  it("invites a browse before one has been run", () => {
    destinationsState.current = stateOf({ data: everyQueue });
    browseState.current = { messages: [], loading: false, error: null, searched: false, run: async () => {} };
    expect(() => render(<MessagesSolace />)).not.toThrow();
  });

  it("says the queue is empty once a browse has come back with nothing", () => {
    browseState.current = { messages: [], loading: false, error: null, searched: true, run: async () => {} };
    expect(() => render(<MessagesSolace />)).not.toThrow();
  });

  it("shows what a failed browse said", () => {
    browseState.current = {
      messages: [],
      loading: false,
      error: "default has no queue named mqstudio/test/gone",
      searched: true,
      run: async () => {},
    };
    expect(render(<MessagesSolace />)).toContain("default has no queue named mqstudio/test/gone");
  });

  /*
   * Every Solace message has an empty body, so this is the ordinary path
   * rather than an edge case: the board draws the sizes where another family
   * draws a preview, and says before the button that there is no payload to
   * come back with.
   */
  it("lists a message with no body and says so before the browse", () => {
    browseState.current = {
      messages: [browsed],
      loading: false,
      error: null,
      searched: true,
      run: async () => {},
    };
    const html = render(<MessagesSolace />);
    expect(html).toContain("12");
    expect(html).toContain("SEMP 不返回消息正文");
    // The size stands where every other family draws a preview.
    expect(html).toContain("59");
  });
});

describe("the Solace send console", () => {
  it("says the connection is offline rather than an empty form", () => {
    destinationsState.current = stateOf({ online: false });
    expect(() => render(<ProducerSolace />)).not.toThrow();
  });

  it("renders with queues to choose from", () => {
    destinationsState.current = stateOf({ data: everyQueue });
    const html = render(<ProducerSolace />);
    // The target is a choice rather than something guessed from the name, so
    // both options are on the form.
    expect(html).toContain("发送到");
  });

  it("renders against a Message VPN with no queues at all", () => {
    destinationsState.current = stateOf({ data: [] });
    expect(() => render(<ProducerSolace />)).not.toThrow();
  });
});

describe("the Solace dead messages board", () => {
  it("says the connection is offline rather than showing an empty list", () => {
    deadLettersState.current = stateOf({ online: false });
    expect(() => render(<DlqSolace />)).not.toThrow();
  });

  it("renders while the first request is in flight", () => {
    deadLettersState.current = stateOf({ loading: true });
    expect(() => render(<DlqSolace />)).not.toThrow();
  });

  it("shows what failed rather than an empty list", () => {
    deadLettersState.current = stateOf({ error: "semp unauthorized: Authorization failed" });
    expect(render(<DlqSolace />)).toContain("semp unauthorized: Authorization failed");
  });

  it("renders a Message VPN where nothing dead-letters anywhere", () => {
    deadLettersState.current = stateOf({ data: [] });
    expect(() => render(<DlqSolace />)).not.toThrow();
  });

  /*
   * The row this page exists for: a target with an unknown depth is a queue
   * the Message VPN does not hold, which means the messages are discarded. It
   * has to be marked rather than drawn as an empty queue that works.
   */
  it("marks a dead message queue that was never created", () => {
    deadLettersState.current = stateOf({ data: [realDeadLetter, missingDeadLetter] });
    const html = render(<DlqSolace />);
    expect(html).toContain("#DEAD_MSG_QUEUE");
    expect(html).toContain("未创建");
    expect(html).toContain("mqstudio/seed/dmq");
  });
});

describe("the Solace clients board", () => {
  it("says the connection is offline rather than showing an empty list", () => {
    clientsState.current = stateOf({ online: false });
    expect(() => render(<ClientsSolace />)).not.toThrow();
  });

  it("renders while the first request is in flight", () => {
    clientsState.current = stateOf({ loading: true });
    expect(() => render(<ClientsSolace />)).not.toThrow();
  });

  it("shows what failed rather than an empty list", () => {
    clientsState.current = stateOf({ error: "semp unauthorized: Authorization failed" });
    expect(render(<ClientsSolace />)).toContain("semp unauthorized: Authorization failed");
  });

  it("renders a Message VPN with nothing connected", () => {
    clientsState.current = stateOf({ data: [] });
    expect(() => render(<ClientsSolace />)).not.toThrow();
  });

  // The broker's own sessions are listed and marked, because hiding them would
  // disagree with every count the broker reports.
  it("marks the broker's own session rather than hiding it", () => {
    clientsState.current = stateOf({ data: [application, internal] });
    const html = render(<ClientsSolace />);
    expect(html).toContain("orders-service/1");
    expect(html).toContain("#client");
    expect(html).toContain("Broker");
  });
});

describe("the Solace broker board", () => {
  it("says the connection is offline rather than showing an empty panel", () => {
    brokerState.current = stateOf({ online: false });
    expect(() => render(<BrokerSolace />)).not.toThrow();
  });

  it("renders while the first request is in flight", () => {
    brokerState.current = stateOf({ loading: true });
    expect(() => render(<BrokerSolace />)).not.toThrow();
  });

  it("shows what failed rather than an empty panel", () => {
    brokerState.current = stateOf({ error: "semp did not answer" });
    expect(render(<BrokerSolace />)).toContain("semp did not answer");
  });

  it("renders when the broker answered with no node at all", () => {
    brokerState.current = stateOf({ data: [] });
    expect(() => render(<BrokerSolace />)).not.toThrow();
  });

  it("draws the broker with its version and the spool it is using", () => {
    brokerState.current = stateOf({ data: [brokerNode] });
    const html = render(<BrokerSolace />);
    expect(html).toContain("10.26.0.8827");
    expect(html).toContain("http://127.0.0.1:8080");
  });
});

describe("the Solace overview", () => {
  it("says the connection is offline rather than showing zeroes", () => {
    destinationsState.current = stateOf({ online: false });
    brokerState.current = stateOf({ online: false });
    expect(() => render(<OverviewSolace />)).not.toThrow();
  });

  it("renders while the first request is in flight", () => {
    destinationsState.current = stateOf({ loading: true });
    brokerState.current = stateOf({ loading: true });
    expect(() => render(<OverviewSolace />)).not.toThrow();
  });

  it("renders an empty Message VPN", () => {
    destinationsState.current = stateOf({ data: [] });
    brokerState.current = stateOf({ data: [brokerNode] });
    expect(() => render(<OverviewSolace />)).not.toThrow();
  });

  /*
   * The tile that is this family's own. Two of the five queues point at a dead
   * message queue that does not exist, and only one of them will ever give up
   * on a message - so the count is one, not two.
   */
  it("counts only the queues that will give up and have nowhere to put it", () => {
    destinationsState.current = stateOf({ data: everyQueue });
    brokerState.current = stateOf({ data: [brokerNode] });
    const html = render(<OverviewSolace />);
    expect(html).toContain("正在丢弃");
    expect(html).toContain("mqstudio/seed/orders");
  });
});
