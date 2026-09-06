import { useCallback, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { Send } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Field, FieldDescription, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";
import { Spinner } from "@/components/ui/spinner";
import { Panel, PanelHeader, SelectField, toast } from "@/components";
import { Page, PageBody, PageHeader } from "@/design/shell";
import { BoardState } from "@/design/boards/BoardState";
import { useConnectionScope } from "@/mq/ConnectionScope";
import { useIbmMqDestinations } from "@/hooks/ibmmq/useIbmMqDestinations";
import { destination as readDestination } from "@/mq/ibmmq/destinations";
import {
  CONTENT_TYPES,
  MAX_COUNT,
  emptyIbmMqProducerDraft,
  sendProblem,
  toPublishInput,
  type IbmMqProducerDraft,
} from "@/mq/ibmmq/publish";
import { formatErrorMessage } from "@/lib/utils";
import * as ibmmqApi from "@/api/ibmmq";

/**
 * The IBM MQ send console.
 *
 * Its own board rather than the shared one, because the shared one collects a
 * topic, tags, keys and a delay level - RocketMQ's vocabulary, of which only
 * the destination means anything here. What an MQ message carries instead is a
 * descriptor: a correlation identifier that matches a reply to its request, a
 * persistence that decides whether it survives a restart, and an expiry.
 *
 * The destination list holds queues only, and that is the interface rather
 * than a filter this board applies for tidiness: the messaging REST API has no
 * topic resource at any version, so publishing needs an MQ client and offering
 * a topic here would be offering a send that cannot happen.
 *
 * Persistence is a switch rather than a default because both answers are
 * ordinary. A non-persistent message is faster and is gone if the queue
 * manager restarts; a persistent one is written to the log. The console
 * defaults to non-persistent, which is the honest default for a message
 * somebody is testing with.
 */
export function ProducerIbmMq() {
  const { t } = useTranslation();
  const { id: connID } = useConnectionScope();

  const destinations = useIbmMqDestinations();
  const [draft, setDraft] = useState<IbmMqProducerDraft>(emptyIbmMqProducerDraft);
  const [sending, setSending] = useState(false);
  const [sent, setSent] = useState<string | null>(null);

  const set = <K extends keyof IbmMqProducerDraft>(key: K, value: IbmMqProducerDraft[K]) =>
    setDraft((previous) => ({ ...previous, [key]: value }));

  const queues = useMemo(
    () =>
      (destinations.data ?? [])
        .map(readDestination)
        .filter((entry) => entry.kind === "queue" && entry.queueType === "local")
        .map((entry) => entry.name),
    [destinations.data],
  );

  const problem = sendProblem(draft);

  const send = useCallback(async () => {
    const input = toPublishInput(draft);
    if (input == null || sending) return;
    setSending(true);
    try {
      const result = await ibmmqApi.publish(connID, input);
      setSent(t("board.ibmmq.producer.sentAs", { id: result.messageId }));
      toast.success(t("board.ibmmq.producer.sent", { count: result.sent }));
      await destinations.refresh();
    } catch (sendError) {
      setSent(null);
      toast.error(t("board.ibmmq.producer.failed"), {
        description: formatErrorMessage(sendError),
      });
    } finally {
      setSending(false);
    }
  }, [connID, destinations, draft, sending, t]);

  return (
    <Page>
      <PageHeader
        title={t("board.ibmmq.producer.title")}
        subtitle={t("board.ibmmq.producer.subtitle")}
      />
      <BoardState state={destinations}>
        <PageBody>
          <Panel style={{ maxWidth: "620px" }}>
            <PanelHeader title={t("board.ibmmq.producer.title")} />
            <div style={{ padding: "12px" }}>
              <FieldGroup>
                <Field>
                  <FieldLabel htmlFor="ibmmq-send-queue">
                    {t("board.ibmmq.producer.queue")}
                  </FieldLabel>
                  <SelectField<string>
                    value={draft.queue}
                    options={queues.map((name) => ({ value: name, label: name }))}
                    placeholder={t("board.ibmmq.producer.pickQueue")}
                    onValueChange={(next) => set("queue", next)}
                  />
                  <FieldDescription>{t("board.ibmmq.producer.queueHint")}</FieldDescription>
                </Field>

                <Field>
                  <FieldLabel htmlFor="ibmmq-send-body">
                    {t("board.ibmmq.producer.body")}
                  </FieldLabel>
                  <Textarea
                    id="ibmmq-send-body"
                    rows={6}
                    value={draft.body}
                    onChange={(event) => set("body", event.target.value)}
                  />
                  <FieldDescription>{t("board.ibmmq.producer.bodyHint")}</FieldDescription>
                </Field>

                <Field>
                  <FieldLabel htmlFor="ibmmq-send-content-type">
                    {t("board.ibmmq.producer.contentType")}
                  </FieldLabel>
                  <SelectField<string>
                    value={draft.contentType}
                    options={CONTENT_TYPES.map((value) => ({ value, label: value }))}
                    onValueChange={(next) => set("contentType", next)}
                  />
                </Field>

                <Field>
                  <FieldLabel htmlFor="ibmmq-send-persistent">
                    {t("board.ibmmq.producer.persistent")}
                  </FieldLabel>
                  <Switch
                    id="ibmmq-send-persistent"
                    checked={draft.persistent}
                    onCheckedChange={(next: boolean) => set("persistent", next)}
                  />
                  <FieldDescription>{t("board.ibmmq.producer.persistentHint")}</FieldDescription>
                </Field>

                <Field>
                  <FieldLabel htmlFor="ibmmq-send-correlation">
                    {t("board.ibmmq.producer.correlationId")}
                  </FieldLabel>
                  <Input
                    id="ibmmq-send-correlation"
                    className="mono3"
                    value={draft.correlationId}
                    onChange={(event) => set("correlationId", event.target.value)}
                  />
                  <FieldDescription>
                    {t("board.ibmmq.producer.correlationIdHint")}
                  </FieldDescription>
                </Field>

                <Field>
                  <FieldLabel htmlFor="ibmmq-send-expiry">
                    {t("board.ibmmq.producer.expiry")}
                  </FieldLabel>
                  <Input
                    id="ibmmq-send-expiry"
                    type="number"
                    value={draft.expirySeconds}
                    onChange={(event) => set("expirySeconds", event.target.value)}
                  />
                  <FieldDescription>{t("board.ibmmq.producer.expiryHint")}</FieldDescription>
                </Field>

                <Field>
                  <FieldLabel htmlFor="ibmmq-send-count">
                    {t("board.ibmmq.producer.count")}
                  </FieldLabel>
                  <Input
                    id="ibmmq-send-count"
                    type="number"
                    value={draft.count}
                    onChange={(event) => set("count", event.target.value)}
                  />
                  <FieldDescription>
                    {t("board.ibmmq.producer.countHint", { max: MAX_COUNT })}
                  </FieldDescription>
                </Field>

                {problem != null && (
                  <FieldDescription style={{ color: "var(--c-warn)" }}>
                    {t(`board.ibmmq.producer.problem.${problem}`)}
                  </FieldDescription>
                )}
                {sent != null && <FieldDescription>{sent}</FieldDescription>}

                <div>
                  <Button disabled={problem != null || sending} onClick={() => void send()}>
                    {sending ? <Spinner className="size-3.5" /> : <Send size={13} aria-hidden />}
                    {t("board.ibmmq.producer.send")}
                  </Button>
                </div>
              </FieldGroup>
            </div>
          </Panel>
          <div style={{ fontSize: "11px", color: "var(--c-muted)", paddingTop: "8px" }}>
            {t("board.ibmmq.producer.note")}
          </div>
        </PageBody>
      </BoardState>
    </Page>
  );
}
