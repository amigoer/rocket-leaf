import { useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { Check, RefreshCw, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Spinner } from "@/components/ui/spinner";
import {
  SectionLabel,
} from "@/components";
import { ProtocolIcon } from "@/design/icons/ProtocolIcon";
import {
  PROTOCOL_GROUPS,
  PROTOCOL_ORDER,
  isProtocolReady,
  protocolsIn,
  type ProtocolId,
} from "@/design/data/protocols";
import { cn, formatErrorMessage } from "@/lib/utils";
import type { ConnectionDraft, CredentialsMode } from "@/api/connection";
import type { Connection as ConnectionProfile } from "@/api/models";
import {
  KafkaForm,
  ActiveMQForm,
  MqttForm,
  NatsForm,
  NsqForm,
  PulsarForm,
  RabbitMQForm,
  RedisForm,
  RocketMQForm,
  SqsForm,
  GooglePubSubForm,
  IbmMqForm,
  SolaceForm,
  KinesisForm,
  AzureServiceBusForm,
} from "./ConnectionForms";
import {
  emptyDraft,
  isDraftable,
  toDraft,
  toSubmission,
  type ProtocolDraft,
} from "./connectionDraft";
import { draftInvalidReason } from "./connectionValidation";

/** Version ranges printed under each tile in the 3a protocol picker. */
const TILE: Record<ProtocolId, { name: string; versions: string }> = {
  rocketmq: { name: "RocketMQ", versions: "4.x / 5.x" },
  kafka: { name: "Kafka", versions: "3.x / 4.x" },
  rabbitmq: { name: "RabbitMQ", versions: "3.x / 4.x" },
  pulsar: { name: "Pulsar", versions: "2.x / 3.x" },
  redis: { name: "Redis Stream", versions: "6.0+" },
  mqtt: { name: "MQTT", versions: "3.1 / 5.0" },
  nats: { name: "NATS", versions: "2.x" },
  // One tile for two products. Which one is behind the console is the
  // driver's to work out, so asking here would only let a user get it wrong.
  activemq: { name: "ActiveMQ", versions: "Classic 5/6 · Artemis 2.x" },
  nsq: { name: "NSQ", versions: "1.x" },
  // No version to print: SQS is a managed service with one, whichever AWS
  // is running. What varies is the region, and that is a form field.
  sqs: { name: "Amazon SQS", versions: "managed" },
  // Managed for the same reason SQS is, and what varies is the project rather
  // than a region: one address serves every project there is.
  "google-pubsub": { name: "Google Pub/Sub", versions: "managed" },
  // Managed too, and what varies is the namespace - which unlike a region or
  // a project is an address, so it is the one hosted family with an endpoint
  // row rather than a field standing in for one.
  "azure-servicebus": { name: "Azure Service Bus", versions: "managed" },
  // Managed, and back to a region: Kinesis is the second AWS family here and
  // is reached exactly the way SQS is.
  kinesis: { name: "Amazon Kinesis", versions: "managed" },
  // Not managed: this one is a queue manager somebody runs, reached through
  // the web server beside it. The version printed is the mqweb server's REST
  // API rather than the product's, because that is what the driver speaks.
  ibmmq: { name: "IBM MQ", versions: "9.1+ (mqweb REST)" },
  // Not managed either, and the version printed is SEMP's rather than the
  // broker's: v2 is what the driver speaks, and it is the half of the API
  // that has a config and a monitor tree instead of an XML RPC.
  solace: { name: "Solace PubSub+", versions: "9.4+ (SEMP v2)" },
};

/** What the probe last reported, drawn in the footer beside the test button. */
type ProbeState =
  | { kind: "idle" }
  | { kind: "running" }
  | { kind: "ok"; latency: string }
  | { kind: "failed"; message: string };

/**
 * Board 3a with one form per protocol. The canvas drew a field set for every
 * family it imagined; a tile is offered only where a driver and a form both
 * exist, and anything without both is shown disabled rather than offering a
 * form that cannot be saved.
 *
 * `editing` turns the dialog into the edit form for a stored profile, which
 * the canvas never drew separately because the field set is the same one.
 */
export function NewConnectionDialog({
  open,
  onClose,
  initialProtocol = "rocketmq",
  editing,
  onSubmit,
  onProbe,
}: {
  open: boolean;
  onClose?: () => void;
  initialProtocol?: ProtocolId;
  /** Set to edit a stored profile instead of creating one. */
  editing?: ConnectionProfile;
  /** Resolves when the profile is stored; rejects with what Go reported. */
  onSubmit?: (draft: ConnectionDraft, credentialsMode: CredentialsMode) => Promise<void>;
  onProbe?: (draft: ConnectionDraft, credentialsMode: CredentialsMode) => Promise<number>;
}) {
  const { t } = useTranslation();
  const [draft, setDraft] = useState<ProtocolDraft>(() => emptyDraft("rocketmq"));
  const protocol = draft.protocol;
  /*
   * Two steps, not one column.
   *
   * The protocol list and the form it configures are separate questions, and
   * stacking them made the dialog a scroll: fifteen tiles above, the fields
   * that are actually being filled in below, and every vendor added pushing
   * the form further down. Each step now has the whole dialog.
   *
   * An edit opens on the form and cannot go back - the protocol is what a
   * stored profile is, so changing it would make this a different connection.
   */
  const [step, setStep] = useState<"protocol" | "form">(editing == null ? "protocol" : "form");
  const [search, setSearch] = useState("");
  const [probe, setProbe] = useState<ProbeState>({ kind: "idle" });
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Reopening the dialog has to start from what it is opening on, not from
  // whatever the last edit left in state.
  useEffect(() => {
    if (!open) return;
    if (editing != null) {
      setDraft(toDraft(editing));
    } else {
      const wanted = isProtocolReady(initialProtocol) && isDraftable(initialProtocol)
        ? initialProtocol
        : "rocketmq";
      setDraft(emptyDraft(wanted));
    }
    // Open on a new connection so the whole list is browsable, shut on an
    // edit where the protocol cannot change anyway.
    setStep(editing == null ? "protocol" : "form");
    setSearch("");
    setProbe({ kind: "idle" });
    setError(null);
    setSaving(false);
  }, [editing, initialProtocol, open]);

  /*
   * Matches a protocol against the search box, by the two things the tile
   * shows: its name and its versions. Typing "managed" finds the hosted
   * three, and "5.0" finds MQTT - both are on the tile, so both are things
   * somebody will try.
   */
  const matches = useMemo(() => {
    const needle = search.trim().toLowerCase();
    return (candidate: ProtocolId) => {
      if (needle === "") return true;
      const tile = TILE[candidate];
      return (
        tile.name.toLowerCase().includes(needle) ||
        tile.versions.toLowerCase().includes(needle)
      );
    };
  }, [search]);

  const invalid = useMemo(() => draftInvalidReason(draft, t), [draft, t]);

  const runProbe = async () => {
    if (invalid != null || onProbe == null) return;
    setProbe({ kind: "running" });
    try {
      const submission = toSubmission(draft);
      const elapsed = await onProbe(submission.draft, submission.credentialsMode);
      setProbe({
        kind: "ok",
        latency: elapsed < 1000 ? `${Math.round(elapsed)}ms` : `${(elapsed / 1000).toFixed(1)}s`,
      });
    } catch (probeError) {
      setProbe({ kind: "failed", message: formatErrorMessage(probeError) });
    }
  };

  const save = async () => {
    if (invalid != null || onSubmit == null) return;
    setSaving(true);
    setError(null);
    try {
      const submission = toSubmission(draft);
      await onSubmit(submission.draft, submission.credentialsMode);
      onClose?.();
    } catch (saveError) {
      setError(formatErrorMessage(saveError));
    } finally {
      setSaving(false);
    }
  };

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (!next) onClose?.();
      }}
    >
      <DialogContent className="flex max-h-[85vh] flex-col gap-3.5 overflow-y-auto sm:max-w-[580px]">
        <DialogHeader>
          <DialogTitle>
            {t(
              editing != null
                ? "page.connections.dialogTitleEdit"
                : step === "protocol"
                  ? "page.connections.dialogTitleProtocol"
                  : "page.connections.dialogTitle",
            )}
          </DialogTitle>
        </DialogHeader>
      {step === "protocol" ? (
        <>
          <Input
            value={search}
            onChange={(event) => setSearch(event.target.value)}
            placeholder={t("page.connections.protocolSearch")}
            autoFocus
          />
          {PROTOCOL_GROUPS.map((group) => {
            const listed = protocolsIn(group.id).filter(matches);
            if (listed.length === 0) return null;
            return (
              <div key={group.id}>
                <SectionLabel style={{ marginBottom: "8px" }}>{t(group.label)}</SectionLabel>
                <div style={{ display: "grid", gridTemplateColumns: "repeat(4,1fr)", gap: "8px" }}>
                  {listed.map((p) => {
                    const ready = isProtocolReady(p);
                    return (
                      <button
                        key={p}
                        type="button"
                        disabled={!ready}
                        aria-pressed={p === protocol}
                        /* Highlighted so returning from the form shows which
                           one is in hand, rather than an unmarked grid. */
                        className={cn("ptile", p === protocol && "sel")}
                        onClick={() => {
                          if (!isDraftable(p)) return;
                          setDraft(emptyDraft(p));
                          setProbe({ kind: "idle" });
                          setError(null);
                          setStep("form");
                        }}
                      >
                        <ProtocolIcon protocol={p} size={20} className="" />
                        {TILE[p].name}
                        <span className="pv">
                          {ready ? TILE[p].versions : t("page.connections.soon")}
                        </span>
                      </button>
                    );
                  })}
                </div>
              </div>
            );
          })}
          {PROTOCOL_ORDER.every((p) => !matches(p)) ? (
            <div style={{ fontSize: "11px", color: "var(--c-muted)" }}>
              {t("page.connections.protocolNoMatch")}
            </div>
          ) : null}
          {/* Only when something really is off. The version that printed
              unconditionally outlived the last dimmed tile and sat there
              describing protocols that all had drivers. */}
          {PROTOCOL_ORDER.some((p) => !isProtocolReady(p)) ? (
            <div style={{ fontSize: "11px", color: "var(--c-muted)" }}>
              {t("page.connections.protocolSoonHint")}
            </div>
          ) : null}
          <DialogFooter>
            <Button variant="outline" onClick={onClose}>
              {t("common.cancel")}
            </Button>
          </DialogFooter>
        </>
      ) : (
        <>
      <div style={{ display: "flex", alignItems: "center", gap: "8px" }}>
        {editing == null ? (
          <Button
            type="button"
            size="sm"
            variant="ghost"
            onClick={() => {
              setStep("protocol");
              setSearch("");
            }}
          >
            {t("page.connections.protocolBack")}
          </Button>
        ) : null}
        <ProtocolIcon protocol={protocol} size={18} className="" />
        <span style={{ fontWeight: 500 }}>{TILE[protocol].name}</span>
        <span className="pv">{TILE[protocol].versions}</span>
      </div>
      {draft.protocol === "rabbitmq" ? (
        <RabbitMQForm
          value={draft.value}
          onChange={(next) => setDraft({ protocol: "rabbitmq", value: next })}
        />
      ) : draft.protocol === "kafka" ? (
        <KafkaForm
          value={draft.value}
          onChange={(next) => setDraft({ protocol: "kafka", value: next })}
        />
      ) : draft.protocol === "pulsar" ? (
        <PulsarForm
          value={draft.value}
          onChange={(next) => setDraft({ protocol: "pulsar", value: next })}
        />
      ) : draft.protocol === "redis" ? (
        <RedisForm
          value={draft.value}
          onChange={(next) => setDraft({ protocol: "redis", value: next })}
        />
      ) : draft.protocol === "mqtt" ? (
        <MqttForm
          value={draft.value}
          onChange={(next) => setDraft({ protocol: "mqtt", value: next })}
        />
      ) : draft.protocol === "nats" ? (
        <NatsForm
          value={draft.value}
          onChange={(next) => setDraft({ protocol: "nats", value: next })}
        />
      ) : draft.protocol === "activemq" ? (
        <ActiveMQForm
          value={draft.value}
          onChange={(next) => setDraft({ protocol: "activemq", value: next })}
        />
      ) : draft.protocol === "nsq" ? (
        <NsqForm
          value={draft.value}
          onChange={(next) => setDraft({ protocol: "nsq", value: next })}
        />
      ) : draft.protocol === "sqs" ? (
        <SqsForm
          value={draft.value}
          onChange={(next) => setDraft({ protocol: "sqs", value: next })}
        />
      ) : draft.protocol === "google-pubsub" ? (
        <GooglePubSubForm
          value={draft.value}
          onChange={(next) => setDraft({ protocol: "google-pubsub", value: next })}
        />
      ) : draft.protocol === "azure-servicebus" ? (
        <AzureServiceBusForm
          value={draft.value}
          onChange={(next) => setDraft({ protocol: "azure-servicebus", value: next })}
        />
      ) : draft.protocol === "kinesis" ? (
        <KinesisForm
          value={draft.value}
          onChange={(next) => setDraft({ protocol: "kinesis", value: next })}
        />
      ) : draft.protocol === "ibmmq" ? (
        <IbmMqForm
          value={draft.value}
          onChange={(next) => setDraft({ protocol: "ibmmq", value: next })}
        />
      ) : draft.protocol === "solace" ? (
        <SolaceForm
          value={draft.value}
          onChange={(next) => setDraft({ protocol: "solace", value: next })}
        />
      ) : (
        <RocketMQForm
          value={draft.value}
          onChange={(next) => setDraft({ protocol: "rocketmq", value: next })}
        />
      )}

        <DialogFooter className="items-center">
          <Button
            variant="outline"
            disabled={invalid != null || probe.kind === "running"}
            onClick={runProbe}
          >
            {probe.kind === "running" && <Spinner />}
            {t("page.connections.dialogTest")}
          </Button>
          <ProbeResult state={probe} />
          <span className="flex-1" />
          {/* The blocking reason belongs beside the button it blocks, not in a
              toast that appears after the click that did nothing. */}
          {(invalid ?? error) != null && (
            <span
              className={
                "max-w-80 text-right text-xs " +
                (error != null ? "text-(--c-err)" : "text-muted-foreground")
              }
            >
              {error ?? invalid}
            </span>
          )}
          <Button variant="outline" onClick={onClose}>
            {t("common.cancel")}
          </Button>
          <Button disabled={invalid != null || saving} onClick={save}>
            {saving && <Spinner />}
            {t(editing != null ? "page.connections.dialogSaveOnly" : "page.connections.dialogSave")}
          </Button>
        </DialogFooter>
        </>
      )}
      </DialogContent>
    </Dialog>
  );
}

