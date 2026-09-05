import { beforeAll, describe, expect, it, vi } from "vitest";

/**
 * Every NSQ board, through the states it can be in.
 *
 * The i18n sweep renders each board once with nothing connected, which covers
 * the offline notice and the strings. It cannot cover the rest: a board only
 * touches its data on the path where data exists, and that is exactly where a
 * missing attribute or an empty list throws. Each board is rendered here
 * against a stubbed hook so loading, failed, connected-but-empty and populated
 * all get exercised.
 *
 * The stubs return the shapes internal/driver/nsq actually sends, so a driver
 * that renames an attribute key breaks a board test rather than a screenshot.
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

const topicsState = vi.hoisted(() => ({ current: null as unknown }));
const channelsState = vi.hoisted(() => ({ current: null as unknown }));
const clusterState = vi.hoisted(() => ({ current: null as unknown }));
const configState = vi.hoisted(() => ({ current: null as unknown }));
const clientsState = vi.hoisted(() => ({ current: null as unknown }));
const nodesState = vi.hoisted(() => ({ current: null as unknown }));

vi.mock("@/hooks/nsq/useNsqDestinations", () => ({
  useNsqDestinations: () => topicsState.current,
}));
vi.mock("@/hooks/nsq/useNsqSubscriptions", () => ({
  useNsqSubscriptions: () => channelsState.current,
}));
vi.mock("@/hooks/nsq/useNsqCluster", () => ({
  useNsqCluster: () => clusterState.current,
  useNsqNodeConfig: () => configState.current,
  useNsqConnections: () => clientsState.current,
}));
vi.mock("@/hooks/nsq/useNsqNodes", () => ({
  useNsqNodes: () => nodesState.current,
}));

let render: (element: React.ReactElement) => string;
let TopicsNsq: typeof import("./topics/TopicsNsq").TopicsNsq;
let ChannelsNsq: typeof import("./consumers/ChannelsNsq").ChannelsNsq;
let ClientsNsq: typeof import("./consumers/ClientsNsq").ClientsNsq;
let ClusterNsq: typeof import("./cluster/ClusterNsq").ClusterNsq;
let ProducerNsq: typeof import("./producer/ProducerNsq").ProducerNsq;
let OverviewNsq: typeof import("./overview/OverviewNsq").OverviewNsq;

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

  const [server, topics, channels, clients, cluster, producer, overview, ui, i18n, settings] =
    await Promise.all([
      import("react-dom/server"),
      import("./topics/TopicsNsq"),
      import("./consumers/ChannelsNsq"),
      import("./consumers/ClientsNsq"),
      import("./cluster/ClusterNsq"),
      import("./producer/ProducerNsq"),
      import("./overview/OverviewNsq"),
      import("@/components"),
      import("@/i18n"),
      import("@/hooks/useSettings"),
    ]);
  await i18n.default.changeLanguage("zh");
  render = (node) =>
    server.renderToStaticMarkup(
      <ui.ConfirmProvider>
        <settings.SettingsProvider>{node}</settings.SettingsProvider>
      </ui.ConfirmProvider>,
    );
  TopicsNsq = topics.TopicsNsq;
  ChannelsNsq = channels.ChannelsNsq;
  ClientsNsq = clients.ClientsNsq;
  ClusterNsq = cluster.ClusterNsq;
  ProducerNsq = producer.ProducerNsq;
  OverviewNsq = overview.OverviewNsq;
});

/** A topic on both daemons, as internal/driver/nsq/destination.go sends one. */
const orders = {
  id: 1,
  ref: { namespace: "", name: "MQS.SEED.orders" },
  partitions: -1,
  subscribers: 2,
  depth: 320,
  rateIn: -1,
  rateOut: -1,
  lastUpdated: "",
  attributes: {
    paused: "false",
    topicDepth: "0",
    channelDepth: "320",
    backendDepth: "0",
    messageCount: "163",
    messageBytes: "4096",
    inFlight: "0",
    deferred: "3",
    requeued: "0",
    timedOut: "0",
    ephemeral: "false",
    nodes: "127.0.0.1:4151,127.0.0.1:4153",
    channels: "analytics,archive",
  },
};

