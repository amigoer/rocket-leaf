/**
 * Whether a connection form can be submitted, and the message when it cannot.
 *
 * These rules sit beside the draft rather than inside the dialog for the same
 * reason the draft itself does: they are the part worth testing, and a rule
 * that only exists inside a useMemo can only be reached by rendering.
 *
 * The dispatch is exhaustive on purpose. It used to end in a bare fallthrough
 * that happened to hold RocketMQ's rules, so a protocol added to ProtocolDraft
 * without a branch of its own was silently asked for a name server and a
 * RocketMQ namespace. The compiler says so now instead.
 */
import type { ProtocolDraft } from "./connectionDraft";

/** Just enough of i18next's t for these rules: a key in, a message out. */
type Translate = (key: string) => string;

/**
 * How many addresses a comma-, semicolon- or whitespace-separated field holds.
 *
 * It mirrors the driver's own parseAddrs so the form refuses what the driver
 * would refuse. The two are separate implementations on purpose - one is
 * validation the user sees while typing, the other is the last word - but they
 * have to agree on the count, which is what internal/driver/redisstream's
 * TestStandaloneRefusesASecondAddress and this file's tests each pin.
 */
export function countAddresses(raw: string): number {
  return raw.split(/[,;\s]+/).filter((part) => part.trim() !== "").length;
}

