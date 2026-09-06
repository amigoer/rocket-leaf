import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { ListArea, ListPane, Page, PageHeader, Toolbar } from "@/design/shell";
import { Button } from "@/components/ui/button";
import { Spinner } from "@/components/ui/spinner";
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
  DetailPanelHeader,
  JsonBlock,
  KV,
  SectionLabel,
  SelectField,
  Status,
} from "@/components";
import { useServiceBusBrowse } from "@/hooks/azureservicebus/useServiceBusBrowse";
import { useServiceBusEntities } from "@/hooks/azureservicebus/useServiceBusEntities";
import { useServiceBusSubscriptions } from "@/hooks/azureservicebus/useServiceBusSubscriptions";
import { entity as readEntity } from "@/mq/azureservicebus/entities";
import { subscription as readSubscription } from "@/mq/azureservicebus/subscriptions";
import {
  contentType,
  correlationId,
  deliveryCount,
  expiresAt,
  groupingKey,
  nextSequence,
  scheduledFor,
  senderProperties,
  sequenceNumber,
  state,
} from "@/mq/azureservicebus/messages";
import { formatMessageTime } from "@/lib/time";
import type { MessageItem } from "@bindings/model/models";

const MONO11 = { fontSize: "11px" } as const;
const RIGHT = { textAlign: "right" } as const;
const DASH = "—";
const BROWSE_LIMIT = 100;

function preview(message: MessageItem): string {
  const body = message.body.replace(/\s+/g, " ").trim();
  return body.length > 90 ? `${body.slice(0, 90)}…` : body;
}

/** One picker entry: a queue, or a topic's subscription. */
interface Target {
  key: string;
  label: string;
  entity: string;
  subscription: string;
}

/**
 * Azure Service Bus messages.
 *
 * The one board in this app where browsing is free. Every other family's
 * messages page either alters what it read or warns that it does: RabbitMQ's
 * basic.get changes queue state, SQS's ReceiveMessage hides what it read for a
 * visibility timeout, Pub/Sub's Pull raises a delivery attempt that counts
 * towards being dead-lettered. PeekMessages does none of it - no lock, no
 * move, no delivery count - so there is no banner here and no caveat on the
 * capability, and their absence is deliberate rather than forgotten.
 *
 * What that buys is a page that shows more than a consumer would ever see. A
 * scheduled message is held back until its enqueue time and a deferred one has
 * been set aside by sequence number; neither is offered to any receiver, and
 * both appear here with a state saying which.
 *
 * The picker offers queues and subscriptions and not topics, and that is the
 * family: a topic holds nothing to read. Its messages are in the subscriptions.
 */
