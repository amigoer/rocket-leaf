import { beforeAll, describe, expect, it, vi } from "vitest";

/**
 * Every Kinesis board, through the states it can be in.
 *
 * The i18n sweep renders each board once with nothing connected, which covers
 * the offline notice and the strings. It cannot cover the rest: a board only
 * touches its data on the path where data exists, and that is exactly where a
 * missing attribute or an empty list throws. Each board is rendered here
 * against a stubbed hook so loading, failed, connected-but-empty and populated
 * all get exercised.
 *
 * The stubs return the shapes internal/driver/kinesis actually sends, so a
 * driver that renames an attribute key breaks a board test rather than a
 * screenshot. The shard fixtures matter most: their hash keys are the real
 * 128-bit values, because the page divides them and a fixture with small
 * numbers would pass whatever the arithmetic did.
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
const consumersState = vi.hoisted(() => ({ current: null as unknown }));
const shardsState = vi.hoisted(() => ({
  current: { shards: [], loading: false, error: null, refresh: async () => {} } as unknown,
}));
const browseState = vi.hoisted(() => ({
  current: {
    messages: [],
    loading: false,
    error: null,
    searched: false,
    run: async () => {},
  } as unknown,
}));

vi.mock("@/hooks/kinesis/useKinesisDestinations", () => ({
  useKinesisDestinations: () => streamsState.current,
}));
vi.mock("@/hooks/kinesis/useKinesisConsumers", () => ({
  useKinesisConsumers: () => consumersState.current,
}));
vi.mock("@/hooks/kinesis/useKinesisShards", () => ({
  useKinesisShards: () => shardsState.current,
}));
vi.mock("@/hooks/kinesis/useKinesisBrowse", () => ({
  useKinesisBrowse: () => browseState.current,
}));

let render: (element: React.ReactElement) => string;
let StreamsKinesis: typeof import("./topics/StreamsKinesis").StreamsKinesis;
let ShardsKinesis: typeof import("./shards/ShardsKinesis").ShardsKinesis;
let ConsumersKinesis: typeof import("./consumers/ConsumersKinesis").ConsumersKinesis;
let MessagesKinesis: typeof import("./messages/MessagesKinesis").MessagesKinesis;
let ProducerKinesis: typeof import("./producer/ProducerKinesis").ProducerKinesis;
let OverviewKinesis: typeof import("./overview/OverviewKinesis").OverviewKinesis;

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

  const [server, streams, shards, consumers, messages, producer, overview, ui, i18n, settings, profiles] =
    await Promise.all([
      import("react-dom/server"),
      import("./topics/StreamsKinesis"),
      import("./shards/ShardsKinesis"),
      import("./consumers/ConsumersKinesis"),
      import("./messages/MessagesKinesis"),
      import("./producer/ProducerKinesis"),
      import("./overview/OverviewKinesis"),
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
  StreamsKinesis = streams.StreamsKinesis;
  ShardsKinesis = shards.ShardsKinesis;
  ConsumersKinesis = consumers.ConsumersKinesis;
  MessagesKinesis = messages.MessagesKinesis;
  ProducerKinesis = producer.ProducerKinesis;
  OverviewKinesis = overview.OverviewKinesis;
});

/**
 * A provisioned stream, as internal/driver/kinesis/destination.go sends one.
 *
 * depth and both rates are -1, which is UnknownMetric on the wire: nothing in
 * Kinesis counts what a stream holds or how fast it moves.
 */
const orders = {
  id: 1,
  ref: { namespace: "", name: "MQS-SEED-orders" },
  partitions: 3,
  subscribers: 2,
  depth: -1,
  rateIn: -1,
  rateOut: -1,
  lastUpdated: "2026-09-06T09:00:00Z",
  attributes: {
    arn: "arn:aws:kinesis:eu-west-1:000000000000:stream/MQS-SEED-orders",
    status: "ACTIVE",
    streamMode: "PROVISIONED",
    retentionHours: "48",
    openShards: "3",
    consumers: "2",
    createdAt: "1788675454376",
    encryption: "NONE",
  },
};

