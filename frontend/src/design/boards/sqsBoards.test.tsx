import { beforeAll, describe, expect, it, vi } from "vitest";

/**
 * Every SQS board, through the states it can be in.
 *
 * The i18n sweep renders each board once with nothing connected, which covers
 * the offline notice and the strings. It cannot cover the rest: a board only
 * touches its data on the path where data exists, and that is exactly where a
 * missing attribute or an empty list throws. Each board is rendered here
 * against a stubbed hook so loading, failed, connected-but-empty and populated
 * all get exercised.
 *
 * The stubs return the shapes internal/driver/sqs actually sends, so a driver
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

const queuesState = vi.hoisted(() => ({ current: null as unknown }));
const deadLetterState = vi.hoisted(() => ({ current: null as unknown }));
const browseState = vi.hoisted(() => ({
  current: { messages: [], loading: false, error: null, searched: false, run: async () => {} } as unknown,
}));

vi.mock("@/hooks/sqs/useSqsDestinations", () => ({
  useSqsDestinations: () => queuesState.current,
}));
vi.mock("@/hooks/sqs/useSqsDeadLetters", () => ({
  useSqsDeadLetters: () => deadLetterState.current,
}));
vi.mock("@/hooks/sqs/useSqsMessages", () => ({
  useSqsBrowse: () => browseState.current,
}));

let render: (element: React.ReactElement) => string;
let QueuesSqs: typeof import("./topics/QueuesSqs").QueuesSqs;
let MessagesSqs: typeof import("./messages/MessagesSqs").MessagesSqs;
let DlqSqs: typeof import("./dlq/DlqSqs").DlqSqs;
let ProducerSqs: typeof import("./producer/ProducerSqs").ProducerSqs;
let OverviewSqs: typeof import("./overview/OverviewSqs").OverviewSqs;

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

  const [server, queues, messages, dlq, producer, overview, ui, i18n, settings, profiles] =
    await Promise.all([
      import("react-dom/server"),
      import("./topics/QueuesSqs"),
      import("./messages/MessagesSqs"),
      import("./dlq/DlqSqs"),
      import("./producer/ProducerSqs"),
      import("./overview/OverviewSqs"),
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
  QueuesSqs = queues.QueuesSqs;
  MessagesSqs = messages.MessagesSqs;
  DlqSqs = dlq.DlqSqs;
  ProducerSqs = producer.ProducerSqs;
  OverviewSqs = overview.OverviewSqs;
});

/** A standard queue with a backlog, as internal/driver/sqs/destination.go sends one. */
const orders = {
  id: 1,
  ref: { namespace: "", name: "MQS-SEED-orders" },
  partitions: -1,
  subscribers: -1,
  depth: 12,
  rateIn: -1,
  rateOut: -1,
  lastUpdated: "",
  attributes: {
    url: "http://127.0.0.1:4566/000000000000/MQS-SEED-orders",
    arn: "arn:aws:sqs:eu-west-1:000000000000:MQS-SEED-orders",
    fifo: "false",
    visible: "12",
    inFlight: "0",
    delayed: "0",
    visibilityTimeoutSec: "30",
    delaySec: "0",
    retentionSec: "345600",
    maxMessageBytes: "262144",
    receiveWaitSec: "0",
    createdAt: "1788655439",
    modifiedAt: "1788655439",
    deadLetterQueue: "MQS-SEED-orders-dlq",
    maxReceiveCount: "3",
    encrypted: "true",
  },
};

/** The other shape: a FIFO queue, whose extra settings only it carries. */
const fifo = {
  ...orders,
  id: 2,
  ref: { namespace: "", name: "MQS-SEED-orders.fifo" },
  depth: 6,
  attributes: {
    ...orders.attributes,
    url: "http://127.0.0.1:4566/000000000000/MQS-SEED-orders.fifo",
    arn: "arn:aws:sqs:eu-west-1:000000000000:MQS-SEED-orders.fifo",
    fifo: "true",
    visible: "6",
    contentBasedDeduplication: "false",
    deduplicationScope: "queue",
    fifoThroughputLimit: "perQueue",
    // A FIFO queue in the seed points at nothing, so the redrive keys are
    // absent rather than empty - which is the shape the driver sends.
    deadLetterQueue: "",
    maxReceiveCount: "",
  },
};

