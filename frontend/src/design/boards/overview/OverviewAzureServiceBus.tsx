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
import { useServiceBusEntities } from "@/hooks/azureservicebus/useServiceBusEntities";
import { useServiceBusSubscriptions } from "@/hooks/azureservicebus/useServiceBusSubscriptions";
import { entity as readEntity, isDisabled } from "@/mq/azureservicebus/entities";
import {
  receivesNothing,
  subscription as readSubscription,
} from "@/mq/azureservicebus/subscriptions";
import { useConnectionScope } from "@/mq/ConnectionScope";
import { useConnectionProfiles } from "@/hooks/useConnectionProfiles";
import { formatCount } from "@/lib/format";
import { KPI_GRID } from "./_shared";

const MONO11 = { fontSize: "11px" } as const;
const DASH = "—";

/**
 * The Azure Service Bus overview.
 *
 * Built from the entity and subscription listings and nothing else, because
 * that is the whole of what the management API reports. There is no cluster
 * panel, no node table and no version: Microsoft runs the service, and a
 * topology board would have one invented row on it. There is no throughput
 * chart either - the rates live in Azure Monitor, a different API under a
 * different credential, and a figure derived from two samples here would be
 * this app's arithmetic presented as Microsoft's.
 *
 * What the tiles show instead is where the routing is broken, because on this
 * family those are the faults with no other symptom. A topic with no
 * subscription accepts every send and discards it. A subscription with no
 * rules receives nothing while reporting itself Active with an empty backlog.
 * An entity with sends or receives switched off refuses work at the client and
 * leaves no mark on any board. All three report success everywhere else, and
 * all three are counted here.
 */
export function OverviewAzureServiceBus() {
  const { t } = useTranslation();
  const state = useServiceBusEntities();
  const subscriptionState = useServiceBusSubscriptions();
  const { id: connID } = useConnectionScope();
  const { profiles } = useConnectionProfiles();

  const entities = useMemo(() => (state.data ?? []).map(readEntity), [state.data]);
  const subscriptions = useMemo(
    () => (subscriptionState.data ?? []).map(readSubscription),
    [subscriptionState.data],
  );

  // The namespace is a real address on this family, unlike the region and the
  // project the two hosted families before it show. It is read off the profile
  // because the listings do not carry it: everything in them is already inside
  // it.
  const namespace = useMemo(
    () => profiles.find((profile) => profile.id === connID)?.endpoints ?? "",
    [connID, profiles],
  );

  const queues = entities.filter((row) => row.kind === "queue").length;
  const topics = entities.filter((row) => row.kind === "topic").length;
  const unsubscribed = entities.filter(
    (row) => row.kind === "topic" && row.subscribers === 0,
  ).length;
  const unroutable = subscriptions.filter(receivesNothing).length;
  const disabled =
    entities.filter(isDisabled).length +
    subscriptions.filter((row) => row.status != null && row.status !== "Active").length;

  // Most-read first, and only topics something actually reads: a page of
  // zeroes is a page nobody reads twice, and the unsubscribed ones already
  // have a tile of their own.
  const busiest = useMemo(
    () =>
      entities
        .filter((row) => row.kind === "topic" && (row.subscribers ?? 0) > 0)
        .sort((left, right) => (right.subscribers ?? 0) - (left.subscribers ?? 0))
        .slice(0, 8),
    [entities],
  );

  return (
    <Page>
      <PageHeader
        title={t("shell.nav.azure-servicebus.overview")}
        subtitle={t("board.azure-servicebus.overview.subtitle", {
          namespace,
          entities: entities.length,
        })}
        actions={
          <RefreshButton
            refreshing={state.refreshing}
            online={state.online}
            onClick={() => {
              void state.refresh();
              void subscriptionState.refresh();
            }}
          />
        }
      />
      <BoardState state={state}>
        <PageBody>
          <div className={KPI_GRID}>
            <StatTile
              label={t("board.azure-servicebus.overview.queues")}
              value={formatCount(queues)}
              hint={t("board.azure-servicebus.overview.queuesHint")}
            />
            <StatTile
              label={t("board.azure-servicebus.overview.topics")}
              value={formatCount(topics)}
              hint={t("board.azure-servicebus.overview.topicsHint")}
            />
            <StatTile
              label={t("board.azure-servicebus.overview.subscriptions")}
              value={formatCount(subscriptions.length)}
              hint={t("board.azure-servicebus.overview.subscriptionsHint")}
            />
            <StatTile
              label={t("board.azure-servicebus.overview.unsubscribed")}
              value={formatCount(unsubscribed)}
              hint={t("board.azure-servicebus.overview.unsubscribedHint")}
              valueColor={unsubscribed > 0 ? "var(--c-warn-text)" : undefined}
            />
            <StatTile
              label={t("board.azure-servicebus.overview.unroutable")}
              value={formatCount(unroutable)}
              hint={t("board.azure-servicebus.overview.unroutableHint")}
              valueColor={unroutable > 0 ? "var(--c-warn-text)" : undefined}
            />
            <StatTile
              label={t("board.azure-servicebus.overview.disabled")}
              value={formatCount(disabled)}
              hint={t("board.azure-servicebus.overview.disabledHint")}
              valueColor={disabled > 0 ? "var(--c-warn-text)" : undefined}
            />
          </div>

          <Panel>
            <PanelHeader title={t("board.azure-servicebus.overview.busiest")} />
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t("board.azure-servicebus.entities.name")}</TableHead>
                  <TableHead className="num">
                    {t("board.azure-servicebus.entities.subscribers")}
                  </TableHead>
                  <TableHead>{t("board.azure-servicebus.entities.subscriptionNames")}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {busiest.map((row) => (
                  <TableRow key={row.name}>
                    <TableCell>
                      <span className="mono3" style={MONO11}>
                        {row.name}
                      </span>
                    </TableCell>
                    <TableCell className="num">
                      <span className="mono3" style={MONO11}>
                        {formatCount(row.subscribers ?? 0)}
                      </span>
                    </TableCell>
                    <TableCell>
                      <span className="mono3" style={MONO11}>
                        {row.subscriptionNames.join(", ") || DASH}
                      </span>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </Panel>

          <p style={{ margin: "0 20px", fontSize: "11.5px", color: "var(--c-muted)" }}>
            {t("board.azure-servicebus.overview.note")}
          </p>
        </PageBody>
      </BoardState>
    </Page>
  );
}