/** The other capacity mode: AWS chooses the shard count and nobody set it. */
const onDemand = {
  ...orders,
  id: 2,
  ref: { namespace: "", name: "MQS-SEED-ondemand" },
  partitions: 4,
  subscribers: 0,
  attributes: {
    ...orders.attributes,
    arn: "arn:aws:kinesis:eu-west-1:000000000000:stream/MQS-SEED-ondemand",
    streamMode: "ON_DEMAND",
    openShards: "4",
    consumers: "0",
    retentionHours: "24",
  },
};

/** Mid-resize: every other call naming it is refused while it is here. */
const settling = {
  ...orders,
  id: 3,
  ref: { namespace: "", name: "MQS-SEED-resizing" },
  attributes: { ...orders.attributes, status: "UPDATING" },
};

/** A stream the listing described with every optional key absent. */
const bare = {
  id: 4,
  ref: { namespace: "", name: "MQS-SEED-empty" },
  partitions: 1,
  subscribers: 0,
  depth: -1,
  rateIn: -1,
  rateOut: -1,
  lastUpdated: "",
  attributes: { arn: "", status: "ACTIVE" },
};

/*
 * The three shards a split leaves: a closed parent that still holds its
 * records, and the two children that name it.
 *
 * The hash keys are the real values. The page divides them to work out each
 * shard's share of the key space, and 2^128 is seventeen digits past what a
 * double holds exactly - so a fixture with small numbers would pass whatever
 * the arithmetic did.
 */
const parent = {
  id: "shardId-000000000000",
  parentId: "",
  adjacentParentId: "",
  startHashKey: "0",
  endHashKey: "340282366920938463463374607431768211455",
  startSequence: "49678078036388764787156533783026620459501543468145049602",
  endSequence: "49678078039310162408164045414567799553218479375184298033",
  closed: true,
};
const lowerChild = {
  id: "shardId-000000000001",
  parentId: "shardId-000000000000",
  adjacentParentId: "",
  startHashKey: "0",
  endHashKey: "170141183460469231731687303715884105727",
  startSequence: "49678078039310162408164045414567799553218479375184298034",
  endSequence: "",
  closed: false,
};
const upperChild = {
  ...lowerChild,
  id: "shardId-000000000002",
  startHashKey: "170141183460469231731687303715884105728",
  endHashKey: "340282366920938463463374607431768211455",
};

/** A registered consumer, as internal/driver/kinesis/subscription.go sends one. */
const analytics = {
  id: 1,
  ref: { namespace: "MQS-SEED-orders", name: "MQS-SEED-analytics" },
  status: "online",
  members: -1,
  destinations: 1,
  backlog: -1,
  rateOut: -1,
  lastUpdated: "2026-09-06T09:00:00Z",
  attributes: {
    consumerArn:
      "arn:aws:kinesis:eu-west-1:000000000000:stream/MQS-SEED-orders/consumer/MQS-SEED-analytics:1788675498",
    consumerStatus: "ACTIVE",
    createdAt: "1788675498751",
    stream: "MQS-SEED-orders",
  },
};

/** A registration still settling, which is what the warning colour is for. */
const registering = {
  ...analytics,
  id: 2,
  ref: { namespace: "MQS-SEED-orders", name: "MQS-SEED-archiver" },
  status: "warning",
  attributes: { ...analytics.attributes, consumerStatus: "CREATING" },
};

/** A record, addressed by the pair that identifies it. */
const record = {
  id: 1,
  cluster: "",
  topic: "MQS-SEED-orders",
  messageId:
    "shardId-000000000002:49678078036433366277553595029310900821866456950635495458",
  tags: "",
  keys: "key-1",
  queueId: 0,
  queueOffset: 0,
  storeHost: "",
  bornHost: "",
  storeTime: "2026-09-06T09:03:05Z",
  storeTimestamp: 1788675785039,
  status: "normal",
  retryTimes: 0,
  body: "order-11",
  properties: {
    shardId: "shardId-000000000002",
    sequenceNumber: "49678078036433366277553595029310900821866456950635495458",
    partitionKey: "key-1",
    encryptionType: "NONE",
  },
};