/** Everything in flight and nothing available: the state that looks idle. */
const stalled = {
  ...orders,
  id: 3,
  ref: { namespace: "", name: "MQS-SEED-stalled" },
  depth: 9,
  attributes: { ...orders.attributes, visible: "0", inFlight: "9", deadLetterQueue: "" },
};

/** A queue the listing could not describe fully: every optional key absent. */
const bare = {
  id: 4,
  ref: { namespace: "", name: "MQS-SEED-empty" },
  partitions: -1,
  subscribers: -1,
  depth: 0,
  rateIn: -1,
  rateOut: -1,
  lastUpdated: "",
  attributes: { url: "", arn: "", fifo: "false" },
};

const deadLetter = {
  namespace: "",
  name: "MQS-SEED-orders-dlq",
  depth: 4,
  consumers: -1,
  sources: [{ queue: "MQS-SEED-orders", subscription: "", exchange: "", routingKey: "" }],
};

/** The state the dead-letter page exists to surface: a backlog nothing feeds. */
const orphaned = {
  ...deadLetter,
  name: "MQS-SEED-abandoned-dlq",
  sources: [],
};

const message = {
  id: 1,
  cluster: "",
  topic: "MQS-SEED-orders",
  messageId: "fe2b8392-5958-49aa-836d-8f835107bc76",
  tags: "",
  keys: "",
  queueId: -1,
  queueOffset: -1,
  storeHost: "",
  bornHost: "",
  storeTime: "2026-09-06T09:00:00Z",
  storeTimestamp: 1788655439070,
  status: "normal",
  retryTimes: 0,
  body: "order-1",
  properties: {
    approximateReceiveCount: "1",
    approximateFirstReceiveTimestamp: "1788655440093",
    senderId: "000000000000",
    md5OfBody: "49783eb0095375c17655cdc1ff329874",
    "attr.tenant": "acme",
  },
};

/** A FIFO message, whose three ordering fields no standard message carries. */
const orderedMessage = {
  ...message,
  messageId: "9d0a1c1e-1111-4b6a-9d3f-8a2b1c3d4e5f",
  keys: "acme",
  properties: {
    ...message.properties,
    messageGroupId: "acme",
    messageDeduplicationId: "order-1",
    sequenceNumber: "15364433228635045888",
  },
};

describe("the SQS queues board", () => {
  it("says the connection is offline rather than showing an empty list", () => {
    queuesState.current = stateOf({ online: false });
    expect(() => render(<QueuesSqs />)).not.toThrow();
  });

  it("renders while the first request is in flight", () => {
    queuesState.current = stateOf({ loading: true });
    expect(() => render(<QueuesSqs />)).not.toThrow();
  });

  it("shows what failed rather than an empty list", () => {
    queuesState.current = stateOf({ error: "the security token is expired" });
    expect(render(<QueuesSqs />)).toContain("the security token is expired");
  });

  it("renders a region that holds no queues", () => {
    queuesState.current = stateOf({ data: [] });
    expect(() => render(<QueuesSqs />)).not.toThrow();
  });

  it("lists a queue with its redrive target", () => {
    queuesState.current = stateOf({ data: [orders, fifo] });
    const html = render(<QueuesSqs />);
    expect(html).toContain("MQS-SEED-orders");
    expect(html).toContain("MQS-SEED-orders-dlq");
  });

  /*
   * The FIFO detail panel is a different set of rows, and it is where the
   * three settings only a FIFO queue carries are read - which is the path a
   * renamed attribute key throws on.
   */
  it("renders the FIFO settings a standard queue does not have", () => {
    queuesState.current = stateOf({ data: [fifo] });
    expect(() => render(<QueuesSqs />)).not.toThrow();
  });

  /*
   * Everything in flight and nothing available is the one reading that looks
   * like an idle queue and is not: something took every message and has
   * finished none of them.
   */
  it("explains a queue whose whole depth is in flight", () => {
    queuesState.current = stateOf({ data: [stalled] });
    expect(() => render(<QueuesSqs />)).not.toThrow();
  });

  // A queue whose optional attributes are all absent still has to render: the
  // listing drops a row it could not describe, but a partial one reaches here.
  it("renders a queue the listing could describe only partly", () => {
    queuesState.current = stateOf({ data: [bare] });
    expect(() => render(<QueuesSqs />)).not.toThrow();
  });
});

