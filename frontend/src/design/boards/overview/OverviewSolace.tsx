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
import { useSolaceDestinations } from "@/hooks/solace/useSolaceDestinations";
import { useSolaceBroker } from "@/hooks/solace/useSolaceCluster";
import {
  destination as readDestination,
  halted,
  hasDeadMsgQueue,
} from "@/mq/solace/destinations";
import { broker as readBroker } from "@/mq/solace/cluster";
import { formatCount } from "@/lib/format";
import { KPI_GRID } from "./_shared";

const MONO11 = { fontSize: "11px" } as const;

/**
 * The Solace overview.
 *
 * Built from the queue listing and the one broker row, because between them
 * they are what a Message VPN is: endpoints that hold messages, and the spool
 * they all come out of. There is no node table - a redundancy pair shares a
 * virtual router and only the active half answers - and no throughput chart,
 * because what SEMP reports is an instantaneous rate rather than a series.
 *
 * The tile worth explaining is "discarding". Every Solace endpoint ships
 * pointing at a dead message queue called #DEAD_MSG_QUEUE, and no broker
 * creates a queue by that name - so a queue with a redelivery limit is
 * configured to dead-letter and actually discards, with every other figure on
 * every other page looking perfectly healthy. It is the first thing that goes
 * wrong on this family and the last thing anybody notices.
 *
 * The spool tile is the Message VPN's share rather than a disk. Past its quota
 * the broker refuses guaranteed messages for this VPN alone, which is an
 * outage that shows up nowhere else on the appliance.
 */
export function OverviewSolace() {
  const { t } = useTranslation();
  const state = useSolaceDestinations();
  const brokerState = useSolaceBroker();

  const queues = useMemo(() => (state.data ?? []).map(readDestination), [state.data]);
  const broker = useMemo(() => {
    const first = (brokerState.data ?? [])[0];
    return first != null ? readBroker(first) : null;
  }, [brokerState.data]);

  const spooled = queues.reduce((total, entry) => total + (entry.depth ?? 0), 0);
  const bound = queues.reduce((total, entry) => total + (entry.boundConsumers ?? 0), 0);
  const stopped = queues.filter(halted).length;

  // Queues that will give up on a message and have nowhere to put it. The same
  // test the alert rule makes, because the tile and the alert disagreeing
  // would be worse than either.
  const names = useMemo(() => new Set(queues.map((entry) => entry.name)), [queues]);
  const discarding = queues.filter(
    (entry) =>
      hasDeadMsgQueue(entry) &&
      !names.has(entry.deadMsgQueue ?? "") &&
      ((entry.maxRedeliveryCount ?? 0) > 0 ||
        (entry.respectTtlEnabled && (entry.maxTtlSec ?? 0) > 0)),
  ).length;

  // Worst first, and only what is worth a second look: the deepest queues,
  // with a stopped one always ahead of a deeper one that is working.
  const notable = useMemo(
    () =>
      [...queues]
        .sort((left, right) => {
          const byHalted = Number(halted(right)) - Number(halted(left));
          if (byHalted !== 0) return byHalted;
          return (right.depth ?? 0) - (left.depth ?? 0);
        })
        .slice(0, 8),
    [queues],
  );

  return (
    <Page>
      <PageHeader
        title={t("shell.nav.solace.overview")}
        subtitle={t("board.solace.overview.subtitle", {
          vpn: broker?.msgVpn ?? "",
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
              label={t("board.solace.overview.queues")}
              value={formatCount(queues.length)}
              hint={t("board.solace.overview.queuesHint")}
            />
            <StatTile
              label={t("board.solace.overview.spooled")}
              value={formatCount(spooled)}
              hint={t("board.solace.overview.spooledHint")}
            />
            <StatTile
              label={t("board.solace.overview.bound")}
              value={formatCount(bound)}
              hint={t("board.solace.overview.boundHint")}
            />
            <StatTile
              label={t("board.solace.overview.spool")}
              value={broker?.spoolPercent == null ? "—" : `${broker.spoolPercent}%`}
              hint={t("board.solace.overview.spoolHint")}
              valueColor={
                (broker?.spoolPercent ?? 0) >= 80 ? "var(--c-warn-text)" : undefined
              }
            />
            <StatTile
              label={t("board.solace.overview.discarding")}
              value={formatCount(discarding)}
              hint={t("board.solace.overview.discardingHint")}
              valueColor={discarding > 0 ? "var(--c-warn-text)" : undefined}
            />
            <StatTile
              label={t("board.solace.overview.stopped")}
              value={formatCount(stopped)}
              hint={t("board.solace.overview.stoppedHint")}
              valueColor={stopped > 0 ? "var(--c-warn-text)" : undefined}
            />
          </div>

          <Panel>
            <PanelHeader title={t("board.solace.overview.deepest")} />
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t("board.solace.queues.name")}</TableHead>
                  <TableHead className="num">{t("board.solace.queues.spooled")}</TableHead>
                  <TableHead className="num">{t("board.solace.queues.bound")}</TableHead>
                  <TableHead>{t("board.solace.overview.state")}</TableHead>
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
                        {formatCount(entry.boundConsumers ?? 0)}
                      </span>
                    </TableCell>
                    <TableCell>
                      <span style={{ fontSize: "11px", color: "var(--c-muted)" }}>
                        {halted(entry)
                          ? t("board.solace.overview.stateHalted")
                          : (entry.boundConsumers ?? 0) === 0 && (entry.depth ?? 0) > 0
                            ? t("board.solace.overview.stateUndrained")
                            : t("board.solace.overview.stateOrdinary")}
                      </span>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </Panel>

          <p style={{ margin: "0 20px", fontSize: "11.5px", color: "var(--c-muted)" }}>
            {t("board.solace.overview.note")}
          </p>
        </PageBody>
      </BoardState>
    </Page>
  );
}
