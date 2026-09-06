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
import { useSqsBrowse } from "@/hooks/sqs/useSqsMessages";
import { useSqsDestinations } from "@/hooks/sqs/useSqsDestinations";
import { queue as readQueue } from "@/mq/sqs/destinations";
import {
  bodyMd5,
  deduplicationId,
  firstReceivedAtMs,
  groupId,
  messageIdOf,
  producerAttributes,
  receiveCount,
  senderId,
  sentAtMs,
  sequenceNumber,
} from "@/mq/sqs/messages";
import { formatCount } from "@/lib/format";
import { formatMessageTime } from "@/lib/time";
import type { MessageItem } from "@bindings/model/models";

const MONO11 = { fontSize: "11px" } as const;
const RIGHT = { textAlign: "right" } as const;
const DASH = "—";

/** How many messages one browse asks for. Ten is one ReceiveMessage. */
const BROWSE_LIMIT = 10;

function preview(message: MessageItem): string {
  const body = message.body ?? "";
  return body.length > 120 ? `${body.slice(0, 120)}…` : body;
}

/**
 * Browsing an SQS queue.
 *
 * A button rather than a list that loads itself, and that is not a style
 * choice. SQS has one read - ReceiveMessage - and it is the same call a
 * consumer makes: what comes back is hidden from everyone else until the
 * driver hands it straight back, and each message's receive count goes up for
 * good. A page that fetched on mount and refreshed on a timer would be taking
 * messages away from a live consumer for nobody's benefit.
 *
 * The banner says so. The driver declares the same thing as a caveat on the
 * capability, which is what the sidebar shows; this is where somebody about to
 * press the button reads it.
 *
 * There are no filters, because there is nothing to filter with: SQS has no
 * server-side selector, so narrowing would mean receiving everything and
 * discarding most of it - hiding far more messages, for far longer, than the
 * page showed.
 */
export function MessagesSqs() {
  const { t } = useTranslation();
  const queues = useSqsDestinations();
  const browse = useSqsBrowse();

  const names = useMemo(
    () => (queues.data ?? []).map((row) => readQueue(row).name),
    [queues.data],
  );

  const [queue, setQueue] = useState("");
  const [selected, setSelected] = useState<string | null>(null);

  const chosen = queue !== "" ? queue : (names[0] ?? "");
  const panel = useMemo(
    () => browse.messages.find((message) => messageIdOf(message) === selected) ?? null,
    [browse.messages, selected],
  );

  return (
    <Page>
      <PageHeader title={t("board.sqs.messages.title")} subtitle={t("board.sqs.messages.subtitle")} />
      <Toolbar>
        <div style={{ width: "260px", flex: "none" }}>
          <SelectField<string>
            value={chosen}
            options={names.map((name) => ({ value: name, label: name }))}
            placeholder={t("board.sqs.messages.pickQueue")}
            onValueChange={setQueue}
          />
        </div>
        <Button
          disabled={chosen === "" || browse.loading}
          onClick={() => void browse.run(chosen, BROWSE_LIMIT)}
        >
          {browse.loading && <Spinner className="size-3.5" />}
          {t("board.sqs.messages.receive")}
        </Button>
        <span className="flex-1" />
        <span style={{ fontSize: "11px", color: "var(--c-muted)" }}>
          {t("board.sqs.messages.found", { count: browse.messages.length })}
        </span>
      </Toolbar>

      {/* Said before the button rather than after it. Everything on this page
          is a real receive: the messages were hidden from live consumers while
          it ran, and their receive counts do not come back down. */}
      <div
        style={{
          padding: "6px 12px",
          fontSize: "11px",
          color: "var(--c-warn-text)",
          borderBottom: "1px solid var(--c-border)",
        }}
      >
        {t("board.sqs.messages.receiveNote")}
      </div>

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
                ? t("board.sqs.messages.nothingVisible")
                : t("board.sqs.messages.startHere")}
            </div>
          ) : (
            <Table inset>
              <TableHeader>
                <TableRow>
                  <TableHead>{t("board.sqs.messages.messageId")}</TableHead>
                  <TableHead>{t("board.sqs.messages.sent")}</TableHead>
                  <TableHead style={RIGHT}>{t("board.sqs.messages.receiveCount")}</TableHead>
                  <TableHead>{t("board.sqs.messages.body")}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {browse.messages.map((message) => (
                  <TableRow
                    key={messageIdOf(message)}
                    selected={selected === messageIdOf(message)}
                    onClick={() => setSelected(messageIdOf(message))}
                  >
                    <TableCell className="mono3" style={MONO11}>
                      {messageIdOf(message)}
                    </TableCell>
                    <TableCell className="mono3" style={MONO11}>
                      {formatMessageTime(sentAtMs(message))}
                    </TableCell>
                    <TableCell className="mono3" style={RIGHT}>
                      {receiveCount(message) == null
                        ? DASH
                        : formatCount(receiveCount(message) as number)}
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
              title={messageIdOf(panel)}
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
                  [t("board.sqs.messages.sent"), formatMessageTime(sentAtMs(panel))],
                  [
                    t("board.sqs.messages.firstReceived"),
                    firstReceivedAtMs(panel) == null
                      ? DASH
                      : formatMessageTime(firstReceivedAtMs(panel)),
                  ],
                  [
                    t("board.sqs.messages.receiveCount"),
                    receiveCount(panel) == null ? DASH : String(receiveCount(panel)),
                  ],
                  [t("board.sqs.messages.sender"), senderId(panel) ?? DASH],
                  [t("board.sqs.messages.bodyMd5"), bodyMd5(panel) ?? DASH],
                ]}
              />

              {/* FIFO only. On a standard queue every one of these is absent,
                  and a section of dashes would suggest the queue was missing
                  something rather than being a different kind. */}
              {groupId(panel) != null && (
                <>
                  <SectionLabel>{t("board.sqs.messages.section.fifo")}</SectionLabel>
                  <KV
                    rows={[
                      [t("board.sqs.messages.groupId"), groupId(panel) ?? DASH],
                      [t("board.sqs.messages.deduplicationId"), deduplicationId(panel) ?? DASH],
                      [t("board.sqs.messages.sequenceNumber"), sequenceNumber(panel) ?? DASH],
                    ]}
                  />
                </>
              )}

              {producerAttributes(panel).length > 0 && (
                <>
                  <SectionLabel>{t("board.sqs.messages.section.attributes")}</SectionLabel>
                  <KV rows={producerAttributes(panel)} />
                </>
              )}

              <SectionLabel>{t("board.sqs.messages.body")}</SectionLabel>
              <JsonBlock>{panel.body}</JsonBlock>
            </DetailPanelBody>
          </DetailPanel>
        )}
      </ListArea>
    </Page>
  );
}
