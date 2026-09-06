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
import { useServiceBusRouting } from "@/hooks/azureservicebus/useServiceBusRouting";
import {
  DEFAULT_RULE,
  matchesEverything,
  matchesNothing,
  rule as readRule,
  type ServiceBusRule,
} from "@/mq/azureservicebus/rules";
import { entity as readEntity } from "@/mq/azureservicebus/entities";
import { formatErrorMessage } from "@/lib/utils";
import { useConnectionScope } from "@/mq/ConnectionScope";
import * as serviceBusApi from "@/api/azureservicebus";
import { RuleDialogAzureServiceBus } from "./RuleDialogAzureServiceBus";

const MONO11 = { fontSize: "11px" } as const;
const DASH = "—";

/**
 * Azure Service Bus rules.
 *
 * The routing page, and this family earns one where the two hosted families
 * before it did not. Their filtering is a field on the reader: a Pub/Sub
 * subscription carries a filter string set once at creation, an SQS queue
 * carries nothing at all. A Service Bus subscription carries rules - objects
 * with names, several to a subscription, each a filter and optionally an
 * action that rewrites the message on the way in - so which messages reach
 * which subscription is a topology, and it belongs beside RabbitMQ's exchanges
 * and bindings rather than on a form.
 *
 * Two states this page exists to make visible, and neither shows anywhere
 * else. A subscription whose rules have all been deleted receives nothing
 * while reporting itself Active with an empty backlog. And a rule with a false
 * filter is legal, matches nothing, and looks exactly like one that works.
 */
export function RulesAzureServiceBus() {
  const { t } = useTranslation();
  const state = useServiceBusRouting();
  const { id: connID } = useConnectionScope();
  const confirm = useConfirm();
  const [search, setSearch] = useState("");
  const [selected, setSelected] = useState<string | null>(null);
  const [formOpen, setFormOpen] = useState(false);

  const rules = useMemo(() => (state.data?.rules ?? []).map(readRule), [state.data]);
  const topics = useMemo(
    () => (state.data?.topics ?? []).map(readEntity),
    [state.data],
  );

  const shown = useMemo(() => {
    const needle = search.trim().toLowerCase();
    return rules.filter(
      (row) =>
        needle === "" ||
        row.topic.toLowerCase().includes(needle) ||
        row.subscription.toLowerCase().includes(needle) ||
        row.name.toLowerCase().includes(needle),
    );
  }, [rules, search]);

  const key = (row: ServiceBusRule) => `${row.topic}/${row.subscription}/${row.name}`;
  const detail = useMemo(
    () => shown.find((row) => key(row) === selected) ?? shown[0] ?? null,
    [shown, selected],
  );

  const save = useCallback(
    async (input: serviceBusApi.AzureServiceBusRuleInput) => {
      await serviceBusApi.createRule(connID, input);
      toast.success(t("board.azure-servicebus.rules.created", { name: input.name }));
      await state.refresh();
    },
    [connID, state, t],
  );

  /*
   * The confirmation is about the last one rather than about this one. A
   * subscription with no rules is not an error anywhere: the service allows
   * it, the subscription stays Active, and its backlog stays empty because
   * nothing can arrive.
   */
  const remove = useCallback(
    async (row: ServiceBusRule) => {
      const siblings = rules.filter(
        (other) => other.topic === row.topic && other.subscription === row.subscription,
      );
      const ok = await confirm({
        title: t("board.azure-servicebus.rules.deleteTitle", { name: row.name }),
        description:
          siblings.length <= 1
            ? t("board.azure-servicebus.rules.deleteLastDesc", { subscription: row.subscription })
            : t("board.azure-servicebus.rules.deleteDesc", { subscription: row.subscription }),
        confirmLabel: t("common.delete"),
        danger: true,
      });
      if (!ok) return;
      try {
        await serviceBusApi.removeRule(connID, row.topic, row.subscription, row.name);
        toast.success(t("board.azure-servicebus.rules.deleted", { name: row.name }));
        setSelected(null);
        await state.refresh();
      } catch (deleteError) {
        toast.error(t("board.azure-servicebus.rules.deleteFailed"), {
          description: formatErrorMessage(deleteError),
        });
      }
    },
    [confirm, connID, rules, state, t],
  );

  return (
    <Page>
      <PageHeader
        title={t("shell.nav.azure-servicebus.exchanges")}
        subtitle={t("board.azure-servicebus.rules.count", {
          rules: rules.length,
          topics: topics.length,
        })}
        actions={
          <div style={{ display: "flex", gap: "8px", alignItems: "center" }}>
            <Input
              style={{ width: "200px" }}
              value={search}
              placeholder={t("board.azure-servicebus.rules.search")}
              onChange={(event) => setSearch(event.target.value)}
            />
            <Button size="sm" onClick={() => setFormOpen(true)}>
              {t("board.azure-servicebus.rules.create")}
            </Button>
            <RefreshButton
              refreshing={state.refreshing}
              online={state.online}
              onClick={() => void state.refresh()}
            />
          </div>
        }
      />
      <RuleDialogAzureServiceBus
        open={formOpen}
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
                    <TableHead>{t("board.azure-servicebus.rules.topic")}</TableHead>
                    <TableHead>{t("board.azure-servicebus.rules.subscription")}</TableHead>
                    <TableHead>{t("board.azure-servicebus.rules.name")}</TableHead>
                    <TableHead>{t("board.azure-servicebus.rules.filter")}</TableHead>
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
                          {row.topic}
                        </span>
                      </TableCell>
                      <TableCell>
                        <span className="mono3" style={MONO11}>
                          {row.subscription}
                        </span>
                      </TableCell>
                      <TableCell>
                        <span className="mono3" style={MONO11}>
                          {row.name}
                        </span>
                        {row.name === DEFAULT_RULE && (
                          <span className="pb pASB" style={{ marginLeft: "6px" }}>
                            {t("board.azure-servicebus.rules.defaultBadge")}
                          </span>
                        )}
                      </TableCell>
                      <TableCell>
                        <span
                          className="mono3"
                          style={{
                            ...MONO11,
                            color: matchesNothing(row) ? "var(--c-warn)" : undefined,
                          }}
                        >
                          {row.summary || DASH}
                        </span>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </Panel>

            {detail != null && <RuleDetail entry={detail} onRemove={() => void remove(detail)} />}
          </div>
          <p
            style={{
              margin: "0 20px",
              fontSize: "11.5px",
              color: "var(--c-muted)",
              flex: "none",
            }}
          >
            {t("board.azure-servicebus.rules.note")}
          </p>
        </PageBody>
      </BoardState>
    </Page>
  );
}

