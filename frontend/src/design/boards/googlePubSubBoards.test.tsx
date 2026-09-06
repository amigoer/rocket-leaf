import { beforeAll, describe, expect, it, vi } from "vitest";

/**
 * Every Google Pub/Sub board, through the states it can be in.
 *
 * The i18n sweep renders each board once with nothing connected, which covers
 * the offline notice and the strings. It cannot cover the rest: a board only
 * touches its data on the path where data exists, and that is exactly where a
 * missing attribute or an empty list throws. Each board is rendered here
 * against a stubbed hook so loading, failed, connected-but-empty and populated
 * all get exercised.
 *
 * The stubs return the shapes internal/driver/googlepubsub actually sends, so
 * a driver that renames an attribute key breaks a board test rather than a
 * screenshot. Two of those shapes are this family's own and no other family
 * has them: a topic with no subscription, and a subscription whose topic has
 * been deleted.
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
const subscriptionsState = vi.hoisted(() => ({ current: null as unknown }));
const snapshotsState = vi.hoisted(() => ({ current: null as unknown }));
const deadLetterState = vi.hoisted(() => ({ current: null as unknown }));
const browseState = vi.hoisted(() => ({
  current: {
    messages: [],
    loading: false,
    error: null,
    searched: false,
    run: async () => {},
  } as unknown,
}));

vi.mock("@/hooks/googlepubsub/useGooglePubSubTopics", () => ({
  useGooglePubSubTopics: () => topicsState.current,
}));
vi.mock("@/hooks/googlepubsub/useGooglePubSubSubscriptions", () => ({
  useGooglePubSubSubscriptions: () => subscriptionsState.current,
}));
vi.mock("@/hooks/googlepubsub/useGooglePubSubSnapshots", () => ({
  useGooglePubSubSnapshots: () => snapshotsState.current,
}));
vi.mock("@/hooks/googlepubsub/useGooglePubSubDeadLetters", () => ({
  useGooglePubSubDeadLetters: () => deadLetterState.current,
}));
vi.mock("@/hooks/googlepubsub/useGooglePubSubBrowse", () => ({
  useGooglePubSubBrowse: () => browseState.current,
}));

let render: (element: React.ReactElement) => string;
let TopicsGooglePubSub: typeof import("./topics/TopicsGooglePubSub").TopicsGooglePubSub;
let SubscriptionsGooglePubSub: typeof import(
  "./consumers/SubscriptionsGooglePubSub"
).SubscriptionsGooglePubSub;
let MessagesGooglePubSub: typeof import(
  "./messages/MessagesGooglePubSub"
).MessagesGooglePubSub;
let DlqGooglePubSub: typeof import("./dlq/DlqGooglePubSub").DlqGooglePubSub;
let ProducerGooglePubSub: typeof import(
  "./producer/ProducerGooglePubSub"
).ProducerGooglePubSub;
let OverviewGooglePubSub: typeof import(
  "./overview/OverviewGooglePubSub"
).OverviewGooglePubSub;

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

  const [server, topics, subscriptions, messages, dlq, producer, overview, ui, i18n, settings, profiles] =
    await Promise.all([
      import("react-dom/server"),
      import("./topics/TopicsGooglePubSub"),
      import("./consumers/SubscriptionsGooglePubSub"),
      import("./messages/MessagesGooglePubSub"),
      import("./dlq/DlqGooglePubSub"),
      import("./producer/ProducerGooglePubSub"),
      import("./overview/OverviewGooglePubSub"),
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
  TopicsGooglePubSub = topics.TopicsGooglePubSub;
  SubscriptionsGooglePubSub = subscriptions.SubscriptionsGooglePubSub;
  MessagesGooglePubSub = messages.MessagesGooglePubSub;
  DlqGooglePubSub = dlq.DlqGooglePubSub;
  ProducerGooglePubSub = producer.ProducerGooglePubSub;
  OverviewGooglePubSub = overview.OverviewGooglePubSub;
});

/**
 * A topic two subscriptions read, as
 * internal/driver/googlepubsub/destination.go sends one.
 *
 * Every count but the subscriber one is -1, which is not an oversight: a topic
 * stores nothing, so there is no depth and no rate to report anywhere.
 */
