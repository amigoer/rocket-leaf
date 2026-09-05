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
import { useActiveMQSubscriptions } from "@/hooks/activemq/useActiveMQSubscriptions";
import { useActiveMQDestinations } from "@/hooks/activemq/useActiveMQDestinations";
import {
  subscription as readSubscription,
  type ActiveMQSubscription,
} from "@/mq/activemq/subscriptions";
import { destination as readDestination } from "@/mq/activemq/destinations";
import { formatCount } from "@/lib/format";
import { formatErrorMessage } from "@/lib/utils";
import { useConnectionScope } from "@/mq/ConnectionScope";
import * as activemqApi from "@/api/activemq";
import { SubscriptionDialogActiveMQ } from "./SubscriptionDialogActiveMQ";

const MONO11 = { fontSize: "11px" } as const;
const DASH = "—";

function count(value: number | null): string {
  return value == null ? DASH : formatCount(value);
}

/**
 * ActiveMQ durable subscriptions.
 *
 * The page shows both attached and detached ones, and the difference is not a
 * health signal. A durable subscription with nothing connected is the broker
 * holding messages for a client that has gone away - which is the entire
 * reason durability exists - so an idle row reads as idle rather than as a
 * fault. What is a fault is Classic's slow-consumer flag, which means the
 * subscriber is falling behind what is being dispatched to it.
 *
 * Only topics have these. A JMS queue's consumers are connections: they have
 * no name, they keep no position, and they are gone when the socket closes.
 * They belong on the connections page and are not listed here.
 */
