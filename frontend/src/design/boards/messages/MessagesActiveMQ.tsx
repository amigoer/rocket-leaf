import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { ListArea, ListPane, Page, PageHeader, Toolbar } from "@/design/shell";
import { Button } from "@/components/ui/button";
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
} from "@/components";
import { BoardState } from "@/design/boards/BoardState";
import { useActiveMQMessages } from "@/hooks/activemq/useActiveMQMessages";
import { useActiveMQDestinations } from "@/hooks/activemq/useActiveMQDestinations";
import { useActiveMQSubscriptions } from "@/hooks/activemq/useActiveMQSubscriptions";
import {
  message as readMessage,
  summary,
  type ActiveMQMessage,
} from "@/mq/activemq/messages";
import {
  browseWillBeCapped,
  destination as readDestination,
} from "@/mq/activemq/destinations";
import { subscription as readSubscription } from "@/mq/activemq/subscriptions";
import { copyText } from "@/api/platform";
import { toast } from "sonner";

const MONO11 = { fontSize: "11px" } as const;
const DASH = "—";

/**
 * ActiveMQ messages.
 *
 * Browsing here is a management operation on both products, so opening this
 * page takes nothing off the destination and puts nothing back. That is worth
 * knowing because it is not how every family works - RabbitMQ's equivalent
 * goes through basic.get and alters the queue even when what it read is
 * requeued, and its page says so.
 *
 * What this page does have to say is Classic's limit. browse() stops at
 * maxBrowsePageSize - 400 by default - however deep the destination is, and
 * the limit is not readable over JMX, so a deep queue quietly returns a short
 * page. The notice appears only when the chosen destination is actually deeper
 * than that.
 *
 * An Artemis topic is not offered. A multicast address holds nothing itself:
 * the messages are in the subscription queues under it, and which one to read
 * is a choice, so those are listed instead.
 */
