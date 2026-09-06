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
import { useIbmMqChannels } from "@/hooks/ibmmq/useIbmMqChannels";
import { group, metric, statusExpected, unhealthy } from "@/mq/ibmmq/channels";
import { formatCount } from "@/lib/format";
import type { Channel } from "@bindings/model/models";

const MONO11 = { fontSize: "11px" } as const;

/** A dash, not a zero: an absent figure and a measured zero are not alike. */
const DASH = "—";

function count(value: number | null): string {
  return value == null ? DASH : formatCount(value);
}

/**
 * The channels a queue manager defines.
 *
 * This is not the clients page and could not be. A channel is a definition an
 * administrator made: it is here whether or not anything is using it, it is
 * what decides whether a connection is allowed at all, and one definition
 * carries one running instance per connected application. A page built from
 * connections would be empty on a queue manager whose applications are idle,
 * which is exactly when somebody is looking for why they cannot connect.
 *
 * An empty status is a real answer and is not coloured. A channel nobody has
 * started has none, and a client-connection definition never has one - this
 * queue manager holds it on behalf of clients and does not run it. Painting
 * either red would paint most of a fresh queue manager red.
 *
 * The counters are one running instance's totals since it started rather than
 * the definition's lifetime, and they reset when a channel restarts. That is
 * worth knowing before reading a zero as "never used", which is why a channel
 * with nothing running shows a dash instead.
 */
export function ChannelsIbmMq() {
  const { t } = useTranslation();
  const state = useIbmMqChannels();
  const [search, setSearch] = useState("");
  const [selected, setSelected] = useState<string | null>(null);

  const channels = useMemo(() => state.data ?? [], [state.data]);

  const shown = useMemo(() => {
    const needle = search.trim().toLowerCase();
    return channels.filter(
      (entry) =>
        needle === "" ||
        entry.name.toLowerCase().includes(needle) ||
        entry.connectionName.toLowerCase().includes(needle),
    );
  }, [channels, search]);

  const detail = useMemo(
    () => shown.find((entry) => entry.name === selected) ?? shown[0] ?? null,
    [shown, selected],
  );

  const statusOf = (entry: Channel) =>
    entry.status !== ""
      ? entry.status
      : statusExpected(entry)
        ? t("board.ibmmq.channels.neverStarted")
        : t("board.ibmmq.channels.notRunHere");

  return (
    <Page>
      <PageHeader
        title={t("shell.nav.ibmmq.channels")}
        subtitle={t("board.ibmmq.channels.count", { count: channels.length })}
        actions={
          <div style={{ display: "flex", gap: "8px", alignItems: "center" }}>
            <Input
              style={{ width: "200px" }}
              value={search}
              placeholder={t("board.ibmmq.channels.search")}
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
                    <TableHead>{t("board.ibmmq.channels.name")}</TableHead>
                    <TableHead>{t("board.ibmmq.channels.type")}</TableHead>
                    <TableHead>{t("board.ibmmq.channels.status")}</TableHead>
                    <TableHead className="num">{t("board.ibmmq.channels.instances")}</TableHead>
                    <TableHead className="num">{t("board.ibmmq.channels.messages")}</TableHead>
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
                        {entry.inDoubt && (
                          <span className="pb pIMQ" style={{ marginLeft: "6px" }}>
                            {t("board.ibmmq.channels.inDoubtBadge")}
                          </span>
                        )}
                      </TableCell>
                      <TableCell>
                        <span className="mono3" style={MONO11}>
                          {t(`board.ibmmq.channels.group.${group(entry)}`)}
                        </span>
                      </TableCell>
                      <TableCell>
                        <span
                          className="mono3"
                          style={{
                            ...MONO11,
                            color: unhealthy(entry) ? "var(--c-warn)" : undefined,
                          }}
                        >
                          {statusOf(entry)}
                        </span>
                      </TableCell>
                      <TableCell className="num">
                        <span className="mono3" style={MONO11}>
                          {entry.instances}
                        </span>
                      </TableCell>
                      <TableCell className="num">
                        <span className="mono3" style={MONO11}>
                          {count(metric(entry.messages))}
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
                  <SectionLabel>{t("board.ibmmq.channels.section.definition")}</SectionLabel>
                  <KV
                    rows={[
                      [t("board.ibmmq.channels.type"), detail.type],
                      [t("board.ibmmq.channels.connectionName"), detail.connectionName || DASH],
                      [
                        t("board.ibmmq.channels.transmissionQueue"),
                        detail.transmissionQueue || DASH,
                      ],
                      [t("board.ibmmq.channels.description"), detail.description || DASH],
                    ]}
                  />

                  <SectionLabel>{t("board.ibmmq.channels.section.runtime")}</SectionLabel>
                  <KV
                    rows={[
                      [t("board.ibmmq.channels.status"), statusOf(detail)],
                      [t("board.ibmmq.channels.substate"), detail.substate || DASH],
                      [t("board.ibmmq.channels.instances"), String(detail.instances)],
                      [
                        t("board.ibmmq.channels.remoteQueueManager"),
                        detail.remoteQueueManager || DASH,
                      ],
                      [t("board.ibmmq.channels.startedAt"), detail.startedAt || DASH],
                      [t("board.ibmmq.channels.lastMessageAt"), detail.lastMessageAt || DASH],
                      [t("board.ibmmq.channels.messages"), count(metric(detail.messages))],
                      [t("board.ibmmq.channels.bytesSent"), count(metric(detail.bytesSent))],
                      [
                        t("board.ibmmq.channels.bytesReceived"),
                        count(metric(detail.bytesReceived)),
                      ],
                    ]}
                  />

                  {detail.inDoubt && (
                    <div style={{ fontSize: "11px", color: "var(--c-warn)" }}>
                      {t("board.ibmmq.channels.inDoubtNote")}
                    </div>
                  )}
                  {detail.stopRequested && (
                    <div style={{ fontSize: "11px", color: "var(--c-warn)" }}>
                      {t("board.ibmmq.channels.stopRequestedNote")}
                    </div>
                  )}
                </div>
              </Panel>
            )}
          </div>
          <div style={{ fontSize: "11px", color: "var(--c-muted)", paddingTop: "8px" }}>
            {t("board.ibmmq.channels.note")}
          </div>
        </PageBody>
      </BoardState>
    </Page>
  );
}