export function SubscriptionsActiveMQ() {
  const { t } = useTranslation();
  const state = useActiveMQSubscriptions();
  const destinationState = useActiveMQDestinations();
  const { id: connID } = useConnectionScope();
  const confirm = useConfirm();
  const [search, setSearch] = useState("");
  const [selected, setSelected] = useState<string | null>(null);
  const [creating, setCreating] = useState(false);

  const subscriptions = useMemo(
    () => (state.data ?? []).map(readSubscription),
    [state.data],
  );

  // The dialog needs somewhere to put a subscription, and only a topic can
  // hold one - offering a queue would produce an error the broker explains
  // and the form could have prevented.
  const topics = useMemo(
    () =>
      (destinationState.data ?? [])
        .map(readDestination)
        .filter((entry) => entry.kind === "topic")
        .map((entry) => entry.name),
    [destinationState.data],
  );

  const shown = useMemo(() => {
    const needle = search.trim().toLowerCase();
    if (needle === "") return subscriptions;
    return subscriptions.filter(
      (entry) =>
        entry.name.toLowerCase().includes(needle) ||
        entry.topic.toLowerCase().includes(needle),
    );
  }, [subscriptions, search]);

  const detail = useMemo(
    () => shown.find((entry) => entry.name === selected) ?? shown[0] ?? null,
    [shown, selected],
  );

  const create = useCallback(
    async (topic: string, name: string, selector: string) => {
      await activemqApi.createSubscription(connID, topic, name, selector);
      toast.success(t("board.activemq.subscriptions.created", { name }));
      await state.refresh();
    },
    [connID, state, t],
  );

  const remove = useCallback(
    async (entry: ActiveMQSubscription) => {
      const ok = await confirm({
        title: t("board.activemq.subscriptions.deleteTitle", { name: entry.name }),
        description: t("board.activemq.subscriptions.deleteDesc", {
          count: entry.backlog ?? 0,
        }),
        confirmLabel: t("common.delete"),
        danger: true,
      });
      if (!ok) return;
      try {
        await activemqApi.removeSubscription(connID, entry.name);
        toast.success(t("board.activemq.subscriptions.deleted", { name: entry.name }));
        setSelected(null);
        await state.refresh();
      } catch (removeError) {
        toast.error(t("board.activemq.subscriptions.deleteFailed"), {
          description: formatErrorMessage(removeError),
        });
      }
    },
    [confirm, connID, state, t],
  );

  return (
    <Page>
      <PageHeader
        title={t("shell.nav.activemq.consumers")}
        subtitle={t("board.activemq.subscriptions.count", {
          count: subscriptions.length,
          attached: subscriptions.filter((entry) => entry.active).length,
        })}
        actions={
          <div style={{ display: "flex", gap: "8px", alignItems: "center" }}>
            <Input
              style={{ width: "200px" }}
              value={search}
              placeholder={t("board.activemq.subscriptions.search")}
              onChange={(event) => setSearch(event.target.value)}
            />
            <Button size="sm" onClick={() => setCreating(true)}>
              {t("board.activemq.subscriptions.create")}
            </Button>
            <RefreshButton
              refreshing={state.refreshing}
              online={state.online}
              onClick={() => void state.refresh()}
            />
          </div>
        }
      />
      <SubscriptionDialogActiveMQ
        open={creating}
        onOpenChange={setCreating}
        topics={topics}
        product={subscriptions[0]?.product ?? "artemis"}
        onCreate={create}
      />
      <BoardState state={state}>
        <PageBody>
          <div style={{ display: "flex", gap: "12px", minHeight: 0, flex: 1 }}>
            <Panel style={{ flex: 1, overflow: "auto" }}>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t("board.activemq.subscriptions.name")}</TableHead>
                    <TableHead>{t("board.activemq.subscriptions.topic")}</TableHead>
                    <TableHead>{t("board.activemq.subscriptions.state")}</TableHead>
                    <TableHead className="num">
                      {t("board.activemq.subscriptions.backlog")}
                    </TableHead>
                    <TableHead className="num">
                      {t("board.activemq.subscriptions.pendingAck")}
                    </TableHead>
                    <TableHead className="num">
                      {t("board.activemq.subscriptions.consumed")}
                    </TableHead>
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
                          {entry.subscriptionName ?? entry.name}
                        </span>
                      </TableCell>
                      <TableCell>
                        <span className="mono3" style={MONO11}>
                          {entry.topic}
                        </span>
                      </TableCell>
                      <TableCell>
                        {/*
                          Detached is the resting state, so it is worded as
                          waiting rather than coloured as broken. Slow is the
                          one that is a problem.
                        */}
                        {entry.slow === true
                          ? t("board.activemq.subscriptions.slow")
                          : entry.active
                            ? t("board.activemq.subscriptions.attached")
                            : t("board.activemq.subscriptions.waiting")}
                      </TableCell>
                      <TableCell className="num">
                        <span className="mono3" style={MONO11}>
                          {count(entry.backlog)}
                        </span>
                      </TableCell>
                      <TableCell className="num">
                        <span className="mono3" style={MONO11}>
                          {count(entry.pendingAck)}
                        </span>
                      </TableCell>
                      <TableCell className="num">
                        <span className="mono3" style={MONO11}>
                          {count(entry.consumed)}
                        </span>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </Panel>

            {detail != null && (
              <Panel style={{ width: "300px", flex: "none", overflow: "auto" }}>
                <PanelHeader title={detail.subscriptionName ?? detail.name} />
                <div style={{ padding: "0 12px 12px" }}>
                  <SectionLabel>{t("board.activemq.subscriptions.section.state")}</SectionLabel>
                  <KV
                    rows={[
                      [t("board.activemq.subscriptions.topic"), detail.topic],
                      [t("board.activemq.subscriptions.backlog"), count(detail.backlog)],
                      [t("board.activemq.subscriptions.pendingAck"), count(detail.pendingAck)],
                      [t("board.activemq.subscriptions.dispatched"), count(detail.dispatched)],
                      [t("board.activemq.subscriptions.consumed"), count(detail.consumed)],
                      [
                        t("board.activemq.subscriptions.members"),
                        formatCount(detail.members),
                      ],
                    ]}
                  />

                  <SectionLabel>
                    {t("board.activemq.subscriptions.section.identity")}
                  </SectionLabel>
                  <KV
                    rows={[
                      // Classic identifies a subscription by the client that
                      // registered it; Artemis by the queue's own name, and
                      // has no client id to show.
                      [t("board.activemq.subscriptions.clientId"), detail.clientId ?? DASH],
                      [t("board.activemq.subscriptions.selector"), detail.selector ?? DASH],
                      [
                        t("board.activemq.subscriptions.durable"),
                        detail.durable == null
                          ? DASH
                          : detail.durable
                            ? t("common.yes")
                            : t("common.no"),
                      ],
                      [
                        t("board.activemq.subscriptions.deadLetterAddress"),
                        detail.deadLetterAddress ?? DASH,
                      ],
                      [t("board.activemq.subscriptions.prefetch"), count(detail.prefetch)],
                    ]}
                  />

                  <div style={{ display: "flex", gap: "6px", marginTop: "12px" }}>
                    <Button size="sm" variant="destructive" onClick={() => void remove(detail)}>
                      {t("board.activemq.subscriptions.unsubscribe")}
                    </Button>
                  </div>
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
            {t("board.activemq.subscriptions.note")}
          </p>
        </PageBody>
      </BoardState>
    </Page>
  );
}
