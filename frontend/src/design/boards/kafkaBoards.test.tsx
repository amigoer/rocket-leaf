import { beforeAll, describe, expect, it, vi } from "vitest";

/**
 * Every Kafka board, through the four states it can be in.
 *
 * The i18n sweep renders each board once with nothing connected, which covers
 * the offline notice and the strings. It cannot cover the rest: a board only
 * touches its data on the path where data exists, and that is exactly where a
 * missing field or an empty list throws. Each board is rendered here against a
 * stubbed hook so loading, failed, connected-but-empty and populated all get
 * exercised.
 *
 * The stubs return the shapes the Go side actually sends - attribute keys
 * included - so a driver that renames one breaks a board test rather than a
 * screenshot nobody is looking at.
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

const clusterState = vi.hoisted(() => ({ current: null as unknown }));
const topicsState = vi.hoisted(() => ({ current: null as unknown }));
const topicDetailState = vi.hoisted(() => ({ current: null as unknown }));
const groupsState = vi.hoisted(() => ({ current: null as unknown }));
const groupDetailState = vi.hoisted(() => ({ current: null as unknown }));
const logDirState = vi.hoisted(() => ({ current: null as unknown }));
const transactionState = vi.hoisted(() => ({ current: null as unknown }));
const brokerConfigState = vi.hoisted(() => ({ current: null as unknown }));
const readState = vi.hoisted(() => ({ current: null as unknown }));
const tailState = vi.hoisted(() => ({ current: null as unknown }));

vi.mock("@/hooks/kafka/useKafkaCluster", () => ({
  useKafkaCluster: () => clusterState.current,
  useKafkaLogDirs: () => logDirState.current,
  useKafkaTransactions: () => transactionState.current,
  useKafkaBrokerConfig: () => brokerConfigState.current,
}));
vi.mock("@/hooks/kafka/useKafkaTopics", () => ({
  useKafkaTopics: () => topicsState.current,
  useKafkaTopicDetail: () => topicDetailState.current,
}));
vi.mock("@/hooks/kafka/useKafkaGroups", () => ({
  useKafkaGroups: () => groupsState.current,
  useKafkaGroupDetail: () => groupDetailState.current,
}));
vi.mock("@/hooks/kafka/useKafkaMessages", () => ({
  useKafkaRead: () => readState.current,
  useKafkaTail: () => tailState.current,
}));
vi.mock("@/mq/ConnectionScope", () => ({
  useConnectionScope: () => ({ id: 1, kind: "kafka", key: "k1", online: true }),
}));

let render: (element: React.ReactElement) => string;
let OverviewKafka: typeof import("./overview/OverviewKafka").OverviewKafka;
let TopicsKafka: typeof import("./topics/TopicsKafka").TopicsKafka;
let ConsumersKafka: typeof import("./consumers/ConsumersKafka").ConsumersKafka;
let BrokersKafka: typeof import("./cluster/BrokersKafka").BrokersKafka;
let KafkaTransactionsPanel: typeof import("./cluster/BrokersKafka").KafkaTransactionsPanel;
let MessagesKafka: typeof import("./messages/MessagesKafka").MessagesKafka;

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

  const [server, overview, topics, consumers, brokers, messages, ui, i18n, settings] =
    await Promise.all([
    import("react-dom/server"),
    import("./overview/OverviewKafka"),
    import("./topics/TopicsKafka"),
    import("./consumers/ConsumersKafka"),
    import("./cluster/BrokersKafka"),
    import("./messages/MessagesKafka"),
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
  OverviewKafka = overview.OverviewKafka;
  TopicsKafka = topics.TopicsKafka;
  ConsumersKafka = consumers.ConsumersKafka;
  BrokersKafka = brokers.BrokersKafka;
  KafkaTransactionsPanel = brokers.KafkaTransactionsPanel;
  MessagesKafka = messages.MessagesKafka;
});

/** A healthy three-broker cluster, shaped the way the driver sends it. */
const healthyCluster = {
  overview: {
    name: "mq-studio-e2e-kafka-0",
    totalNodes: 3,
    onlineNodes: 3,
    destinations: 2,
    subscriptions: 1,
    avgDiskUsage: -1,
    attributes: {
      clusterId: "mq-studio-e2e-kafka-0",
      controllerNode: "2",
      brokers: "3",
      topics: "2",
      internalTopics: "1",
      partitions: "9",
      underReplicatedPartitions: "0",
      offlinePartitions: "0",
      leaderlessPartitions: "0",
      consumerGroups: "1",
    },
  },
  nodes: [
    {
      id: 1,
      name: "broker-1",
      address: "127.0.0.1:9092",
      cluster: "",
      version: "",
      status: "online",
      rateIn: -1,
      rateOut: -1,
      diskUsage: -1,
      lastSeen: "",
      attributes: { nodeId: "1", rack: "eu-west-1a", controller: "false" },
    },
    {
      id: 2,
      name: "broker-2",
      address: "127.0.0.1:9094",
      cluster: "",
      version: "",
      status: "online",
      rateIn: -1,
      rateOut: -1,
      diskUsage: -1,
      lastSeen: "",
      attributes: { nodeId: "2", rack: "", controller: "true" },
    },
  ],
};

