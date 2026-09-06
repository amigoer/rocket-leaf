import { useCallback, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { ListArea, ListPane, Page, PageHeader, RefreshButton, Toolbar } from "@/design/shell";
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
import {
  DetailPanel,
  DetailPanelBody,
  DetailPanelFooter,
  DetailPanelHeader,
  KV,
  Panel,
  SectionLabel,
  Status,
  toast,
  useConfirm,
} from "@/components";
import { BoardState } from "@/design/boards/BoardState";
import { ConsumerDialogNats } from "./ConsumerDialogNats";
import { useNatsConsumers } from "@/hooks/nats/useNatsConsumers";
import { useNatsStreams } from "@/hooks/nats/useNatsStreams";
import { useConnectionScope } from "@/mq/ConnectionScope";
import * as natsApi from "@/api/nats";
import { formatErrorMessage } from "@/lib/utils";
import { formatCount } from "@/lib/format";
import { streamName } from "@/mq/nats/destinations";
import type { NATSConsumerInput } from "@bindings/bridge/models";
import type { Subscription } from "@bindings/model/models";
import {
  ackFloorSequence,
  ackPending,
  ackPolicy,
  ackWait,
  backlog,
  clusterName,
  consumerName,
  createdAt,
  deliverGroup,
  deliverPolicy,
  deliverSubject,
  deliveredSequence,
  filterSubjects,
  isDurable,
  isPush,
  leader,
  maxAckPending,
  maxDeliver,
  members,
  redelivered,
  replayPolicy,
  streamOf,
  waitingRequests,
} from "@/mq/nats/subscriptions";

const NAME = { fontSize: "11.5px" } as const;
const MONO11 = { fontSize: "11px" } as const;
const RIGHT = { textAlign: "right" } as const;

/** A figure the server did not report reads as a dash, never as a zero. */
function Metric({ value }: { value: number | null }) {
  if (value == null) return <span style={{ color: "var(--c-muted-2)" }}>—</span>;
  return <>{formatCount(value)}</>;
}

/**
 * NATS consumers.
 *
 * The members column is the one that makes this board NATS's rather than
 * another family's with different words. Only a push consumer can be asked how
 * many clients are attached, and only yes or no; a pull consumer has nobody to
 * count, because clients ask for messages when they want them and hold nothing
 * open in between. Reporting zero there would call a perfectly healthy
 * consumer unattended, so it shows a dash and the number of parked pull
 * requests goes in its own column, labelled for what it is.
 *
 * There is no reset-position control, and its absence is the same kind of
 * honesty. A consumer's start policy is fixed when it is created and the
 * server refuses to change it; the only way to move one is to delete it and
 * make another, which changes its identity and drops what it had acknowledged.
 * A button that did that under the name "reset" would be lying about what it
 * does.
 */
export function ConsumersNats() {
  const { t } = useTranslation();
  const state = useNatsConsumers();
  const streams = useNatsStreams();
  const { id: connID } = useConnectionScope();
  const confirm = useConfirm();
  const [search, setSearch] = useState("");
  const [selected, setSelected] = useState<string | null>(null);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [editing, setEditing] = useState<Subscription | null>(null);

  const consumers = useMemo(() => state.data ?? [], [state.data]);
  const streamNames = useMemo(
    () => (streams.data ?? []).map((stream) => streamName(stream)),
    [streams.data],
  );

  const key = (subscription: Subscription) =>
    `${streamOf(subscription)}/${consumerName(subscription)}`;

  const rows = useMemo(() => {
    const needle = search.trim().toLowerCase();
    if (needle === "") return consumers;
    return consumers.filter(
      (consumer) =>
        consumerName(consumer).toLowerCase().includes(needle) ||
        streamOf(consumer).toLowerCase().includes(needle),
    );
  }, [consumers, search]);

  const panel = useMemo(
    () => consumers.find((consumer) => key(consumer) === selected) ?? null,
    [consumers, selected],
  );

  const save = useCallback(
    async (input: NATSConsumerInput, update: boolean) => {
      if (update) {
        await natsApi.updateConsumer(connID, input);
        toast.success(t("board.consumers.nats.updated", { name: input.name }));
      } else {
        await natsApi.createConsumer(connID, input);
        toast.success(t("board.consumers.nats.created", { name: input.name }));
      }
      await state.refresh();
    },
    [connID, state, t],
  );

  const remove = useCallback(
    async (consumer: Subscription) => {
      const ok = await confirm({
        title: t("board.consumers.nats.deleteTitle", { name: consumerName(consumer) }),
        /* The position is the warning. Deleting a consumer discards what it
           had acknowledged, so whatever replaces it starts from its own policy
           rather than from where this one had got to. */
        description: t("board.consumers.nats.deleteDescription", {
          stream: streamOf(consumer),
          count: backlog(consumer),
        }),
        confirmLabel: t("board.common.delete"),
        danger: true,
      });
      if (!ok) return;
      try {
        await natsApi.deleteConsumer(connID, streamOf(consumer), consumerName(consumer));
        toast.success(t("board.consumers.nats.deleted", { name: consumerName(consumer) }));
        setSelected(null);
        await state.refresh();
      } catch (deleteError) {
        toast.error(t("board.consumers.nats.deleteFailed"), {
          description: formatErrorMessage(deleteError),
        });
      }
    },
    [confirm, connID, state, t],
  );

  return (
    <Page>
      <ConsumerDialogNats
        open={dialogOpen}
        onOpenChange={(open) => {
          setDialogOpen(open);
          if (!open) setEditing(null);
        }}
        stream={editing != null ? streamOf(editing) : (streamNames[0] ?? "")}
        editing={editing}
        onSubmit={save}
      />
      <PageHeader
        title={t("board.consumers.nats.title")}
        subtitle={t("board.consumers.nats.subtitle")}
        actions={
          <>
            <Button
              // Nothing to declare a consumer on until a stream exists, and a
              // dialog that opened onto an empty stream picker would be a
              // dead end rather than a form.
              disabled={streamNames.length === 0}
              onClick={() => {
                setEditing(null);
                setDialogOpen(true);
              }}
            >
              {t("board.consumers.nats.newConsumerAction")}
            </Button>
            <RefreshButton
              refreshing={state.refreshing}
              online={state.online}
              onClick={() => void state.refresh()}
            />
          </>
        }
      />
      <Toolbar>
        <Input
          className="w-[240px] flex-none"
          placeholder={t("board.consumers.nats.search")}
          value={search}
          onChange={(event) => setSearch(event.target.value)}
        />
        <span className="flex-1" />
        <span style={{ fontSize: "11px", color: "var(--c-muted)" }}>
          {t("board.consumers.nats.found", { count: rows.length })}
        </span>
      </Toolbar>

      <BoardState
        state={state}
        empty={
          rows.length === 0 ? (
            <ListArea>
              <ListPane>
                <div
                  style={{
                    padding: "24px",
                    fontSize: "11.5px",
                    color: "var(--c-muted)",
                    textAlign: "center",
                  }}
                >
                  {consumers.length === 0
                    ? t("board.consumers.nats.noConsumers")
                    : t("board.consumers.nats.noMatches")}
                </div>
              </ListPane>
            </ListArea>
          ) : undefined
        }
      >
        <ListArea>
          <ListPane>
            <Table inset>
              <TableHeader>
                <TableRow>
                  <TableHead>{t("board.consumers.nats.consumer")}</TableHead>
                  <TableHead>{t("board.consumers.nats.stream")}</TableHead>
                  <TableHead>{t("board.consumers.nats.kind")}</TableHead>
                  <TableHead style={RIGHT}>{t("board.consumers.nats.backlog")}</TableHead>
                  <TableHead style={RIGHT}>{t("board.consumers.nats.unacked")}</TableHead>
                  <TableHead style={RIGHT}>{t("board.consumers.nats.redelivered")}</TableHead>
                  <TableHead style={RIGHT}>{t("board.consumers.nats.attached")}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {rows.map((consumer) => (
                  <TableRow
                    key={key(consumer)}
                    selected={selected === key(consumer)}
                    onClick={() => setSelected(key(consumer))}
                  >
                    <TableCell>
                      <b className="mono3" style={{ fontWeight: 500, ...NAME }}>
                        {consumerName(consumer)}
                      </b>
                      {!isDurable(consumer) && (
                        <Status tone="off" style={{ fontSize: "10px", marginLeft: "6px" }}>
                          {t("board.consumers.nats.ephemeral")}
                        </Status>
                      )}
                    </TableCell>
                    <TableCell className="mono3" style={MONO11}>
                      {streamOf(consumer)}
                    </TableCell>
                    <TableCell style={MONO11}>
                      <Status
                        tone={consumer.status === "warning" ? "warn" : "off"}
                        style={{ fontSize: "10px" }}
                      >
                        {isPush(consumer)
                          ? t("board.consumers.nats.push")
                          : t("board.consumers.nats.pull")}
                      </Status>
                    </TableCell>
                    <TableCell className="mono3" style={RIGHT}>
                      {formatCount(backlog(consumer))}
                    </TableCell>
                    <TableCell className="mono3" style={RIGHT}>
                      <Metric value={ackPending(consumer)} />
                    </TableCell>
                    <TableCell className="mono3" style={RIGHT}>
                      <Metric value={redelivered(consumer)} />
                    </TableCell>
                    <TableCell className="mono3" style={RIGHT}>
                      {/* Null for a pull consumer, and it has to read as "no
                          answer" rather than as none attached. */}
                      <Metric value={members(consumer)} />
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </ListPane>

          {panel != null && (
            <DetailPanel width={400} onDismiss={() => setSelected(null)}>
              <DetailPanelHeader
                title={consumerName(panel)}
                badge={
                  <Status tone="off" style={{ fontSize: "10px" }}>
                    {isPush(panel)
                      ? t("board.consumers.nats.push")
                      : t("board.consumers.nats.pull")}
                  </Status>
                }
                onClose={() => setSelected(null)}
              />
              <DetailPanelBody>
                <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: "8px" }}>
                  <Panel style={{ padding: "9px 12px" }}>
                    <div style={{ fontSize: "10.5px", color: "var(--c-muted)" }}>
                      {t("board.consumers.nats.backlog")}
                    </div>
                    <div
                      className="mono3"
                      style={{ fontSize: "15px", fontWeight: 600, marginTop: "2px" }}
                    >
                      {formatCount(backlog(panel))}
                    </div>
                  </Panel>
                  <Panel style={{ padding: "9px 12px" }}>
                    <div style={{ fontSize: "10.5px", color: "var(--c-muted)" }}>
                      {t("board.consumers.nats.unacked")}
                    </div>
                    <div
                      className="mono3"
                      style={{ fontSize: "15px", fontWeight: 600, marginTop: "2px" }}
                    >
                      <Metric value={ackPending(panel)} />
                    </div>
                  </Panel>
                </div>

                <KV
                  rows={[
                    [t("board.consumers.nats.stream"), mono(streamOf(panel))],
                    [
                      t("board.consumers.nats.position"),
                      <span className="mono3" style={MONO11}>
                        {/* Two numbers because they answer different
                            questions: the furthest handed out, and the point
                            below which everything is settled. */}
                        {deliveredSequence(panel) == null || ackFloorSequence(panel) == null ? (
                          "—"
                        ) : (
                          <>
                            {t("board.consumers.nats.ackFloor")}{" "}
                            {formatCount(ackFloorSequence(panel) ?? 0)} ·{" "}
                            {t("board.consumers.nats.delivered")}{" "}
                            {formatCount(deliveredSequence(panel) ?? 0)}
                          </>
                        )}
                      </span>,
                    ],
                    [t("board.consumers.nats.startAt"), mono(deliverPolicy(panel))],
                    [t("board.consumers.nats.ackPolicy"), mono(ackPolicy(panel))],
                    [t("board.consumers.nats.ackWait"), mono(ackWait(panel))],
                    [t("board.consumers.nats.replay"), mono(replayPolicy(panel))],
                    [
                      t("board.consumers.nats.maxDeliver"),
                      <span className="mono3" style={MONO11}>
                        {maxDeliver(panel) ?? (
                          <span style={{ color: "var(--c-muted-2)" }}>
                            {t("board.consumers.nats.unlimited")}
                          </span>
                        )}
                      </span>,
                    ],
                    [
                      t("board.consumers.nats.maxAckPending"),
                      <span className="mono3" style={MONO11}>
                        <Metric value={maxAckPending(panel)} />
                      </span>,
                    ],
                    [t("board.consumers.nats.createdAt"), mono(createdAt(panel))],
                  ]}
                />

                <div>
                  <SectionLabel style={{ marginBottom: "6px" }}>
                    {t("board.consumers.nats.filter")}
                  </SectionLabel>
                  {filterSubjects(panel).length === 0 ? (
                    <div style={{ fontSize: "11px", color: "var(--c-muted)" }}>
                      {t("board.consumers.nats.wholeStream")}
                    </div>
                  ) : (
                    <div style={{ display: "flex", flexWrap: "wrap", gap: "4px" }}>
                      {filterSubjects(panel).map((subject) => (
                        <Status key={subject} tone="off" style={{ fontSize: "10px" }}>
                          {subject}
                        </Status>
                      ))}
                    </div>
                  )}
                </div>

                <KV
                  rows={
                    isPush(panel)
                      ? [
                          [t("board.consumers.nats.deliverSubject"), mono(deliverSubject(panel))],
                          [t("board.consumers.nats.deliverGroup"), mono(deliverGroup(panel))],
                          [
                            t("board.consumers.nats.attached"),
                            <span className="mono3" style={MONO11}>
                              {members(panel) === 1
                                ? t("board.consumers.nats.bound")
                                : t("board.consumers.nats.notBound")}
                            </span>,
                          ],
                        ]
                      : [
                          [
                            t("board.consumers.nats.waiting"),
                            <span className="mono3" style={MONO11}>
                              <Metric value={waitingRequests(panel)} />
                            </span>,
                          ],
                          [
                            t("board.consumers.nats.attached"),
                            <span style={{ fontSize: "11px", color: "var(--c-muted)" }}>
                              {/* Not "0 attached": there is nobody to count,
                                  which is how pull consumers work. */}
                              {t("board.consumers.nats.pullHasNoMembers")}
                            </span>,
                          ],
                        ]
                  }
                />

                {clusterName(panel) != null && (
                  <div style={{ fontSize: "11px", color: "var(--c-muted)" }}>
                    {t("board.consumers.nats.stateOn", { server: leader(panel) ?? "?" })}
                  </div>
                )}
              </DetailPanelBody>
              <DetailPanelFooter>
                <Button
                  variant="outline"
                  onClick={() => {
                    setEditing(panel);
                    setDialogOpen(true);
                  }}
                >
                  {t("board.common.edit")}
                </Button>
                <Button variant="destructive" onClick={() => void remove(panel)}>
                  {t("board.common.delete")}
                </Button>
              </DetailPanelFooter>
            </DetailPanel>
          )}
        </ListArea>
      </BoardState>
    </Page>
  );
}

function mono(value: string | null) {
  return (
    <span className="mono3" style={MONO11}>
      {value ?? "—"}
    </span>
  );
}
