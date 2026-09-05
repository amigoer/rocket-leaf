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
      {/* The label grows and the input sits at the bottom, so two fields side
          by side line up whatever their hints do. Without this a hint that
          wraps pushes its own input down and leaves its neighbour's floating
          half a field above it. */}
      <span className="flex-1 font-medium">
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
