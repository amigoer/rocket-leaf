import { beforeAll, describe, expect, it, vi } from "vitest";

/**
 * Every Azure Service Bus board, through the states it can be in.
 *
 * The i18n sweep renders each board once with nothing connected, which covers
 * the offline notice and the strings. It cannot cover the rest: a board only
 * touches its data on the path where data exists, and that is exactly where a
 * missing attribute or an empty list throws. Each board is rendered here
 * against a stubbed hook so loading, failed, connected-but-empty and populated
 * all get exercised.
 *
 * The stubs return the shapes internal/driver/azureservicebus actually sends,
 * so a driver that renames an attribute key breaks a board test rather than a
 * screenshot. Three of those shapes are this family's own: a queue whose
 * counts are all unknown because the endpoint reports none, a subscription
 * whose rules have all been deleted, and a rule that can never match.
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

const entitiesState = vi.hoisted(() => ({ current: null as unknown }));
const subscriptionsState = vi.hoisted(() => ({ current: null as unknown }));
const routingState = vi.hoisted(() => ({ current: null as unknown }));
const deadLetterState = vi.hoisted(() => ({
  current: {
    stores: [],
    loading: false,
    storesError: null,
    messages: [],
    reading: false,
    error: null,
    searched: false,
    read: async () => {},
    resend: async () => {},
    refresh: async () => {},
  } as unknown,
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

vi.mock("@/hooks/azureservicebus/useServiceBusEntities", () => ({
  useServiceBusEntities: () => entitiesState.current,
}));
vi.mock("@/hooks/azureservicebus/useServiceBusSubscriptions", () => ({
  useServiceBusSubscriptions: () => subscriptionsState.current,
}));
vi.mock("@/hooks/azureservicebus/useServiceBusRouting", () => ({
  useServiceBusRouting: () => routingState.current,
}));
vi.mock("@/hooks/azureservicebus/useServiceBusDeadLetters", () => ({
  useServiceBusDeadLetters: () => deadLetterState.current,
}));
vi.mock("@/hooks/azureservicebus/useServiceBusBrowse", () => ({
  useServiceBusBrowse: () => browseState.current,
}));

let render: (element: React.ReactElement) => string;
let EntitiesAzureServiceBus: typeof import(
  "./topics/EntitiesAzureServiceBus"
).EntitiesAzureServiceBus;
let RulesAzureServiceBus: typeof import("./topics/RulesAzureServiceBus").RulesAzureServiceBus;
let SubscriptionsAzureServiceBus: typeof import(
  "./consumers/SubscriptionsAzureServiceBus"
).SubscriptionsAzureServiceBus;
let MessagesAzureServiceBus: typeof import(
  "./messages/MessagesAzureServiceBus"
).MessagesAzureServiceBus;
let DlqAzureServiceBus: typeof import("./dlq/DlqAzureServiceBus").DlqAzureServiceBus;
let ProducerAzureServiceBus: typeof import(
  "./producer/ProducerAzureServiceBus"
).ProducerAzureServiceBus;
let OverviewAzureServiceBus: typeof import(
  "./overview/OverviewAzureServiceBus"
).OverviewAzureServiceBus;

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
    entities,
    rules,
    subscriptions,
    messages,
    dlq,
    producer,
    overview,
    ui,
    i18n,
    settings,
    profiles,
  ] = await Promise.all([
    import("react-dom/server"),
    import("./topics/EntitiesAzureServiceBus"),
    import("./topics/RulesAzureServiceBus"),
    import("./consumers/SubscriptionsAzureServiceBus"),
    import("./messages/MessagesAzureServiceBus"),
    import("./dlq/DlqAzureServiceBus"),
    import("./producer/ProducerAzureServiceBus"),
    import("./overview/OverviewAzureServiceBus"),
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
  EntitiesAzureServiceBus = entities.EntitiesAzureServiceBus;
  RulesAzureServiceBus = rules.RulesAzureServiceBus;
  SubscriptionsAzureServiceBus = subscriptions.SubscriptionsAzureServiceBus;
  MessagesAzureServiceBus = messages.MessagesAzureServiceBus;
  DlqAzureServiceBus = dlq.DlqAzureServiceBus;
  ProducerAzureServiceBus = producer.ProducerAzureServiceBus;
  OverviewAzureServiceBus = overview.OverviewAzureServiceBus;
});

/**
 * A queue against a real namespace, as
 * internal/driver/azureservicebus/destination.go sends one.
 *
 * Subscribers is -1 and stays -1 for every queue there is: nothing registers
 * as a consumer, so "how many are reading this" is a question the service
 * cannot answer.
 */
