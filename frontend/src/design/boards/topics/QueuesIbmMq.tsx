import { useCallback, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { Page, PageBody, PageHeader, RefreshButton } from "@/design/shell";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
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
import { useIbmMqDestinations } from "@/hooks/ibmmq/useIbmMqDestinations";
import {
  destination as readDestination,
  inhibited,
  type IbmMqDestination,
} from "@/mq/ibmmq/destinations";
import { formatCount } from "@/lib/format";
import { formatErrorMessage } from "@/lib/utils";
import { useConnectionScope } from "@/mq/ConnectionScope";
import * as ibmmqApi from "@/api/ibmmq";
import { QueueDialogIbmMq } from "./QueueDialogIbmMq";

const MONO11 = { fontSize: "11px" } as const;

/** A dash, not a zero: an absent figure and a measured zero are not alike. */
const DASH = "—";

function count(value: number | null): string {
  return value == null ? DASH : formatCount(value);
}

/**
 * The queue manager's queues and topics.
 *
 * One page for both, because from an application's side they are one thing:
 * something is opened by name and a message goes into it. Everything else
 * about them differs, which is why the depth column is empty on every topic
 * row and the subscribers column is empty on every queue row - a topic stores
 * nothing, and nothing subscribes to a queue.
 *
 * There is no rate column, and that is the queue manager rather than this
 * board. MQ reports what a queue holds, when it was last put to and last got
 * from, and how old its oldest message is; how fast it is moving is a
 * statistics message published to a system queue on a timer, which is a
 * different thing from a figure a listing can read.
 *
 * The alias and remote rows are here on purpose. They hold nothing, so every
 * figure on them is a dash - and leaving them out would hide exactly the
 * indirection somebody tracing a message needs to see.
 */
export function QueuesIbmMq() {
  const { t } = useTranslation();
  const state = useIbmMqDestinations();
  const { id: connID } = useConnectionScope();
  const confirm = useConfirm();
  const [search, setSearch] = useState("");
  const [selected, setSelected] = useState<string | null>(null);
  const [formOpen, setFormOpen] = useState(false);

  const destinations = useMemo(() => (state.data ?? []).map(readDestination), [state.data]);

  const shown = useMemo(() => {
    const needle = search.trim().toLowerCase();
    return destinations.filter(
      (entry) =>
        needle === "" ||
        entry.name.toLowerCase().includes(needle) ||
        (entry.topicString ?? "").toLowerCase().includes(needle),
    );
  }, [destinations, search]);

  const detail = useMemo(
    () => shown.find((entry) => entry.name === selected) ?? shown[0] ?? null,
    [shown, selected],
  );

  const create = useCallback(
    async (input: ibmmqApi.IBMMQDestinationInput) => {
      await ibmmqApi.createDestination(connID, input);
      toast.success(t("board.ibmmq.queues.created", { name: input.name }));
      await state.refresh();
    },
    [connID, state, t],
  );

  /*
   * The confirmation asks twice on a queue that holds messages, and the second
   * question is a different one. The queue manager refuses to delete a queue
   * with a depth, so deleting one means saying the messages go too - and they
   * are gone, not moved to the dead-letter queue.
   */
  const remove = useCallback(
    async (entry: IbmMqDestination) => {
      const holding = entry.kind === "queue" && (entry.depth ?? 0) > 0;
      const ok = await confirm({
        title: t("board.ibmmq.queues.deleteTitle", { name: entry.name }),
        description: holding
          ? t("board.ibmmq.queues.deleteHolding", { count: entry.depth ?? 0 })
          : t("board.ibmmq.queues.deleteDesc"),
        confirmLabel: t("common.delete"),
        danger: true,
      });
      if (!ok) return;
      try {
        await ibmmqApi.removeDestination(connID, entry.name, holding);
        toast.success(t("board.ibmmq.queues.deleted", { name: entry.name }));
        setSelected(null);
        await state.refresh();
      } catch (deleteError) {
        toast.error(t("board.ibmmq.queues.deleteFailed"), {
          description: formatErrorMessage(deleteError),
        });
      }
    },
    [confirm, connID, state, t],
  );

  return (
    <Page>
      <PageHeader
        title={t("shell.nav.ibmmq.topics")}
        subtitle={t("board.ibmmq.queues.count", { count: destinations.length })}
        actions={
          <div style={{ display: "flex", gap: "8px", alignItems: "center" }}>
            <Input
              style={{ width: "200px" }}
              value={search}
              placeholder={t("board.ibmmq.queues.search")}
              onChange={(event) => setSearch(event.target.value)}
            />
            <Button size="sm" onClick={() => setFormOpen(true)}>
              {t("board.ibmmq.queues.create")}
            </Button>
            <RefreshButton
              refreshing={state.refreshing}
              online={state.online}
              onClick={() => void state.refresh()}
            />
          </div>
        }
      />
      <QueueDialogIbmMq open={formOpen} onOpenChange={setFormOpen} onSubmit={create} />
      <BoardState state={state}>
        <PageBody>
          <div style={{ display: "flex", gap: "12px", minHeight: 0, flex: 1 }}>
            <Panel style={{ flex: 1, overflow: "auto" }}>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t("board.ibmmq.queues.name")}</TableHead>
                    <TableHead>{t("board.ibmmq.queues.type")}</TableHead>
                    <TableHead className="num">{t("board.ibmmq.queues.depth")}</TableHead>
                    <TableHead className="num">{t("board.ibmmq.queues.openInput")}</TableHead>
                    <TableHead className="num">{t("board.ibmmq.queues.subscribers")}</TableHead>
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
                        {entry.deadLetterQueue && (
                          <span className="pb pIMQ" style={{ marginLeft: "6px" }}>
                            {t("board.ibmmq.queues.deadLetterBadge")}
                          </span>
                        )}
                      </TableCell>
                      <TableCell>
                        <span
                          className="mono3"
                          style={{
                            ...MONO11,
                            color: inhibited(entry) ? "var(--c-warn)" : undefined,
                          }}
                        >
                          {entry.kind === "topic"
                            ? t("board.ibmmq.queues.kindTopic")
                            : (entry.queueType ?? DASH)}
                        </span>
                      </TableCell>
                      <TableCell className="num">
                        <span className="mono3" style={MONO11}>
                          {count(entry.depth)}
                        </span>
                      </TableCell>
                      <TableCell className="num">
                        <span className="mono3" style={MONO11}>
                          {count(entry.openInput)}
                        </span>
                      </TableCell>
                      <TableCell className="num">
                        <span className="mono3" style={MONO11}>
                          {count(entry.subscribers)}
                        </span>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </Panel>

            {detail != null && (
              <Panel style={{ width: "320px", overflow: "auto" }}>
                <PanelHeader
                  title={detail.name}
                  action={
                    <Button size="sm" variant="outline" onClick={() => void remove(detail)}>
                      {t("common.delete")}
                    </Button>
                  }
                />
                <div style={{ padding: "10px 12px", display: "grid", gap: "8px" }}>
                  <SectionLabel>{t("board.ibmmq.queues.section.definition")}</SectionLabel>
                  <KV
                    rows={[
                      [
                        t("board.ibmmq.queues.type"),
                        detail.kind === "topic"
                          ? t("board.ibmmq.queues.kindTopic")
                          : (detail.queueType ?? DASH),
                      ],
                      ...(detail.kind === "topic"
                        ? ([
                            [t("board.ibmmq.queues.topicString"), detail.topicString ?? DASH],
                            [t("board.ibmmq.queues.topicType"), detail.topicType ?? DASH],
                          ] as const)
                        : ([
                            [t("board.ibmmq.queues.maxDepth"), count(detail.maxDepth)],
                            [
                              t("board.ibmmq.queues.maxMessageLength"),
                              count(detail.maxMessageLength),
                            ],
                            [t("board.ibmmq.queues.cluster"), detail.cluster ?? DASH],
                          ] as const)),
                      [t("board.ibmmq.queues.description"), detail.description ?? DASH],
                      [t("board.ibmmq.queues.altered"), detail.altered ?? DASH],
                    ]}
                  />

                  {detail.kind === "queue" && (
                    <>
                      <SectionLabel>{t("board.ibmmq.queues.section.runtime")}</SectionLabel>
                      <KV
                        rows={[
                          [t("board.ibmmq.queues.depth"), count(detail.depth)],
                          [t("board.ibmmq.queues.openInput"), count(detail.openInput)],
                          [t("board.ibmmq.queues.openOutput"), count(detail.openOutput)],
                          [t("board.ibmmq.queues.uncommitted"), count(detail.uncommitted)],
                          [
                            t("board.ibmmq.queues.oldestMessage"),
                            detail.oldestMessageAgeSec == null
                              ? DASH
                              : t("board.ibmmq.queues.seconds", {
                                  count: detail.oldestMessageAgeSec,
                                }),
                          ],
                          [t("board.ibmmq.queues.lastPut"), detail.lastPut ?? DASH],
                          [t("board.ibmmq.queues.lastGet"), detail.lastGet ?? DASH],
                        ]}
                      />

                      <SectionLabel>{t("board.ibmmq.queues.section.handling")}</SectionLabel>
                      <KV
                        rows={[
                          [t("board.ibmmq.queues.backoutQueue"), detail.backoutQueue ?? DASH],
                          [
                            t("board.ibmmq.queues.backoutThreshold"),
                            count(detail.backoutThreshold),
                          ],
                        ]}
                      />
                    </>
                  )}

                  {inhibited(detail) && (
                    <div style={{ fontSize: "11px", color: "var(--c-warn)" }}>
                      {t("board.ibmmq.queues.inhibited", {
                        what: detail.inhibitPut
                          ? t("board.ibmmq.queues.inhibitPut")
                          : t("board.ibmmq.queues.inhibitGet"),
                      })}
                    </div>
                  )}
                </div>
              </Panel>
            )}
          </div>
          <div style={{ fontSize: "11px", color: "var(--c-muted)", paddingTop: "8px" }}>
            {t("board.ibmmq.queues.note")}
          </div>
        </PageBody>
      </BoardState>
    </Page>
  );
}
