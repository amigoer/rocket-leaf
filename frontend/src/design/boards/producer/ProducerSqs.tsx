import { useCallback, useState } from "react";
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
import { useSqsDestinations } from "@/hooks/sqs/useSqsDestinations";
import {
  MAX_COUNT,
  MAX_DELAY_SEC,
  emptySqsProducerDraft,
  sendProblem,
  sendWarning,
  targetsFifo,
  toPublishInput,
  type SqsProducerDraft,
} from "@/mq/sqs/publish";
import { formatErrorMessage } from "@/lib/utils";
import * as sqsApi from "@/api/sqs";

function numberField(raw: string): number {
  const parsed = Number.parseInt(raw, 10);
  return Number.isNaN(parsed) ? 0 : parsed;
}

/**
 * The SQS send console.
 *
 * Its own board rather than the shared one, because the shared one collects a
 * topic, tags, keys and a delay level - RocketMQ's vocabulary, of which only
 * the destination and the delay mean anything here. An SQS message is a body
 * and a table of named attributes, so a tag field would collect something that
 * could not be sent.
 *
 * The two fields no other console here has are the FIFO ones, and they appear
 * and disappear with the queue's name. That is not a nicety: SQS refuses a
 * FIFO send with no group id and a standard send with one, and names
 * MessageGroupId in both answers - so half the people reading the service's
 * own message would be sent to the wrong field.
 */