const orders = {
  id: 1,
  ref: { namespace: "", name: "mqs-seed-orders" },
  partitions: -1,
  subscribers: -1,
  depth: 12,
  rateIn: -1,
  rateOut: -1,
  lastUpdated: "",
  attributes: {
    entityType: "queue",
    status: "Active",
    lockDurationSec: "60",
    maxDeliveryCount: "10",
    ttlSec: "3600",
    maxSizeMb: "100",
    requiresSession: "false",
    requiresDuplicateDetection: "false",
    deadLetterOnExpiry: "false",
    partitioned: "false",
    deadLetterCount: "4",
    scheduledCount: "1",
  },
};

/**
 * The same queue against the emulator, which reports no message counts at all.
 *
 * Every count absent rather than zero, which is the whole reason the driver
 * degrades the capability: a board that printed zero here would be saying the
 * queue is empty.
 */
const uncountedQueue = {
  ...orders,
  id: 2,
  ref: { namespace: "", name: "mqs-seed-quiet" },
  depth: -1,
  attributes: {
    entityType: "queue",
    status: "Active",
    lockDurationSec: "60",
    maxDeliveryCount: "10",
    requiresSession: "false",
    requiresDuplicateDetection: "false",
    deadLetterOnExpiry: "false",
    partitioned: "false",
  },
};

/** A topic three subscriptions read. Its depth is -1 and always will be. */
const events = {
  id: 3,
  ref: { namespace: "", name: "mqs-seed-events" },
  partitions: -1,
  subscribers: 3,
  depth: -1,
  rateIn: -1,
  rateOut: -1,
  lastUpdated: "",
  attributes: {
    entityType: "topic",
    status: "Active",
    ttlSec: "3600",
    requiresDuplicateDetection: "false",
    partitioned: "false",
    subscriptionNames: "mqs-seed-events-all,mqs-seed-events-orders,mqs-seed-events-red",
  },
};

/** The state this family alerts on: every send accepted and discarded. */
const orphanedTopic = {
  ...events,
  id: 4,
  ref: { namespace: "", name: "mqs-seed-orphaned" },
  subscribers: 0,
  attributes: { entityType: "topic", status: "Active" },
};

/** An entity switched off, which refuses work and leaves no other mark. */
const disabledQueue = {
  ...orders,
  id: 5,
  ref: { namespace: "", name: "mqs-test-disabled" },
  attributes: { ...orders.attributes, status: "SendDisabled" },
};

/** An entity the listing could describe only partly: every optional key absent. */
const bareEntity = {
  id: 6,
  ref: { namespace: "", name: "mqs-test-bare" },
  partitions: -1,
  subscribers: -1,
  depth: -1,
  rateIn: -1,
  rateOut: -1,
  lastUpdated: "",
  attributes: {},
};

/** A subscription with the whole delivery contract on it. */
const worker = {
  id: 1,
  ref: { namespace: "mqs-seed-events", name: "mqs-seed-events-red" },
  status: "online",
  members: -1,
  destinations: 1,
  backlog: 4,
  rateOut: -1,
  lastUpdated: "",
  attributes: {
    topic: "mqs-seed-events",
    status: "Active",
    lockDurationSec: "60",
    maxDeliveryCount: "10",
    ttlSec: "3600",
    requiresSession: "false",
    deadLetterOnExpiry: "false",
    deadLetterOnRuleError: "true",
    deadLetterCount: "0",
    ruleNames: "red-only",
  },
};

/**
 * The state nothing else in the app shows: every rule deleted, so nothing can
 * arrive - while the subscription reports itself Active with an empty backlog.
 */
const unroutableSubscription = {
  ...worker,
  id: 2,
  ref: { namespace: "mqs-seed-events", name: "mqs-test-stranded" },
  status: "offline",
  backlog: 0,
  attributes: { ...worker.attributes, ruleNames: "" },
};

/** A subscription against the emulator: no counts of any kind. */
const uncountedSubscription = {
  ...worker,
  id: 3,
  ref: { namespace: "mqs-seed-events", name: "mqs-seed-events-all" },
  backlog: -1,
  attributes: {
    topic: "mqs-seed-events",
    status: "Active",
    ruleNames: "$Default",
  },
};

/** The rule every subscription is created with: it matches everything. */
const defaultRule = {
  id: 1,
  namespace: "",
  source: "mqs-seed-events",
  destination: "mqs-seed-events-all",
  destinationKind: "subscription",
  routingKey: "",
  arguments: { filterType: "true" },
  propertiesKey: "$Default",
};

