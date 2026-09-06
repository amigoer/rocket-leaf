import { describe, expect, it } from "vitest";
import en from "./locales/en.json";
import zh from "./locales/zh.json";

/**
 * Every reason a driver can report has to resolve to a sentence.
 *
 * keys.test.ts cannot see these. It scans the sources for literal `t("…")`
 * calls, and a degraded reason never appears in one: the driver sends the key
 * across the bridge and the sidebar resolves whatever arrives. So a missing
 * one is invisible to the whole test suite and shows up only as a tooltip
 * reading "mq.mqtt.degraded.managementAbsent" at somebody who wanted to know
 * why a page was blocked — which is what happened.
 *
 * The lists below are the constants each driver declares. They are duplicated
 * here on purpose, the same way the sidebar capability contract is: nothing in
 * either language ties a Go string to a JSON key, so the only thing that can
 * catch a rename is a second copy that goes red.
 */
const REASONS: Record<string, string[]> = {
  // internal/driver/mqtt/conn.go and management.go
  mqtt: [
    "sysRefused",
    "sysSilent",
    "managementAbsent",
    "managementUnreachable",
    "managementCredentials",
    "managementUnknown",
  ],
  // internal/driver/kafka/conn.go
  kafka: ["credentials", "forbidden", "timeout", "accessControl", "unreachable"],
  // internal/driver/nats/conn.go
  //
  // Six rather than three, because each pair is one tier that can be missing
  // two ways, and the two ways have different fixes. A server built without
  // JetStream is not an account denied it; an endpoint nobody named is not one
  // that did not answer; credentials never given are not credentials refused.
  nats: [
    "jetstreamDisabled",
    "jetstreamNoAccount",
    "monitorAbsent",
    "monitorUnreachable",
    "systemAbsent",
    "systemForbidden",
  ],
  // internal/driver/activemq/conn.go
  //
  // Three for one optional tier, because AMQP can be missing three ways that
  // lead three different places: the connection form, the broker's acceptor
  // list, and the broker's authentication realm.
  activemq: ["amqpAbsent", "amqpUnreachable", "amqpForbidden"],
  // internal/driver/nsq/cluster.go
  //
  // One, and only one way to be missing: nsqlookupd is a separate daemon a
  // profile either names or does not. There is no "it did not answer" here -
  // an address that answers nothing fails the connection at open rather than
  // leaving a tier degraded.
  nsq: ["lookupdAbsent"],
  // internal/driver/googlepubsub/conn.go
  //
  // One, and it is not about the endpoint at all: a subscription's backlog is
  // a Cloud Monitoring metric, so no Pub/Sub connection anywhere can report it.
  // Degraded rather than absent because the family does have the concept - and
  // a page that said nothing would leave a reader looking for a column that is
  // never coming.
  "google-pubsub": ["lagInMonitoring"],
  // internal/driver/azureservicebus/conn.go - the emulator serves no usable
  // CountDetails, so a backlog is unavailable against it and a real figure
  // everywhere else. Degraded only on that endpoint, which is what declare()
  // narrowing is for.
  "azure-servicebus": ["countsNotInEmulator"],
  // internal/driver/kinesis/conn.go
  //
  // One, and unconditional like Pub/Sub's rather than endpoint-specific: a
  // stream records that a consumer is registered and nothing about where it
  // has read to. Unlike Pub/Sub's, the number does not exist in a second API
  // either - a classic consumer keeps its position in a DynamoDB table the
  // KCL owns, and an enhanced fan-out consumer keeps none at all.
  kinesis: ["positionInDynamo"],
  // internal/driver/ibmmq/conn.go
  //
  // Two, and both about one tier: the messaging REST API is authorised
  // separately from the administrative one, and it can be unavailable two ways
  // that lead two different places - the mqweb server's role mapping, and the
  // connection form's second credential.
  ibmmq: ["messagingForbidden", "messagingRefused"],
};

