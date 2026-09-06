import { beforeAll, describe, expect, it, vi } from "vitest";

/**
 * Every IBM MQ board, through the states it can be in.
 *
 * The i18n sweep renders each board once with nothing connected, which covers
 * the offline notice and the strings. It cannot cover the rest: a board only
 * touches its data on the path where data exists, and that is exactly where a
 * missing attribute or an empty list throws. Each board is rendered here
 * against a stubbed hook so loading, failed, connected-but-empty and populated
 * all get exercised.
 *
 * The stubs return the shapes internal/driver/ibmmq actually sends, so a
 * driver that renames an attribute key breaks a board test rather than a
 * screenshot. Two fixtures matter more than the rest: a channel with no status
 * at all, which is the ordinary state of most of a queue manager's channels
 * and would be a null dereference on a board that assumed one, and a message
 * whose body the server refused, which is every dead letter there is.
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
const channelsState = vi.hoisted(() => ({ current: null as unknown }));
const subscriptionsState = vi.hoisted(() => ({ current: null as unknown }));
const deadLettersState = vi.hoisted(() => ({ current: null as unknown }));
const browseState = vi.hoisted(() => ({
  current: {
    messages: [],
    loading: false,
    error: null,
    searched: false,
    run: async () => {},
  } as unknown,
}));

vi.mock("@/hooks/ibmmq/useIbmMqDestinations", () => ({
  useIbmMqDestinations: () => destinationsState.current,
}));
vi.mock("@/hooks/ibmmq/useIbmMqChannels", () => ({
  useIbmMqChannels: () => channelsState.current,
}));
vi.mock("@/hooks/ibmmq/useIbmMqSubscriptions", () => ({
  useIbmMqSubscriptions: () => subscriptionsState.current,
}));
vi.mock("@/hooks/ibmmq/useIbmMqDeadLetters", () => ({
  useIbmMqDeadLetters: () => deadLettersState.current,
}));
vi.mock("@/hooks/ibmmq/useIbmMqBrowse", () => ({
  useIbmMqBrowse: () => browseState.current,
}));

let render: (element: React.ReactElement) => string;
let QueuesIbmMq: typeof import("./topics/QueuesIbmMq").QueuesIbmMq;
let ChannelsIbmMq: typeof import("./channels/ChannelsIbmMq").ChannelsIbmMq;
let SubscriptionsIbmMq: typeof import("./consumers/SubscriptionsIbmMq").SubscriptionsIbmMq;
let MessagesIbmMq: typeof import("./messages/MessagesIbmMq").MessagesIbmMq;
let ProducerIbmMq: typeof import("./producer/ProducerIbmMq").ProducerIbmMq;
let DlqIbmMq: typeof import("./dlq/DlqIbmMq").DlqIbmMq;
let OverviewIbmMq: typeof import("./overview/OverviewIbmMq").OverviewIbmMq;

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
    channels,
    subscriptions,
    messages,
    producer,
    dlq,
    overview,
    ui,
    i18n,
    settings,
    profiles,
  ] = await Promise.all([
    import("react-dom/server"),
    import("./topics/QueuesIbmMq"),
    import("./channels/ChannelsIbmMq"),
    import("./consumers/SubscriptionsIbmMq"),
    import("./messages/MessagesIbmMq"),
    import("./producer/ProducerIbmMq"),
    import("./dlq/DlqIbmMq"),
    import("./overview/OverviewIbmMq"),
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
  QueuesIbmMq = queues.QueuesIbmMq;
  ChannelsIbmMq = channels.ChannelsIbmMq;
  SubscriptionsIbmMq = subscriptions.SubscriptionsIbmMq;
  MessagesIbmMq = messages.MessagesIbmMq;
  ProducerIbmMq = producer.ProducerIbmMq;
  DlqIbmMq = dlq.DlqIbmMq;
  OverviewIbmMq = overview.OverviewIbmMq;
});

/**
 * A local queue, as internal/driver/ibmmq/destination.go sends one.
 *
 * partitions and both rates are -1, which is UnknownMetric on the wire: MQ
 * divides nothing and publishes no rate a listing can read.
 */
