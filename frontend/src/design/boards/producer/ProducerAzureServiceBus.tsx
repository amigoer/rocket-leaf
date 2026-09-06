import { useCallback, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { Plus, Send, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Field, FieldDescription, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Spinner } from "@/components/ui/spinner";
import { Panel, PanelHeader, SectionLabel, SelectField, toast } from "@/components";
import { Page, PageBody, PageHeader } from "@/design/shell";
import { BoardState } from "@/design/boards/BoardState";
import { useConnectionScope } from "@/mq/ConnectionScope";
import { useServiceBusEntities } from "@/hooks/azureservicebus/useServiceBusEntities";
import { entity as readEntity } from "@/mq/azureservicebus/entities";
import {
  MAX_COUNT,
  emptyServiceBusProducerDraft,
  sendProblem,
  sendWarning,
  toSendInput,
  type ServiceBusProducerDraft,
} from "@/mq/azureservicebus/publish";
import { formatErrorMessage } from "@/lib/utils";
import * as serviceBusApi from "@/api/azureservicebus";

function numberField(raw: string): number {
  const parsed = Number.parseInt(raw, 10);
  return Number.isNaN(parsed) ? 0 : parsed;
}

/**
 * The Azure Service Bus send console.
 *
 * Its own board rather than the shared one, because the shared one collects a
 * topic, tags, keys and a delay level - RocketMQ's vocabulary - and a Service
 * Bus message carries more than that and means different things by it. The
 * subject and the named properties are not extras here: they are what a
 * subscription's rules select on, so a console without them could send nothing
 * that a filtered subscription would ever receive.
 *
 * The delay is a real one, unlike Pub/Sub's absence of one: a scheduled
 * message sits in the entity with a state of its own until its time comes, and
 * the messages page is where it can be seen waiting.
 *
 * The warning under the button is the state no queue can be in. A topic stores
 * nothing: with no subscription attached, the send is accepted, reported as
 * sent, and discarded - and no board afterwards records that it happened,
 * because there is no backlog to notice it by.
 */
export function ProducerAzureServiceBus() {
  const { t } = useTranslation();
  const { id: connID } = useConnectionScope();

  const entities = useServiceBusEntities();
  const [draft, setDraft] = useState<ServiceBusProducerDraft>(emptyServiceBusProducerDraft);
  const [sending, setSending] = useState(false);
  const [sent, setSent] = useState<string | null>(null);

  const set = <K extends keyof ServiceBusProducerDraft>(
    key: K,
    value: ServiceBusProducerDraft[K],
  ) => setDraft((previous) => ({ ...previous, [key]: value }));

  const rows = useMemo(() => (entities.data ?? []).map(readEntity), [entities.data]);
  /* Null rather than zero for a queue, which has no subscriptions and cannot
     discard anything, and for an entity the listing did not include: the
     connection may be filtered by a prefix, and "not loaded" must not read as
     "nothing subscribes to it". */
  const subscribers = useMemo(() => {
    const chosen = rows.find((row) => row.name === draft.entity.trim());
    return chosen == null || chosen.kind !== "topic" ? null : chosen.subscribers;
  }, [rows, draft.entity]);

  const problem = sendProblem(draft);
  const warning = sendWarning(draft, subscribers);

  const send = useCallback(async () => {
    const input = toSendInput(draft);
    if (input == null || sending) return;
    setSending(true);
    try {
      const result = await serviceBusApi.send(connID, input);
      setSent(
        result.sequenceNumbers.length > 0
          ? t("board.azure-servicebus.producer.scheduledAs", {
              count: result.sent,
              sequences: result.sequenceNumbers.join(", "),
            })
          : t("board.azure-servicebus.producer.sentCount", { count: result.sent }),
      );
      toast.success(t("board.azure-servicebus.producer.sent", { count: result.sent }));
      await entities.refresh();
    } catch (sendError) {
      toast.error(t("board.azure-servicebus.producer.failed"), {
        description: formatErrorMessage(sendError),
      });
    } finally {
      setSending(false);
    }
  }, [connID, draft, entities, sending, t]);

  // Queues and topics both, because both can be sent to. A subscription cannot:
  // it receives what its topic copies into it.
  const entityOptions = rows.map((row) => ({ value: row.name, label: row.name }));

  return (
    <Page>
      <PageHeader
        title={t("shell.nav.azure-servicebus.producer")}
        subtitle={t("board.azure-servicebus.producer.subtitle")}
      />
      <BoardState state={entities}>
        <PageBody>
          <Panel style={{ maxWidth: "760px" }}>
            <PanelHeader title={t("board.azure-servicebus.producer.title")} />
            <div style={{ padding: "0 12px 12px" }}>
              <FieldGroup>
                <Field>
                  <FieldLabel htmlFor="sb-send-entity">
                    {t("board.azure-servicebus.producer.entity")}
                  </FieldLabel>
                  {entityOptions.length > 0 ? (
                    <SelectField
                      value={draft.entity}
                      options={entityOptions}
                      onValueChange={(next) => set("entity", next)}
                    />
                  ) : (
                    <Input
                      id="sb-send-entity"
                      value={draft.entity}
                      placeholder="orders"
                      onChange={(event) => set("entity", event.target.value)}
                    />
                  )}
                  <FieldDescription>
                    {t("board.azure-servicebus.producer.entityHint")}
                  </FieldDescription>
                </Field>

                <Field>
                  <FieldLabel htmlFor="sb-send-body">
                    {t("board.azure-servicebus.producer.body")}
                  </FieldLabel>
                  <Textarea
                    id="sb-send-body"
                    rows={6}
                    value={draft.body}
                    onChange={(event) => set("body", event.target.value)}
                  />
                  <FieldDescription>
                    {t("board.azure-servicebus.producer.bodyHint")}
                  </FieldDescription>
                </Field>

                <Field>
                  <FieldLabel htmlFor="sb-send-count">
                    {t("board.azure-servicebus.producer.count")}
                  </FieldLabel>
                  <Input
                    id="sb-send-count"
                    type="number"
                    value={String(draft.count)}
                    onChange={(event) => set("count", numberField(event.target.value))}
                  />
                  <FieldDescription>
                    {t("board.azure-servicebus.producer.countHint", { max: MAX_COUNT })}
                  </FieldDescription>
                </Field>

                <Field>
                  <FieldLabel htmlFor="sb-send-subject">
                    {t("board.azure-servicebus.producer.subject")}
                  </FieldLabel>
                  <Input
                    id="sb-send-subject"
                    className="mono3"
                    value={draft.subject}
                    placeholder="order"
                    onChange={(event) => set("subject", event.target.value)}
                  />
                  <FieldDescription>
                    {t("board.azure-servicebus.producer.subjectHint")}
                  </FieldDescription>
                </Field>

                <Field>
                  <FieldLabel htmlFor="sb-send-correlation">
                    {t("board.azure-servicebus.producer.correlationId")}
                  </FieldLabel>
                  <Input
                    id="sb-send-correlation"
                    className="mono3"
                    value={draft.correlationId}
                    onChange={(event) => set("correlationId", event.target.value)}
                  />
                  <FieldDescription>
                    {t("board.azure-servicebus.producer.correlationIdHint")}
                  </FieldDescription>
                </Field>

                <Field>
                  <FieldLabel htmlFor="sb-send-session">
                    {t("board.azure-servicebus.producer.sessionId")}
                  </FieldLabel>
                  <Input
                    id="sb-send-session"
                    className="mono3"
                    value={draft.sessionId}
                    placeholder="customer-1"
                    onChange={(event) => set("sessionId", event.target.value)}
                  />
                  <FieldDescription>
                    {t("board.azure-servicebus.producer.sessionIdHint")}
                  </FieldDescription>
                </Field>

                <Field>
                  <FieldLabel htmlFor="sb-send-delay">
                    {t("board.azure-servicebus.producer.delay")}
                  </FieldLabel>
                  <Input
                    id="sb-send-delay"
                    type="number"
                    value={String(draft.delaySec)}
                    onChange={(event) => set("delaySec", numberField(event.target.value))}
                  />
                  <FieldDescription>
                    {t("board.azure-servicebus.producer.delayHint")}
                  </FieldDescription>
                </Field>
              </FieldGroup>

              <SectionLabel>{t("board.azure-servicebus.producer.properties")}</SectionLabel>
              <p style={{ fontSize: "11px", color: "var(--c-muted)", margin: "0 0 8px" }}>
                {t("board.azure-servicebus.producer.propertiesHint")}
              </p>
              {draft.properties.map((row, index) => (
                <div key={index} style={{ display: "flex", gap: "6px", marginBottom: "6px" }}>
                  <Input
                    className="mono3"
                    style={{ flex: 1 }}
                    value={row.name}
                    placeholder={t("board.azure-servicebus.producer.propertyName")}
                    onChange={(event) =>
                      set(
                        "properties",
                        draft.properties.map((current, at) =>
                          at === index ? { ...current, name: event.target.value } : current,
                        ),
                      )
                    }
                  />
                  <Input
                    className="mono3"
                    style={{ flex: 2 }}
                    value={row.value}
                    placeholder={t("board.azure-servicebus.producer.propertyValue")}
                    onChange={(event) =>
                      set(
                        "properties",
                        draft.properties.map((current, at) =>
                          at === index ? { ...current, value: event.target.value } : current,
                        ),
                      )
                    }
                  />
                  <Button
                    size="sm"
                    variant="outline"
                    aria-label={t("common.delete")}
                    onClick={() =>
                      set(
                        "properties",
                        draft.properties.filter((_, at) => at !== index),
                      )
                    }
                  >
                    <X size={12} aria-hidden />
                  </Button>
                </div>
              ))}
              <Button
                size="sm"
                variant="outline"
                onClick={() => set("properties", [...draft.properties, { name: "", value: "" }])}
              >
                <Plus size={12} aria-hidden />
                {t("board.azure-servicebus.producer.addProperty")}
              </Button>

              <div
                style={{ display: "flex", gap: "8px", alignItems: "center", marginTop: "14px" }}
              >
                <Button disabled={problem != null || sending} onClick={() => void send()}>
                  {sending ? <Spinner className="size-3.5" /> : <Send size={12} aria-hidden />}
                  {t("board.azure-servicebus.producer.send")}
                </Button>
                {problem != null && (
                  <span style={{ fontSize: "11px", color: "var(--c-muted)" }}>{t(problem)}</span>
                )}
                {problem == null && warning != null && (
                  <span style={{ fontSize: "11px", color: "var(--c-warn-text)" }}>
                    {t(warning)}
                  </span>
                )}
                {problem == null && warning == null && sent != null && (
                  <span style={{ fontSize: "11px", color: "var(--c-muted)" }}>{sent}</span>
                )}
              </div>
            </div>
          </Panel>
        </PageBody>
      </BoardState>
    </Page>
  );
}