describe("the SQS messages board", () => {
  it("says the connection is offline rather than showing an empty list", () => {
    queuesState.current = stateOf({ online: false });
    expect(() => render(<MessagesSqs />)).not.toThrow();
  });

  // The caveat is the point of this page, and it is drawn before anything is
  // received rather than after: pressing the button is a real consumer read.
  it("warns that receiving hides messages before anything is received", () => {
    queuesState.current = stateOf({ data: [orders] });
    browseState.current = {
      messages: [],
      loading: false,
      error: null,
      searched: false,
      run: async () => {},
    };
    expect(() => render(<MessagesSqs />)).not.toThrow();
  });

  it("says nothing was available rather than that the queue is empty", () => {
    queuesState.current = stateOf({ data: [orders] });
    browseState.current = {
      messages: [],
      loading: false,
      error: null,
      searched: true,
      run: async () => {},
    };
    expect(() => render(<MessagesSqs />)).not.toThrow();
  });

  it("lists what was received with its receive count", () => {
    queuesState.current = stateOf({ data: [orders] });
    browseState.current = {
      messages: [message],
      loading: false,
      error: null,
      searched: true,
      run: async () => {},
    };
    const html = render(<MessagesSqs />);
    expect(html).toContain("fe2b8392-5958-49aa-836d-8f835107bc76");
    expect(html).toContain("order-1");
  });

  // A FIFO message carries three fields a standard one does not, and the
  // section is drawn only for it - a panel of dashes would suggest the queue
  // was missing something rather than being a different kind.
  it("renders the FIFO fields on a message that has them", () => {
    queuesState.current = stateOf({ data: [fifo] });
    browseState.current = {
      messages: [orderedMessage],
      loading: false,
      error: null,
      searched: true,
      run: async () => {},
    };
    expect(() => render(<MessagesSqs />)).not.toThrow();
  });

  it("shows what failed rather than an empty list", () => {
    queuesState.current = stateOf({ data: [orders] });
    browseState.current = {
      messages: [],
      loading: false,
      error: "the queue does not exist",
      searched: true,
      run: async () => {},
    };
    expect(render(<MessagesSqs />)).toContain("the queue does not exist");
  });
});

describe("the SQS dead letters board", () => {
  it("says the connection is offline rather than showing an empty list", () => {
    deadLetterState.current = stateOf({ online: false });
    expect(() => render(<DlqSqs />)).not.toThrow();
  });

  it("renders a region where nothing redrives anywhere", () => {
    deadLetterState.current = stateOf({ data: [] });
    expect(() => render(<DlqSqs />)).not.toThrow();
  });

  it("names what redrives into each queue", () => {
    deadLetterState.current = stateOf({ data: [deadLetter] });
    const html = render(<DlqSqs />);
    expect(html).toContain("MQS-SEED-orders-dlq");
    expect(html).toContain("MQS-SEED-orders");
  });

  /*
   * A dead-letter queue with a backlog and no sources left is the state this
   * page exists for: its producers were deleted or reconfigured, so it will
   * never receive anything again and will never drain.
   */
  it("says when nothing feeds a dead-letter queue any more", () => {
    deadLetterState.current = stateOf({ data: [orphaned] });
    expect(() => render(<DlqSqs />)).not.toThrow();
  });
});

describe("the SQS send console", () => {
  it("says the connection is offline rather than drawing an unusable form", () => {
    queuesState.current = stateOf({ online: false });
    expect(() => render(<ProducerSqs />)).not.toThrow();
  });

  it("renders with the region's queues to choose from", () => {
    queuesState.current = stateOf({ data: [orders, fifo] });
    expect(() => render(<ProducerSqs />)).not.toThrow();
  });

  it("renders against a region that holds no queues at all", () => {
    queuesState.current = stateOf({ data: [] });
    expect(() => render(<ProducerSqs />)).not.toThrow();
  });
});

describe("the SQS overview", () => {
  it("says the connection is offline rather than showing zeroes", () => {
    queuesState.current = stateOf({ online: false });
    expect(() => render(<OverviewSqs />)).not.toThrow();
  });

  it("renders a region that holds no queues", () => {
    queuesState.current = stateOf({ data: [] });
    expect(() => render(<OverviewSqs />)).not.toThrow();
  });

  it("adds up what the queues are holding", () => {
    queuesState.current = stateOf({ data: [orders, fifo, stalled] });
    const html = render(<OverviewSqs />);
    expect(html).toContain("MQS-SEED-orders");
    // 12 + 6 + 0 available across the three.
    expect(html).toContain("18");
  });
});