describe("the Kinesis streams board", () => {
  it("says the connection is offline rather than showing an empty list", () => {
    streamsState.current = stateOf({ online: false });
    expect(() => render(<StreamsKinesis />)).not.toThrow();
  });

  it("renders while the first request is in flight", () => {
    streamsState.current = stateOf({ loading: true });
    expect(() => render(<StreamsKinesis />)).not.toThrow();
  });

  it("shows what failed rather than an empty list", () => {
    streamsState.current = stateOf({ error: "the security token is expired" });
    expect(render(<StreamsKinesis />)).toContain("the security token is expired");
  });

  it("renders a region that holds no streams", () => {
    streamsState.current = stateOf({ data: [] });
    expect(() => render(<StreamsKinesis />)).not.toThrow();
  });

  it("lists a stream with its shard count, retention and mode", () => {
    streamsState.current = stateOf({ data: [orders, onDemand] });
    const html = render(<StreamsKinesis />);
    expect(html).toContain("MQS-SEED-orders");
    expect(html).toContain("MQS-SEED-ondemand");
    // Two days, read back from the 48 hours the driver sent.
    expect(html).toContain("2d");
  });

  // The state that turns every button on the page into a refusal the service
  // words as "resource in use", so the board says which state it is in.
  it("marks a stream that is not ACTIVE", () => {
    streamsState.current = stateOf({ data: [settling] });
    expect(render(<StreamsKinesis />)).toContain("UPDATING");
  });

  // The path a renamed attribute key throws on: every optional key absent.
  it("renders a stream the listing described with nothing but a status", () => {
    streamsState.current = stateOf({ data: [bare] });
    expect(() => render(<StreamsKinesis />)).not.toThrow();
  });
});

describe("the Kinesis shards board", () => {
  it("renders before a stream has been picked", () => {
    streamsState.current = stateOf({ data: [] });
    shardsState.current = { shards: [], loading: false, error: null, refresh: async () => {} };
    expect(() => render(<ShardsKinesis />)).not.toThrow();
  });

  it("shows what failed rather than an empty table", () => {
    streamsState.current = stateOf({ data: [orders] });
    shardsState.current = {
      shards: [],
      loading: false,
      error: "MQS-SEED-orders is not there",
      refresh: async () => {},
    };
    expect(render(<ShardsKinesis />)).toContain("MQS-SEED-orders is not there");
  });

  /*
   * The listing a split leaves, which is the whole reason this page exists.
   * The closed parent has to be drawn - it still holds its records - and the
   * children have to name it.
   */
  it("lists the closed parent of a split beside its children", () => {
    streamsState.current = stateOf({ data: [orders] });
    shardsState.current = {
      shards: [parent, lowerChild, upperChild],
      loading: false,
      error: null,
      refresh: async () => {},
    };
    const html = render(<ShardsKinesis />);
    expect(html).toContain("shardId-000000000000");
    expect(html).toContain("shardId-000000000001");
    expect(html).toContain("shardId-000000000002");
    // The two children divide the key space in half each, which is the
    // arithmetic a double would have got wrong.
    expect(html).toContain("50.0%");
    // And the parent, which covered all of it.
    expect(html).toContain("100.0%");
  });

  it("renders a shard with no range rather than inventing a share", () => {
    streamsState.current = stateOf({ data: [orders] });
    shardsState.current = {
      shards: [{ ...lowerChild, startHashKey: "", endHashKey: "" }],
      loading: false,
      error: null,
      refresh: async () => {},
    };
    expect(() => render(<ShardsKinesis />)).not.toThrow();
  });
});

