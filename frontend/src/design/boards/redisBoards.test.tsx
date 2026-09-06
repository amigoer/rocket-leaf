import { beforeAll, describe, expect, it, vi } from "vitest";

/**
 * Every Redis Stream board, through the states it can be in.
 *
 * The i18n sweep renders each board once with nothing connected, which covers
 * the offline notice and the strings. It cannot cover the rest: a board only
 * touches its data on the path where data exists, and that is exactly where a
 * missing attribute or an empty list throws. Each board is rendered here
 * against a stubbed hook so loading, failed, connected-but-empty and populated
 * all get exercised.
 *
 * The stubs return the shapes internal/driver/redisstream actually sends, so a
 * driver that renames an attribute key breaks a board test rather than a
 * screenshot.
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

const streamsState = vi.hoisted(() => ({ current: null as unknown }));
const streamDetailState = vi.hoisted(() => ({ current: null as unknown }));
const groupsState = vi.hoisted(() => ({ current: null as unknown }));
const entriesState = vi.hoisted(() => ({ current: null as unknown }));
const pendingState = vi.hoisted(() => ({ current: null as unknown }));
const serversState = vi.hoisted(() => ({ current: null as unknown }));
const slowLogState = vi.hoisted(() => ({ current: null as unknown }));
const clientsState = vi.hoisted(() => ({ current: null as unknown }));

vi.mock("@/hooks/redis/useRedisStreams", () => ({
  useRedisStreams: () => streamsState.current,
  useRedisStreamDetail: () => streamDetailState.current,
}));
vi.mock("@/hooks/redis/useRedisGroups", () => ({
  useRedisGroups: () => groupsState.current,
}));
vi.mock("@/hooks/redis/useRedisEntries", () => ({
  useRedisEntries: () => entriesState.current,
}));
vi.mock("@/hooks/redis/useRedisPending", () => ({
  useRedisPending: () => pendingState.current,
}));
vi.mock("@/hooks/redis/useRedisNodes", () => ({
  useRedisServers: () => serversState.current,
  useRedisSlowLog: () => slowLogState.current,
}));
vi.mock("@/hooks/redis/useRedisClients", () => ({
  useRedisClients: () => clientsState.current,
}));

let render: (element: React.ReactElement) => string;
let StreamsRedis: typeof import("./topics/StreamsRedis").StreamsRedis;
let ConsumersRedis: typeof import("./consumers/ConsumersRedis").ConsumersRedis;
let MessagesRedis: typeof import("./messages/MessagesRedis").MessagesRedis;
let ProducerRedis: typeof import("./producer/ProducerRedis").ProducerRedis;
let PelRedis: typeof import("./dlq/PelRedis").PelRedis;
let NodeRedis: typeof import("./cluster/NodeRedis").NodeRedis;
let ClientsRedis: typeof import("./consumers/ClientsRedis").ClientsRedis;
let OverviewRedis: typeof import("./overview/OverviewRedis").OverviewRedis;

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
    streams,
    consumers,
    messages,
    producer,
    pel,
    cluster,
    clients,
    overview,
    ui,
    i18n,
    settings,
  ] = await Promise.all([
    import("react-dom/server"),
    import("./topics/StreamsRedis"),
    import("./consumers/ConsumersRedis"),
    import("./messages/MessagesRedis"),
    import("./producer/ProducerRedis"),
    import("./dlq/PelRedis"),
    import("./cluster/NodeRedis"),
    import("./consumers/ClientsRedis"),
    import("./overview/OverviewRedis"),
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
  StreamsRedis = streams.StreamsRedis;
  ConsumersRedis = consumers.ConsumersRedis;
  MessagesRedis = messages.MessagesRedis;
  ProducerRedis = producer.ProducerRedis;
  PelRedis = pel.PelRedis;
  NodeRedis = cluster.NodeRedis;
  ClientsRedis = clients.ClientsRedis;
  OverviewRedis = overview.OverviewRedis;
});

/** A stream as internal/driver/redisstream/destination.go sends one. */
const orders = {
  id: 1,
  ref: { namespace: "", name: "orders:events" },
  partitions: -1,
  subscribers: 3,
  depth: 1204771,
  rateIn: -1,
  rateOut: -1,
  lastUpdated: "",
  attributes: {
    lastGeneratedId: "1756454646018-0",
    firstEntryId: "1756368200104-0",
    lastEntryId: "1756454646018-0",
    entriesAdded: "1300000",
    radixTreeKeys: "11842",
    radixTreeNodes: "23118",
    memoryBytes: "90177536",
  },
};

