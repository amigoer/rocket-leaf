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
import { useServiceBusEntities } from "@/hooks/azureservicebus/useServiceBusEntities";
import {
  discardsEverything,
  entity as readEntity,
  isDisabled,
  type ServiceBusEntity,
} from "@/mq/azureservicebus/entities";
import { formatCount } from "@/lib/format";
import { formatErrorMessage } from "@/lib/utils";
import { useConnectionScope } from "@/mq/ConnectionScope";
import * as serviceBusApi from "@/api/azureservicebus";
import { EntityDialogAzureServiceBus } from "./EntityDialogAzureServiceBus";

const MONO11 = { fontSize: "11px" } as const;

/** A dash, not a zero: an absent figure and a measured zero are not alike. */
const DASH = "—";

/** Seconds as the service stores them, read back as something human. */
export function duration(seconds: number | null): string {
  if (seconds == null) return DASH;
  // The service spells "never" as P10675199DT2H48M5.4775807S, which is a real
  // number of seconds and useless as one. Anything past a century is that.
  if (seconds > 100 * 365 * 86400) return "∞";
  if (seconds < 60) return `${seconds}s`;
  if (seconds < 3600) return `${Math.round(seconds / 60)}m`;
  if (seconds < 86400) return `${Math.round(seconds / 3600)}h`;
  return `${Math.round(seconds / 86400)}d`;
}

/** A count, or a dash where the service reports none. */
export function count(value: number | null): string {
  return value == null ? DASH : formatCount(value);
}

/**
 * Azure Service Bus queues and topics.
 *
 * One board for both, because they are the same thing to create, configure and
 * delete: the same properties, the same operations, one name space between
 * them. What separates them is where a message ends up, and the columns say
 * so - a queue has a depth and a topic never can, because a topic holds
 * nothing. A send to it is copied into every subscription whose rules let it
 * through and discarded if none do.
 *
 * A depth that is missing on a queue means something else, and the page says
 * which: the Service Bus emulator reports no message counts at all, so every
 * depth is a dash there and a number against a real namespace.
 *
 * There is no rate column and no chart. Service Bus publishes its rates to
 * Azure Monitor, which is a different API under a different credential, and a
 * figure derived from two samples here would be this app's arithmetic
 * presented as Microsoft's.
 */