/** The same cluster with a broker falling behind. */
const unhealthyCluster = {
  ...healthyCluster,
  overview: {
    ...healthyCluster.overview,
    attributes: {
      ...healthyCluster.overview.attributes,
      underReplicatedPartitions: "3",
      offlinePartitions: "2",
      leaderlessPartitions: "1",
    },
  },
};

/** A cluster that answered but has nothing on it yet. */
const emptyCluster = {
  overview: {
    name: "",
    totalNodes: 0,
    onlineNodes: 0,
    destinations: 0,
    subscriptions: -1,
    avgDiskUsage: -1,
    attributes: {},
  },
  nodes: [],
};

describe("the Kafka overview board", () => {
  it("draws the offline notice with nothing dialled", () => {
    clusterState.current = stateOf({ online: false });
    expect(render(<OverviewKafka />)).toContain("未连接");
  });

  /*
   * Busy, not yet spinning. A read that lands inside the spinner's delay never
   * draws one - what has to hold is that the board does not render as though
   * it had answered.
   */
  it("marks itself busy before the first answer", () => {
    clusterState.current = stateOf({ loading: true });
    const html = render(<OverviewKafka />);
    expect(html).toContain('aria-busy="true"');
    expect(html).not.toContain("正在读取");
  });

  it("draws the failure and its reason", () => {
    clusterState.current = stateOf({ error: "mq.kafka.degraded.credentials" });
    const html = render(<OverviewKafka />);
    // A driver reports a reason the user can act on as an i18n key, and the
    // board has to resolve it rather than printing the key.
    expect(html).not.toContain("mq.kafka.degraded.credentials");
    expect(html).toContain("集群拒绝了这组凭据");
  });

  it("renders a cluster that answered with nothing on it", () => {
    clusterState.current = stateOf({ data: emptyCluster });
    const html = render(<OverviewKafka />);
    expect(html).toContain("尚未选出控制器");
    // Counts the cluster did not report read as absent, never as zero.
    expect(html).toContain("—");
  });

  it("renders a healthy cluster and says so", () => {
    clusterState.current = stateOf({ data: healthyCluster });
    const html = render(<OverviewKafka />);

    expect(html).toContain("全部同步");
    expect(html).toContain("控制器是 broker 2");
    expect(html).toContain("另有 1 个内部 topic");
    expect(html).toContain("127.0.0.1:9092");
    expect(html).toContain("eu-west-1a");
    // A broker with no rack shows an em dash, not an empty cell.
    expect(html).toContain("—");
  });

  it("says a cluster needs attention when any partition counter is not zero", () => {
    clusterState.current = stateOf({ data: unhealthyCluster });
    const html = render(<OverviewKafka />);

    expect(html).toContain("需要关注");
    expect(html).not.toContain("全部同步");
  });

  // The canvas drew a throughput chart and a produce rate. Kafka's admin
  // protocol reports neither, so nothing on this page may imply it does.
  it("shows no per-second figure anywhere", () => {
    clusterState.current = stateOf({ data: healthyCluster });
    expect(render(<OverviewKafka />)).not.toMatch(/\/s\b/);
  });
});

