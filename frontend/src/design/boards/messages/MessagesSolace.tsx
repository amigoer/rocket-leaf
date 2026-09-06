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
  KV,
  SectionLabel,
  SelectField,
  Status,
} from "@/components";
import { useSolaceBrowse } from "@/hooks/solace/useSolaceBrowse";
import { useSolaceDestinations } from "@/hooks/solace/useSolaceDestinations";
import { destination as readDestination } from "@/mq/solace/destinations";
import {
  attachmentSizeOf,
  contentSizeOf,
  dmqEligible,
  messageIdOf,
  otherProperties,
  partitionKeyOf,
  publisherIdOf,
  redeliveryCountOf,
  replicationIdOf,
  replicationStateOf,
  undelivered,
} from "@/mq/solace/messages";
import { formatBytes } from "@/lib/format";

const MONO11 = { fontSize: "11px" } as const;
const DASH = "—";

/** How many messages one browse asks for. */
const BROWSE_LIMIT = 100;

function size(value: number | null): string {
  return value == null ? DASH : formatBytes(value);
}

/**
 * Browsing a queue.
 *
 * A button rather than a list that loads itself, and for a different reason
 * from the hosted families. A SEMP browse takes nothing at all: the queue's
 * depth, its spool usage and its delivery counters are identical afterwards,
 * and any number of readers can look at the same message. Polling would harm
 * nothing; it would just be a management API being read on behalf of a tab
 * nobody is looking at.
 *
 * What this page cannot show is the message. Every field SEMP carries is
 * metadata - an id, a spooled time, two sizes, a redelivery count and a few
 * flags - and there is no payload anywhere in the API. The broker's own
 * manager shows one by opening a browser flow over the messaging protocol,
 * which is a wire client this app deliberately does not have. So the size
 * column stands where every other family draws a preview, and the panel says
 * plainly why there is nothing under it rather than showing an empty box.
 *
 * What is still worth coming here for is the three columns beside it. A
 * message that has been tried and not acknowledged, one nothing has touched,
 * and one that will be discarded rather than dead-lettered when it is given up
 * on all look identical from the queues page, and each of them is a different
 * problem.
 */
export function MessagesSolace() {
  const { t } = useTranslation();
  const destinations = useSolaceDestinations();
  const browse = useSolaceBrowse();

  const queues = useMemo(
    () => (destinations.data ?? []).map(readDestination).map((entry) => entry.name),
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
        title={t("board.solace.messages.title")}
        subtitle={t("board.solace.messages.subtitle")}
      />
      <Toolbar>
        <div style={{ width: "260px", flex: "none" }}>
          <SelectField<string>
            value={chosen}
            options={queues.map((name) => ({ value: name, label: name }))}
            placeholder={t("board.solace.messages.pickQueue")}
            onValueChange={setQueue}
          />
        </div>
        <Button
          disabled={chosen === "" || browse.loading}
          onClick={() => void browse.run(chosen, BROWSE_LIMIT)}
        >
          {browse.loading && <Spinner className="size-3.5" />}
          {t("board.solace.messages.read")}
        </Button>
        <span className="flex-1" />
        <span style={{ fontSize: "11px", color: "var(--c-muted)" }}>
          {t("board.solace.messages.found", { count: browse.messages.length })}
        </span>
      </Toolbar>

      {/* Said before the button rather than after the disappointment. Nothing
          here is taken from anyone - which is the opposite of what the hosted
          families warn about - and nothing here shows a payload either. */}
      <div
        style={{
          padding: "6px 12px",
          fontSize: "11px",
          color: "var(--c-muted)",
          borderBottom: "1px solid var(--c-border)",
        }}
      >
        {t("board.solace.messages.readNote")}
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
                ? t("board.solace.messages.nothingThere")
                : t("board.solace.messages.startHere")}
            </div>
          ) : (
            <Table inset>
              <TableHeader>
                <TableRow>
                  <TableHead>{t("board.solace.messages.messageId")}</TableHead>
                  <TableHead>{t("board.solace.messages.spooled")}</TableHead>
                  <TableHead className="num">{t("board.solace.messages.size")}</TableHead>
                  <TableHead className="num">{t("board.solace.messages.redelivery")}</TableHead>
                  <TableHead>{t("board.solace.messages.state")}</TableHead>
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
                      {message.storeTime}
                    </TableCell>
                    <TableCell className="num mono3" style={MONO11}>
                      {size(attachmentSizeOf(message))}
                    </TableCell>
                    <TableCell className="num mono3" style={MONO11}>
                      {redeliveryCountOf(message) ?? DASH}
                    </TableCell>
                    <TableCell className="mono3" style={MONO11}>
                      {undelivered(message)
                        ? t("board.solace.messages.undelivered")
                        : t("board.solace.messages.delivered")}
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
                  [t("board.solace.messages.spooled"), panel.storeTime],
                  [t("board.solace.messages.attachment"), size(attachmentSizeOf(panel))],
                  [t("board.solace.messages.content"), size(contentSizeOf(panel))],
                  [t("board.solace.messages.redelivery"), String(redeliveryCountOf(panel) ?? 0)],
                  [
                    t("board.solace.messages.state"),
                    undelivered(panel)
                      ? t("board.solace.messages.undelivered")
                      : t("board.solace.messages.delivered"),
                  ],
                  [
                    t("board.solace.messages.dmqEligible"),
                    dmqEligible(panel)
                      ? t("board.solace.messages.dmqYes")
                      : t("board.solace.messages.dmqNo"),
                  ],
                  [t("board.solace.messages.partitionKey"), partitionKeyOf(panel) ?? DASH],
                  [t("board.solace.messages.publisher"), publisherIdOf(panel) ?? DASH],
                  [t("board.solace.messages.replicationId"), replicationIdOf(panel) ?? DASH],
                  [t("board.solace.messages.replicationState"), replicationStateOf(panel) ?? DASH],
                ]}
              />

              {otherProperties(panel).length > 0 && (
                <>
                  <SectionLabel>{t("board.solace.messages.section.properties")}</SectionLabel>
                  <KV rows={otherProperties(panel)} />
                </>
              )}

              <SectionLabel>{t("board.solace.messages.body")}</SectionLabel>
              {/* Not an empty JsonBlock, which would read as a message with no
                  content. There is no payload in SEMP at any version, and the
                  sizes above are what says whether one exists. */}
              <p style={{ margin: 0, fontSize: "11px", color: "var(--c-muted)" }}>
                {t("board.solace.messages.noBody")}
              </p>
            </DetailPanelBody>
          </DetailPanel>
        )}
      </ListArea>
    </Page>
  );
}
