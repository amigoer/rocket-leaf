import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { Page, PageBody, PageHeader, RefreshButton } from "@/design/shell";
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
import { JsonBlock, KV, Panel, PanelHeader, SectionLabel, toast, useConfirm } from "@/components";
import { useServiceBusDeadLetters } from "@/hooks/azureservicebus/useServiceBusDeadLetters";
import {
  deadLetterDescription,
  deadLetterReason,
  deadLetterSource,
  deliveryCount,
  senderProperties,
  sequenceNumber,
} from "@/mq/azureservicebus/messages";
import { count } from "@/design/boards/topics/EntitiesAzureServiceBus";
import { formatMessageTime } from "@/lib/time";
import { formatErrorMessage } from "@/lib/utils";

const MONO11 = { fontSize: "11px" } as const;
const RIGHT = { textAlign: "right" } as const;
const DASH = "—";
const READ_LIMIT = 100;

/**
 * Azure Service Bus dead letters.
 *
 * The left column is every queue and every subscription in the namespace, and
 * that is not a shortcut: each of them is created with a $DeadLetterQueue of
 * its own, so each of them has one whether or not it has ever failed a
 * message. There is no listing to fetch, because a $DeadLetterQueue is part of
 * its entity rather than an object in its own right.
 *
 * That is the whole difference from the two hosted families before this one.
 * SQS's and Pub/Sub's dead-letter pages walk the topology to find which
 * ordinary queue or topic something else points at, and both are empty on an
 * account that has configured no such policy. This page can only be empty when
 * the namespace is.
 *
 * Reading one is a peek and takes nothing, the way the messages page does.
 * Putting one back is not: it receives the message, sends a copy to the parent
 * entity, and completes the original, and the confirmation says so.
 */