/** Two topics as the driver sends them, one of them internal. */
const topicRows = [
  {
    id: 1,
    ref: { namespace: "", name: "orders.created" },
    partitions: 3,
    subscribers: -1,
    depth: 88204771,
    rateIn: -1,
    rateOut: -1,
    lastUpdated: "",
    attributes: {
      internal: "false",
      replicationFactor: "3",
      minInsyncReplicas: "2",
      cleanupPolicy: "delete",
      underReplicatedPartitions: "1",
      offlinePartitions: "0",
      leaderlessPartitions: "0",
    },
  },
  {
    id: 2,
    ref: { namespace: "", name: "__consumer_offsets" },
    partitions: 50,
    subscribers: -1,
    depth: -1,
    rateIn: -1,
    rateOut: -1,
    lastUpdated: "",
    attributes: {
      internal: "true",
      replicationFactor: "3",
      underReplicatedPartitions: "0",
      offlinePartitions: "0",
      leaderlessPartitions: "0",
    },
  },
];

describe("the Kafka topics board", () => {
  it("draws the offline notice with nothing dialled", () => {
    topicsState.current = stateOf({ online: false });
    topicDetailState.current = stateOf({});
    expect(render(<TopicsKafka />)).toContain("未连接");
  });

  it("draws the failure and its reason", () => {
    topicsState.current = stateOf({ error: "mq.kafka.degraded.forbidden" });
    topicDetailState.current = stateOf({});
    const html = render(<TopicsKafka />);
    expect(html).not.toContain("mq.kafka.degraded.forbidden");
    expect(html).toContain("没有描述集群的权限");
  });

  it("says so when a cluster has no topics of its own", () => {
    topicsState.current = stateOf({ data: [] });
    topicDetailState.current = stateOf({});
    expect(render(<TopicsKafka />)).toContain("还没有 topic");
  });

  // Internal topics exist on every cluster and nobody made them. They stay out
  // of the list until asked for, which is what the empty state above says.
  it("hides internal topics by default", () => {
    topicsState.current = stateOf({ data: topicRows });
    topicDetailState.current = stateOf({});
    const html = render(<TopicsKafka />);

    expect(html).toContain("orders.created");
    expect(html).not.toContain("__consumer_offsets");
  });

  it("marks a topic whose replicas are behind", () => {
    topicsState.current = stateOf({ data: topicRows });
    topicDetailState.current = stateOf({});
    expect(render(<TopicsKafka />)).toContain("URP 1");
  });

  // A topic the cluster would not answer for shows an em dash, never a zero:
  // "no records" and "nobody asked" are different facts.
  it("draws an unreported count as absent", () => {
    const first = topicRows[0]!;
    topicsState.current = stateOf({
      data: [{ ...first, depth: -1, attributes: { ...first.attributes, minInsyncReplicas: "" } }],
    });
    topicDetailState.current = stateOf({});
    expect(render(<TopicsKafka />)).toContain("—");
  });

  // The canvas drew a produce rate and a backlog. Kafka reports no rate, and a
  // backlog belongs to a group reading a topic rather than to the topic.
  it("shows no per-second figure", () => {
    topicsState.current = stateOf({ data: topicRows });
    topicDetailState.current = stateOf({});
    expect(render(<TopicsKafka />)).not.toMatch(/\/s\b/);
  });
});

const group = (over: Record<string, unknown> = {}) => ({
  id: 1,
  ref: { namespace: "", name: "settle-consumer" },
  status: "online",
  members: 2,
  destinations: 1,
  backlog: 9820,
  rateOut: -1,
  lastUpdated: "",
  attributes: {
    state: "Stable",
    protocol: "consumer",
    assignor: "range",
    coordinator: "2",
    topics: "orders.created",
    hasMembers: "true",
  },
  ...over,
});

