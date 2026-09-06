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
import { useGooglePubSubTopics } from "@/hooks/googlepubsub/useGooglePubSubTopics";
import { topic as readTopic } from "@/mq/googlepubsub/topics";
import {
  MAX_COUNT,
  emptyPubSubProducerDraft,
  sendProblem,
  sendWarning,
  toPublishInput,
  type PubSubProducerDraft,
} from "@/mq/googlepubsub/publish";
import { formatErrorMessage } from "@/lib/utils";
import * as pubsubApi from "@/api/googlepubsub";

function numberField(raw: string): number {
  const parsed = Number.parseInt(raw, 10);
  return Number.isNaN(parsed) ? 0 : parsed;
}

/**
 * The Google Pub/Sub send console.
 *
 * Its own board rather than the shared one, because the shared one collects a
 * topic, tags, keys and a delay level - RocketMQ's vocabulary, of which only
 * the topic means anything here. A Pub/Sub message is a body and a table of
 * string attributes; there is no tag, and there is no way to hold a message
 * back at all, so a delay field would collect something that could not be sent.
 *
 * The warning under the button is the one no other console here has. A topic
 * stores nothing: with no subscription attached, the publish is accepted,
 * reported as sent, and discarded - and no board anywhere afterwards records
 * that it happened, because there is no backlog to notice it by.
 */
export function ProducerGooglePubSub() {
  const { t } = useTranslation();
  const { id: connID } = useConnectionScope();

  const topics = useGooglePubSubTopics();
  const [draft, setDraft] = useState<PubSubProducerDraft>(emptyPubSubProducerDraft);
  const [sending, setSending] = useState(false);
  const [sent, setSent] = useState<string | null>(null);

  const set = <K extends keyof PubSubProducerDraft>(key: K, value: PubSubProducerDraft[K]) =>
    setDraft((previous) => ({ ...previous, [key]: value }));

  const rows = useMemo(() => (topics.data ?? []).map(readTopic), [topics.data]);
  /* Null rather than zero when the topic is not in the listing: the connection
     may be filtered by a prefix, and "not loaded" must not read as "nothing
     subscribes to it". */
  const subscribers = useMemo(() => {
    const chosen = rows.find((entry) => entry.name === draft.topic.trim());
    return chosen == null ? null : chosen.subscribers;
  }, [rows, draft.topic]);

  const problem = sendProblem(draft);
  const warning = sendWarning(draft, subscribers);

  const send = useCallback(async () => {
    const input = toPublishInput(draft);
    if (input == null || sending) return;
    setSending(true);
    try {
      const result = await pubsubApi.publish(connID, input);
      setSent(
        t("board.google-pubsub.producer.sentAs", { count: result.sent, id: result.messageId }),
      );
      toast.success(t("board.google-pubsub.producer.sent", { count: result.sent }));
      await topics.refresh();
    } catch (sendError) {
      toast.error(t("board.google-pubsub.producer.failed"), {
        description: formatErrorMessage(sendError),
      });
    } finally {
      setSending(false);
    }
  }, [connID, draft, sending, t, topics]);

  const topicOptions = rows.map((entry) => ({ value: entry.name, label: entry.name }));

  return (
    <Page>
      <PageHeader
        title={t("shell.nav.google-pubsub.producer")}
        subtitle={t("board.google-pubsub.producer.subtitle")}
      />
      <BoardState state={topics}>
        <PageBody>
          <Panel style={{ maxWidth: "760px" }}>
            <PanelHeader title={t("board.google-pubsub.producer.title")} />
            <div style={{ padding: "0 12px 12px" }}>
              <FieldGroup>
                <Field>
                  <FieldLabel htmlFor="pubsub-send-topic">
                    {t("board.google-pubsub.producer.topic")}
                  </FieldLabel>
                  {topicOptions.length > 0 ? (
                    <SelectField
                      value={draft.topic}
                      options={topicOptions}
                      onValueChange={(next) => set("topic", next)}
                    />
                  ) : (
                    <Input
                      id="pubsub-send-topic"
                      value={draft.topic}
                      placeholder="orders"
                      onChange={(event) => set("topic", event.target.value)}
                    />
                  )}
                  <FieldDescription>
                    {t("board.google-pubsub.producer.topicHint")}
                  </FieldDescription>
                </Field>

                <Field>
                  <FieldLabel htmlFor="pubsub-send-body">
                    {t("board.google-pubsub.producer.body")}
                  </FieldLabel>
                  <Textarea
                    id="pubsub-send-body"
                    rows={6}
                    value={draft.body}
                    onChange={(event) => set("body", event.target.value)}
                  />
                  <FieldDescription>{t("board.google-pubsub.producer.bodyHint")}</FieldDescription>
                </Field>

                <Field>
                  <FieldLabel htmlFor="pubsub-send-count">
                    {t("board.google-pubsub.producer.count")}
                  </FieldLabel>
                  <Input
                    id="pubsub-send-count"
                    type="number"
                    value={String(draft.count)}
                    onChange={(event) => set("count", numberField(event.target.value))}
                  />
                  <FieldDescription>
                    {t("board.google-pubsub.producer.countHint", { max: MAX_COUNT })}
                  </FieldDescription>
                </Field>

                <Field>
                  <FieldLabel htmlFor="pubsub-send-ordering">
                    {t("board.google-pubsub.producer.orderingKey")}
                  </FieldLabel>
                  <Input
                    id="pubsub-send-ordering"
                    className="mono3"
                    value={draft.orderingKey}
                    placeholder="customer-1"
                    onChange={(event) => set("orderingKey", event.target.value)}
                  />
                  <FieldDescription>
                    {t("board.google-pubsub.producer.orderingKeyHint")}
                  </FieldDescription>
                </Field>
              </FieldGroup>

              <SectionLabel>{t("board.google-pubsub.producer.attributes")}</SectionLabel>
              <p style={{ fontSize: "11px", color: "var(--c-muted)", margin: "0 0 8px" }}>
                {t("board.google-pubsub.producer.attributesHint")}
              </p>
              {draft.attributes.map((row, index) => (
                <div key={index} style={{ display: "flex", gap: "6px", marginBottom: "6px" }}>
                  <Input
                    className="mono3"
                    style={{ flex: 1 }}
                    value={row.name}
                    placeholder={t("board.google-pubsub.producer.attributeName")}
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
                    placeholder={t("board.google-pubsub.producer.attributeValue")}
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
                {t("board.google-pubsub.producer.addAttribute")}
              </Button>

              <div
                style={{ display: "flex", gap: "8px", alignItems: "center", marginTop: "14px" }}
              >
                <Button disabled={problem != null || sending} onClick={() => void send()}>
                  {sending ? <Spinner className="size-3.5" /> : <Send size={12} aria-hidden />}
                  {t("board.google-pubsub.producer.send")}
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
