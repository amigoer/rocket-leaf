/**
 * Each protocol's form fields translated into what ConnectionService stores.
 *
 * It lives beside the forms rather than inside them because the credentials
 * rules are the part worth testing: secrets are stored encrypted and never
 * come back, so a blank field means different things on a new connection, on
 * an edit, and after the clear control was used - and getting that wrong
 * either drops a working credential or stores an empty one.
 *
 * The draft is a union rather than one shared shape. A RocketMQ connection is
 * name servers and an ACL pair; a RabbitMQ one is two addresses, a virtual
 * host and a password; a Kafka one is a bootstrap list, a SASL mechanism and a
 * TLS block. Flattening them into one record would mean every field carrying a
 * note about which protocols it applies to.
 */
import {
  AuthMechanism,
  MQKind,
  type ConnectionDraft,
  type CredentialsMode,
} from "@/api/connection";
import type { Connection as ConnectionProfile } from "@/api/models";
import type { ProtocolId } from "@/design/data/protocols";
import {
  OPTION_ACCESS,
  OPTION_AMQP,
  OPTION_KAFKA_SCRAM_SHA,
  OPTION_KAFKA_TLS,
  OPTION_KAFKA_TLS_CA_FILE,
  OPTION_KAFKA_TLS_SKIP_VERIFY,
  OPTION_MQTT_CLEAN_START,
  OPTION_MQTT_CLIENT_ID,
  OPTION_MQTT_KEEP_ALIVE,
  OPTION_MQTT_MANAGEMENT_URL,
  OPTION_MQTT_PROTOCOL,
  OPTION_MQTT_SESSION_EXPIRY,
  OPTION_MQTT_TLS_CA_FILE,
  OPTION_MQTT_TLS_SKIP_VERIFY,
  OPTION_MQTT_TRANSPORT,
  OPTION_MQTT_WS_PATH,
  OPTION_NAMESPACE,
  OPTION_ACTIVEMQ_AMQP_URL,
  OPTION_ACTIVEMQ_BROKER_NAME,
  OPTION_ACTIVEMQ_JOLOKIA_PATH,
  OPTION_ACTIVEMQ_ORIGIN,
  OPTION_ACTIVEMQ_TLS_SKIP_VERIFY,
  OPTION_NSQ_LOOKUPD,
  OPTION_NSQ_TLS_SKIP_VERIFY,
  OPTION_IBMMQ_QUEUE_MANAGER,
  OPTION_IBMMQ_TLS_SKIP_VERIFY,
  OPTION_SOLACE_MSG_VPN,
  OPTION_SOLACE_REST_URL,
  OPTION_SOLACE_TLS_SKIP_VERIFY,
  OPTION_KINESIS_ENDPOINT_URL,
  OPTION_KINESIS_REGION,
  OPTION_KINESIS_STREAM_PREFIX,
  OPTION_SQS_ENDPOINT_URL,
  OPTION_SQS_QUEUE_PREFIX,
  OPTION_SQS_REGION,
  OPTION_PUBSUB_EMULATOR_HOST,
  OPTION_PUBSUB_PROJECT_ID,
  OPTION_PUBSUB_RESOURCE_PREFIX,
  OPTION_SB_EMULATOR_MANAGEMENT,
  OPTION_SB_ENTITY_PREFIX,
  OPTION_SB_KEY_NAME,
  OPTION_NATS_CREDS_FILE,
  OPTION_NATS_JS_DOMAIN,
  OPTION_NATS_MONITOR_URL,
  OPTION_NATS_TLS,
  OPTION_NATS_TLS_CA_FILE,
  OPTION_NATS_TLS_CERT_FILE,
  OPTION_NATS_TLS_KEY_FILE,
  OPTION_NATS_TLS_SKIP_VERIFY,
  OPTION_PULSAR_ADMIN_URL,
  OPTION_PULSAR_NAMESPACE,
  OPTION_PULSAR_TENANT,
  OPTION_PULSAR_TLS,
  OPTION_PULSAR_TLS_CA_FILE,
  OPTION_PULSAR_TLS_SKIP_VERIFY,
  OPTION_REDIS_DB,
  OPTION_REDIS_DEPLOYMENT,
  OPTION_REDIS_MASTER_NAME,
  OPTION_REDIS_STREAM_FILTER,
  OPTION_REDIS_TLS,
  OPTION_REDIS_TLS_SKIP_VERIFY,
  OPTION_TLS,
  OPTION_TLS_SKIP_VERIFY,
  OPTION_VERSION,
  OPTION_VHOST,
  emptyKafkaDraft,
  emptyMqttDraft,
  emptyNsqDraft,
  emptyIbmMqDraft,
  emptySolaceDraft,
  emptyKinesisDraft,
  emptySqsDraft,
  emptyGooglePubSubDraft,
  emptyAzureServiceBusDraft,
  emptyActiveMQDraft,
  emptyNatsDraft,
  emptyPulsarDraft,
  emptyRabbitMQDraft,
  emptyRedisDraft,
  emptyRocketMQDraft,
  type KafkaDraft,
  type KafkaMechanism,
  type MqttDraft,
  type MqttMechanism,
  type MqttProtocol,
  type MqttTransport,
  type NatsDraft,
  type NsqDraft,
  type IbmMqDraft,
  type IbmMqMechanism,
  type SolaceDraft,
  type SolaceMechanism,
  type KinesisDraft,
  type SqsDraft,
  type GooglePubSubDraft,
  type AzureServiceBusDraft,
  type ActiveMQDraft,
  type ActiveMQMechanism,
  type NatsMechanism,
  type PulsarAuth,
  type PulsarDraft,
  type RabbitMQDraft,
  type RedisDeployment,
  type RedisDraft,
  type RocketMQDraft,
} from "./ConnectionForms";

export interface Submission {
  draft: ConnectionDraft;
  credentialsMode: CredentialsMode;
}

/** One protocol's form state, tagged so the dialog can dispatch on it. */
export type ProtocolDraft =
  | { protocol: "rocketmq"; value: RocketMQDraft }
  | { protocol: "rabbitmq"; value: RabbitMQDraft }
  | { protocol: "kafka"; value: KafkaDraft }
  | { protocol: "pulsar"; value: PulsarDraft }
  | { protocol: "redis"; value: RedisDraft }
  | { protocol: "mqtt"; value: MqttDraft }
  | { protocol: "nats"; value: NatsDraft }
  | { protocol: "activemq"; value: ActiveMQDraft }
  | { protocol: "nsq"; value: NsqDraft }
  | { protocol: "sqs"; value: SqsDraft }
  | { protocol: "google-pubsub"; value: GooglePubSubDraft }
  | { protocol: "azure-servicebus"; value: AzureServiceBusDraft }
  | { protocol: "kinesis"; value: KinesisDraft }
  | { protocol: "ibmmq"; value: IbmMqDraft }
  | { protocol: "solace"; value: SolaceDraft };

/** The protocols this file can build a submission for. */
export const DRAFTABLE: readonly ProtocolDraft["protocol"][] = [
  "rocketmq",
  "rabbitmq",
  "kafka",
  "pulsar",
  "redis",
  "mqtt",
  "nats",
  "activemq",
  "nsq",
  "sqs",
  "google-pubsub",
  "azure-servicebus",
  "kinesis",
  "ibmmq",
  "solace",
];

export function isDraftable(protocol: ProtocolId): protocol is ProtocolDraft["protocol"] {
  return (DRAFTABLE as readonly string[]).includes(protocol);
}

