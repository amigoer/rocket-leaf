import { useCallback, useState } from "react";
import { useTranslation } from "react-i18next";
import { Send } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Field, FieldDescription, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Spinner } from "@/components/ui/spinner";
import { Panel, PanelHeader, SelectField, toast } from "@/components";
import { Page, PageBody, PageHeader } from "@/design/shell";
import { BoardState } from "@/design/boards/BoardState";
import { useConnectionScope } from "@/mq/ConnectionScope";
import { useKinesisDestinations } from "@/hooks/kinesis/useKinesisDestinations";
import { useKinesisShards } from "@/hooks/kinesis/useKinesisShards";
import {
  MAX_COUNT,
  emptyKinesisProducerDraft,
  sendProblem,
  sendWarning,
  toPublishInput,
  type KinesisProducerDraft,
} from "@/mq/kinesis/publish";
import { formatErrorMessage } from "@/lib/utils";
import * as kinesisApi from "@/api/kinesis";

function numberField(raw: string): number {
  const parsed = Number.parseInt(raw, 10);
  return Number.isNaN(parsed) ? 0 : parsed;
}

/**
 * The Kinesis send console.
 *
 * Its own board rather than the shared one, because the shared one collects a
 * topic, tags, keys and a delay level - RocketMQ's vocabulary, of which only
 * the destination means anything here. A Kinesis record is bytes and a
 * partition key: there is no header table, no tag, and nothing anywhere in the
 * service that holds a record back, so three of those four fields would
 * collect something that could not be sent.
 *
 * The partition key is required, and the field below it is the one no other
 * console in this app has. A partition key is hashed to choose a shard; an
 * explicit hash key replaces that hash with one the sender picks, which is the
 * only way to aim a record at a named shard. The shard picker fills it in from
 * the shards page's own numbers, because typing a 39-digit integer by hand is
 * not a thing anyone should have to do.
 */
