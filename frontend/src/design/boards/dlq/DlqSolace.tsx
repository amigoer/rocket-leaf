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
import { useSolaceDeadLetters } from "@/hooks/solace/useSolaceDeadLetters";
import {
  SOURCE_TOPIC_ENDPOINT,
  movesEverything,
  redeliveryLimit,
  silentSources,
  targetExists,
} from "@/mq/solace/deadletters";
import { formatCount } from "@/lib/format";
import type { DeadLetterQueue } from "@bindings/model/models";

const MONO11 = { fontSize: "11px" } as const;
const DASH = "—";

function count(value: number): string {
  return value < 0 ? DASH : formatCount(value);
}

/**
 * What points at this queue, as label-and-value pairs.
 *
 * A queue and a topic endpoint are labelled differently rather than merged,
 * because they are configured in different places and a reader fixing one has
 * to know which they are looking at. The redelivery limit sits beside each,
 * because it is what decides when a message travels the pointer at all.
 */
function sourceRows(
  queue: DeadLetterQueue,
  t: (key: string, values?: Record<string, unknown>) => string,
): [string, string][] {
  const rows: [string, string][] = [];
  for (const source of queue.sources ?? []) {
    if (source == null) continue;
    const limit = redeliveryLimit(source);
    const detail =
      limit > 0
        ? t("board.solace.dlq.afterTries", { count: limit })
        : t("board.solace.dlq.noLimit");
    const label =
      source.exchange === SOURCE_TOPIC_ENDPOINT
        ? t("board.solace.dlq.fromEndpoint")
        : t("board.solace.dlq.fromQueue");
    const marked = movesEverything(source) ? "" : ` · ${t("board.solace.dlq.markedOnly")}`;
    rows.push([label, `${source.queue} (${detail})${marked}`]);
  }
  return rows;
}

/**
 * Solace dead messages.
 *
 * There is no dead message queue object on this broker. What there is, on
 * every queue and every topic endpoint, is a pointer at the queue its
 * undelivered messages should go to - so this page is that pointer inverted,
 * and every row is an ordinary queue that something else happens to name.
 *
 * Two failures are what the page is for, and neither shows anywhere else.
 *
 * The first is a pointer at a queue that does not exist. Every endpoint ships
 * pointing at "#DEAD_MSG_QUEUE" and no broker creates a queue by that name, so
 * the ordinary state of an unconfigured Message VPN is that everything given
 * up on is discarded. Those rows are drawn with no depth and said out loud,
 * because a zero would read as an empty queue that is working.
 *
 * The second is a pointer that is never followed. A queue with
 * respectDmqEligible on - the default - moves only a message whose publisher
 * marked it eligible, and most clients mark nothing. So a row can point at a
 * real, empty queue and be wrong for a completely different reason.
 */
export function DlqSolace() {
  const { t } = useTranslation();
  const state = useSolaceDeadLetters();
  const [selected, setSelected] = useState<string | null>(null);

  const queues = useMemo(() => state.data ?? [], [state.data]);
  const detail = useMemo(
    () => queues.find((entry) => entry.name === selected) ?? queues[0] ?? null,
    [queues, selected],
  );

  return (
    <Page>
      <PageHeader
        title={t("shell.nav.solace.dlq")}
        subtitle={t("board.solace.dlq.count", { count: queues.length })}
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
                    <TableHead>{t("board.solace.dlq.name")}</TableHead>
                    <TableHead className="num">{t("board.solace.dlq.depth")}</TableHead>
                    <TableHead className="num">{t("board.solace.dlq.consumers")}</TableHead>
                    <TableHead className="num">{t("board.solace.dlq.sources")}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {queues.map((entry) => (
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
                        {!targetExists(entry) && (
                          <span
                            className="pb"
                            style={{
                              marginLeft: "6px",
                              background: "var(--c-warn-bg)",
                              color: "var(--c-warn)",
                            }}
                          >
                            {t("board.solace.dlq.missingBadge")}
                          </span>
                        )}
                      </TableCell>
                      <TableCell className="num">
                        <span
                          className="mono3"
                          style={{
                            ...MONO11,
                            color: entry.depth > 0 ? "var(--c-warn)" : undefined,
                          }}
                        >
                          {count(entry.depth)}
                        </span>
                      </TableCell>
                      <TableCell className="num">
                        <span className="mono3" style={MONO11}>
                          {count(entry.consumers)}
                        </span>
                      </TableCell>
                      <TableCell className="num">
                        <span className="mono3" style={MONO11}>
                          {(entry.sources ?? []).length}
                        </span>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </Panel>

            {detail != null && (
              <Panel style={{ width: "340px", overflow: "auto" }}>
                <PanelHeader title={detail.name} />
                <div style={{ padding: "10px 12px", display: "grid", gap: "8px" }}>
                  <KV
                    rows={[
                      [t("board.solace.dlq.msgVpn"), detail.namespace || DASH],
                      [t("board.solace.dlq.depth"), count(detail.depth)],
                      [t("board.solace.dlq.consumers"), count(detail.consumers)],
                    ]}
                  />

                  {!targetExists(detail) && (
                    <div style={{ fontSize: "11px", color: "var(--c-warn)" }}>
                      {t("board.solace.dlq.missingNote", { name: detail.name })}
                    </div>
                  )}
                  {targetExists(detail) && detail.depth > 0 && detail.consumers === 0 && (
                    <div style={{ fontSize: "11px", color: "var(--c-warn)" }}>
                      {t("board.solace.dlq.nobodyDraining")}
                    </div>
                  )}
                  {silentSources(detail).length > 0 && (
                    <div style={{ fontSize: "11px", color: "var(--c-warn)" }}>
                      {t("board.solace.dlq.markedOnlyNote", {
                        count: silentSources(detail).length,
                      })}
                    </div>
                  )}

                  <SectionLabel>{t("board.solace.dlq.section.sources")}</SectionLabel>
                  {(detail.sources ?? []).length === 0 ? (
                    <div style={{ fontSize: "11px", color: "var(--c-muted)" }}>
                      {t("board.solace.dlq.noSources")}
                    </div>
                  ) : (
                    <KV rows={sourceRows(detail, t)} />
                  )}
                </div>
              </Panel>
            )}
          </div>
          <div style={{ fontSize: "11px", color: "var(--c-muted)", paddingTop: "8px" }}>
            {t("board.solace.dlq.note")}
          </div>
        </PageBody>
      </BoardState>
    </Page>
  );
}