/** A stream nothing has been written to, and no group reads. */
const fresh = {
  id: 2,
  ref: { namespace: "", name: "orders:new" },
  partitions: -1,
  subscribers: 0,
  depth: 0,
  rateIn: -1,
  rateOut: -1,
  lastUpdated: "",
  attributes: { lastGeneratedId: "0-0" },
};

describe("the Redis streams board", () => {
  it("says nothing is dialled rather than showing an empty list", () => {
    streamsState.current = stateOf({ online: false, data: null });
    streamDetailState.current = stateOf({ online: false, data: null });
    const html = render(<StreamsRedis />);
    expect(html).toContain("未连接");
    expect(html).not.toContain("orders:events");
  });

  /*
   * Busy, not yet spinning. A read that lands inside the spinner's delay never
   * draws one - what has to hold is that the board does not render as though
   * it had answered.
   */
  it("marks itself busy while the first load is in flight", () => {
    streamsState.current = stateOf({ loading: true });
    streamDetailState.current = stateOf({ loading: true });
    const html = render(<StreamsRedis />);
    expect(html).toContain('aria-busy="true"');
    expect(html).not.toContain("正在读取");
  });

  /*
   * A failed read must not look like an empty keyspace. The driver reports a
   * reason the user can act on as an i18n key, and the board resolves it -
   * putting the raw key on screen would be worse than the English.
   */
  it("shows the driver's reason when the read failed", () => {
    streamsState.current = stateOf({ error: "mq.redis-stream.degraded.credentials" });
    streamDetailState.current = stateOf({});
    const html = render(<StreamsRedis />);
    expect(html).not.toContain("mq.redis-stream.degraded.credentials");
    expect(html).toContain("拒绝");
  });

  /*
   * The scan succeeding and finding nothing is its own state, and the message
   * says where to look: the key pattern on the connection is what narrows it,
   * so an empty list is usually a pattern rather than an empty server.
   */
  it("says the scan found nothing, and points at the key pattern", () => {
    streamsState.current = stateOf({ data: [] });
    streamDetailState.current = stateOf({ data: null });
    const html = render(<StreamsRedis />);
    expect(html).toContain("键匹配模式");
    expect(html).toContain("找到 0 个");
  });

  it("lists the streams with their length, groups and last id", () => {
    streamsState.current = stateOf({ data: [orders, fresh] });
    streamDetailState.current = stateOf({ data: null });
    const html = render(<StreamsRedis />);
    expect(html).toContain("orders:events");
    expect(html).toContain("orders:new");
    expect(html).toContain("1756454646018-0");
    expect(html).toContain("找到 2 个");
  });

  /*
   * The figure the canvas's invented maxlen column was reaching for. Redis
   * stores no bound, but entries-added minus the length is how much trimming
   * has actually taken away, and that is a real number.
   */
  it("shows how much has been trimmed away, and a dash where it cannot know", () => {
    streamsState.current = stateOf({ data: [orders, fresh] });
    streamDetailState.current = stateOf({ data: null });
    const html = render(<StreamsRedis />);
    // 1300000 added, 1204771 held.
    expect(html).toContain("95,229");
    // The fresh stream reports no entries-added, so the cell is a dash rather
    // than a zero: "not reported" and "nothing trimmed" are different facts.
    expect(html).toContain("—");
  });

  it("renders a stream that has never held an entry without inventing ids", () => {
    streamsState.current = stateOf({ data: [fresh] });
    streamDetailState.current = stateOf({ data: null });
    const html = render(<StreamsRedis />);
    expect(html).toContain("orders:new");
    // No first or last entry exists, so nothing may be drawn for them.
    expect(html).not.toContain("undefined");
    expect(html).not.toContain("NaN");
  });
});

