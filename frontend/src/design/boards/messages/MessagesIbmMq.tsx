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
import { useIbmMqBrowse } from "@/hooks/ibmmq/useIbmMqBrowse";
import { useIbmMqDestinations } from "@/hooks/ibmmq/useIbmMqDestinations";
import { destination as readDestination } from "@/mq/ibmmq/destinations";
import {
  bodyUnavailableAs,
  correlationIdOf,
  expiryOf,
  formatOf,
  messageIdOf,
  persistenceOf,
  replyToOf,
  replyToQmgrOf,
  userProperties,
} from "@/mq/ibmmq/messages";
import type { MessageItem } from "@bindings/model/models";

const MONO11 = { fontSize: "11px" } as const;
const DASH = "—";

/** How many messages one browse asks for. Each one costs its own request. */
const BROWSE_LIMIT = 50;

function preview(message: MessageItem): string {
  const refused = bodyUnavailableAs(message);
  if (refused != null) return "";
  const body = message.body ?? "";
  return body.length > 120 ? `${body.slice(0, 120)}…` : body;
}

/**
 * Browsing a queue.
 *
 * A button rather than a list that loads itself, and not for the reason SQS
 * and Pub/Sub have one. An MQ browse takes nothing: the depth is the same
 * afterwards, the messages stay in order, and any number of readers can look
 * at the same one. What it costs is round trips - the driver lists the
 * identifiers in one request and reads each message in another - so a page
 * that refreshed on a timer would be making fifty calls at whoever left the
 * tab open.
 *
 * Only queues are offered, and a topic is not a gap in the list. A topic
 * stores nothing; a publication lives on the queues its subscriptions deliver
 * to, and those are queues and are browsed here like any other.
 *
 * A row whose body was refused is still a row. The messaging interface carries
 * character data and nothing else, so a dead letter or an event message is
 * listed with its identifier and its format and cannot be opened - and a
 * browse that quietly returned fewer rows than the depth would be the more
 * confusing answer.
 */
export function MessagesIbmMq() {
  const { t } = useTranslation();
  const destinations = useIbmMqDestinations();
  const browse = useIbmMqBrowse();

  const queues = useMemo(
    () =>
      (destinations.data ?? [])
        .map(readDestination)
        .filter((entry) => entry.kind === "queue" && entry.queueType === "local")
        .map((entry) => entry.name),
    [destinations.data],
  );

  const [queue, setQueue] = useState("");
  const [selected, setSelected] = useState<string | null>(null);

  const chosen = queue !== "" ? queue : (queues[0] ?? "");
  const panel = useMemo(
    () => browse.messages.find((message) => messageIdOf(message) === selected) ?? null,
    [browse.messages, selected],
  );

  return (
    <Page>
      <PageHeader
        title={t("board.ibmmq.messages.title")}
        subtitle={t("board.ibmmq.messages.subtitle")}
      />
      <Toolbar>
        <div style={{ width: "260px", flex: "none" }}>
          <SelectField<string>
            value={chosen}
            options={queues.map((name) => ({ value: name, label: name }))}
            placeholder={t("board.ibmmq.messages.pickQueue")}
            onValueChange={setQueue}
          />
        </div>
        <Button
          disabled={chosen === "" || browse.loading}
          onClick={() => void browse.run(chosen, BROWSE_LIMIT)}
        >
          {browse.loading && <Spinner className="size-3.5" />}
          {t("board.ibmmq.messages.read")}
        </Button>
        <span className="flex-1" />
        <span style={{ fontSize: "11px", color: "var(--c-muted)" }}>
          {t("board.ibmmq.messages.found", { count: browse.messages.length })}
        </span>
      </Toolbar>

      {/* Said before the button, and it is the opposite of what the hosted
          families say here: nothing on this page is taken from anyone. What
          the server will not do is hand back a body it cannot read as text. */}
      <div
        style={{
          padding: "6px 12px",
          fontSize: "11px",
          color: "var(--c-muted)",
          borderBottom: "1px solid var(--c-border)",
        }}
      >
        {t("board.ibmmq.messages.readNote")}
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
                ? t("board.ibmmq.messages.nothingThere")
                : t("board.ibmmq.messages.startHere")}
            </div>
          ) : (
            <Table inset>
              <TableHeader>
                <TableRow>
                  <TableHead>{t("board.ibmmq.messages.messageId")}</TableHead>
                  <TableHead>{t("board.ibmmq.messages.format")}</TableHead>
                  <TableHead>{t("board.ibmmq.messages.persistence")}</TableHead>
                  <TableHead>{t("board.ibmmq.messages.body")}</TableHead>
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
                      {formatOf(message) ?? DASH}
                    </TableCell>
                    <TableCell className="mono3" style={MONO11}>
                      {persistenceOf(message) ?? DASH}
                    </TableCell>
                    <TableCell className="mono3" style={MONO11}>
                      {bodyUnavailableAs(message) != null ? (
                        <span style={{ color: "var(--c-muted)" }}>
                          {t("board.ibmmq.messages.bodyUnavailable", {
                            format: bodyUnavailableAs(message),
                          })}
                        </span>
                      ) : (
                        preview(message)
                      )}
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
                  [t("board.ibmmq.messages.format"), formatOf(panel) ?? DASH],
                  [t("board.ibmmq.messages.persistence"), persistenceOf(panel) ?? DASH],
                  [t("board.ibmmq.messages.expiry"), expiryOf(panel) ?? DASH],
                  [t("board.ibmmq.messages.correlationId"), correlationIdOf(panel) ?? DASH],
                  [t("board.ibmmq.messages.replyTo"), replyToOf(panel) ?? DASH],
                  [t("board.ibmmq.messages.replyToQmgr"), replyToQmgrOf(panel) ?? DASH],
                ]}
              />
              {/* Spelled out because a reader will look for it: there is no
                  put time in the descriptor mqweb returns, and this app is not
                  going to print its own clock in its place. */}
              <p style={{ margin: "6px 0 0", fontSize: "11px", color: "var(--c-muted)" }}>
                {t("board.ibmmq.messages.noPutTime")}
              </p>

              {userProperties(panel).length > 0 && (
                <>
                  <SectionLabel>{t("board.ibmmq.messages.section.properties")}</SectionLabel>
                  <KV rows={userProperties(panel)} />
                </>
              )}

              <SectionLabel>{t("board.ibmmq.messages.body")}</SectionLabel>
              {bodyUnavailableAs(panel) != null ? (
                <p style={{ margin: 0, fontSize: "11px", color: "var(--c-muted)" }}>
                  {t("board.ibmmq.messages.bodyUnavailableNote", {
                    format: bodyUnavailableAs(panel),
                  })}
                </p>
              ) : (
                <JsonBlock>{panel.body}</JsonBlock>
              )}
            </DetailPanelBody>
          </DetailPanel>
        )}
      </ListArea>
    </Page>
  );
}
