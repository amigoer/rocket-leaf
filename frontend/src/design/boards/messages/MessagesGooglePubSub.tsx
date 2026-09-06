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
import { useGooglePubSubBrowse } from "@/hooks/googlepubsub/useGooglePubSubBrowse";
import { useGooglePubSubSubscriptions } from "@/hooks/googlepubsub/useGooglePubSubSubscriptions";
import { subscription as readSubscription } from "@/mq/googlepubsub/subscriptions";
import {
  deliveryAttempt,
  messageIdOf,
  orderingKey,
  publishedAtMs,
  publisherAttributes,
  readFrom,
} from "@/mq/googlepubsub/messages";
import { formatMessageTime } from "@/lib/time";
import type { MessageItem } from "@bindings/model/models";

const MONO11 = { fontSize: "11px" } as const;
const RIGHT = { textAlign: "right" } as const;
const DASH = "—";

/** How many messages one browse asks for. */
const BROWSE_LIMIT = 20;

function preview(message: MessageItem): string {
  const body = message.body ?? "";
  return body.length > 120 ? `${body.slice(0, 120)}…` : body;
}

/**
 * Browsing a Google Pub/Sub subscription.
 *
 * The picker offers subscriptions rather than topics, which is the family
 * rather than a preference: a topic holds nothing, so there is no such thing
 * as browsing one. Two subscriptions on the same topic hold different
 * messages, and there is no third place with all of them.
 *
 * A button rather than a list that loads itself, and that is not a style
 * choice either. Pull is the only read Pub/Sub has and it is the same call a
 * consumer makes: what comes back is held away from everyone else until the
 * driver hands it straight back, and each message's delivery attempt goes up
 * for good - which counts towards being dead-lettered. A page that fetched on
 * mount and refreshed on a timer would be taking messages away from a live
 * consumer for nobody's benefit.
 *
 * The banner says so. The driver declares the same thing as a caveat on the
 * capability, which is what the sidebar shows; this is where somebody about to
 * press the button reads it.
 */
export function MessagesGooglePubSub() {
  const { t } = useTranslation();
  const subscriptions = useGooglePubSubSubscriptions();
  const browse = useGooglePubSubBrowse();

  /* Only pull subscriptions. A push, BigQuery, Cloud Storage or Bigtable one
     is written straight through by the service and has no backlog a Pull could
     reach - offering it would be a picker entry that always answers empty. */
  const names = useMemo(
    () =>
      (subscriptions.data ?? [])
        .map(readSubscription)
        .filter((entry) => entry.delivery === "pull")
        .map((entry) => entry.name),
    [subscriptions.data],
  );

  const [chosenName, setChosenName] = useState("");
  const [selected, setSelected] = useState<string | null>(null);

  const chosen = chosenName !== "" ? chosenName : (names[0] ?? "");
  const panel = useMemo(
    () => browse.messages.find((message) => messageIdOf(message) === selected) ?? null,
    [browse.messages, selected],
  );

  return (
    <Page>
      <PageHeader
        title={t("board.google-pubsub.messages.title")}
        subtitle={t("board.google-pubsub.messages.subtitle")}
      />
      <Toolbar>
        <div style={{ width: "280px", flex: "none" }}>
          <SelectField<string>
            value={chosen}
            options={names.map((name) => ({ value: name, label: name }))}
            placeholder={t("board.google-pubsub.messages.pickSubscription")}
            onValueChange={setChosenName}
          />
        </div>
        <Button
          disabled={chosen === "" || browse.loading}
          onClick={() => void browse.run(chosen, BROWSE_LIMIT)}
        >
          {browse.loading && <Spinner className="size-3.5" />}
          {t("board.google-pubsub.messages.pull")}
        </Button>
        <span className="flex-1" />
        <span style={{ fontSize: "11px", color: "var(--c-muted)" }}>
          {t("board.google-pubsub.messages.found", { count: browse.messages.length })}
        </span>
      </Toolbar>

      {/* Said before the button rather than after it. Everything on this page
          is a real delivery: the messages were held away from live consumers
          while it ran, and their delivery attempts do not come back down. */}
      <div
        style={{
          padding: "6px 12px",
          fontSize: "11px",
          color: "var(--c-warn-text)",
          borderBottom: "1px solid var(--c-border)",
        }}
      >
        {t("board.google-pubsub.messages.pullNote")}
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
                ? t("board.google-pubsub.messages.nothingHeld")
                : t("board.google-pubsub.messages.startHere")}
            </div>
          ) : (
            <Table inset>
              <TableHeader>
                <TableRow>
                  <TableHead>{t("board.google-pubsub.messages.messageId")}</TableHead>
                  <TableHead>{t("board.google-pubsub.messages.published")}</TableHead>
                  <TableHead style={RIGHT}>
                    {t("board.google-pubsub.messages.deliveryAttempt")}
                  </TableHead>
                  <TableHead>{t("board.google-pubsub.messages.body")}</TableHead>
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
                      {formatMessageTime(publishedAtMs(message))}
                    </TableCell>
                    <TableCell className="mono3" style={RIGHT}>
                      {deliveryAttempt(message) == null
                        ? DASH
                        : String(deliveryAttempt(message))}
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
                  {readFrom(panel)}
                </Status>
              }
              onClose={() => setSelected(null)}
            />
            <DetailPanelBody>
              <KV
                rows={[
                  [
                    t("board.google-pubsub.messages.published"),
                    formatMessageTime(publishedAtMs(panel)),
                  ],
                  [
                    t("board.google-pubsub.messages.deliveryAttempt"),
                    deliveryAttempt(panel) == null ? DASH : String(deliveryAttempt(panel)),
                  ],
                  [t("board.google-pubsub.messages.orderingKey"), orderingKey(panel) ?? DASH],
                  [t("board.google-pubsub.messages.subscription"), readFrom(panel)],
                ]}
              />

              {/* The delivery attempt is only tracked on a subscription with a
                  dead-letter policy, so its absence says something about the
                  subscription rather than about the message. */}
              {deliveryAttempt(panel) == null && (
                <p style={{ fontSize: "11px", color: "var(--c-muted)" }}>
                  {t("board.google-pubsub.messages.noAttemptCount")}
                </p>
              )}

              {publisherAttributes(panel).length > 0 && (
                <>
                  <SectionLabel>{t("board.google-pubsub.messages.section.attributes")}</SectionLabel>
                  <KV rows={publisherAttributes(panel)} />
                </>
              )}

              <SectionLabel>{t("board.google-pubsub.messages.body")}</SectionLabel>
              <JsonBlock>{panel.body}</JsonBlock>
            </DetailPanelBody>
          </DetailPanel>
        )}
      </ListArea>
    </Page>
  );
}