export function emptyDraft(protocol: ProtocolDraft["protocol"]): ProtocolDraft {
  switch (protocol) {
    case "rabbitmq":
      return { protocol, value: emptyRabbitMQDraft() };
    case "kafka":
      return { protocol, value: emptyKafkaDraft() };
    case "pulsar":
      return { protocol, value: emptyPulsarDraft() };
    case "redis":
      return { protocol, value: emptyRedisDraft() };
    case "mqtt":
      return { protocol, value: emptyMqttDraft() };
    case "nats":
      return { protocol, value: emptyNatsDraft() };
    case "activemq":
      return { protocol, value: emptyActiveMQDraft() };
    case "nsq":
      return { protocol, value: emptyNsqDraft() };
    case "sqs":
      return { protocol, value: emptySqsDraft() };
    case "google-pubsub":
      return { protocol, value: emptyGooglePubSubDraft() };
    case "azure-servicebus":
      return { protocol, value: emptyAzureServiceBusDraft() };
    case "kinesis":
      return { protocol, value: emptyKinesisDraft() };
    case "ibmmq":
      return { protocol, value: emptyIbmMqDraft() };
    case "solace":
      return { protocol, value: emptySolaceDraft() };
    default:
      return { protocol, value: emptyRocketMQDraft() };
  }
}

export function toSubmission(draft: ProtocolDraft): Submission {
  switch (draft.protocol) {
    case "rabbitmq":
      return rabbitMQSubmission(draft.value);
    case "kafka":
      return kafkaSubmission(draft.value);
    case "pulsar":
      return pulsarSubmission(draft.value);
    case "redis":
      return redisSubmission(draft.value);
    case "mqtt":
      return mqttSubmission(draft.value);
    case "nats":
      return natsSubmission(draft.value);
    case "activemq":
      return activeMQSubmission(draft.value);
    case "nsq":
      return nsqSubmission(draft.value);
    case "sqs":
      return sqsSubmission(draft.value);
    case "google-pubsub":
      return googlePubSubSubmission(draft.value);
    case "azure-servicebus":
      return azureServiceBusSubmission(draft.value);
    case "kinesis":
      return kinesisSubmission(draft.value);
    case "ibmmq":
      return ibmMqSubmission(draft.value);
    case "solace":
      return solaceSubmission(draft.value);
    default:
      return rocketMQSubmission(draft.value);
  }
}

/** Reads a stored profile back into its own form's field set. */
export function toDraft(profile: ConnectionProfile): ProtocolDraft {
  switch (profile.kind) {
    case MQKind.KindRabbitMQ:
      return { protocol: "rabbitmq", value: toRabbitMQDraft(profile) };
    case MQKind.KindKafka:
      return { protocol: "kafka", value: toKafkaDraft(profile) };
    case MQKind.KindPulsar:
      return { protocol: "pulsar", value: toPulsarDraft(profile) };
    case MQKind.KindRedisStream:
      return { protocol: "redis", value: toRedisDraft(profile) };
    case MQKind.KindMQTT:
      return { protocol: "mqtt", value: toMqttDraft(profile) };
    case MQKind.KindNATS:
      return { protocol: "nats", value: toNatsDraft(profile) };
    case MQKind.KindActiveMQ:
      return { protocol: "activemq", value: toActiveMQDraft(profile) };
    case MQKind.KindNSQ:
      return { protocol: "nsq", value: toNsqDraft(profile) };
    case MQKind.KindSQS:
      return { protocol: "sqs", value: toSqsDraft(profile) };
    case MQKind.KindGooglePubSub:
      return { protocol: "google-pubsub", value: toGooglePubSubDraft(profile) };
    case MQKind.KindAzureServiceBus:
      return { protocol: "azure-servicebus", value: toAzureServiceBusDraft(profile) };
    case MQKind.KindKinesis:
      return { protocol: "kinesis", value: toKinesisDraft(profile) };
    case MQKind.KindIBMMQ:
      return { protocol: "ibmmq", value: toIbmMqDraft(profile) };
    case MQKind.KindSolace:
      return { protocol: "solace", value: toSolaceDraft(profile) };
    default:
      return { protocol: "rocketmq", value: toRocketMQDraft(profile) };
  }
}

function rocketMQSubmission(draft: RocketMQDraft): Submission {
  const accessKey = draft.accessKey.trim();
  const secretKey = draft.secretKey.trim();
  const typed = accessKey !== "" || secretKey !== "";
  const keepStored = draft.credentialsStored && !draft.clearCredentials;
  return {
    draft: {
      name: draft.name.trim(),
      group: draft.group,
      kind: MQKind.KindRocketMQ,
      endpoints: draft.endpoints.trim(),
      timeoutSec: draft.timeoutSec,
      authMechanism: typed || keepStored ? AuthMechanism.AuthACL : AuthMechanism.AuthNone,
      options: {
        [OPTION_VERSION]: draft.version,
        [OPTION_ACCESS]: draft.access,
        [OPTION_NAMESPACE]: draft.namespace.trim(),
      },
      secrets: { accessKey, secretKey },
      remark: draft.remark,
    },
    credentialsMode: draft.clearCredentials ? "clear" : keepStored ? "preserve" : "replace",
  };
}

function rabbitMQSubmission(draft: RabbitMQDraft): Submission {
  const username = draft.username.trim();
  const password = draft.password.trim();
  const typed = username !== "" || password !== "";
  const keepStored = draft.credentialsStored && !draft.clearCredentials;
  return {
    draft: {
      name: draft.name.trim(),
      group: draft.group,
      kind: MQKind.KindRabbitMQ,
      // The management API is the connection's address: it is the whole admin
      // plane, and the AMQP side is derived from it when left blank.
      endpoints: draft.management.trim(),
      timeoutSec: draft.timeoutSec,
      // RabbitMQ has no anonymous management API. A connection with no
      // credential cannot read anything, so the mechanism is not conditional
      // the way RocketMQ's optional ACL is.
      authMechanism: AuthMechanism.AuthPlain,
      options: {
        [OPTION_VHOST]: draft.vhost.trim() === "" ? "/" : draft.vhost.trim(),
        [OPTION_AMQP]: draft.amqp.trim(),
        [OPTION_TLS]: String(draft.tls),
        // Only meaningful with TLS on, and storing it otherwise would re-apply
        // it silently the day someone turns TLS back on.
        [OPTION_TLS_SKIP_VERIFY]: String(draft.tls && draft.tlsSkipVerify),
      },
      secrets: { username, password },
      remark: draft.remark,
    },
    credentialsMode: draft.clearCredentials ? "clear" : typed || !keepStored ? "replace" : "preserve",
  };
}

function kafkaSubmission(draft: KafkaDraft): Submission {
  const authenticating = draft.mechanism !== "none";
  const username = authenticating ? draft.username.trim() : "";
  const password = authenticating ? draft.password.trim() : "";
  const typed = username !== "" || password !== "";
  const keepStored = authenticating && draft.credentialsStored && !draft.clearCredentials;

  return {
    draft: {
      name: draft.name.trim(),
      group: draft.group,
      kind: MQKind.KindKafka,
      endpoints: draft.endpoints.trim(),
      timeoutSec: draft.timeoutSec,
      authMechanism: MECHANISM[draft.mechanism],
      options: {
        [OPTION_KAFKA_SCRAM_SHA]: draft.scramSha,
        [OPTION_KAFKA_TLS]: String(draft.tls),
        // The CA file and skip-verify only mean anything with TLS on, and
        // storing them otherwise would re-apply them silently the day someone
        // turns TLS back on.
        [OPTION_KAFKA_TLS_CA_FILE]: draft.tls ? draft.tlsCaFile.trim() : "",
        [OPTION_KAFKA_TLS_SKIP_VERIFY]: String(draft.tls && draft.tlsSkipVerify),
      },
      secrets: { username, password },
      remark: draft.remark,
    },
    // Anonymous is a real choice on Kafka, not a blank one, so dropping to it
    // clears the stored credential rather than leaving one that would come
    // back the day someone re-selects SASL.
    credentialsMode: !authenticating || draft.clearCredentials
      ? "clear"
      : typed || !keepStored
        ? "replace"
        : "preserve",
  };
}