const groupDetail = {
  partitions: [
    {
      topic: "orders.created",
      partition: 0,
      member: "c-1@10.2.3.4",
      committed: 88199021,
      start: 0,
      end: 88204771,
      lag: 5750,
    },
    {
      topic: "orders.created",
      partition: 1,
      member: "",
      committed: -1,
      start: 0,
      end: 400,
      lag: 400,
    },
  ],
  members: [
    {
      memberId: "m-1",
      clientId: "c-1",
      clientHost: "10.2.3.4",
      instanceId: "worker-a",
      assigned: ["orders.created:0"],
    },
  ],
};

describe("the Kafka consumer groups board", () => {
  it("draws the offline notice with nothing dialled", () => {
    groupsState.current = stateOf({ online: false });
    groupDetailState.current = stateOf({});
    expect(render(<ConsumersKafka />)).toContain("未连接");
  });

  it("says so when no group has committed an offset yet", () => {
    groupsState.current = stateOf({ data: [] });
    groupDetailState.current = stateOf({});
    expect(render(<ConsumersKafka />)).toContain("还没有消费组提交过位点");
  });

  it("draws a stable group with its lag and assignor", () => {
    groupsState.current = stateOf({ data: [group()] });
    groupDetailState.current = stateOf({ data: groupDetail });
    const html = render(<ConsumersKafka />);

    expect(html).toContain("settle-consumer");
    expect(html).toContain("Stable");
    expect(html).toContain("range");
  });

  /*
   * Empty is the state worth naming. Offsets committed and nothing connected
   * is either a gap between deployments or a consumer that died leaving a
   * backlog growing, and the protocol does not say which - so the board names
   * the state rather than folding it into "offline".
   */
  it("names an empty group rather than calling it offline", () => {
    groupsState.current = stateOf({
      data: [group({ status: "warning", members: 0, attributes: { ...group().attributes, state: "Empty", hasMembers: "false" } })],
    });
    groupDetailState.current = stateOf({ data: groupDetail });
    expect(render(<ConsumersKafka />)).toContain("Empty");
  });

  it("marks a group that is rebalancing", () => {
    groupsState.current = stateOf({
      data: [group({ attributes: { ...group().attributes, state: "PreparingRebalance" } })],
    });
    groupDetailState.current = stateOf({ data: groupDetail });
    expect(render(<ConsumersKafka />)).toContain("PreparingRebalance");
  });

  // A lag nobody could measure is not a caught-up group.
  it("draws an unmeasurable lag as absent", () => {
    groupsState.current = stateOf({ data: [group({ backlog: -1 })] });
    groupDetailState.current = stateOf({ data: groupDetail });
    expect(render(<ConsumersKafka />)).toContain("—");
  });

  // The canvas drew a consume rate. Kafka's admin protocol reports none.
  it("shows no per-second figure", () => {
    groupsState.current = stateOf({ data: [group()] });
    groupDetailState.current = stateOf({ data: groupDetail });
    expect(render(<ConsumersKafka />)).not.toMatch(/\/s\b/);
  });
});

