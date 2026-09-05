import { useCallback, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { Page, PageBody, PageHeader, RefreshButton } from "@/design/shell";
import { Button } from "@/components/ui/button";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { KV, Panel, PanelHeader, SectionLabel, useConfirm } from "@/components";
import { BoardState } from "@/design/boards/BoardState";
import { useActiveMQDeadLetters } from "@/hooks/activemq/useActiveMQDeadLetters";
import { useActiveMQDestinations } from "@/hooks/activemq/useActiveMQDestinations";
import { destination as readDestination } from "@/mq/activemq/destinations";
import { formatCount } from "@/lib/format";
import { formatErrorMessage } from "@/lib/utils";
import { useConnectionScope } from "@/mq/ConnectionScope";
import * as activemqApi from "@/api/activemq";
import type { DeadLetterQueue } from "@bindings/model/models";

const MONO11 = { fontSize: "11px" } as const;
const DASH = "—";

/**
 * ActiveMQ dead letters.
 *
 * The first family in this app with the full shape. Kafka has no broker-side
 * dead-letter queue at all, NATS moves nothing and publishes an advisory
 * instead, Redis keeps a pending list because it gives up on nothing - so this
 * page has been half empty everywhere except RabbitMQ. ActiveMQ moves the
 * message to a destination the operator named and can put it back, which no
 * other family here can do.
 *
 * A dead-letter destination is an ordinary destination that something else
 * points at, so it is found by walking the declarations backwards rather than
 * by looking up a name. On Artemis every queue records the address its
 * undeliverable messages go to, so the page can say what feeds this one and,
 * more usefully, when nothing does any more - a backlog whose producers were
 * all deleted will never receive anything again and never drain. Classic
 * decides by a broker-wide policy and keeps no such record, so that column is
 * blank there rather than guessed at from names.
 */
export function DlqActiveMQ() {
  const { t } = useTranslation();
  const state = useActiveMQDeadLetters();
  const destinationState = useActiveMQDestinations();
  const { id: connID } = useConnectionScope();
  const confirm = useConfirm();
  const [selected, setSelected] = useState<string | null>(null);

  const queues = useMemo(() => state.data ?? [], [state.data]);
  const detail = useMemo(
    () => queues.find((entry) => entry.name === selected) ?? queues[0] ?? null,
    [queues, selected],
  );

  // Which product answered, so the page can explain an empty sources column
  // rather than leaving it looking like missing data.
  const product = useMemo(
    () => (destinationState.data ?? []).map(readDestination)[0]?.product ?? null,
    [destinationState.data],
  );

  const retry = useCallback(
    async (queue: DeadLetterQueue) => {
      const ok = await confirm({
        title: t("board.activemq.dlq.retryTitle", { name: queue.name }),
        description: t("board.activemq.dlq.retryDesc", { count: queue.depth }),
        confirmLabel: t("board.activemq.dlq.retry"),
      });
      if (!ok) return;
      try {
        const moved = await activemqApi.retryDeadLetters(connID, queue.name);
        toast.success(t("board.activemq.dlq.retried", { count: moved }));
        await state.refresh();
      } catch (retryError) {
        toast.error(t("board.activemq.dlq.retryFailed"), {
          description: formatErrorMessage(retryError),
        });
      }
    },
    [confirm, connID, state, t],
  );

  return (
    <Page>
      <PageHeader
        title={t("shell.nav.activemq.dlq")}
        subtitle={t("board.activemq.dlq.count", { count: queues.length })}
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
                    <TableHead>{t("board.activemq.dlq.name")}</TableHead>
                    <TableHead className="num">{t("board.activemq.dlq.depth")}</TableHead>
                    <TableHead className="num">{t("board.activemq.dlq.consumers")}</TableHead>
                    <TableHead className="num">{t("board.activemq.dlq.sources")}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {queues.map((queue) => (
                    <TableRow
                      key={queue.name}
                      data-state={detail?.name === queue.name ? "selected" : undefined}
                      onClick={() => setSelected(queue.name)}
                      style={{ cursor: "pointer" }}
                    >
                      <TableCell>
                        <span className="mono3" style={MONO11}>
                          {queue.name}
                        </span>
                      </TableCell>
                      <TableCell className="num">
                        <span className="mono3" style={MONO11}>
                          {formatCount(Number(queue.depth))}
                        </span>
                      </TableCell>
                      <TableCell className="num">
                        <span className="mono3" style={MONO11}>
                          {formatCount(queue.consumers)}
                        </span>
                      </TableCell>
                      <TableCell className="num">
                        <span className="mono3" style={MONO11}>
                          {/*
                            Blank rather than 0 on Classic: nothing was
                            counted, as against nothing pointing here.
                          */}
                          {product === "classic" ? DASH : formatCount(queue.sources?.length ?? 0)}
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
                  <SectionLabel>{t("board.activemq.dlq.section.state")}</SectionLabel>
                  <KV
                    rows={[
                      [t("board.activemq.dlq.depth"), formatCount(Number(detail.depth))],
                      [t("board.activemq.dlq.consumers"), formatCount(detail.consumers)],
                    ]}
                  />

                  <SectionLabel>{t("board.activemq.dlq.section.sources")}</SectionLabel>
                  {product === "classic" ? (
                    <p style={{ fontSize: "11px", color: "var(--c-muted)" }}>
                      {t("board.activemq.dlq.noSourcesClassic")}
                    </p>
                  ) : (detail.sources?.length ?? 0) === 0 ? (
                    <p style={{ fontSize: "11px", color: "var(--c-muted)" }}>
                      {t("board.activemq.dlq.noSourcesArtemis")}
                    </p>
                  ) : (
                    <KV
                      rows={(detail.sources ?? [])
                        .filter((source) => source != null)
                        .map((source) => [
                          source.queue,
                          // Which reader of the source gave up, where the
                          // family attaches the policy to a subscriber. Both
                          // ActiveMQ products attach it to the destination, so
                          // this is blank here and the column is kept because
                          // the model is shared.
                          source.subscription === "" ? DASH : source.subscription,
                        ])}
                    />
                  )}

                  <Button
                    size="sm"
                    style={{ marginTop: "12px" }}
                    disabled={Number(detail.depth) === 0}
                    onClick={() => void retry(detail)}
                  >
                    {t("board.activemq.dlq.retry")}
                  </Button>
                  <p
                    style={{ marginTop: "8px", fontSize: "11px", color: "var(--c-muted)" }}
                  >
                    {t("board.activemq.dlq.retryNote")}
                  </p>
                </div>
              </Panel>
            )}
          </div>
        </PageBody>
      </BoardState>
    </Page>
  );
}
