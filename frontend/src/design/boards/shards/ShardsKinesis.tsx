import { useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { Page, PageBody, PageHeader, RefreshButton } from "@/design/shell";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
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
import { useKinesisShards } from "@/hooks/kinesis/useKinesisShards";
import { stream as readStream } from "@/mq/kinesis/destinations";
import { childrenOf, keyShare, openCount, parentsOf, type Shard } from "@/mq/kinesis/shards";
import { formatCount } from "@/lib/format";

const MONO11 = { fontSize: "11px" } as const;

/** A dash, not a zero: an absent figure and a measured zero are not alike. */
const DASH = "—";

function share(value: number | null): string {
  return value == null ? DASH : `${(value * 100).toFixed(1)}%`;
}

/**
 * A stream's shards.
 *
 * The page this family needed and no other has. Every other partitioned broker
 * here reports a count, and a count is all a partition is worth: they are
 * interchangeable slots addressed by index. A shard is not. It is named, it
 * owns the slice of the hash space that decides which records land on it, and
 * it is changed by being split in two or merged with a neighbour rather than
 * resized - which leaves the old shard in place, closed, still holding its
 * records until retention expires, and named as its children's parent.
 *
 * So the table shows closed shards as well as open ones, and says which is
 * which. A reader who came here because a stream "lost data" after a resize is
 * looking for exactly that row: the records are on a parent nobody drained.
 *
 * The key share column is the closest thing to "how much of the traffic should
 * land here". Records are placed by hashing the partition key, so an even
 * spread lands in proportion to those widths - and a stream split unevenly has
 * one shard doing most of the work, which the shard count cannot say.
 */
export function ShardsKinesis() {
  const { t } = useTranslation();
  const streams = useKinesisDestinations();
  const [stream, setStream] = useState<string | null>(null);
  const [selected, setSelected] = useState<string | null>(null);

  const names = useMemo(
    () => (streams.data ?? []).map(readStream).map((entry) => entry.name),
    [streams.data],
  );

  // The first stream, once there is one. Chosen here rather than defaulted in
  // state because the listing arrives after the first render.
  useEffect(() => {
    if (stream == null && names.length > 0) setStream(names[0] ?? null);
  }, [names, stream]);

  const shards = useKinesisShards(stream);
  const detail = useMemo(
    () => shards.shards.find((entry) => entry.id === selected) ?? shards.shards[0] ?? null,
    [shards.shards, selected],
  );

  const open = openCount(shards.shards);
  const closed = shards.shards.length - open;

  return (
    <Page>
      <PageHeader
        title={t("shell.nav.kinesis.shards")}
        subtitle={t("board.kinesis.shards.count", {
          open: formatCount(open),
          closed: formatCount(closed),
        })}
        actions={
          <div style={{ display: "flex", gap: "8px", alignItems: "center" }}>
            <Select value={stream ?? ""} onValueChange={(next: string) => setStream(next)}>
              <SelectTrigger size="sm" style={{ width: "220px" }}>
                <SelectValue placeholder={t("board.kinesis.shards.pickStream")} />
              </SelectTrigger>
              <SelectContent>
                {names.map((name) => (
                  <SelectItem key={name} value={name}>
                    {name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <RefreshButton
              refreshing={shards.loading}
              online={streams.online}
              onClick={() => void shards.refresh()}
            />
          </div>
        }
      />
      <BoardState state={streams}>
        <PageBody>
          {shards.error != null && (
            <p style={{ margin: "0 20px", fontSize: "11.5px", color: "var(--c-danger)" }}>
              {shards.error}
            </p>
          )}
          <div style={{ display: "flex", gap: "12px", minHeight: 0, flex: 1 }}>
            <Panel style={{ flex: 1, overflow: "auto" }}>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t("board.kinesis.shards.id")}</TableHead>
                    <TableHead>{t("board.kinesis.shards.state")}</TableHead>
                    <TableHead className="num">{t("board.kinesis.shards.keyShare")}</TableHead>
                    <TableHead>{t("board.kinesis.shards.parents")}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {shards.shards.map((entry) => (
                    <TableRow
                      key={entry.id}
                      data-state={detail?.id === entry.id ? "selected" : undefined}
                      onClick={() => setSelected(entry.id)}
                      style={{ cursor: "pointer" }}
                    >
                      <TableCell>
                        <span className="mono3" style={MONO11}>
                          {entry.id}
                        </span>
                      </TableCell>
                      <TableCell>
                        <span
                          className="mono3"
                          style={{ ...MONO11, color: entry.closed ? "var(--c-muted)" : undefined }}
                        >
                          {t(
                            entry.closed
                              ? "board.kinesis.shards.closed"
                              : "board.kinesis.shards.open",
                          )}
                        </span>
                      </TableCell>
                      <TableCell className="num">
                        <span className="mono3" style={MONO11}>
                          {share(keyShare(entry))}
                        </span>
                      </TableCell>
                      <TableCell>
                        <span className="mono3" style={MONO11}>
                          {parentsOf(entry).join(", ") || DASH}
                        </span>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </Panel>

            {detail != null && <ShardDetail entry={detail} shards={shards.shards} />}
          </div>
          <p
            style={{
              margin: "0 20px",
              fontSize: "11.5px",
              color: "var(--c-muted)",
              flex: "none",
            }}
          >
            {t("board.kinesis.shards.note")}
          </p>
        </PageBody>
      </BoardState>
    </Page>
  );
}

function ShardDetail({ entry, shards }: { entry: Shard; shards: Shard[] }) {
  const { t } = useTranslation();
  const children = childrenOf(shards, entry.id);
  const parents = parentsOf(entry);

  return (
    <Panel style={{ width: "320px", flex: "none", overflow: "auto" }}>
      <PanelHeader title={entry.id} />
      <div style={{ padding: "0 12px 12px" }}>
        <SectionLabel>{t("board.kinesis.shards.section.range")}</SectionLabel>
        <KV
          rows={[
            [
              t("board.kinesis.shards.state"),
              t(entry.closed ? "board.kinesis.shards.closed" : "board.kinesis.shards.open"),
            ],
            [t("board.kinesis.shards.keyShare"), share(keyShare(entry))],
            [t("board.kinesis.shards.startHashKey"), entry.startHashKey || DASH],
            [t("board.kinesis.shards.endHashKey"), entry.endHashKey || DASH],
          ]}
        />

        <SectionLabel>{t("board.kinesis.shards.section.sequence")}</SectionLabel>
        <KV
          rows={[
            [t("board.kinesis.shards.startSequence"), entry.startSequence || DASH],
            [t("board.kinesis.shards.endSequence"), entry.endSequence || DASH],
          ]}
        />

        <SectionLabel>{t("board.kinesis.shards.section.lineage")}</SectionLabel>
        <KV
          rows={[
            [t("board.kinesis.shards.parents"), parents.join(", ") || DASH],
            [
              t("board.kinesis.shards.children"),
              children.map((child) => child.id).join(", ") || DASH,
            ],
          ]}
        />

        {/* Said where the row is, because a closed shard reads as gone and is
            not: it holds its records until retention expires, and a consumer
            that skipped it skipped them. */}
        {entry.closed && (
          <p style={{ marginTop: "10px", fontSize: "11px", color: "var(--c-warn)" }}>
            {t("board.kinesis.shards.closedNote")}
          </p>
        )}
      </div>
    </Panel>
  );
}
