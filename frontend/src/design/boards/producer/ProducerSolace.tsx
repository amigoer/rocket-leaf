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
import { useSolaceDestinations } from "@/hooks/solace/useSolaceDestinations";
import { destination as readDestination } from "@/mq/solace/destinations";
import {
  CONTENT_TYPES,
  DELIVERY_MODES,
  MAX_COUNT,
  TARGETS,
  emptySolaceProducerDraft,
  sendProblem,
  toPublishInput,
  type SendTarget,
  type SolaceProducerDraft,
} from "@/mq/solace/publish";
import { formatErrorMessage } from "@/lib/utils";
import * as solaceApi from "@/api/solace";

/**
 * The Solace send console.
 *
 * Its own board rather than the shared one, because the shared one collects a
 * topic, tags, keys and a delay level - RocketMQ's vocabulary, of which only
 * the destination means anything here. What a Solace message carries instead
 * is a delivery mode that decides whether the broker writes it to the spool at
 * all, a time to live, and the flag that decides whether it is moved or
 * discarded when it is given up on.
 *
 * The target is a choice rather than something derived from the name, and that
 * is the field this board exists for. A queue and a topic are routinely called
 * the same thing - "orders/eu" is an ordinary name for either - and the two
 * sends do different things: one names an endpoint, the other is matched
 * against every subscription in the Message VPN and lands nowhere at all when
 * none match. A console that guessed would sometimes be silently right and
 * sometimes silently wrong.
 *
 * Dead-message eligibility is offered because the broker's default is off, and
 * that default is why a queue configured with a dead message queue can still
 * discard quietly: unless the publisher marks the message, or the queue is set
 * to ignore the mark, nothing is ever moved.
 *
 * Nothing here reports a message id, and the field is absent rather than
 * empty: the broker answers a send it took with no identifier of any kind, so
 * what comes back is a count.
 */