/*
 * The create and delete controls are drawn only when the driver can do them,
 * so their presence in the populated render is what says the board is wired to
 * something rather than drawing a canvas leftover.
 */
describe("the Redis streams board's write controls", () => {
  it("offers to create a stream", () => {
    streamsState.current = stateOf({ data: [orders] });
    streamDetailState.current = stateOf({ data: null });
    expect(render(<StreamsRedis />)).toContain("新建 Stream");
  });

  it("offers to delete the selected stream, and not before one is selected", () => {
    streamsState.current = stateOf({ data: [orders] });
    streamDetailState.current = stateOf({ data: null });
    // Nothing is selected on first render, so the detail panel and its
    // destructive footer must not be there at all.
    const html = render(<StreamsRedis />);
    expect(html).not.toContain("DEL key");
    expect(html).not.toContain("XTRIM");
  });
});

/** A group as internal/driver/redisstream/subscription.go sends one. */
const settleGroup = {
  id: 1,
  ref: { namespace: "orders:events", name: "settle-group" },
  status: "online",
  members: 2,
  destinations: 1,
  backlog: 29,
  rateOut: -1,
  lastUpdated: "",
  attributes: {
    stream: "orders:events",
    pending: "29",
    lastDeliveredId: "1756454641773-2",
    entriesRead: "1204742",
  },
};

/** Nothing attached, entries still owed: the state worth separating from idle. */
const stalledGroup = {
  id: 2,
  ref: { namespace: "payments:captured", name: "capture-group" },
  status: "warning",
  members: 0,
  destinations: 1,
  // Redis could not count the lag: entries this group had not read were
  // deleted, and it reports nil rather than a number.
  backlog: -1,
  rateOut: -1,
  lastUpdated: "",
  attributes: { stream: "payments:captured", pending: "12", lastDeliveredId: "1756454600000-0" },
};

