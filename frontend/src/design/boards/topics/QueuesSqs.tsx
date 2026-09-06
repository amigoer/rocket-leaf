import { useCallback, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { Page, PageBody, PageHeader, RefreshButton } from "@/design/shell";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { KV, Panel, PanelHeader, SectionLabel, useConfirm } from "@/components";
import { BoardState } from "@/design/boards/BoardState";
import { useSqsDestinations } from "@/hooks/sqs/useSqsDestinations";
import { queue as readQueue, stalledInFlight, type SqsQueue } from "@/mq/sqs/destinations";
import { formatBytes, formatCount } from "@/lib/format";
import { formatMessageTime } from "@/lib/time";
import { formatErrorMessage } from "@/lib/utils";
import { useConnectionScope } from "@/mq/ConnectionScope";
import * as sqsApi from "@/api/sqs";
import { QueueDialogSqs } from "./QueueDialogSqs";

const MONO11 = { fontSize: "11px" } as const;

/** A dash, not a zero: an absent figure and a measured zero are not alike. */
const DASH = "—";

function count(value: number | null): string {
  return value == null ? DASH : formatCount(value);
}

/** Seconds as the service stores them, read back as something human. */
function duration(seconds: number | null): string {
  if (seconds == null) return DASH;
  if (seconds < 60) return `${seconds}s`;
  if (seconds < 3600) return `${Math.round(seconds / 60)}m`;
  if (seconds < 86400) return `${Math.round(seconds / 3600)}h`;
  return `${Math.round(seconds / 86400)}d`;
}

/**
 * Amazon SQS queues.
 *
 * One row per queue in the region, narrowed by the connection's prefix. There
 * is no second object anywhere in SQS - no topic, no subscription, no consumer
 * group - so this board is the whole of the family's topology.
 *
 * Three counts rather than one, because they mean three different things and a
 * single depth hides the one worth acting on. Available is what a consumer
 * would be handed now; in flight is what has been handed out and not deleted,
 * so a queue with everything in flight has a consumer that is not finishing;
 * delayed is what is not due yet, which is a queue behaving exactly as asked.
 *
 * There is no rate column and no chart. SQS publishes its rates to CloudWatch,
 * which is a different API under a different permission, and a figure derived
 * from two samples here would be this app's arithmetic presented as AWS's.
 */
export function QueuesSqs() {
  const { t } = useTranslation();
  const state = useSqsDestinations();
  const { id: connID } = useConnectionScope();
  const confirm = useConfirm();
  const [search, setSearch] = useState("");
  const [selected, setSelected] = useState<string | null>(null);
  const [editing, setEditing] = useState<SqsQueue | null>(null);
  const [formOpen, setFormOpen] = useState(false);

  const queues = useMemo(() => (state.data ?? []).map(readQueue), [state.data]);

  const shown = useMemo(() => {
    const needle = search.trim().toLowerCase();
    return queues.filter((entry) => needle === "" || entry.name.toLowerCase().includes(needle));
  }, [queues, search]);

  const detail = useMemo(
    () => shown.find((entry) => entry.name === selected) ?? shown[0] ?? null,
    [shown, selected],
  );

  const save = useCallback(
    async (input: sqsApi.SQSQueueInput) => {
      if (editing == null) {
        await sqsApi.createQueue(connID, input);
        toast.success(t("board.sqs.queues.created", { name: input.name }));
      } else {
        await sqsApi.updateQueue(connID, input);
        toast.success(t("board.sqs.queues.updated", { name: input.name }));
      }
      await state.refresh();
    },
    [connID, editing, state, t],
  );

  /*
   * The confirmation names the count because that is what is about to go, and
   * the note says what the service will not: the call returning is not the
   * queue being empty, and anything sent in the following minute may go with
   * it.
   */
  const purge = useCallback(
    async (entry: SqsQueue) => {
      const ok = await confirm({
        title: t("board.sqs.queues.purgeTitle", { name: entry.name }),
        description: t("board.sqs.queues.purgeDesc", { count: entry.depth ?? 0 }),
        confirmLabel: t("board.sqs.queues.purgeAction"),
        danger: true,
      });
      if (!ok) return;
      try {
        await sqsApi.purgeQueue(connID, entry.name);
        toast.success(t("board.sqs.queues.purged", { name: entry.name }));
        await state.refresh();
      } catch (purgeError) {
        toast.error(t("board.sqs.queues.purgeFailed"), {
          description: formatErrorMessage(purgeError),
        });
      }
    },
    [confirm, connID, state, t],
  );

  const remove = useCallback(
    async (entry: SqsQueue) => {
      const ok = await confirm({
        title: t("board.sqs.queues.deleteTitle", { name: entry.name }),
        description: t("board.sqs.queues.deleteDesc"),
        confirmLabel: t("common.delete"),
        danger: true,
      });
      if (!ok) return;
      try {
        await sqsApi.removeQueue(connID, entry.name);
        toast.success(t("board.sqs.queues.deleted", { name: entry.name }));
        setSelected(null);
        await state.refresh();
      } catch (deleteError) {
        toast.error(t("board.sqs.queues.deleteFailed"), {
          description: formatErrorMessage(deleteError),
        });
      }
    },
    [confirm, connID, state, t],
  );

  return (
    <Page>
      <PageHeader
        title={t("shell.nav.sqs.topics")}
        subtitle={t("board.sqs.queues.count", { count: queues.length })}
        actions={
          <div style={{ display: "flex", gap: "8px", alignItems: "center" }}>
            <Input
              style={{ width: "200px" }}
              value={search}
              placeholder={t("board.sqs.queues.search")}
              onChange={(event) => setSearch(event.target.value)}
            />
            <Button
              size="sm"
              onClick={() => {
                setEditing(null);
                setFormOpen(true);
              }}
            >
              {t("board.sqs.queues.create")}
            </Button>
            <RefreshButton
              refreshing={state.refreshing}
              online={state.online}
              onClick={() => void state.refresh()}
            />
          </div>
        }
      />
      <QueueDialogSqs
        open={formOpen}
        editing={editing}
        onOpenChange={setFormOpen}
        onSubmit={save}
      />
      <BoardState state={state}>
        <PageBody>
          <div style={{ display: "flex", gap: "12px", minHeight: 0, flex: 1 }}>
            <Panel style={{ flex: 1, overflow: "auto" }}>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t("board.sqs.queues.name")}</TableHead>
                    <TableHead className="num">{t("board.sqs.queues.visible")}</TableHead>
                    <TableHead className="num">{t("board.sqs.queues.inFlight")}</TableHead>
                    <TableHead className="num">{t("board.sqs.queues.delayed")}</TableHead>
                    <TableHead className="num">{t("board.sqs.queues.depth")}</TableHead>
                    <TableHead>{t("board.sqs.queues.deadLetterQueue")}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {shown.map((entry) => (
                    <TableRow
                      key={entry.name}
                      data-state={detail?.name === entry.name ? "selected" : undefined}
                      onClick={() => setSelected(entry.name)}
                      style={{ cursor: "pointer" }}
                    >
                      <TableCell>
                        <span className="mono3" style={MONO11}>
                          {entry.name}
                        </span>
                        {entry.fifo && (
                          <span className="pb pSQS" style={{ marginLeft: "6px" }}>
                            {t("board.sqs.queues.fifoBadge")}
                          </span>
                        )}
                      </TableCell>
                      <TableCell className="num">
                        <span className="mono3" style={MONO11}>
                          {count(entry.visible)}
                        </span>
                      </TableCell>
                      <TableCell className="num">
                        <span className="mono3" style={MONO11}>
                          {count(entry.inFlight)}
                        </span>
                      </TableCell>
                      <TableCell className="num">
                        <span className="mono3" style={MONO11}>
                          {count(entry.delayed)}
                        </span>
                      </TableCell>
                      <TableCell className="num">
                        <span className="mono3" style={MONO11}>
                          {count(entry.depth)}
                        </span>
                      </TableCell>
                      <TableCell>
                        <span className="mono3" style={MONO11}>
                          {entry.deadLetterQueue ?? DASH}
                        </span>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </Panel>

            {detail != null && (
              <QueueDetail
                entry={detail}
                onEdit={() => {
                  setEditing(detail);
                  setFormOpen(true);
                }}
                onPurge={() => void purge(detail)}
                onRemove={() => void remove(detail)}
              />
            )}
          </div>
          <p
            style={{
              margin: "0 20px",
              fontSize: "11.5px",
              color: "var(--c-muted)",
              flex: "none",
            }}
          >
            {t("board.sqs.queues.note")}
          </p>
        </PageBody>
      </BoardState>
    </Page>
  );
}

function QueueDetail({
  entry,
  onEdit,
  onPurge,
  onRemove,
}: {
  entry: SqsQueue;
  onEdit: () => void;
  onPurge: () => void;
  onRemove: () => void;
}) {
  const { t } = useTranslation();
  return (
    <Panel style={{ width: "300px", flex: "none", overflow: "auto" }}>
      <PanelHeader title={entry.name} />
      <div style={{ padding: "0 12px 12px" }}>
        <SectionLabel>{t("board.sqs.queues.section.messages")}</SectionLabel>
        <KV
          rows={[
            [t("board.sqs.queues.visible"), count(entry.visible)],
            [t("board.sqs.queues.inFlight"), count(entry.inFlight)],
            [t("board.sqs.queues.delayed"), count(entry.delayed)],
            [t("board.sqs.queues.depth"), count(entry.depth)],
          ]}
        />

        <SectionLabel>{t("board.sqs.queues.section.settings")}</SectionLabel>
        <KV
          rows={[
            [t("board.sqs.queues.visibilityTimeout"), duration(entry.visibilityTimeoutSec)],
            [t("board.sqs.queues.deliveryDelay"), duration(entry.delaySec)],
            [t("board.sqs.queues.retention"), duration(entry.retentionSec)],
            [t("board.sqs.queues.receiveWait"), duration(entry.receiveWaitSec)],
            [
              t("board.sqs.queues.maxMessageBytes"),
              entry.maxMessageBytes == null ? DASH : formatBytes(entry.maxMessageBytes),
            ],
            [
              t("board.sqs.queues.encrypted"),
              t(entry.encrypted ? "common.yes" : "common.no"),
            ],
          ]}
        />

        {entry.fifo && (
          <>
            <SectionLabel>{t("board.sqs.queues.section.fifo")}</SectionLabel>
            <KV
              rows={[
                [
                  t("board.sqs.queues.contentDedup"),
                  entry.contentBasedDeduplication == null
                    ? DASH
                    : t(entry.contentBasedDeduplication ? "common.yes" : "common.no"),
                ],
                [t("board.sqs.queues.dedupScope"), entry.deduplicationScope ?? DASH],
                [t("board.sqs.queues.throughputLimit"), entry.fifoThroughputLimit ?? DASH],
              ]}
            />
          </>
        )}

        <SectionLabel>{t("board.sqs.queues.section.identity")}</SectionLabel>
        <KV
          rows={[
            [t("board.sqs.queues.deadLetterQueue"), entry.deadLetterQueue ?? DASH],
            [t("board.sqs.queues.maxReceiveCount"), count(entry.maxReceiveCount)],
            [t("board.sqs.queues.arn"), entry.arn ?? DASH],
            [t("board.sqs.queues.created"), formatMessageTime(entry.createdAtMs)],
            [t("board.sqs.queues.modified"), formatMessageTime(entry.modifiedAtMs)],
          ]}
        />

        <div style={{ display: "flex", gap: "6px", marginTop: "12px" }}>
          <Button size="sm" variant="outline" onClick={onEdit}>
            {t("common.edit")}
          </Button>
          <Button size="sm" variant="destructive" onClick={onPurge}>
            {t("board.sqs.queues.purgeAction")}
          </Button>
          <Button size="sm" variant="destructive" onClick={onRemove}>
            {t("common.delete")}
          </Button>
        </div>

        {/* Said where the number is, because this is the one reading that
            looks like an idle queue and is not: something took every message
            and has finished none of them. */}
        {stalledInFlight(entry) && (
          <p style={{ marginTop: "10px", fontSize: "11px", color: "var(--c-warn)" }}>
            {t("board.sqs.queues.allInFlight", { count: entry.inFlight ?? 0 })}
          </p>
        )}
      </div>
    </Panel>
  );
}