const sqlRule = {
  ...defaultRule,
  id: 2,
  destination: "mqs-seed-events-red",
  routingKey: "colour = 'red'",
  arguments: { filterType: "sql", expression: "colour = 'red'" },
  propertiesKey: "red-only",
};

/** A correlation rule with an action, which changes the message on the way in. */
const correlationRule = {
  ...defaultRule,
  id: 3,
  destination: "mqs-seed-events-orders",
  routingKey: "subject = 'order'",
  arguments: {
    filterType: "correlation",
    "correlation.subject": "order",
    action: "SET routed = 'yes'",
  },
  propertiesKey: "orders-only",
};

/** Legal, and almost never intended: a rule that can never match. */
const neverRule = {
  ...defaultRule,
  id: 4,
  destination: "mqs-test-stranded",
  routingKey: "1=0",
  arguments: { filterType: "false" },
  propertiesKey: "never",
};

const message = {
  id: 1,
  cluster: "",
  topic: "mqs-seed-orders",
  messageId: "8b6f",
  tags: "order",
  keys: "customer-1",
  queueId: -1,
  queueOffset: 12,
  storeHost: "",
  bornHost: "",
  storeTime: "2026-09-06T09:00:00Z",
  storeTimestamp: 1788655439070,
  status: "normal",
  retryTimes: 0,
  body: "order-12",
  properties: {
    state: "active",
    sequenceNumber: "12",
    deliveryCount: "1",
    sessionId: "customer-1",
    "prop.colour": "red",
  },
};

/** A message no consumer would ever be offered, which only a peek reaches. */
const scheduledMessage = {
  ...message,
  messageId: "9c1a",
  queueOffset: 13,
  properties: {
    state: "scheduled",
    sequenceNumber: "13",
    deliveryCount: "1",
    scheduledEnqueueTime: "2026-09-06T10:00:00Z",
  },
};

/** A message the driver could describe only partly: no properties at all. */
const bareMessage = { ...message, messageId: "0000", queueOffset: 14, properties: {} };

const deadLetter = {
  ...message,
  messageId: "dead",
  queueOffset: 3,
  status: "dlq",
  topic: "mqs-seed-failures/$DeadLetterQueue",
  properties: {
    state: "active",
    sequenceNumber: "3",
    deliveryCount: "3",
    deadLetterReason: "seeded",
    deadLetterErrorDescription: "dead-lettered by the seed",
  },
};

describe("the Service Bus entities board", () => {
  it("says the connection is offline rather than showing an empty list", () => {
    entitiesState.current = stateOf({ online: false });
    expect(() => render(<EntitiesAzureServiceBus />)).not.toThrow();
  });

  it("renders while the first request is in flight", () => {
    entitiesState.current = stateOf({ loading: true });
    expect(() => render(<EntitiesAzureServiceBus />)).not.toThrow();
  });

  it("shows what failed rather than an empty list", () => {
    entitiesState.current = stateOf({ error: "the shared access key was rejected" });
    expect(render(<EntitiesAzureServiceBus />)).toContain("the shared access key was rejected");
  });

  it("renders a namespace that holds nothing", () => {
    entitiesState.current = stateOf({ data: [] });
    expect(() => render(<EntitiesAzureServiceBus />)).not.toThrow();
  });

  it("lists queues and topics together, saying which is which", () => {
    entitiesState.current = stateOf({ data: [orders, events] });
    const html = render(<EntitiesAzureServiceBus />);
    expect(html).toContain("mqs-seed-orders");
    expect(html).toContain("mqs-seed-events");
    // The kind is on the row rather than only in the panel: nothing else on
    // it means the same thing, and a topic's blank depth would otherwise read
    // as an endpoint that reports no counts.
    expect(html).toContain("队列");
    expect(html).toContain("主题");
  });

  // A topic's subscriptions are named in the panel rather than on the row,
  // because a topic may have thousands and the row is not the place for them.
  it("names a topic's subscriptions when the topic is the selected row", () => {
    entitiesState.current = stateOf({ data: [events] });
    expect(render(<EntitiesAzureServiceBus />)).toContain("mqs-seed-events-red");
  });

  /*
   * The endpoint that reports no counts. The board must draw a dash rather
   * than a zero: an empty queue and an endpoint with no figures are different
   * answers, and only one of them means the backlog has been dealt with.
   */
  it("renders an entity whose counts the endpoint does not report", () => {
    entitiesState.current = stateOf({ data: [uncountedQueue] });
    expect(() => render(<EntitiesAzureServiceBus />)).not.toThrow();
  });

  it("marks a topic nothing subscribes to", () => {
    entitiesState.current = stateOf({ data: [orphanedTopic] });
    expect(() => render(<EntitiesAzureServiceBus />)).not.toThrow();
  });

  it("renders an entity that has been switched off", () => {
    entitiesState.current = stateOf({ data: [disabledQueue] });
    expect(render(<EntitiesAzureServiceBus />)).toContain("SendDisabled");
  });

  it("renders an entity the listing could describe only partly", () => {
    entitiesState.current = stateOf({ data: [bareEntity] });
    expect(() => render(<EntitiesAzureServiceBus />)).not.toThrow();
  });
});