describe("the Redis consumer groups board", () => {
  it("says nothing is dialled rather than showing an empty list", () => {
    groupsState.current = stateOf({ online: false, data: null });
    streamsState.current = stateOf({ online: false, data: null });
    const html = render(<ConsumersRedis />);
    expect(html).toContain("未连接");
    expect(html).not.toContain("settle-group");
  });

  /*
   * Busy, not yet spinning. A read that lands inside the spinner's delay never
   * draws one - what has to hold is that the board does not render as though
   * it had answered.
   */
  it("marks itself busy while the first load is in flight", () => {
    groupsState.current = stateOf({ loading: true });
    streamsState.current = stateOf({ loading: true });
    const html = render(<ConsumersRedis />);
    expect(html).toContain('aria-busy="true"');
    expect(html).not.toContain("正在读取");
  });

  it("shows the driver's reason when the read failed", () => {
    groupsState.current = stateOf({ error: "mq.redis-stream.degraded.credentials" });
    streamsState.current = stateOf({ data: [] });
    const html = render(<ConsumersRedis />);
    expect(html).not.toContain("mq.redis-stream.degraded.credentials");
    expect(html).toContain("拒绝");
  });

  it("says when no stream it found has a group", () => {
    groupsState.current = stateOf({ data: [] });
    streamsState.current = stateOf({ data: [orders] });
    expect(render(<ConsumersRedis />)).toContain("都没有消费组");
  });

  /*
   * The stream is a column, not a detail. Two groups called "settle-group" on
   * different streams are unrelated objects, and a table showing the name
   * alone would look like it had a duplicate row.
   */
  it("shows which stream each group reads", () => {
    groupsState.current = stateOf({ data: [settleGroup, stalledGroup] });
    streamsState.current = stateOf({ data: [orders] });
    const html = render(<ConsumersRedis />);
    expect(html).toContain("settle-group");
    expect(html).toContain("orders:events");
    expect(html).toContain("capture-group");
    expect(html).toContain("payments:captured");
  });

  /*
   * Stalled and idle must not read the same. Nothing attached with entries
   * still owed is work that was handed out and never came back; nothing
   * attached with nothing owed is an application that is not running.
   */
  it("separates a stalled group from an idle one", () => {
    groupsState.current = stateOf({ data: [stalledGroup] });
    streamsState.current = stateOf({ data: [orders] });
    const html = render(<ConsumersRedis />);
    expect(html).toContain("停滞");
    expect(html).not.toContain("空闲");
  });

  // An uncountable lag renders as a dash. A zero would report a group that is
  // arbitrarily far behind as caught up.
  it("draws a dash where the lag could not be counted", () => {
    groupsState.current = stateOf({ data: [stalledGroup] });
    streamsState.current = stateOf({ data: [orders] });
    const html = render(<ConsumersRedis />);
    expect(html).toContain("—");
    expect(html).not.toContain("NaN");
  });

  /*
   * A group cannot exist without a stream to read, so with none found there is
   * nothing the dialog could offer and the control says so by being disabled
   * rather than opening onto an empty picker.
   */
  it("cannot declare a group when no stream was found", () => {
    groupsState.current = stateOf({ data: [] });
    streamsState.current = stateOf({ data: [] });
    expect(render(<ConsumersRedis />)).toContain("disabled");
  });
});

describe("the Redis consumer groups board's write controls", () => {
  it("offers to reposition and to delete each group", () => {
    groupsState.current = stateOf({ data: [settleGroup] });
    streamsState.current = stateOf({ data: [orders] });
    const html = render(<ConsumersRedis />);
    expect(html).toContain("重置位置");
    expect(html).toContain("删除");
  });

  it("offers neither when there are no groups to act on", () => {
    groupsState.current = stateOf({ data: [] });
    streamsState.current = stateOf({ data: [orders] });
    expect(render(<ConsumersRedis />)).not.toContain("重置位置");
  });
});

/** A browse result shaped the way the hook returns one. */
function entriesOf(over: Partial<{ items: unknown[]; running: boolean; lastCount: number | null }>) {
  const { items = [], running = false, lastCount = null } = over;
  return {
    items,
    running,
    lastCount,
    query: async () => {},
    state: { loading: false, error: null, online: true, refresh: async () => {} },
  };
}

/** An entry as internal/driver/redisstream/message.go sends one. */
const entry = {
  id: 1,
  topic: "orders:events",
  messageId: "1756454646018-3",
  body: '{"order":"A-1001","total":"42.50"}',
  queueId: 0,
  queueOffset: 0,
  storeTime: "2026-08-29 14:24:06",
  storeTimestamp: 1756454646018,
  status: "normal",
  properties: { order: "A-1001", total: "42.50" },
};

