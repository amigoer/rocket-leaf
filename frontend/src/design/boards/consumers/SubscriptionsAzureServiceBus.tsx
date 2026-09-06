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
import { useServiceBusSubscriptions } from "@/hooks/azureservicebus/useServiceBusSubscriptions";
import {
  receivesNothing,
  subscription as readSubscription,
  type ServiceBusSubscription,
} from "@/mq/azureservicebus/subscriptions";
import { count, duration } from "@/design/boards/topics/EntitiesAzureServiceBus";
import { formatErrorMessage } from "@/lib/utils";
import { useConnectionScope } from "@/mq/ConnectionScope";
import * as serviceBusApi from "@/api/azureservicebus";
import { SubscriptionDialogAzureServiceBus } from "./SubscriptionDialogAzureServiceBus";

const MONO11 = { fontSize: "11px" } as const;
const DASH = "—";

/**
 * Azure Service Bus subscriptions.
 *
 * One row per subscription across every topic in the namespace. This is where
 * a topic's messages actually are: a send is copied into each subscription
 * whose rules let it through, so the backlog on this board is the whole of
 * what a topic is holding on anyone's behalf.
 *
 * The rules column is not decoration. A subscription is created with a
 * $Default rule matching everything, and one whose rules have all been deleted
 * receives nothing while looking healthy on every other figure - it exists, it
 * is Active, and its backlog is zero because nothing can arrive.
 *
 * There is no consumer column, and there cannot be one: a subscription is read
 * by whoever opens a receiver on it and Service Bus keeps no register of who
 * that was. A zero would say "nothing is consuming this".
 */