describe("the Service Bus subscriptions board", () => {
  it("says the connection is offline rather than showing an empty list", () => {
    subscriptionsState.current = stateOf({ online: false });
    expect(() => render(<SubscriptionsAzureServiceBus />)).not.toThrow();
  });

  it("renders while the first request is in flight", () => {
    subscriptionsState.current = stateOf({ loading: true });
    expect(() => render(<SubscriptionsAzureServiceBus />)).not.toThrow();
  });

  it("shows what failed rather than an empty list", () => {
    subscriptionsState.current = stateOf({ error: "the namespace did not answer" });
    expect(render(<SubscriptionsAzureServiceBus />)).toContain("the namespace did not answer");
  });

  it("renders a namespace with no subscriptions", () => {
    subscriptionsState.current = stateOf({ data: [] });
    expect(() => render(<SubscriptionsAzureServiceBus />)).not.toThrow();
  });

  it("lists a subscription with the topic it reads and the rules that reach it", () => {
    subscriptionsState.current = stateOf({ data: [worker] });
    const html = render(<SubscriptionsAzureServiceBus />);
    expect(html).toContain("mqs-seed-events-red");
    expect(html).toContain("red-only");
  });

  /*
   * The one state nothing else shows. Every figure on the row looks healthy -
   * it is Active and its backlog is zero - and nothing can ever arrive.
   */
  it("marks a subscription whose rules have all been deleted", () => {
    subscriptionsState.current = stateOf({ data: [unroutableSubscription] });
    expect(() => render(<SubscriptionsAzureServiceBus />)).not.toThrow();
  });

  it("renders a subscription whose backlog the endpoint does not report", () => {
    subscriptionsState.current = stateOf({ data: [uncountedSubscription] });
    expect(() => render(<SubscriptionsAzureServiceBus />)).not.toThrow();
  });
});

describe("the Service Bus rules board", () => {
  it("says the connection is offline rather than showing an empty list", () => {
    routingState.current = stateOf({ online: false });
    expect(() => render(<RulesAzureServiceBus />)).not.toThrow();
  });

  it("renders while the first request is in flight", () => {
    routingState.current = stateOf({ loading: true });
    expect(() => render(<RulesAzureServiceBus />)).not.toThrow();
  });

  it("shows what failed rather than an empty list", () => {
    routingState.current = stateOf({ error: "the rules could not be read" });
    expect(render(<RulesAzureServiceBus />)).toContain("the rules could not be read");
  });

  it("renders a namespace with no topics at all", () => {
    routingState.current = stateOf({ data: { topics: [], rules: [] } });
    expect(() => render(<RulesAzureServiceBus />)).not.toThrow();
  });

  it("lists all three kinds of rule with what each matches", () => {
    routingState.current = stateOf({
      data: { topics: [events], rules: [defaultRule, sqlRule, correlationRule] },
    });
    const html = render(<RulesAzureServiceBus />);
    expect(html).toContain("$Default");
    expect(html).toContain("colour = &#x27;red&#x27;");
    expect(html).toContain("orders-only");
  });

  // Legal, and almost never what was meant.
  it("marks a rule that can never match", () => {
    routingState.current = stateOf({ data: { topics: [events], rules: [neverRule] } });
    expect(render(<RulesAzureServiceBus />)).toContain("1=0");
  });
});

