import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import { Page, PageBody, PageHeader, RefreshButton } from "@/design/shell";
import { Panel, PanelHeader, StatTile } from "@/components";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { BoardState } from "@/design/boards/BoardState";
import { useKinesisDestinations } from "@/hooks/kinesis/useKinesisDestinations";
import { useKinesisConsumers } from "@/hooks/kinesis/useKinesisConsumers";
import { retention, settling, stream as readStream } from "@/mq/kinesis/destinations";
import { consumer as readConsumer } from "@/mq/kinesis/subscriptions";
import { useConnectionScope } from "@/mq/ConnectionScope";
import { useConnectionProfiles } from "@/hooks/useConnectionProfiles";
import { formatCount } from "@/lib/format";
import { KPI_GRID } from "./_shared";

const MONO11 = { fontSize: "11px" } as const;
const DASH = "—";

/**
 * The Amazon Kinesis overview.
 *
 * Built from the stream listing and the consumer listing, because that is the
 * whole of what the service reports. There is no cluster panel, no node table
 * and no version: AWS runs it, and a topology board would have one invented
 * row. There is no throughput chart either - IncomingRecords and its
 * neighbours are CloudWatch metrics, a different service under a different
 * permission, and two samples taken here would be this app's arithmetic
 * presented as AWS's.
 *
 * What the tiles count instead is capacity, because on this family that is the
 * figure with a bill and a limit attached. Open shards are what a provisioned
 * stream is charged for and what bounds its throughput; closed shards are the
 * ones a resize left behind, which take no writes and still hold their records
 * until retention expires. Separating them is the point: their sum is what the
 * shards page lists and their difference is what the streams page reports.
 *
 * Registered consumers get a tile because they are the only readers the
 * service knows about, and because there are at most twenty per stream - a
 * quota an operator hits without warning otherwise.
 */
export function OverviewKinesis() {
  const { t } = useTranslation();
  const state = useKinesisDestinations();
  const consumersState = useKinesisConsumers();
  const { id: connID } = useConnectionScope();
  const { profiles } = useConnectionProfiles();

  const streams = useMemo(() => (state.data ?? []).map(readStream), [state.data]);
  const consumers = useMemo(
    () => (consumersState.data ?? []).map(readConsumer),
    [consumersState.data],
  );

  // The region is what an address would be on any other family, so it is what
  // the subtitle names. It is read off the profile because the stream listing
  // does not carry it - every ARN in it already belongs to that region.
  const region = useMemo(
    () => profiles.find((profile) => profile.id === connID)?.options?.region ?? "",
    [connID, profiles],
  );

  const openShards = streams.reduce((total, entry) => total + (entry.openShards ?? 0), 0);
  const onDemand = streams.filter((entry) => entry.mode === "ON_DEMAND").length;
  const unsettled = streams.filter(settling).length;

  // Worst first, and only what is worth a second look: a stream mid-operation,
  // then the widest ones, because those are what the capacity bill is.
  const notable = useMemo(
    () =>
      [...streams]
        .sort((left, right) => {
          const bySettling = Number(settling(right)) - Number(settling(left));
          if (bySettling !== 0) return bySettling;
          return (right.openShards ?? 0) - (left.openShards ?? 0);
        })
        .slice(0, 8),
    [streams],
  );

  const consumersOf = (name: string) =>
    consumers.filter((entry) => entry.stream === name).length;

  return (
    <Page>
      <PageHeader
        title={t("shell.nav.kinesis.overview")}
        subtitle={t("board.kinesis.overview.subtitle", { region, streams: streams.length })}
        actions={
          <RefreshButton
            refreshing={state.refreshing}
            online={state.online}
            onClick={() => void state.refresh()}
          />
        }
      />
      <BoardState state={state}>
        <PageBody>
          <div className={KPI_GRID}>
            <StatTile
              label={t("board.kinesis.overview.streams")}
              value={formatCount(streams.length)}
              hint={t("board.kinesis.overview.streamsHint")}
            />
            <StatTile
              label={t("board.kinesis.overview.openShards")}
              value={formatCount(openShards)}
              hint={t("board.kinesis.overview.openShardsHint")}
            />
            <StatTile
              label={t("board.kinesis.overview.onDemand")}
              value={formatCount(onDemand)}
              hint={t("board.kinesis.overview.onDemandHint")}
            />
            <StatTile
              label={t("board.kinesis.overview.consumers")}
              value={formatCount(consumers.length)}
              hint={t("board.kinesis.overview.consumersHint")}
            />
            <StatTile
              label={t("board.kinesis.overview.settling")}
              value={formatCount(unsettled)}
              hint={t("board.kinesis.overview.settlingHint")}
              valueColor={unsettled > 0 ? "var(--c-warn-text)" : undefined}
            />
          </div>

          <Panel>
            <PanelHeader title={t("board.kinesis.overview.widest")} />
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t("board.kinesis.streams.name")}</TableHead>
                  <TableHead className="num">{t("board.kinesis.streams.openShards")}</TableHead>
                  <TableHead className="num">{t("board.kinesis.streams.consumers")}</TableHead>
                  <TableHead className="num">{t("board.kinesis.streams.retention")}</TableHead>
                  <TableHead>{t("board.kinesis.overview.state")}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {notable.map((entry) => (
                  <TableRow key={entry.name}>
                    <TableCell>
                      <span className="mono3" style={MONO11}>
                        {entry.name}
                      </span>
                    </TableCell>
                    <TableCell className="num">
                      <span className="mono3" style={MONO11}>
                        {formatCount(entry.openShards ?? 0)}
                      </span>
                    </TableCell>
                    <TableCell className="num">
                      <span className="mono3" style={MONO11}>
                        {formatCount(consumersOf(entry.name))}
                      </span>
                    </TableCell>
                    <TableCell className="num">
                      <span className="mono3" style={MONO11}>
                        {retention(entry.retentionHours) ?? DASH}
                      </span>
                    </TableCell>
                    <TableCell>
                      <span style={{ fontSize: "11px", color: "var(--c-muted)" }}>
                        {settling(entry)
                          ? t("board.kinesis.overview.stateSettling", { status: entry.status })
                          : entry.mode === "ON_DEMAND"
                            ? t("board.kinesis.overview.stateOnDemand")
                            : t("board.kinesis.overview.stateProvisioned")}
                      </span>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </Panel>

          <p style={{ margin: "0 20px", fontSize: "11.5px", color: "var(--c-muted)" }}>
            {t("board.kinesis.overview.note")}
          </p>
        </PageBody>
      </BoardState>
    </Page>
  );
}
