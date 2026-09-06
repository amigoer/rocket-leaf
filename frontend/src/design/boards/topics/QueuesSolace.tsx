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
import { useSolaceDestinations } from "@/hooks/solace/useSolaceDestinations";
import {
  destination as readDestination,
  halted,
  hasDeadMsgQueue,
  type SolaceDestination,
} from "@/mq/solace/destinations";
import { formatBytes, formatCount } from "@/lib/format";
import { formatErrorMessage } from "@/lib/utils";
import { useConnectionScope } from "@/mq/ConnectionScope";
import * as solaceApi from "@/api/solace";
import { QueueDialogSolace } from "./QueueDialogSolace";

const MONO11 = { fontSize: "11px" } as const;

/** A dash, not a zero: an absent figure and a measured zero are not alike. */
const DASH = "—";

function count(value: number | null): string {
  return value == null ? DASH : formatCount(value);
}

/**
 * The Message VPN's queues.
 *
 * Queues only, and that is a decision rather than an omission. A Message VPN
 * has a second kind of endpoint - a topic endpoint - whose name is its
 * routing: it takes what is published to the topic it is named for and there
 * is nothing else to say about it. Those are on the routing page beside the
 * topic subscriptions, which is where a reader asking "where does this
 * publication land" is looking.
 *
 * The spooled column is the one figure on this page that no field on the queue
 * carries. SEMP's spooledMsgCount reads exactly like a depth and is a lifetime
 * counter: it survives a queue being drained and it is reset by clearStats, so
 * a board built on it shows a full queue as empty and an empty one as full.
 * The driver reads the message collection's own count instead, and the
 * lifetime figure is on the detail panel under its own name.
 */