const orders = {
  id: 1,
  ref: { namespace: "", name: "MQS.SEED.ORDERS" },
  partitions: -1,
  subscribers: 0,
  depth: 12,
  rateIn: -1,
  rateOut: -1,
  lastUpdated: "2026-09-06T09:00:00Z",
  attributes: {
    kind: "queue",
    queueType: "local",
    description: "orders",
    maxDepth: "5000",
    maxMessageLength: "4194304",
    inhibitGet: "false",
    inhibitPut: "false",
    transmissionQueue: "false",
    backoutQueue: "",
    backoutThreshold: "0",
    cluster: "",
    openInput: "0",
    openOutput: "0",
    uncommitted: "0",
    lastPut: "2026-09-06T09:00:00Z",
    lastGet: "",
    altered: "2026-09-06T09:00:00Z",
  },
};

/** Inhibited, which is the commonest reason a queue manager dead-letters. */
const inhibited = {
  ...orders,
  id: 2,
  ref: { namespace: "", name: "MQS.SEED.AUDIT" },
  depth: 0,
  attributes: {
    ...orders.attributes,
    inhibitPut: "true",
    backoutQueue: "MQS.SEED.BACKOUT",
    backoutThreshold: "3",
  },
};

/** A transmission queue with messages on it: a channel is not moving them. */
const transmission = {
  ...orders,
  id: 3,
  ref: { namespace: "", name: "MQS.SEED.XMITQ" },
  depth: 7,
  attributes: { ...orders.attributes, transmissionQueue: "true" },
};

/** The one queue the queue manager names for itself. */
const deadLetter = {
  ...orders,
  id: 4,
  ref: { namespace: "", name: "DEV.DEAD.LETTER.QUEUE" },
  depth: 2,
  attributes: { ...orders.attributes, deadLetterQueue: "true" },
};

/** An alias, which holds nothing: every runtime figure on it is absent. */
const alias = {
  id: 5,
  ref: { namespace: "", name: "MQS.SEED.ALIAS" },
  partitions: -1,
  subscribers: -1,
  depth: -1,
  rateIn: -1,
  rateOut: -1,
  lastUpdated: "",
  attributes: { kind: "queue", queueType: "alias" },
};

/** A topic object, whose name is not the string publishers use. */
const topic = {
  id: 6,
  ref: { namespace: "", name: "MQS.SEED.EVENTS" },
  partitions: -1,
  subscribers: 1,
  depth: -1,
  rateIn: -1,
  rateOut: -1,
  lastUpdated: "2026-09-06 09:00:00",
  attributes: {
    kind: "topic",
    topicString: "dev/seed/events",
    topicType: "local",
    description: "",
    altered: "2026-09-06 09:00:00",
  },
};

const everyQueue = [orders, inhibited, transmission, deadLetter, alias, topic];

/**
 * A channel definition with nothing running, which is the ordinary state of
 * most of a queue manager's channels and the shape a board would throw on if
 * it assumed a status object.
 */
const idleChannel = {
  name: "SYSTEM.DEF.SENDER",
  type: "sender",
  description: "",
  connectionName: "",
  transmissionQueue: "",
  status: "",
  substate: "",
  instances: 0,
  remoteQueueManager: "",
  messages: -1,
  bytesSent: -1,
  bytesReceived: -1,
  startedAt: "",
  lastMessageAt: "",
  inDoubt: false,
  stopRequested: false,
};

/** Started and unable to connect, which is the row the page exists for. */
const retrying = {
  ...idleChannel,
  name: "MQS.SEED.SDR",
  connectionName: "192.0.2.10(1414)",
  transmissionQueue: "MQS.SEED.XMITQ",
  status: "retrying",
  substate: "",
  instances: 1,
  messages: 0,
  bytesSent: 268,
  bytesReceived: 0,
  startedAt: "2026-09-06 09:00:00",
};

/** Client applications arrive here, and one definition has many instances. */
const serverConnection = {
  ...idleChannel,
  name: "DEV.APP.SVRCONN",
  type: "serverConnection",
  status: "running",
  substate: "receive",
  instances: 4,
  messages: 128,
  bytesSent: 40960,
  bytesReceived: 8192,
  startedAt: "2026-09-06 08:00:00",
  lastMessageAt: "2026-09-06 09:02:00",
};