/** The other shape: paused, holding its own messages, on one daemon. */
const held = {
  id: 2,
  ref: { namespace: "", name: "MQS.SEED.paused" },
  partitions: -1,
  subscribers: 1,
  depth: 5,
  rateIn: -1,
  rateOut: -1,
  lastUpdated: "",
  attributes: {
    paused: "true",
    topicDepth: "5",
    channelDepth: "0",
    backendDepth: "0",
    messageCount: "5",
    messageBytes: "40",
    inFlight: "0",
    deferred: "0",
    requeued: "0",
    timedOut: "0",
    ephemeral: "false",
    nodes: "127.0.0.1:4151",
    channels: "analytics",
  },
};

const analytics = {
  id: 1,
  ref: { namespace: "MQS.SEED.orders", name: "analytics" },
  status: "offline",
  members: 0,
  destinations: 1,
  backlog: 160,
  rateOut: -1,
  lastUpdated: "",
  attributes: {
    topic: "MQS.SEED.orders",
    paused: "false",
    inFlight: "0",
    deferred: "3",
    requeued: "0",
    timedOut: "0",
    messageCount: "163",
    backendDepth: "0",
    ephemeral: "false",
    nodes: "127.0.0.1:4151,127.0.0.1:4153",
  },
};

/** A paused channel, which is the state the boards have to name rather than
    show as an idle one. */
const pausedChannel = {
  ...analytics,
  id: 2,
  ref: { namespace: "MQS.SEED.audit", name: "analytics" },
  status: "warning",
  members: 1,
  backlog: 12,
  attributes: { ...analytics.attributes, topic: "MQS.SEED.audit", paused: "true" },
};

const nsqd = {
  id: 1,
  name: "nsqd",
  address: "127.0.0.1:4151",
  cluster: "",
  version: "1.3.0",
  status: "online",
  rateIn: -1,
  rateOut: -1,
  diskUsage: -1,
  lastSeen: "2026-09-06T00:00:00Z",
  attributes: {
    hostname: "nsqd",
    broadcastAddress: "127.0.0.1",
    tcpPort: "4150",
    httpPort: "4151",
    startTime: "1788644706",
    health: "OK",
    topicCount: "4",
    channelCount: "3",
    clientCount: "1",
    depth: "332",
    heapInUseBytes: "2498560",
    heapObjects: "2753",
    gcTotalRuns: "25",
  },
};

const lookupd = {
  id: 2,
  name: "127.0.0.1:4161",
  address: "127.0.0.1:4161",
  cluster: "",
  version: "1.3.0",
  status: "online",
  rateIn: -1,
  rateOut: -1,
  diskUsage: -1,
  lastSeen: "",
  attributes: {
    producerCount: "2",
    directoryTopics: "4",
    nodes: "127.0.0.1:4151,127.0.0.1:4153",
  },
};

const consumer = {
  name: "172.17.0.5:52344 -> 127.0.0.1:4151",
  clientName: "watcher",
  namespace: "",
  user: "",
  node: "127.0.0.1:4151",
  peerHost: "172.17.0.5",
  peerPort: 52344,
  protocol: "nsq V2",
  state: "subscribed",
  channels: 1,
  tls: false,
  cipher: "",
  heartbeatSec: 0,
  recvBytes: 0,
  sendBytes: 0,
  recvByteRate: 0,
  sendByteRate: 0,
  connectedAtMs: 1788644706000,
  blockedBy: "",
  attributes: {
    topic: "MQS.SEED.events",
    channel: "watchers",
    readyCount: "0",
    inFlight: "0",
    messageCount: "0",
    finishCount: "0",
    requeued: "0",
    userAgent: "nsq_tail/1.3.0 go-nsq/1.1.0",
    hostname: "consumer",
    snappy: "false",
    node: "127.0.0.1:4151",
  },
};