export function ProducerKinesis() {
  const { t } = useTranslation();
  const { id: connID } = useConnectionScope();

  const streams = useKinesisDestinations();
  const [draft, setDraft] = useState<KinesisProducerDraft>(emptyKinesisProducerDraft);
  const [sending, setSending] = useState(false);
  const [sent, setSent] = useState<string | null>(null);

  const set = <K extends keyof KinesisProducerDraft>(
    key: K,
    value: KinesisProducerDraft[K],
  ) => setDraft((previous) => ({ ...previous, [key]: value }));

  const problem = sendProblem(draft);
  const warning = sendWarning(draft);
  const shards = useKinesisShards(draft.stream === "" ? null : draft.stream);

  const send = useCallback(async () => {
    const input = toPublishInput(draft);
    if (input == null || sending) return;
    setSending(true);
    try {
      const result = await kinesisApi.publish(connID, input);
      setSent(
        t("board.kinesis.producer.sentAs", {
          count: result.sent,
          shard: result.shardId,
          sequence: result.sequenceNumber,
        }),
      );
      toast.success(t("board.kinesis.producer.sent", { count: result.sent }));
      await streams.refresh();
    } catch (sendError) {
      toast.error(t("board.kinesis.producer.failed"), {
        description: formatErrorMessage(sendError),
      });
    } finally {
      setSending(false);
    }
  }, [connID, draft, sending, streams, t]);

  const streamOptions = (streams.data ?? []).map((entry) => ({
    value: entry.ref.name,
    label: entry.ref.name,
  }));

  // Only the open shards. A closed one takes no writes, so aiming at it would
  // be a control that reliably fails.
  const shardOptions = [
    { value: "", label: t("board.kinesis.producer.byPartitionKey") },
    ...shards.shards
      .filter((shard) => !shard.closed)
      .map((shard) => ({ value: shard.startHashKey, label: shard.id })),
  ];

  return (
    <Page>
      <PageHeader
        title={t("shell.nav.kinesis.producer")}
        subtitle={t("board.kinesis.producer.subtitle")}
      />
      <BoardState state={streams}>
        <PageBody>
          <Panel style={{ maxWidth: "760px" }}>
            <PanelHeader title={t("board.kinesis.producer.title")} />
            <div style={{ padding: "0 12px 12px" }}>
              <FieldGroup>
                <Field>
                  <FieldLabel htmlFor="kinesis-send-stream">
                    {t("board.kinesis.producer.stream")}
                  </FieldLabel>
                  {streamOptions.length > 0 ? (
                    <SelectField
                      value={draft.stream}
                      options={streamOptions}
                      onValueChange={(next) => set("stream", next)}
                    />
                  ) : (
                    <Input
                      id="kinesis-send-stream"
                      value={draft.stream}
                      placeholder="orders"
                      onChange={(event) => set("stream", event.target.value)}
                    />
                  )}
                  <FieldDescription>{t("board.kinesis.producer.streamHint")}</FieldDescription>
                </Field>

                <Field>
                  <FieldLabel htmlFor="kinesis-send-body">
                    {t("board.kinesis.producer.body")}
                  </FieldLabel>
                  <Textarea
                    id="kinesis-send-body"
                    rows={6}
                    value={draft.body}
                    onChange={(event) => set("body", event.target.value)}
                  />
                  <FieldDescription>{t("board.kinesis.producer.bodyHint")}</FieldDescription>
                </Field>

                <Field>
                  <FieldLabel htmlFor="kinesis-send-key">
                    {t("board.kinesis.producer.partitionKey")}
                  </FieldLabel>
                  <Input
                    id="kinesis-send-key"
                    className="mono3"
                    value={draft.partitionKey}
                    placeholder="orders-1"
                    onChange={(event) => set("partitionKey", event.target.value)}
                  />
                  <FieldDescription>
                    {t("board.kinesis.producer.partitionKeyHint")}
                  </FieldDescription>
                </Field>

                <Field>
                  <FieldLabel htmlFor="kinesis-send-shard">
                    {t("board.kinesis.producer.aimAtShard")}
                  </FieldLabel>
                  <SelectField
                    value={draft.explicitHashKey}
                    options={shardOptions}
                    onValueChange={(next) => set("explicitHashKey", next)}
                  />
                  <FieldDescription>{t("board.kinesis.producer.aimAtShardHint")}</FieldDescription>
                </Field>

                <Field>
                  <FieldLabel htmlFor="kinesis-send-count">
                    {t("board.kinesis.producer.count")}
                  </FieldLabel>
                  <Input
                    id="kinesis-send-count"
                    type="number"
                    value={String(draft.count)}
                    onChange={(event) => set("count", numberField(event.target.value))}
                  />
                  <FieldDescription>
                    {t("board.kinesis.producer.countHint", { max: MAX_COUNT })}
                  </FieldDescription>
                </Field>
              </FieldGroup>

              {warning != null && (
                <p style={{ marginTop: "10px", fontSize: "11px", color: "var(--c-warn)" }}>
                  {t(warning)}
                </p>
              )}

              <div
                style={{ display: "flex", gap: "8px", alignItems: "center", marginTop: "12px" }}
              >
                <Button disabled={problem != null || sending} onClick={() => void send()}>
                  {sending ? <Spinner className="size-3.5" /> : <Send size={13} aria-hidden />}
                  {t("board.kinesis.producer.send")}
                </Button>
                {problem != null && (
                  <span style={{ fontSize: "11px", color: "var(--c-muted)" }}>{t(problem)}</span>
                )}
              </div>

              {/* The pair, because neither half addresses a record on its own -
                  which is the same thing the messages page says about an id. */}
              {sent != null && (
                <p style={{ marginTop: "10px", fontSize: "11px", color: "var(--c-muted)" }}>
                  {sent}
                </p>
              )}
            </div>
          </Panel>
        </PageBody>
      </BoardState>
    </Page>
  );
}