export function MessagesActiveMQ() {
  const { t } = useTranslation();
  const destinationState = useActiveMQDestinations();
  const subscriptionState = useActiveMQSubscriptions();
  const [target, setTarget] = useState<string>("");
  const [limit, setLimit] = useState(200);
  const [selected, setSelected] = useState<string | null>(null);

  const destinations = useMemo(
    () => (destinationState.data ?? []).map(readDestination),
    [destinationState.data],
  );
  const subscriptions = useMemo(
    () => (subscriptionState.data ?? []).map(readSubscription),
    [subscriptionState.data],
  );

  // What can actually be browsed. Every queue, plus - on Artemis - each
  // subscription queue under a topic, because that is where a topic's
  // messages really are. A Classic topic is browsable directly.
  const options = useMemo(() => {
    const product = destinations[0]?.product ?? "classic";
    const queues = destinations
      .filter((entry) => entry.kind === "queue" || product === "classic")
      .map((entry) => ({ value: entry.name, label: entry.name }));
    if (product !== "artemis") return queues;
    return [
      ...queues,
      ...subscriptions.map((entry) => ({
        value: entry.name,
        label: `${entry.topic} › ${entry.subscriptionName ?? entry.name}`,
      })),
    ];
  }, [destinations, subscriptions]);

  const chosen = target !== "" ? target : (options[0]?.value ?? null);
  const state = useActiveMQMessages(chosen, limit);

  const messages = useMemo(() => (state.data ?? []).map(readMessage), [state.data]);
  const detail = useMemo(
    () => messages.find((entry) => entry.id === selected) ?? messages[0] ?? null,
    [messages, selected],
  );

  // Only when it matters: a shallow queue on Classic has nothing to warn about.
  const capped = useMemo(() => {
    const entry = destinations.find((option) => option.name === chosen);
    return entry != null && browseWillBeCapped(entry);
  }, [destinations, chosen]);

  const copy = async (entry: ActiveMQMessage) => {
    await copyText(entry.body);
    toast.success(t("board.activemq.messages.copied"));
  };

  return (
    <Page>
      <PageHeader
        title={t("shell.nav.activemq.messages")}
        subtitle={t("board.activemq.messages.count", { count: messages.length })}
      />
      <Toolbar>
        <SelectField
          value={chosen ?? ""}
          options={options}
          onValueChange={(next) => {
            setTarget(next);
            setSelected(null);
          }}
        />
        <SelectField<string>
          value={String(limit)}
          options={[
            { value: "50", label: "50" },
            { value: "200", label: "200" },
            { value: "500", label: "500" },
          ]}
          onValueChange={(next) => setLimit(Number(next))}
        />
        <Button
          size="sm"
          variant="outline"
          disabled={state.refreshing}
          onClick={() => void state.refresh()}
        >
          {t("board.activemq.messages.browse")}
        </Button>
      </Toolbar>
      {capped && (
        <p
          style={{
            margin: "0 20px",
            fontSize: "11.5px",
            color: "var(--c-muted)",
            flex: "none",
          }}
        >
          {t("board.activemq.messages.capped")}
        </p>
      )}
      <BoardState state={state}>
        <ListArea>
          <ListPane>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t("board.activemq.messages.id")}</TableHead>
                  <TableHead>{t("board.activemq.messages.body")}</TableHead>
                  <TableHead>{t("board.activemq.messages.time")}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {messages.map((entry) => (
                  <TableRow
                    key={entry.id}
                    data-state={detail?.id === entry.id ? "selected" : undefined}
                    onClick={() => setSelected(entry.id)}
                    style={{ cursor: "pointer" }}
                  >
                    <TableCell>
                      <span className="mono3" style={MONO11}>
                        {entry.id}
                      </span>
                    </TableCell>
                    <TableCell>
                      <span style={MONO11}>
                        {entry.truncated
                          ? t("board.activemq.messages.truncated")
                          : summary(entry)}
                      </span>
                    </TableCell>
                    <TableCell>
                      <span className="mono3" style={MONO11}>
                        {entry.storeTime === "" ? DASH : entry.storeTime}
                      </span>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </ListPane>

          {detail != null && (
            <DetailPanel>
              <DetailPanelHeader title={detail.id} />
              <DetailPanelBody>
                <SectionLabel>{t("board.activemq.messages.section.headers")}</SectionLabel>
                <KV
                  rows={[
                    [t("board.activemq.messages.time"), detail.storeTime || DASH],
                    [
                      t("board.activemq.messages.priority"),
                      detail.priority == null ? DASH : String(detail.priority),
                    ],
                    [
                      t("board.activemq.messages.persistent"),
                      detail.persistent == null
                        ? DASH
                        : detail.persistent
                          ? t("common.yes")
                          : t("common.no"),
                    ],
                    [
                      t("board.activemq.messages.redelivered"),
                      detail.redelivered == null
                        ? DASH
                        : detail.redelivered
                          ? t("common.yes")
                          : t("common.no"),
                    ],
                    [t("board.activemq.messages.correlationId"), detail.correlationId ?? DASH],
                    [t("board.activemq.messages.replyTo"), detail.replyTo ?? DASH],
                    [t("board.activemq.messages.jmsType"), detail.jmsType ?? DASH],
                    [t("board.activemq.messages.groupId"), detail.groupId ?? DASH],
                    // Artemis reports which wire protocol a message arrived
                    // on, which Classic does not - so this row is blank on
                    // half the connections rather than absent.
                    [t("board.activemq.messages.protocol"), detail.protocol ?? DASH],
                  ]}
                />

                <SectionLabel>{t("board.activemq.messages.section.properties")}</SectionLabel>
                {Object.keys(detail.properties).length === 0 ? (
                  <p style={{ fontSize: "11px", color: "var(--c-muted)" }}>
                    {t("board.activemq.messages.noProperties")}
                  </p>
                ) : (
                  <KV rows={Object.entries(detail.properties)} />
                )}

                <SectionLabel>{t("board.activemq.messages.section.body")}</SectionLabel>
                {detail.truncated ? (
                  <p style={{ fontSize: "11px", color: "var(--c-muted)" }}>
                    {t("board.activemq.messages.largeBody")}
                  </p>
                ) : (
                  <pre
                    className="mono3"
                    style={{
                      ...MONO11,
                      whiteSpace: "pre-wrap",
                      wordBreak: "break-all",
                      margin: 0,
                    }}
                  >
                    {detail.body}
                  </pre>
                )}
                <Button
                  size="sm"
                  variant="outline"
                  style={{ marginTop: "8px" }}
                  disabled={detail.truncated}
                  onClick={() => void copy(detail)}
                >
                  {t("board.activemq.messages.copy")}
                </Button>
              </DetailPanelBody>
            </DetailPanel>
          )}
        </ListArea>
      </BoardState>
    </Page>
  );
}
