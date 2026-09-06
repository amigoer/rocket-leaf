import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { Page, PageBody, PageHeader, RefreshButton } from "@/design/shell";
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
import { useGooglePubSubDeadLetters } from "@/hooks/googlepubsub/useGooglePubSubDeadLetters";
import { formatCount } from "@/lib/format";
import type { DeadLetterQueue, DeadLetterSource } from "@bindings/model/models";

const MONO11 = { fontSize: "11px" } as const;
const DASH = "—";

function count(value: number): string {
  return value < 0 ? DASH : formatCount(value);
}

/** The bridge types every element as nullable; a source with no topic is not one. */
function sourcesOf(entry: DeadLetterQueue): DeadLetterSource[] {
  return (entry.sources ?? []).flatMap((source) => (source?.queue ? [source] : []));
}

/**
 * Google Pub/Sub dead letters.
 *
 * A dead-letter topic here is an ordinary topic that some subscription's
 * policy points at. Nothing marks one, so the set is found by walking the
 * topology backwards, and reading one afterwards means browsing a subscription
 * on it like any other.
 *
 * The sources column names two things rather than one, and that is this
 * family rather than a flourish: the policy belongs to the subscription, so
 * one topic read by three subscriptions can give up into three different
 * places for three different reasons. Knowing which reader stopped trying is
 * most of what makes the row actionable.
 *
 * There is no depth column, because a topic holds nothing countable - the
 * dead letters are waiting on whatever subscribes to it, and each of those
 * backlogs is a Cloud Monitoring metric. What replaces it is the subscription
 * count, and zero is the row to act on: a dead letter published to a topic
 * nothing subscribes to is discarded on arrival, which makes the messages a
 * system gave up on the ones nobody notices disappearing.
 */
export function DlqGooglePubSub() {
  const { t } = useTranslation();
  const state = useGooglePubSubDeadLetters();
  const [selected, setSelected] = useState<string | null>(null);

  const topics = useMemo(() => state.data ?? [], [state.data]);
  const detail = useMemo(
    () => topics.find((entry) => entry.name === selected) ?? topics[0] ?? null,
    [topics, selected],
  );

  return (
    <Page>
      <PageHeader
        title={t("shell.nav.google-pubsub.dlq")}
        subtitle={t("board.google-pubsub.dlq.count", { count: topics.length })}
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
          <div style={{ display: "flex", gap: "12px", minHeight: 0, flex: 1 }}>
            <Panel style={{ flex: 1, overflow: "auto" }}>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t("board.google-pubsub.dlq.name")}</TableHead>
                    <TableHead className="num">{t("board.google-pubsub.dlq.readers")}</TableHead>
                    <TableHead>{t("board.google-pubsub.dlq.sources")}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {topics.map((entry) => (
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
                        {/* Nothing reads it, so every dead letter that arrives
                            is thrown away on the spot. */}
                        {entry.consumers === 0 && (
                          <span className="pb pGPS" style={{ marginLeft: "6px" }}>
                            {t("board.google-pubsub.dlq.noReaderBadge")}
                          </span>
                        )}
                      </TableCell>
                      <TableCell className="num">
                        <span className="mono3" style={MONO11}>
                          {count(entry.consumers)}
                        </span>
                      </TableCell>
                      <TableCell>
                        <span className="mono3" style={MONO11}>
                          {sourcesOf(entry).length === 0
                            ? t("board.google-pubsub.dlq.noSources")
                            : sourcesOf(entry)
                                .map((source) => `${source.queue} · ${source.subscription}`)
                                .join(", ")}
                        </span>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </Panel>

            {detail != null && (
              <Panel style={{ width: "320px", flex: "none", overflow: "auto" }}>
                <PanelHeader title={detail.name} />
                <div style={{ padding: "0 12px 12px" }}>
                  <KV
                    rows={[
                      [t("board.google-pubsub.dlq.readers"), count(detail.consumers)],
                      [
                        t("board.google-pubsub.dlq.sourceCount"),
                        String(sourcesOf(detail).length),
                      ],
                    ]}
                  />

                  {detail.consumers === 0 && (
                    <p style={{ fontSize: "11px", color: "var(--c-warn-text)", margin: "8px 0 0" }}>
                      {t("board.google-pubsub.dlq.noReaderNote")}
                    </p>
                  )}

                  <SectionLabel>{t("board.google-pubsub.dlq.sources")}</SectionLabel>
                  {sourcesOf(detail).length === 0 ? (
                    <p style={{ fontSize: "11px", color: "var(--c-warn-text)", margin: 0 }}>
                      {/* The state worth naming: nothing points here any more,
                          so it will never receive anything again. */}
                      {t("board.google-pubsub.dlq.orphaned")}
                    </p>
                  ) : (
                    <KV
                      rows={sourcesOf(detail).map((source) => [
                        source.subscription,
                        t("board.google-pubsub.dlq.givesUpFrom", { topic: source.queue }),
                      ])}
                    />
                  )}
                </div>
              </Panel>
            )}
          </div>
          <p
            style={{
              margin: "0 20px",
              fontSize: "11.5px",
              color: "var(--c-muted)",
              flex: "none",
            }}
          >
            {t("board.google-pubsub.dlq.note")}
          </p>
        </PageBody>
      </BoardState>
    </Page>
  );
}