function redisSubmission(draft: RedisDraft): Submission {
  const username = draft.username.trim();
  const password = draft.password.trim();
  const typed = username !== "" || password !== "";
  const keepStored = draft.credentialsStored && !draft.clearCredentials;
  const cluster = draft.deployment === "cluster";

  return {
    draft: {
      name: draft.name.trim(),
      group: draft.group,
      kind: MQKind.KindRedisStream,
      endpoints: draft.endpoints.trim(),
      timeoutSec: draft.timeoutSec,
      // Redis before 6 has no users and a password alone is the credential,
      // and 6 onwards treats an omitted username as the default user - so an
      // empty pair is an anonymous server rather than an unfinished form.
      authMechanism: typed || keepStored ? AuthMechanism.AuthPlain : AuthMechanism.AuthNone,
      options: {
        [OPTION_REDIS_DEPLOYMENT]: draft.deployment,
        // Only a sentinel connection has a master to name, and keeping a
        // stale one would send a standalone profile looking for it the day
        // someone switched back.
        [OPTION_REDIS_MASTER_NAME]: draft.deployment === "sentinel" ? draft.masterName.trim() : "",
        // A cluster has one database and refuses SELECT.
        [OPTION_REDIS_DB]: cluster ? "0" : String(draft.db),
        [OPTION_REDIS_STREAM_FILTER]: draft.streamFilter.trim(),
        [OPTION_REDIS_TLS]: String(draft.tls),
        // Only meaningful with TLS on, and storing it otherwise would re-apply
        // it silently the day someone turns TLS back on.
        [OPTION_REDIS_TLS_SKIP_VERIFY]: String(draft.tls && draft.tlsSkipVerify),
      },
      secrets: { username, password },
      remark: draft.remark,
    },
    credentialsMode: draft.clearCredentials ? "clear" : typed || !keepStored ? "replace" : "preserve",
  };
}

/*
 * MQTT is the one family here with two independent credentials: the broker's
 * username and password authenticate the session, and the management API key
 * authenticates a separate HTTP endpoint the protocol knows nothing about.
 *
 * That is why the mode is "preserve" wherever anything is stored, rather than
 * "replace" the moment something is typed the way the single-credential forms
 * do. Preserve fills blank fields per key, so typing a new API key keeps the
 * broker password; replace would submit the untouched password as blank and
 * wipe it.
 *
 * The cost is that a blank field can no longer mean "remove this one" - it
 * means "keep it", which is what the field's own placeholder says. Removing a
 * credential is the clear control, and it clears both, because the mode the
 * bridge takes is per-connection rather than per-secret.
 */
function mqttSubmission(draft: MqttDraft): Submission {
  const authenticating = draft.mechanism !== "none";
  const username = authenticating ? draft.username.trim() : "";
  const password = authenticating ? draft.password.trim() : "";
  const managementUrl = draft.managementUrl.trim();
  const managementKey = managementUrl === "" ? "" : draft.managementKey.trim();
  const managementSecret = managementUrl === "" ? "" : draft.managementSecret.trim();
  const keepStored = draft.credentialsStored && !draft.clearCredentials;
  const encrypted = draft.transport === "tls" || draft.transport === "wss";
  const webSocket = draft.transport === "ws" || draft.transport === "wss";

  return {
    draft: {
      name: draft.name.trim(),
      group: draft.group,
      kind: MQKind.KindMQTT,
      endpoints: draft.endpoints.trim(),
      timeoutSec: draft.timeoutSec,
      authMechanism: authenticating ? AuthMechanism.AuthPlain : AuthMechanism.AuthNone,
      options: {
        [OPTION_MQTT_PROTOCOL]: draft.protocol,
        [OPTION_MQTT_TRANSPORT]: draft.transport,
        // The path only means anything over WebSocket, and storing it
        // otherwise would re-apply it the day someone switches transport.
        [OPTION_MQTT_WS_PATH]: webSocket ? draft.wsPath.trim() : "",
        [OPTION_MQTT_CLIENT_ID]: draft.clientId.trim(),
        [OPTION_MQTT_KEEP_ALIVE]: String(draft.keepAliveSec),
        [OPTION_MQTT_CLEAN_START]: String(draft.cleanStart),
        // Session expiry is 5.0 only; 3.1.1 has no field for it.
        [OPTION_MQTT_SESSION_EXPIRY]:
          draft.protocol === "5" ? String(draft.sessionExpirySec) : "0",
        [OPTION_MQTT_TLS_CA_FILE]: encrypted ? draft.tlsCaFile.trim() : "",
        [OPTION_MQTT_TLS_SKIP_VERIFY]: String(encrypted && draft.tlsSkipVerify),
        [OPTION_MQTT_MANAGEMENT_URL]: managementUrl,
      },
      secrets: {
        username,
        password,
        managementApiKey: managementKey,
        managementSecretKey: managementSecret,
      },
      remark: draft.remark,
    },
    credentialsMode: draft.clearCredentials ? "clear" : keepStored ? "preserve" : "replace",
  };
}

function toMqttDraft(profile: ConnectionProfile): MqttDraft {
  const transport = MQTT_TRANSPORTS.includes(
    profile.options?.[OPTION_MQTT_TRANSPORT] as MqttTransport,
  )
    ? (profile.options?.[OPTION_MQTT_TRANSPORT] as MqttTransport)
    : "tcp";
  const encrypted = transport === "tls" || transport === "wss";
  const protocol: MqttProtocol = profile.options?.[OPTION_MQTT_PROTOCOL] === "311" ? "311" : "5";
  const mechanism: MqttMechanism =
    profile.authMechanism === AuthMechanism.AuthPlain ? "plain" : "none";

  return {
    name: profile.name,
    endpoints: profile.endpoints,
    protocol,
    transport,
    wsPath: profile.options?.[OPTION_MQTT_WS_PATH] || "/mqtt",
    clientId: profile.options?.[OPTION_MQTT_CLIENT_ID] ?? "",
    keepAliveSec: numberOption(profile.options?.[OPTION_MQTT_KEEP_ALIVE], 60),
    // Stored as a string, and only "false" turns it off: an older profile
    // written before the field existed has no value and must not silently
    // start resuming sessions.
    cleanStart: profile.options?.[OPTION_MQTT_CLEAN_START] !== "false",
    sessionExpirySec:
      protocol === "5" ? numberOption(profile.options?.[OPTION_MQTT_SESSION_EXPIRY], 0) : 0,
    mechanism,
    username: "",
    password: "",
    tlsCaFile: encrypted ? (profile.options?.[OPTION_MQTT_TLS_CA_FILE] ?? "") : "",
    tlsSkipVerify: encrypted && profile.options?.[OPTION_MQTT_TLS_SKIP_VERIFY] === "true",
    managementUrl: profile.options?.[OPTION_MQTT_MANAGEMENT_URL] ?? "",
    managementKey: "",
    managementSecret: "",
    group: profile.group,
    remark: profile.remark,
    timeoutSec: profile.timeoutSec,
    // Either credential counts: a connection may authenticate to the broker,
    // to the management API, or to both.
    credentialsStored: profile.secretsConfigured.length > 0,
    clearCredentials: false,
  };
}


