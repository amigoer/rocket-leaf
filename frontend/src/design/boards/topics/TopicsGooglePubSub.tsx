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
import { useGooglePubSubTopics } from "@/hooks/googlepubsub/useGooglePubSubTopics";
import {
  discardsEverything,
  topic as readTopic,
  type PubSubTopic,
} from "@/mq/googlepubsub/topics";
import { formatCount } from "@/lib/format";

const MONO11 = { fontSize: "11px" } as const;

/** A dash, not a zero: an absent figure and a measured zero are not alike. */
const DASH = "—";

/** Seconds as the service stores them, read back as something human. */
function duration(seconds: number | null): string {
  if (seconds == null) return DASH;
  if (seconds < 60) return `${seconds}s`;
  if (seconds < 3600) return `${Math.round(seconds / 60)}m`;
  if (seconds < 86400) return `${Math.round(seconds / 3600)}h`;
  return `${Math.round(seconds / 86400)}d`;
}

/**
 * Google Pub/Sub topics.
 *
 * One row per topic in the project, narrowed by the connection's prefix.
 *
 * There is no depth column and there cannot be one. A topic holds nothing: a
 * publish is fanned out to whatever subscriptions exist at that instant, and
 * anything a subscription has not acknowledged is that subscription's backlog
 * rather than the topic's. So the count this board leads with is how many
 * subscriptions read the topic - the one figure that separates a topic doing
 * its job from one accepting every message and throwing it away.
 *
 * There is no rate column and no chart either. Pub/Sub publishes its rates to
 * Cloud Monitoring, which is a different API under a different credential, and
 * a figure derived from two samples here would be this app's arithmetic
 * presented as Google's.
 */
export function TopicsGooglePubSub() {
  const { t } = useTranslation();
  const state = useGooglePubSubTopics();
  const [search, setSearch] = useState("");
  const [selected, setSelected] = useState<string | null>(null);

  const topics = useMemo(() => (state.data ?? []).map(readTopic), [state.data]);

  const shown = useMemo(() => {
    const needle = search.trim().toLowerCase();
    return topics.filter((entry) => needle === "" || entry.name.toLowerCase().includes(needle));
  }, [topics, search]);

  const detail = useMemo(
    () => shown.find((entry) => entry.name === selected) ?? shown[0] ?? null,
    [shown, selected],
  );

  return (
    <Page>
      <PageHeader
        title={t("shell.nav.google-pubsub.topics")}
        subtitle={t("board.google-pubsub.topics.count", { count: topics.length })}
        actions={
          <div style={{ display: "flex", gap: "8px", alignItems: "center" }}>
            <Input
              style={{ width: "200px" }}
              value={search}
              placeholder={t("board.google-pubsub.topics.search")}
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
                    <TableHead>{t("board.google-pubsub.topics.name")}</TableHead>
                    <TableHead className="num">
                      {t("board.google-pubsub.topics.subscribers")}
                    </TableHead>
                    <TableHead>{t("board.google-pubsub.topics.retention")}</TableHead>
                    <TableHead>{t("board.google-pubsub.topics.subscriptionNames")}</TableHead>
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
                        {/* Said on the row rather than only in the panel: it
                            is the one state that looks healthy everywhere
                            else, because a discarded message leaves no
                            backlog behind it. */}
                        {discardsEverything(entry) && (
                          <span className="pb pGPS" style={{ marginLeft: "6px" }}>
                            {t("board.google-pubsub.topics.noSubscriberBadge")}
                          </span>
                        )}
                      </TableCell>
                      <TableCell className="num">
                        <span className="mono3" style={MONO11}>
                          {formatCount(entry.subscribers)}
                        </span>
                      </TableCell>
                      <TableCell>
                        <span className="mono3" style={MONO11}>
                          {duration(entry.retentionSec)}
                        </span>
                      </TableCell>
                      <TableCell>
                        <span className="mono3" style={MONO11}>
                          {entry.subscriptionNames.join(", ") || DASH}
                        </span>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </Panel>

            {detail != null && <TopicDetail entry={detail} />}
          </div>
          <p
            style={{
              margin: "0 20px",
              fontSize: "11.5px",
              color: "var(--c-muted)",
              flex: "none",
            }}
          >
            {t("board.google-pubsub.topics.note")}
          </p>
        </PageBody>
      </BoardState>
    </Page>
  );
}

function TopicDetail({ entry }: { entry: PubSubTopic }) {
  const { t } = useTranslation();
  return (
    <Panel style={{ width: "300px", flex: "none", overflow: "auto" }}>
      <PanelHeader title={entry.name} />
      <div style={{ padding: "0 12px 12px" }}>
        <SectionLabel>{t("board.google-pubsub.topics.section.delivery")}</SectionLabel>
        <KV
          rows={[
            [t("board.google-pubsub.topics.subscribers"), formatCount(entry.subscribers)],
            [
              t("board.google-pubsub.topics.subscriptionNames"),
              entry.subscriptionNames.join(", ") || DASH,
            ],
            [t("board.google-pubsub.topics.retention"), duration(entry.retentionSec)],
          ]}
        />

        <SectionLabel>{t("board.google-pubsub.topics.section.identity")}</SectionLabel>
        <KV
          rows={[
            [t("board.google-pubsub.topics.path"), entry.path ?? DASH],
            [t("board.google-pubsub.topics.state"), entry.state ?? DASH],
            [t("board.google-pubsub.topics.schema"), entry.schema ?? DASH],
            [t("board.google-pubsub.topics.schemaEncoding"), entry.schemaEncoding ?? DASH],
            [t("board.google-pubsub.topics.kmsKey"), entry.kmsKey ?? DASH],
            [
              t("board.google-pubsub.topics.storageRegions"),
              entry.storageRegions.join(", ") || DASH,
            ],
          ]}
        />

        {entry.labels.length > 0 && (
          <>
            <SectionLabel>{t("board.google-pubsub.topics.section.labels")}</SectionLabel>
            <KV rows={entry.labels} />
          </>
        )}

        {/* Said where the number is, because a topic nothing subscribes to
            reports success on every publish and leaves nothing behind. */}
        {discardsEverything(entry) && (
          <p style={{ marginTop: "10px", fontSize: "11px", color: "var(--c-warn)" }}>
            {t("board.google-pubsub.topics.noSubscriberNote")}
          </p>
        )}
      </div>
    </Panel>
  );
}