export function DlqAzureServiceBus() {
  const { t } = useTranslation();
  const dead = useServiceBusDeadLetters();
  const confirm = useConfirm();
  const [chosen, setChosen] = useState<string | null>(null);
  const [selected, setSelected] = useState<number | null>(null);
  const [resending, setResending] = useState(false);

  const store = useMemo(
    () => dead.stores.find((row) => row.path === chosen) ?? null,
    [dead.stores, chosen],
  );
  const panel = useMemo(
    () => dead.messages.find((message) => sequenceNumber(message) === selected) ?? null,
    [dead.messages, selected],
  );

  const open = (path: string) => {
    setChosen(path);
    setSelected(null);
    void dead.read(path, READ_LIMIT);
  };

  const putBack = async () => {
    if (store == null || panel == null || resending) return;
    const sequence = sequenceNumber(panel);
    if (sequence == null) return;
    const ok = await confirm({
      title: t("board.azure-servicebus.dlq.resendTitle", { sequence }),
      description: t("board.azure-servicebus.dlq.resendDesc", { entity: store.path }),
      confirmLabel: t("board.azure-servicebus.dlq.resend"),
    });
    if (!ok) return;
    setResending(true);
    try {
      await dead.resend(store.path, sequence);
      toast.success(t("board.azure-servicebus.dlq.resent", { sequence }));
      setSelected(null);
      await dead.refresh();
    } catch (resendError) {
      toast.error(t("board.azure-servicebus.dlq.resendFailed"), {
        description: formatErrorMessage(resendError),
      });
    } finally {
      setResending(false);
    }
  };

  return (
    <Page>
      <PageHeader
        title={t("shell.nav.azure-servicebus.dlq")}
        subtitle={t("board.azure-servicebus.dlq.subtitle")}
        actions={
          <RefreshButton
            refreshing={dead.loading}
            online
            onClick={() => void dead.refresh()}
          />
        }
      />
      <PageBody>
        <div style={{ display: "flex", gap: "12px", minHeight: 0, flex: 1 }}>
          <Panel style={{ width: "280px", flex: "none", overflow: "auto" }}>
            <PanelHeader title={t("board.azure-servicebus.dlq.stores")} />
            {dead.storesError != null ? (
              <div style={{ padding: "16px", fontSize: "11.5px", color: "var(--c-err)" }}>
                {dead.storesError}
              </div>
            ) : dead.stores.length === 0 ? (
              <div style={{ padding: "16px", fontSize: "11.5px", color: "var(--c-muted)" }}>
                {t("board.azure-servicebus.dlq.noEntities")}
              </div>
            ) : (
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t("board.azure-servicebus.dlq.entity")}</TableHead>
                    <TableHead className="num">
                      {t("board.azure-servicebus.dlq.held")}
                    </TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {dead.stores.map((row) => (
                    <TableRow
                      key={row.path}
                      data-state={chosen === row.path ? "selected" : undefined}
                      onClick={() => open(row.path)}
                      style={{ cursor: "pointer" }}
                    >
                      <TableCell>
                        <span className="mono3" style={MONO11}>
                          {row.path}
                        </span>
                      </TableCell>
                      <TableCell className="num">
                        <span className="mono3" style={MONO11}>
                          {count(row.count)}
                        </span>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            )}
          </Panel>

          <Panel style={{ flex: 1, overflow: "auto" }}>
            <PanelHeader
              title={
                store == null
                  ? t("board.azure-servicebus.dlq.pickOne")
                  : `${store.path}/$DeadLetterQueue`
              }
            />
            {dead.reading ? (
              <div style={{ padding: "24px", textAlign: "center" }}>
                <Spinner />
              </div>
            ) : dead.error != null ? (
              <div style={{ padding: "24px", fontSize: "11.5px", color: "var(--c-err)" }}>
                {dead.error}
              </div>
            ) : dead.messages.length === 0 ? (
              <div
                style={{
                  padding: "24px",
                  fontSize: "11.5px",
                  color: "var(--c-muted)",
                  textAlign: "center",
                }}
              >
                {dead.searched
                  ? t("board.azure-servicebus.dlq.nothingGivenUp")
                  : t("board.azure-servicebus.dlq.startHere")}
              </div>
            ) : (
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead style={RIGHT}>{t("board.azure-servicebus.dlq.sequence")}</TableHead>
                    <TableHead>{t("board.azure-servicebus.dlq.reason")}</TableHead>
                    <TableHead style={RIGHT}>
                      {t("board.azure-servicebus.dlq.deliveries")}
                    </TableHead>
                    <TableHead>{t("board.azure-servicebus.dlq.enqueued")}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {dead.messages.map((message) => (
                    <TableRow
                      key={sequenceNumber(message) ?? message.messageId}
                      data-state={
                        selected === sequenceNumber(message) ? "selected" : undefined
                      }
                      onClick={() => setSelected(sequenceNumber(message))}
                      style={{ cursor: "pointer" }}
                    >
                      <TableCell className="num">
                        <span className="mono3" style={MONO11}>
                          {sequenceNumber(message) ?? DASH}
                        </span>
                      </TableCell>
                      <TableCell>
                        <span className="mono3" style={MONO11}>
                          {deadLetterReason(message) ?? DASH}
                        </span>
                      </TableCell>
                      <TableCell className="num">
                        <span className="mono3" style={MONO11}>
                          {deliveryCount(message) ?? DASH}
                        </span>
                      </TableCell>
                      <TableCell>
                        <span className="mono3" style={MONO11}>
                          {formatMessageTime(message.storeTimestamp)}
                        </span>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            )}
          </Panel>

          {panel != null && (
            <Panel style={{ width: "320px", flex: "none", overflow: "auto" }}>
              <PanelHeader title={String(sequenceNumber(panel) ?? panel.messageId)} />
              <div style={{ padding: "0 12px 12px" }}>
                <SectionLabel>{t("board.azure-servicebus.dlq.section.whyItStopped")}</SectionLabel>
                <KV
                  rows={[
                    [t("board.azure-servicebus.dlq.reason"), deadLetterReason(panel) ?? DASH],
                    [
                      t("board.azure-servicebus.dlq.description"),
                      deadLetterDescription(panel) ?? DASH,
                    ],
                    [
                      t("board.azure-servicebus.dlq.deliveries"),
                      deliveryCount(panel) == null ? DASH : String(deliveryCount(panel)),
                    ],
                    [
                      t("board.azure-servicebus.dlq.enqueued"),
                      formatMessageTime(panel.storeTimestamp),
                    ],
                    [t("board.azure-servicebus.dlq.subject"), panel.tags || DASH],
                  ]}
                />

                {/* Only set when the message arrived here by forwarding, and
                    then it names where it actually failed - which is not the
                    store it is sitting in. */}
                {deadLetterSource(panel) != null && (
                  <p style={{ fontSize: "11px", color: "var(--c-muted)" }}>
                    {t("board.azure-servicebus.dlq.forwardedFrom", {
                      source: deadLetterSource(panel),
                    })}
                  </p>
                )}

                {senderProperties(panel).length > 0 && (
                  <>
                    <SectionLabel>
                      {t("board.azure-servicebus.dlq.section.properties")}
                    </SectionLabel>
                    <KV rows={senderProperties(panel)} />
                  </>
                )}

                <SectionLabel>{t("board.azure-servicebus.dlq.body")}</SectionLabel>
                <JsonBlock>{panel.body}</JsonBlock>

                <Button
                  size="sm"
                  style={{ marginTop: "12px" }}
                  disabled={resending}
                  onClick={() => void putBack()}
                >
                  {resending && <Spinner className="size-3.5" />}
                  {t("board.azure-servicebus.dlq.resend")}
                </Button>
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
          {t("board.azure-servicebus.dlq.note")}
        </p>
      </PageBody>
    </Page>
  );
}