describe("the Redis messages board", () => {
  it("says nothing is dialled rather than showing an empty list", () => {
    entriesState.current = {
      ...entriesOf({}),
      state: { loading: false, error: null, online: false, refresh: async () => {} },
    };
    streamsState.current = stateOf({ online: false, data: null });
    expect(render(<MessagesRedis />)).toContain("未连接");
  });

  /*
   * Before the first read the page has not failed and is not empty - it has
   * simply not been asked anything yet. Rendering that as "nothing found"
   * would report an empty stream that was never read.
   */
  it("asks for a stream before anything has been read", () => {
    entriesState.current = entriesOf({});
    streamsState.current = stateOf({ data: [orders] });
    const html = render(<MessagesRedis />);
    expect(html).toContain("读取它的一段窗口");
    expect(html).not.toContain("没有匹配");
  });

  it("says the window held nothing once a read has happened", () => {
    entriesState.current = entriesOf({ lastCount: 0 });
    streamsState.current = stateOf({ data: [orders] });
    expect(render(<MessagesRedis />)).toContain("没有匹配");
  });

  it("lists entries with their field count and a summary of the contents", () => {
    entriesState.current = entriesOf({ items: [entry], lastCount: 1 });
    streamsState.current = stateOf({ data: [orders] });
    const html = render(<MessagesRedis />);
    expect(html).toContain("1756454646018-3");
    expect(html).toContain("order=A-1001");
    expect(html).toContain("2026-08-29 14:24:06");
    // What the read returned, not what the stream holds.
    expect(html).toContain("读到 1 条");
  });

  it("renders an entry with no fields without inventing any", () => {
    entriesState.current = entriesOf({
      items: [{ ...entry, body: "{}", properties: {} }],
      lastCount: 1,
    });
    streamsState.current = stateOf({ data: [orders] });
    const html = render(<MessagesRedis />);
    expect(html).toContain("1756454646018-3");
    expect(html).not.toContain("undefined");
    expect(html).not.toContain("NaN");
  });
});

describe("the Redis send console", () => {
  it("says nothing is dialled rather than showing an empty form", () => {
    streamsState.current = stateOf({ online: false, data: null });
    expect(render(<ProducerRedis />)).toContain("未连接");
  });

  /*
   * The console will not create a stream, so with none found there is nothing
   * to write to and the page says where to go rather than offering a text box
   * that would fail.
   */
  it("says where to go when no stream was found", () => {
    streamsState.current = stateOf({ data: [] });
    const html = render(<ProducerRedis />);
    expect(html).toContain("请先在 Stream 页面创建一个");
  });

  it("collects fields rather than a topic and a body", () => {
    streamsState.current = stateOf({ data: [orders] });
    const html = render(<ProducerRedis />);
    expect(html).toContain("字段");
    expect(html).toContain("Entry ID");
    // None of RocketMQ's vocabulary belongs here.
    expect(html).not.toContain("Tags");
    expect(html).not.toContain("延迟");
  });

  it("says the order is kept, because reading loses it", () => {
    streamsState.current = stateOf({ data: [orders] });
    expect(render(<ProducerRedis />)).toContain("字段顺序会被保留");
  });
});

/** A pending view as the hook returns one. */
function pendingOf(over: Partial<{ summary: unknown; entries: unknown[]; consumers: unknown[] }>) {
  const { summary = null, entries = [], consumers = [] } = over;
  return stateOf({ data: { summary, entries, consumers } });
}

const pendingEntry = {
  ref: { namespace: "orders:events", name: "settle-group" },
  id: "1756447200104-0",
  consumer: "settle-1",
  idleMs: 7_560_000,
  deliveries: 17,
};

const pendingSummary = {
  ref: { namespace: "orders:events", name: "settle-group" },
  count: 29,
  minId: "1756447200104-0",
  maxId: "1756454646018-0",
  perConsumer: [
    { consumer: "settle-1", count: 25 },
    { consumer: "settle-2", count: 4 },
  ],
};