export function ProducerSqs() {
  const { t } = useTranslation();
  const { id: connID } = useConnectionScope();

  const queues = useSqsDestinations();
  const [draft, setDraft] = useState<SqsProducerDraft>(emptySqsProducerDraft);
  const [sending, setSending] = useState(false);
  const [sent, setSent] = useState<string | null>(null);

  const set = <K extends keyof SqsProducerDraft>(key: K, value: SqsProducerDraft[K]) =>
    setDraft((previous) => ({ ...previous, [key]: value }));

  const problem = sendProblem(draft);
  const warning = sendWarning(draft);
  const fifo = targetsFifo(draft);

  const send = useCallback(async () => {
    const input = toPublishInput(draft);
    if (input == null || sending) return;
    setSending(true);
    try {
      const result = await sqsApi.publish(connID, input);
      setSent(t("board.sqs.producer.sentAs", { count: result.sent, id: result.messageId }));
      toast.success(t("board.sqs.producer.sent", { count: result.sent }));
      await queues.refresh();
    } catch (sendError) {
      toast.error(t("board.sqs.producer.failed"), {
        description: formatErrorMessage(sendError),
      });
    } finally {
      setSending(false);
    }
  }, [connID, draft, queues, sending, t]);

  const queueOptions = (queues.data ?? []).map((entry) => ({
    value: entry.ref.name,
    label: entry.ref.name,
  }));

  return (
    <Page>
      <PageHeader title={t("shell.nav.sqs.producer")} subtitle={t("board.sqs.producer.subtitle")} />
      <BoardState state={queues}>
        <PageBody>
          <Panel style={{ maxWidth: "760px" }}>
            <PanelHeader title={t("board.sqs.producer.title")} />
            <div style={{ padding: "0 12px 12px" }}>
              <FieldGroup>
                <Field>
                  <FieldLabel htmlFor="sqs-send-queue">{t("board.sqs.producer.queue")}</FieldLabel>
                  {queueOptions.length > 0 ? (
                    <SelectField
                      value={draft.queue}
                      options={queueOptions}
                      onValueChange={(next) => set("queue", next)}
                    />
                  ) : (
                    <Input
                      id="sqs-send-queue"
                      value={draft.queue}
                      placeholder="orders"
                      onChange={(event) => set("queue", event.target.value)}
                    />
                  )}
                  <FieldDescription>{t("board.sqs.producer.queueHint")}</FieldDescription>
                </Field>

                <Field>
                  <FieldLabel htmlFor="sqs-send-body">{t("board.sqs.producer.body")}</FieldLabel>
                  <Textarea
                    id="sqs-send-body"
                    rows={6}
                    value={draft.body}
                    onChange={(event) => set("body", event.target.value)}
                  />
                  <FieldDescription>{t("board.sqs.producer.bodyHint")}</FieldDescription>
                </Field>

                <Field>
                  <FieldLabel htmlFor="sqs-send-count">{t("board.sqs.producer.count")}</FieldLabel>
                  <Input
                    id="sqs-send-count"
                    type="number"
                    value={String(draft.count)}
                    onChange={(event) => set("count", numberField(event.target.value))}
                  />
                  <FieldDescription>
                    {t("board.sqs.producer.countHint", { max: MAX_COUNT })}
                  </FieldDescription>
                </Field>

                {/* A FIFO queue's delay is a queue setting, so the field is
                    not drawn where it cannot be used. */}
                {!fifo && (
                  <Field>
                    <FieldLabel htmlFor="sqs-send-delay">
                      {t("board.sqs.producer.delay")}
                    </FieldLabel>
                    <Input
                      id="sqs-send-delay"
                      type="number"
                      value={String(draft.delaySec)}
                      onChange={(event) => set("delaySec", numberField(event.target.value))}
                    />
                    <FieldDescription>
                      {t("board.sqs.producer.delayHint", { max: MAX_DELAY_SEC })}
                    </FieldDescription>
                  </Field>
                )}

                {fifo && (
                  <>
                    <Field>
                      <FieldLabel htmlFor="sqs-send-group">
                        {t("board.sqs.producer.groupId")}
                      </FieldLabel>
                      <Input
                        id="sqs-send-group"
                        value={draft.groupId}
                        placeholder="acme"
                        onChange={(event) => set("groupId", event.target.value)}
                      />
                      <FieldDescription>{t("board.sqs.producer.groupIdHint")}</FieldDescription>
                    </Field>
                    <Field>
                      <FieldLabel htmlFor="sqs-send-dedup">
                        {t("board.sqs.producer.deduplicationId")}
                      </FieldLabel>
                      <Input
                        id="sqs-send-dedup"
                        value={draft.deduplicationId}
                        onChange={(event) => set("deduplicationId", event.target.value)}
                      />
                      <FieldDescription>
                        {t("board.sqs.producer.deduplicationIdHint")}
                      </FieldDescription>
                    </Field>
                  </>
                )}
              </FieldGroup>

              <SectionLabel>{t("board.sqs.producer.attributes")}</SectionLabel>
              <p style={{ fontSize: "11px", color: "var(--c-muted)", margin: "0 0 8px" }}>
                {t("board.sqs.producer.attributesHint")}
              </p>
              {draft.attributes.map((row, index) => (
                <div key={index} style={{ display: "flex", gap: "6px", marginBottom: "6px" }}>
                  <Input
                    className="mono3"
                    style={{ flex: 1 }}
                    value={row.name}
                    placeholder={t("board.sqs.producer.attributeName")}
                    onChange={(event) =>
                      set(
                        "attributes",
                        draft.attributes.map((current, at) =>
                          at === index ? { ...current, name: event.target.value } : current,
                        ),
                      )
                    }
                  />
                  <Input
                    className="mono3"
                    style={{ flex: 2 }}
                    value={row.value}
                    placeholder={t("board.sqs.producer.attributeValue")}
                    onChange={(event) =>
                      set(
                        "attributes",
                        draft.attributes.map((current, at) =>
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
                        "attributes",
                        draft.attributes.filter((_, at) => at !== index),
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
                onClick={() => set("attributes", [...draft.attributes, { name: "", value: "" }])}
              >
                <Plus size={12} aria-hidden />
                {t("board.sqs.producer.addAttribute")}
              </Button>

              <div
                style={{ display: "flex", gap: "8px", alignItems: "center", marginTop: "14px" }}
              >
                <Button disabled={problem != null || sending} onClick={() => void send()}>
                  {sending ? <Spinner className="size-3.5" /> : <Send size={12} aria-hidden />}
                  {t("board.sqs.producer.send")}
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