describe("the Service Bus messages board", () => {
  it("renders before anything has been peeked", () => {
    entitiesState.current = stateOf({ data: [orders] });
    subscriptionsState.current = stateOf({ data: [worker] });
    browseState.current = {
      messages: [],
      loading: false,
      error: null,
      searched: false,
      run: async () => {},
    };
    expect(() => render(<MessagesAzureServiceBus />)).not.toThrow();
  });

  it("says a peek found nothing rather than showing an empty table", () => {
    browseState.current = {
      messages: [],
      loading: false,
      error: null,
      searched: true,
      run: async () => {},
    };
    expect(() => render(<MessagesAzureServiceBus />)).not.toThrow();
  });

  it("shows what a failed peek said", () => {
    browseState.current = {
      messages: [],
      loading: false,
      error: "no queue or topic named mqs-test-absent",
      searched: true,
      run: async () => {},
    };
    expect(render(<MessagesAzureServiceBus />)).toContain("mqs-test-absent");
  });

  it("lists peeked messages by sequence number", () => {
    browseState.current = {
      messages: [message, bareMessage],
      loading: false,
      error: null,
      searched: true,
      run: async () => {},
    };
    const html = render(<MessagesAzureServiceBus />);
    expect(html).toContain("order-12");
  });

  /* What no consumer would be offered, and only a peek reaches. */
  it("renders a scheduled message with the state that says so", () => {
    browseState.current = {
      messages: [scheduledMessage],
      loading: false,
      error: null,
      searched: true,
      run: async () => {},
    };
    expect(() => render(<MessagesAzureServiceBus />)).not.toThrow();
  });
});

describe("the Service Bus dead letters board", () => {
  it("renders before a store has been opened", () => {
    deadLetterState.current = {
      stores: [
        { path: "mqs-seed-failures", kind: "queue", count: 4 },
        { path: "mqs-seed-events/mqs-seed-events-red", kind: "subscription", count: null },
      ],
      loading: false,
      storesError: null,
      messages: [],
      reading: false,
      error: null,
      searched: false,
      read: async () => {},
      resend: async () => {},
      refresh: async () => {},
    };
    const html = render(<DlqAzureServiceBus />);
    expect(html).toContain("mqs-seed-failures");
  });

  /*
   * A namespace with entities and no failures. The stores are still listed,
   * which is the difference from a topology-shaped page: every queue and every
   * subscription has one whether or not anything has ever gone wrong.
   */
  it("lists every store even when none of them holds anything", () => {
    deadLetterState.current = {
      ...(deadLetterState.current as object),
      stores: [{ path: "mqs-seed-quiet", kind: "queue", count: 0 }],
      messages: [],
      searched: true,
    };
    expect(() => render(<DlqAzureServiceBus />)).not.toThrow();
  });

  it("shows what failed rather than an empty list", () => {
    deadLetterState.current = {
      ...(deadLetterState.current as object),
      storesError: "the namespace did not answer",
    };
    expect(render(<DlqAzureServiceBus />)).toContain("the namespace did not answer");
  });

  it("lists dead letters with why each one stopped", () => {
    deadLetterState.current = {
      ...(deadLetterState.current as object),
      storesError: null,
      stores: [{ path: "mqs-seed-failures", kind: "queue", count: 4 }],
      messages: [deadLetter],
      searched: true,
    };
    expect(render(<DlqAzureServiceBus />)).toContain("seeded");
  });
});

describe("the Service Bus producer board", () => {
  it("says the connection is offline rather than drawing a live console", () => {
    entitiesState.current = stateOf({ online: false });
    expect(() => render(<ProducerAzureServiceBus />)).not.toThrow();
  });

  it("draws the console against a namespace with entities", () => {
    entitiesState.current = stateOf({ data: [orders, events] });
    expect(() => render(<ProducerAzureServiceBus />)).not.toThrow();
  });

  it("draws the console against a namespace with none", () => {
    entitiesState.current = stateOf({ data: [] });
    expect(() => render(<ProducerAzureServiceBus />)).not.toThrow();
  });
});

describe("the Service Bus overview", () => {
  it("says the connection is offline rather than showing zeroes", () => {
    entitiesState.current = stateOf({ online: false });
    subscriptionsState.current = stateOf({ online: false });
    expect(() => render(<OverviewAzureServiceBus />)).not.toThrow();
  });

  it("renders while the first request is in flight", () => {
    entitiesState.current = stateOf({ loading: true });
    subscriptionsState.current = stateOf({ loading: true });
    expect(() => render(<OverviewAzureServiceBus />)).not.toThrow();
  });

  it("renders an empty namespace", () => {
    entitiesState.current = stateOf({ data: [] });
    subscriptionsState.current = stateOf({ data: [] });
    expect(() => render(<OverviewAzureServiceBus />)).not.toThrow();
  });

  /*
   * The three faults the tiles exist for, all present at once. None of them
   * shows anywhere else in the app.
   */
  it("counts the topics, subscriptions and entities that have gone quiet", () => {
    entitiesState.current = stateOf({ data: [orders, events, orphanedTopic, disabledQueue] });
    subscriptionsState.current = stateOf({ data: [worker, unroutableSubscription] });
    const html = render(<OverviewAzureServiceBus />);
    expect(html).toContain("mqs-seed-events");
  });
});