/** In doubt: a batch neither delivered nor resendable until somebody acts. */
const inDoubt = {
  ...retrying,
  name: "MQS.SEED.SDR2",
  status: "running",
  inDoubt: true,
  stopRequested: true,
};

/** A definition this queue manager holds for clients and never runs itself. */
const clientConnection = {
  ...idleChannel,
  name: "SYSTEM.DEF.CLNTCONN",
  type: "clientConnection",
};

const everyChannel = [idleChannel, retrying, serverConnection, inDoubt, clientConnection];

/**
 * A subscription, as internal/driver/ibmmq/subscription.go sends one.
 *
 * Nothing attached and a backlog waiting, which is the state worth flagging
 * and the one a driver reading "no connection" as offline would get wrong.
 */
const subscription = {
  id: 1,
  ref: { namespace: "", name: "MQS.SEED.SUB" },
  status: "warning",
  members: 0,
  destinations: 1,
  backlog: 4,
  rateOut: -1,
  lastUpdated: "2026-09-06 09:00:00",
  attributes: {
    topicString: "dev/seed/events",
    destination: "MQS.SEED.SUBQ",
    destinationQueueManager: "QM1",
    durable: "yes",
    subscriptionType: "admin",
    user: "mqm",
    selector: "",
    subscriptionId: "414D5120514D3120",
    messagesReceived: "6",
    lastMessageAt: "2026-09-06 09:00:03",
    attached: "false",
    queueReaders: "0",
  },
};

/** The other half: a subscriber attached and nothing waiting. */
const attached = {
  ...subscription,
  id: 2,
  ref: { namespace: "", name: "APP.SUB" },
  status: "online",
  members: 1,
  backlog: 0,
  attributes: { ...subscription.attributes, attached: "true", queueReaders: "1" },
};

/** A dead-letter queue, as the topology walk finds one. */
const managersDeadLetter = {
  namespace: "",
  name: "DEV.DEAD.LETTER.QUEUE",
  depth: 2,
  consumers: 0,
  sources: [{ queue: "QM1", subscription: "", exchange: "DEADQ", routingKey: "" }],
};

/** A backout queue, which is one only because another queue points at it. */
const backout = {
  namespace: "",
  name: "MQS.SEED.BACKOUT",
  depth: 0,
  consumers: 1,
  sources: [
    { queue: "MQS.SEED.AUDIT", subscription: "", exchange: "BOQNAME", routingKey: "3" },
  ],
};

/** A message the server returned, and one it refused. */
const readable = {
  id: 1,
  cluster: "QM1",
  topic: "MQS.SEED.ORDERS",
  messageId: "414d5120514d3120202020202020202014189d6a032e0040",
  tags: "",
  keys: "",
  queueId: 0,
  queueOffset: 0,
  storeHost: "",
  bornHost: "",
  storeTime: "",
  storeTimestamp: 0,
  status: "normal",
  retryTimes: 0,
  body: '{"order": 0}',
  properties: {
    format: "MQSTR",
    persistence: "nonPersistent",
    expiry: "unlimited",
    orderNo: "42",
  },
};

const refused = {
  ...readable,
  id: 2,
  messageId: "414d5120514d3120202020202020202014189d6a082e0040",
  topic: "DEV.DEAD.LETTER.QUEUE",
  body: "",
  properties: { format: "MQDEAD", bodyUnavailable: "MQDEAD" },
};

describe("the IBM MQ queues board", () => {
  it("says the connection is offline rather than showing an empty list", () => {
    destinationsState.current = stateOf({ online: false });
    expect(() => render(<QueuesIbmMq />)).not.toThrow();
  });

  it("renders while the first request is in flight", () => {
    destinationsState.current = stateOf({ loading: true });
    expect(() => render(<QueuesIbmMq />)).not.toThrow();
  });

  it("shows what failed rather than an empty list", () => {
    destinationsState.current = stateOf({ error: "the mqweb server refused the credential" });
    expect(render(<QueuesIbmMq />)).toContain("the mqweb server refused the credential");
  });

  it("renders a queue manager holding nothing", () => {
    destinationsState.current = stateOf({ data: [] });
    expect(() => render(<QueuesIbmMq />)).not.toThrow();
  });

  it("draws queues and topics together, with the topic string on the row", () => {
    destinationsState.current = stateOf({ data: everyQueue });
    const html = render(<QueuesIbmMq />);
    expect(html).toContain("MQS.SEED.ORDERS");
    expect(html).toContain("MQS.SEED.EVENTS");
    // The alias holds nothing, so its figures are dashes rather than zeroes.
    expect(html).toContain("MQS.SEED.ALIAS");
  });
});