/**
 * Caveats have the same problem and needed the same list.
 *
 * A caveat is not a degraded reason: the capability works, and doing it has a
 * consequence worth saying out loud. But it crosses the bridge as a key
 * exactly the way a reason does, so it is invisible to keys.test.ts for
 * exactly the same reason - and mq.rabbitmq.caveat.browseAltersQueue sat
 * unguarded here until ActiveMQ needed a second one.
 */
const CAVEATS: Record<string, string[]> = {
  // internal/driver/rabbitmq - browsing goes through basic.get, which alters
  // the queue even when what it read is put back.
  rabbitmq: ["browseAltersQueue"],
  // internal/driver/activemq/conn.go - Classic stops at maxBrowsePageSize.
  activemq: ["browseCapped"],
  // internal/driver/sqs/conn.go - the only read SQS has is the one a consumer
  // makes, so a browse hides what it read and raises its receive count.
  sqs: ["receiveHides"],
  // internal/driver/googlepubsub/conn.go - Pull is the only read Pub/Sub has,
  // so a browse holds what it read away from consumers and raises its delivery
  // attempt, which counts towards being dead-lettered.
  "google-pubsub": ["pullDelivers"],
  // internal/driver/kinesis/conn.go - and this one says the opposite of the
  // three above it. A Kinesis read takes nothing; what it spends is the
  // shard's read allowance, which every consumer on that shard shares.
  kinesis: ["readQuota"],
  // internal/driver/ibmmq/conn.go - two, and neither is about consuming: an MQ
  // browse leaves the depth alone. What the mqweb server will not do is return
  // a body it cannot read as text, and on the way in it refuses one outright
  // and has no topic endpoint to send to at all.
  ibmmq: ["browseCharacterOnly", "sendQueueOnly"],
};

type Bundle = Record<string, unknown>;

function resolve(bundle: Bundle, path: string): unknown {
  return path
    .split(".")
    .reduce<unknown>(
      (node, part) =>
        node != null && typeof node === "object"
          ? (node as Record<string, unknown>)[part]
          : undefined,
      bundle,
    );
}

/**
 * How short is too short, per language.
 *
 * Chinese carries about three times as much per character, so one threshold
 * flags perfectly good Chinese: "集群接受了连接，但没有在超时时间内应答。" is a
 * complete sentence in twenty characters and its English is sixty-three.
 */
const FLOOR: Record<string, number> = { en: 30, zh: 12 };

describe.each([
  ["en", en as Bundle],
  ["zh", zh as Bundle],
])("the %s bundle", (language, bundle) => {
  it("has a sentence for every degraded reason a driver reports", () => {
    const missing: string[] = [];
    for (const [kind, reasons] of Object.entries(REASONS)) {
      for (const reason of reasons) {
        const key = `mq.${kind}.degraded.${reason}`;
        if (typeof resolve(bundle, key) !== "string") missing.push(key);
      }
    }
    expect(missing).toEqual([]);
  });

  it("has a sentence for every caveat a driver attaches", () => {
    const missing: string[] = [];
    for (const [kind, caveats] of Object.entries(CAVEATS)) {
      for (const caveat of caveats) {
        const key = `mq.${kind}.caveat.${caveat}`;
        if (typeof resolve(bundle, key) !== "string") missing.push(key);
      }
    }
    expect(missing).toEqual([]);
  });

  // A reason is read by somebody who has just been stopped from opening a
  // page. "Not supported" tells them nothing they did not already know.
  it("says what to do rather than only what is wrong", () => {
    const tooShort: string[] = [];
    for (const [kind, reasons] of Object.entries(REASONS)) {
      for (const reason of reasons) {
        const key = `mq.${kind}.degraded.${reason}`;
        const text = resolve(bundle, key);
        if (typeof text === "string" && text.length < (FLOOR[language] ?? 30)) {
          tooShort.push(key);
        }
      }
    }
    expect(tooShort).toEqual([]);
  });
});