/*
 * NATS collects up to two credentials, and which of the first one it collects
 * depends on the mechanism.
 *
 * Only the credential the chosen mechanism uses is submitted. The driver reads
 * exactly one and ignores the rest, so sending the others would store secrets
 * nothing will ever send - and a user who switched from a token to a password
 * would leave the token behind on disk.
 *
 * The system-account pair is submitted whatever the mechanism, because it is a
 * different account rather than a different way into the same one.
 */
function natsSubmission(draft: NatsDraft): Submission {
  const mechanism = draft.mechanism;
  const monitorUrl = draft.monitorUrl.trim();
  const keepStored = draft.credentialsStored && !draft.clearCredentials;
  const encrypted = draft.tls || mechanism === "mtls";

  return {
    draft: {
      name: draft.name.trim(),
      group: draft.group,
      kind: MQKind.KindNATS,
      endpoints: draft.endpoints.trim(),
      timeoutSec: draft.timeoutSec,
      authMechanism: NATS_MECHANISMS[mechanism],
      options: {
        [OPTION_NATS_TLS]: String(draft.tls),
        [OPTION_NATS_TLS_CA_FILE]: encrypted ? draft.tlsCaFile.trim() : "",
        [OPTION_NATS_TLS_SKIP_VERIFY]: String(encrypted && draft.tlsSkipVerify),
        // A client certificate is only presented under mutual TLS. Storing it
        // otherwise would re-apply it the day somebody changes mechanism.
        [OPTION_NATS_TLS_CERT_FILE]: mechanism === "mtls" ? draft.tlsCertFile.trim() : "",
        [OPTION_NATS_TLS_KEY_FILE]: mechanism === "mtls" ? draft.tlsKeyFile.trim() : "",
        [OPTION_NATS_CREDS_FILE]: mechanism === "creds" ? draft.credsFile.trim() : "",
        [OPTION_NATS_MONITOR_URL]: monitorUrl,
        [OPTION_NATS_JS_DOMAIN]: draft.jsDomain.trim(),
      },
      secrets: {
        username: mechanism === "plain" ? draft.username.trim() : "",
        password: mechanism === "plain" ? draft.password.trim() : "",
        token: mechanism === "token" ? draft.token.trim() : "",
        nkeySeed: mechanism === "nkey" ? draft.nkeySeed.trim() : "",
        systemUser: draft.systemUser.trim(),
        systemPassword: draft.systemPassword.trim(),
      },
      remark: draft.remark,
    },
    // "preserve" rather than "replace", for the reason MQTT has it: with two
    // credentials on one form, a user editing the monitoring address would
    // otherwise wipe whichever of them they did not retype.
    credentialsMode: draft.clearCredentials ? "clear" : keepStored ? "preserve" : "replace",
  };
}

const NATS_MECHANISMS: Record<NatsMechanism, AuthMechanism> = {
  none: AuthMechanism.AuthNone,
  plain: AuthMechanism.AuthPlain,
  token: AuthMechanism.AuthToken,
  nkey: AuthMechanism.AuthNKey,
  creds: AuthMechanism.AuthCreds,
  mtls: AuthMechanism.AuthMutualTLS,
};

function toNatsDraft(profile: ConnectionProfile): NatsDraft {
  const mechanism = NATS_MECHANISM_OF[profile.authMechanism] ?? "none";
  const tls = profile.options?.[OPTION_NATS_TLS] === "true";
  const encrypted = tls || mechanism === "mtls";

  return {
    name: profile.name,
    endpoints: profile.endpoints,
    mechanism,
    // Secrets never come back from the store. Blank with credentialsStored set
    // is what tells the form to say "kept" rather than "empty".
    username: "",
    password: "",
    token: "",
    nkeySeed: "",
    credsFile: mechanism === "creds" ? (profile.options?.[OPTION_NATS_CREDS_FILE] ?? "") : "",
    tls,
    tlsCaFile: encrypted ? (profile.options?.[OPTION_NATS_TLS_CA_FILE] ?? "") : "",
    tlsCertFile:
      mechanism === "mtls" ? (profile.options?.[OPTION_NATS_TLS_CERT_FILE] ?? "") : "",
    tlsKeyFile: mechanism === "mtls" ? (profile.options?.[OPTION_NATS_TLS_KEY_FILE] ?? "") : "",
    tlsSkipVerify: encrypted && profile.options?.[OPTION_NATS_TLS_SKIP_VERIFY] === "true",
    monitorUrl: profile.options?.[OPTION_NATS_MONITOR_URL] ?? "",
    systemUser: "",
    systemPassword: "",
    jsDomain: profile.options?.[OPTION_NATS_JS_DOMAIN] ?? "",
    group: profile.group,
    remark: profile.remark,
    timeoutSec: profile.timeoutSec,
    // Either credential counts: a connection may authenticate to its own
    // account, to the system account, or to both.
    credentialsStored: profile.secretsConfigured.length > 0,
    clearCredentials: false,
  };
}

const NATS_MECHANISM_OF: Partial<Record<AuthMechanism, NatsMechanism>> = {
  [AuthMechanism.AuthPlain]: "plain",
  [AuthMechanism.AuthToken]: "token",
  [AuthMechanism.AuthNKey]: "nkey",
  [AuthMechanism.AuthCreds]: "creds",
  [AuthMechanism.AuthMutualTLS]: "mtls",
};

const MQTT_TRANSPORTS: readonly MqttTransport[] = ["tcp", "tls", "ws", "wss"];

function numberOption(raw: string | undefined, fallback: number): number {
  const parsed = Number.parseInt(raw ?? "", 10);
  return Number.isNaN(parsed) ? fallback : parsed;
}

/** The form's three choices in the store's own vocabulary. */
const MECHANISM: Record<KafkaMechanism, AuthMechanism> = {
  none: AuthMechanism.AuthNone,
  "sasl-plain": AuthMechanism.AuthSASLPlain,
  "sasl-scram": AuthMechanism.AuthSASLScram,
};

function pulsarSubmission(draft: PulsarDraft): Submission {
  const authenticating = draft.auth !== "none";
  const token = authenticating ? draft.token.trim() : "";
  const typed = token !== "";
  const keepStored = authenticating && draft.credentialsStored && !draft.clearCredentials;

  return {
    draft: {
      name: draft.name.trim(),
      group: draft.group,
      kind: MQKind.KindPulsar,
      // The broker's binary address is the connection's own: it is what
      // publishes and reads, and the admin API is a second address beside it
      // rather than something derived from it.
      endpoints: draft.service.trim(),
      timeoutSec: draft.timeoutSec,
      authMechanism: authenticating ? AuthMechanism.AuthToken : AuthMechanism.AuthNone,
      options: {
        [OPTION_PULSAR_ADMIN_URL]: draft.admin.trim(),
        [OPTION_PULSAR_TENANT]: draft.tenant.trim(),
        [OPTION_PULSAR_NAMESPACE]: draft.namespace.trim(),
        [OPTION_PULSAR_TLS]: String(draft.tls),
        // The CA file and skip-verify only mean anything with TLS on, and
        // storing them otherwise would re-apply them silently the day someone
        // turns TLS back on.
        [OPTION_PULSAR_TLS_CA_FILE]: draft.tls ? draft.tlsCaFile.trim() : "",
        [OPTION_PULSAR_TLS_SKIP_VERIFY]: String(draft.tls && draft.tlsSkipVerify),
      },
      secrets: { token },
      remark: draft.remark,
    },
    // Anonymous is a real choice on Pulsar, not a blank one, so dropping to it
    // clears the stored token rather than leaving one that would come back the
    // day someone re-selects Token.
    credentialsMode: !authenticating || draft.clearCredentials
      ? "clear"
      : typed || !keepStored
        ? "replace"
        : "preserve",
  };
}