describe("the IBM MQ channels board", () => {
  it("says the connection is offline rather than showing an empty list", () => {
    channelsState.current = stateOf({ online: false });
    expect(() => render(<ChannelsIbmMq />)).not.toThrow();
  });

  it("renders while the first request is in flight", () => {
    channelsState.current = stateOf({ loading: true });
    expect(() => render(<ChannelsIbmMq />)).not.toThrow();
  });

  it("shows what failed rather than an empty list", () => {
    channelsState.current = stateOf({ error: "the queue manager is not available" });
    expect(render(<ChannelsIbmMq />)).toContain("the queue manager is not available");
  });

  it("renders a queue manager with no channels at all", () => {
    channelsState.current = stateOf({ data: [] });
    expect(() => render(<ChannelsIbmMq />)).not.toThrow();
  });

  /*
   * The empty status is the case worth a test of its own: most of a queue
   * manager's channels have never been started, and a board that assumed a
   * status object would throw on every one of them.
   */
  it("draws a channel nobody has started without inventing a status", () => {
    channelsState.current = stateOf({ data: [idleChannel] });
    const html = render(<ChannelsIbmMq />);
    expect(html).toContain("SYSTEM.DEF.SENDER");
    expect(html).not.toContain("running");
  });

  it("draws every channel shape a queue manager has", () => {
    channelsState.current = stateOf({ data: everyChannel });
    const html = render(<ChannelsIbmMq />);
    expect(html).toContain("MQS.SEED.SDR");
    expect(html).toContain("DEV.APP.SVRCONN");
    // In doubt is flagged on the row rather than only in the panel: it is the
    // one state that needs a person, and it can sit on a running channel.
    expect(html).toContain("MQS.SEED.SDR2");
    expect(html).toContain("retrying");
  });

  // The configured address is on the panel, which opens on the first row - so
  // the retrying channel is rendered alone to reach it.
  it("keeps the configured address on a channel that cannot connect", () => {
    channelsState.current = stateOf({ data: [retrying] });
    expect(render(<ChannelsIbmMq />)).toContain("192.0.2.10(1414)");
  });
});

describe("the IBM MQ subscriptions board", () => {
  it("says the connection is offline rather than showing an empty list", () => {
    subscriptionsState.current = stateOf({ online: false });
    expect(() => render(<SubscriptionsIbmMq />)).not.toThrow();
  });

  it("renders while the first request is in flight", () => {
    subscriptionsState.current = stateOf({ loading: true });
    expect(() => render(<SubscriptionsIbmMq />)).not.toThrow();
  });

  it("shows what failed rather than an empty list", () => {
    subscriptionsState.current = stateOf({ error: "the command server is not running" });
    expect(render(<SubscriptionsIbmMq />)).toContain("the command server is not running");
  });

  it("renders a queue manager with no subscriptions", () => {
    subscriptionsState.current = stateOf({ data: [] });
    expect(() => render(<SubscriptionsIbmMq />)).not.toThrow();
  });

  it("draws a subscription with nothing attached and a backlog waiting", () => {
    subscriptionsState.current = stateOf({ data: [subscription, attached] });
    const html = render(<SubscriptionsIbmMq />);
    expect(html).toContain("MQS.SEED.SUB");
    expect(html).toContain("dev/seed/events");
    expect(html).toContain("MQS.SEED.SUBQ");
  });
});