export function MessagesAzureServiceBus() {
  const { t } = useTranslation();
  const entities = useServiceBusEntities();
  const subscriptions = useServiceBusSubscriptions();
  const browse = useServiceBusBrowse();

  const targets = useMemo<Target[]>(() => {
    const queues = (entities.data ?? [])
      .map(readEntity)
      .filter((row) => row.kind === "queue")
      .map((row) => ({ key: row.name, label: row.name, entity: row.name, subscription: "" }));
    const subs = (subscriptions.data ?? []).map(readSubscription).map((row) => ({
      key: `${row.topic}/${row.name}`,
      label: `${row.topic}/${row.name}`,
      entity: row.topic,
      subscription: row.name,
    }));
    return [...queues, ...subs];
  }, [entities.data, subscriptions.data]);

  const [chosenKey, setChosenKey] = useState("");
  const [selected, setSelected] = useState<number | null>(null);
  const [from, setFrom] = useState(0);

  const chosen = targets.find((target) => target.key === chosenKey) ?? targets[0] ?? null;
  const panel = useMemo(
    () => browse.messages.find((message) => sequenceNumber(message) === selected) ?? null,
    [browse.messages, selected],
  );
  const next = nextSequence(browse.messages);

  const run = (start: number) => {
    if (chosen == null) return;
    setFrom(start);
    void browse.run({
      entity: chosen.entity,
      subscription: chosen.subscription,
      deadLetters: false,
      fromSequence: start,
      limit: BROWSE_LIMIT,
    });
  };

  return (
    <Page>
      <PageHeader
        title={t("board.azure-servicebus.messages.title")}
        subtitle={t("board.azure-servicebus.messages.subtitle")}
      />
      <Toolbar>
        <div style={{ width: "300px", flex: "none" }}>
          <SelectField<string>
            value={chosen?.key ?? ""}
            options={targets.map((target) => ({ value: target.key, label: target.label }))}
            placeholder={t("board.azure-servicebus.messages.pickEntity")}
            onValueChange={setChosenKey}
          />
        </div>
        <Button disabled={chosen == null || browse.loading} onClick={() => run(0)}>
          {browse.loading && <Spinner className="size-3.5" />}
          {t("board.azure-servicebus.messages.peek")}
        </Button>
        {/* A peek carries a cursor, so "more" is a sequence number rather than
            a page token: it starts where the last row left off. */}
        <Button
          variant="outline"
          disabled={chosen == null || browse.loading || next == null}
          onClick={() => next != null && run(next)}
        >
          {t("board.azure-servicebus.messages.more")}
        </Button>
        <span className="flex-1" />
        <span style={{ fontSize: "11px", color: "var(--c-muted)" }}>
          {from > 0
            ? t("board.azure-servicebus.messages.foundFrom", {
                count: browse.messages.length,
                from,
              })
            : t("board.azure-servicebus.messages.found", { count: browse.messages.length })}
        </span>
      </Toolbar>

      <ListArea>
        <ListPane>
          {browse.error != null ? (
            <div style={{ padding: "24px", fontSize: "11.5px", color: "var(--c-err)" }}>
              {browse.error}
            </div>
          ) : browse.messages.length === 0 ? (
            <div
              style={{
                padding: "24px",
                fontSize: "11.5px",
                color: "var(--c-muted)",
                textAlign: "center",
              }}
            >
              {browse.searched
                ? t("board.azure-servicebus.messages.nothingHeld")
                : t("board.azure-servicebus.messages.startHere")}
            </div>
          ) : (
            <Table inset>
              <TableHeader>
                <TableRow>
                  <TableHead style={RIGHT}>
                    {t("board.azure-servicebus.messages.sequence")}
                  </TableHead>
                  <TableHead>{t("board.azure-servicebus.messages.enqueued")}</TableHead>
                  <TableHead>{t("board.azure-servicebus.messages.state")}</TableHead>
                  <TableHead style={RIGHT}>
                    {t("board.azure-servicebus.messages.deliveries")}
                  </TableHead>
                  <TableHead>{t("board.azure-servicebus.messages.body")}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {browse.messages.map((message) => (
                  <TableRow
                    key={sequenceNumber(message) ?? message.messageId}
                    selected={selected === sequenceNumber(message)}
                    onClick={() => setSelected(sequenceNumber(message))}
                  >
                    <TableCell className="mono3" style={RIGHT}>
                      {sequenceNumber(message) ?? DASH}
                    </TableCell>
                    <TableCell className="mono3" style={MONO11}>
                      {formatMessageTime(message.storeTimestamp)}
                    </TableCell>
                    <TableCell className="mono3" style={MONO11}>
                      <StateLabel message={message} />
                    </TableCell>
                    <TableCell className="mono3" style={RIGHT}>
                      {deliveryCount(message) ?? DASH}
                    </TableCell>
                    <TableCell className="mono3" style={MONO11}>
                      {preview(message)}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </ListPane>

        {panel != null && (
          <DetailPanel width={420} onDismiss={() => setSelected(null)}>
            <DetailPanelHeader
              title={String(sequenceNumber(panel) ?? panel.messageId)}
              badge={
                <Status tone="off" style={{ fontSize: "10px" }}>
                  {panel.topic}
                </Status>
              }
              onClose={() => setSelected(null)}
            />
            <DetailPanelBody>
              <KV
                rows={[
                  [
                    t("board.azure-servicebus.messages.enqueued"),
                    formatMessageTime(panel.storeTimestamp),
                  ],
                  [t("board.azure-servicebus.messages.messageId"), panel.messageId || DASH],
                  [t("board.azure-servicebus.messages.subject"), panel.tags || DASH],
                  [
                    t("board.azure-servicebus.messages.deliveries"),
                    deliveryCount(panel) == null ? DASH : String(deliveryCount(panel)),
                  ],
                  [t("board.azure-servicebus.messages.groupingKey"), groupingKey(panel) ?? DASH],
                  [t("board.azure-servicebus.messages.correlationId"), correlationId(panel) ?? DASH],
                  [t("board.azure-servicebus.messages.contentType"), contentType(panel) ?? DASH],
                  [t("board.azure-servicebus.messages.expires"), expiresAt(panel) ?? DASH],
                ]}
              />

              {/* Said where the state is: a scheduled message has not been
                  handed to anything yet, and nothing but a peek would show it
                  at all. */}
              {state(panel) === "scheduled" && (
                <p style={{ fontSize: "11px", color: "var(--c-muted)" }}>
                  {t("board.azure-servicebus.messages.scheduledNote", {
                    at: scheduledFor(panel) ?? DASH,
                  })}
                </p>
              )}
              {state(panel) === "deferred" && (
                <p style={{ fontSize: "11px", color: "var(--c-muted)" }}>
                  {t("board.azure-servicebus.messages.deferredNote")}
                </p>
              )}

              {senderProperties(panel).length > 0 && (
                <>
                  <SectionLabel>
                    {t("board.azure-servicebus.messages.section.properties")}
                  </SectionLabel>
                  <KV rows={senderProperties(panel)} />
                </>
              )}

              <SectionLabel>{t("board.azure-servicebus.messages.body")}</SectionLabel>
              <JsonBlock>{panel.body}</JsonBlock>
            </DetailPanelBody>
          </DetailPanel>
        )}
      </ListArea>
    </Page>
  );
}

function StateLabel({ message }: { message: MessageItem }) {
  const { t } = useTranslation();
  switch (state(message)) {
    case "scheduled":
      return <>{t("board.azure-servicebus.messages.stateScheduled")}</>;
    case "deferred":
      return <>{t("board.azure-servicebus.messages.stateDeferred")}</>;
    default:
      return <>{t("board.azure-servicebus.messages.stateActive")}</>;
  }
}