export function QueuesSolace() {
  const { t } = useTranslation();
  const state = useSolaceDestinations();
  const { id: connID } = useConnectionScope();
  const confirm = useConfirm();
  const [search, setSearch] = useState("");
  const [selected, setSelected] = useState<string | null>(null);
  const [formOpen, setFormOpen] = useState(false);

  const destinations = useMemo(() => (state.data ?? []).map(readDestination), [state.data]);

  const shown = useMemo(() => {
    const needle = search.trim().toLowerCase();
    return destinations.filter(
      (entry) => needle === "" || entry.name.toLowerCase().includes(needle),
    );
  }, [destinations, search]);

  const detail = useMemo(
    () => shown.find((entry) => entry.name === selected) ?? shown[0] ?? null,
    [shown, selected],
  );

  const create = useCallback(
    async (input: solaceApi.SolaceQueueInput) => {
      await solaceApi.createQueue(connID, input);
      toast.success(t("board.solace.queues.created", { name: input.name }));
      await state.refresh();
    },
    [connID, state, t],
  );

  /*
   * The confirmation says the messages go too, because they do and because
   * nothing stops it.
   *
   * IBM MQ refuses to delete a queue with a depth unless the caller asks for a
   * purge, so its dialog can offer the check as a second question. SEMP has no
   * such precondition: it deletes a queue holding a quarter of a million
   * messages without a word, and they are discarded rather than dead-lettered.
   * This wording is the only guard there is.
   */
  const remove = useCallback(
    async (entry: SolaceDestination) => {
      const holding = entry.depth ?? 0;
      const ok = await confirm({
        title: t("board.solace.queues.deleteTitle", { name: entry.name }),
        description:
          holding > 0
            ? t("board.solace.queues.deleteHolding", { count: holding })
            : t("board.solace.queues.deleteDesc"),
        confirmLabel: t("common.delete"),
        danger: true,
      });
      if (!ok) return;
      try {
        await solaceApi.removeQueue(connID, entry.name);
        toast.success(t("board.solace.queues.deleted", { name: entry.name }));
        setSelected(null);
        await state.refresh();
      } catch (deleteError) {
        toast.error(t("board.solace.queues.deleteFailed"), {
          description: formatErrorMessage(deleteError),
        });
      }
    },
    [confirm, connID, state, t],
  );

  return (
    <Page>
      <PageHeader
        title={t("shell.nav.solace.topics")}
        subtitle={t("board.solace.queues.count", { count: destinations.length })}
        actions={
          <div style={{ display: "flex", gap: "8px", alignItems: "center" }}>
            <Input
              style={{ width: "200px" }}
              value={search}
              placeholder={t("board.solace.queues.search")}
              onChange={(event) => setSearch(event.target.value)}
            />
            <Button size="sm" onClick={() => setFormOpen(true)}>
              {t("board.solace.queues.create")}
            </Button>
            <RefreshButton
              refreshing={state.refreshing}
              online={state.online}
              onClick={() => void state.refresh()}
            />
          </div>
        }
      />
      <QueueDialogSolace open={formOpen} onOpenChange={setFormOpen} onSubmit={create} />
      <BoardState state={state}>
        <PageBody>
          <div style={{ display: "flex", gap: "12px", minHeight: 0, flex: 1 }}>
            <Panel style={{ flex: 1, overflow: "auto" }}>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t("board.solace.queues.name")}</TableHead>
                    <TableHead>{t("board.solace.queues.accessType")}</TableHead>
                    <TableHead className="num">{t("board.solace.queues.spooled")}</TableHead>
                    <TableHead className="num">{t("board.solace.queues.bound")}</TableHead>
                    <TableHead className="num">{t("board.solace.queues.rateIn")}</TableHead>
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
                        {hasDeadMsgQueue(entry) && (
                          <span className="pb pSOL" style={{ marginLeft: "6px" }}>
                            {t("board.solace.queues.dmqBadge")}
                          </span>
                        )}
                      </TableCell>
                      <TableCell>
                        <span
                          className="mono3"
                          style={{
                            ...MONO11,
                            color: halted(entry) ? "var(--c-warn)" : undefined,
                          }}
                        >
                          {entry.accessType ?? DASH}
                        </span>
                      </TableCell>
                      <TableCell className="num">
                        <span className="mono3" style={MONO11}>
                          {count(entry.depth)}
                        </span>
                      </TableCell>
                      <TableCell className="num">
                        <span className="mono3" style={MONO11}>
                          {count(entry.boundConsumers)}
                        </span>
                      </TableCell>
                      <TableCell className="num">
                        <span className="mono3" style={MONO11}>
                          {count(entry.rateIn)}
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
                  <SectionLabel>{t("board.solace.queues.section.definition")}</SectionLabel>
                  <KV
                    rows={[
                      [t("board.solace.queues.accessType"), detail.accessType ?? DASH],
                      [t("board.solace.queues.permission"), detail.permission ?? DASH],
                      [t("board.solace.queues.owner"), detail.owner ?? DASH],
                      [
                        t("board.solace.queues.maxSpool"),
                        detail.maxSpoolUsageMb == null
                          ? DASH
                          : formatBytes(detail.maxSpoolUsageMb * 1024 * 1024),
                      ],
                      [
                        t("board.solace.queues.maxMsgSize"),
                        detail.maxMsgSizeBytes == null ? DASH : formatBytes(detail.maxMsgSizeBytes),
                      ],
                      [t("board.solace.queues.partitions"), count(detail.partitionCount)],
                    ]}
                  />

                  <SectionLabel>{t("board.solace.queues.section.runtime")}</SectionLabel>
                  <KV
                    rows={[
                      [t("board.solace.queues.spooled"), count(detail.depth)],
                      [
                        t("board.solace.queues.spoolUsage"),
                        detail.spoolUsageBytes == null ? DASH : formatBytes(detail.spoolUsageBytes),
                      ],
                      [t("board.solace.queues.bound"), count(detail.boundConsumers)],
                      [t("board.solace.queues.unacked"), count(detail.unackedMsgCount)],
                      [t("board.solace.queues.redelivered"), count(detail.redeliveredMsgCount)],
                      [t("board.solace.queues.spooledTotal"), count(detail.spooledTotal)],
                    ]}
                  />

                  <SectionLabel>{t("board.solace.queues.section.handling")}</SectionLabel>
                  <KV
                    rows={[
                      [t("board.solace.queues.deadMsgQueue"), detail.deadMsgQueue ?? DASH],
                      [t("board.solace.queues.maxRedelivery"), count(detail.maxRedeliveryCount)],
                      [
                        t("board.solace.queues.maxTtl"),
                        detail.maxTtlSec == null || detail.maxTtlSec === 0
                          ? DASH
                          : t("board.solace.queues.seconds", { count: detail.maxTtlSec }),
                      ],
                      [t("board.solace.queues.toDmqTtl"), count(detail.ttlExpiredToDmq)],
                      [t("board.solace.queues.toDmqRetry"), count(detail.redeliveryExceededToDmq)],
                    ]}
                  />

                  {halted(detail) && (
                    <div style={{ fontSize: "11px", color: "var(--c-warn)" }}>
                      {t("board.solace.queues.halted", {
                        what: !detail.ingressEnabled
                          ? t("board.solace.queues.ingress")
                          : t("board.solace.queues.egress"),
                      })}
                    </div>
                  )}
                </div>
              </Panel>
            )}
          </div>
          <div style={{ fontSize: "11px", color: "var(--c-muted)", paddingTop: "8px" }}>
            {t("board.solace.queues.note")}
          </div>
        </PageBody>
      </BoardState>
    </Page>
  );
}