function toPulsarDraft(profile: ConnectionProfile): PulsarDraft {
  const tls = profile.options?.[OPTION_PULSAR_TLS] === "true";
  const auth: PulsarAuth = profile.authMechanism === AuthMechanism.AuthToken ? "token" : "none";
  return {
    name: profile.name,
    service: profile.endpoints,
    admin: profile.options?.[OPTION_PULSAR_ADMIN_URL] ?? "",
    tenant: profile.options?.[OPTION_PULSAR_TENANT] ?? "public",
    namespace: profile.options?.[OPTION_PULSAR_NAMESPACE] ?? "default",
    auth,
    token: "",
    tls,
    tlsCaFile: tls ? (profile.options?.[OPTION_PULSAR_TLS_CA_FILE] ?? "") : "",
    tlsSkipVerify: tls && profile.options?.[OPTION_PULSAR_TLS_SKIP_VERIFY] === "true",
    group: profile.group,
    remark: profile.remark,
    timeoutSec: profile.timeoutSec,
    // A profile that authenticates with nothing has no credential to keep,
    // whatever is still sitting in the secret store.
    credentialsStored: auth !== "none" && profile.secretsConfigured.length > 0,
    clearCredentials: false,
  };
}

function toRocketMQDraft(profile: ConnectionProfile): RocketMQDraft {
  return {
    name: profile.name,
    version: profile.options?.[OPTION_VERSION] === "4.x" ? "4.x" : "5.x",
    access: profile.options?.[OPTION_ACCESS] === "proxy" ? "proxy" : "ns",
    endpoints: profile.endpoints,
    accessKey: "",
    secretKey: "",
    group: profile.group,
    remark: profile.remark,
    timeoutSec: profile.timeoutSec,
    namespace: profile.options?.[OPTION_NAMESPACE] ?? "",
    credentialsStored: profile.secretsConfigured.length > 0,
    clearCredentials: false,
  };
}

function toKafkaDraft(profile: ConnectionProfile): KafkaDraft {
  const tls = profile.options?.[OPTION_KAFKA_TLS] === "true";
  const mechanism = KAFKA_MECHANISM_BY_STORED[profile.authMechanism] ?? "none";
  return {
    name: profile.name,
    endpoints: profile.endpoints,
    mechanism,
    scramSha: profile.options?.[OPTION_KAFKA_SCRAM_SHA] === "256" ? "256" : "512",
    username: "",
    password: "",
    tls,
    tlsCaFile: tls ? (profile.options?.[OPTION_KAFKA_TLS_CA_FILE] ?? "") : "",
    tlsSkipVerify: tls && profile.options?.[OPTION_KAFKA_TLS_SKIP_VERIFY] === "true",
    group: profile.group,
    remark: profile.remark,
    timeoutSec: profile.timeoutSec,
    // A profile that authenticates with nothing has no credential to keep,
    // whatever is still sitting in the secret store.
    credentialsStored: mechanism !== "none" && profile.secretsConfigured.length > 0,
    clearCredentials: false,
  };
}

const KAFKA_MECHANISM_BY_STORED: Partial<Record<AuthMechanism, KafkaMechanism>> = {
  [AuthMechanism.AuthSASLPlain]: "sasl-plain",
  [AuthMechanism.AuthSASLScram]: "sasl-scram",
};

function toRabbitMQDraft(profile: ConnectionProfile): RabbitMQDraft {
  const tls = profile.options?.[OPTION_TLS] === "true";
  return {
    name: profile.name,
    management: profile.endpoints,
    amqp: profile.options?.[OPTION_AMQP] ?? "",
    vhost: profile.options?.[OPTION_VHOST] ?? "/",
    username: "",
    password: "",
    tls,
    tlsSkipVerify: tls && profile.options?.[OPTION_TLS_SKIP_VERIFY] === "true",
    group: profile.group,
    remark: profile.remark,
    timeoutSec: profile.timeoutSec,
    credentialsStored: profile.secretsConfigured.length > 0,
    clearCredentials: false,
  };
}


function toRedisDraft(profile: ConnectionProfile): RedisDraft {
  const deployment = REDIS_DEPLOYMENTS.includes(
    profile.options?.[OPTION_REDIS_DEPLOYMENT] as RedisDeployment,
  )
    ? (profile.options?.[OPTION_REDIS_DEPLOYMENT] as RedisDeployment)
    : // A profile saved before this field existed is the standalone server it
      // has always connected to, which is what the driver assumes too.
      "standalone";
  const tls = profile.options?.[OPTION_REDIS_TLS] === "true";
  const db = Number.parseInt(profile.options?.[OPTION_REDIS_DB] ?? "0", 10);

  return {
    name: profile.name,
    deployment,
    endpoints: profile.endpoints,
    masterName: deployment === "sentinel" ? (profile.options?.[OPTION_REDIS_MASTER_NAME] ?? "") : "",
    db: deployment === "cluster" || Number.isNaN(db) || db < 0 ? 0 : db,
    streamFilter: profile.options?.[OPTION_REDIS_STREAM_FILTER] ?? "",
    username: "",
    password: "",
    tls,
    tlsSkipVerify: tls && profile.options?.[OPTION_REDIS_TLS_SKIP_VERIFY] === "true",
    group: profile.group,
    remark: profile.remark,
    timeoutSec: profile.timeoutSec,
    credentialsStored: profile.secretsConfigured.length > 0,
    clearCredentials: false,
  };
}

const REDIS_DEPLOYMENTS: readonly RedisDeployment[] = ["standalone", "sentinel", "cluster"];

function activeMQSubmission(draft: ActiveMQDraft): Submission {
  const mechanism = draft.mechanism;
  const keepStored = draft.credentialsStored && !draft.clearCredentials;
  const amqpUrl = draft.amqpUrl.trim();

  return {
    draft: {
      name: draft.name.trim(),
      group: draft.group,
      kind: MQKind.KindActiveMQ,
      endpoints: draft.endpoints.trim(),
      timeoutSec: draft.timeoutSec,
      authMechanism: ACTIVEMQ_MECHANISMS[mechanism],
      options: {
        [OPTION_ACTIVEMQ_JOLOKIA_PATH]: draft.jolokiaPath.trim(),
        [OPTION_ACTIVEMQ_BROKER_NAME]: draft.brokerName.trim(),
        [OPTION_ACTIVEMQ_ORIGIN]: draft.originHeader.trim(),
        [OPTION_ACTIVEMQ_AMQP_URL]: amqpUrl,
        [OPTION_ACTIVEMQ_TLS_SKIP_VERIFY]: String(draft.tlsSkipVerify),
      },
      secrets: {
        username: mechanism === "plain" ? draft.username.trim() : "",
        password: mechanism === "plain" ? draft.password.trim() : "",
        // Dropped with the address they belong to: keeping them would
        // re-apply them the day somebody fills the acceptor back in with a
        // different account in mind.
        amqpUsername: amqpUrl === "" ? "" : draft.amqpUsername.trim(),
        amqpPassword: amqpUrl === "" ? "" : draft.amqpPassword.trim(),
      },
      remark: draft.remark,
    },
    // "preserve" rather than "replace", for the reason MQTT and NATS have it:
    // with two credentials on one form, a user editing the console address
    // would otherwise wipe whichever pair they did not retype.
    credentialsMode: draft.clearCredentials ? "clear" : keepStored ? "preserve" : "replace",
  };
}