describe("the Kafka cluster board", () => {
  it("draws the offline notice with nothing dialled", () => {
    clusterState.current = stateOf({ online: false });
    logDirState.current = stateOf({});
    transactionState.current = stateOf({});
    brokerConfigState.current = stateOf({});
    expect(render(<BrokersKafka />)).toContain("未连接");
  });

  it("lists the brokers and marks the controller", () => {
    clusterState.current = stateOf({ data: healthyCluster });
    logDirState.current = stateOf({});
    transactionState.current = stateOf({});
    brokerConfigState.current = stateOf({});
    const html = render(<BrokersKafka />);

    expect(html).toContain("127.0.0.1:9092");
    expect(html).toContain("控制器");
    expect(html).toContain("eu-west-1a");
  });

  /*
   * The canvas drew a disk-usage percentage. Kafka reports occupied bytes per
   * log directory and nothing about the filesystem holding them, so there is
   * no denominator to build one from.
   */
  it("shows no disk percentage", () => {
    clusterState.current = stateOf({ data: healthyCluster });
    logDirState.current = stateOf({});
    transactionState.current = stateOf({});
    brokerConfigState.current = stateOf({});
    const html = render(<BrokersKafka />);
    // Percentages in inline layout styles are not a claim about a disk; only
    // one inside rendered text would be.
    const text = html.replace(/<[^>]*>/g, " ");
    expect(text).not.toMatch(/\d+\s*%/);
  });
});

const NOW = 1_756_000_000_000;

const transaction = (over: Record<string, unknown> = {}) => ({
  id: "orders-writer",
  state: "Ongoing",
  coordinator: 2,
  producerId: 1001,
  producerEpoch: 7,
  startedAt: NOW - 90_000,
  timeoutMs: 60_000,
  partitions: ["orders.created:0", "orders.created:1"],
  open: true,
  holding: true,
  ...over,
});

/*
 * The transactions tab.
 *
 * It is the one panel in this app whose whole purpose is a state no other page
 * can show: a producer that died mid-transaction, holding the last stable
 * offset while every other figure reads healthy.
 */
describe("the Kafka transactions panel", () => {
  /*
   * Busy, not yet spinning. A read that lands inside the spinner's delay never
   * draws one - what has to hold is that the panel does not render as though
   * it had answered, which for this one would mean no transactions in flight.
   */
  it("marks itself busy while it is loading", () => {
    const html = render(
      <KafkaTransactionsPanel state={stateOf({ loading: true })} now={NOW} />,
    );
    expect(html).toContain('aria-busy="true"');
    expect(html).not.toContain("正在读取");
  });

  it("shows the reason when the cluster would not answer", () => {
    const html = render(
      <KafkaTransactionsPanel
        state={stateOf({ error: "coordinator not available" })}
        now={NOW}
      />,
    );
    expect(html).toContain("coordinator not available");
  });

  // A cluster with no transactional producer at all is the normal case, and it
  // must not read as a page that failed to load.
  it("says the cluster has none rather than showing an empty table", () => {
    const html = render(
      <KafkaTransactionsPanel
        state={stateOf({ data: { transactions: [], holding: 0 } })}
        now={NOW}
      />,
    );
    expect(html).toContain("没有注册任何事务生产者");
  });

  it("names the transaction, what it holds, and how long it has held it", () => {
    const html = render(
      <KafkaTransactionsPanel
        state={stateOf({ data: { transactions: [transaction()], holding: 1 } })}
        now={NOW}
      />,
    );

    expect(html).toContain("orders-writer");
    expect(html).toContain("Ongoing");
    expect(html).toContain("orders.created:0");
    expect(html).toContain("1m");
    // The producer and its epoch together: a fenced-out producer keeps the id
    // and takes a new epoch, so the id alone does not identify the session.
    expect(html).toContain("1001");
    expect(html).toContain("/7");
  });

  /*
   * Past its timeout is the finding, not the age. The coordinator undertook to
   * abort the transaction after that long, so one still open past it is not a
   * slow job - it is a transaction nothing is finishing.
   */
  it("marks a transaction the cluster should already have aborted", () => {
    const html = render(
      <KafkaTransactionsPanel
        state={stateOf({ data: { transactions: [transaction()], holding: 1 } })}
        now={NOW}
      />,
    );
    expect(html).toContain(">已超时<");
  });

  it("leaves a transaction inside its timeout unmarked", () => {
    const html = render(
      <KafkaTransactionsPanel
        state={stateOf({
          data: { transactions: [transaction({ startedAt: NOW - 5_000 })], holding: 1 },
        })}
        now={NOW}
      />,
    );
    expect(html).not.toContain(">已超时<");
  });

  // A completed transaction is listed but holds nothing, and the panel must
  // not colour it as trouble.
  it("does not flag a transaction that has finished", () => {
    const html = render(
      <KafkaTransactionsPanel
        state={stateOf({
          data: {
            transactions: [
              transaction({
                state: "CompleteCommit",
                open: false,
                holding: false,
                partitions: [],
                // Long past its timeout, which is every finished transaction
                // the cluster is still listing.
                startedAt: NOW - 6 * 3600 * 1000,
              }),
            ],
            holding: 0,
          },
        })}
        now={NOW}
      />,
    );
    expect(html).toContain("CompleteCommit");
    expect(html).not.toContain(">已超时<");
  });

  // Unknown is not zero: a coordinator that did not report a start time must
  // not produce an age, and must not be called overdue either.
  it("reports no age when the coordinator gave no start time", () => {
    const html = render(
      <KafkaTransactionsPanel
        state={stateOf({
          data: { transactions: [transaction({ startedAt: -1 })], holding: 1 },
        })}
        now={NOW}
      />,
    );
    expect(html).not.toContain(">已超时<");
    expect(html).toContain("—");
  });
});