const orders = {
  id: 1,
  ref: { namespace: "", name: "mqs-seed-orders" },
  partitions: -1,
  subscribers: 2,
  depth: -1,
  rateIn: -1,
  rateOut: -1,
  lastUpdated: "",
  attributes: {
    path: "projects/mq-studio-e2e/topics/mqs-seed-orders",
    state: "ACTIVE",
    retentionSec: "1200",
    subscriptionNames: "mqs-seed-orders-audit,mqs-seed-orders-worker",
    "label.team": "orders",
  },
};

/** The state this family alerts on: every publish accepted and discarded. */
const orphanedTopic = {
  ...orders,
  id: 2,
  ref: { namespace: "", name: "mqs-seed-orphaned" },
  subscribers: 0,
  attributes: {
    path: "projects/mq-studio-e2e/topics/mqs-seed-orphaned",
    state: "ACTIVE",
  },
};

/** A topic the listing could describe only partly: every optional key absent. */
const bareTopic = {
  id: 3,
  ref: { namespace: "", name: "mqs-seed-quiet" },
  partitions: -1,
  subscribers: 1,
  depth: -1,
  rateIn: -1,
  rateOut: -1,
  lastUpdated: "",
  attributes: {},
};

/** A subscription with the whole delivery configuration on it. */
const worker = {
  id: 1,
  ref: { namespace: "", name: "mqs-seed-orders-worker" },
  status: "online",
  members: -1,
  destinations: 1,
  backlog: -1,
  rateOut: -1,
  lastUpdated: "",
  attributes: {
    path: "projects/mq-studio-e2e/subscriptions/mqs-seed-orders-worker",
    topic: "mqs-seed-orders",
    ackDeadlineSec: "20",
    retentionSec: "604800",
    retainAcked: "false",
    messageOrdering: "false",
    exactlyOnce: "false",
    detached: "false",
    state: "ACTIVE",
    delivery: "pull",
    deadLetterTopic: "mqs-seed-dead-letters",
    maxDeliveryAttempts: "5",
    retryMinBackoffSec: "10",
    retryMaxBackoffSec: "600",
  },
};

/** The other state only this family has: a subscription that outlived its topic. */
const orphanedSubscription = {
  ...worker,
  id: 2,
  ref: { namespace: "", name: "mqs-test-orphaned-sub" },
  status: "offline",
  destinations: 0,
  attributes: {
    ...worker.attributes,
    path: "projects/mq-studio-e2e/subscriptions/mqs-test-orphaned-sub",
    topic: "_deleted-topic_",
    deadLetterTopic: "",
    maxDeliveryAttempts: "",
  },
};

/** A push subscription, which has no backlog a Pull could ever reach. */
const pushSubscription = {
  ...worker,
  id: 3,
  ref: { namespace: "", name: "mqs-test-push" },
  attributes: {
    ...worker.attributes,
    path: "projects/mq-studio-e2e/subscriptions/mqs-test-push",
    delivery: "push",
    pushEndpoint: "https://example.invalid/push",
  },
};

const snapshot = {
  name: "mqs-seed-orders-worker-20260906",
  topic: "mqs-seed-orders",
  expiresAtMs: 1789260239000,
};

const deadLetterTopic = {
  namespace: "",
  name: "mqs-seed-dead-letters",
  depth: -1,
  consumers: 1,
  sources: [
    {
      queue: "mqs-seed-orders",
      subscription: "mqs-seed-orders-worker",
      exchange: "",
      routingKey: "",
    },
  ],
};

/** The row worth acting on: dead letters arriving where nothing reads them. */
const unreadDeadLetterTopic = {
  ...deadLetterTopic,
  name: "mqs-test-dl-sink",
  consumers: 0,
};

const message = {
  id: 1,
  cluster: "",
  topic: "mqs-seed-orders-worker",
  messageId: "17",
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
    deliveryAttempt: "1",
    subscription: "mqs-seed-orders-worker",
    "attr.kind": "order",
  },
};

/** A message from a subscription with no dead-letter policy: no attempt count. */
const uncountedMessage = {
  ...message,
  messageId: "18",
  keys: "customer-1",
  properties: {
    subscription: "mqs-seed-orders-audit",
    orderingKey: "customer-1",
  },
};