const ACTIVEMQ_MECHANISMS: Record<ActiveMQMechanism, AuthMechanism> = {
  none: AuthMechanism.AuthNone,
  plain: AuthMechanism.AuthPlain,
};

function toActiveMQDraft(profile: ConnectionProfile): ActiveMQDraft {
  const mechanism: ActiveMQMechanism =
    profile.authMechanism === AuthMechanism.AuthNone ? "none" : "plain";

  return {
    name: profile.name,
    endpoints: profile.endpoints,
    mechanism,
    // Secrets never come back from the store. Blank with credentialsStored set
    // is what tells the form to say "kept" rather than "empty".
    username: "",
    password: "",
    jolokiaPath: profile.options?.[OPTION_ACTIVEMQ_JOLOKIA_PATH] ?? "",
    brokerName: profile.options?.[OPTION_ACTIVEMQ_BROKER_NAME] ?? "",
    originHeader: profile.options?.[OPTION_ACTIVEMQ_ORIGIN] ?? "",
    tlsSkipVerify: profile.options?.[OPTION_ACTIVEMQ_TLS_SKIP_VERIFY] === "true",
    amqpUrl: profile.options?.[OPTION_ACTIVEMQ_AMQP_URL] ?? "",
    amqpUsername: "",
    amqpPassword: "",
    group: profile.group,
    remark: profile.remark,
    timeoutSec: profile.timeoutSec,
    credentialsStored: profile.secretsConfigured.length > 0,
    clearCredentials: false,
  };
}

/**
 * NSQ stores no credential, so this is the one submission with nothing to
 * decide about secrets.
 *
 * "replace" rather than "preserve": the mode governs a secrets map, and an
 * empty one written every time is the honest instruction for a family whose
 * management API takes no credential at all. Preserving would keep whatever a
 * profile of another kind had left behind if its kind were ever changed.
 */
function nsqSubmission(draft: NsqDraft): Submission {
  return {
    draft: {
      name: draft.name.trim(),
      group: draft.group,
      kind: MQKind.KindNSQ,
      endpoints: draft.endpoints.trim(),
      timeoutSec: draft.timeoutSec,
      authMechanism: AuthMechanism.AuthNone,
      options: {
        [OPTION_NSQ_LOOKUPD]: draft.lookupdEndpoints.trim(),
        [OPTION_NSQ_TLS_SKIP_VERIFY]: String(draft.tlsSkipVerify),
      },
      secrets: {},
      remark: draft.remark,
    },
    credentialsMode: "replace",
  };
}

function toNsqDraft(profile: ConnectionProfile): NsqDraft {
  return {
    name: profile.name,
    endpoints: profile.endpoints,
    lookupdEndpoints: profile.options?.[OPTION_NSQ_LOOKUPD] ?? "",
    tlsSkipVerify: profile.options?.[OPTION_NSQ_TLS_SKIP_VERIFY] === "true",
    group: profile.group,
    remark: profile.remark,
    timeoutSec: profile.timeoutSec,
  };
}

/*
 * SQS is the one submission with nothing to put in `endpoints`, and that empty
 * string is the point of the whole family.
 *
 * Every other form here writes an address into it. This one has none to write:
 * the region is an option, the SDK builds the endpoint from it, and the driver
 * declares no endpoint field - which is what lets the connection service accept
 * a profile whose address is blank rather than refusing it as unfinished.
 *
 * The credential pair is optional. Absent, the driver uses the machine's own
 * AWS identity, so an empty pair is a real choice rather than a blank form -
 * which is why the mechanism drops to none rather than staying plain and
 * dialling with nothing to sign with.
 */
function sqsSubmission(draft: SqsDraft): Submission {
  const accessKeyId = draft.accessKeyId.trim();
  const secretAccessKey = draft.secretAccessKey.trim();
  const sessionToken = draft.sessionToken.trim();
  const typed = accessKeyId !== "" || secretAccessKey !== "" || sessionToken !== "";
  const keepStored = draft.credentialsStored && !draft.clearCredentials;

  return {
    draft: {
      name: draft.name.trim(),
      group: draft.group,
      kind: MQKind.KindSQS,
      // No address, because there is none. The region below is what says
      // where the queues are.
      endpoints: "",
      timeoutSec: draft.timeoutSec,
      authMechanism: typed || keepStored ? AuthMechanism.AuthPlain : AuthMechanism.AuthNone,
      options: {
        [OPTION_SQS_REGION]: draft.region.trim(),
        [OPTION_SQS_QUEUE_PREFIX]: draft.queuePrefix.trim(),
        [OPTION_SQS_ENDPOINT_URL]: draft.endpointUrl.trim(),
      },
      secrets: {
        // Not accessKey and secretKey: those two names are reserved for
        // RocketMQ's ACL and are cleared on save for any other family.
        awsAccessKeyId: accessKeyId,
        awsSecretAccessKey: secretAccessKey,
        awsSessionToken: sessionToken,
      },
      remark: draft.remark,
    },
    // "preserve" rather than "replace", for the reason MQTT and NATS have it:
    // the session token is a third credential on one form, and a user editing
    // the region would otherwise wipe whichever of the three they did not
    // retype.
    credentialsMode: draft.clearCredentials ? "clear" : keepStored ? "preserve" : "replace",
  };
}

function toSqsDraft(profile: ConnectionProfile): SqsDraft {
  return {
    name: profile.name,
    region: profile.options?.[OPTION_SQS_REGION] ?? "",
    // Secrets never come back from the store. Blank with credentialsStored set
    // is what tells the form to say "kept" rather than "empty".
    accessKeyId: "",
    secretAccessKey: "",
    sessionToken: "",
    queuePrefix: profile.options?.[OPTION_SQS_QUEUE_PREFIX] ?? "",
    endpointUrl: profile.options?.[OPTION_SQS_ENDPOINT_URL] ?? "",
    group: profile.group,
    remark: profile.remark,
    timeoutSec: profile.timeoutSec,
    // A profile signing with the machine's own identity has no credential to
    // keep, whatever is still sitting in the secret store.
    credentialsStored:
      profile.authMechanism === AuthMechanism.AuthPlain && profile.secretsConfigured.length > 0,
    clearCredentials: false,
  };
}

/*
 * The second submission with nothing to put in `endpoints`, and the first
 * whose credential is a whole document.
 *
 * The empty string is the point of the family: the project is an option, the
 * client builds the endpoint from nothing at all, and the driver declares no
 * endpoint field - which is what lets the connection service accept a profile
 * whose address is blank rather than refusing it as unfinished.
 *
 * The key is optional. Absent, the driver uses Application Default Credentials
 * - a gcloud login, GOOGLE_APPLICATION_CREDENTIALS, the metadata server on a
 * workload inside Google Cloud - so an empty field is a real choice rather
 * than a blank form, which is why the mechanism drops to none rather than
 * staying plain and connecting with nothing to sign with.
 */
