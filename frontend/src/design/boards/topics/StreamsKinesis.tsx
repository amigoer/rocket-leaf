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
import { useKinesisDestinations } from "@/hooks/kinesis/useKinesisDestinations";
import {
  retention,
  settling,
  stream as readStream,
  type KinesisStream,
} from "@/mq/kinesis/destinations";
import { formatCount } from "@/lib/format";
import { formatMessageTime } from "@/lib/time";
import { formatErrorMessage } from "@/lib/utils";
import { useConnectionScope } from "@/mq/ConnectionScope";
import * as kinesisApi from "@/api/kinesis";
import { StreamDialogKinesis } from "./StreamDialogKinesis";

const MONO11 = { fontSize: "11px" } as const;

/** A dash, not a zero: an absent figure and a measured zero are not alike. */
const DASH = "—";

function count(value: number | null): string {
  return value == null ? DASH : formatCount(value);
}

/**
 * Amazon Kinesis data streams.
 *
 * One row per stream in the region, narrowed by the connection's prefix. The
 * shard count is here and the shards are not: a count is a true thing to say
 * about a stream, and everything that makes a shard worth looking at - its id,
 * the slice of the hash space it owns, the parent it was split from - needs a
 * table of its own, which is the shards page.
 *
 * There is no depth column, and that is the service rather than this board.
 * Kinesis keeps no count of what a stream is holding; the only way to produce
 * one would be to read every shard end to end, which would spend the read
 * quota of every shard on the board to show a figure that was stale on
 * arrival. There is no rate column either - those are CloudWatch's.
 */
