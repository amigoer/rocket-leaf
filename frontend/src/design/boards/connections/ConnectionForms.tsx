import { useState, type CSSProperties, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { ChevronRight } from "lucide-react";
import { Input } from "@/components/ui/input";
import { Switch } from "@/components/ui/switch";
import {
  Segmented,
  SelectField,
} from "@/components";

/** Label (with optional grey hint) above the control. */
function Fld({
  label,
  hint,
  span,
  children,
}: {
  label: ReactNode;
  hint?: ReactNode;
  /** Set to make the field span both grid columns. */
  span?: boolean;
  children: ReactNode;
}) {
  return (
    <div
      className="flex min-w-0 flex-col gap-1.5 text-xs"
      style={span ? { gridColumn: "1/3" } : undefined}
    >
      {/*
        * The free space goes above the label, not below it.
        *
        * Both fields in a row are as tall as the taller hint, which is what
        * lines their inputs up. Growing the label instead put that space
        * between a short label and its own input: "连接名称" sat four lines
        * clear of the box it names, while the wrapped hint beside it ran right
        * down to its own. Pushing the pair down keeps the inputs aligned and
        * the label against the thing it labels.
        */}
      <span className="mt-auto font-medium">
        {label} {hint != null && <span className="font-normal text-(--c-muted-2)">{hint}</span>}
      </span>
      {children}
    </div>
  );
}

const GRID = {
  display: "grid",
  gridTemplateColumns: "1fr 1fr",
  gap: "12px 14px",
} as const;

const MONO = { fontSize: "11.5px" } as const;

/** The 高级 disclosure line and the right-hand caveat under every form. */
function FormNote({ advanced, note }: { advanced: ReactNode; note: ReactNode }) {
  return (
    <div style={{ display: "flex", justifyContent: "space-between", fontSize: "11px", color: "var(--c-muted)" }}>
      <span>{advanced}</span>
      <span>{note}</span>
    </div>
  );
}

/** Layout for a switch and its explanation on one row. */
const SWITCH_ROW: CSSProperties = {
  display: "flex",
  alignItems: "center",
  gap: "8px",
  marginTop: "3px",
};

/** Option keys the RocketMQ driver reads back off a stored profile. */
export const OPTION_VERSION = "version";
export const OPTION_ACCESS = "access";
export const OPTION_NAMESPACE = "namespace";

/** What Go falls back to when the profile asks for no timeout of its own. */
const DEFAULT_TIMEOUT_SEC = 5;

/**
 * What board 6a collects. The fields the canvas does not draw - group, remark,
 * request timeout - ride along so editing a profile round-trips them instead
 * of clearing what another screen set.
 */
export interface RocketMQDraft {
  name: string;
  version: "4.x" | "5.x";
  access: "ns" | "proxy";
  endpoints: string;
  accessKey: string;
  secretKey: string;
  group: string;
  remark: string;
  timeoutSec: number;
  /**
   * RocketMQ 5.x namespace, empty for a connection that sees the cluster
   * unscoped. It is not a filter: the driver wraps every topic and group name
   * it sends, the way a client's ClientConfig.namespace does.
   */
  namespace: string;
  /**
   * Editing a profile that already has ACL credentials. Go never sends a
   * stored secret back, so the fields show that one exists rather than a value,
   * and a blank field means "keep it" - which is why clearing needs its own
   * gesture below.
   */
  credentialsStored: boolean;
  /** Set by the clear control: submits as credentialsMode "clear". */
  clearCredentials: boolean;
}

export function emptyRocketMQDraft(): RocketMQDraft {
  return {
    name: "",
    version: "5.x",
    access: "ns",
    endpoints: "",
    accessKey: "",
    secretKey: "",
    group: "",
    remark: "",
    timeoutSec: DEFAULT_TIMEOUT_SEC,
    namespace: "",
    credentialsStored: false,
    clearCredentials: false,
  };
}

/** Board 6a — RocketMQ. Version and access mode drive which fields exist. */
export function RocketMQForm({
  value,
  onChange,
}: {
  value: RocketMQDraft;
  onChange: (next: RocketMQDraft) => void;
}) {
  const { t } = useTranslation();
  const set = <K extends keyof RocketMQDraft>(key: K, next: RocketMQDraft[K]) =>
    onChange({ ...value, [key]: next });
  // Opens on a profile that already sets one of these, so editing never hides
  // a value the connection is actually using.
  const [advancedOpen, setAdvancedOpen] = useState(
    value.timeoutSec !== DEFAULT_TIMEOUT_SEC || value.remark !== "" || value.namespace !== "",
  );
  // Once cleared, the fields are empty and typing into them is what sets new
  // credentials, so the placeholder and the clear control both go away.
  const stored = value.credentialsStored && !value.clearCredentials;

  return (
    <>
      <div style={GRID}>
        <Fld label={t("page.connections.form.name")}>
          <Input
            value={value.name}
            placeholder="rocketmq-order"
            onChange={(event) => set("name", event.target.value)}
          />
        </Fld>
        <Fld label={t("page.connections.form.rocketmq.version")}>
          <Segmented
            block
            value={value.version}
            onChange={(next: "4.x" | "5.x") =>
              // 4.x has no Proxy, so leaving access on it would submit a mode
              // the version cannot have.
              onChange({ ...value, version: next, access: next === "4.x" ? "ns" : value.access })
            }
            options={[
              { value: "4.x", label: "4.x" },
              { value: "5.x", label: "5.x" },
            ]}
          />
        </Fld>
        {value.version === "5.x" && (
          <Fld span label={t("page.connections.form.rocketmq.access")} hint={t("page.connections.form.rocketmq.accessHint")}>
            <Segmented
              block
              value={value.access}
              onChange={(next: "ns" | "proxy") => set("access", next)}
              options={[
                { value: "ns", label: t("page.connections.form.rocketmq.accessDirect") },
                { value: "proxy", label: "gRPC Proxy" },
              ]}
            />
          </Fld>
        )}
        <Fld span label={t("page.connections.form.rocketmq.nameServer")} hint={t("page.connections.form.rocketmq.nameServerHint")}>
          <Input
            className="mono3"
            style={MONO}
            value={value.endpoints}
            placeholder="10.12.3.44:9876;10.12.3.45:9876"
            onChange={(event) => set("endpoints", event.target.value)}
          />
        </Fld>
        <Fld
          label="AccessKey"
          hint={
            stored ? (
              <button type="button" className="mqs-linkbtn" onClick={() => set("clearCredentials", true)}>
                {t("page.connections.form.clearCredentials")}
              </button>
            ) : (
              t("page.connections.form.rocketmq.accessKeyHint")
            )
          }
        >
          <Input
            value={value.accessKey}
            placeholder={stored ? t("page.connections.form.secretStored") : undefined}
            onChange={(event) => set("accessKey", event.target.value)}
          />
        </Fld>
        <Fld label="SecretKey">
          <Input
            type="password"
            value={value.secretKey}
            placeholder={stored ? t("page.connections.form.secretStored") : undefined}
            onChange={(event) => set("secretKey", event.target.value)}
          />
        </Fld>
      </div>
      <FormNote
        advanced={
          <button
            type="button"
            className="mqs-disclosure"
            aria-expanded={advancedOpen}
            onClick={() => setAdvancedOpen((open) => !open)}
          >
            <ChevronRight size={12} aria-hidden />
            {t("page.connections.form.rocketmq.advanced")}
          </button>
        }
        note={
          value.access === "proxy"
            ? t("page.connections.form.rocketmq.proxyNote")
            : t("page.connections.form.rocketmq.note")
        }
      />
      {advancedOpen && (
        <div style={GRID}>
          <Fld
            label={t("page.connections.form.rocketmq.timeout")}
            hint={t("page.connections.form.rocketmq.timeoutHint")}
          >
            <Input
              type="number"
              min={1}
              max={300}
              // Blank is a real state: Go reads 0 as "use the default".
              value={value.timeoutSec > 0 ? String(value.timeoutSec) : ""}
              onChange={(event) => {
                const seconds = Number.parseInt(event.target.value, 10);
                set("timeoutSec", Number.isNaN(seconds) ? 0 : seconds);
              }}
            />
          </Fld>
          <Fld label={t("page.connections.form.remark")} hint={t("page.connections.form.remarkHint")}>
            <Input value={value.remark} onChange={(event) => set("remark", event.target.value)} />
          </Fld>
          <Fld
            label={t("page.connections.form.rocketmq.namespace")}
            hint={t("page.connections.form.rocketmq.namespaceHint")}
          >
            <Input
              value={value.namespace}
              placeholder="MQ_INST_1234567890_xxxxxxx"
              onChange={(event) => set("namespace", event.target.value)}
            />
          </Fld>
          {/* Still dead: the admin library dials with name servers, a timeout
              and ACL, and nothing else, so a trace topic and TLS have nowhere
              to go until it grows the options. */}
          <Fld
            label={t("page.connections.form.rocketmq.traceTopic")}
            hint={t("page.connections.soon")}
          >
            <Input disabled placeholder="RMQ_SYS_TRACE_TOPIC" />
          </Fld>
          <Fld span label="TLS" hint={t("page.connections.soon")}>
            <div style={{ display: "flex", alignItems: "center", gap: "8px", marginTop: "3px" }}>
              <Switch disabled />
              <span style={{ color: "var(--c-muted)" }}>
                {t("page.connections.form.rocketmq.tlsNote")}
              </span>
            </div>
          </Fld>
        </div>
      )}
    </>
  );
}

/**
 * Option keys the Kafka driver reads back off a stored profile.
 *
 * They repeat two of RabbitMQ's strings, and are declared separately on
 * purpose: each set is a private contract between one Go driver and this form,
 * so renaming one family's key must not silently rename the other's.
 */
export const OPTION_KAFKA_SCRAM_SHA = "scramSha";
export const OPTION_KAFKA_TLS = "tls";
export const OPTION_KAFKA_TLS_SKIP_VERIFY = "tlsSkipVerify";
export const OPTION_KAFKA_TLS_CA_FILE = "tlsCaFile";

/** How a Kafka connection authenticates. Anonymous is a real option here. */
export type KafkaMechanism = "none" | "sasl-plain" | "sasl-scram";

/** Kafka's two SCRAM mechanisms are separate credentials on the broker. */
export type KafkaScramSha = "256" | "512";

/**
 * What the Kafka form collects.
 *
 * One address list rather than two: Kafka administers itself over the protocol
 * that carries records, so there is no second endpoint to name. The bootstrap
 * list is only a starting point - the cluster answers with the address of
 * every broker, and those are what the client actually talks to.
 */
export interface KafkaDraft {
  name: string;
  /** Bootstrap servers. This is the profile's endpoints field. */
  endpoints: string;
  mechanism: KafkaMechanism;
  scramSha: KafkaScramSha;
  username: string;
  password: string;
  tls: boolean;
  tlsCaFile: string;
  tlsSkipVerify: boolean;
  group: string;
  remark: string;
  timeoutSec: number;
  /** A stored password never comes back, so blank means "keep it". */
  credentialsStored: boolean;
  clearCredentials: boolean;
}

export function emptyKafkaDraft(): KafkaDraft {
  return {
    name: "",
    endpoints: "",
    mechanism: "none",
    scramSha: "512",
    username: "",
    password: "",
    tls: false,
    tlsCaFile: "",
    tlsSkipVerify: false,
    group: "",
    remark: "",
    timeoutSec: DEFAULT_TIMEOUT_SEC,
    credentialsStored: false,
    clearCredentials: false,
  };
}

/** Board 6b — Kafka. The mechanism decides whether the credential rows show. */
export function KafkaForm({
  value,
  onChange,
}: {
  value: KafkaDraft;
  onChange: (next: KafkaDraft) => void;
}) {
  const { t } = useTranslation();
  const set = <K extends keyof KafkaDraft>(key: K, next: KafkaDraft[K]) =>
    onChange({ ...value, [key]: next });
  const [advancedOpen, setAdvancedOpen] = useState(
    value.timeoutSec !== DEFAULT_TIMEOUT_SEC || value.remark !== "" || value.tls,
  );
  const authenticating = value.mechanism !== "none";
  const stored = value.credentialsStored && !value.clearCredentials;

  return (
    <>
      <div style={GRID}>
        <Fld label={t("page.connections.form.name")}>
          <Input
            value={value.name}
            placeholder="kafka-staging"
            onChange={(event) => set("name", event.target.value)}
          />
        </Fld>
        <Fld label={t("page.connections.form.kafka.mechanism")}>
          <SelectField<KafkaMechanism>
            value={value.mechanism}
            options={[
              { value: "none", label: t("page.connections.form.kafka.mechanismNone") },
              { value: "sasl-plain", label: "SASL/PLAIN" },
              { value: "sasl-scram", label: "SASL/SCRAM" },
            ]}
            onValueChange={(next) =>
              // Dropping to anonymous drops the credential with it. Keeping it
              // would put the old password back the day someone re-selects
              // SASL, without them being shown that it was still there.
              onChange({
                ...value,
                mechanism: next,
                username: next === "none" ? "" : value.username,
                password: next === "none" ? "" : value.password,
              })
            }
          />
        </Fld>
        <Fld
          span
          label={t("page.connections.form.kafka.bootstrap")}
          hint={t("page.connections.form.kafka.bootstrapHint")}
        >
          <Input
            className="mono3"
            style={MONO}
            value={value.endpoints}
            placeholder="kafka-1:9092, kafka-2:9092, kafka-3:9092"
            onChange={(event) => set("endpoints", event.target.value)}
          />
        </Fld>
        {authenticating && (
          <>
            <Fld
              label={t("page.connections.form.username")}
              hint={
                stored ? (
                  <button type="button" className="mqs-linkbtn" onClick={() => set("clearCredentials", true)}>
                    {t("page.connections.form.clearCredentials")}
                  </button>
                ) : undefined
              }
            >
              <Input
                value={value.username}
                placeholder={stored ? t("page.connections.form.secretStored") : undefined}
                onChange={(event) => set("username", event.target.value)}
              />
            </Fld>
            <Fld label={t("page.connections.form.password")}>
              <Input
                type="password"
                value={value.password}
                placeholder={stored ? t("page.connections.form.secretStored") : undefined}
                onChange={(event) => set("password", event.target.value)}
              />
            </Fld>
          </>
        )}
        {value.mechanism === "sasl-scram" && (
          <Fld
            span
            label={t("page.connections.form.kafka.scramSha")}
            hint={t("page.connections.form.kafka.scramShaHint")}
          >
            <Segmented<KafkaScramSha>
              options={[
                { value: "256", label: "SCRAM-SHA-256" },
                { value: "512", label: "SCRAM-SHA-512" },
              ]}
              value={value.scramSha}
              onChange={(next) => set("scramSha", next)}
            />
          </Fld>
        )}
      </div>
      <FormNote
        advanced={
          <button
            type="button"
            className="mqs-disclosure"
            aria-expanded={advancedOpen}
            onClick={() => setAdvancedOpen((open) => !open)}
          >
            <ChevronRight size={12} aria-hidden />
            {t("page.connections.form.kafka.advanced")}
          </button>
        }
        note={t("page.connections.form.kafka.note")}
      />
      {advancedOpen && (
        <div style={GRID}>
          <Fld
            label={t("page.connections.form.rocketmq.timeout")}
            hint={t("page.connections.form.rocketmq.timeoutHint")}
          >
            <Input
              type="number"
              min={1}
              max={300}
              value={value.timeoutSec > 0 ? String(value.timeoutSec) : ""}
              onChange={(event) => {
                const seconds = Number.parseInt(event.target.value, 10);
                set("timeoutSec", Number.isNaN(seconds) ? 0 : seconds);
              }}
            />
          </Fld>
          <Fld label={t("page.connections.form.remark")} hint={t("page.connections.form.remarkHint")}>
            <Input value={value.remark} onChange={(event) => set("remark", event.target.value)} />
          </Fld>
          <Fld span label="TLS" hint={t("page.connections.form.kafka.tlsHint")}>
            <div style={SWITCH_ROW}>
              <Switch
                checked={value.tls}
                onCheckedChange={(next: boolean) =>
                  // The CA file and skip-verify only mean anything with TLS on,
                  // and leaving them set would silently re-apply them.
                  onChange({
                    ...value,
                    tls: next,
                    tlsCaFile: next ? value.tlsCaFile : "",
                    tlsSkipVerify: next && value.tlsSkipVerify,
                  })
                }
              />
              <span style={{ color: "var(--c-muted)" }}>
                {t("page.connections.form.kafka.tls")}
              </span>
            </div>
          </Fld>
          {value.tls && (
            <>
              <Fld
                span
                label={t("page.connections.form.kafka.caFile")}
                hint={t("page.connections.form.kafka.caFileHint")}
              >
                <Input
                  className="mono3"
                  style={MONO}
                  value={value.tlsCaFile}
                  placeholder="/etc/kafka/ca.pem"
                  onChange={(event) => set("tlsCaFile", event.target.value)}
                />
              </Fld>
              <Fld
                span
                label={t("page.connections.form.kafka.skipVerify")}
                hint={t("page.connections.form.kafka.skipVerifyHint")}
              >
                <div style={SWITCH_ROW}>
                  <Switch
                    checked={value.tlsSkipVerify}
                    onCheckedChange={(next: boolean) => set("tlsSkipVerify", next)}
                  />
                  <span style={{ color: "var(--c-muted)" }}>
                    {t("page.connections.form.kafka.skipVerifyNote")}
                  </span>
                </div>
              </Fld>
            </>
          )}
        </div>
      )}
    </>
  );
}

/** Option keys the RabbitMQ driver reads back off a stored profile. */
export const OPTION_MQTT_PROTOCOL = "protocolVersion";
export const OPTION_MQTT_TRANSPORT = "transport";
export const OPTION_MQTT_WS_PATH = "wsPath";
export const OPTION_MQTT_CLIENT_ID = "clientId";
export const OPTION_MQTT_KEEP_ALIVE = "keepAliveSec";
export const OPTION_MQTT_CLEAN_START = "cleanStart";
export const OPTION_MQTT_SESSION_EXPIRY = "sessionExpirySec";
export const OPTION_MQTT_TLS_CA_FILE = "tlsCaFile";
export const OPTION_MQTT_TLS_SKIP_VERIFY = "tlsSkipVerify";
export const OPTION_MQTT_MANAGEMENT_URL = "managementUrl";

export const OPTION_VHOST = "vhost";
export const OPTION_AMQP = "amqpEndpoint";
export const OPTION_TLS = "tls";
export const OPTION_TLS_SKIP_VERIFY = "tlsSkipVerify";

/**
 * What the RabbitMQ form collects.
 *
 * Two addresses rather than one, because RabbitMQ is two listeners: the
 * management API answers the admin pages over HTTP, and AMQP carries the
 * messages. They need not be on one host, so both are asked for - but the AMQP
 * one is optional and derived from the management host when blank, which is
 * what most deployments want.
 */
export interface RabbitMQDraft {
  name: string;
  /** The management API address. This is the profile's endpoints field. */
  management: string;
  amqp: string;
  vhost: string;
  username: string;
  password: string;
  tls: boolean;
  tlsSkipVerify: boolean;
  group: string;
  remark: string;
  timeoutSec: number;
  /** A stored password never comes back, so blank means "keep it". */
  credentialsStored: boolean;
  clearCredentials: boolean;
}

export function emptyRabbitMQDraft(): RabbitMQDraft {
  return {
    name: "",
    management: "",
    amqp: "",
    vhost: "/",
    username: "",
    password: "",
    tls: false,
    tlsSkipVerify: false,
    group: "",
    remark: "",
    timeoutSec: DEFAULT_TIMEOUT_SEC,
    credentialsStored: false,
    clearCredentials: false,
  };
}

/** Board 6c — RabbitMQ. The management API is the whole admin plane. */
export function RabbitMQForm({
  value,
  onChange,
}: {
  value: RabbitMQDraft;
  onChange: (next: RabbitMQDraft) => void;
}) {
  const { t } = useTranslation();
  const set = <K extends keyof RabbitMQDraft>(key: K, next: RabbitMQDraft[K]) =>
    onChange({ ...value, [key]: next });
  const [advancedOpen, setAdvancedOpen] = useState(
    value.timeoutSec !== DEFAULT_TIMEOUT_SEC || value.remark !== "" || value.tls,
  );
  const stored = value.credentialsStored && !value.clearCredentials;

  return (
    <>
      <div style={GRID}>
        <Fld label={t("page.connections.form.name")}>
          <Input
            value={value.name}
            placeholder="rabbit-staging"
            onChange={(event) => set("name", event.target.value)}
          />
        </Fld>
        <Fld label={t("page.connections.form.rabbitmq.vhost")}>
          <Input
            className="mono3"
            style={MONO}
            value={value.vhost}
            placeholder="/"
            onChange={(event) => set("vhost", event.target.value)}
          />
        </Fld>
        <Fld
          span
          label={t("page.connections.form.rabbitmq.management")}
          hint={t("page.connections.form.rabbitmq.managementHint")}
        >
          <Input
            className="mono3"
            style={MONO}
            value={value.management}
            placeholder="http://rabbit.example.com:15672"
            onChange={(event) => set("management", event.target.value)}
          />
        </Fld>
        <Fld
          span
          label={t("page.connections.form.rabbitmq.amqp")}
          hint={t("page.connections.form.rabbitmq.amqpHint")}
        >
          <Input
            className="mono3"
            style={MONO}
            value={value.amqp}
            placeholder={value.tls ? "amqps://rabbit.example.com:5671" : "amqp://rabbit.example.com:5672"}
            onChange={(event) => set("amqp", event.target.value)}
          />
        </Fld>
        <Fld
          label={t("page.connections.form.username")}
          hint={
            stored ? (
              <button type="button" className="mqs-linkbtn" onClick={() => set("clearCredentials", true)}>
                {t("page.connections.form.clearCredentials")}
              </button>
            ) : (
              t("page.connections.form.rabbitmq.usernameHint")
            )
          }
        >
          <Input
            value={value.username}
            placeholder={stored ? t("page.connections.form.secretStored") : undefined}
            onChange={(event) => set("username", event.target.value)}
          />
        </Fld>
        <Fld label={t("page.connections.form.password")}>
          <Input
            type="password"
            value={value.password}
            placeholder={stored ? t("page.connections.form.secretStored") : undefined}
            onChange={(event) => set("password", event.target.value)}
          />
        </Fld>
      </div>
      <FormNote
        advanced={
          <button
            type="button"
            className="mqs-disclosure"
            aria-expanded={advancedOpen}
            onClick={() => setAdvancedOpen((open) => !open)}
          >
            <ChevronRight size={12} aria-hidden />
            {t("page.connections.form.rabbitmq.advanced")}
          </button>
        }
        note={t("page.connections.form.rabbitmq.note")}
      />
      {advancedOpen && (
        <div style={GRID}>
          <Fld
            label={t("page.connections.form.rocketmq.timeout")}
            hint={t("page.connections.form.rocketmq.timeoutHint")}
          >
            <Input
              type="number"
              min={1}
              max={300}
              value={value.timeoutSec > 0 ? String(value.timeoutSec) : ""}
              onChange={(event) => {
                const seconds = Number.parseInt(event.target.value, 10);
                set("timeoutSec", Number.isNaN(seconds) ? 0 : seconds);
              }}
            />
          </Fld>
          <Fld label={t("page.connections.form.remark")} hint={t("page.connections.form.remarkHint")}>
            <Input value={value.remark} onChange={(event) => set("remark", event.target.value)} />
          </Fld>
          <Fld span label="TLS" hint={t("page.connections.form.rabbitmq.tlsHint")}>
            <div style={SWITCH_ROW}>
              <Switch
                checked={value.tls}
                onCheckedChange={(next: boolean) =>
                  // Skipping verification only means anything with TLS on, and
                  // leaving it set while TLS is off would silently re-apply it.
                  onChange({ ...value, tls: next, tlsSkipVerify: next && value.tlsSkipVerify })
                }
              />
              <span style={{ color: "var(--c-muted)" }}>
                {t("page.connections.form.rabbitmq.tls")}
              </span>
            </div>
          </Fld>
          {value.tls && (
            <Fld span label={t("page.connections.form.rabbitmq.tlsSkipVerify")} hint={t("page.connections.form.rabbitmq.tlsSkipVerifyHint")}>
              <div style={SWITCH_ROW}>
                <Switch
                  checked={value.tlsSkipVerify}
                  onCheckedChange={(next: boolean) => set("tlsSkipVerify", next)}
                />
                <span style={{ color: "var(--c-muted)" }}>
                  {t("page.connections.form.rabbitmq.tlsSkipVerifyNote")}
                </span>
              </div>
            </Fld>
          )}
        </div>
      )}
    </>
  );
}

/** Option and secret keys the Pulsar driver reads back off a stored profile. */
export const OPTION_PULSAR_ADMIN_URL = "adminUrl";
export const OPTION_PULSAR_TENANT = "tenant";
export const OPTION_PULSAR_NAMESPACE = "namespace";
export const OPTION_PULSAR_TLS = "tls";
export const OPTION_PULSAR_TLS_CA_FILE = "tlsCaFile";
export const OPTION_PULSAR_TLS_SKIP_VERIFY = "tlsSkipVerify";

/** The two authentications this driver implements. */
export type PulsarAuth = "none" | "token";

/**
 * What the Pulsar form collects.
 *
 * Two addresses, because Pulsar is two listeners: the web service answers the
 * admin pages over HTTP and the broker port carries messages over the binary
 * protocol. Neither is derived from the other - they are routinely behind
 * different ingresses, and guessing 8080 from a 6650 host is wrong the moment
 * either is proxied.
 *
 * The tenant and namespace are part of the connection rather than a filter on
 * a page: a Pulsar topic is addressed as tenant/namespace/name, so a profile
 * that names neither has no scope to read within.
 */
export interface PulsarDraft {
  name: string;
  /** The broker's binary address. This is the profile's endpoints field. */
  service: string;
  admin: string;
  tenant: string;
  namespace: string;
  auth: PulsarAuth;
  token: string;
  tls: boolean;
  tlsCaFile: string;
  tlsSkipVerify: boolean;
  group: string;
  remark: string;
  timeoutSec: number;
  /** A stored token never comes back, so blank means "keep it". */
  credentialsStored: boolean;
  clearCredentials: boolean;
}

export function emptyPulsarDraft(): PulsarDraft {
  return {
    name: "",
    service: "",
    admin: "",
    tenant: "public",
    namespace: "default",
    auth: "none",
    token: "",
    tls: false,
    tlsCaFile: "",
    tlsSkipVerify: false,
    group: "",
    remark: "",
    timeoutSec: DEFAULT_TIMEOUT_SEC,
    credentialsStored: false,
    clearCredentials: false,
  };
}

/** Board 6d — Pulsar. The auth choice decides whether the token row shows. */
export function PulsarForm({
  value,
  onChange,
}: {
  value: PulsarDraft;
  onChange: (next: PulsarDraft) => void;
}) {
  const { t } = useTranslation();
  const set = <K extends keyof PulsarDraft>(key: K, next: PulsarDraft[K]) =>
    onChange({ ...value, [key]: next });
  const [advancedOpen, setAdvancedOpen] = useState(
    value.timeoutSec !== DEFAULT_TIMEOUT_SEC || value.remark !== "" || value.tls,
  );
  const stored = value.credentialsStored && !value.clearCredentials;

  return (
    <>
      <div style={GRID}>
        <Fld label={t("page.connections.form.name")}>
          <Input
            value={value.name}
            placeholder="pulsar-staging"
            onChange={(event) => set("name", event.target.value)}
          />
        </Fld>
        <Fld label={t("page.connections.form.pulsar.auth")}>
          <SelectField<PulsarAuth>
            value={value.auth}
            options={[
              { value: "none", label: t("page.connections.form.pulsar.authNone") },
              { value: "token", label: "Token" },
            ]}
            onValueChange={(next) =>
              // Dropping to anonymous drops the token with it. Keeping it would
              // put the old credential back the day someone re-selects Token,
              // without them being shown that it was still there.
              onChange({ ...value, auth: next, token: next === "none" ? "" : value.token })
            }
          />
        </Fld>
        <Fld
          span
          label={t("page.connections.form.pulsar.service")}
          hint={t("page.connections.form.pulsar.serviceHint")}
        >
          <Input
            className="mono3"
            style={MONO}
            value={value.service}
            placeholder="pulsar://localhost:6650"
            onChange={(event) => set("service", event.target.value)}
          />
        </Fld>
        <Fld
          span
          label={t("page.connections.form.pulsar.admin")}
          hint={t("page.connections.form.pulsar.adminHint")}
        >
          <Input
            className="mono3"
            style={MONO}
            value={value.admin}
            placeholder="http://localhost:8080"
            onChange={(event) => set("admin", event.target.value)}
          />
        </Fld>
        {value.auth === "token" && (
          <Fld
            span
            label="Token"
            hint={
              stored ? (
                <button
                  type="button"
                  className="mqs-linkbtn"
                  onClick={() => set("clearCredentials", true)}
                >
                  {t("page.connections.form.clearCredentials")}
                </button>
              ) : undefined
            }
          >
            <Input
              type="password"
              className="mono3"
              style={MONO}
              value={value.token}
              placeholder={stored ? t("page.connections.form.secretStored") : undefined}
              onChange={(event) => set("token", event.target.value)}
            />
          </Fld>
        )}
        <Fld label={t("page.connections.form.pulsar.tenant")}>
          <Input
            className="mono3"
            style={MONO}
            value={value.tenant}
            placeholder="public"
            onChange={(event) => set("tenant", event.target.value)}
          />
        </Fld>
        <Fld label={t("page.connections.form.pulsar.namespace")}>
          <Input
            className="mono3"
            style={MONO}
            value={value.namespace}
            placeholder="default"
            onChange={(event) => set("namespace", event.target.value)}
          />
        </Fld>
      </div>
      <FormNote
        advanced={
          <button
            type="button"
            className="mqs-disclosure"
            aria-expanded={advancedOpen}
            onClick={() => setAdvancedOpen((open) => !open)}
          >
            <ChevronRight size={12} aria-hidden />
            {t("page.connections.form.pulsar.advanced")}
          </button>
        }
        note={t("page.connections.form.pulsar.note")}
      />
      {advancedOpen && (
        <div style={GRID}>
          <Fld
            label={t("page.connections.form.rocketmq.timeout")}
            hint={t("page.connections.form.rocketmq.timeoutHint")}
          >
            <Input
              type="number"
              min={1}
              max={300}
              value={value.timeoutSec > 0 ? String(value.timeoutSec) : ""}
              onChange={(event) => {
                const seconds = Number.parseInt(event.target.value, 10);
                set("timeoutSec", Number.isNaN(seconds) ? 0 : seconds);
              }}
            />
          </Fld>
          <Fld label={t("page.connections.form.remark")} hint={t("page.connections.form.remarkHint")}>
            <Input value={value.remark} onChange={(event) => set("remark", event.target.value)} />
          </Fld>
          <Fld span label="TLS" hint={t("page.connections.form.pulsar.tlsHint")}>
            <div style={SWITCH_ROW}>
              <Switch
                checked={value.tls}
                onCheckedChange={(next: boolean) =>
                  // The CA file and skip-verify only mean anything with TLS on,
                  // and leaving them set would silently re-apply them.
                  onChange({
                    ...value,
                    tls: next,
                    tlsCaFile: next ? value.tlsCaFile : "",
                    tlsSkipVerify: next && value.tlsSkipVerify,
                  })
                }
              />
              <span style={{ color: "var(--c-muted)" }}>
                {t("page.connections.form.pulsar.tls")}
              </span>
            </div>
          </Fld>
          {value.tls && (
            <>
              <Fld
                span
                label={t("page.connections.form.kafka.caFile")}
                hint={t("page.connections.form.kafka.caFileHint")}
              >
                <Input
                  className="mono3"
                  style={MONO}
                  value={value.tlsCaFile}
                  placeholder="/etc/pulsar/ca.pem"
                  onChange={(event) => set("tlsCaFile", event.target.value)}
                />
              </Fld>
              <Fld
                span
                label={t("page.connections.form.kafka.skipVerify")}
                hint={t("page.connections.form.kafka.skipVerifyHint")}
              >
                <div style={SWITCH_ROW}>
                  <Switch
                    checked={value.tlsSkipVerify}
                    onCheckedChange={(next: boolean) => set("tlsSkipVerify", next)}
                  />
                  <span style={{ color: "var(--c-muted)" }}>
                    {t("page.connections.form.kafka.skipVerifyNote")}
                  </span>
                </div>
              </Fld>
            </>
          )}
        </div>
      )}
    </>
  );
}

/** Option keys the Redis Stream driver reads back off a stored profile. */
export const OPTION_REDIS_DEPLOYMENT = "deployment";
export const OPTION_REDIS_MASTER_NAME = "masterName";
export const OPTION_REDIS_DB = "db";
export const OPTION_REDIS_STREAM_FILTER = "streamFilter";
export const OPTION_REDIS_TLS = "tls";
export const OPTION_REDIS_TLS_SKIP_VERIFY = "tlsSkipVerify";

/** How a profile reaches Redis. The three are different clients, not a style. */
export type RedisDeployment = "standalone" | "sentinel" | "cluster";

/**
 * What the Redis Stream form collects.
 *
 * The deployment is asked for rather than guessed, because one host:port is a
 * server, a sentinel, or a cluster's configuration endpoint and nothing in the
 * address says which. It also decides what the rest of the form means: a
 * sentinel needs the master's name, and a cluster has one database and refuses
 * SELECT, so the index is not collected for it at all.
 *
 * The stream filter is here rather than on the list page because it is what
 * the SCAN that finds streams matches on. A production keyspace holds far more
 * than streams, and narrowing the scan once at connect time is what keeps that
 * page usable.
 */
export interface RedisDraft {
  name: string;
  deployment: RedisDeployment;
  /** One address for standalone; a seed list for sentinel and cluster. */
  endpoints: string;
  masterName: string;
  db: number;
  streamFilter: string;
  username: string;
  password: string;
  tls: boolean;
  tlsSkipVerify: boolean;
  group: string;
  remark: string;
  timeoutSec: number;
  /** A stored password never comes back, so blank means "keep it". */
  credentialsStored: boolean;
  clearCredentials: boolean;
}

export function emptyRedisDraft(): RedisDraft {
  return {
    name: "",
    deployment: "standalone",
    endpoints: "",
    masterName: "",
    db: 0,
    streamFilter: "",
    username: "",
    password: "",
    tls: false,
    tlsSkipVerify: false,
    group: "",
    remark: "",
    timeoutSec: DEFAULT_TIMEOUT_SEC,
    credentialsStored: false,
    clearCredentials: false,
  };
}

/** Board 6e — Redis Stream. The key filter decides the left-hand Stream list. */
export function RedisForm({
  value,
  onChange,
}: {
  value: RedisDraft;
  onChange: (next: RedisDraft) => void;
}) {
  const { t } = useTranslation();
  const set = <K extends keyof RedisDraft>(key: K, next: RedisDraft[K]) =>
    onChange({ ...value, [key]: next });
  const [advancedOpen, setAdvancedOpen] = useState(
    value.timeoutSec !== DEFAULT_TIMEOUT_SEC || value.remark !== "" || value.tls,
  );
  const stored = value.credentialsStored && !value.clearCredentials;
  const cluster = value.deployment === "cluster";

  return (
    <>
      <div style={GRID}>
        <Fld label={t("page.connections.form.name")}>
          <Input
            value={value.name}
            placeholder="redis-stream-01"
            onChange={(event) => set("name", event.target.value)}
          />
        </Fld>
        <Fld span label={t("page.connections.form.redis.mode")}>
          <Segmented
            style={{ alignSelf: "flex-start" }}
            value={value.deployment}
            onChange={(next: RedisDeployment) =>
              // A cluster has one database and refuses SELECT, so a stored
              // index must not survive the switch and come back the day
              // someone picks standalone again.
              onChange({
                ...value,
                deployment: next,
                db: next === "cluster" ? 0 : value.db,
                masterName: next === "sentinel" ? value.masterName : "",
              })
            }
            options={[
              { value: "standalone", label: t("page.connections.form.redis.standalone") },
              { value: "sentinel", label: t("page.connections.form.redis.sentinel") },
              { value: "cluster", label: t("page.connections.form.redis.cluster") },
            ]}
          />
        </Fld>
        <Fld
          span={value.deployment === "standalone"}
          label={t("page.connections.form.redis.address")}
          hint={
            value.deployment === "standalone"
              ? undefined
              : t("page.connections.form.redis.addressHint")
          }
        >
          <Input
            className="mono3"
            style={MONO}
            value={value.endpoints}
            placeholder={
              value.deployment === "standalone"
                ? "10.2.0.8:6379"
                : "10.2.0.8:6379, 10.2.0.9:6379"
            }
            onChange={(event) => set("endpoints", event.target.value)}
          />
        </Fld>
        {value.deployment === "sentinel" && (
          <Fld
            label={t("page.connections.form.redis.masterName")}
            hint={t("page.connections.form.redis.masterNameHint")}
          >
            <Input
              className="mono3"
              style={MONO}
              value={value.masterName}
              placeholder="mymaster"
              onChange={(event) => set("masterName", event.target.value)}
            />
          </Fld>
        )}
        {!cluster && (
          <Fld label={t("page.connections.form.redis.db")} hint={t("page.connections.form.redis.dbHint")}>
            <Input
              className="mono3"
              style={MONO}
              type="number"
              min={0}
              max={15}
              value={String(value.db)}
              onChange={(event) => {
                const index = Number.parseInt(event.target.value, 10);
                set("db", Number.isNaN(index) || index < 0 ? 0 : index);
              }}
            />
          </Fld>
        )}
        <Fld
          label={t("page.connections.form.username")}
          hint={
            stored ? (
              <button type="button" className="mqs-linkbtn" onClick={() => set("clearCredentials", true)}>
                {t("page.connections.form.clearCredentials")}
              </button>
            ) : (
              t("page.connections.form.redis.usernameHint")
            )
          }
        >
          <Input
            value={value.username}
            placeholder={stored ? t("page.connections.form.secretStored") : "default"}
            onChange={(event) => set("username", event.target.value)}
          />
        </Fld>
        <Fld label={t("page.connections.form.password")}>
          <Input
            type="password"
            value={value.password}
            placeholder={stored ? t("page.connections.form.secretStored") : undefined}
            onChange={(event) => set("password", event.target.value)}
          />
        </Fld>
        <Fld
          span
          label={t("page.connections.form.redis.streamFilter")}
          hint={t("page.connections.form.redis.streamFilterHint")}
        >
          <Input
            className="mono3"
            style={MONO}
            value={value.streamFilter}
            placeholder="orders:*"
            onChange={(event) => set("streamFilter", event.target.value)}
          />
        </Fld>
      </div>
      <FormNote
        advanced={
          <button
            type="button"
            className="mqs-disclosure"
            aria-expanded={advancedOpen}
            onClick={() => setAdvancedOpen((open) => !open)}
          >
            <ChevronRight size={12} aria-hidden />
            {t("page.connections.form.redis.advanced")}
          </button>
        }
        note={t("page.connections.form.redis.note")}
      />
      {advancedOpen && (
        <div style={GRID}>
          <Fld
            label={t("page.connections.form.rocketmq.timeout")}
            hint={t("page.connections.form.rocketmq.timeoutHint")}
          >
            <Input
              type="number"
              min={1}
              max={300}
              value={value.timeoutSec > 0 ? String(value.timeoutSec) : ""}
              onChange={(event) => {
                const seconds = Number.parseInt(event.target.value, 10);
                set("timeoutSec", Number.isNaN(seconds) ? 0 : seconds);
              }}
            />
          </Fld>
          <Fld label={t("page.connections.form.remark")} hint={t("page.connections.form.remarkHint")}>
            <Input value={value.remark} onChange={(event) => set("remark", event.target.value)} />
          </Fld>
          <Fld span label="TLS" hint={t("page.connections.form.redis.tlsHint")}>
            <div style={SWITCH_ROW}>
              <Switch
                checked={value.tls}
                onCheckedChange={(next: boolean) =>
                  // Skipping verification only means anything with TLS on, and
                  // leaving it set while TLS is off would silently re-apply it.
                  onChange({ ...value, tls: next, tlsSkipVerify: next && value.tlsSkipVerify })
                }
              />
              <span style={{ color: "var(--c-muted)" }}>
                {t("page.connections.form.redis.tls")}
              </span>
            </div>
          </Fld>
          {value.tls && (
            <Fld
              span
              label={t("page.connections.form.redis.tlsSkipVerify")}
              hint={t("page.connections.form.redis.tlsSkipVerifyHint")}
            >
              <div style={SWITCH_ROW}>
                <Switch
                  checked={value.tlsSkipVerify}
                  onCheckedChange={(next: boolean) => set("tlsSkipVerify", next)}
                />
                <span style={{ color: "var(--c-muted)" }}>
                  {t("page.connections.form.redis.tlsSkipVerifyNote")}
                </span>
              </div>
            </Fld>
          )}
        </div>
      )}
    </>
  );
}

/** Board 6f — MQTT. Clean Start and session expiry are 5.0-only. */
export type MqttProtocol = "5" | "311";
export type MqttTransport = "tcp" | "tls" | "ws" | "wss";
export type MqttMechanism = "none" | "plain";

export interface MqttDraft {
  name: string;
  /** host:port, one or more. This is the profile's endpoints field. */
  endpoints: string;
  protocol: MqttProtocol;
  transport: MqttTransport;
  wsPath: string;
  /** Empty means the driver generates one per connection. */
  clientId: string;
  keepAliveSec: number;
  cleanStart: boolean;
  sessionExpirySec: number;
  mechanism: MqttMechanism;
  username: string;
  password: string;
  tlsCaFile: string;
  tlsSkipVerify: boolean;
  /** The broker's own management API. MQTT has none of its own. */
  managementUrl: string;
  managementKey: string;
  managementSecret: string;
  group: string;
  remark: string;
  timeoutSec: number;
  /** A stored secret never comes back, so blank means "keep it". */
  credentialsStored: boolean;
  clearCredentials: boolean;
}

export function emptyMqttDraft(): MqttDraft {
  return {
    name: "",
    endpoints: "",
    protocol: "5",
    transport: "tcp",
    wsPath: "/mqtt",
    clientId: "",
    keepAliveSec: 60,
    cleanStart: true,
    sessionExpirySec: 0,
    mechanism: "none",
    username: "",
    password: "",
    tlsCaFile: "",
    tlsSkipVerify: false,
    managementUrl: "",
    managementKey: "",
    managementSecret: "",
    group: "",
    remark: "",
    timeoutSec: DEFAULT_TIMEOUT_SEC,
    credentialsStored: false,
    clearCredentials: false,
  };
}

/**
 * Board 6e - MQTT.
 *
 * Two credential blocks rather than one, as in NATS: the broker's username
 * and password authenticate the session, and the management API key
 * authenticates a completely separate HTTP endpoint that the protocol knows
 * nothing about. A connection can have either, both or neither.
 */
export function MqttForm({
  value,
  onChange,
}: {
  value: MqttDraft;
  onChange: (next: MqttDraft) => void;
}) {
  const { t } = useTranslation();
  const set = <K extends keyof MqttDraft>(key: K, next: MqttDraft[K]) =>
    onChange({ ...value, [key]: next });
  const [advancedOpen, setAdvancedOpen] = useState(
    value.timeoutSec !== DEFAULT_TIMEOUT_SEC ||
      value.remark !== "" ||
      value.clientId !== "" ||
      value.managementUrl !== "",
  );
  const authenticating = value.mechanism !== "none";
  const encrypted = value.transport === "tls" || value.transport === "wss";
  const webSocket = value.transport === "ws" || value.transport === "wss";
  const stored = value.credentialsStored && !value.clearCredentials;

  return (
    <>
      <div style={GRID}>
        <Fld label={t("page.connections.form.name")}>
          <Input
            value={value.name}
            placeholder="iot-broker"
            onChange={(event) => set("name", event.target.value)}
          />
        </Fld>
        <Fld
          label={t("page.connections.form.mqtt.version")}
          hint={t("page.connections.form.mqtt.versionHint")}
        >
          <Segmented<MqttProtocol>
            style={{ alignSelf: "flex-start" }}
            value={value.protocol}
            options={[
              { value: "311", label: "3.1.1" },
              { value: "5", label: "5.0" },
            ]}
            onChange={(next) =>
              // Session expiry is 5.0 only. Leaving a value behind would send
              // it to a 3.1.1 broker that has no field for it.
              onChange({
                ...value,
                protocol: next,
                sessionExpirySec: next === "5" ? value.sessionExpirySec : 0,
              })
            }
          />
        </Fld>
        <Fld
          span
          label={t("page.connections.form.mqtt.broker")}
          hint={t("page.connections.form.mqtt.brokerHint")}
        >
          <Input
            className="mono3"
            style={MONO}
            value={value.endpoints}
            placeholder="iot.example.com:1883"
            onChange={(event) => set("endpoints", event.target.value)}
          />
        </Fld>
        <Fld
          label={t("page.connections.form.mqtt.transport")}
          hint={t("page.connections.form.mqtt.transportHint")}
        >
          <SelectField<MqttTransport>
            value={value.transport}
            options={[
              { value: "tcp", label: "TCP" },
              { value: "tls", label: "TLS" },
              { value: "ws", label: "WebSocket" },
              { value: "wss", label: "WebSocket + TLS" },
            ]}
            onValueChange={(next) =>
              // The TLS fields only mean anything on an encrypted transport,
              // and leaving them set would silently re-apply them.
              onChange({
                ...value,
                transport: next,
                tlsCaFile: next === "tls" || next === "wss" ? value.tlsCaFile : "",
                tlsSkipVerify: (next === "tls" || next === "wss") && value.tlsSkipVerify,
              })
            }
          />
        </Fld>
        <Fld label={t("page.connections.form.mqtt.mechanism")}>
          <SelectField<MqttMechanism>
            value={value.mechanism}
            options={[
              { value: "none", label: t("page.connections.form.mqtt.mechanismNone") },
              { value: "plain", label: t("page.connections.form.mqtt.mechanismPlain") },
            ]}
            onValueChange={(next) =>
              onChange({
                ...value,
                mechanism: next,
                username: next === "none" ? "" : value.username,
                password: next === "none" ? "" : value.password,
              })
            }
          />
        </Fld>
        {webSocket && (
          <Fld
            span
            label={t("page.connections.form.mqtt.wsPath")}
            hint={t("page.connections.form.mqtt.wsPathHint")}
          >
            <Input
              className="mono3"
              style={MONO}
              value={value.wsPath}
              placeholder="/mqtt"
              onChange={(event) => set("wsPath", event.target.value)}
            />
          </Fld>
        )}
        {authenticating && (
          <>
            <Fld
              label={t("page.connections.form.username")}
              hint={
                stored ? (
                  <button
                    type="button"
                    className="mqs-linkbtn"
                    onClick={() => set("clearCredentials", true)}
                  >
                    {t("page.connections.form.clearCredentials")}
                  </button>
                ) : undefined
              }
            >
              <Input
                value={value.username}
                placeholder={stored ? t("page.connections.form.secretStored") : undefined}
                onChange={(event) => set("username", event.target.value)}
              />
            </Fld>
            <Fld label={t("page.connections.form.password")}>
              <Input
                type="password"
                value={value.password}
                placeholder={stored ? t("page.connections.form.secretStored") : undefined}
                onChange={(event) => set("password", event.target.value)}
              />
            </Fld>
          </>
        )}
        {value.protocol === "5" && (
          <Fld
            label={t("page.connections.form.mqtt.sessionExpiry")}
            hint={t("page.connections.form.mqtt.sessionExpiryHint")}
          >
            <Input
              type="number"
              min={0}
              value={String(value.sessionExpirySec)}
              onChange={(event) => {
                const seconds = Number.parseInt(event.target.value, 10);
                set("sessionExpirySec", Number.isNaN(seconds) ? 0 : seconds);
              }}
            />
          </Fld>
        )}
      </div>
      <FormNote
        advanced={
          <button
            type="button"
            className="mqs-disclosure"
            aria-expanded={advancedOpen}
            onClick={() => setAdvancedOpen((open) => !open)}
          >
            <ChevronRight size={12} aria-hidden />
            {t("page.connections.form.mqtt.advanced")}
          </button>
        }
        note={t("page.connections.form.mqtt.note")}
      />
      {advancedOpen && (
        <div style={GRID}>
          <Fld
            label={t("page.connections.form.rocketmq.timeout")}
            hint={t("page.connections.form.rocketmq.timeoutHint")}
          >
            <Input
              type="number"
              min={1}
              max={300}
              value={value.timeoutSec > 0 ? String(value.timeoutSec) : ""}
              onChange={(event) => {
                const seconds = Number.parseInt(event.target.value, 10);
                set("timeoutSec", Number.isNaN(seconds) ? 0 : seconds);
              }}
            />
          </Fld>
          <Fld label={t("page.connections.form.remark")} hint={t("page.connections.form.remarkHint")}>
            <Input value={value.remark} onChange={(event) => set("remark", event.target.value)} />
          </Fld>
          <Fld
            label={t("page.connections.form.mqtt.clientId")}
            hint={t("page.connections.form.mqtt.clientIdHint")}
          >
            <Input
              className="mono3"
              style={MONO}
              value={value.clientId}
              placeholder="mq-studio-…"
              onChange={(event) => set("clientId", event.target.value)}
            />
          </Fld>
          <Fld
            label={t("page.connections.form.mqtt.keepAlive")}
            hint={t("page.connections.form.mqtt.keepAliveHint")}
          >
            <Input
              type="number"
              min={0}
              max={65535}
              value={String(value.keepAliveSec)}
              onChange={(event) => {
                const seconds = Number.parseInt(event.target.value, 10);
                set("keepAliveSec", Number.isNaN(seconds) ? 0 : seconds);
              }}
            />
          </Fld>
          <Fld
            span
            label={t("page.connections.form.mqtt.cleanStart")}
            hint={t("page.connections.form.mqtt.cleanStartHint")}
          >
            <div style={SWITCH_ROW}>
              <Switch
                checked={value.cleanStart}
                onCheckedChange={(next: boolean) => set("cleanStart", next)}
              />
              <span style={{ color: "var(--c-muted)" }}>
                {t("page.connections.form.mqtt.cleanStartOn")}
              </span>
            </div>
          </Fld>
          {encrypted && (
            <>
              <Fld
                span
                label={t("page.connections.form.kafka.caFile")}
                hint={t("page.connections.form.kafka.caFileHint")}
              >
                <Input
                  className="mono3"
                  style={MONO}
                  value={value.tlsCaFile}
                  placeholder="/etc/mosquitto/ca.pem"
                  onChange={(event) => set("tlsCaFile", event.target.value)}
                />
              </Fld>
              <Fld
                span
                label={t("page.connections.form.kafka.skipVerify")}
                hint={t("page.connections.form.kafka.skipVerifyHint")}
              >
                <div style={SWITCH_ROW}>
                  <Switch
                    checked={value.tlsSkipVerify}
                    onCheckedChange={(next: boolean) => set("tlsSkipVerify", next)}
                  />
                  <span style={{ color: "var(--c-muted)" }}>
                    {t("page.connections.form.kafka.skipVerifyNote")}
                  </span>
                </div>
              </Fld>
            </>
          )}
          <Fld
            span
            label={t("page.connections.form.mqtt.management")}
            hint={t("page.connections.form.mqtt.managementHint")}
          >
            <Input
              className="mono3"
              style={MONO}
              value={value.managementUrl}
              placeholder="http://iot.example.com:18083"
              onChange={(event) => set("managementUrl", event.target.value)}
            />
          </Fld>
          {value.managementUrl.trim() !== "" && (
            <>
              <Fld label={t("page.connections.form.mqtt.managementKey")}>
                <Input
                  value={value.managementKey}
                  placeholder={stored ? t("page.connections.form.secretStored") : undefined}
                  onChange={(event) => set("managementKey", event.target.value)}
                />
              </Fld>
              <Fld label={t("page.connections.form.mqtt.managementSecret")}>
                <Input
                  type="password"
                  value={value.managementSecret}
                  placeholder={stored ? t("page.connections.form.secretStored") : undefined}
                  onChange={(event) => set("managementSecret", event.target.value)}
                />
              </Fld>
            </>
          )}
        </div>
      )}
    </>
  );
}

/** Option keys the NATS driver reads back off a stored profile. */
export const OPTION_NATS_TLS = "tls";
export const OPTION_NATS_TLS_CA_FILE = "tlsCaFile";
export const OPTION_NATS_TLS_CERT_FILE = "tlsCertFile";
export const OPTION_NATS_TLS_KEY_FILE = "tlsKeyFile";
export const OPTION_NATS_TLS_SKIP_VERIFY = "tlsSkipVerify";
export const OPTION_NATS_MONITOR_URL = "monitorUrl";
export const OPTION_NATS_JS_DOMAIN = "jsDomain";
export const OPTION_NATS_CREDS_FILE = "credsFile";

/**
 * Five mechanisms, and none of them is a preference.
 *
 * A NATS server is configured for one and refuses the others, so this is a
 * statement about the server rather than a choice about how careful to be. An
 * nkey signs a nonce the server issues; a creds file adds the JWT claims that
 * say what the bearer may do; a token is a shared string the server compares.
 */
export type NatsMechanism = "none" | "plain" | "token" | "nkey" | "creds" | "mtls";

export interface NatsDraft {
  name: string;
  /** nats://host:port, one or more. This is the profile's endpoints field. */
  endpoints: string;
  mechanism: NatsMechanism;
  username: string;
  password: string;
  token: string;
  /** The seed itself, not a path: it is the whole credential. */
  nkeySeed: string;
  /** A path, because a creds file carries a JWT the library reads too. */
  credsFile: string;
  tls: boolean;
  tlsCaFile: string;
  tlsCertFile: string;
  tlsKeyFile: string;
  tlsSkipVerify: boolean;
  /** The server's HTTP monitoring port. NATS has nothing like it in-protocol. */
  monitorUrl: string;
  /** The system account is a second account, so a second credential. */
  systemUser: string;
  systemPassword: string;
  jsDomain: string;
  group: string;
  remark: string;
  timeoutSec: number;
  /** A stored secret never comes back, so blank means "keep it". */
  credentialsStored: boolean;
  clearCredentials: boolean;
}

export function emptyNatsDraft(): NatsDraft {
  return {
    name: "",
    endpoints: "",
    mechanism: "none",
    username: "",
    password: "",
    token: "",
    nkeySeed: "",
    credsFile: "",
    tls: false,
    tlsCaFile: "",
    tlsCertFile: "",
    tlsKeyFile: "",
    tlsSkipVerify: false,
    monitorUrl: "",
    systemUser: "",
    systemPassword: "",
    jsDomain: "",
    group: "",
    remark: "",
    timeoutSec: DEFAULT_TIMEOUT_SEC,
    credentialsStored: false,
    clearCredentials: false,
  };
}

/**
 * NATS.
 *
 * Two credential blocks, like MQTT and for a related reason - but here they
 * are not a protocol and an API bolted beside it. The system account is an
 * ordinary NATS account that happens to receive the server's own events, and
 * an account is an isolation boundary: the credentials on the left cannot
 * reach $SYS however many permissions they carry. Without the second pair the
 * cluster can only be asked about the one server the monitoring address names.
 *
 * Both are optional, and a connection with neither still works - it just sees
 * one server instead of the cluster.
 */
export function NatsForm({
  value,
  onChange,
}: {
  value: NatsDraft;
  onChange: (next: NatsDraft) => void;
}) {
  const { t } = useTranslation();
  const set = <K extends keyof NatsDraft>(key: K, next: NatsDraft[K]) =>
    onChange({ ...value, [key]: next });
  const [advancedOpen, setAdvancedOpen] = useState(
    value.timeoutSec !== DEFAULT_TIMEOUT_SEC ||
      value.remark !== "" ||
      value.monitorUrl !== "" ||
      value.jsDomain !== "" ||
      value.systemUser !== "",
  );
  const stored = value.credentialsStored && !value.clearCredentials;
  const encrypted = value.tls || value.mechanism === "mtls";

  return (
    <>
      <div style={GRID}>
        <Fld label={t("page.connections.form.name")}>
          <Input
            value={value.name}
            placeholder="nats-prod"
            onChange={(event) => set("name", event.target.value)}
          />
        </Fld>
        <Fld label={t("page.connections.form.nats.mechanism")}>
          <SelectField<NatsMechanism>
            value={value.mechanism}
            options={[
              { value: "none", label: t("page.connections.form.nats.mechanismNone") },
              { value: "plain", label: t("page.connections.form.nats.mechanismPlain") },
              { value: "token", label: t("page.connections.form.nats.mechanismToken") },
              { value: "nkey", label: t("page.connections.form.nats.mechanismNkey") },
              { value: "creds", label: t("page.connections.form.nats.mechanismCreds") },
              { value: "mtls", label: t("page.connections.form.nats.mechanismMtls") },
            ]}
            onValueChange={(next) =>
              // Each mechanism reads one credential and the driver ignores the
              // rest, so leaving the others filled in would store secrets
              // nothing will ever send.
              onChange({
                ...value,
                mechanism: next,
                username: next === "plain" ? value.username : "",
                password: next === "plain" ? value.password : "",
                token: next === "token" ? value.token : "",
                nkeySeed: next === "nkey" ? value.nkeySeed : "",
                credsFile: next === "creds" ? value.credsFile : "",
                tlsCertFile: next === "mtls" ? value.tlsCertFile : "",
                tlsKeyFile: next === "mtls" ? value.tlsKeyFile : "",
              })
            }
          />
        </Fld>
        <Fld
          span
          label={t("page.connections.form.nats.servers")}
          hint={t("page.connections.form.nats.serversHint")}
        >
          <Input
            className="mono3"
            style={MONO}
            value={value.endpoints}
            placeholder="nats://nats.example.com:4222"
            onChange={(event) => set("endpoints", event.target.value)}
          />
        </Fld>
        {value.mechanism === "plain" && (
          <>
            <Fld
              label={t("page.connections.form.username")}
              hint={
                stored ? (
                  <button
                    type="button"
                    className="mqs-linkbtn"
                    onClick={() => set("clearCredentials", true)}
                  >
                    {t("page.connections.form.clearCredentials")}
                  </button>
                ) : undefined
              }
            >
              <Input
                value={value.username}
                placeholder={stored ? t("page.connections.form.secretStored") : undefined}
                onChange={(event) => set("username", event.target.value)}
              />
            </Fld>
            <Fld label={t("page.connections.form.password")}>
              <Input
                type="password"
                value={value.password}
                placeholder={stored ? t("page.connections.form.secretStored") : undefined}
                onChange={(event) => set("password", event.target.value)}
              />
            </Fld>
          </>
        )}
        {value.mechanism === "token" && (
          <Fld
            span
            label={t("page.connections.form.nats.token")}
            hint={
              stored ? (
                <button
                  type="button"
                  className="mqs-linkbtn"
                  onClick={() => set("clearCredentials", true)}
                >
                  {t("page.connections.form.clearCredentials")}
                </button>
              ) : undefined
            }
          >
            <Input
              type="password"
              value={value.token}
              placeholder={stored ? t("page.connections.form.secretStored") : undefined}
              onChange={(event) => set("token", event.target.value)}
            />
          </Fld>
        )}
        {value.mechanism === "nkey" && (
          <Fld
            span
            label={t("page.connections.form.nats.nkeySeed")}
            hint={t("page.connections.form.nats.nkeySeedHint")}
          >
            <Input
              type="password"
              className="mono3"
              style={MONO}
              value={value.nkeySeed}
              placeholder={stored ? t("page.connections.form.secretStored") : "SUA…"}
              onChange={(event) => set("nkeySeed", event.target.value)}
            />
          </Fld>
        )}
        {value.mechanism === "creds" && (
          <Fld
            span
            label={t("page.connections.form.nats.credsFile")}
            hint={t("page.connections.form.nats.credsFileHint")}
          >
            <Input
              className="mono3"
              style={MONO}
              value={value.credsFile}
              placeholder="~/.nkeys/creds/op/acct/user.creds"
              onChange={(event) => set("credsFile", event.target.value)}
            />
          </Fld>
        )}
        <Fld
          span
          label={t("page.connections.form.nats.tls")}
          hint={t("page.connections.form.nats.tlsHint")}
        >
          <div style={SWITCH_ROW}>
            <Switch
              checked={value.tls}
              onCheckedChange={(next: boolean) =>
                // The TLS files only mean anything on an encrypted transport,
                // and leaving them set would silently re-apply them.
                onChange({
                  ...value,
                  tls: next,
                  tlsCaFile: next ? value.tlsCaFile : "",
                  tlsSkipVerify: next && value.tlsSkipVerify,
                })
              }
            />
            <span style={{ color: "var(--c-muted)" }}>
              {t("page.connections.form.nats.tlsNote")}
            </span>
          </div>
        </Fld>
      </div>
      <FormNote
        advanced={
          <button
            type="button"
            className="mqs-disclosure"
            aria-expanded={advancedOpen}
            onClick={() => setAdvancedOpen((open) => !open)}
          >
            <ChevronRight size={12} aria-hidden />
            {t("page.connections.form.nats.advanced")}
          </button>
        }
        note={t("page.connections.form.nats.note")}
      />
      {advancedOpen && (
        <div style={GRID}>
          <Fld
            label={t("page.connections.form.rocketmq.timeout")}
            hint={t("page.connections.form.rocketmq.timeoutHint")}
          >
            <Input
              type="number"
              min={1}
              max={300}
              value={value.timeoutSec > 0 ? String(value.timeoutSec) : ""}
              onChange={(event) => {
                const seconds = Number.parseInt(event.target.value, 10);
                set("timeoutSec", Number.isNaN(seconds) ? 0 : seconds);
              }}
            />
          </Fld>
          <Fld label={t("page.connections.form.remark")} hint={t("page.connections.form.remarkHint")}>
            <Input value={value.remark} onChange={(event) => set("remark", event.target.value)} />
          </Fld>
          <Fld
            span
            label={t("page.connections.form.nats.monitor")}
            hint={t("page.connections.form.nats.monitorHint")}
          >
            <Input
              className="mono3"
              style={MONO}
              value={value.monitorUrl}
              placeholder="http://nats.example.com:8222"
              onChange={(event) => set("monitorUrl", event.target.value)}
            />
          </Fld>
          <Fld
            label={t("page.connections.form.nats.systemUser")}
            hint={t("page.connections.form.nats.systemUserHint")}
          >
            <Input
              value={value.systemUser}
              placeholder={stored ? t("page.connections.form.secretStored") : undefined}
              onChange={(event) => set("systemUser", event.target.value)}
            />
          </Fld>
          <Fld label={t("page.connections.form.nats.systemPassword")}>
            <Input
              type="password"
              value={value.systemPassword}
              placeholder={stored ? t("page.connections.form.secretStored") : undefined}
              onChange={(event) => set("systemPassword", event.target.value)}
            />
          </Fld>
          <Fld
            span
            label={t("page.connections.form.nats.jsDomain")}
            hint={t("page.connections.form.nats.jsDomainHint")}
          >
            <Input
              className="mono3"
              style={MONO}
              value={value.jsDomain}
              onChange={(event) => set("jsDomain", event.target.value)}
            />
          </Fld>
          {encrypted && (
            <>
              <Fld
                span
                label={t("page.connections.form.kafka.caFile")}
                hint={t("page.connections.form.kafka.caFileHint")}
              >
                <Input
                  className="mono3"
                  style={MONO}
                  value={value.tlsCaFile}
                  placeholder="/etc/nats/ca.pem"
                  onChange={(event) => set("tlsCaFile", event.target.value)}
                />
              </Fld>
              <Fld
                span
                label={t("page.connections.form.kafka.skipVerify")}
                hint={t("page.connections.form.kafka.skipVerifyHint")}
              >
                <div style={SWITCH_ROW}>
                  <Switch
                    checked={value.tlsSkipVerify}
                    onCheckedChange={(next: boolean) => set("tlsSkipVerify", next)}
                  />
                  <span style={{ color: "var(--c-muted)" }}>
                    {t("page.connections.form.kafka.skipVerifyNote")}
                  </span>
                </div>
              </Fld>
            </>
          )}
          {value.mechanism === "mtls" && (
            <>
              <Fld
                span
                label={t("page.connections.form.nats.certFile")}
                hint={t("page.connections.form.nats.certFileHint")}
              >
                <Input
                  className="mono3"
                  style={MONO}
                  value={value.tlsCertFile}
                  placeholder="/etc/nats/client-cert.pem"
                  onChange={(event) => set("tlsCertFile", event.target.value)}
                />
              </Fld>
              <Fld span label={t("page.connections.form.nats.keyFile")}>
                <Input
                  className="mono3"
                  style={MONO}
                  value={value.tlsKeyFile}
                  placeholder="/etc/nats/client-key.pem"
                  onChange={(event) => set("tlsKeyFile", event.target.value)}
                />
              </Fld>
            </>
          )}
        </div>
      )}
    </>
  );
}

/** Option keys the ActiveMQ driver reads back off a stored profile. */
export const OPTION_ACTIVEMQ_JOLOKIA_PATH = "jolokiaPath";
export const OPTION_ACTIVEMQ_BROKER_NAME = "brokerName";
export const OPTION_ACTIVEMQ_ORIGIN = "originHeader";
export const OPTION_ACTIVEMQ_AMQP_URL = "amqpUrl";
export const OPTION_ACTIVEMQ_TLS_SKIP_VERIFY = "tlsSkipVerify";

/**
 * Two mechanisms, because the console has two.
 *
 * The broker's own authentication is a JAAS realm configured in XML and has no
 * bearing on reaching Jolokia - which is the whole management plane here, so
 * these credentials are the console's rather than the broker's.
 */
export type ActiveMQMechanism = "none" | "plain";

export interface ActiveMQDraft {
  name: string;
  /** The web console's URL. For this family the endpoint is HTTP. */
  endpoints: string;
  mechanism: ActiveMQMechanism;
  username: string;
  password: string;
  /** Left empty the driver probes both agent paths and keeps what answered. */
  jolokiaPath: string;
  /** The name inside every ObjectName; read off a search when left empty. */
  brokerName: string;
  /** Not decoration - see the hint. A blank field still sends localhost. */
  originHeader: string;
  tlsSkipVerify: boolean;
  /** Optional. Without it everything works except live tail and binary sends. */
  amqpUrl: string;
  amqpUsername: string;
  amqpPassword: string;
  group: string;
  remark: string;
  timeoutSec: number;
  /** A stored secret never comes back, so blank means "keep it". */
  credentialsStored: boolean;
  clearCredentials: boolean;
}

export function emptyActiveMQDraft(): ActiveMQDraft {
  return {
    name: "",
    endpoints: "",
    mechanism: "plain",
    username: "",
    password: "",
    jolokiaPath: "",
    brokerName: "",
    originHeader: "",
    tlsSkipVerify: false,
    amqpUrl: "",
    amqpUsername: "",
    amqpPassword: "",
    group: "",
    remark: "",
    timeoutSec: DEFAULT_TIMEOUT_SEC,
    credentialsStored: false,
    clearCredentials: false,
  };
}

/**
 * ActiveMQ, both products.
 *
 * The address is a console URL rather than a broker port, which is unlike
 * every other family here and is not a simplification: Jolokia, the JMX bridge
 * under the web console, is the management plane and the data plane both.
 * Browsing and sending are JMX operations on Classic and on Artemis alike.
 *
 * Which product is behind the console is not asked. The driver finds out by
 * searching for each one's MBean domain, because a user should not have to
 * know - and a user who picked wrong would get a connection that opened and
 * then failed on every page.
 *
 * Two credential blocks, as MQTT and NATS have. The AMQP pair is optional
 * twice over: the tier itself is optional, and left blank the console's
 * credentials are reused, which is what most deployments want.
 */
export function ActiveMQForm({
  value,
  onChange,
}: {
  value: ActiveMQDraft;
  onChange: (next: ActiveMQDraft) => void;
}) {
  const { t } = useTranslation();
  const set = <K extends keyof ActiveMQDraft>(key: K, next: ActiveMQDraft[K]) =>
    onChange({ ...value, [key]: next });
  const [advancedOpen, setAdvancedOpen] = useState(
    value.timeoutSec !== DEFAULT_TIMEOUT_SEC ||
      value.remark !== "" ||
      value.jolokiaPath !== "" ||
      value.brokerName !== "" ||
      value.originHeader !== "" ||
      value.tlsSkipVerify,
  );
  const stored = value.credentialsStored && !value.clearCredentials;

  return (
    <>
      <div style={GRID}>
        <Fld label={t("page.connections.form.name")}>
          <Input
            value={value.name}
            placeholder="artemis-prod"
            onChange={(event) => set("name", event.target.value)}
          />
        </Fld>
        <Fld label={t("page.connections.form.activemq.mechanism")}>
          <SelectField<ActiveMQMechanism>
            value={value.mechanism}
            options={[
              { value: "plain", label: t("page.connections.form.activemq.mechanismPlain") },
              { value: "none", label: t("page.connections.form.activemq.mechanismNone") },
            ]}
            onValueChange={(next) =>
              onChange({
                ...value,
                mechanism: next,
                username: next === "plain" ? value.username : "",
                password: next === "plain" ? value.password : "",
              })
            }
          />
        </Fld>
        <Fld
          span
          label={t("page.connections.form.activemq.console")}
          hint={t("page.connections.form.activemq.consoleHint")}
        >
          <Input
            className="mono3"
            style={MONO}
            value={value.endpoints}
            placeholder="http://activemq.example.com:8161"
            onChange={(event) => set("endpoints", event.target.value)}
          />
        </Fld>
        {value.mechanism === "plain" && (
          <>
            <Fld
              label={t("page.connections.form.username")}
              hint={
                stored ? (
                  <button
                    type="button"
                    className="mqs-linkbtn"
                    onClick={() => set("clearCredentials", true)}
                  >
                    {t("page.connections.form.clearCredentials")}
                  </button>
                ) : undefined
              }
            >
              <Input
                value={value.username}
                placeholder={stored ? t("page.connections.form.secretStored") : undefined}
                onChange={(event) => set("username", event.target.value)}
              />
            </Fld>
            <Fld label={t("page.connections.form.password")}>
              <Input
                type="password"
                value={value.password}
                placeholder={stored ? t("page.connections.form.secretStored") : undefined}
                onChange={(event) => set("password", event.target.value)}
              />
            </Fld>
          </>
        )}
        <Fld
          span
          label={t("page.connections.form.activemq.amqpUrl")}
          hint={t("page.connections.form.activemq.amqpUrlHint")}
        >
          <Input
            className="mono3"
            style={MONO}
            value={value.amqpUrl}
            placeholder="amqp://activemq.example.com:61616"
            onChange={(event) => set("amqpUrl", event.target.value)}
          />
        </Fld>
        {value.amqpUrl.trim() !== "" && (
          <>
            <Fld
              label={t("page.connections.form.activemq.amqpUsername")}
              hint={t("page.connections.form.activemq.amqpUsernameHint")}
            >
              <Input
                value={value.amqpUsername}
                placeholder={stored ? t("page.connections.form.secretStored") : undefined}
                onChange={(event) => set("amqpUsername", event.target.value)}
              />
            </Fld>
            <Fld label={t("page.connections.form.activemq.amqpPassword")}>
              <Input
                type="password"
                value={value.amqpPassword}
                placeholder={stored ? t("page.connections.form.secretStored") : undefined}
                onChange={(event) => set("amqpPassword", event.target.value)}
              />
            </Fld>
          </>
        )}
      </div>

      <FormNote
        advanced={
          <button
            type="button"
            className="mqs-disclosure"
            aria-expanded={advancedOpen}
            onClick={() => setAdvancedOpen((open) => !open)}
          >
            <ChevronRight size={12} aria-hidden />
            {t("page.connections.form.rocketmq.advanced")}
          </button>
        }
        note={t("page.connections.form.activemq.note")}
      />
      {advancedOpen && (
        <div style={GRID}>
          <Fld
            label={t("page.connections.form.rocketmq.timeout")}
            hint={t("page.connections.form.rocketmq.timeoutHint")}
          >
            <Input
              type="number"
              value={value.timeoutSec > 0 ? String(value.timeoutSec) : ""}
              onChange={(event) => {
                const seconds = Number.parseInt(event.target.value, 10);
                set("timeoutSec", Number.isNaN(seconds) ? 0 : seconds);
              }}
            />
          </Fld>
          <Fld
            label={t("page.connections.form.remark")}
            hint={t("page.connections.form.remarkHint")}
          >
            <Input value={value.remark} onChange={(event) => set("remark", event.target.value)} />
          </Fld>
          <Fld
            span
            label={t("page.connections.form.activemq.jolokiaPath")}
            hint={t("page.connections.form.activemq.jolokiaPathHint")}
          >
            <Input
              className="mono3"
              style={MONO}
              value={value.jolokiaPath}
              placeholder="/api/jolokia"
              onChange={(event) => set("jolokiaPath", event.target.value)}
            />
          </Fld>
          <Fld
            span
            label={t("page.connections.form.activemq.brokerName")}
            hint={t("page.connections.form.activemq.brokerNameHint")}
          >
            <Input
              className="mono3"
              style={MONO}
              value={value.brokerName}
              placeholder="localhost"
              onChange={(event) => set("brokerName", event.target.value)}
            />
          </Fld>
          <Fld
            span
            label={t("page.connections.form.activemq.origin")}
            hint={t("page.connections.form.activemq.originHint")}
          >
            <Input
              className="mono3"
              style={MONO}
              value={value.originHeader}
              placeholder="http://localhost"
              onChange={(event) => set("originHeader", event.target.value)}
            />
          </Fld>
          <Fld
            span
            label={t("page.connections.form.kafka.skipVerify")}
            hint={t("page.connections.form.kafka.skipVerifyHint")}
          >
            <div style={SWITCH_ROW}>
              <Switch
                checked={value.tlsSkipVerify}
                onCheckedChange={(next: boolean) => set("tlsSkipVerify", next)}
              />
              <span style={{ color: "var(--c-muted)" }}>
                {t("page.connections.form.kafka.skipVerifyNote")}
              </span>
            </div>
          </Fld>
        </div>
      )}
    </>
  );
}

/** Option keys the NSQ driver reads back off a stored profile. */
export const OPTION_NSQ_LOOKUPD = "lookupdEndpoints";
export const OPTION_NSQ_TLS_SKIP_VERIFY = "tlsSkipVerify";

/**
 * The one form here with no credential row, and it is not an omission.
 *
 * nsqd's HTTP API authenticates nobody. Its --auth-http-address delegates
 * authorisation for clients arriving over the TCP protocol to a service
 * outside NSQ and never touches these endpoints, so a username field would be
 * a control that authenticates nothing.
 */
export interface NsqDraft {
  name: string;
  /** Every nsqd's HTTP address. A cluster is the set, not one of them. */
  endpoints: string;
  /** Optional. Without it everything works except the directory board. */
  lookupdEndpoints: string;
  tlsSkipVerify: boolean;
  group: string;
  remark: string;
  timeoutSec: number;
}

export function emptyNsqDraft(): NsqDraft {
  return {
    name: "",
    endpoints: "",
    lookupdEndpoints: "",
    tlsSkipVerify: false,
    group: "",
    remark: "",
    timeoutSec: DEFAULT_TIMEOUT_SEC,
  };
}

/**
 * NSQ, addressed as a set of nsqd.
 *
 * The address field is a genuine list, unlike every other family's here. A
 * topic exists on each nsqd that was asked to carry it and each daemon answers
 * only for itself, so a depth on the topics board is a sum across the
 * addresses in this field - and one left out is a figure that is quietly
 * short rather than an error.
 *
 * nsqlookupd is asked for separately and is optional twice over: a single-node
 * NSQ has none, and a cluster that has one still works here without it. It is
 * the discovery tier, so it knows which nsqd holds what and holds nothing
 * itself; leaving it blank costs the directory board and nothing else.
 */
export function NsqForm({
  value,
  onChange,
}: {
  value: NsqDraft;
  onChange: (next: NsqDraft) => void;
}) {
  const { t } = useTranslation();
  const set = <K extends keyof NsqDraft>(key: K, next: NsqDraft[K]) =>
    onChange({ ...value, [key]: next });
  const [advancedOpen, setAdvancedOpen] = useState(
    value.timeoutSec !== DEFAULT_TIMEOUT_SEC || value.remark !== "" || value.tlsSkipVerify,
  );

  return (
    <>
      <div style={GRID}>
        <Fld span label={t("page.connections.form.name")}>
          <Input
            value={value.name}
            placeholder="nsq-prod"
            onChange={(event) => set("name", event.target.value)}
          />
        </Fld>
        <Fld
          span
          label={t("page.connections.form.nsq.nsqd")}
          hint={t("page.connections.form.nsq.nsqdHint")}
        >
          <Input
            className="mono3"
            style={MONO}
            value={value.endpoints}
            placeholder="http://nsqd-1:4151, http://nsqd-2:4151"
            onChange={(event) => set("endpoints", event.target.value)}
          />
        </Fld>
        <Fld
          span
          label={t("page.connections.form.nsq.lookupd")}
          hint={t("page.connections.form.nsq.lookupdHint")}
        >
          <Input
            className="mono3"
            style={MONO}
            value={value.lookupdEndpoints}
            placeholder="http://nsqlookupd:4161"
            onChange={(event) => set("lookupdEndpoints", event.target.value)}
          />
        </Fld>
      </div>

      <FormNote
        advanced={
          <button
            type="button"
            className="mqs-disclosure"
            aria-expanded={advancedOpen}
            onClick={() => setAdvancedOpen((open) => !open)}
          >
            <ChevronRight size={12} aria-hidden />
            {t("page.connections.form.rocketmq.advanced")}
          </button>
        }
        note={t("page.connections.form.nsq.note")}
      />
      {advancedOpen && (
        <div style={GRID}>
          <Fld
            label={t("page.connections.form.rocketmq.timeout")}
            hint={t("page.connections.form.rocketmq.timeoutHint")}
          >
            <Input
              type="number"
              value={value.timeoutSec > 0 ? String(value.timeoutSec) : ""}
              onChange={(event) => {
                const seconds = Number.parseInt(event.target.value, 10);
                set("timeoutSec", Number.isNaN(seconds) ? 0 : seconds);
              }}
            />
          </Fld>
          <Fld
            label={t("page.connections.form.remark")}
            hint={t("page.connections.form.remarkHint")}
          >
            <Input value={value.remark} onChange={(event) => set("remark", event.target.value)} />
          </Fld>
          <Fld
            span
            label={t("page.connections.form.kafka.skipVerify")}
            hint={t("page.connections.form.kafka.skipVerifyHint")}
          >
            <div style={SWITCH_ROW}>
              <Switch
                checked={value.tlsSkipVerify}
                onCheckedChange={(next: boolean) => set("tlsSkipVerify", next)}
              />
              <span style={{ color: "var(--c-muted)" }}>
                {t("page.connections.form.kafka.skipVerifyNote")}
              </span>
            </div>
          </Fld>
        </div>
      )}
    </>
  );
}

/** Option keys the SQS driver reads back off a stored profile. */
export const OPTION_SQS_REGION = "region";
export const OPTION_SQS_QUEUE_PREFIX = "queuePrefix";
export const OPTION_SQS_ENDPOINT_URL = "endpointUrl";

/**
 * The first form here with no address field, and it is not an omission.
 *
 * There is no broker to dial. A queue is reached by naming a region and
 * signing a request with an AWS credential, and the SDK resolves
 * sqs.<region>.amazonaws.com for itself - so the region is what every other
 * family's endpoint row is, and the connection row shows it in the address
 * column for the same reason.
 *
 * The credential pair is optional, which is the one thing about this form
 * worth reading twice. Left blank the driver uses the machine's own AWS
 * identity - environment variables, the shared config file, an instance or
 * container role - which is how this app is expected to run on anything
 * already inside AWS. Half a pair is refused, because falling back to that
 * identity when someone typed one key and not the other would connect as
 * whoever the machine is rather than as the account they meant.
 */
export interface SqsDraft {
  name: string;
  /** Required. What an endpoint field is on every other family. */
  region: string;
  accessKeyId: string;
  secretAccessKey: string;
  /** Only temporary credentials carry one, and it expires with the session. */
  sessionToken: string;
  /** Narrows every listing. An account's queues are not one team's. */
  queuePrefix: string;
  /** A VPC interface endpoint, or an emulator. Still signed for the region. */
  endpointUrl: string;
  group: string;
  remark: string;
  timeoutSec: number;
  credentialsStored: boolean;
  clearCredentials: boolean;
}

export function emptySqsDraft(): SqsDraft {
  return {
    name: "",
    region: "",
    accessKeyId: "",
    secretAccessKey: "",
    sessionToken: "",
    queuePrefix: "",
    endpointUrl: "",
    group: "",
    remark: "",
    timeoutSec: DEFAULT_TIMEOUT_SEC,
    credentialsStored: false,
    clearCredentials: false,
  };
}

/** Amazon SQS, addressed by a region rather than by an address. */
export function SqsForm({
  value,
  onChange,
}: {
  value: SqsDraft;
  onChange: (next: SqsDraft) => void;
}) {
  const { t } = useTranslation();
  const set = <K extends keyof SqsDraft>(key: K, next: SqsDraft[K]) =>
    onChange({ ...value, [key]: next });
  const [advancedOpen, setAdvancedOpen] = useState(
    value.timeoutSec !== DEFAULT_TIMEOUT_SEC ||
      value.remark !== "" ||
      value.queuePrefix !== "" ||
      value.endpointUrl !== "",
  );
  const stored = value.credentialsStored && !value.clearCredentials;

  return (
    <>
      <div style={GRID}>
        <Fld label={t("page.connections.form.name")}>
          <Input
            value={value.name}
            placeholder="sqs-prod"
            onChange={(event) => set("name", event.target.value)}
          />
        </Fld>
        <Fld
          label={t("page.connections.form.sqs.region")}
          hint={t("page.connections.form.sqs.regionHint")}
        >
          <Input
            className="mono3"
            style={MONO}
            value={value.region}
            placeholder="eu-west-1"
            onChange={(event) => set("region", event.target.value)}
          />
        </Fld>
        <Fld
          label={t("page.connections.form.sqs.accessKeyId")}
          hint={
            stored ? (
              <button
                type="button"
                className="mqs-linkbtn"
                onClick={() => set("clearCredentials", true)}
              >
                {t("page.connections.form.clearCredentials")}
              </button>
            ) : (
              t("page.connections.form.sqs.credentialsHint")
            )
          }
        >
          <Input
            className="mono3"
            style={MONO}
            value={value.accessKeyId}
            placeholder={stored ? t("page.connections.form.secretStored") : "AKIA..."}
            onChange={(event) => set("accessKeyId", event.target.value)}
          />
        </Fld>
        <Fld label={t("page.connections.form.sqs.secretAccessKey")}>
          <Input
            type="password"
            value={value.secretAccessKey}
            placeholder={stored ? t("page.connections.form.secretStored") : undefined}
            onChange={(event) => set("secretAccessKey", event.target.value)}
          />
        </Fld>
        <Fld
          span
          label={t("page.connections.form.sqs.sessionToken")}
          hint={t("page.connections.form.sqs.sessionTokenHint")}
        >
          <Input
            type="password"
            value={value.sessionToken}
            placeholder={stored ? t("page.connections.form.secretStored") : undefined}
            onChange={(event) => set("sessionToken", event.target.value)}
          />
        </Fld>
      </div>

      <FormNote
        advanced={
          <button
            type="button"
            className="mqs-disclosure"
            aria-expanded={advancedOpen}
            onClick={() => setAdvancedOpen((open) => !open)}
          >
            <ChevronRight size={12} aria-hidden />
            {t("page.connections.form.rocketmq.advanced")}
          </button>
        }
        note={t("page.connections.form.sqs.note")}
      />
      {advancedOpen && (
        <div style={GRID}>
          <Fld
            label={t("page.connections.form.sqs.queuePrefix")}
            hint={t("page.connections.form.sqs.queuePrefixHint")}
          >
            <Input
              className="mono3"
              style={MONO}
              value={value.queuePrefix}
              placeholder="team-orders-"
              onChange={(event) => set("queuePrefix", event.target.value)}
            />
          </Fld>
          <Fld
            label={t("page.connections.form.rocketmq.timeout")}
            hint={t("page.connections.form.rocketmq.timeoutHint")}
          >
            <Input
              type="number"
              value={value.timeoutSec > 0 ? String(value.timeoutSec) : ""}
              onChange={(event) => {
                const seconds = Number.parseInt(event.target.value, 10);
                set("timeoutSec", Number.isNaN(seconds) ? 0 : seconds);
              }}
            />
          </Fld>
          <Fld
            span
            label={t("page.connections.form.sqs.endpointUrl")}
            hint={t("page.connections.form.sqs.endpointUrlHint")}
          >
            <Input
              className="mono3"
              style={MONO}
              value={value.endpointUrl}
              placeholder="https://vpce-0abc.sqs.eu-west-1.vpce.amazonaws.com"
              onChange={(event) => set("endpointUrl", event.target.value)}
            />
          </Fld>
          <Fld
            span
            label={t("page.connections.form.remark")}
            hint={t("page.connections.form.remarkHint")}
          >
            <Input value={value.remark} onChange={(event) => set("remark", event.target.value)} />
          </Fld>
        </div>
      )}
    </>
  );
}

/** Option keys the Google Pub/Sub driver reads back off a stored profile. */
export const OPTION_PUBSUB_PROJECT_ID = "projectId";
export const OPTION_PUBSUB_EMULATOR_HOST = "emulatorHost";
export const OPTION_PUBSUB_RESOURCE_PREFIX = "resourcePrefix";

/**
 * The second form here with no address field, and the first whose credential
 * is a document rather than a string.
 *
 * There is no broker to dial. A topic is reached by naming a project and
 * authenticating, and the client resolves pubsub.googleapis.com for every
 * project there is - so the project is what every other family's endpoint row
 * is, and the connection row shows it in the address column for the same
 * reason.
 *
 * The credential is a service account key pasted whole, which is why it is a
 * textarea and not an input. Taking a path instead would have been the smaller
 * field and the worse one: it points at a file this app does not own, so the
 * key stays in plain text where it was downloaded and the profile breaks the
 * moment it moves. Left blank the driver uses Application Default Credentials
 * - a gcloud login, GOOGLE_APPLICATION_CREDENTIALS, or the metadata server on
 * a workload already inside Google Cloud - which is how this app is expected
 * to run there.
 *
 * The emulator host is named for the emulator rather than as a general
 * endpoint override, because that is the only thing it can be: the real
 * service has one address for every project. A connection that names a host is
 * by definition not talking to it, and the driver reads exactly that to report
 * what the emulator cannot do.
 */
export interface GooglePubSubDraft {
  name: string;
  /** Required. What an endpoint field is on every other family. */
  projectId: string;
  /** The whole service account key. Blank uses the machine's own identity. */
  credentialsJson: string;
  /** Narrows every listing. A project's topics are not one team's. */
  resourcePrefix: string;
  /** host:port of a Pub/Sub emulator. Blank is the real service. */
  emulatorHost: string;
  group: string;
  remark: string;
  timeoutSec: number;
  credentialsStored: boolean;
  clearCredentials: boolean;
}

export function emptyGooglePubSubDraft(): GooglePubSubDraft {
  return {
    name: "",
    projectId: "",
    credentialsJson: "",
    resourcePrefix: "",
    emulatorHost: "",
    group: "",
    remark: "",
    timeoutSec: DEFAULT_TIMEOUT_SEC,
    credentialsStored: false,
    clearCredentials: false,
  };
}

/** Google Cloud Pub/Sub, addressed by a project rather than by an address. */
export function GooglePubSubForm({
  value,
  onChange,
}: {
  value: GooglePubSubDraft;
  onChange: (next: GooglePubSubDraft) => void;
}) {
  const { t } = useTranslation();
  const set = <K extends keyof GooglePubSubDraft>(key: K, next: GooglePubSubDraft[K]) =>
    onChange({ ...value, [key]: next });
  const [advancedOpen, setAdvancedOpen] = useState(
    value.timeoutSec !== DEFAULT_TIMEOUT_SEC ||
      value.remark !== "" ||
      value.resourcePrefix !== "" ||
      value.emulatorHost !== "",
  );
  const stored = value.credentialsStored && !value.clearCredentials;

  return (
    <>
      <div style={GRID}>
        <Fld label={t("page.connections.form.name")}>
          <Input
            value={value.name}
            placeholder="pubsub-prod"
            onChange={(event) => set("name", event.target.value)}
          />
        </Fld>
        <Fld
          label={t("page.connections.form.google-pubsub.projectId")}
          hint={t("page.connections.form.google-pubsub.projectIdHint")}
        >
          <Input
            className="mono3"
            style={MONO}
            value={value.projectId}
            placeholder="my-project"
            onChange={(event) => set("projectId", event.target.value)}
          />
        </Fld>
        <Fld
          span
          label={t("page.connections.form.google-pubsub.credentialsJson")}
          hint={
            stored ? (
              <button
                type="button"
                className="mqs-linkbtn"
                onClick={() => set("clearCredentials", true)}
              >
                {t("page.connections.form.clearCredentials")}
              </button>
            ) : (
              t("page.connections.form.google-pubsub.credentialsJsonHint")
            )
          }
        >
          {/* A textarea rather than an input: a service account key is a
              JSON document of a dozen lines, and a single-line password
              field would show four characters of it. */}
          <textarea
            className="mono3 min-h-20 rounded-md border border-(--c-line) bg-(--c-surface) px-2.5 py-2 outline-none focus-visible:border-(--c-accent)"
            style={MONO}
            spellCheck={false}
            value={value.credentialsJson}
            placeholder={
              stored
                ? t("page.connections.form.secretStored")
                : '{"type":"service_account", ...}'
            }
            onChange={(event) => set("credentialsJson", event.target.value)}
          />
        </Fld>
      </div>

      <FormNote
        advanced={
          <button
            type="button"
            className="mqs-disclosure"
            aria-expanded={advancedOpen}
            onClick={() => setAdvancedOpen((open) => !open)}
          >
            <ChevronRight size={12} aria-hidden />
            {t("page.connections.form.rocketmq.advanced")}
          </button>
        }
        note={t("page.connections.form.google-pubsub.note")}
      />
      {advancedOpen && (
        <div style={GRID}>
          <Fld
            label={t("page.connections.form.google-pubsub.resourcePrefix")}
            hint={t("page.connections.form.google-pubsub.resourcePrefixHint")}
          >
            <Input
              className="mono3"
              style={MONO}
              value={value.resourcePrefix}
              placeholder="team-orders-"
              onChange={(event) => set("resourcePrefix", event.target.value)}
            />
          </Fld>
          <Fld
            label={t("page.connections.form.rocketmq.timeout")}
            hint={t("page.connections.form.rocketmq.timeoutHint")}
          >
            <Input
              type="number"
              value={value.timeoutSec > 0 ? String(value.timeoutSec) : ""}
              onChange={(event) => {
                const seconds = Number.parseInt(event.target.value, 10);
                set("timeoutSec", Number.isNaN(seconds) ? 0 : seconds);
              }}
            />
          </Fld>
          <Fld
            span
            label={t("page.connections.form.google-pubsub.emulatorHost")}
            hint={t("page.connections.form.google-pubsub.emulatorHostHint")}
          >
            <Input
              className="mono3"
              style={MONO}
              value={value.emulatorHost}
              placeholder="127.0.0.1:8085"
              onChange={(event) => set("emulatorHost", event.target.value)}
            />
          </Fld>
          <Fld
            span
            label={t("page.connections.form.remark")}
            hint={t("page.connections.form.remarkHint")}
          >
            <Input value={value.remark} onChange={(event) => set("remark", event.target.value)} />
          </Fld>
        </div>
      )}
    </>
  );
}

/** Option keys the Azure Service Bus driver reads back off a stored profile. */
export const OPTION_SB_KEY_NAME = "sharedAccessKeyName";
export const OPTION_SB_EMULATOR_MANAGEMENT = "emulatorManagementHost";
export const OPTION_SB_ENTITY_PREFIX = "entityPrefix";

/** The shared access key name every namespace is created with. */
const SB_DEFAULT_KEY_NAME = "RootManageSharedAccessKey";

/**
 * The third hosted form, and the first of them with an address row.
 *
 * SQS and Pub/Sub have none because a region and a project are not addresses.
 * A Service Bus namespace is one: myns.servicebus.windows.net resolves, and
 * the driver opens an AMQP connection to it and sends HTTPS requests to it. So
 * it takes the endpoint row every dialled family has, and the connection list
 * prints it in the address column like any other.
 *
 * There are two ways to fill this in and both are on the form, because both
 * are what people actually have. The Azure portal offers a whole connection
 * string, so there is a field for one; a least-privilege deployment hands out
 * a policy name and a key instead, so there are fields for those. A pasted
 * string wins, because it carries an endpoint of its own and a merge would
 * have to decide which of two disagreeing namespaces was meant.
 *
 * The emulator management host is a second port rather than an endpoint
 * override, and that is the emulator rather than a choice here: it serves AMQP
 * and its Atom management API on two ports, where a real namespace serves both
 * on the one host it is named after. A connection that has to be told a second
 * address is by definition not talking to one, and the driver reads exactly
 * that to report what the emulator cannot do.
 */
export interface AzureServiceBusDraft {
  name: string;
  /** The fully qualified namespace. Required: both clients dial it. */
  endpoints: string;
  /** Which shared access policy the key below belongs to. */
  keyName: string;
  /** The shared access key. Blank is fine only if a string is pasted. */
  sharedAccessKey: string;
  /** The whole connection string, for whoever copied one out of the portal. */
  connectionString: string;
  /** Narrows every listing. A namespace's entities are not one team's. */
  entityPrefix: string;
  /** host:port of a Service Bus emulator's management port. */
  emulatorManagement: string;
  group: string;
  remark: string;
  timeoutSec: number;
  credentialsStored: boolean;
  clearCredentials: boolean;
}

export function emptyAzureServiceBusDraft(): AzureServiceBusDraft {
  return {
    name: "",
    endpoints: "",
    keyName: SB_DEFAULT_KEY_NAME,
    sharedAccessKey: "",
    connectionString: "",
    entityPrefix: "",
    emulatorManagement: "",
    group: "",
    remark: "",
    timeoutSec: DEFAULT_TIMEOUT_SEC,
    credentialsStored: false,
    clearCredentials: false,
  };
}

/** Azure Service Bus, addressed by a namespace and signed with a shared key. */
export function AzureServiceBusForm({
  value,
  onChange,
}: {
  value: AzureServiceBusDraft;
  onChange: (next: AzureServiceBusDraft) => void;
}) {
  const { t } = useTranslation();
  const set = <K extends keyof AzureServiceBusDraft>(key: K, next: AzureServiceBusDraft[K]) =>
    onChange({ ...value, [key]: next });
  const [advancedOpen, setAdvancedOpen] = useState(
    value.timeoutSec !== DEFAULT_TIMEOUT_SEC ||
      value.remark !== "" ||
      value.entityPrefix !== "" ||
      value.emulatorManagement !== "" ||
      value.connectionString !== "",
  );
  const stored = value.credentialsStored && !value.clearCredentials;

  return (
    <>
      <div style={GRID}>
        <Fld label={t("page.connections.form.name")}>
          <Input
            value={value.name}
            placeholder="servicebus-prod"
            onChange={(event) => set("name", event.target.value)}
          />
        </Fld>
        <Fld
          label={t("page.connections.form.azure-servicebus.namespace")}
          hint={t("page.connections.form.azure-servicebus.namespaceHint")}
        >
          <Input
            className="mono3"
            style={MONO}
            value={value.endpoints}
            placeholder="my-namespace.servicebus.windows.net"
            onChange={(event) => set("endpoints", event.target.value)}
          />
        </Fld>
        <Fld
          label={t("page.connections.form.azure-servicebus.keyName")}
          hint={t("page.connections.form.azure-servicebus.keyNameHint")}
        >
          <Input
            className="mono3"
            style={MONO}
            value={value.keyName}
            placeholder={SB_DEFAULT_KEY_NAME}
            onChange={(event) => set("keyName", event.target.value)}
          />
        </Fld>
        <Fld
          label={t("page.connections.form.azure-servicebus.sharedAccessKey")}
          hint={
            stored ? (
              <button
                type="button"
                className="mqs-linkbtn"
                onClick={() => set("clearCredentials", true)}
              >
                {t("page.connections.form.clearCredentials")}
              </button>
            ) : (
              t("page.connections.form.azure-servicebus.sharedAccessKeyHint")
            )
          }
        >
          <Input
            type="password"
            value={value.sharedAccessKey}
            placeholder={stored ? t("page.connections.form.secretStored") : ""}
            onChange={(event) => set("sharedAccessKey", event.target.value)}
          />
        </Fld>
      </div>

      <FormNote
        advanced={
          <button
            type="button"
            className="mqs-disclosure"
            aria-expanded={advancedOpen}
            onClick={() => setAdvancedOpen((open) => !open)}
          >
            <ChevronRight size={12} aria-hidden />
            {t("page.connections.form.rocketmq.advanced")}
          </button>
        }
        note={t("page.connections.form.azure-servicebus.note")}
      />
      {advancedOpen && (
        <div style={GRID}>
          <Fld
            span
            label={t("page.connections.form.azure-servicebus.connectionString")}
            hint={t("page.connections.form.azure-servicebus.connectionStringHint")}
          >
            {/* A textarea rather than a password input: the string runs past
                a hundred characters, and a form that showed four of them
                could not be checked by the person pasting it. */}
            <textarea
              className="mono3 min-h-14 rounded-md border border-(--c-line) bg-(--c-surface) px-2.5 py-2 outline-none focus-visible:border-(--c-accent)"
              style={MONO}
              spellCheck={false}
              value={value.connectionString}
              placeholder={
                stored
                  ? t("page.connections.form.secretStored")
                  : "Endpoint=sb://...;SharedAccessKeyName=...;SharedAccessKey=..."
              }
              onChange={(event) => set("connectionString", event.target.value)}
            />
          </Fld>
          <Fld
            label={t("page.connections.form.azure-servicebus.entityPrefix")}
            hint={t("page.connections.form.azure-servicebus.entityPrefixHint")}
          >
            <Input
              className="mono3"
              style={MONO}
              value={value.entityPrefix}
              placeholder="team-orders-"
              onChange={(event) => set("entityPrefix", event.target.value)}
            />
          </Fld>
          <Fld
            label={t("page.connections.form.rocketmq.timeout")}
            hint={t("page.connections.form.rocketmq.timeoutHint")}
          >
            <Input
              type="number"
              value={value.timeoutSec > 0 ? String(value.timeoutSec) : ""}
              onChange={(event) => {
                const seconds = Number.parseInt(event.target.value, 10);
                set("timeoutSec", Number.isNaN(seconds) ? 0 : seconds);
              }}
            />
          </Fld>
          <Fld
            span
            label={t("page.connections.form.azure-servicebus.emulatorManagement")}
            hint={t("page.connections.form.azure-servicebus.emulatorManagementHint")}
          >
            <Input
              className="mono3"
              style={MONO}
              value={value.emulatorManagement}
              placeholder="127.0.0.1:5300"
              onChange={(event) => set("emulatorManagement", event.target.value)}
            />
          </Fld>
          <Fld
            span
            label={t("page.connections.form.remark")}
            hint={t("page.connections.form.remarkHint")}
          >
            <Input value={value.remark} onChange={(event) => set("remark", event.target.value)} />
          </Fld>
        </div>
      )}
    </>
  );
}

/** Option keys the Kinesis driver reads back off a stored profile. */
export const OPTION_KINESIS_REGION = "region";
export const OPTION_KINESIS_STREAM_PREFIX = "streamPrefix";
export const OPTION_KINESIS_ENDPOINT_URL = "endpointUrl";

/**
 * The fourth form with no address field, and the second whose address is a
 * region.
 *
 * It is SQS's shape because it is the same service boundary: a stream is
 * reached by naming a region and signing a request, the SDK resolves
 * kinesis.<region>.amazonaws.com for itself, and the credential pair is
 * optional because a blank one means the machine's own AWS identity. What is
 * not shared is anything past the credential - a stream prefix narrows a
 * listing this driver filters itself, because ListStreams offers no filter the
 * way ListQueues does.
 */
export interface KinesisDraft {
  name: string;
  /** Required. What an endpoint field is on every other family. */
  region: string;
  accessKeyId: string;
  secretAccessKey: string;
  /** Only temporary credentials carry one, and it expires with the session. */
  sessionToken: string;
  /** Narrows every listing. An account's streams are not one team's. */
  streamPrefix: string;
  /** A VPC interface endpoint, or an emulator. Still signed for the region. */
  endpointUrl: string;
  group: string;
  remark: string;
  timeoutSec: number;
  credentialsStored: boolean;
  clearCredentials: boolean;
}

export function emptyKinesisDraft(): KinesisDraft {
  return {
    name: "",
    region: "",
    accessKeyId: "",
    secretAccessKey: "",
    sessionToken: "",
    streamPrefix: "",
    endpointUrl: "",
    group: "",
    remark: "",
    timeoutSec: DEFAULT_TIMEOUT_SEC,
    credentialsStored: false,
    clearCredentials: false,
  };
}

/** Amazon Kinesis Data Streams, addressed by a region rather than an address. */
export function KinesisForm({
  value,
  onChange,
}: {
  value: KinesisDraft;
  onChange: (next: KinesisDraft) => void;
}) {
  const { t } = useTranslation();
  const set = <K extends keyof KinesisDraft>(key: K, next: KinesisDraft[K]) =>
    onChange({ ...value, [key]: next });
  const [advancedOpen, setAdvancedOpen] = useState(
    value.timeoutSec !== DEFAULT_TIMEOUT_SEC ||
      value.remark !== "" ||
      value.streamPrefix !== "" ||
      value.endpointUrl !== "",
  );
  const stored = value.credentialsStored && !value.clearCredentials;

  return (
    <>
      <div style={GRID}>
        <Fld label={t("page.connections.form.name")}>
          <Input
            value={value.name}
            placeholder="kinesis-prod"
            onChange={(event) => set("name", event.target.value)}
          />
        </Fld>
        <Fld
          label={t("page.connections.form.kinesis.region")}
          hint={t("page.connections.form.kinesis.regionHint")}
        >
          <Input
            className="mono3"
            style={MONO}
            value={value.region}
            placeholder="eu-west-1"
            onChange={(event) => set("region", event.target.value)}
          />
        </Fld>
        <Fld
          label={t("page.connections.form.kinesis.accessKeyId")}
          hint={
            stored ? (
              <button
                type="button"
                className="mqs-linkbtn"
                onClick={() => set("clearCredentials", true)}
              >
                {t("page.connections.form.clearCredentials")}
              </button>
            ) : (
              t("page.connections.form.kinesis.credentialsHint")
            )
          }
        >
          <Input
            className="mono3"
            style={MONO}
            value={value.accessKeyId}
            placeholder={stored ? t("page.connections.form.secretStored") : "AKIA..."}
            onChange={(event) => set("accessKeyId", event.target.value)}
          />
        </Fld>
        <Fld label={t("page.connections.form.kinesis.secretAccessKey")}>
          <Input
            type="password"
            value={value.secretAccessKey}
            placeholder={stored ? t("page.connections.form.secretStored") : undefined}
            onChange={(event) => set("secretAccessKey", event.target.value)}
          />
        </Fld>
        <Fld
          span
          label={t("page.connections.form.kinesis.sessionToken")}
          hint={t("page.connections.form.kinesis.sessionTokenHint")}
        >
          <Input
            type="password"
            value={value.sessionToken}
            placeholder={stored ? t("page.connections.form.secretStored") : undefined}
            onChange={(event) => set("sessionToken", event.target.value)}
          />
        </Fld>
      </div>

      <FormNote
        advanced={
          <button
            type="button"
            className="mqs-disclosure"
            aria-expanded={advancedOpen}
            onClick={() => setAdvancedOpen((open) => !open)}
          >
            <ChevronRight size={12} aria-hidden />
            {t("page.connections.form.rocketmq.advanced")}
          </button>
        }
        note={t("page.connections.form.kinesis.note")}
      />
      {advancedOpen && (
        <div style={GRID}>
          <Fld
            label={t("page.connections.form.kinesis.streamPrefix")}
            hint={t("page.connections.form.kinesis.streamPrefixHint")}
          >
            <Input
              className="mono3"
              style={MONO}
              value={value.streamPrefix}
              placeholder="team-orders-"
              onChange={(event) => set("streamPrefix", event.target.value)}
            />
          </Fld>
          <Fld
            label={t("page.connections.form.rocketmq.timeout")}
            hint={t("page.connections.form.rocketmq.timeoutHint")}
          >
            <Input
              type="number"
              value={value.timeoutSec > 0 ? String(value.timeoutSec) : ""}
              onChange={(event) => {
                const seconds = Number.parseInt(event.target.value, 10);
                set("timeoutSec", Number.isNaN(seconds) ? 0 : seconds);
              }}
            />
          </Fld>
          <Fld
            span
            label={t("page.connections.form.kinesis.endpointUrl")}
            hint={t("page.connections.form.kinesis.endpointUrlHint")}
          >
            <Input
              className="mono3"
              style={MONO}
              value={value.endpointUrl}
              placeholder="https://vpce-0abc.kinesis.eu-west-1.vpce.amazonaws.com"
              onChange={(event) => set("endpointUrl", event.target.value)}
            />
          </Fld>
          <Fld
            span
            label={t("page.connections.form.remark")}
            hint={t("page.connections.form.remarkHint")}
          >
            <Input value={value.remark} onChange={(event) => set("remark", event.target.value)} />
          </Fld>
        </div>
      )}
    </>
  );
}

/** Option keys the IBM MQ driver reads back off a stored profile. */
export const OPTION_IBMMQ_QUEUE_MANAGER = "queueManager";
export const OPTION_IBMMQ_TLS_SKIP_VERIFY = "tlsSkipVerify";

/**
 * Two mechanisms, because the mqweb server has two.
 *
 * The queue manager's own authentication is a CONNAUTH object applied to its
 * channels, and it has no bearing on reaching the web server - which is the
 * whole management plane here, so these credentials are mqweb's rather than
 * the queue manager's.
 */
export type IbmMqMechanism = "none" | "plain";

export interface IbmMqDraft {
  name: string;
  /** The mqweb server's URL. For this family the endpoint is HTTPS. */
  endpoints: string;
  mechanism: IbmMqMechanism;
  username: string;
  password: string;
  /** Which queue manager behind that server; discovered when there is one. */
  queueManager: string;
  /** The messaging interface's own account. Blank reuses the pair above. */
  messagingUsername: string;
  messagingPassword: string;
  /** mqweb signs its own certificate unless it has been given a real one. */
  tlsSkipVerify: boolean;
  group: string;
  remark: string;
  timeoutSec: number;
  /** A stored secret never comes back, so blank means "keep it". */
  credentialsStored: boolean;
  clearCredentials: boolean;
}

export function emptyIbmMqDraft(): IbmMqDraft {
  return {
    name: "",
    endpoints: "",
    mechanism: "plain",
    username: "",
    password: "",
    queueManager: "",
    messagingUsername: "",
    messagingPassword: "",
    tlsSkipVerify: false,
    group: "",
    remark: "",
    timeoutSec: DEFAULT_TIMEOUT_SEC,
    credentialsStored: false,
    clearCredentials: false,
  };
}

/**
 * IBM MQ, addressed by its web server rather than by its listener.
 *
 * The address is the mqweb server's URL and not the queue manager's port 1414,
 * which is unlike every other family with a broker to dial and is not a
 * simplification: mqweb hosts both REST interfaces, and between them they are
 * the whole of what this app needs. 1414 belongs to applications.
 *
 * The queue manager is a field rather than part of the address because it is a
 * path segment on that server, and it is optional because most installations
 * front exactly one - the driver asks and takes the answer. Filling it in is
 * for a server fronting several, and for a profile that should say out loud
 * which one it is for.
 *
 * Two credential blocks, as ActiveMQ, MQTT and NATS have. The second is
 * optional: mqweb maps the administrative interface and the messaging one to
 * two roles, most deployments give one account both, and IBM's developer image
 * gives one account each - which is exactly when this pair is needed.
 */
export function IbmMqForm({
  value,
  onChange,
}: {
  value: IbmMqDraft;
  onChange: (next: IbmMqDraft) => void;
}) {
  const { t } = useTranslation();
  const set = <K extends keyof IbmMqDraft>(key: K, next: IbmMqDraft[K]) =>
    onChange({ ...value, [key]: next });
  const [advancedOpen, setAdvancedOpen] = useState(
    value.timeoutSec !== DEFAULT_TIMEOUT_SEC ||
      value.remark !== "" ||
      value.messagingUsername !== "" ||
      value.tlsSkipVerify,
  );
  const stored = value.credentialsStored && !value.clearCredentials;

  return (
    <>
      <div style={GRID}>
        <Fld label={t("page.connections.form.name")}>
          <Input
            value={value.name}
            placeholder="ibmmq-prod"
            onChange={(event) => set("name", event.target.value)}
          />
        </Fld>
        <Fld label={t("page.connections.form.ibmmq.mechanism")}>
          <SelectField<IbmMqMechanism>
            value={value.mechanism}
            options={[
              { value: "plain", label: t("page.connections.form.ibmmq.mechanismPlain") },
              { value: "none", label: t("page.connections.form.ibmmq.mechanismNone") },
            ]}
            onValueChange={(next) =>
              onChange({
                ...value,
                mechanism: next,
                username: next === "plain" ? value.username : "",
                password: next === "plain" ? value.password : "",
              })
            }
          />
        </Fld>
        <Fld
          span
          label={t("page.connections.form.ibmmq.mqweb")}
          hint={t("page.connections.form.ibmmq.mqwebHint")}
        >
          <Input
            className="mono3"
            style={MONO}
            value={value.endpoints}
            placeholder="https://mq.example.com:9443"
            onChange={(event) => set("endpoints", event.target.value)}
          />
        </Fld>
        <Fld
          span
          label={t("page.connections.form.ibmmq.queueManager")}
          hint={t("page.connections.form.ibmmq.queueManagerHint")}
        >
          <Input
            className="mono3"
            style={MONO}
            value={value.queueManager}
            placeholder="QM1"
            onChange={(event) => set("queueManager", event.target.value)}
          />
        </Fld>
        {value.mechanism === "plain" && (
          <>
            <Fld
              label={t("page.connections.form.username")}
              hint={
                stored ? (
                  <button
                    type="button"
                    className="mqs-linkbtn"
                    onClick={() => set("clearCredentials", true)}
                  >
                    {t("page.connections.form.clearCredentials")}
                  </button>
                ) : (
                  t("page.connections.form.ibmmq.adminHint")
                )
              }
            >
              <Input
                value={value.username}
                placeholder={stored ? t("page.connections.form.secretStored") : undefined}
                onChange={(event) => set("username", event.target.value)}
              />
            </Fld>
            <Fld label={t("page.connections.form.password")}>
              <Input
                type="password"
                value={value.password}
                placeholder={stored ? t("page.connections.form.secretStored") : undefined}
                onChange={(event) => set("password", event.target.value)}
              />
            </Fld>
          </>
        )}
      </div>

      <FormNote
        advanced={
          <button
            type="button"
            className="mqs-disclosure"
            aria-expanded={advancedOpen}
            onClick={() => setAdvancedOpen((open) => !open)}
          >
            <ChevronRight size={12} aria-hidden />
            {t("page.connections.form.rocketmq.advanced")}
          </button>
        }
        note={t("page.connections.form.ibmmq.note")}
      />
      {advancedOpen && (
        <div style={GRID}>
          <Fld
            label={t("page.connections.form.ibmmq.messagingUsername")}
            hint={t("page.connections.form.ibmmq.messagingHint")}
          >
            <Input
              value={value.messagingUsername}
              placeholder={stored ? t("page.connections.form.secretStored") : undefined}
              onChange={(event) => set("messagingUsername", event.target.value)}
            />
          </Fld>
          <Fld label={t("page.connections.form.ibmmq.messagingPassword")}>
            <Input
              type="password"
              value={value.messagingPassword}
              placeholder={stored ? t("page.connections.form.secretStored") : undefined}
              onChange={(event) => set("messagingPassword", event.target.value)}
            />
          </Fld>
          <Fld
            label={t("page.connections.form.rocketmq.timeout")}
            hint={t("page.connections.form.rocketmq.timeoutHint")}
          >
            <Input
              type="number"
              value={value.timeoutSec > 0 ? String(value.timeoutSec) : ""}
              onChange={(event) => {
                const seconds = Number.parseInt(event.target.value, 10);
                set("timeoutSec", Number.isNaN(seconds) ? 0 : seconds);
              }}
            />
          </Fld>
          <Fld
            label={t("page.connections.form.remark")}
            hint={t("page.connections.form.remarkHint")}
          >
            <Input value={value.remark} onChange={(event) => set("remark", event.target.value)} />
          </Fld>
          <Fld
            span
            label={t("page.connections.form.ibmmq.skipVerify")}
            hint={t("page.connections.form.ibmmq.skipVerifyHint")}
          >
            <div style={SWITCH_ROW}>
              <Switch
                checked={value.tlsSkipVerify}
                onCheckedChange={(next: boolean) => set("tlsSkipVerify", next)}
              />
              <span style={{ color: "var(--c-muted)" }}>
                {t("page.connections.form.kafka.skipVerifyNote")}
              </span>
            </div>
          </Fld>
        </div>
      )}
    </>
  );
}

/** Option keys the Solace driver reads back off a stored profile. */
export const OPTION_SOLACE_MSG_VPN = "msgVpn";
export const OPTION_SOLACE_REST_URL = "restUrl";
export const OPTION_SOLACE_TLS_SKIP_VERIFY = "tlsSkipVerify";

/**
 * Two mechanisms, because SEMP has two.
 *
 * These credentials are the broker's management users - broker-wide accounts
 * with an access level - and not the Message VPN's client-usernames, which is
 * what an application authenticates as. The two live in different directories
 * and neither stands in for the other, which is why the REST messaging pair
 * below is its own.
 */
export type SolaceMechanism = "none" | "plain";

export interface SolaceDraft {
  name: string;
  /** The broker's SEMP URL. For this family the endpoint is HTTP. */
  endpoints: string;
  mechanism: SolaceMechanism;
  username: string;
  password: string;
  /** Which Message VPN on that broker. Blank falls back to "default". */
  msgVpn: string;
  /** The REST messaging interface, when it is not simply another port here. */
  restUrl: string;
  /** A client-username in that VPN. Blank sends no credential at all. */
  restUsername: string;
  restPassword: string;
  /** A broker signs its own certificate unless it has been given a real one. */
  tlsSkipVerify: boolean;
  group: string;
  remark: string;
  timeoutSec: number;
  /** A stored secret never comes back, so blank means "keep it". */
  credentialsStored: boolean;
  clearCredentials: boolean;
}

export function emptySolaceDraft(): SolaceDraft {
  return {
    name: "",
    endpoints: "",
    mechanism: "plain",
    username: "",
    password: "",
    msgVpn: "",
    restUrl: "",
    restUsername: "",
    restPassword: "",
    tlsSkipVerify: false,
    group: "",
    remark: "",
    timeoutSec: DEFAULT_TIMEOUT_SEC,
    credentialsStored: false,
    clearCredentials: false,
  };
}

/**
 * Solace PubSub+, addressed by SEMP rather than by its messaging listener.
 *
 * The address is the broker's SEMP URL and not port 55555, which is the shape
 * ActiveMQ and IBM MQ already have: the management plane is HTTP, and it is
 * the whole of what the boards read. 55555 belongs to applications.
 *
 * The Message VPN is a field rather than part of that address because it is a
 * path segment on the broker, and it is optional because every broker ships
 * one called "default". It is also the field the sidebar's scope switcher
 * writes, so a connection is re-pointed at another VPN without being edited.
 *
 * Two credential blocks, and the second is not the first under another name.
 * SEMP authenticates a management user; the REST messaging interface
 * authenticates a client-username, which is an object inside one Message VPN.
 * A Message VPN that takes any username - the out-of-the-box setting - needs
 * neither, which is why the pair is in the advanced block and may be left
 * empty.
 */
export function SolaceForm({
  value,
  onChange,
}: {
  value: SolaceDraft;
  onChange: (next: SolaceDraft) => void;
}) {
  const { t } = useTranslation();
  const set = <K extends keyof SolaceDraft>(key: K, next: SolaceDraft[K]) =>
    onChange({ ...value, [key]: next });
  const [advancedOpen, setAdvancedOpen] = useState(
    value.timeoutSec !== DEFAULT_TIMEOUT_SEC ||
      value.remark !== "" ||
      value.restUrl !== "" ||
      value.restUsername !== "" ||
      value.tlsSkipVerify,
  );
  const stored = value.credentialsStored && !value.clearCredentials;

  return (
    <>
      <div style={GRID}>
        <Fld label={t("page.connections.form.name")}>
          <Input
            value={value.name}
            placeholder="solace-prod"
            onChange={(event) => set("name", event.target.value)}
          />
        </Fld>
        <Fld label={t("page.connections.form.solace.mechanism")}>
          <SelectField<SolaceMechanism>
            value={value.mechanism}
            options={[
              { value: "plain", label: t("page.connections.form.solace.mechanismPlain") },
              { value: "none", label: t("page.connections.form.solace.mechanismNone") },
            ]}
            onValueChange={(next) =>
              onChange({
                ...value,
                mechanism: next,
                username: next === "plain" ? value.username : "",
                password: next === "plain" ? value.password : "",
              })
            }
          />
        </Fld>
        <Fld
          span
          label={t("page.connections.form.solace.semp")}
          hint={t("page.connections.form.solace.sempHint")}
        >
          <Input
            className="mono3"
            style={MONO}
            value={value.endpoints}
            placeholder="http://solace.example.com:8080"
            onChange={(event) => set("endpoints", event.target.value)}
          />
        </Fld>
        <Fld
          span
          label={t("page.connections.form.solace.msgVpn")}
          hint={t("page.connections.form.solace.msgVpnHint")}
        >
          <Input
            className="mono3"
            style={MONO}
            value={value.msgVpn}
            placeholder="default"
            onChange={(event) => set("msgVpn", event.target.value)}
          />
        </Fld>
        {value.mechanism === "plain" && (
          <>
            <Fld
              label={t("page.connections.form.username")}
              hint={
                stored ? (
                  <button
                    type="button"
                    className="mqs-linkbtn"
                    onClick={() => set("clearCredentials", true)}
                  >
                    {t("page.connections.form.clearCredentials")}
                  </button>
                ) : (
                  t("page.connections.form.solace.adminHint")
                )
              }
            >
              <Input
                value={value.username}
                placeholder={stored ? t("page.connections.form.secretStored") : undefined}
                onChange={(event) => set("username", event.target.value)}
              />
            </Fld>
            <Fld label={t("page.connections.form.password")}>
              <Input
                type="password"
                value={value.password}
                placeholder={stored ? t("page.connections.form.secretStored") : undefined}
                onChange={(event) => set("password", event.target.value)}
              />
            </Fld>
          </>
        )}
      </div>

      <FormNote
        advanced={
          <button
            type="button"
            className="mqs-disclosure"
            aria-expanded={advancedOpen}
            onClick={() => setAdvancedOpen((open) => !open)}
          >
            <ChevronRight size={12} aria-hidden />
            {t("page.connections.form.rocketmq.advanced")}
          </button>
        }
        note={t("page.connections.form.solace.note")}
      />
      {advancedOpen && (
        <div style={GRID}>
          <Fld
            span
            label={t("page.connections.form.solace.restUrl")}
            hint={t("page.connections.form.solace.restUrlHint")}
          >
            <Input
              className="mono3"
              style={MONO}
              value={value.restUrl}
              placeholder="http://solace.example.com:9000"
              onChange={(event) => set("restUrl", event.target.value)}
            />
          </Fld>
          <Fld
            label={t("page.connections.form.solace.restUsername")}
            hint={t("page.connections.form.solace.restHint")}
          >
            <Input
              value={value.restUsername}
              placeholder={stored ? t("page.connections.form.secretStored") : undefined}
              onChange={(event) => set("restUsername", event.target.value)}
            />
          </Fld>
          <Fld label={t("page.connections.form.solace.restPassword")}>
            <Input
              type="password"
              value={value.restPassword}
              placeholder={stored ? t("page.connections.form.secretStored") : undefined}
              onChange={(event) => set("restPassword", event.target.value)}
            />
          </Fld>
          <Fld
            label={t("page.connections.form.rocketmq.timeout")}
            hint={t("page.connections.form.rocketmq.timeoutHint")}
          >
            <Input
              type="number"
              value={value.timeoutSec > 0 ? String(value.timeoutSec) : ""}
              onChange={(event) => {
                const seconds = Number.parseInt(event.target.value, 10);
                set("timeoutSec", Number.isNaN(seconds) ? 0 : seconds);
              }}
            />
          </Fld>
          <Fld
            label={t("page.connections.form.remark")}
            hint={t("page.connections.form.remarkHint")}
          >
            <Input value={value.remark} onChange={(event) => set("remark", event.target.value)} />
          </Fld>
          <Fld
            span
            label={t("page.connections.form.solace.skipVerify")}
            hint={t("page.connections.form.solace.skipVerifyHint")}
          >
            <div style={SWITCH_ROW}>
              <Switch
                checked={value.tlsSkipVerify}
                onCheckedChange={(next: boolean) => set("tlsSkipVerify", next)}
              />
              <span style={{ color: "var(--c-muted)" }}>
                {t("page.connections.form.kafka.skipVerifyNote")}
              </span>
            </div>
          </Fld>
        </div>
      )}
    </>
  );
}