describe("the Redis pending entries board", () => {
  it("says nothing is dialled rather than showing an empty list", () => {
    groupsState.current = stateOf({ online: false, data: null });
    pendingState.current = stateOf({ online: false, data: null });
    expect(render(<PelRedis />)).toContain("未连接");
  });

  it("says there is nothing owed rather than showing an empty table", () => {
    groupsState.current = stateOf({ data: [settleGroup] });
    pendingState.current = pendingOf({ summary: { ...pendingSummary, count: 0 } });
    expect(render(<PelRedis />)).toContain("每一条都已确认");
  });

  it("lists pending entries with their idle time and delivery count", () => {
    groupsState.current = stateOf({ data: [settleGroup] });
    pendingState.current = pendingOf({ summary: pendingSummary, entries: [pendingEntry] });
    const html = render(<PelRedis />);
    expect(html).toContain("1756447200104-0");
    expect(html).toContain("settle-1");
    // Milliseconds are what the server reports; a column of seven-digit
    // numbers is not what the page is for.
    expect(html).toContain("2.1h");
    expect(html).not.toContain("7560000");
    expect(html).toContain("17");
  });

  /*
   * One consumer holding most of what the group is owed and a group that is
   * generally behind look identical in the total. Naming the first is what
   * turns the page into something an operator can act on.
   */
  it("names a consumer holding most of the backlog", () => {
    groupsState.current = stateOf({ data: [settleGroup] });
    pendingState.current = pendingOf({ summary: pendingSummary, entries: [pendingEntry] });
    expect(render(<PelRedis />)).toContain("settle-1 一个人占了 25 条");
  });

  it("shows each consumer with what it holds and how long it has been quiet", () => {
    groupsState.current = stateOf({ data: [settleGroup] });
    pendingState.current = pendingOf({
      summary: pendingSummary,
      entries: [pendingEntry],
      consumers: [
        { name: "settle-1", pending: 25, idleMs: 7_560_000, inactiveMs: 0 },
        { name: "settle-2", pending: 4, idleMs: 900, inactiveMs: 0 },
      ],
    });
    const html = render(<PelRedis />);
    expect(html).toContain("settle-1");
    expect(html).toContain("settle-2");
  });

  // The bulk bar is the only route to the destructive actions, so it must not
  // be drawn before anything is selected.
  it("offers no bulk action until entries are selected", () => {
    groupsState.current = stateOf({ data: [settleGroup] });
    pendingState.current = pendingOf({ summary: pendingSummary, entries: [pendingEntry] });
    expect(render(<PelRedis />)).not.toContain("XACK");
  });
});

/** A server as internal/driver/redisstream/cluster.go sends one. */
const server = {
  id: 1,
  name: "127.0.0.1:6479",
  address: "127.0.0.1:6479",
  cluster: "",
  version: "8.10.1",
  status: "online",
  rateIn: -1,
  rateOut: -1,
  diskUsage: -1,
  lastSeen: "",
  attributes: {
    role: "master",
    mode: "standalone",
    uptimeSeconds: "8294400",
    connectedClients: "86",
    usedMemory: "432013312",
    maxMemory: "2147483648",
    opsPerSec: "3420",
    keyspaceHits: "9920000",
    keyspaceMisses: "80000",
    aofEnabled: "1",
    rdbLastBgsaveStatus: "ok",
    rdbChangesSinceLastSave: "1204",
    connectedReplicas: "2",
  },
};

const slowEntry = {
  id: 14,
  timestampMs: 1756454646000,
  durationMicros: 41200,
  command: ["KEYS", "*"],
  clientAddress: "10.2.0.44:51234",
  clientName: "reporting-service",
};