const record = (over: Record<string, unknown> = {}) => ({
  id: 1,
  topic: "orders.created",
  messageId: "orders.created-3-42",
  keys: "ORD-1",
  queueId: 3,
  queueOffset: 42,
  storeTime: "2026-08-31T10:24:07Z",
  storeTimestamp: 1_756_636_247_000,
  status: "normal",
  body: '{"orderId":"ORD-1"}',
  properties: { "trace-id": "abc" },
  ...over,
});

const readOf = (over: Record<string, unknown> = {}) => ({
  records: [],
  loading: false,
  error: null,
  ran: false,
  run: () => {},
  ...over,
});

const tailOf = (over: Record<string, unknown> = {}) => ({
  records: [],
  dropped: 0,
  error: null,
  step: async () => {},
  reset: () => {},
  ...over,
});

describe("the Kafka messages board", () => {
  it("asks for a topic before anything is read", () => {
    topicsState.current = stateOf({ data: topicRows });
    readState.current = readOf();
    tailState.current = tailOf();
    expect(render(<MessagesKafka />)).toContain("选一个 topic 再读取");
  });

  it("says a query found nothing rather than looking unread", () => {
    topicsState.current = stateOf({ data: topicRows });
    readState.current = readOf({ ran: true });
    tailState.current = tailOf();
    expect(render(<MessagesKafka />)).toContain("没有匹配的消息");
  });

  it("draws a record with its coordinates and headers", () => {
    topicsState.current = stateOf({ data: topicRows });
    readState.current = readOf({ records: [record()], ran: true });
    tailState.current = tailOf();
    const html = render(<MessagesKafka />);

    expect(html).toContain("42");
    expect(html).toContain("ORD-1");
  });

  /*
   * A record with no key at all is spread across partitions; one with an empty
   * key is pinned. Drawing both as blank would hide why two records that look
   * the same went to different places.
   */
  it("says when a record has no key at all", () => {
    topicsState.current = stateOf({ data: topicRows });
    readState.current = readOf({
      records: [record({ keys: " __mqs_null_key" })],
      ran: true,
    });
    tailState.current = tailOf();
    expect(render(<MessagesKafka />)).toContain("无 key");
  });

  /*
   * The canvas drew a produce rate beside the records. Kafka reports none, and
   * a message list is the last place a made-up throughput would be noticed.
   *
   * What a tail lost to retention is asserted in the driver instead: the
   * banner only exists on the follow panel, and a static render cannot switch
   * to it, while the arithmetic behind the number is what actually matters.
   */
  it("shows no per-second figure", () => {
    topicsState.current = stateOf({ data: topicRows });
    readState.current = readOf({ records: [record()], ran: true });
    tailState.current = tailOf();
    expect(render(<MessagesKafka />)).not.toMatch(/\/s\b/);
  });
});