export function ProducerSolace() {
  const { t } = useTranslation();
  const { id: connID } = useConnectionScope();

  const destinations = useSolaceDestinations();
  const [draft, setDraft] = useState<SolaceProducerDraft>(emptySolaceProducerDraft);
  const [sending, setSending] = useState(false);

  const set = <K extends keyof SolaceProducerDraft>(key: K, value: SolaceProducerDraft[K]) =>
    setDraft((previous) => ({ ...previous, [key]: value }));

  const queues = useMemo(
    () => (destinations.data ?? []).map(readDestination).map((entry) => entry.name),
    [destinations.data],
  );

  const problem = sendProblem(draft);

  const send = useCallback(async () => {
    const input = toPublishInput(draft);
    if (input == null || sending) return;
    setSending(true);
    try {
      const result = await solaceApi.publish(connID, input);
      toast.success(t("board.solace.producer.sent", { count: result.sent }));
      await destinations.refresh();
    } catch (sendError) {
      toast.error(t("board.solace.producer.failed"), {
        description: formatErrorMessage(sendError),
      });
    } finally {
      setSending(false);
    }
  }, [connID, destinations, draft, sending, t]);

  return (
    <Page>
      <PageHeader
        title={t("board.solace.producer.title")}
        subtitle={t("board.solace.producer.subtitle")}
      />
      <BoardState state={destinations}>
        <PageBody>
          <Panel style={{ maxWidth: "620px" }}>
            <PanelHeader title={t("board.solace.producer.title")} />
            <div style={{ padding: "12px" }}>
              <FieldGroup>
                <Field>
                  <FieldLabel htmlFor="solace-send-target">
                    {t("board.solace.producer.target")}
                  </FieldLabel>
                  <SelectField<SendTarget>
                    value={draft.target}
                    options={TARGETS.map((value) => ({
                      value,
                      label: t(`board.solace.producer.target_${value}`),
                    }))}
                    onValueChange={(next) => set("target", next)}
                  />
                  <FieldDescription>
                    {draft.target === "queue"
                      ? t("board.solace.producer.targetQueueHint")
                      : t("board.solace.producer.targetTopicHint")}
                  </FieldDescription>
                </Field>

                <Field>
                  <FieldLabel htmlFor="solace-send-destination">
                    {t("board.solace.producer.destination")}
                  </FieldLabel>
                  {draft.target === "queue" ? (
                    <SelectField<string>
                      value={draft.destination}
                      options={queues.map((name) => ({ value: name, label: name }))}
                      placeholder={t("board.solace.producer.pickQueue")}
                      onValueChange={(next) => set("destination", next)}
                    />
                  ) : (
                    <Input
                      id="solace-send-destination"
                      className="mono3"
                      value={draft.destination}
                      placeholder="orders/eu/created"
                      onChange={(event) => set("destination", event.target.value)}
                    />
                  )}
                  <FieldDescription>
                    {draft.target === "queue"
                      ? t("board.solace.producer.destinationQueueHint")
                      : t("board.solace.producer.destinationTopicHint")}
                  </FieldDescription>
                </Field>

                <Field>
                  <FieldLabel htmlFor="solace-send-body">
                    {t("board.solace.producer.body")}
                  </FieldLabel>
                  <Textarea
                    id="solace-send-body"
                    className="mono3"
                    rows={7}
                    value={draft.body}
                    onChange={(event) => set("body", event.target.value)}
                  />
                </Field>

                <Field>
                  <FieldLabel htmlFor="solace-send-content-type">
                    {t("board.solace.producer.contentType")}
                  </FieldLabel>
                  <SelectField<string>
                    value={draft.contentType}
                    options={CONTENT_TYPES.map((value) => ({ value, label: value }))}
                    onValueChange={(next) => set("contentType", next)}
                  />
                </Field>

                <Field>
                  <FieldLabel htmlFor="solace-send-delivery">
                    {t("board.solace.producer.deliveryMode")}
                  </FieldLabel>
                  <SelectField<string>
                    value={draft.deliveryMode}
                    options={DELIVERY_MODES.map((value) => ({ value, label: value }))}
                    onValueChange={(next) => set("deliveryMode", next)}
                  />
                  <FieldDescription>
                    {t("board.solace.producer.deliveryModeHint")}
                  </FieldDescription>
                </Field>

                <Field>
                  <FieldLabel htmlFor="solace-send-ttl">
                    {t("board.solace.producer.ttl")}
                  </FieldLabel>
                  <Input
                    id="solace-send-ttl"
                    type="number"
                    value={draft.timeToLiveMs}
                    placeholder="0"
                    onChange={(event) => set("timeToLiveMs", event.target.value)}
                  />
                  <FieldDescription>{t("board.solace.producer.ttlHint")}</FieldDescription>
                </Field>

                <Field orientation="horizontal">
                  <Switch
                    id="solace-send-dmq"
                    checked={draft.dmqEligible}
                    onCheckedChange={(next: boolean) => set("dmqEligible", next)}
                  />
                  <div>
                    <FieldLabel htmlFor="solace-send-dmq">
                      {t("board.solace.producer.dmqEligible")}
                    </FieldLabel>
                    <FieldDescription>
                      {t("board.solace.producer.dmqEligibleHint")}
                    </FieldDescription>
                  </div>
                </Field>

                <Field>
                  <FieldLabel htmlFor="solace-send-correlation">
                    {t("board.solace.producer.correlationId")}
                  </FieldLabel>
                  <Input
                    id="solace-send-correlation"
                    className="mono3"
                    value={draft.correlationId}
                    onChange={(event) => set("correlationId", event.target.value)}
                  />
                </Field>

                <Field>
                  <FieldLabel htmlFor="solace-send-reply-to">
                    {t("board.solace.producer.replyTo")}
                  </FieldLabel>
                  <Input
                    id="solace-send-reply-to"
                    className="mono3"
                    value={draft.replyTo}
                    placeholder="/QUEUE/replies"
                    onChange={(event) => set("replyTo", event.target.value)}
                  />
                  <FieldDescription>{t("board.solace.producer.replyToHint")}</FieldDescription>
                </Field>

                <Field>
                  <FieldLabel htmlFor="solace-send-count">
                    {t("board.solace.producer.count")}
                  </FieldLabel>
                  <Input
                    id="solace-send-count"
                    type="number"
                    value={draft.count}
                    onChange={(event) => set("count", event.target.value)}
                  />
                  <FieldDescription>
                    {t("board.solace.producer.countHint", { max: MAX_COUNT })}
                  </FieldDescription>
                </Field>

                {problem != null && (
                  <FieldDescription style={{ color: "var(--c-danger)" }}>
                    {t(`board.solace.producer.problem.${problem}`)}
                  </FieldDescription>
                )}

                <div style={{ display: "flex", gap: "8px", alignItems: "center" }}>
                  <Button disabled={problem != null || sending} onClick={() => void send()}>
                    {sending ? <Spinner className="size-3.5" /> : <Send size={13} aria-hidden />}
                    {t("board.solace.producer.send")}
                  </Button>
                  <span style={{ fontSize: "11px", color: "var(--c-muted)" }}>
                    {t("board.solace.producer.noId")}
                  </span>
                </div>
              </FieldGroup>
            </div>
          </Panel>
        </PageBody>
      </BoardState>
    </Page>
  );
}
