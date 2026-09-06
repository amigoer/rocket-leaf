import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { Page, PageBody, PageHeader, RefreshButton } from "@/design/shell";
import { Input } from "@/components/ui/input";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { KV, Panel, PanelHeader, SectionLabel } from "@/components";
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
  const [search, setSearch] = useState("");
  const [selected, setSelected] = useState<string | null>(null);

  const streams = useMemo(() => (state.data ?? []).map(readStream), [state.data]);

  const shown = useMemo(() => {
    const needle = search.trim().toLowerCase();
    return streams.filter((entry) => needle === "" || entry.name.toLowerCase().includes(needle));
  }, [streams, search]);

  const detail = useMemo(
    () => shown.find((entry) => entry.name === selected) ?? shown[0] ?? null,
    [shown, selected],
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
            <RefreshButton
              refreshing={state.refreshing}
              online={state.online}
              onClick={() => void state.refresh()}
            />
          </div>
        }
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

            {detail != null && <StreamDetail entry={detail} />}
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

function StreamDetail({ entry }: { entry: KinesisStream }) {
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
            [t("board.kinesis.streams.created"), formatMessageTime(entry.createdAtMs)],
            [
              t("board.kinesis.streams.shardMetrics"),
              entry.shardMetrics.length === 0
                ? t("board.kinesis.streams.shardMetricsNone")
                : entry.shardMetrics.join(", "),
            ],
          ]}
        />

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