function ProbeResult({ state }: { state: ProbeState }) {
  const { t } = useTranslation();
  if (state.kind === "idle") return null;

  const style = {
    display: "inline-flex",
    alignItems: "center",
    gap: "5px",
    fontSize: "11.5px",
    maxWidth: "260px",
  } as const;

  if (state.kind === "running") {
    return (
      <span style={{ ...style, color: "var(--c-muted)" }}>
        <RefreshCw size={13} className="mqs-turning" aria-hidden />
        {t("page.connections.testing")}
      </span>
    );
  }
  if (state.kind === "ok") {
    return (
      <span style={{ ...style, color: "var(--c-ok-text)" }}>
        <Check size={13} aria-hidden />
        {t("page.connections.probeOk", { latency: state.latency })}
      </span>
    );
  }
  /*
   * The reason is shown, not hovered.
   *
   * It used to live in a title attribute, which on WKWebView is no tooltip at
   * all - so a failed test said "could not connect" and nothing else, and the
   * one piece of information the button exists to produce was unreachable.
   */
  return (
    <span
      style={{ ...style, color: "var(--c-err)", alignItems: "flex-start" }}
      title={state.message}
    >
      <X size={13} aria-hidden style={{ flex: "none", marginTop: "2px" }} />
      <span style={{ minWidth: 0 }}>
        {t("page.connections.probeFailed")}
        <span style={{ display: "block", color: "var(--c-muted)", overflowWrap: "anywhere" }}>
          {state.message}
        </span>
      </span>
    </span>
  );
}
