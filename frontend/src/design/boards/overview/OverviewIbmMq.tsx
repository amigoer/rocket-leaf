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
import { useIbmMqDestinations } from "@/hooks/ibmmq/useIbmMqDestinations";
import { useIbmMqChannels } from "@/hooks/ibmmq/useIbmMqChannels";
import { destination as readDestination, inhibited } from "@/mq/ibmmq/destinations";
import { running, statusExpected, unhealthy } from "@/mq/ibmmq/channels";
import { useConnectionScope } from "@/mq/ConnectionScope";
import { useConnectionProfiles } from "@/hooks/useConnectionProfiles";
import { formatCount } from "@/lib/format";
import { KPI_GRID } from "./_shared";

const MONO11 = { fontSize: "11px" } as const;

/**
 * The IBM MQ overview.
 *
 * Built from the queue listing and the channel listing, because between them
 * they are what a queue manager is: objects that hold messages, and objects
 * that let anything reach them. There is no cluster panel and no node table -
 * this connection speaks to one queue manager, and an MQ cluster is a set of
 * queue managers publishing to each other's repositories rather than nodes of
 * this one. There is no throughput chart either: MQ reports what a queue holds
 * and when it was last touched, and how fast it is moving is a statistics
 * message published to a system queue on a timer rather than a figure a page
 * can read.
 *
 * The two tiles worth explaining are the channel ones, and they are here
 * rather than only on the channels page because they are the first thing that
 * goes wrong on this family. A channel that is not running is not
 * automatically a fault - most of a fresh queue manager's channels have never
 * been started and never will be - so what is counted is the ones that were
 * started and are not working, which is a different and much smaller number.
 *
 * Inhibited queues get a tile for the same reason: it is the commonest cause
 * of a queue manager quietly dead-lettering, and it leaves no other mark.
 */
export function OverviewIbmMq() {
  const { t } = useTranslation();
  const state = useIbmMqDestinations();
  const channelState = useIbmMqChannels();
  const { id: connID } = useConnectionScope();
  const { profiles } = useConnectionProfiles();

  const destinations = useMemo(() => (state.data ?? []).map(readDestination), [state.data]);
  const channels = useMemo(() => channelState.data ?? [], [channelState.data]);

  // The queue manager is what the subtitle names, because a second one behind
  // the same server is a different set of objects entirely. It is read off the
  // profile because a profile may leave it blank and let the driver discover
  // it, in which case the server's address is all this page can honestly say.
  const profile = useMemo(
    () => profiles.find((candidate) => candidate.id === connID),
    [connID, profiles],
  );
  const queueManager = profile?.options?.queueManager ?? "";

  const queues = destinations.filter((entry) => entry.kind === "queue");
  const topics = destinations.filter((entry) => entry.kind === "topic");
  const depth = queues.reduce((total, entry) => total + (entry.depth ?? 0), 0);
  const blocked = queues.filter(inhibited).length;
  const runningChannels = channels.filter(running).length;
  // Started and not working, which is not the same as "not running": a channel
  // nobody has started reports no status at all, and there are a dozen of
  // those on a queue manager that has never been used.
  const troubled = channels.filter(
    (channel) => statusExpected(channel) && channel.status !== "" && unhealthy(channel),
  ).length;

  // Worst first, and only what is worth a second look: the deepest queues,
  // with an inhibited one always ahead of a deeper one that is working.
  const notable = useMemo(
    () =>
      [...queues]
        .sort((left, right) => {
          const byInhibited = Number(inhibited(right)) - Number(inhibited(left));
          if (byInhibited !== 0) return byInhibited;
          return (right.depth ?? 0) - (left.depth ?? 0);
        })
        .slice(0, 8),
    [queues],
  );

  return (
    <Page>
      <PageHeader
        title={t("shell.nav.ibmmq.overview")}
        subtitle={t("board.ibmmq.overview.subtitle", {
          qmgr: queueManager === "" ? t("board.ibmmq.overview.discovered") : queueManager,
          queues: queues.length,
        })}
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
              label={t("board.ibmmq.overview.queues")}
              value={formatCount(queues.length)}
              hint={t("board.ibmmq.overview.queuesHint")}
            />
            <StatTile
              label={t("board.ibmmq.overview.depth")}
              value={formatCount(depth)}
              hint={t("board.ibmmq.overview.depthHint")}
            />
            <StatTile
              label={t("board.ibmmq.overview.topics")}
              value={formatCount(topics.length)}
              hint={t("board.ibmmq.overview.topicsHint")}
            />
            <StatTile
              label={t("board.ibmmq.overview.runningChannels")}
              value={formatCount(runningChannels)}
              hint={t("board.ibmmq.overview.runningChannelsHint")}
            />
            <StatTile
              label={t("board.ibmmq.overview.troubledChannels")}
              value={formatCount(troubled)}
              hint={t("board.ibmmq.overview.troubledChannelsHint")}
              valueColor={troubled > 0 ? "var(--c-warn-text)" : undefined}
            />
            <StatTile
              label={t("board.ibmmq.overview.inhibited")}
              value={formatCount(blocked)}
              hint={t("board.ibmmq.overview.inhibitedHint")}
              valueColor={blocked > 0 ? "var(--c-warn-text)" : undefined}
            />
          </div>

          <Panel>
            <PanelHeader title={t("board.ibmmq.overview.deepest")} />
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t("board.ibmmq.queues.name")}</TableHead>
                  <TableHead className="num">{t("board.ibmmq.queues.depth")}</TableHead>
                  <TableHead className="num">{t("board.ibmmq.queues.maxDepth")}</TableHead>
                  <TableHead className="num">{t("board.ibmmq.queues.openInput")}</TableHead>
                  <TableHead>{t("board.ibmmq.overview.state")}</TableHead>
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
                        {formatCount(entry.depth ?? 0)}
                      </span>
                    </TableCell>
                    <TableCell className="num">
                      <span className="mono3" style={MONO11}>
                        {formatCount(entry.maxDepth ?? 0)}
                      </span>
                    </TableCell>
                    <TableCell className="num">
                      <span className="mono3" style={MONO11}>
                        {formatCount(entry.openInput ?? 0)}
                      </span>
                    </TableCell>
                    <TableCell>
                      <span style={{ fontSize: "11px", color: "var(--c-muted)" }}>
                        {entry.deadLetterQueue
                          ? t("board.ibmmq.overview.stateDeadLetter")
                          : inhibited(entry)
                            ? t("board.ibmmq.overview.stateInhibited")
                            : entry.transmissionQueue
                              ? t("board.ibmmq.overview.stateTransmission")
                              : t("board.ibmmq.overview.stateOrdinary")}
                      </span>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </Panel>

          <p style={{ margin: "0 20px", fontSize: "11.5px", color: "var(--c-muted)" }}>
            {t("board.ibmmq.overview.note")}
          </p>
        </PageBody>
      </BoardState>
    </Page>
  );
}