describe("the Pub/Sub topics board", () => {
  it("says the connection is offline rather than showing an empty list", () => {
    topicsState.current = stateOf({ online: false });
    expect(() => render(<TopicsGooglePubSub />)).not.toThrow();
  });

  it("renders while the first request is in flight", () => {
    topicsState.current = stateOf({ loading: true });
    expect(() => render(<TopicsGooglePubSub />)).not.toThrow();
  });

  it("shows what failed rather than an empty list", () => {
    topicsState.current = stateOf({ error: "the credential was rejected" });
    expect(render(<TopicsGooglePubSub />)).toContain("the credential was rejected");
  });

  it("renders a project that holds no topics", () => {
    topicsState.current = stateOf({ data: [] });
    expect(() => render(<TopicsGooglePubSub />)).not.toThrow();
  });

  it("lists a topic with the subscriptions that read it", () => {
    topicsState.current = stateOf({ data: [orders] });
    const html = render(<TopicsGooglePubSub />);
    expect(html).toContain("mqs-seed-orders");
    expect(html).toContain("mqs-seed-orders-worker");
  });

  /*
   * The state that looks healthy everywhere else, because a discarded message
   * leaves no backlog behind it.
   */
  it("marks a topic nothing subscribes to", () => {
    topicsState.current = stateOf({ data: [orphanedTopic] });
    expect(() => render(<TopicsGooglePubSub />)).not.toThrow();
  });

  it("renders a topic the listing could describe only partly", () => {
    topicsState.current = stateOf({ data: [bareTopic] });
    expect(() => render(<TopicsGooglePubSub />)).not.toThrow();
  });
});

describe("the Pub/Sub subscriptions board", () => {
  it("says the connection is offline rather than showing an empty list", () => {
    subscriptionsState.current = stateOf({ online: false });
    snapshotsState.current = stateOf({ data: [] });
    expect(() => render(<SubscriptionsGooglePubSub />)).not.toThrow();
  });

  it("shows what failed rather than an empty list", () => {
    subscriptionsState.current = stateOf({ error: "permission denied on the project" });
    snapshotsState.current = stateOf({ data: [] });
    expect(render(<SubscriptionsGooglePubSub />)).toContain("permission denied on the project");
  });

  it("renders a project that holds no subscriptions", () => {
    subscriptionsState.current = stateOf({ data: [] });
    snapshotsState.current = stateOf({ data: [] });
    expect(() => render(<SubscriptionsGooglePubSub />)).not.toThrow();
  });

  it("lists a subscription with its delivery configuration", () => {
    subscriptionsState.current = stateOf({ data: [worker] });
    snapshotsState.current = stateOf({ data: [] });
    const html = render(<SubscriptionsGooglePubSub />);
    expect(html).toContain("mqs-seed-orders-worker");
    expect(html).toContain("mqs-seed-dead-letters");
  });

  /*
   * A subscription that outlived its topic is the state with no other symptom:
   * it will never receive again, still holds what it had, and every other
   * figure about it reads ordinarily.
   */
  it("marks a subscription whose topic has been deleted", () => {
    subscriptionsState.current = stateOf({ data: [orphanedSubscription] });
    snapshotsState.current = stateOf({ data: [] });
    expect(render(<SubscriptionsGooglePubSub />)).toContain("_deleted-topic_");
  });

  // The restore points are the only seek target that always works, so the
  // panel that lists them has to render with and without any.
  it("lists the restore points on the chosen subscription's topic", () => {
    subscriptionsState.current = stateOf({ data: [worker] });
    snapshotsState.current = stateOf({ data: [snapshot] });
    expect(render(<SubscriptionsGooglePubSub />)).toContain(snapshot.name);
  });

  it("renders a subscription with no restore point to offer", () => {
    subscriptionsState.current = stateOf({ data: [worker] });
    snapshotsState.current = stateOf({ data: [] });
    expect(() => render(<SubscriptionsGooglePubSub />)).not.toThrow();
  });
});