/** The reason the draft cannot be saved, or null when it can. */
export function draftInvalidReason(draft: ProtocolDraft, t: Translate): string | null {
  const shared = draft.value;
  if (shared.name.trim() === "") return t("page.connections.nameRequired");
  // 0 is blank, which means the connection takes the timeout from settings.
  if (shared.timeoutSec < 0 || shared.timeoutSec > 300) {
    return t("page.connections.timeoutRange");
  }
  if (draft.protocol === "rabbitmq") {
    if (draft.value.management.trim() === "") {
      return t("page.connections.managementRequired");
    }
    // The management API has no anonymous mode, so a connection with no
    // credential cannot read a single page.
    const stored = draft.value.credentialsStored && !draft.value.clearCredentials;
    if (!stored && draft.value.username.trim() === "") {
      return t("page.connections.usernameRequired");
    }
    return null;
  }
  if (draft.protocol === "kafka") {
    if (draft.value.endpoints.trim() === "") {
      return t("page.connections.bootstrapRequired");
    }
    // Anonymous is a real choice here, so the credential is only required
    // once a mechanism has been picked.
    if (draft.value.mechanism !== "none") {
      const stored = draft.value.credentialsStored && !draft.value.clearCredentials;
      if (!stored && draft.value.username.trim() === "") {
        return t("page.connections.usernameRequired");
      }
    }
    return null;
  }
  if (draft.protocol === "pulsar") {
    if (draft.value.service.trim() === "") {
      return t("page.connections.serviceUrlRequired");
    }
    // The admin API is where every listing comes from, and it is a second
    // address rather than one derived from the service URL, so a profile
    // without it can connect and read nothing.
    if (draft.value.admin.trim() === "") {
      return t("page.connections.adminUrlRequired");
    }
    // A topic is addressed as tenant/namespace/name, so a profile naming
    // neither has no scope to read within.
    if (draft.value.tenant.trim() === "" || draft.value.namespace.trim() === "") {
      return t("page.connections.scopeRequired");
    }
    // Anonymous is a real choice here, so the token is only required once
    // Token has been picked.
    if (draft.value.auth === "token") {
      const stored = draft.value.credentialsStored && !draft.value.clearCredentials;
      if (!stored && draft.value.token.trim() === "") {
        return t("page.connections.tokenRequired");
      }
    }
    return null;
  }
  if (draft.protocol === "redis") {
    if (draft.value.endpoints.trim() === "") {
      return t("page.connections.endpointsRequired");
    }
    // go-redis builds a cluster client the moment it is handed a second
    // address, so a standalone profile with two would stop being standalone
    // without saying so. The driver refuses it; saying so here means the
    // user finds out while the field is still in front of them.
    if (draft.value.deployment === "standalone" && countAddresses(draft.value.endpoints) > 1) {
      return t("page.connections.form.redis.oneAddressOnly");
    }
    // Without a master name a sentinel connection has nothing to ask for,
    // and would end up talking to the sentinels themselves - which answer
    // PING and hold no streams at all.
    if (draft.value.deployment === "sentinel" && draft.value.masterName.trim() === "") {
      return t("page.connections.form.redis.masterNameRequired");
    }
    return null;
  }
  if (draft.protocol === "mqtt") {
    if (draft.value.endpoints.trim() === "") {
      return t("page.connections.brokerRequired");
    }
    // Anonymous is a real choice on MQTT, so the credential is only required
    // once the user has asked to authenticate.
    if (draft.value.mechanism !== "none") {
      const stored = draft.value.credentialsStored && !draft.value.clearCredentials;
      if (!stored && draft.value.username.trim() === "") {
        return t("page.connections.usernameRequired");
      }
    }
    // A management endpoint with no key would reach EMQX and be refused on
    // every call, which reads as a broken endpoint rather than a missing
    // credential.
    if (draft.value.managementUrl.trim() !== "") {
      const stored = draft.value.credentialsStored && !draft.value.clearCredentials;
      if (!stored && draft.value.managementKey.trim() === "") {
        return t("page.connections.form.mqtt.managementKeyRequired");
      }
    }
    return null;
  }
  if (draft.protocol === "nats") {
    if (draft.value.endpoints.trim() === "") {
      return t("page.connections.form.nats.serversRequired");
    }
    // Anonymous is a real choice on NATS, so a credential is only required
    // once the user has picked a mechanism that needs one. Each mechanism
    // needs a different one, and naming the wrong field would send them
    // looking at a row that is not on screen.
    const stored = draft.value.credentialsStored && !draft.value.clearCredentials;
    if (!stored) {
      if (draft.value.mechanism === "plain" && draft.value.username.trim() === "") {
        return t("page.connections.usernameRequired");
      }
      if (draft.value.mechanism === "token" && draft.value.token.trim() === "") {
        return t("page.connections.form.nats.tokenRequired");
      }
      if (draft.value.mechanism === "nkey" && draft.value.nkeySeed.trim() === "") {
        return t("page.connections.form.nats.nkeySeedRequired");
      }
    }
    // A creds file is a path rather than a secret, so it is required whether
    // or not credentials are already stored.
    if (draft.value.mechanism === "creds" && draft.value.credsFile.trim() === "") {
      return t("page.connections.form.nats.credsFileRequired");
    }
    // Mutual TLS needs both halves: a certificate with no key cannot be
    // presented, and the server would report it as a failed handshake.
    if (draft.value.mechanism === "mtls") {
      if (draft.value.tlsCertFile.trim() === "" || draft.value.tlsKeyFile.trim() === "") {
        return t("page.connections.form.nats.certPairRequired");
      }
    }
    // A system user with no password reaches $SYS and is refused on every
    // call, which reads as a broken endpoint rather than a missing half.
    if (!stored && draft.value.systemUser.trim() !== "" && draft.value.systemPassword.trim() === "") {
      return t("page.connections.form.nats.systemPasswordRequired");
    }
    return null;
  }
  if (draft.protocol === "activemq") {
    if (draft.value.endpoints.trim() === "") return t("page.connections.endpointsRequired");
    // A broker port here is the commonest way to get this wrong: every other
    // family's endpoint is one, and this one's is the console over HTTP.
    if (!/^https?:\/\//i.test(draft.value.endpoints.trim())) {
      return t("page.connections.form.activemq.consoleScheme");
    }
    return null;
  }
  if (draft.protocol === "rocketmq") {
    if (draft.value.endpoints.trim() === "") return t("page.connections.endpointsRequired");
    if (draft.value.version === "5.x" && draft.value.access === "proxy") {
      return t("page.connections.form.rocketmq.proxyNote");
    }
    // '%' is what RocketMQ joins a namespace to a topic with, so one inside the
    // namespace would produce a name nothing could take apart again. The rest
    // is the broker's own rule for a topic or group, minus that separator.
    if (!/^[\w|-]*$/.test(draft.value.namespace.trim())) {
      return t("page.connections.form.rocketmq.namespaceInvalid");
    }
    return null;
  }
  return unhandledProtocol(draft);
}

/**
 * Reports a ProtocolDraft member no branch above handled.
 *
 * The parameter is never, so a protocol added to the union without rules of
 * its own fails to compile here rather than inheriting whichever branch
 * happens to be last.
 */
function unhandledProtocol(draft: never): never {
  throw new Error(`no validation rules for protocol ${(draft as ProtocolDraft).protocol}`);
}
