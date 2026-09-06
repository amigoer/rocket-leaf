import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import { Page, PageBody, PageHeader, RefreshButton } from "@/design/shell";
import {
  Panel,
  PanelHeader,
  StatTile,
  Status,
} from "@/components";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { BoardState } from "@/design/boards/BoardState";
import { useNsqCluster } from "@/hooks/nsq/useNsqCluster";
import { useNsqDestinations } from "@/hooks/nsq/useNsqDestinations";
import { useNsqSubscriptions } from "@/hooks/nsq/useNsqSubscriptions";
import { node as readNode } from "@/mq/nsq/cluster";
import { topic as readTopic } from "@/mq/nsq/destinations";
import { channel as readChannel, channelKey } from "@/mq/nsq/subscriptions";
import { present } from "@/api/client";
import { formatCount } from "@/lib/format";
import { KPI_GRID } from "./_shared";

const MONO11 = { fontSize: "11px" } as const;

/**
 * The NSQ overview.
 *
 * No throughput chart and no rate anywhere, and the reason is the family's:
 * nsqd counts messages since it started and reports no rate at all. Dividing
 * two samples of a counter would be this app's arithmetic printed as the
 * broker's figure - the same call the Kafka and ActiveMQ overviews make.
 *
 * What the tiles show instead is where the messages are sitting, because on
 * this family that is three different problems with three different fixes. A
 * channel with a backlog and consumers attached is a slow consumer. One with a
 * backlog and nothing attached is a consumer that is not running. And a topic
 * holding its own messages has no channel to copy them into at all, or is
 * paused - which is the only way in NSQ to accumulate messages that no
 * consumer will ever be offered.
 *
 * The paused count has a tile of its own for the same reason it has an alert:
 * pausing keeps accepting publishes and delivers nothing, so every other
 * figure on this page stays healthy while the backlog grows.
 */
export function OverviewNsq() {
  const { t } = useTranslation();
  const cluster = useNsqCluster();
  const topicState = useNsqDestinations();
  const channelState = useNsqSubscriptions();

  const nodes = useMemo(() => present(cluster.data?.nodes).map(readNode), [cluster.data]);
  const topics = useMemo(() => (topicState.data ?? []).map(readTopic), [topicState.data]);
  const channels = useMemo(
    () => (channelState.data ?? []).map(readChannel),
    [channelState.data],
  );

  const backlog = channels.reduce((total, entry) => total + (entry.backlog ?? 0), 0);
  // Undrained is the figure worth a tile: a backlog with nothing attached is
  // one nobody is working through, which a total alone hides.
  const undrained = channels
    .filter((entry) => entry.clients === 0)
    .reduce((total, entry) => total + (entry.backlog ?? 0), 0);
  const held = topics.reduce((total, entry) => total + (entry.topicDepth ?? 0), 0);
  const paused =
    topics.filter((entry) => entry.paused).length +
    channels.filter((entry) => entry.paused).length;

  // Worst first, and only what is actually waiting: a page of zeroes is a
  // page nobody reads twice.
  const busiest = useMemo(
    () =>
      [...channels]
        .filter((entry) => (entry.backlog ?? 0) > 0)
        .sort((left, right) => (right.backlog ?? 0) - (left.backlog ?? 0))
        .slice(0, 8),
    [channels],
  );

  return (
    <Page>
      <PageHeader
        title={t("shell.nav.nsq.overview")}
        subtitle={t("board.nsq.overview.subtitle", {
          nodes: nodes.length,
          directory: cluster.data?.directory?.length ?? 0,
          version: nodes[0]?.version ?? "",
        })}
        actions={
          <RefreshButton
            refreshing={cluster.refreshing}
            online={cluster.online}
            onClick={() => void cluster.refresh()}
          />
        }
      />
      <BoardState state={cluster}>
        <PageBody>
          <div className={KPI_GRID}>
            <StatTile
              label={t("board.nsq.overview.topics")}
              value={formatCount(topics.length)}
            />
            <StatTile
              label={t("board.nsq.overview.channels")}
              value={formatCount(channels.length)}
            />
            <StatTile
              label={t("board.nsq.overview.backlog")}
              value={formatCount(backlog)}
              hint={t("board.nsq.overview.backlogHint")}
            />
            <StatTile
              label={t("board.nsq.overview.undrained")}
              value={formatCount(undrained)}
              hint={t("board.nsq.overview.undrainedHint")}
            />
            <StatTile
              label={t("board.nsq.overview.held")}
              value={formatCount(held)}
              hint={t("board.nsq.overview.heldHint")}
            />
            <StatTile
              label={t("board.nsq.overview.paused")}
              value={formatCount(paused)}
              hint={t("board.nsq.overview.pausedHint")}
            />
          </div>

          <Panel style={{ overflow: "auto" }}>
            <PanelHeader title={t("board.nsq.overview.busiest")} />
            {busiest.length === 0 ? (
              <p style={{ padding: "0 12px 12px", fontSize: "11.5px", color: "var(--c-muted)" }}>
                {t("board.nsq.overview.nothingWaiting")}
              </p>
            ) : (
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t("board.nsq.overview.channel")}</TableHead>
                    <TableHead className="num">{t("board.nsq.overview.backlog")}</TableHead>
                    <TableHead className="num">{t("board.nsq.overview.inFlight")}</TableHead>
                    <TableHead className="num">{t("board.nsq.overview.consumers")}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {busiest.map((entry) => (
                    <TableRow key={channelKey(entry)}>
                      <TableCell>
                        <span className="mono3" style={MONO11}>
                          {channelKey(entry)}
                        </span>
                        {entry.paused && (
                          <Status tone="warn" style={{ fontSize: "10px", marginLeft: "6px" }}>
                            {t("board.nsq.overview.pausedBadge")}
                          </Status>
                        )}
                        {!entry.paused && entry.clients === 0 && (
                          <Status tone="off" style={{ fontSize: "10px", marginLeft: "6px" }}>
                            {t("board.nsq.overview.idleBadge")}
                          </Status>
                        )}
                      </TableCell>
                      <TableCell className="num mono3" style={MONO11}>
                        {formatCount(entry.backlog ?? 0)}
                      </TableCell>
                      <TableCell className="num mono3" style={MONO11}>
                        {formatCount(entry.inFlight ?? 0)}
                      </TableCell>
                      <TableCell className="num mono3" style={MONO11}>
                        {entry.clients}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            )}
          </Panel>

          <p
            style={{
              margin: "0 20px",
              fontSize: "11.5px",
              color: "var(--c-muted)",
              flex: "none",
            }}
          >
            {t("board.nsq.overview.note")}
          </p>
        </PageBody>
      </BoardState>
    </Page>
  );
}