export function SubscriptionsAzureServiceBus() {
  const { t } = useTranslation();
  const state = useServiceBusSubscriptions();
  const { id: connID } = useConnectionScope();
  const confirm = useConfirm();
  const [search, setSearch] = useState("");
  const [selected, setSelected] = useState<string | null>(null);
  const [editing, setEditing] = useState<ServiceBusSubscription | null>(null);
  const [formOpen, setFormOpen] = useState(false);

  const subscriptions = useMemo(
    () => (state.data ?? []).map(readSubscription),
    [state.data],
  );

  const shown = useMemo(() => {
    const needle = search.trim().toLowerCase();
    return subscriptions.filter(
      (row) =>
        needle === "" ||
        row.name.toLowerCase().includes(needle) ||
        row.topic.toLowerCase().includes(needle),
    );
  }, [subscriptions, search]);

  const key = (row: ServiceBusSubscription) => `${row.topic}/${row.name}`;
  const detail = useMemo(
    () => shown.find((row) => key(row) === selected) ?? shown[0] ?? null,
    [shown, selected],
  );

  const save = useCallback(
    async (input: serviceBusApi.AzureServiceBusSubscriptionInput) => {
      if (editing == null) {
        await serviceBusApi.createSubscription(connID, input);
        toast.success(t("board.azure-servicebus.subscriptions.created", { name: input.name }));
      } else {
        await serviceBusApi.updateSubscription(connID, input);
        toast.success(t("board.azure-servicebus.subscriptions.updated", { name: input.name }));
      }
      await state.refresh();
    },
    [connID, editing, state, t],
  );

  /*
   * The confirmation says what goes with it. A subscription's backlog is its
   * own - the copy that reached it was never the topic's again - so deleting
   * one discards those messages rather than returning them anywhere.
   */
  const remove = useCallback(
    async (row: ServiceBusSubscription) => {
      const ok = await confirm({
        title: t("board.azure-servicebus.subscriptions.deleteTitle", { name: row.name }),
        description: t("board.azure-servicebus.subscriptions.deleteDesc", { topic: row.topic }),
        confirmLabel: t("common.delete"),
        danger: true,
      });
      if (!ok) return;
      try {
        await serviceBusApi.removeSubscription(connID, row.topic, row.name);
        toast.success(t("board.azure-servicebus.subscriptions.deleted", { name: row.name }));
        setSelected(null);
        await state.refresh();
      } catch (deleteError) {
        toast.error(t("board.azure-servicebus.subscriptions.deleteFailed"), {
          description: formatErrorMessage(deleteError),
        });
      }
    },
    [confirm, connID, state, t],
  );

  return (
    <Page>
      <PageHeader
        title={t("shell.nav.azure-servicebus.consumers")}
        subtitle={t("board.azure-servicebus.subscriptions.count", {
          count: subscriptions.length,
        })}
        actions={
          <div style={{ display: "flex", gap: "8px", alignItems: "center" }}>
            <Input
              style={{ width: "200px" }}
              value={search}
              placeholder={t("board.azure-servicebus.subscriptions.search")}
              onChange={(event) => setSearch(event.target.value)}
            />
            <Button
              size="sm"
              onClick={() => {
                setEditing(null);
                setFormOpen(true);
              }}
            >
              {t("board.azure-servicebus.subscriptions.create")}
            </Button>
            <RefreshButton
              refreshing={state.refreshing}
              online={state.online}
              onClick={() => void state.refresh()}
            />
          </div>
        }
      />
      <SubscriptionDialogAzureServiceBus
        open={formOpen}
        editing={editing}
        onOpenChange={setFormOpen}
        onSubmit={save}
      />
      <BoardState state={state}>
        <PageBody>
          <div style={{ display: "flex", gap: "12px", minHeight: 0, flex: 1 }}>
            <Panel style={{ flex: 1, overflow: "auto" }}>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t("board.azure-servicebus.subscriptions.name")}</TableHead>
                    <TableHead>{t("board.azure-servicebus.subscriptions.topic")}</TableHead>
                    <TableHead className="num">
                      {t("board.azure-servicebus.subscriptions.backlog")}
                    </TableHead>
                    <TableHead className="num">
                      {t("board.azure-servicebus.subscriptions.deadLetters")}
                    </TableHead>
                    <TableHead>{t("board.azure-servicebus.subscriptions.rules")}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {shown.map((row) => (
                    <TableRow
                      key={key(row)}
                      data-state={detail != null && key(detail) === key(row) ? "selected" : undefined}
                      onClick={() => setSelected(key(row))}
                      style={{ cursor: "pointer" }}
                    >
                      <TableCell>
                        <span className="mono3" style={MONO11}>
                          {row.name}
                        </span>
                        {/* Said on the row: it is the one state that looks
                            healthy on every other figure, because nothing can
                            arrive to be counted. */}
                        {receivesNothing(row) && (
                          <span className="pb pASB" style={{ marginLeft: "6px" }}>
                            {t("board.azure-servicebus.subscriptions.noRuleBadge")}
                          </span>
                        )}
                      </TableCell>
                      <TableCell>
                        <span className="mono3" style={MONO11}>
                          {row.topic}
                        </span>
                      </TableCell>
                      <TableCell className="num">
                        <span className="mono3" style={MONO11}>
                          {count(row.backlog)}
                        </span>
                      </TableCell>
                      <TableCell className="num">
                        <span className="mono3" style={MONO11}>
                          {count(row.deadLetterCount)}
                        </span>
                      </TableCell>
                      <TableCell>
                        <span className="mono3" style={MONO11}>
                          {row.ruleNames.join(", ") || DASH}
                        </span>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </Panel>

            {detail != null && (
              <SubscriptionDetail
                entry={detail}
                onEdit={() => {
                  setEditing(detail);
                  setFormOpen(true);
                }}
                onRemove={() => void remove(detail)}
              />
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
            {t("board.azure-servicebus.subscriptions.note")}
          </p>
        </PageBody>
      </BoardState>
    </Page>
  );
}

function SubscriptionDetail({
  entry,
  onEdit,
  onRemove,
}: {
  entry: ServiceBusSubscription;
  onEdit: () => void;
  onRemove: () => void;
}) {
  const { t } = useTranslation();
  return (
    <Panel style={{ width: "300px", flex: "none", overflow: "auto" }}>
      <PanelHeader title={entry.name} />
      <div style={{ padding: "0 12px 12px" }}>
        <SectionLabel>{t("board.azure-servicebus.subscriptions.section.holding")}</SectionLabel>
        <KV
          rows={[
            [t("board.azure-servicebus.subscriptions.topic"), entry.topic],
            [t("board.azure-servicebus.subscriptions.backlog"), count(entry.backlog)],
            [
              t("board.azure-servicebus.subscriptions.deadLetters"),
              count(entry.deadLetterCount),
            ],
            [
              t("board.azure-servicebus.subscriptions.rules"),
              entry.ruleNames.join(", ") || DASH,
            ],
          ]}
        />

        <SectionLabel>{t("board.azure-servicebus.subscriptions.section.delivery")}</SectionLabel>
        <KV
          rows={[
            [t("board.azure-servicebus.subscriptions.status"), entry.status ?? DASH],
            [
              t("board.azure-servicebus.subscriptions.lockDuration"),
              duration(entry.lockDurationSec),
            ],
            [
              t("board.azure-servicebus.subscriptions.maxDelivery"),
              count(entry.maxDeliveryCount),
            ],
            [t("board.azure-servicebus.subscriptions.ttl"), duration(entry.ttlSec)],
            [
              t("board.azure-servicebus.subscriptions.autoDelete"),
              duration(entry.autoDeleteOnIdleSec),
            ],
            [t("board.azure-servicebus.subscriptions.forwardTo"), entry.forwardTo ?? DASH],
            [
              t("board.azure-servicebus.subscriptions.forwardDeadLettersTo"),
              entry.forwardDeadLettersTo ?? DASH,
            ],
          ]}
        />

        <div style={{ display: "flex", gap: "6px", marginTop: "12px" }}>
          <Button size="sm" variant="outline" onClick={onEdit}>
            {t("common.edit")}
          </Button>
          <Button size="sm" variant="destructive" onClick={onRemove}>
            {t("common.delete")}
          </Button>
        </div>

        {receivesNothing(entry) && (
          <p style={{ marginTop: "10px", fontSize: "11px", color: "var(--c-warn)" }}>
            {t("board.azure-servicebus.subscriptions.noRuleNote")}
          </p>
        )}
      </div>
    </Panel>
  );
}