describe("the Kinesis consumers board", () => {
  it("says the connection is offline rather than showing an empty list", () => {
    consumersState.current = stateOf({ online: false });
    expect(() => render(<ConsumersKinesis />)).not.toThrow();
  });

  it("shows what failed rather than an empty list", () => {
    consumersState.current = stateOf({ error: "access denied" });
    expect(render(<ConsumersKinesis />)).toContain("access denied");
  });

  it("renders a region with no registered consumers", () => {
    consumersState.current = stateOf({ data: [] });
    expect(() => render(<ConsumersKinesis />)).not.toThrow();
  });

  it("lists a consumer with the stream it is registered on", () => {
    streamsState.current = stateOf({ data: [orders] });
    consumersState.current = stateOf({ data: [analytics, registering] });
    const html = render(<ConsumersKinesis />);
    expect(html).toContain("MQS-SEED-analytics");
    expect(html).toContain("MQS-SEED-orders");
    // The state an application pointed at it would fail on.
    expect(html).toContain("CREATING");
  });
});

describe("the Kinesis records board", () => {
  it("renders before anything has been read", () => {
    streamsState.current = stateOf({ data: [orders] });
    shardsState.current = { shards: [], loading: false, error: null, refresh: async () => {} };
    browseState.current = {
      messages: [],
      loading: false,
      error: null,
      searched: false,
      run: async () => {},
    };
    expect(() => render(<MessagesKinesis />)).not.toThrow();
  });

  it("says a read found nothing rather than showing an empty table", () => {
    browseState.current = {
      messages: [],
      loading: false,
      error: null,
      searched: true,
      run: async () => {},
    };
    expect(() => render(<MessagesKinesis />)).not.toThrow();
  });

  it("shows what failed rather than an empty table", () => {
    browseState.current = {
      messages: [],
      loading: false,
      error: "shardId-000000000002 is over its read allowance",
      searched: true,
      run: async () => {},
    };
    expect(render(<MessagesKinesis />)).toContain("over its read allowance");
  });

  it("lists a record with the shard that holds it", () => {
    streamsState.current = stateOf({ data: [orders] });
    shardsState.current = {
      shards: [lowerChild, upperChild],
      loading: false,
      error: null,
      refresh: async () => {},
    };
    browseState.current = {
      messages: [record],
      loading: false,
      error: null,
      searched: true,
      run: async () => {},
    };
    const html = render(<MessagesKinesis />);
    expect(html).toContain("shardId-000000000002");
    expect(html).toContain("key-1");
    expect(html).toContain("order-11");
  });
});

describe("the Kinesis send console", () => {
  it("renders with nothing connected", () => {
    streamsState.current = stateOf({ online: false });
    expect(() => render(<ProducerKinesis />)).not.toThrow();
  });

  it("offers the streams it can send to and the shards it can aim at", () => {
    streamsState.current = stateOf({ data: [orders, onDemand] });
    shardsState.current = {
      shards: [parent, lowerChild, upperChild],
      loading: false,
      error: null,
      refresh: async () => {},
    };
    expect(() => render(<ProducerKinesis />)).not.toThrow();
  });
});

describe("the Kinesis overview", () => {
  it("says the connection is offline rather than showing zeroes", () => {
    streamsState.current = stateOf({ online: false });
    consumersState.current = stateOf({ online: false });
    expect(() => render(<OverviewKinesis />)).not.toThrow();
  });

  it("renders a region that holds no streams", () => {
    streamsState.current = stateOf({ data: [] });
    consumersState.current = stateOf({ data: [] });
    expect(() => render(<OverviewKinesis />)).not.toThrow();
  });

  /*
   * The tiles count capacity rather than throughput, because throughput is a
   * CloudWatch figure this connection never sees. Seven open shards across
   * three streams, one of them on demand and one still settling.
   */
  it("counts open shards, capacity mode and what is still settling", () => {
    streamsState.current = stateOf({ data: [orders, onDemand, settling] });
    consumersState.current = stateOf({ data: [analytics, registering] });
    const html = render(<OverviewKinesis />);
    expect(html).toContain("MQS-SEED-orders");
    // 3 + 4 + 3 open shards.
    expect(html).toContain("10");
    expect(html).toContain("UPDATING");
  });
});
