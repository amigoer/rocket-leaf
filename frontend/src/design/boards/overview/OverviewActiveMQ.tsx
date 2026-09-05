import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import { Page, PageBody, PageHeader, RefreshButton } from "@/design/shell";
import { KV, Panel, PanelHeader, StatTile } from "@/components";
import { BoardState } from "@/design/boards/BoardState";
import { useActiveMQNodes } from "@/hooks/activemq/useActiveMQCluster";
import { useActiveMQDestinations } from "@/hooks/activemq/useActiveMQDestinations";
import { useActiveMQSubscriptions } from "@/hooks/activemq/useActiveMQSubscriptions";
import { node as readNode } from "@/mq/activemq/cluster";
import { destination as readDestination } from "@/mq/activemq/destinations";
import { subscription as readSubscription } from "@/mq/activemq/subscriptions";
import { formatCount } from "@/lib/format";
import { KPI_GRID } from "./_shared";

const DASH = "—";

function reported(value: number | null): string {
  return value == null ? DASH : formatCount(value);
}

/**
 * The ActiveMQ overview.
 *
 * No rate and no throughput chart, and the reason is the family's: both
 * products keep cumulative enqueue and dequeue counters and report no rate at
 * all. Dividing two samples of a counter would be this app's arithmetic
 * printed as the broker's figure - the same call the Kafka overview makes, for
 * the same reason.
 *
 * What replaces them is the thing a JMS operator actually opens this page for:
 * how much is sitting undelivered, and how much of it is owed to a subscriber
 * that is not connected. A queue with a depth and no consumer and a topic with
 * two detached durable subscriptions are different problems, and the tiles
 * separate them.
 *
 * The product is named on the page rather than assumed. One MQKind covers two
 * brokers that report different things, so a reader needs to know which one
 * they are looking at before an empty column means anything.
 */
export function OverviewActiveMQ() {
  const { t } = useTranslation();
  const state = useActiveMQNodes();
  const destinationState = useActiveMQDestinations();
  const subscriptionState = useActiveMQSubscriptions();

  const broker = useMemo(() => {
    const nodes = (state.data ?? []).map(readNode);
    return nodes.find((entry) => !entry.bridge) ?? nodes[0] ?? null;
  }, [state.data]);

  const destinations = useMemo(
    () => (destinationState.data ?? []).map(readDestination),
    [destinationState.data],
  );
  const subscriptions = useMemo(
    () => (subscriptionState.data ?? []).map(readSubscription),
    [subscriptionState.data],
  );

  const queues = destinations.filter((entry) => entry.kind === "queue");
  const topics = destinations.filter((entry) => entry.kind === "topic");
  const held = destinations.reduce((total, entry) => total + (entry.depth ?? 0), 0);
  // Undrained is the figure worth a tile: a depth with nobody attached is a
  // backlog nobody is working through, which a total depth alone hides.
  const undrained = queues
    .filter((entry) => (entry.consumers ?? 0) === 0)
    .reduce((total, entry) => total + (entry.depth ?? 0), 0);
  const detached = subscriptions.filter((entry) => !entry.active).length;

  return (
    <Page>
      <PageHeader
        title={t("shell.nav.activemq.overview")}
        subtitle={
          broker == null
            ? undefined
            : t("board.activemq.overview.subtitle", {
                product: t(`board.activemq.overview.product.${broker.product}`),
                version: broker.version,
              })
        }
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
              label={t("board.activemq.overview.queues")}
              value={formatCount(queues.length)}
            />
            <StatTile
              label={t("board.activemq.overview.topics")}
              value={formatCount(topics.length)}
            />
            <StatTile
              label={t("board.activemq.overview.held")}
              value={formatCount(held)}
            />
            <StatTile
              label={t("board.activemq.overview.undrained")}
              value={formatCount(undrained)}
              hint={t("board.activemq.overview.undrainedHint")}
            />
            <StatTile
              label={t("board.activemq.overview.subscriptions")}
              value={formatCount(subscriptions.length)}
            />
            <StatTile
              label={t("board.activemq.overview.detached")}
              value={formatCount(detached)}
              hint={t("board.activemq.overview.detachedHint")}
            />
          </div>

          {broker != null && (
            <Panel style={{ overflow: "auto" }}>
              <PanelHeader title={broker.name} />
              <div style={{ padding: "0 12px 12px" }}>
                <KV
                  rows={[
                    [t("board.activemq.overview.version"), broker.version || DASH],
                    [t("board.activemq.overview.uptime"), broker.uptime ?? DASH],
                    [
                      t("board.activemq.overview.connections"),
                      reported(broker.connections),
                    ],
                    [t("board.activemq.overview.consumers"), reported(broker.consumers)],
                    [
                      t("board.activemq.overview.disk"),
                      broker.diskUsage == null ? DASH : `${broker.diskUsage}%`,
                    ],
                    [
                      t("board.activemq.overview.memory"),
                      broker.memoryPercent == null ? DASH : `${broker.memoryPercent}%`,
                    ],
                    [
                      t("board.activemq.overview.persistent"),
                      broker.persistent == null
                        ? DASH
                        : broker.persistent
                          ? t("common.yes")
                          : t("common.no"),
                    ],
                  ]}
                />
              </div>
            </Panel>
          )}

          <p
            style={{
              margin: "0 20px",
              fontSize: "11.5px",
              color: "var(--c-muted)",
              flex: "none",
            }}
          >
            {t("board.activemq.overview.note")}
          </p>
        </PageBody>
      </BoardState>
    </Page>
  );
}