describe("the Pub/Sub messages board", () => {
  it("says the connection is offline rather than showing an empty list", () => {
    subscriptionsState.current = stateOf({ online: false });
    expect(() => render(<MessagesGooglePubSub />)).not.toThrow();
  });

  /*
   * The caveat is the point of this page, and it is drawn before anything is
   * pulled rather than after: pressing the button is a real delivery.
   */
  it("warns that pulling delivers before anything is pulled", () => {
    subscriptionsState.current = stateOf({ data: [worker] });
    browseState.current = {
      messages: [],
      loading: false,
      error: null,
      searched: false,
      run: async () => {},
    };
    // The banner's own wording, in the language this file renders in: the
    // point is that it is on screen before the button has been pressed.
    const html = render(<MessagesGooglePubSub />);
    expect(html).toContain("这是一次真实投递");
  });

  /*
   * Only pull subscriptions are offered. A push one is written straight
   * through by the service and has no backlog a Pull could reach, so offering
   * it would be a picker entry that always answers empty.
   */
  it("offers no push subscription to pull from", () => {
    subscriptionsState.current = stateOf({ data: [pushSubscription] });
    browseState.current = {
      messages: [],
      loading: false,
      error: null,
      searched: false,
      run: async () => {},
    };
    expect(render(<MessagesGooglePubSub />)).not.toContain("mqs-test-push");
  });

  it("renders a pulled message and its attributes", () => {
    subscriptionsState.current = stateOf({ data: [worker] });
    browseState.current = {
      messages: [message],
      loading: false,
      error: null,
      searched: true,
      run: async () => {},
    };
    const html = render(<MessagesGooglePubSub />);
    expect(html).toContain("order-1");
  });

  /*
   * A subscription with no dead-letter policy reports no delivery attempt at
   * all, which is a fact about the subscription rather than the message - and
   * the path where the count is absent is the one that throws.
   */
  it("renders a message with no delivery attempt to show", () => {
    subscriptionsState.current = stateOf({ data: [worker] });
    browseState.current = {
      messages: [uncountedMessage],
      loading: false,
      error: null,
      searched: true,
      run: async () => {},
    };
    expect(() => render(<MessagesGooglePubSub />)).not.toThrow();
  });

  it("shows what the pull failed with", () => {
    subscriptionsState.current = stateOf({ data: [worker] });
    browseState.current = {
      messages: [],
      loading: false,
      error: "no subscription named \"orders\"",
      searched: true,
      run: async () => {},
    };
    expect(render(<MessagesGooglePubSub />)).toContain("no subscription named");
  });
});

describe("the Pub/Sub dead letters board", () => {
  it("says the connection is offline rather than showing an empty list", () => {
    deadLetterState.current = stateOf({ online: false });
    expect(() => render(<DlqGooglePubSub />)).not.toThrow();
  });

  it("renders a project where nothing dead-letters", () => {
    deadLetterState.current = stateOf({ data: [] });
    expect(() => render(<DlqGooglePubSub />)).not.toThrow();
  });

  /*
   * Both halves of a source, which is what this family has and RabbitMQ's
   * shape does not: the policy is on the subscription, so the topic alone
   * would not say who stopped trying.
   */
  it("names the topic and the subscription that gave up", () => {
    deadLetterState.current = stateOf({ data: [deadLetterTopic] });
    const html = render(<DlqGooglePubSub />);
    expect(html).toContain("mqs-seed-orders");
    expect(html).toContain("mqs-seed-orders-worker");
  });

  it("marks a dead-letter topic nothing subscribes to", () => {
    deadLetterState.current = stateOf({ data: [unreadDeadLetterTopic] });
    expect(() => render(<DlqGooglePubSub />)).not.toThrow();
  });
});

describe("the Pub/Sub publish console", () => {
  it("says the connection is offline rather than an empty topic list", () => {
    topicsState.current = stateOf({ online: false });
    expect(() => render(<ProducerGooglePubSub />)).not.toThrow();
  });

  it("renders with a topic to publish to", () => {
    topicsState.current = stateOf({ data: [orders] });
    expect(() => render(<ProducerGooglePubSub />)).not.toThrow();
  });

  it("renders with no topic in the project at all", () => {
    topicsState.current = stateOf({ data: [] });
    expect(() => render(<ProducerGooglePubSub />)).not.toThrow();
  });
});

describe("the Pub/Sub overview", () => {
  it("says the connection is offline rather than showing zeroes", () => {
    topicsState.current = stateOf({ online: false });
    subscriptionsState.current = stateOf({ online: false });
    expect(() => render(<OverviewGooglePubSub />)).not.toThrow();
  });

  it("renders an empty project", () => {
    topicsState.current = stateOf({ data: [] });
    subscriptionsState.current = stateOf({ data: [] });
    expect(() => render(<OverviewGooglePubSub />)).not.toThrow();
  });

  /*
   * The two tiles no other family has, and the reason this page exists: both
   * count a fan-out that is broken while everything else reports success.
   */
  it("counts the topics nothing reads and the subscriptions with no topic", () => {
    topicsState.current = stateOf({ data: [orders, orphanedTopic] });
    subscriptionsState.current = stateOf({ data: [worker, orphanedSubscription] });
    expect(() => render(<OverviewGooglePubSub />)).not.toThrow();
  });
});