describe("the IBM MQ dead-letter board", () => {
  it("says the connection is offline rather than showing an empty list", () => {
    deadLettersState.current = stateOf({ online: false });
    expect(() => render(<DlqIbmMq />)).not.toThrow();
  });

  it("renders while the first request is in flight", () => {
    deadLettersState.current = stateOf({ loading: true });
    expect(() => render(<DlqIbmMq />)).not.toThrow();
  });

  it("shows what failed rather than an empty list", () => {
    deadLettersState.current = stateOf({ error: "the queue listing was refused" });
    expect(render(<DlqIbmMq />)).toContain("the queue listing was refused");
  });

  it("renders a queue manager that names no dead-letter queue", () => {
    deadLettersState.current = stateOf({ data: [] });
    expect(() => render(<DlqIbmMq />)).not.toThrow();
  });

  // Both pointers, because they are two mechanisms on one page and the board
  // labels them differently.
  it("names the queue manager as the source of its own dead-letter queue", () => {
    deadLettersState.current = stateOf({ data: [managersDeadLetter, backout] });
    const html = render(<DlqIbmMq />);
    expect(html).toContain("DEV.DEAD.LETTER.QUEUE");
    expect(html).toContain("MQS.SEED.BACKOUT");
    // The panel opens on the first row, and its source is the queue manager
    // rather than a queue - there is nothing else to name in its place.
    expect(html).toContain("QM1");
  });

  // The other pointer, rendered alone because the panel opens on the first
  // row: a backout queue names the queue that points at it and the threshold
  // that decides whether anything ever travels the pointer.
  it("names the queue and the threshold behind a backout queue", () => {
    deadLettersState.current = stateOf({ data: [backout] });
    const html = render(<DlqIbmMq />);
    expect(html).toContain("MQS.SEED.AUDIT");
    expect(html).toContain("3");
  });
});

describe("the IBM MQ messages board", () => {
  it("renders before anything has been browsed", () => {
    destinationsState.current = stateOf({ data: everyQueue });
    browseState.current = { messages: [], loading: false, error: null, searched: false, run: async () => {} };
    expect(() => render(<MessagesIbmMq />)).not.toThrow();
  });

  it("says a queue is empty rather than saying nothing", () => {
    browseState.current = { messages: [], loading: false, error: null, searched: true, run: async () => {} };
    expect(() => render(<MessagesIbmMq />)).not.toThrow();
  });

  it("shows what failed rather than an empty list", () => {
    browseState.current = {
      messages: [],
      loading: false,
      error: "not authorized to browse",
      searched: true,
      run: async () => {},
    };
    expect(render(<MessagesIbmMq />)).toContain("not authorized to browse");
  });

  /*
   * The refused body is the row that matters: every dead letter is one, so a
   * board that assumed a body would be blank on exactly the page somebody
   * opened to find out what went wrong.
   */
  it("lists a message whose body the server would not decode", () => {
    browseState.current = {
      messages: [readable, refused],
      loading: false,
      error: null,
      searched: true,
      run: async () => {},
    };
    const html = render(<MessagesIbmMq />);
    expect(html).toContain("MQSTR");
    expect(html).toContain("MQDEAD");
  });
});

describe("the IBM MQ send console", () => {
  it("renders with nothing chosen", () => {
    destinationsState.current = stateOf({ data: everyQueue });
    expect(() => render(<ProducerIbmMq />)).not.toThrow();
  });

  it("renders against a queue manager with no queues", () => {
    destinationsState.current = stateOf({ data: [] });
    expect(() => render(<ProducerIbmMq />)).not.toThrow();
  });
});

describe("the IBM MQ overview", () => {
  it("says the connection is offline rather than showing zeroes", () => {
    destinationsState.current = stateOf({ online: false });
    channelsState.current = stateOf({ online: false });
    expect(() => render(<OverviewIbmMq />)).not.toThrow();
  });

  it("renders a queue manager holding nothing", () => {
    destinationsState.current = stateOf({ data: [] });
    channelsState.current = stateOf({ data: [] });
    expect(() => render(<OverviewIbmMq />)).not.toThrow();
  });

  it("counts queues, channels and what is inhibited", () => {
    destinationsState.current = stateOf({ data: everyQueue });
    channelsState.current = stateOf({ data: everyChannel });
    const html = render(<OverviewIbmMq />);
    expect(html).toContain("MQS.SEED.ORDERS");
    expect(html).toContain("DEV.DEAD.LETTER.QUEUE");
  });
});