export function EntitiesAzureServiceBus() {
  const { t } = useTranslation();
  const state = useServiceBusEntities();
  const { id: connID } = useConnectionScope();
  const confirm = useConfirm();
  const [search, setSearch] = useState("");
  const [selected, setSelected] = useState<string | null>(null);
  const [editing, setEditing] = useState<ServiceBusEntity | null>(null);
  const [formOpen, setFormOpen] = useState(false);

  const entities = useMemo(() => (state.data ?? []).map(readEntity), [state.data]);

  const shown = useMemo(() => {
    const needle = search.trim().toLowerCase();
    return entities.filter((row) => needle === "" || row.name.toLowerCase().includes(needle));
  }, [entities, search]);

  const detail = useMemo(
    () => shown.find((row) => row.name === selected) ?? shown[0] ?? null,
    [shown, selected],
  );

  const save = useCallback(
    async (input: serviceBusApi.AzureServiceBusEntityInput) => {
      if (editing == null) {
        await serviceBusApi.createEntity(connID, input);
        toast.success(t("board.azure-servicebus.entities.created", { name: input.name }));
      } else {
        await serviceBusApi.updateEntity(connID, input);
        toast.success(t("board.azure-servicebus.entities.updated", { name: input.name }));
      }
      await state.refresh();
    },
    [connID, editing, state, t],
  );

  /*
   * The confirmation says what goes with it. An entity's dead letters are a
   * sub-entity of it rather than a queue of their own, and a topic takes every
   * subscription on it - so a delete here discards backlogs that no other
   * board would have shown as being at risk.
   */
  const remove = useCallback(
    async (row: ServiceBusEntity) => {
      const ok = await confirm({
        title: t("board.azure-servicebus.entities.deleteTitle", { name: row.name }),
        description:
          row.kind === "topic"
            ? t("board.azure-servicebus.entities.deleteTopicDesc", { count: row.subscribers ?? 0 })
            : t("board.azure-servicebus.entities.deleteQueueDesc"),
        confirmLabel: t("common.delete"),
        danger: true,
      });
      if (!ok) return;
      try {
        await serviceBusApi.removeEntity(connID, row.name);
        toast.success(t("board.azure-servicebus.entities.deleted", { name: row.name }));
        setSelected(null);
        await state.refresh();
      } catch (deleteError) {
        toast.error(t("board.azure-servicebus.entities.deleteFailed"), {
          description: formatErrorMessage(deleteError),
        });
      }
    },
    [confirm, connID, state, t],
  );

  return (
    <Page>
      <PageHeader
        title={t("shell.nav.azure-servicebus.topics")}
        subtitle={t("board.azure-servicebus.entities.count", {
          queues: entities.filter((row) => row.kind === "queue").length,
          topics: entities.filter((row) => row.kind === "topic").length,
        })}
        actions={
          <div style={{ display: "flex", gap: "8px", alignItems: "center" }}>
            <Input
              style={{ width: "200px" }}
              value={search}
              placeholder={t("board.azure-servicebus.entities.search")}
              onChange={(event) => setSearch(event.target.value)}
            />
            <Button
              size="sm"
              onClick={() => {
                setEditing(null);
                setFormOpen(true);
              }}
            >
              {t("board.azure-servicebus.entities.create")}
            </Button>
            <RefreshButton
              refreshing={state.refreshing}
              online={state.online}
              onClick={() => void state.refresh()}
            />
          </div>
        }
      />
      <EntityDialogAzureServiceBus
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
                    <TableHead>{t("board.azure-servicebus.entities.name")}</TableHead>
                    <TableHead>{t("board.azure-servicebus.entities.kind")}</TableHead>
                    <TableHead className="num">
                      {t("board.azure-servicebus.entities.depth")}
                    </TableHead>
                    <TableHead className="num">
                      {t("board.azure-servicebus.entities.deadLetters")}
                    </TableHead>
                    <TableHead className="num">
                      {t("board.azure-servicebus.entities.subscribers")}
                    </TableHead>
                    <TableHead>{t("board.azure-servicebus.entities.status")}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {shown.map((row) => (
                    <TableRow
                      key={row.name}
                      data-state={detail?.name === row.name ? "selected" : undefined}
                      onClick={() => setSelected(row.name)}
                      style={{ cursor: "pointer" }}
                    >
                      <TableCell>
                        <span className="mono3" style={MONO11}>
                          {row.name}
                        </span>
                        {/* Said on the row rather than only in the panel: it
                            is the one state that looks healthy everywhere
                            else, because a discarded message leaves no
                            backlog behind it. */}
                        {discardsEverything(row) && (
                          <span className="pb pASB" style={{ marginLeft: "6px" }}>
                            {t("board.azure-servicebus.entities.noSubscriberBadge")}
                          </span>
                        )}
                      </TableCell>
                      <TableCell>
                        <span className="mono3" style={MONO11}>
                          {row.kind === "topic"
                            ? t("board.azure-servicebus.entities.kindTopic")
                            : t("board.azure-servicebus.entities.kindQueue")}
                        </span>
                      </TableCell>
                      <TableCell className="num">
                        <span className="mono3" style={MONO11}>
                          {count(row.depth)}
                        </span>
                      </TableCell>
                      <TableCell className="num">
                        <span className="mono3" style={MONO11}>
                          {count(row.deadLetterCount)}
                        </span>
                      </TableCell>
                      <TableCell className="num">
                        <span className="mono3" style={MONO11}>
                          {count(row.subscribers)}
                        </span>
                      </TableCell>
                      <TableCell>
                        <span
                          className="mono3"
                          style={{
                            ...MONO11,
                            color: isDisabled(row) ? "var(--c-warn)" : undefined,
                          }}
                        >
                          {row.status ?? DASH}
                        </span>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </Panel>

            {detail != null && (
              <EntityDetail
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
            {t("board.azure-servicebus.entities.note")}
          </p>
        </PageBody>
      </BoardState>
    </Page>
  );
}

function EntityDetail({
  entry,
  onEdit,
  onRemove,
}: {
  entry: ServiceBusEntity;
  onEdit: () => void;
  onRemove: () => void;
}) {
  const { t } = useTranslation();
  const isTopic = entry.kind === "topic";

  return (
    <Panel style={{ width: "300px", flex: "none", overflow: "auto" }}>
      <PanelHeader title={entry.name} />
      <div style={{ padding: "0 12px 12px" }}>
        <SectionLabel>{t("board.azure-servicebus.entities.section.holding")}</SectionLabel>
        <KV
          rows={
            isTopic
              ? [
                  [t("board.azure-servicebus.entities.subscribers"), count(entry.subscribers)],
                  [
                    t("board.azure-servicebus.entities.subscriptionNames"),
                    entry.subscriptionNames.join(", ") || DASH,
                  ],
                  [t("board.azure-servicebus.entities.scheduled"), count(entry.scheduledCount)],
                ]
              : [
                  [t("board.azure-servicebus.entities.depth"), count(entry.depth)],
                  [t("board.azure-servicebus.entities.deadLetters"), count(entry.deadLetterCount)],
                  [t("board.azure-servicebus.entities.scheduled"), count(entry.scheduledCount)],
                ]
          }
        />

        <SectionLabel>{t("board.azure-servicebus.entities.section.delivery")}</SectionLabel>
        <KV
          rows={[
            [t("board.azure-servicebus.entities.status"), entry.status ?? DASH],
            [t("board.azure-servicebus.entities.lockDuration"), duration(entry.lockDurationSec)],
            [t("board.azure-servicebus.entities.maxDelivery"), count(entry.maxDeliveryCount)],
            [t("board.azure-servicebus.entities.ttl"), duration(entry.ttlSec)],
            [
              t("board.azure-servicebus.entities.autoDelete"),
              duration(entry.autoDeleteOnIdleSec),
            ],
          ]}
        />

        <SectionLabel>{t("board.azure-servicebus.entities.section.shape")}</SectionLabel>
        <KV
          rows={[
            [
              t("board.azure-servicebus.entities.maxSize"),
              entry.maxSizeMb == null ? DASH : `${formatCount(entry.maxSizeMb)} MB`,
            ],
            [t("board.azure-servicebus.entities.sessions"), yesNo(t, entry.requiresSession)],
            [
              t("board.azure-servicebus.entities.duplicateDetection"),
              yesNo(t, entry.requiresDuplicateDetection),
            ],
            [t("board.azure-servicebus.entities.partitioned"), yesNo(t, entry.partitioned)],
            [t("board.azure-servicebus.entities.forwardTo"), entry.forwardTo ?? DASH],
            [
              t("board.azure-servicebus.entities.forwardDeadLettersTo"),
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

        {/* Said where the number is, because a topic nothing subscribes to
            reports success on every send and leaves nothing behind. */}
        {discardsEverything(entry) && (
          <p style={{ marginTop: "10px", fontSize: "11px", color: "var(--c-warn)" }}>
            {t("board.azure-servicebus.entities.noSubscriberNote")}
          </p>
        )}
      </div>
    </Panel>
  );
}

function yesNo(t: (key: string) => string, value: boolean): string {
  return value ? t("common.yes") : t("common.no");
}
