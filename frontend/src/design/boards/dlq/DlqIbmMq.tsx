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
import { useIbmMqDeadLetters } from "@/hooks/ibmmq/useIbmMqDeadLetters";
import { formatCount } from "@/lib/format";
import type { DeadLetterQueue } from "@bindings/model/models";

const MONO11 = { fontSize: "11px" } as const;
const DASH = "—";

/** The attribute names the driver puts in a source's exchange field. */
const DEADQ = "DEADQ";

function count(value: number): string {
  return value < 0 ? DASH : formatCount(value);
}

/**
 * What points at this queue, as label-and-value pairs.
 *
 * The two pointers are labelled differently rather than merged. The queue
 * manager fills its own DEADQ; a backout queue is filled by whichever
 * application gave up after the threshold, so the threshold belongs beside it
 * - a backout queue whose threshold is zero receives nothing at all.
 */
function sourceRows(
  queue: DeadLetterQueue,
  t: (key: string) => string,
): [string, string][] {
  const rows: [string, string][] = [];
  for (const source of queue.sources ?? []) {
    if (source == null) continue;
    if (source.exchange === DEADQ) {
      rows.push([t("board.ibmmq.dlq.fromQueueManager"), source.queue]);
      continue;
    }
    const threshold =
      source.routingKey === "" || source.routingKey === "0"
        ? t("board.ibmmq.dlq.thresholdNone")
        : source.routingKey;
    rows.push([
      t("board.ibmmq.dlq.fromQueue"),
      `${source.queue} (${t("board.ibmmq.dlq.threshold")} ${threshold})`,
    ]);
  }
  return rows;
}

/**
 * IBM MQ dead letters.
 *
 * Nothing on this queue manager is marked as a dead-letter queue, so the page
 * is built by inverting configuration: the queue manager's DEADQ attribute
 * names one, and every queue's backout queue names another. That is why the
 * sources column is the important one - a dead-letter queue with no source
 * would not be on this page at all, and one whose only source was deleted will
 * never receive anything again and never drain.
 *
 * The two pointers are told apart rather than merged, because they are filled
 * by different things. The queue manager fills its own DEADQ, with anything it
 * could not deliver - a message for a queue that is full, put-inhibited or
 * gone. A backout queue is filled by whichever application decided to give up
 * after the queue's backout threshold, so a threshold of zero means messages
 * never travel that pointer at all.
 *
 * There is no message list here, and that is not an omission. A dead letter
 * carries a dead-letter header in front of its payload, so the queue manager
 * stores it as MQDEAD and the REST interface will not decode it - the messages
 * page lists them with their identifiers and says so.
 */
export function DlqIbmMq() {
  const { t } = useTranslation();
  const state = useIbmMqDeadLetters();
  const [selected, setSelected] = useState<string | null>(null);

  const queues = useMemo(() => state.data ?? [], [state.data]);
  const detail = useMemo(
    () => queues.find((entry) => entry.name === selected) ?? queues[0] ?? null,
    [queues, selected],
  );

  return (
    <Page>
      <PageHeader
        title={t("shell.nav.ibmmq.dlq")}
        subtitle={t("board.ibmmq.dlq.count", { count: queues.length })}
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
                    <TableHead>{t("board.ibmmq.dlq.queue")}</TableHead>
                    <TableHead className="num">{t("board.ibmmq.dlq.depth")}</TableHead>
                    <TableHead className="num">{t("board.ibmmq.dlq.consumers")}</TableHead>
                    <TableHead className="num">{t("board.ibmmq.dlq.sources")}</TableHead>
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
                          {entry.consumers}
                        </span>
                      </TableCell>
                      <TableCell className="num">
                        <span className="mono3" style={MONO11}>
                          {entry.sources?.length ?? 0}
                        </span>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </Panel>

            {detail != null && (
              <Panel style={{ width: "320px", overflow: "auto" }}>
                <PanelHeader title={detail.name} />
                <div style={{ padding: "10px 12px", display: "grid", gap: "8px" }}>
                  <KV
                    rows={[
                      [t("board.ibmmq.dlq.depth"), count(detail.depth)],
                      [t("board.ibmmq.dlq.consumers"), String(detail.consumers)],
                    ]}
                  />
                  {detail.depth > 0 && detail.consumers === 0 && (
                    <div style={{ fontSize: "11px", color: "var(--c-warn)" }}>
                      {t("board.ibmmq.dlq.nobodyDraining")}
                    </div>
                  )}

                  <SectionLabel>{t("board.ibmmq.dlq.section.sources")}</SectionLabel>
                  <KV rows={sourceRows(detail, t)} />
                </div>
              </Panel>
            )}
          </div>
          <div style={{ fontSize: "11px", color: "var(--c-muted)", paddingTop: "8px" }}>
            {t("board.ibmmq.dlq.note")}
          </div>
        </PageBody>
      </BoardState>
    </Page>
  );
}
