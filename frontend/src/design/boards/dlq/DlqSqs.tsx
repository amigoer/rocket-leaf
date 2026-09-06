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
import { useSqsDeadLetters } from "@/hooks/sqs/useSqsDeadLetters";
import { formatCount } from "@/lib/format";

const MONO11 = { fontSize: "11px" } as const;
const DASH = "—";

function count(value: number): string {
  return value < 0 ? DASH : formatCount(value);
}

/**
 * Amazon SQS dead letters.
 *
 * A dead-letter queue here is an ordinary queue another queue's redrive policy
 * points at. Nothing marks one, nothing is named after a consumer group -
 * there are none - so the set is found by walking the topology backwards, and
 * reading one afterwards is an ordinary browse on the messages page.
 *
 * The sources column is the reason to open this page rather than the queues
 * board. A dead-letter queue with a backlog and no sources left is one whose
 * producers were deleted or reconfigured: it will never receive anything
 * again, and nothing else in the app says so.
 *
 * There is no consumer count and no retry button. SQS keeps no record of who
 * reads a queue, so a zero would be an invention; and its redrive is a
 * background task that moves a whole queue back to wherever each message came
 * from, which is a different gesture from handing one message to one reader.
 */
export function DlqSqs() {
  const { t } = useTranslation();
  const state = useSqsDeadLetters();
  const [selected, setSelected] = useState<string | null>(null);

  const queues = useMemo(() => state.data ?? [], [state.data]);
  // The bridge types every element as nullable; a source row with nothing in
  // it is not a source, so it is dropped rather than drawn as a blank name.
  const sourcesOf = (entry: (typeof queues)[number]): string[] =>
    (entry.sources ?? []).flatMap((source) => (source?.queue ? [source.queue] : []));
  const detail = useMemo(
    () => queues.find((entry) => entry.name === selected) ?? queues[0] ?? null,
    [queues, selected],
  );

  return (
    <Page>
      <PageHeader
        title={t("shell.nav.sqs.dlq")}
        subtitle={t("board.sqs.dlq.count", { count: queues.length })}
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
                    <TableHead>{t("board.sqs.dlq.name")}</TableHead>
                    <TableHead className="num">{t("board.sqs.dlq.depth")}</TableHead>
                    <TableHead>{t("board.sqs.dlq.sources")}</TableHead>
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
                        <span className="mono3" style={MONO11}>
                          {count(entry.depth)}
                        </span>
                      </TableCell>
                      <TableCell>
                        <span className="mono3" style={MONO11}>
                          {sourcesOf(entry).length === 0
                            ? t("board.sqs.dlq.noSources")
                            : sourcesOf(entry).join(", ")}
                        </span>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </Panel>

            {detail != null && (
              <Panel style={{ width: "300px", flex: "none", overflow: "auto" }}>
                <PanelHeader title={detail.name} />
                <div style={{ padding: "0 12px 12px" }}>
                  <KV
                    rows={[
                      [t("board.sqs.dlq.depth"), count(detail.depth)],
                      [t("board.sqs.dlq.sourceCount"), String(sourcesOf(detail).length)],
                    ]}
                  />

                  <SectionLabel>{t("board.sqs.dlq.sources")}</SectionLabel>
                  {sourcesOf(detail).length === 0 ? (
                    <p style={{ fontSize: "11px", color: "var(--c-warn-text)", margin: 0 }}>
                      {/* The state worth naming: a backlog whose producers are
                          gone will never receive anything again and will never
                          drain by itself. */}
                      {t("board.sqs.dlq.orphaned")}
                    </p>
                  ) : (
                    <KV
                      rows={sourcesOf(detail).map((source) => [
                        source,
                        t("board.sqs.dlq.redrivesHere"),
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
            {t("board.sqs.dlq.note")}
          </p>
        </PageBody>
      </BoardState>
    </Page>
  );
}
