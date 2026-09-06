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
import { useSqsDestinations } from "@/hooks/sqs/useSqsDestinations";
import { queue as readQueue, stalledInFlight } from "@/mq/sqs/destinations";
import { useConnectionScope } from "@/mq/ConnectionScope";
import { useConnectionProfiles } from "@/hooks/useConnectionProfiles";
import { formatCount } from "@/lib/format";
import { KPI_GRID } from "./_shared";

const MONO11 = { fontSize: "11px" } as const;

/**
 * The Amazon SQS overview.
 *
 * Built from the queue listing and nothing else, because that is the whole of
 * what SQS reports. There is no cluster panel, no node table and no version:
 * AWS runs the service, and a topology board would have one invented row on
 * it. There is no throughput chart either - SQS publishes its rates to
 * CloudWatch, a different API under a different permission, and two samples
 * taken here would be this app's arithmetic presented as AWS's.
 *
 * What the tiles show instead is where the messages are sitting, because on
 * this family that is three different problems with three different fixes.
 * Available with nothing taking it is a consumer that is not running. In
 * flight with nothing available is a consumer that took the work and is not
 * finishing. Delayed is a queue doing exactly what it was asked to, and
 * separating it is what keeps it out of the other two.
 *
 * Dead letters get a tile of their own for the reason they get an alert: a
 * message there was given up on, nothing drains it, and every other figure on
 * this page stays healthy while it accumulates.
 */
export function OverviewSqs() {
  const { t } = useTranslation();
  const state = useSqsDestinations();
  const { id: connID } = useConnectionScope();
  const { profiles } = useConnectionProfiles();

  const queues = useMemo(() => (state.data ?? []).map(readQueue), [state.data]);

  // The region is what an address would be on any other family, so it is what
  // the subtitle names. It is read off the profile because the queue listing
  // does not carry it - every URL in it already belongs to that region.
  const region = useMemo(
    () => profiles.find((profile) => profile.id === connID)?.options?.region ?? "",
    [connID, profiles],
  );

  const visible = queues.reduce((total, entry) => total + (entry.visible ?? 0), 0);
  const inFlight = queues.reduce((total, entry) => total + (entry.inFlight ?? 0), 0);
  const delayed = queues.reduce((total, entry) => total + (entry.delayed ?? 0), 0);

  // A dead-letter queue is one another queue's redrive policy points at, and
  // the listing already carries every target - so this costs no extra request.
  const deadLetterTargets = useMemo(
    () =>
      new Set(
        queues.flatMap((entry) => (entry.deadLetterQueue != null ? [entry.deadLetterQueue] : [])),
      ),
    [queues],
  );
  const deadLettered = queues
    .filter((entry) => deadLetterTargets.has(entry.name))
    .reduce((total, entry) => total + (entry.visible ?? 0), 0);

  // Worst first, and only what is actually waiting: a page of zeroes is a page
  // nobody reads twice.
  const busiest = useMemo(
    () =>
      [...queues]
        .filter((entry) => (entry.depth ?? 0) > 0)
        .sort((left, right) => (right.depth ?? 0) - (left.depth ?? 0))
        .slice(0, 8),
    [queues],
  );

  return (
    <Page>
      <PageHeader
        title={t("shell.nav.sqs.overview")}
        subtitle={t("board.sqs.overview.subtitle", { region, queues: queues.length })}
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
              label={t("board.sqs.overview.queues")}
              value={formatCount(queues.length)}
              hint={t("board.sqs.overview.queuesHint")}
            />
            <StatTile
              label={t("board.sqs.overview.visible")}
              value={formatCount(visible)}
              hint={t("board.sqs.overview.visibleHint")}
            />
            <StatTile
              label={t("board.sqs.overview.inFlight")}
              value={formatCount(inFlight)}
              hint={t("board.sqs.overview.inFlightHint")}
            />
            <StatTile
              label={t("board.sqs.overview.delayed")}
              value={formatCount(delayed)}
              hint={t("board.sqs.overview.delayedHint")}
            />
            <StatTile
              label={t("board.sqs.overview.deadLettered")}
              value={formatCount(deadLettered)}
              hint={t("board.sqs.overview.deadLetteredHint")}
              valueColor={deadLettered > 0 ? "var(--c-warn-text)" : undefined}
            />
          </div>

          <Panel>
            <PanelHeader title={t("board.sqs.overview.busiest")} />
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t("board.sqs.queues.name")}</TableHead>
                  <TableHead className="num">{t("board.sqs.queues.visible")}</TableHead>
                  <TableHead className="num">{t("board.sqs.queues.inFlight")}</TableHead>
                  <TableHead className="num">{t("board.sqs.queues.delayed")}</TableHead>
                  <TableHead>{t("board.sqs.overview.state")}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {busiest.map((entry) => (
                  <TableRow key={entry.name}>
                    <TableCell>
                      <span className="mono3" style={MONO11}>
                        {entry.name}
                      </span>
                    </TableCell>
                    <TableCell className="num">
                      <span className="mono3" style={MONO11}>
                        {formatCount(entry.visible ?? 0)}
                      </span>
                    </TableCell>
                    <TableCell className="num">
                      <span className="mono3" style={MONO11}>
                        {formatCount(entry.inFlight ?? 0)}
                      </span>
                    </TableCell>
                    <TableCell className="num">
                      <span className="mono3" style={MONO11}>
                        {formatCount(entry.delayed ?? 0)}
                      </span>
                    </TableCell>
                    <TableCell>
                      <span style={{ fontSize: "11px", color: "var(--c-muted)" }}>
                        {deadLetterTargets.has(entry.name)
                          ? t("board.sqs.overview.stateDeadLetter")
                          : stalledInFlight(entry)
                            ? t("board.sqs.overview.stateStalled")
                            : t("board.sqs.overview.stateWaiting")}
                      </span>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </Panel>

          <p style={{ margin: "0 20px", fontSize: "11.5px", color: "var(--c-muted)" }}>
            {t("board.sqs.overview.note")}
          </p>
        </PageBody>
      </BoardState>
    </Page>
  );
}
