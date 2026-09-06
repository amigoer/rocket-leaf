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
import { useGooglePubSubTopics } from "@/hooks/googlepubsub/useGooglePubSubTopics";
import { useGooglePubSubSubscriptions } from "@/hooks/googlepubsub/useGooglePubSubSubscriptions";
import { topic as readTopic } from "@/mq/googlepubsub/topics";
import { subscription as readSubscription } from "@/mq/googlepubsub/subscriptions";
import { useConnectionScope } from "@/mq/ConnectionScope";
import { useConnectionProfiles } from "@/hooks/useConnectionProfiles";
import { formatCount } from "@/lib/format";
import { KPI_GRID } from "./_shared";

const MONO11 = { fontSize: "11px" } as const;

/**
 * The Google Pub/Sub overview.
 *
 * Built from the topic and subscription listings and nothing else, because
 * that is the whole of what the admin API reports. There is no cluster panel,
 * no node table and no version: Google runs the service, and a topology board
 * would have one invented row on it. There is no throughput chart and no
 * backlog either - both live in Cloud Monitoring, a different API under a
 * different credential, and a figure derived from two samples here would be
 * this app's arithmetic presented as Google's.
 *
 * What the tiles show instead is where the fan-out is broken, because on this
 * family that is the failure with no other symptom. A topic with no
 * subscription accepts every publish and discards it. A subscription whose
 * topic is gone holds messages nothing will ever collect. Both report success
 * everywhere else, and both are counted here.
 */
export function OverviewGooglePubSub() {
  const { t } = useTranslation();
  const state = useGooglePubSubTopics();
  const subscriptionState = useGooglePubSubSubscriptions();
  const { id: connID } = useConnectionScope();
  const { profiles } = useConnectionProfiles();

  const topics = useMemo(() => (state.data ?? []).map(readTopic), [state.data]);
  const subscriptions = useMemo(
    () => (subscriptionState.data ?? []).map(readSubscription),
    [subscriptionState.data],
  );

  // The project is what an address would be on any other family, so it is what
  // the subtitle names. It is read off the profile because the listings do not
  // carry it - every resource path in them already begins with it.
  const project = useMemo(
    () => profiles.find((profile) => profile.id === connID)?.options?.projectId ?? "",
    [connID, profiles],
  );

  const unsubscribed = topics.filter((entry) => entry.subscribers === 0).length;
  const orphaned = subscriptions.filter((entry) => entry.orphaned).length;
  const withDeadLetters = subscriptions.filter((entry) => entry.deadLetterTopic != null).length;

  // Most-read first, and only topics something actually reads: a page of
  // zeroes is a page nobody reads twice, and the unsubscribed ones already
  // have a tile of their own.
  const busiest = useMemo(
    () =>
      [...topics]
        .filter((entry) => entry.subscribers > 0)
        .sort((left, right) => right.subscribers - left.subscribers)
        .slice(0, 8),
    [topics],
  );

  return (
    <Page>
      <PageHeader
        title={t("shell.nav.google-pubsub.overview")}
        subtitle={t("board.google-pubsub.overview.subtitle", { project, topics: topics.length })}
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
              label={t("board.google-pubsub.overview.topics")}
              value={formatCount(topics.length)}
              hint={t("board.google-pubsub.overview.topicsHint")}
            />
            <StatTile
              label={t("board.google-pubsub.overview.subscriptions")}
              value={formatCount(subscriptions.length)}
              hint={t("board.google-pubsub.overview.subscriptionsHint")}
            />
            <StatTile
              label={t("board.google-pubsub.overview.unsubscribed")}
              value={formatCount(unsubscribed)}
              hint={t("board.google-pubsub.overview.unsubscribedHint")}
              valueColor={unsubscribed > 0 ? "var(--c-warn-text)" : undefined}
            />
            <StatTile
              label={t("board.google-pubsub.overview.orphaned")}
              value={formatCount(orphaned)}
              hint={t("board.google-pubsub.overview.orphanedHint")}
              valueColor={orphaned > 0 ? "var(--c-warn-text)" : undefined}
            />
            <StatTile
              label={t("board.google-pubsub.overview.withDeadLetters")}
              value={formatCount(withDeadLetters)}
              hint={t("board.google-pubsub.overview.withDeadLettersHint")}
            />
          </div>

          <Panel>
            <PanelHeader title={t("board.google-pubsub.overview.busiest")} />
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t("board.google-pubsub.topics.name")}</TableHead>
                  <TableHead className="num">
                    {t("board.google-pubsub.topics.subscribers")}
                  </TableHead>
                  <TableHead>{t("board.google-pubsub.topics.subscriptionNames")}</TableHead>
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
                        {formatCount(entry.subscribers)}
                      </span>
                    </TableCell>
                    <TableCell>
                      <span className="mono3" style={MONO11}>
                        {entry.subscriptionNames.join(", ")}
                      </span>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </Panel>

          <p style={{ margin: "0 20px", fontSize: "11.5px", color: "var(--c-muted)" }}>
            {t("board.google-pubsub.overview.note")}
          </p>
        </PageBody>
      </BoardState>
    </Page>
  );
}