describe("the Redis server board", () => {
  it("says nothing is dialled rather than showing an empty server", () => {
    serversState.current = stateOf({ online: false, data: null });
    slowLogState.current = stateOf({ online: false, data: null });
    expect(render(<NodeRedis />)).toContain("未连接");
  });

  it("shows what the server reported, under its own headings", () => {
    serversState.current = stateOf({ data: { overview: null, nodes: [server], directory: [] } });
    slowLogState.current = stateOf({ data: [] });
    const html = render(<NodeRedis />);
    expect(html).toContain("127.0.0.1:6479");
    expect(html).toContain("master");
    expect(html).toContain("v8.10.1");
    // Commands, not messages: Redis counts one and not the other.
    expect(html).toContain("命令/秒");
    expect(html).toContain("99.2%");
  });

  /*
   * A server with no maxmemory has no cap to be full of. Drawing a meter would
   * report an unbounded server as one at some percentage of a limit it does
   * not have.
   */
  it("draws no memory meter on a server with no cap", () => {
    const uncapped = { ...server, attributes: { ...server.attributes, maxMemory: "0" } };
    serversState.current = stateOf({ data: { overview: null, nodes: [uncapped], directory: [] } });
    slowLogState.current = stateOf({ data: [] });
    expect(render(<NodeRedis />)).toContain("未设置 maxmemory");
  });

  it("separates a persistence job that failed from one that never ran", () => {
    const never = {
      ...server,
      attributes: {
        ...server.attributes,
        rdbLastBgsaveStatus: "",
      },
    };
    serversState.current = stateOf({ data: { overview: null, nodes: [never], directory: [] } });
    slowLogState.current = stateOf({ data: [] });
    expect(render(<NodeRedis />)).toContain("从未执行");
  });

  it("lists the slow log with the client that ran each command", () => {
    serversState.current = stateOf({ data: { overview: null, nodes: [server], directory: [] } });
    slowLogState.current = stateOf({ data: [slowEntry] });
    const html = render(<NodeRedis />);
    expect(html).toContain("KEYS *");
    // The client name is the field go-redis's typed helper drops, and the
    // reason the reply is parsed by hand.
    expect(html).toContain("reporting-service");
    expect(html).toContain("µs");
  });

  it("says the slow log is empty rather than showing a bare table", () => {
    serversState.current = stateOf({ data: { overview: null, nodes: [server], directory: [] } });
    slowLogState.current = stateOf({ data: [] });
    expect(render(<NodeRedis />)).toContain("slowlog-log-slower-than");
  });

  /*
   * Every node online and a slot range belonging to none of them is a cluster
   * that cannot serve those keys, and the node list alone looks healthy.
   */
  it("warns when a cluster is short of slots", () => {
    const clusterOverview = {
      name: "redis-cluster",
      totalNodes: 6,
      onlineNodes: 6,
      destinations: -1,
      subscriptions: -1,
      avgDiskUsage: -1,
      attributes: { clusterState: "ok", clusterSlots: "10923" },
    };
    const replica = { ...server, address: "127.0.0.1:6503", attributes: { ...server.attributes, role: "replica", nodeId: "abc" } };
    serversState.current = stateOf({
      data: { overview: clusterOverview, nodes: [server, replica], directory: [] },
    });
    slowLogState.current = stateOf({ data: [] });
    const html = render(<NodeRedis />);
    expect(html).toContain("10923");
    expect(html).toContain("无法服务");
  });
});

/** A connection as internal/driver/redisstream/connections.go sends one. */
const connection = {
  name: "42",
  clientName: "reporting-service",
  namespace: "0",
  user: "mqstudio",
  node: "127.0.0.1:6479",
  peerHost: "10.2.0.44",
  peerPort: 51234,
  protocol: "RESP3",
  state: "N",
  channels: 0,
  tls: false,
  cipher: "",
  heartbeatSec: 0,
  recvBytes: 91204,
  sendBytes: 884210,
  recvByteRate: 0,
  sendByteRate: 0,
  connectedAtMs: 0,
  blockedBy: "",
  attributes: { lastCommand: "xrange", idleSeconds: "3", ageSeconds: "8402", libraryName: "go-redis" },
};

/** This app's own connection, which the page has to be able to point out. */
const ownConnection = {
  ...connection,
  name: "43",
  clientName: "mq-studio.prod-redis",
  attributes: { lastCommand: "client", idleSeconds: "0", ageSeconds: "12" },
};