function googlePubSubSubmission(draft: GooglePubSubDraft): Submission {
  const credentials = draft.credentialsJson.trim();
  const keepStored = draft.credentialsStored && !draft.clearCredentials;

  return {
    draft: {
      name: draft.name.trim(),
      group: draft.group,
      kind: MQKind.KindGooglePubSub,
      // No address, because there is none. The project below is what says
      // where the topics are.
      endpoints: "",
      timeoutSec: draft.timeoutSec,
      authMechanism:
        credentials !== "" || keepStored ? AuthMechanism.AuthPlain : AuthMechanism.AuthNone,
      options: {
        [OPTION_PUBSUB_PROJECT_ID]: draft.projectId.trim(),
        [OPTION_PUBSUB_RESOURCE_PREFIX]: draft.resourcePrefix.trim(),
        [OPTION_PUBSUB_EMULATOR_HOST]: draft.emulatorHost.trim(),
      },
      secrets: {
        // Not accessKey and secretKey: those two names are reserved for
        // RocketMQ's ACL and are cleared on save for any other family.
        // The key is sent verbatim rather than trimmed of its newlines - it
        // is JSON, and the private key inside it is line-sensitive.
        googleCredentialsJson: credentials,
      },
      remark: draft.remark,
    },
    credentialsMode: draft.clearCredentials ? "clear" : keepStored ? "preserve" : "replace",
  };
}

function toGooglePubSubDraft(profile: ConnectionProfile): GooglePubSubDraft {
  return {
    name: profile.name,
    projectId: profile.options?.[OPTION_PUBSUB_PROJECT_ID] ?? "",
    // Secrets never come back from the store. Blank with credentialsStored set
    // is what tells the form to say "kept" rather than "empty".
    credentialsJson: "",
    resourcePrefix: profile.options?.[OPTION_PUBSUB_RESOURCE_PREFIX] ?? "",
    emulatorHost: profile.options?.[OPTION_PUBSUB_EMULATOR_HOST] ?? "",
    group: profile.group,
    remark: profile.remark,
    timeoutSec: profile.timeoutSec,
    // A profile using the machine's own identity has no credential to keep,
    // whatever is still sitting in the secret store.
    credentialsStored:
      profile.authMechanism === AuthMechanism.AuthPlain && profile.secretsConfigured.length > 0,
    clearCredentials: false,
  };
}

/*
 * The third hosted submission and the first with something to put in
 * `endpoints`.
 *
 * The namespace goes there rather than into an option because it is an
 * address: the driver builds its connection string from it and both clients
 * dial it. That is the whole difference from the two hosted families before
 * this one, and it is why RequiresEndpoints is true here and false there.
 *
 * Two secrets rather than one, because there are two ways to hold a Service
 * Bus credential and people have both. Neither is accessKey or secretKey -
 * those names are reserved for RocketMQ's ACL and are cleared on save for any
 * other family.
 */
function azureServiceBusSubmission(draft: AzureServiceBusDraft): Submission {
  const key = draft.sharedAccessKey.trim();
  const connectionString = draft.connectionString.trim();
  const keepStored = draft.credentialsStored && !draft.clearCredentials;
  const given = key !== "" || connectionString !== "" || keepStored;

  return {
    draft: {
      name: draft.name.trim(),
      group: draft.group,
      kind: MQKind.KindAzureServiceBus,
      endpoints: draft.endpoints.trim(),
      timeoutSec: draft.timeoutSec,
      // There is no ambient credential here the way SQS and Pub/Sub have one,
      // so a profile with nothing signed is a profile that cannot connect -
      // but the mechanism still drops to none rather than claiming plain, so
      // the connection list says so rather than showing a credential it has
      // not got.
      authMechanism: given ? AuthMechanism.AuthPlain : AuthMechanism.AuthNone,
      options: {
        [OPTION_SB_KEY_NAME]: draft.keyName.trim(),
        [OPTION_SB_ENTITY_PREFIX]: draft.entityPrefix.trim(),
        [OPTION_SB_EMULATOR_MANAGEMENT]: draft.emulatorManagement.trim(),
      },
      secrets: {
        azureSharedAccessKey: key,
        azureConnectionString: connectionString,
      },
      remark: draft.remark,
    },
    credentialsMode: draft.clearCredentials ? "clear" : keepStored ? "preserve" : "replace",
  };
}

function toAzureServiceBusDraft(profile: ConnectionProfile): AzureServiceBusDraft {
  return {
    name: profile.name,
    endpoints: profile.endpoints,
    // The portal's own default, so an edit of a profile saved before this
    // field existed reads as the policy it was actually using.
    keyName: profile.options?.[OPTION_SB_KEY_NAME] || "RootManageSharedAccessKey",
    // Secrets never come back from the store. Blank with credentialsStored set
    // is what tells the form to say "kept" rather than "empty".
    sharedAccessKey: "",
    connectionString: "",
    entityPrefix: profile.options?.[OPTION_SB_ENTITY_PREFIX] ?? "",
    emulatorManagement: profile.options?.[OPTION_SB_EMULATOR_MANAGEMENT] ?? "",
    group: profile.group,
    remark: profile.remark,
    timeoutSec: profile.timeoutSec,
    credentialsStored:
      profile.authMechanism === AuthMechanism.AuthPlain && profile.secretsConfigured.length > 0,
    clearCredentials: false,
  };
}

/*
 * The second submission that writes an empty `endpoints`, and it is SQS's for
 * the same reason rather than by copying: a stream is reached by naming a
 * region and signing a request, so there is no address to write and the driver
 * declares no endpoint field for one to go in.
 *
 * The credential pair is optional here too. Absent, the driver uses the
 * machine's own AWS identity, so an empty pair is a real choice rather than a
 * blank form - which is why the mechanism drops to none rather than staying
 * plain and dialling with nothing to sign with.
 */
function kinesisSubmission(draft: KinesisDraft): Submission {
  const accessKeyId = draft.accessKeyId.trim();
  const secretAccessKey = draft.secretAccessKey.trim();
  const sessionToken = draft.sessionToken.trim();
  const typed = accessKeyId !== "" || secretAccessKey !== "" || sessionToken !== "";
  const keepStored = draft.credentialsStored && !draft.clearCredentials;

  return {
    draft: {
      name: draft.name.trim(),
      group: draft.group,
      kind: MQKind.KindKinesis,
      // No address, because there is none. The region below is what says
      // where the streams are.
      endpoints: "",
      timeoutSec: draft.timeoutSec,
      authMechanism: typed || keepStored ? AuthMechanism.AuthPlain : AuthMechanism.AuthNone,
      options: {
        [OPTION_KINESIS_REGION]: draft.region.trim(),
        [OPTION_KINESIS_STREAM_PREFIX]: draft.streamPrefix.trim(),
        [OPTION_KINESIS_ENDPOINT_URL]: draft.endpointUrl.trim(),
      },
      secrets: {
        // Not accessKey and secretKey: those two names are reserved for
        // RocketMQ's ACL and are cleared on save for any other family.
        awsAccessKeyId: accessKeyId,
        awsSecretAccessKey: secretAccessKey,
        awsSessionToken: sessionToken,
      },
      remark: draft.remark,
    },
    // "preserve" for the reason SQS has it: the session token is a third
    // credential on one form, and a user editing the region would otherwise
    // wipe whichever of the three they did not retype.
    credentialsMode: draft.clearCredentials ? "clear" : keepStored ? "preserve" : "replace",
  };
}