describe("the NSQ topics board", () => {
  it("says the connection is offline rather than showing an empty list", () => {
    topicsState.current = stateOf({ online: false });
    expect(() => render(<TopicsNsq />)).not.toThrow();
  });

  it("renders while the first request is in flight", () => {
    topicsState.current = stateOf({ loading: true });
    expect(() => render(<TopicsNsq />)).not.toThrow();
  });

  it("shows what failed rather than an empty list", () => {
    topicsState.current = stateOf({ error: "no nsqd answered" });
    expect(render(<TopicsNsq />)).toContain("no nsqd answered");
  });

  it("renders a cluster that has declared no topics", () => {
    topicsState.current = stateOf({ data: [] });
    expect(() => render(<TopicsNsq />)).not.toThrow();
  });

  it("lists a topic with its channels and the daemons carrying it", () => {
    topicsState.current = stateOf({ data: [orders, held] });
    const html = render(<TopicsNsq />);
    expect(html).toContain("MQS.SEED.orders");
    expect(html).toContain("MQS.SEED.paused");
  });

  /*
   * A topic depth above zero is the state a reader has to be told about: the
   * messages reached no channel, so no consumer will ever be offered them.
   * The detail panel is also where every attribute is read, which is the path
   * a renamed key throws on.
   */
  it("explains a paused topic holding its own messages", () => {
    topicsState.current = stateOf({ data: [held] });
    const html = render(<TopicsNsq />);
    expect(html).toContain("5");
    expect(() => render(<TopicsNsq />)).not.toThrow();
  });
});

describe("the NSQ channels board", () => {
  it("says the connection is offline rather than showing an empty list", () => {
    channelsState.current = stateOf({ online: false });
    expect(() => render(<ChannelsNsq />)).not.toThrow();
  });

  it("renders a cluster with no channels at all", () => {
    channelsState.current = stateOf({ data: [] });
    expect(() => render(<ChannelsNsq />)).not.toThrow();
  });

  /*
   * The topic is half a channel's identity, so it has to be on the row: the
   * same channel name under two topics is two channels with separate
   * backlogs, and a board showing only the name would read as a duplicate.
   */
  it("lists a channel with the topic it belongs to", () => {
    channelsState.current = stateOf({ data: [analytics, pausedChannel] });
    const html = render(<ChannelsNsq />);
    expect(html).toContain("analytics");
    expect(html).toContain("MQS.SEED.orders");
    expect(html).toContain("MQS.SEED.audit");
  });

  it("renders the detail panel for a paused channel with every attribute set", () => {
    channelsState.current = stateOf({ data: [pausedChannel] });
    expect(() => render(<ChannelsNsq />)).not.toThrow();
  });
});

describe("the NSQ clients board", () => {
  it("says the connection is offline rather than showing an empty list", () => {
    clientsState.current = stateOf({ online: false });
    expect(() => render(<ClientsNsq />)).not.toThrow();
  });

  it("renders a cluster with nothing consuming", () => {
    clientsState.current = stateOf({ data: [] });
    expect(() => render(<ClientsNsq />)).not.toThrow();
  });

  /*
   * The ready count is why this page exists. A consumer that has told nsqd it
   * will accept nothing is connected, holding its channel, and taking no work
   * - and nothing else in the app can see it.
   */
  it("marks a consumer that is asking for nothing", () => {
    clientsState.current = stateOf({ data: [consumer] });
    const html = render(<ClientsNsq />);
    expect(html).toContain("MQS.SEED.events");
    expect(html).toContain("watchers");
    expect(html).toContain("172.17.0.5:52344");
  });
});