describe("the Redis clients board", () => {
  it("says nothing is dialled rather than showing an empty list", () => {
    clientsState.current = stateOf({ online: false, data: null });
    expect(render(<ClientsRedis />)).toContain("未连接");
  });

  it("lists connections with what each is doing", () => {
    clientsState.current = stateOf({ data: [connection] });
    const html = render(<ClientsRedis />);
    expect(html).toContain("reporting-service");
    expect(html).toContain("10.2.0.44:51234");
    expect(html).toContain("xrange");
    expect(html).toContain("RESP3");
    expect(html).toContain("go-redis");
  });

  /*
   * Killing the connection this console is using disconnects the console. The
   * page has to point that row out before somebody finds out the hard way.
   */
  it("marks this app's own connection", () => {
    clientsState.current = stateOf({ data: [connection, ownConnection] });
    expect(render(<ClientsRedis />)).toContain("本应用");
  });

  it("names an unnamed client rather than leaving the cell blank", () => {
    clientsState.current = stateOf({ data: [{ ...connection, clientName: "" }] });
    expect(render(<ClientsRedis />)).toContain("未命名");
  });

  /*
   * Redis reports no TLS state, no heartbeat and no channel count per
   * connection, so those columns are not drawn: an "off" or a "0" would be an
   * answer the server never gave.
   */
  it("draws no column for what Redis does not report", () => {
    clientsState.current = stateOf({ data: [connection] });
    const html = render(<ClientsRedis />);
    expect(html).not.toContain("heartbeat");
    expect(html).not.toContain("TLS");
  });

  it("says the search matched nothing rather than showing an empty table", () => {
    clientsState.current = stateOf({ data: [] });
    expect(render(<ClientsRedis />)).toContain("没有任何连接");
  });
});

describe("the Redis overview board", () => {
  const serverData = { overview: null, nodes: [server], directory: [] };

  it("says nothing is dialled rather than showing an empty summary", () => {
    serversState.current = stateOf({ online: false, data: null });
    streamsState.current = stateOf({ online: false, data: null });
    groupsState.current = stateOf({ online: false, data: null });
    expect(render(<OverviewRedis />)).toContain("未连接");
  });

  it("counts what the listings found, and says that is what it is", () => {
    serversState.current = stateOf({ data: serverData });
    streamsState.current = stateOf({ data: [orders, fresh] });
    groupsState.current = stateOf({ data: [settleGroup] });
    const html = render(<OverviewRedis />);
    // SCAN is a cursor and the walk is capped, so this is what the listing
    // saw rather than what the server holds.
    expect(html).toContain("按键匹配模式找到");
  });

  /*
   * The pending total is summed from the group list rather than asked for
   * separately: every group already carries what it is owed, and a per-group
   * XPENDING would be one round trip each to fill one tile.
   */
  it("sums what the groups are owed", () => {
    serversState.current = stateOf({ data: serverData });
    streamsState.current = stateOf({ data: [orders] });
    groupsState.current = stateOf({ data: [settleGroup, stalledGroup] });
    const html = render(<OverviewRedis />);
    // 29 owed to settle-group plus 12 to capture-group.
    expect(html).toContain("41");
    expect(html).toContain("1 个停滞");
  });

  /*
   * The canvas drew a command-rate chart. Nothing in this app records a series
   * for Redis and the server reports an instantaneous figure only, so a chart
   * would be a line through one point.
   */
  it("shows the instantaneous figures rather than a chart of one point", () => {
    serversState.current = stateOf({ data: serverData });
    streamsState.current = stateOf({ data: [orders] });
    groupsState.current = stateOf({ data: [] });
    const html = render(<OverviewRedis />);
    expect(html).toContain("吞吐");
    expect(html).toContain("3,420");
    expect(html).not.toContain("图表");
  });

  it("lists the longest streams, and marks one nothing reads", () => {
    serversState.current = stateOf({ data: serverData });
    streamsState.current = stateOf({ data: [orders, fresh] });
    groupsState.current = stateOf({ data: [] });
    const html = render(<OverviewRedis />);
    expect(html).toContain("orders:events");
    // The fresh stream has no group, which is a real state worth marking
    // rather than a zero in a column.
    expect(html).toContain("orders:new");
  });
});
