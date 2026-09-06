/**
 * The connection form's own rules, protocol by protocol.
 *
 * The dispatch used to end in a bare fallthrough holding RocketMQ's rules, so
 * a protocol added to ProtocolDraft without a branch of its own inherited them
 * - a name server it has no field for, and a namespace pattern that is one
 * broker's. Nothing on screen said so; the save button simply stayed off.
 *
 * The table below is keyed by the union itself, so a protocol added without an
 * entry stops compiling here the way it now does in draftInvalidReason.
 */
import { describe, expect, it } from "vitest";
import {
  emptyActiveMQDraft,
  emptyKafkaDraft,
  emptyMqttDraft,
  emptyNatsDraft,
  emptyNsqDraft,
  emptyPulsarDraft,
  emptyRabbitMQDraft,
  emptyRedisDraft,
  emptyRocketMQDraft,
} from "./ConnectionForms";
import { emptyDraft, type ProtocolDraft } from "./connectionDraft";
import { countAddresses, draftInvalidReason } from "./connectionValidation";

/** i18n is not initialised here, so a rule's message comes back as its key. */
const key = (k: string) => k;

const NAME = "orders";

interface Rules {
  /** Everything this protocol's own rules ask for. */
  saveable: ProtocolDraft;
  /** Named, and nothing else: whatever it asks for first is missing. */
  bare: ProtocolDraft;
  /** The field that first refusal names. */
  firstAsk: string;
}

const RULES: Record<ProtocolDraft["protocol"], Rules> = {
  rocketmq: {
    saveable: {
      protocol: "rocketmq",
      value: { ...emptyRocketMQDraft(), name: NAME, endpoints: "127.0.0.1:9876" },
    },
    bare: { protocol: "rocketmq", value: { ...emptyRocketMQDraft(), name: NAME } },
    firstAsk: "page.connections.endpointsRequired",
  },
  rabbitmq: {
    saveable: {
      protocol: "rabbitmq",
      value: {
        ...emptyRabbitMQDraft(),
        name: NAME,
        management: "http://127.0.0.1:15672",
        username: "guest",
      },
    },
    bare: { protocol: "rabbitmq", value: { ...emptyRabbitMQDraft(), name: NAME } },
    firstAsk: "page.connections.managementRequired",
  },
  kafka: {
    saveable: {
      protocol: "kafka",
      value: { ...emptyKafkaDraft(), name: NAME, endpoints: "127.0.0.1:9092" },
    },
    bare: { protocol: "kafka", value: { ...emptyKafkaDraft(), name: NAME } },
    firstAsk: "page.connections.bootstrapRequired",
  },
  pulsar: {
    saveable: {
      protocol: "pulsar",
      value: {
        ...emptyPulsarDraft(),
        name: NAME,
        service: "pulsar://127.0.0.1:6650",
        admin: "http://127.0.0.1:8080",
      },
    },
    bare: { protocol: "pulsar", value: { ...emptyPulsarDraft(), name: NAME } },
    firstAsk: "page.connections.serviceUrlRequired",
  },
  redis: {
    saveable: {
      protocol: "redis",
      value: { ...emptyRedisDraft(), name: NAME, endpoints: "127.0.0.1:6379" },
    },
    bare: { protocol: "redis", value: { ...emptyRedisDraft(), name: NAME } },
    firstAsk: "page.connections.endpointsRequired",
  },
  mqtt: {
    saveable: {
      protocol: "mqtt",
      value: { ...emptyMqttDraft(), name: NAME, endpoints: "tcp://127.0.0.1:1883" },
    },
    bare: { protocol: "mqtt", value: { ...emptyMqttDraft(), name: NAME } },
    firstAsk: "page.connections.brokerRequired",
  },
  nats: {
    saveable: {
      protocol: "nats",
      value: { ...emptyNatsDraft(), name: NAME, endpoints: "nats://127.0.0.1:4222" },
    },
    bare: { protocol: "nats", value: { ...emptyNatsDraft(), name: NAME } },
    firstAsk: "page.connections.form.nats.serversRequired",
  },
  activemq: {
    saveable: {
      protocol: "activemq",
      value: { ...emptyActiveMQDraft(), name: NAME, endpoints: "http://127.0.0.1:8161" },
    },
    bare: { protocol: "activemq", value: { ...emptyActiveMQDraft(), name: NAME } },
    firstAsk: "page.connections.endpointsRequired",
  },
  nsq: {
    saveable: {
      protocol: "nsq",
      value: { ...emptyNsqDraft(), name: NAME, endpoints: "http://127.0.0.1:4151" },
    },
    bare: { protocol: "nsq", value: { ...emptyNsqDraft(), name: NAME } },
    firstAsk: "page.connections.form.nsq.nsqdRequired",
  },
};

const PROTOCOLS = Object.keys(RULES) as ProtocolDraft["protocol"][];

describe("the connection draft's validity", () => {
  it("accepts a filled-in draft for every protocol", () => {
    for (const protocol of PROTOCOLS) {
      expect(draftInvalidReason(RULES[protocol].saveable, key), protocol).toBeNull();
    }
  });

  it("asks each protocol for the address its own form draws", () => {
    for (const protocol of PROTOCOLS) {
      const { bare, firstAsk } = RULES[protocol];
      expect(draftInvalidReason(bare, key), protocol).toBe(firstAsk);
    }
  });

  it("refuses a nameless draft whatever the protocol", () => {
    for (const protocol of PROTOCOLS) {
      expect(draftInvalidReason(emptyDraft(protocol), key), protocol).toBe(
        "page.connections.nameRequired",
      );
    }
  });

  /*
   * Pulsar is the protocol the old fallthrough would have damaged silently:
   * it is the only other one whose draft carries a namespace, so RocketMQ's
   * pattern applied to it without so much as a type error. A dot is legal in
   * a Pulsar namespace and illegal in a RocketMQ one.
   */
  it("does not hold Pulsar to RocketMQ's namespace rule", () => {
    const dotted: ProtocolDraft = {
      protocol: "pulsar",
      value: {
        ...emptyPulsarDraft(),
        name: NAME,
        service: "pulsar://127.0.0.1:6650",
        admin: "http://127.0.0.1:8080",
        namespace: "team.orders",
      },
    };
    expect(draftInvalidReason(dotted, key)).toBeNull();
  });

  it("keeps RocketMQ's own namespace rule", () => {
    const dotted: ProtocolDraft = {
      protocol: "rocketmq",
      value: {
        ...emptyRocketMQDraft(),
        name: NAME,
        endpoints: "127.0.0.1:9876",
        namespace: "team.orders",
      },
    };
    expect(draftInvalidReason(dotted, key)).toBe("page.connections.form.rocketmq.namespaceInvalid");
  });

  // go-redis builds a cluster client the moment it is handed a second address,
  // so the form has to count them the way internal/driver/redisstream does.
  it("counts addresses the way the Redis driver does", () => {
    expect(countAddresses(" 10.0.0.1:6379 , 10.0.0.2:6379 ")).toBe(2);
    expect(countAddresses("  ")).toBe(0);

    const clustered: ProtocolDraft = {
      protocol: "redis",
      value: { ...emptyRedisDraft(), name: NAME, endpoints: "10.0.0.1:6379,10.0.0.2:6379" },
    };
    expect(draftInvalidReason(clustered, key)).toBe("page.connections.form.redis.oneAddressOnly");
  });
});