describe("the NSQ cluster board", () => {
  it("says the connection is offline rather than showing an empty list", () => {
    clusterState.current = stateOf({ online: false });
    configState.current = stateOf({});
    expect(() => render(<ClusterNsq />)).not.toThrow();
  });

  it("draws both tiers with their own figures", () => {
    clusterState.current = stateOf({
      data: {
        overview: {
          name: "",
          totalNodes: 1,
          onlineNodes: 1,
          destinations: 4,
          subscriptions: 3,
          avgDiskUsage: -1,
          attributes: { depth: "332", clientCount: "1", directory: "1" },
        },
        nodes: [nsqd],
        directory: [lookupd],
      },
    });
    configState.current = stateOf({ data: { version: "1.3.0" } });
    const html = render(<ClusterNsq />);
    expect(html).toContain("127.0.0.1:4151");
    expect(html).toContain("127.0.0.1:4161");
  });

  /*
   * The one thing this page warns about. nsqlookupd hands a consumer whatever
   * each nsqd broadcast about itself, so a daemon advertised at an address the
   * app cannot reach sends every consumer using discovery somewhere it cannot
   * go - and no other page can see it.
   */
  it("warns when the directory advertises an address the connection cannot reach", () => {
    clusterState.current = stateOf({
      data: {
        overview: {
          name: "",
          totalNodes: 1,
          onlineNodes: 1,
          destinations: 0,
          subscriptions: 0,
          avgDiskUsage: -1,
          attributes: {},
        },
        nodes: [nsqd],
        directory: [
          { ...lookupd, attributes: { ...lookupd.attributes, nodes: "nsqd-container:4151" } },
        ],
      },
    });
    configState.current = stateOf({});
    expect(render(<ClusterNsq />)).toContain("nsqd-container:4151");
  });

  it("explains a connection that names no discovery tier", () => {
    clusterState.current = stateOf({
      data: {
        overview: {
          name: "",
          totalNodes: 1,
          onlineNodes: 1,
          destinations: 0,
          subscriptions: 0,
          avgDiskUsage: -1,
          attributes: {},
        },
        nodes: [nsqd],
        directory: [],
      },
    });
    configState.current = stateOf({});
    expect(() => render(<ClusterNsq />)).not.toThrow();
  });
});

describe("the NSQ send console", () => {
  it("renders with nothing connected", () => {
    topicsState.current = stateOf({ online: false });
    nodesState.current = stateOf({});
    expect(() => render(<ProducerNsq />)).not.toThrow();
  });

  /*
   * The node picker is the field no other console here has, and it needs the
   * daemons the connection holds rather than the ones discovery knows about.
   */
  it("offers the daemons the connection names", () => {
    topicsState.current = stateOf({ data: [orders] });
    nodesState.current = stateOf({ data: ["127.0.0.1:4151", "127.0.0.1:4153"] });
    expect(() => render(<ProducerNsq />)).not.toThrow();
  });
});

describe("the NSQ overview", () => {
  it("renders with nothing connected", () => {
    clusterState.current = stateOf({ online: false });
    topicsState.current = stateOf({ online: false });
    channelsState.current = stateOf({ online: false });
    expect(() => render(<OverviewNsq />)).not.toThrow();
  });

  /*
   * Three separate problems with three separate fixes, which is why the tiles
   * split them: a backlog with consumers attached, a backlog with none, and
   * messages a topic is holding that reached no channel at all.
   */
  it("separates a backlog nobody is draining from one a topic is holding", () => {
    clusterState.current = stateOf({
      data: {
        overview: {
          name: "",
          totalNodes: 2,
          onlineNodes: 2,
          destinations: 2,
          subscriptions: 2,
          avgDiskUsage: -1,
          attributes: {},
        },
        nodes: [nsqd],
        directory: [lookupd],
      },
    });
    topicsState.current = stateOf({ data: [orders, held] });
    channelsState.current = stateOf({ data: [analytics, pausedChannel] });
    const html = render(<OverviewNsq />);
    expect(html).toContain("MQS.SEED.orders/analytics");
    // The undrained total: analytics has 160 waiting with nothing attached.
    expect(html).toContain("160");
    // And the five a paused topic is holding, which reached no channel.
    expect(html).toContain("5");
  });

  it("says so when nothing is waiting anywhere", () => {
    clusterState.current = stateOf({
      data: {
        overview: {
          name: "",
          totalNodes: 1,
          onlineNodes: 1,
          destinations: 0,
          subscriptions: 0,
          avgDiskUsage: -1,
          attributes: {},
        },
        nodes: [nsqd],
        directory: [],
      },
    });
    topicsState.current = stateOf({ data: [] });
    channelsState.current = stateOf({ data: [] });
    expect(() => render(<OverviewNsq />)).not.toThrow();
  });
});
