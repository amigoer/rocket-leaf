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
  emptySqsDraft,
  emptyGooglePubSubDraft,
  emptyAzureServiceBusDraft,
  emptyKinesisDraft,
  emptyIbmMqDraft,
  emptySolaceDraft,
} from "./ConnectionForms";
import { emptyDraft, toSubmission, type ProtocolDraft } from "./connectionDraft";
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
  /*
   * The saveable draft here carries no address, and there is no field it could
   * have gone in. That is the whole of what this family adds: a region is what
   * says where the queues are, and the credential pair is optional because a
   * blank one means the machine's own AWS identity.
   */
  sqs: {
    saveable: {
      protocol: "sqs",
      value: { ...emptySqsDraft(), name: NAME, region: "eu-west-1" },
    },
    bare: { protocol: "sqs", value: { ...emptySqsDraft(), name: NAME } },
    firstAsk: "page.connections.form.sqs.regionRequired",
  },
  /*
   * The second draft with no address, and no field it could have gone in
   * either. A project is what says where the topics are, and the credential is
   * optional because a blank one means the machine's own Google identity.
   */
  "google-pubsub": {
    saveable: {
      protocol: "google-pubsub",
      value: { ...emptyGooglePubSubDraft(), name: NAME, projectId: "orders-prod" },
    },
    bare: { protocol: "google-pubsub", value: { ...emptyGooglePubSubDraft(), name: NAME } },
    firstAsk: "page.connections.form.google-pubsub.projectRequired",
  },
  /*
   * The one hosted draft with an address, and the only family here whose
   * credential is not optional: there is no ambient Azure identity the way
   * there is an AWS credential chain and Application Default Credentials.
   */
  "azure-servicebus": {
    saveable: {
      protocol: "azure-servicebus",
      value: {
        ...emptyAzureServiceBusDraft(),
        name: NAME,
        endpoints: "orders.servicebus.windows.net",
        sharedAccessKey: "c2VjcmV0",
      },
    },
    bare: { protocol: "azure-servicebus", value: { ...emptyAzureServiceBusDraft(), name: NAME } },
    firstAsk: "page.connections.form.azure-servicebus.namespaceRequired",
  },
  /*
   * The fourth hosted draft and the second whose address is a region. Its
   * first refusal is the region rather than a credential, because a blank
   * credential is a real choice here - the machine's own AWS identity.
   */
  kinesis: {
    saveable: {
      protocol: "kinesis",
      value: { ...emptyKinesisDraft(), name: NAME, region: "eu-west-1" },
    },
    bare: { protocol: "kinesis", value: { ...emptyKinesisDraft(), name: NAME } },
    firstAsk: "page.connections.form.kinesis.regionRequired",
  },
  /*
   * The first draft since ActiveMQ's with an address and two credential pairs.
   * Its first refusal is the address, because that is the only field with
   * nothing to fall back on: the queue manager is discovered when the server
   * fronts one, and the messaging pair is reused from the administrative one.
   */
  ibmmq: {
    saveable: {
      protocol: "ibmmq",
      value: { ...emptyIbmMqDraft(), name: NAME, endpoints: "https://mq.example:9443" },
    },
    bare: { protocol: "ibmmq", value: { ...emptyIbmMqDraft(), name: NAME } },
    firstAsk: "page.connections.form.ibmmq.mqwebRequired",
  },
  /*
   * Solace's first refusal is the address for the same reason IBM MQ's is: it
   * is the only field with nothing to fall back on. The Message VPN falls back
   * to "default" and the REST credential is optional outright, so neither can
   * be what a bare draft is asked for first.
   */
  solace: {
    saveable: {
      protocol: "solace",
      value: { ...emptySolaceDraft(), name: NAME, endpoints: "http://solace.example:8080" },
    },
    bare: { protocol: "solace", value: { ...emptySolaceDraft(), name: NAME } },
    firstAsk: "page.connections.form.solace.sempRequired",
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

  /*
   * The frontend half of the addressless connection.
   *
   * Its Go half is internal/driver/sqs's TestDescriptorAsksForNoAddress, which
   * asserts the driver's form declares no endpoint field. This one asserts the
   * dialog agrees: an SQS draft with a name and a region and nothing else is
   * saveable, and what it submits carries an empty address rather than an
   * invented one.
   */
  it("saves an SQS draft that names no address at all", () => {
    const draft: ProtocolDraft = {
      protocol: "sqs",
      value: { ...emptySqsDraft(), name: NAME, region: "eu-west-1" },
    };
    expect(draftInvalidReason(draft, key)).toBeNull();
    expect(toSubmission(draft).draft.endpoints).toBe("");
    expect(toSubmission(draft).draft.options?.region).toBe("eu-west-1");
  });

  // Blank is the machine's own AWS identity and is a real choice. Half a pair
  // is not: falling back to that identity would connect as whoever the machine
  // is rather than as the account being typed.
  it("refuses half an AWS credential and accepts none at all", () => {
    const base = { ...emptySqsDraft(), name: NAME, region: "eu-west-1" };
    expect(draftInvalidReason({ protocol: "sqs", value: base }, key)).toBeNull();
    expect(
      draftInvalidReason({ protocol: "sqs", value: { ...base, accessKeyId: "AKIA" } }, key),
    ).toBe("page.connections.form.sqs.credentialPairRequired");
    expect(
      draftInvalidReason({ protocol: "sqs", value: { ...base, secretAccessKey: "s3cret" } }, key),
    ).toBe("page.connections.form.sqs.credentialPairRequired");
    expect(
      draftInvalidReason(
        { protocol: "sqs", value: { ...base, accessKeyId: "AKIA", secretAccessKey: "s3cret" } },
        key,
      ),
    ).toBeNull();
  });

  // The SDK takes the override verbatim, so a bare hostname would be signed as
  // a relative path and fail somewhere that names neither the field nor why.
  it("holds the SQS endpoint override to a full URL", () => {
    const base = { ...emptySqsDraft(), name: NAME, region: "eu-west-1" };
    expect(
      draftInvalidReason({ protocol: "sqs", value: { ...base, endpointUrl: "vpce-0abc" } }, key),
    ).toBe("page.connections.form.sqs.endpointScheme");
    expect(
      draftInvalidReason(
        { protocol: "sqs", value: { ...base, endpointUrl: "https://vpce-0abc.example" } },
        key,
      ),
    ).toBeNull();
  });

  /*
   * The same three assertions for the second AWS family. They are repeated
   * rather than shared because nothing but this test keeps the two drafts in
   * step: each has its own submission function, its own validation branch and
   * its own copy of the secret names.
   */
  it("saves a Kinesis draft that names no address at all", () => {
    const draft: ProtocolDraft = {
      protocol: "kinesis",
      value: { ...emptyKinesisDraft(), name: NAME, region: "eu-west-1" },
    };
    expect(draftInvalidReason(draft, key)).toBeNull();
    expect(toSubmission(draft).draft.endpoints).toBe("");
    expect(toSubmission(draft).draft.options?.region).toBe("eu-west-1");
  });

  /*
   * IBM MQ's three, and they are its own rather than a neighbour's: the
   * address is a URL because every REST path is built on it, the queue
   * manager is an MQ object name, and half a messaging credential would
   * silently send the administrative password with the username that was
   * typed.
   */
  it("refuses an mqweb address with no scheme", () => {
    const base = { ...emptyIbmMqDraft(), name: NAME, endpoints: "https://mq.example:9443" };
    expect(draftInvalidReason({ protocol: "ibmmq", value: base }, key)).toBeNull();
    expect(
      draftInvalidReason({ protocol: "ibmmq", value: { ...base, endpoints: "mq.example:9443" } }, key),
    ).toBe("page.connections.form.ibmmq.mqwebScheme");
  });

  /*
   * Solace's three, and the last is the one worth having. The address is a URL
   * because every SEMP path is built on it; the Message VPN rule is the
   * broker's own, quoted from the message it refuses a bad name with; and half
   * a REST credential fails in the direction nobody expects - a password with
   * no username is discarded rather than refused.
   */
  it("refuses a semp address with no scheme", () => {
    const base = { ...emptySolaceDraft(), name: NAME, endpoints: "http://solace.example:8080" };
    expect(draftInvalidReason({ protocol: "solace", value: base }, key)).toBeNull();
    expect(
      draftInvalidReason(
        { protocol: "solace", value: { ...base, endpoints: "solace.example:8080" } },
        key,
      ),
    ).toBe("page.connections.form.solace.sempScheme");
  });

  it("refuses a message vpn name the broker would not take", () => {
    const base = { ...emptySolaceDraft(), name: NAME, endpoints: "http://solace.example:8080" };
    expect(
      draftInvalidReason({ protocol: "solace", value: { ...base, msgVpn: "orders/eu" } }, key),
    ).toBeNull();
    expect(
      draftInvalidReason({ protocol: "solace", value: { ...base, msgVpn: "orders*" } }, key),
    ).toBe("page.connections.form.solace.msgVpnName");
    expect(
      draftInvalidReason({ protocol: "solace", value: { ...base, msgVpn: "v".repeat(33) } }, key),
    ).toBe("page.connections.form.solace.msgVpnName");
  });

  it("refuses half a solace rest credential in either direction", () => {
    const base = { ...emptySolaceDraft(), name: NAME, endpoints: "http://solace.example:8080" };
    expect(
      draftInvalidReason({ protocol: "solace", value: { ...base, restUsername: "app" } }, key),
    ).toBe("page.connections.form.solace.restPairRequired");
    expect(
      draftInvalidReason({ protocol: "solace", value: { ...base, restPassword: "secret" } }, key),
    ).toBe("page.connections.form.solace.restPairRequired");
    expect(
      draftInvalidReason(
        { protocol: "solace", value: { ...base, restUsername: "app", restPassword: "secret" } },
        key,
      ),
    ).toBeNull();
  });

  it("refuses a queue manager name IBM MQ could not have", () => {
    const base = { ...emptyIbmMqDraft(), name: NAME, endpoints: "https://mq.example:9443" };
    expect(
      draftInvalidReason({ protocol: "ibmmq", value: { ...base, queueManager: "QM1" } }, key),
    ).toBeNull();
    expect(
      draftInvalidReason({ protocol: "ibmmq", value: { ...base, queueManager: "QM 1" } }, key),
    ).toBe("page.connections.form.ibmmq.queueManagerName");
    expect(
      draftInvalidReason({ protocol: "ibmmq", value: { ...base, queueManager: "Q".repeat(49) } }, key),
    ).toBe("page.connections.form.ibmmq.queueManagerName");
  });

  it("refuses half a messaging credential, which would authenticate as somebody else", () => {
    const base = { ...emptyIbmMqDraft(), name: NAME, endpoints: "https://mq.example:9443" };
    expect(draftInvalidReason({ protocol: "ibmmq", value: base }, key)).toBeNull();
    expect(
      draftInvalidReason({ protocol: "ibmmq", value: { ...base, messagingUsername: "app" } }, key),
    ).toBe("page.connections.form.ibmmq.messagingPairRequired");
    expect(
      draftInvalidReason(
        { protocol: "ibmmq", value: { ...base, messagingUsername: "app", messagingPassword: "pw" } },
        key,
      ),
    ).toBeNull();
  });

  it("refuses half an AWS credential on the Kinesis form too", () => {
    const base = { ...emptyKinesisDraft(), name: NAME, region: "eu-west-1" };
    expect(draftInvalidReason({ protocol: "kinesis", value: base }, key)).toBeNull();
    expect(
      draftInvalidReason({ protocol: "kinesis", value: { ...base, accessKeyId: "AKIA" } }, key),
    ).toBe("page.connections.form.kinesis.credentialPairRequired");
    expect(
      draftInvalidReason({ protocol: "kinesis", value: { ...base, secretAccessKey: "s3cret" } }, key),
    ).toBe("page.connections.form.kinesis.credentialPairRequired");
  });

  it("holds the Kinesis endpoint override to a full URL", () => {
    const base = { ...emptyKinesisDraft(), name: NAME, region: "eu-west-1" };
    expect(
      draftInvalidReason({ protocol: "kinesis", value: { ...base, endpointUrl: "vpce-0abc" } }, key),
    ).toBe("page.connections.form.kinesis.endpointScheme");
    expect(
      draftInvalidReason(
        { protocol: "kinesis", value: { ...base, endpointUrl: "https://vpce-0abc.example" } },
        key,
      ),
    ).toBeNull();
  });

  /*
   * The same pair of assertions for the second addressless family, and it is
   * not a copy: its Go half is internal/driver/googlepubsub's
   * TestDescriptorAsksForNoAddress, and a draft carrying only a name and a
   * project has to be saveable with an empty address rather than an invented
   * one.
   */
  it("saves a Pub/Sub draft that names no address at all", () => {
    const draft: ProtocolDraft = {
      protocol: "google-pubsub",
      value: { ...emptyGooglePubSubDraft(), name: NAME, projectId: "orders-prod" },
    };
    expect(draftInvalidReason(draft, key)).toBeNull();
    expect(toSubmission(draft).draft.endpoints).toBe("");
    expect(toSubmission(draft).draft.options?.projectId).toBe("orders-prod");
  });

  /*
   * Blank is Application Default Credentials and is a real choice. A path is
   * not: the client library reports one as having no credentials at all, which
   * reads as "this machine has no Google identity" - the opposite of what
   * happened.
   */
  it("refuses a Pub/Sub credential that is a path rather than the key", () => {
    const base = { ...emptyGooglePubSubDraft(), name: NAME, projectId: "orders-prod" };
    expect(draftInvalidReason({ protocol: "google-pubsub", value: base }, key)).toBeNull();
    expect(
      draftInvalidReason(
        { protocol: "google-pubsub", value: { ...base, credentialsJson: "~/key.json" } },
        key,
      ),
    ).toBe("page.connections.form.google-pubsub.credentialsNotJson");
    expect(
      draftInvalidReason(
        {
          protocol: "google-pubsub",
          value: { ...base, credentialsJson: '{"type":"service_account"}' },
        },
        key,
      ),
    ).toBeNull();
  });

  // gRPC dials a host and a port. A scheme in front becomes part of the
  // hostname, and the failure names neither the field nor why.
  it("holds the Pub/Sub emulator host to a host and a port", () => {
    const base = { ...emptyGooglePubSubDraft(), name: NAME, projectId: "orders-prod" };
    expect(
      draftInvalidReason(
        { protocol: "google-pubsub", value: { ...base, emulatorHost: "http://127.0.0.1:8085" } },
        key,
      ),
    ).toBe("page.connections.form.google-pubsub.emulatorHostNoScheme");
    expect(
      draftInvalidReason(
        { protocol: "google-pubsub", value: { ...base, emulatorHost: "127.0.0.1:8085" } },
        key,
      ),
    ).toBeNull();
  });

  /*
   * The hosted family that does carry an address, which is the whole of what
   * separates its form from the two above it. Its Go half is
   * internal/driver/azureservicebus's TestDescriptorAsksForAnAddress, and a
   * namespace stashed in an option would have passed every other test here.
   */
  it("keeps the Service Bus namespace in the address field", () => {
    const draft: ProtocolDraft = {
      protocol: "azure-servicebus",
      value: {
        ...emptyAzureServiceBusDraft(),
        name: NAME,
        endpoints: "orders.servicebus.windows.net",
        sharedAccessKey: "c2VjcmV0",
      },
    };
    expect(draftInvalidReason(draft, key)).toBeNull();
    expect(toSubmission(draft).draft.endpoints).toBe("orders.servicebus.windows.net");
  });

  /*
   * Blank is not a choice here. SQS falls back to the AWS credential chain and
   * Pub/Sub to Application Default Credentials; Service Bus signs every call
   * with a shared access key and has nothing to fall back on, so an empty
   * credential fails at the first call with a signature error naming nothing.
   */
  it("asks a Service Bus draft for a credential of some kind", () => {
    const base = {
      ...emptyAzureServiceBusDraft(),
      name: NAME,
      endpoints: "orders.servicebus.windows.net",
    };
    expect(draftInvalidReason({ protocol: "azure-servicebus", value: base }, key)).toBe(
      "page.connections.form.azure-servicebus.credentialRequired",
    );
    expect(
      draftInvalidReason(
        { protocol: "azure-servicebus", value: { ...base, sharedAccessKey: "c2VjcmV0" } },
        key,
      ),
    ).toBeNull();
    // The pasted string is the other way to fill the form, so it satisfies
    // the same requirement on its own.
    expect(
      draftInvalidReason(
        {
          protocol: "azure-servicebus",
          value: {
            ...base,
            connectionString:
              "Endpoint=sb://orders.servicebus.windows.net;SharedAccessKeyName=r;SharedAccessKey=k",
          },
        },
        key,
      ),
    ).toBeNull();
    // And the commonest mistake in that field is the namespace rather than
    // the string, which the SDK reports as a missing Endpoint key.
    expect(
      draftInvalidReason(
        {
          protocol: "azure-servicebus",
          value: { ...base, connectionString: "orders.servicebus.windows.net" },
        },
        key,
      ),
    ).toBe("page.connections.form.azure-servicebus.connectionStringMalformed");
  });

  // The admin client is pointed straight at this host, so a scheme in front
  // becomes part of the hostname and the failure names neither field nor why.
  it("holds the Service Bus emulator management host to a host and a port", () => {
    const base = {
      ...emptyAzureServiceBusDraft(),
      name: NAME,
      endpoints: "localhost:5672",
      sharedAccessKey: "c2VjcmV0",
    };
    expect(
      draftInvalidReason(
        {
          protocol: "azure-servicebus",
          value: { ...base, emulatorManagement: "http://127.0.0.1:5300" },
        },
        key,
      ),
    ).toBe("page.connections.form.azure-servicebus.emulatorManagementNoScheme");
    expect(
      draftInvalidReason(
        { protocol: "azure-servicebus", value: { ...base, emulatorManagement: "127.0.0.1:5300" } },
        key,
      ),
    ).toBeNull();
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
