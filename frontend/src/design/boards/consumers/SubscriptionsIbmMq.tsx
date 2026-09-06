import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { Page, PageBody, PageHeader, RefreshButton } from "@/design/shell";
import { Input } from "@/components/ui/input";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { KV, Panel, PanelHeader, SectionLabel } from "@/components";
import { BoardState } from "@/design/boards/BoardState";
import { useIbmMqSubscriptions } from "@/hooks/ibmmq/useIbmMqSubscriptions";
import { subscription as readSubscription, unread } from "@/mq/ibmmq/subscriptions";
import { formatCount } from "@/lib/format";

const MONO11 = { fontSize: "11px" } as const;
const DASH = "—";

function count(value: number | null): string {
  return value == null ? DASH : formatCount(value);
}

/**
 * The queue manager's subscriptions.
 *
 * A subscription registers interest in a topic string and names a queue to
 * deliver to, so this page carries two numbers that are easy to confuse and
 * both worth having: how many publications the subscription has ever received,
 * which is a lifetime total the queue manager keeps, and how many are still
 * waiting, which is the depth of the queue it delivers to.
 *
 * "Attached" is the column that reads wrong if it is read as "online". An
 * administrative subscription - one an operator defined - has nothing attached
 * by design: the publications land on its destination queue and whichever
 * application reads that queue is the consumer. So the row is flagged when its
 * queue is filling up with nobody at the other end, not when nothing is
 * connected.
 *
 * There is no create and no delete. The commands exist and this driver does
 * not offer them: a subscription's identity is a topic string, a destination
 * queue and a durability together, which needs a form rather than a name
 * field.
 */
export function SubscriptionsIbmMq() {
  const { t } = useTranslation();
  const state = useIbmMqSubscriptions();
  const [search, setSearch] = useState("");
  const [selected, setSelected] = useState<string | null>(null);

  const subscriptions = useMemo(
    () => (state.data ?? []).map(readSubscription),
    [state.data],
  );

  const shown = useMemo(() => {
    const needle = search.trim().toLowerCase();
    return subscriptions.filter(
      (entry) =>
        needle === "" ||
        entry.name.toLowerCase().includes(needle) ||
        (entry.topicString ?? "").toLowerCase().includes(needle),
    );
  }, [subscriptions, search]);

  const detail = useMemo(
    () => shown.find((entry) => entry.name === selected) ?? shown[0] ?? null,
    [shown, selected],
  );

  return (
    <Page>
      <PageHeader
        title={t("shell.nav.ibmmq.consumers")}
        subtitle={t("board.ibmmq.subscriptions.count", { count: subscriptions.length })}
        actions={
          <div style={{ display: "flex", gap: "8px", alignItems: "center" }}>
            <Input
              style={{ width: "220px" }}
              value={search}
              placeholder={t("board.ibmmq.subscriptions.search")}
              onChange={(event) => setSearch(event.target.value)}
            />
            <RefreshButton
              refreshing={state.refreshing}
              online={state.online}
              onClick={() => void state.refresh()}
            />
          </div>
        }
      />
      <BoardState state={state}>
        <PageBody>
          <div style={{ display: "flex", gap: "12px", minHeight: 0, flex: 1 }}>
            <Panel style={{ flex: 1, overflow: "auto" }}>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t("board.ibmmq.subscriptions.name")}</TableHead>
                    <TableHead>{t("board.ibmmq.subscriptions.topicString")}</TableHead>
                    <TableHead>{t("board.ibmmq.subscriptions.destination")}</TableHead>
                    <TableHead className="num">{t("board.ibmmq.subscriptions.waiting")}</TableHead>
                    <TableHead className="num">
                      {t("board.ibmmq.subscriptions.received")}
                    </TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {shown.map((entry) => (
                    <TableRow
                      key={entry.name}
                      data-state={detail?.name === entry.name ? "selected" : undefined}
                      onClick={() => setSelected(entry.name)}
                      style={{ cursor: "pointer" }}
                    >
                      <TableCell>
                        <span className="mono3" style={MONO11}>
                          {entry.name}
                        </span>
                      </TableCell>
                      <TableCell>
                        <span className="mono3" style={MONO11}>
                          {entry.topicString || DASH}
                        </span>
                      </TableCell>
                      <TableCell>
                        <span className="mono3" style={MONO11}>
                          {entry.destination || DASH}
                        </span>
                      </TableCell>
                      <TableCell className="num">
                        <span
                          className="mono3"
                          style={{
                            ...MONO11,
                            color: unread(entry) ? "var(--c-warn)" : undefined,
                          }}
                        >
                          {count(entry.backlog)}
                        </span>
                      </TableCell>
                      <TableCell className="num">
                        <span className="mono3" style={MONO11}>
                          {count(entry.messagesReceived)}
                        </span>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </Panel>

            {detail != null && (
              <Panel style={{ width: "320px", overflow: "auto" }}>
                <PanelHeader title={detail.name} />
                <div style={{ padding: "10px 12px", display: "grid", gap: "8px" }}>
                  <SectionLabel>{t("board.ibmmq.subscriptions.section.definition")}</SectionLabel>
                  <KV
                    rows={[
                      [t("board.ibmmq.subscriptions.topicString"), detail.topicString || DASH],
                      [t("board.ibmmq.subscriptions.destination"), detail.destination || DASH],
                      [
                        t("board.ibmmq.subscriptions.destinationQmgr"),
                        detail.destinationQueueManager || DASH,
                      ],
                      [
                        t("board.ibmmq.subscriptions.kind"),
                        detail.kind == null
                          ? DASH
                          : t(`board.ibmmq.subscriptions.kindValue.${detail.kind}`, {
                              defaultValue: detail.kind,
                            }),
                      ],
                      [
                        t("board.ibmmq.subscriptions.durable"),
                        detail.durable
                          ? t("board.ibmmq.subscriptions.durableYes")
                          : t("board.ibmmq.subscriptions.durableNo"),
                      ],
                      [t("board.ibmmq.subscriptions.user"), detail.user || DASH],
                      [t("board.ibmmq.subscriptions.selector"), detail.selector || DASH],
                      [t("board.ibmmq.subscriptions.id"), detail.subscriptionId || DASH],
                    ]}
                  />

                  <SectionLabel>{t("board.ibmmq.subscriptions.section.runtime")}</SectionLabel>
                  <KV
                    rows={[
                      [t("board.ibmmq.subscriptions.received"), count(detail.messagesReceived)],
                      [t("board.ibmmq.subscriptions.waiting"), count(detail.backlog)],
                      [t("board.ibmmq.subscriptions.queueReaders"), count(detail.queueReaders)],
                      [
                        t("board.ibmmq.subscriptions.attached"),
                        detail.attached
                          ? t("board.ibmmq.subscriptions.attachedYes")
                          : t("board.ibmmq.subscriptions.attachedNo"),
                      ],
                      [t("board.ibmmq.subscriptions.lastMessage"), detail.lastMessageAt || DASH],
                    ]}
                  />

                  {unread(detail) && (
                    <div style={{ fontSize: "11px", color: "var(--c-warn)" }}>
                      {t("board.ibmmq.subscriptions.unread", {
                        queue: detail.destination ?? "",
                      })}
                    </div>
                  )}
                </div>
              </Panel>
            )}
          </div>
          <div style={{ fontSize: "11px", color: "var(--c-muted)", paddingTop: "8px" }}>
            {t("board.ibmmq.subscriptions.note")}
          </div>
        </PageBody>
      </BoardState>
    </Page>
  );
}