export function StreamsKinesis() {
  const { t } = useTranslation();
  const state = useKinesisDestinations();
  const { id: connID } = useConnectionScope();
  const confirm = useConfirm();
  const [search, setSearch] = useState("");
  const [selected, setSelected] = useState<string | null>(null);
  const [editing, setEditing] = useState<KinesisStream | null>(null);
  const [formOpen, setFormOpen] = useState(false);

  const streams = useMemo(() => (state.data ?? []).map(readStream), [state.data]);

  const shown = useMemo(() => {
    const needle = search.trim().toLowerCase();
    return streams.filter((entry) => needle === "" || entry.name.toLowerCase().includes(needle));
  }, [streams, search]);

  const detail = useMemo(
    () => shown.find((entry) => entry.name === selected) ?? shown[0] ?? null,
    [shown, selected],
  );

  const save = useCallback(
    async (input: kinesisApi.KinesisStreamInput) => {
      if (editing == null) {
        await kinesisApi.createStream(connID, input);
        toast.success(t("board.kinesis.streams.created", { name: input.name }));
      } else {
        await kinesisApi.updateStream(connID, input);
        toast.success(t("board.kinesis.streams.updated", { name: input.name }));
      }
      await state.refresh();
    },
    [connID, editing, state, t],
  );

  /*
   * The confirmation says what a delete takes rather than only what it is
   * called. A stream's records go with it whether or not anything read them,
   * and there is no undo - the name can be reused, but the data cannot be
   * recovered from anywhere in the service.
   */
  const remove = useCallback(
    async (entry: KinesisStream) => {
      const ok = await confirm({
        title: t("board.kinesis.streams.deleteTitle", { name: entry.name }),
        description: t("board.kinesis.streams.deleteDesc", { count: entry.openShards ?? 0 }),
        confirmLabel: t("common.delete"),
        danger: true,
      });
      if (!ok) return;
      try {
        await kinesisApi.removeStream(connID, entry.name);
        toast.success(t("board.kinesis.streams.deleted", { name: entry.name }));
        setSelected(null);
        await state.refresh();
      } catch (deleteError) {
        toast.error(t("board.kinesis.streams.deleteFailed"), {
          description: formatErrorMessage(deleteError),
        });
      }
    },
    [confirm, connID, state, t],
  );

  return (
    <Page>
      <PageHeader
        title={t("shell.nav.kinesis.topics")}
        subtitle={t("board.kinesis.streams.count", { count: streams.length })}
        actions={
          <div style={{ display: "flex", gap: "8px", alignItems: "center" }}>
            <Input
              style={{ width: "200px" }}
              value={search}
              placeholder={t("board.kinesis.streams.search")}
              onChange={(event) => setSearch(event.target.value)}
            />
            <Button
              size="sm"
              onClick={() => {
                setEditing(null);
                setFormOpen(true);
              }}
            >
              {t("board.kinesis.streams.create")}
            </Button>
            <RefreshButton
              refreshing={state.refreshing}
              online={state.online}
              onClick={() => void state.refresh()}
            />
          </div>
        }
      />
      <StreamDialogKinesis
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
                    <TableHead>{t("board.kinesis.streams.name")}</TableHead>
                    <TableHead>{t("board.kinesis.streams.status")}</TableHead>
                    <TableHead className="num">{t("board.kinesis.streams.openShards")}</TableHead>
                    <TableHead className="num">{t("board.kinesis.streams.consumers")}</TableHead>
                    <TableHead className="num">{t("board.kinesis.streams.retention")}</TableHead>
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
                        {entry.mode === "ON_DEMAND" && (
                          <span className="pb pKDS" style={{ marginLeft: "6px" }}>
                            {t("board.kinesis.streams.onDemandBadge")}
                          </span>
                        )}
                      </TableCell>
                      <TableCell>
                        <span
                          className="mono3"
                          style={{
                            ...MONO11,
                            color: settling(entry) ? "var(--c-warn)" : undefined,
                          }}
                        >
                          {entry.status ?? DASH}
                        </span>
                      </TableCell>
                      <TableCell className="num">
                        <span className="mono3" style={MONO11}>
                          {count(entry.openShards)}
                        </span>
                      </TableCell>
                      <TableCell className="num">
                        <span className="mono3" style={MONO11}>
                          {count(entry.consumers)}
                        </span>
                      </TableCell>
                      <TableCell className="num">
                        <span className="mono3" style={MONO11}>
                          {retention(entry.retentionHours) ?? DASH}
                        </span>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </Panel>

            {detail != null && (
              <StreamDetail
                entry={detail}
                onEdit={() => {
                  setEditing(detail);
                  setFormOpen(true);
                }}
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
            {t("board.kinesis.streams.note")}
          </p>
        </PageBody>
      </BoardState>
    </Page>
  );
}

function StreamDetail({
  entry,
  onEdit,
  onRemove,
}: {
  entry: KinesisStream;
  onEdit: () => void;
  onRemove: () => void;
}) {
  const { t } = useTranslation();
  return (
    <Panel style={{ width: "300px", flex: "none", overflow: "auto" }}>
      <PanelHeader title={entry.name} />
      <div style={{ padding: "0 12px 12px" }}>
        <SectionLabel>{t("board.kinesis.streams.section.capacity")}</SectionLabel>
        <KV
          rows={[
            [t("board.kinesis.streams.mode"), entry.mode ?? DASH],
            [t("board.kinesis.streams.openShards"), count(entry.openShards)],
            [t("board.kinesis.streams.retention"), retention(entry.retentionHours) ?? DASH],
            [t("board.kinesis.streams.consumers"), count(entry.consumers)],
          ]}
        />

        <SectionLabel>{t("board.kinesis.streams.section.identity")}</SectionLabel>
        <KV
          rows={[
            [t("board.kinesis.streams.status"), entry.status ?? DASH],
            [t("board.kinesis.streams.encryption"), entry.encryption ?? DASH],
            [t("board.kinesis.streams.kmsKey"), entry.kmsKeyId ?? DASH],
            [t("board.kinesis.streams.arn"), entry.arn ?? DASH],
            [t("board.kinesis.streams.createdAt"), formatMessageTime(entry.createdAtMs)],
            [
              t("board.kinesis.streams.shardMetrics"),
              entry.shardMetrics.length === 0
                ? t("board.kinesis.streams.shardMetricsNone")
                : entry.shardMetrics.join(", "),
            ],
          ]}
        />

        <div style={{ display: "flex", gap: "6px", marginTop: "12px" }}>
          <Button size="sm" variant="outline" disabled={settling(entry)} onClick={onEdit}>
            {t("common.edit")}
          </Button>
          <Button
            size="sm"
            variant="destructive"
            disabled={settling(entry)}
            onClick={onRemove}
          >
            {t("common.delete")}
          </Button>
        </div>

        {/* Said where the state is, because this is the reading that turns
            every button on the page into an error the service words badly:
            a stream that is not ACTIVE refuses the call as "resource in use". */}
        {settling(entry) && (
          <p style={{ marginTop: "10px", fontSize: "11px", color: "var(--c-warn)" }}>
            {t("board.kinesis.streams.settling", { status: entry.status })}
          </p>
        )}
      </div>
    </Panel>
  );
}