function toKinesisDraft(profile: ConnectionProfile): KinesisDraft {
  return {
    name: profile.name,
    region: profile.options?.[OPTION_KINESIS_REGION] ?? "",
    // Secrets never come back from the store. Blank with credentialsStored set
    // is what tells the form to say "kept" rather than "empty".
    accessKeyId: "",
    secretAccessKey: "",
    sessionToken: "",
    streamPrefix: profile.options?.[OPTION_KINESIS_STREAM_PREFIX] ?? "",
    endpointUrl: profile.options?.[OPTION_KINESIS_ENDPOINT_URL] ?? "",
    group: profile.group,
    remark: profile.remark,
    timeoutSec: profile.timeoutSec,
    // A profile signing with the machine's own identity has no credential to
    // keep, whatever is still sitting in the secret store.
    credentialsStored:
      profile.authMechanism === AuthMechanism.AuthPlain && profile.secretsConfigured.length > 0,
    clearCredentials: false,
  };
}

/*
 * IBM MQ, which has an address like the on-premise families and two credential
 * pairs like ActiveMQ.
 *
 * The address is the mqweb server's URL. The queue manager is an option rather
 * than part of it because it is a path segment on that server, and it may be
 * left blank: the driver asks the server which ones it fronts and takes the
 * answer when there is exactly one.
 *
 * The second pair is the messaging interface's, and it is dropped when the
 * mechanism is none for the reason the first one is - a connection that
 * authenticates nobody has no account to remember. Otherwise it is written as
 * typed, including empty, which is what tells the driver to reuse the
 * administrative pair.
 */
function ibmMqSubmission(draft: IbmMqDraft): Submission {
  const mechanism = draft.mechanism;
  const keepStored = draft.credentialsStored && !draft.clearCredentials;
  const authenticated = mechanism === "plain";

  return {
    draft: {
      name: draft.name.trim(),
      group: draft.group,
      kind: MQKind.KindIBMMQ,
      endpoints: draft.endpoints.trim(),
      timeoutSec: draft.timeoutSec,
      authMechanism: IBMMQ_MECHANISMS[mechanism],
      options: {
        [OPTION_IBMMQ_QUEUE_MANAGER]: draft.queueManager.trim(),
        [OPTION_IBMMQ_TLS_SKIP_VERIFY]: String(draft.tlsSkipVerify),
      },
      secrets: {
        // Not accessKey and secretKey: those two names are reserved for
        // RocketMQ's ACL and are cleared on save for any other family.
        username: authenticated ? draft.username.trim() : "",
        password: authenticated ? draft.password.trim() : "",
        messagingUsername: authenticated ? draft.messagingUsername.trim() : "",
        messagingPassword: authenticated ? draft.messagingPassword.trim() : "",
      },
      remark: draft.remark,
    },
    // "preserve" rather than "replace", for the reason ActiveMQ has it: with
    // two credential pairs on one form, a user editing the address would
    // otherwise wipe whichever pair they did not retype.
    credentialsMode: draft.clearCredentials ? "clear" : keepStored ? "preserve" : "replace",
  };
}

const IBMMQ_MECHANISMS: Record<IbmMqMechanism, AuthMechanism> = {
  none: AuthMechanism.AuthNone,
  plain: AuthMechanism.AuthPlain,
};

function toIbmMqDraft(profile: ConnectionProfile): IbmMqDraft {
  const mechanism: IbmMqMechanism =
    profile.authMechanism === AuthMechanism.AuthNone ? "none" : "plain";

  return {
    name: profile.name,
    endpoints: profile.endpoints,
    mechanism,
    // Secrets never come back from the store. Blank with credentialsStored set
    // is what tells the form to say "kept" rather than "empty".
    username: "",
    password: "",
    queueManager: profile.options?.[OPTION_IBMMQ_QUEUE_MANAGER] ?? "",
    messagingUsername: "",
    messagingPassword: "",
    tlsSkipVerify: profile.options?.[OPTION_IBMMQ_TLS_SKIP_VERIFY] === "true",
    group: profile.group,
    remark: profile.remark,
    timeoutSec: profile.timeoutSec,
    credentialsStored: profile.secretsConfigured.length > 0,
    clearCredentials: false,
  };
}

/*
 * Solace PubSub+, which has an address like the on-premise families and two
 * credential pairs like IBM MQ - but the second pair means something else.
 *
 * The address is the broker's SEMP URL. The Message VPN is an option rather
 * than part of it because it is a path segment on that broker, and it may be
 * left blank: every broker ships one called "default" and the driver falls
 * back to it.
 *
 * The REST pair does not fall back to the SEMP one, which is where this parts
 * company with IBM MQ. Both of that family's interfaces authenticate against
 * one mqweb registry, so reusing the administrative account is the ordinary
 * deployment; Solace's two are a broker-wide management user and a
 * client-username inside one Message VPN, and offering the first as the second
 * would be refused by any broker that checks. So an empty REST pair is written
 * as empty and means "send no credential", which is what a Message VPN whose
 * basic authentication type is none expects.
 */
function solaceSubmission(draft: SolaceDraft): Submission {
  const mechanism = draft.mechanism;
  const keepStored = draft.credentialsStored && !draft.clearCredentials;
  const authenticated = mechanism === "plain";

  return {
    draft: {
      name: draft.name.trim(),
      group: draft.group,
      kind: MQKind.KindSolace,
      endpoints: draft.endpoints.trim(),
      timeoutSec: draft.timeoutSec,
      authMechanism: SOLACE_MECHANISMS[mechanism],
      options: {
        [OPTION_SOLACE_MSG_VPN]: draft.msgVpn.trim(),
        [OPTION_SOLACE_REST_URL]: draft.restUrl.trim(),
        [OPTION_SOLACE_TLS_SKIP_VERIFY]: String(draft.tlsSkipVerify),
      },
      secrets: {
        // Not accessKey and secretKey: those two names are reserved for
        // RocketMQ's ACL and are cleared on save for any other family.
        username: authenticated ? draft.username.trim() : "",
        password: authenticated ? draft.password.trim() : "",
        // Kept whatever the SEMP mechanism is, because it is a different
        // credential for a different interface: a broker with management
        // security switched off still authenticates its clients.
        restUsername: draft.restUsername.trim(),
        restPassword: draft.restPassword.trim(),
      },
      remark: draft.remark,
    },
    // "preserve" rather than "replace", for the reason ActiveMQ and IBM MQ
    // have it: with two credential pairs on one form, a user editing the
    // address would otherwise wipe whichever pair they did not retype.
    credentialsMode: draft.clearCredentials ? "clear" : keepStored ? "preserve" : "replace",
  };
}

const SOLACE_MECHANISMS: Record<SolaceMechanism, AuthMechanism> = {
  none: AuthMechanism.AuthNone,
  plain: AuthMechanism.AuthPlain,
};

function toSolaceDraft(profile: ConnectionProfile): SolaceDraft {
  const mechanism: SolaceMechanism =
    profile.authMechanism === AuthMechanism.AuthNone ? "none" : "plain";

  return {
    name: profile.name,
    endpoints: profile.endpoints,
    mechanism,
    // Secrets never come back from the store. Blank with credentialsStored set
    // is what tells the form to say "kept" rather than "empty".
    username: "",
    password: "",
    msgVpn: profile.options?.[OPTION_SOLACE_MSG_VPN] ?? "",
    restUrl: profile.options?.[OPTION_SOLACE_REST_URL] ?? "",
    restUsername: "",
    restPassword: "",
    tlsSkipVerify: profile.options?.[OPTION_SOLACE_TLS_SKIP_VERIFY] === "true",
    group: profile.group,
    remark: profile.remark,
    timeoutSec: profile.timeoutSec,
    credentialsStored: profile.secretsConfigured.length > 0,
    clearCredentials: false,
  };
}