function RuleDetail({ entry, onRemove }: { entry: ServiceBusRule; onRemove: () => void }) {
  const { t } = useTranslation();
  return (
    <Panel style={{ width: "320px", flex: "none", overflow: "auto" }}>
      <PanelHeader title={entry.name} />
      <div style={{ padding: "0 12px 12px" }}>
        <SectionLabel>{t("board.azure-servicebus.rules.section.edge")}</SectionLabel>
        <KV
          rows={[
            [t("board.azure-servicebus.rules.topic"), entry.topic],
            [t("board.azure-servicebus.rules.subscription"), entry.subscription],
            [t("board.azure-servicebus.rules.kind"), entry.kind],
          ]}
        />

        <SectionLabel>{t("board.azure-servicebus.rules.section.filter")}</SectionLabel>
        {entry.kind === "correlation" && entry.correlation.length > 0 ? (
          <KV rows={entry.correlation} />
        ) : (
          <KV rows={[[t("board.azure-servicebus.rules.filter"), entry.summary || DASH]]} />
        )}

        {entry.action != null && (
          <>
            <SectionLabel>{t("board.azure-servicebus.rules.section.action")}</SectionLabel>
            <KV rows={[[t("board.azure-servicebus.rules.action"), entry.action]]} />
            <p style={{ fontSize: "11px", color: "var(--c-muted)" }}>
              {t("board.azure-servicebus.rules.actionNote")}
            </p>
          </>
        )}

        <div style={{ display: "flex", gap: "6px", marginTop: "12px" }}>
          <Button size="sm" variant="destructive" onClick={onRemove}>
            {t("common.delete")}
          </Button>
        </div>

        {matchesEverything(entry) && (
          <p style={{ marginTop: "10px", fontSize: "11px", color: "var(--c-muted)" }}>
            {t("board.azure-servicebus.rules.everythingNote")}
          </p>
        )}
        {matchesNothing(entry) && (
          <p style={{ marginTop: "10px", fontSize: "11px", color: "var(--c-warn)" }}>
            {t("board.azure-servicebus.rules.nothingNote")}
          </p>
        )}
      </div>
    </Panel>
  );
}
