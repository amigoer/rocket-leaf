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
import { useSolaceClients } from "@/hooks/solace/useSolaceClients";
import { applications, client as readClient } from "@/mq/solace/clients";
import { formatBytes, formatCount } from "@/lib/format";

const MONO11 = { fontSize: "11px" } as const;
const DASH = "—";

function duration(seconds: number | null): string {
  if (seconds == null) return DASH;
  if (seconds < 60) return `${seconds}s`;
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m`;
  if (seconds < 86400) return `${Math.floor(seconds / 3600)}h`;
  return `${Math.floor(seconds / 86400)}d`;
}

/**
 * Who is connected to this Message VPN.
 *
 * The broker's own sessions are in the list and marked rather than filtered
 * out. They are real connections holding real resources - the internal message
 * bus, the REST listener's own session - and a page that hid them would
 * disagree with every count the broker reports. The header says how many of
 * the rows are applications, which is the number somebody actually came for.
 *
 * There is no protocol column, and its absence is the API rather than an
 * omission: SEMP reports nothing on a client that says whether it arrived over
 * SMF, MQTT, AMQP or REST. What it does report is the platform string the
 * client library sent, which is on the panel and is the closest thing to an
 * answer.
 *
 * The binds column is not a channel count. A Solace flow is per endpoint the
 * client is bound to rather than per session, it has no prefetch of its own,
 * and it is not something an operator closes - so there is no channels page
 * here and this column says "binds" instead.
 */
export function ClientsSolace() {
  const { t } = useTranslation();
  const state = useSolaceClients();
  const [search, setSearch] = useState("");
  const [selected, setSelected] = useState<string | null>(null);

  const clients = useMemo(() => (state.data ?? []).map(readClient), [state.data]);
  const shown = useMemo(() => {
    const needle = search.trim().toLowerCase();
    if (needle === "") return clients;
    return clients.filter(
      (entry) =>
        entry.name.toLowerCase().includes(needle) ||
        entry.address.toLowerCase().includes(needle) ||
        entry.username.toLowerCase().includes(needle),
    );
  }, [clients, search]);

  const detail = useMemo(
    () => shown.find((entry) => entry.name === selected) ?? shown[0] ?? null,
    [shown, selected],
  );

  return (
    <Page>
      <PageHeader
        title={t("shell.nav.solace.clients")}
        subtitle={t("board.solace.clients.count", {
          applications: applications(clients).length,
          total: clients.length,
        })}
        actions={
          <div style={{ display: "flex", gap: "8px", alignItems: "center" }}>
            <Input
              style={{ width: "200px" }}
              value={search}
              placeholder={t("board.solace.clients.search")}
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
                    <TableHead>{t("board.solace.clients.name")}</TableHead>
                    <TableHead>{t("board.solace.clients.address")}</TableHead>
                    <TableHead>{t("board.solace.clients.username")}</TableHead>
                    <TableHead className="num">{t("board.solace.clients.binds")}</TableHead>
                    <TableHead className="num">{t("board.solace.clients.uptime")}</TableHead>
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
                        <span
                          className="mono3"
                          style={{
                            ...MONO11,
                            color: entry.slowSubscriber ? "var(--c-warn)" : undefined,
                          }}
                        >
                          {entry.name}
                        </span>
                        {entry.internal && (
                          <span className="pb pSOL" style={{ marginLeft: "6px" }}>
                            {t("board.solace.clients.internalBadge")}
                          </span>
                        )}
                      </TableCell>
                      <TableCell className="mono3" style={MONO11}>
                        {entry.address}
                      </TableCell>
                      <TableCell className="mono3" style={MONO11}>
                        {entry.username || DASH}
                      </TableCell>
                      <TableCell className="num mono3" style={MONO11}>
                        {formatCount(entry.binds)}
                      </TableCell>
                      <TableCell className="num mono3" style={MONO11}>
                        {duration(entry.uptimeSec)}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </Panel>

            {detail != null && (
              <Panel style={{ width: "340px", overflow: "auto" }}>
                <PanelHeader title={detail.name} />
                <div style={{ padding: "10px 12px", display: "grid", gap: "8px" }}>
                  <KV
                    rows={[
                      [t("board.solace.clients.msgVpn"), detail.msgVpn || DASH],
                      [t("board.solace.clients.address"), detail.address],
                      [t("board.solace.clients.username"), detail.username || DASH],
                      [t("board.solace.clients.uptime"), duration(detail.uptimeSec)],
                      [t("board.solace.clients.binds"), formatCount(detail.binds)],
                    ]}
                  />

                  <SectionLabel>{t("board.solace.clients.section.library")}</SectionLabel>
                  <KV
                    rows={[
                      [t("board.solace.clients.platform"), detail.platform ?? DASH],
                      [t("board.solace.clients.softwareVersion"), detail.softwareVersion ?? DASH],
                      [t("board.solace.clients.description"), detail.description ?? DASH],
                    ]}
                  />

                  <SectionLabel>{t("board.solace.clients.section.limits")}</SectionLabel>
                  <KV
                    rows={[
                      [t("board.solace.clients.clientProfile"), detail.clientProfile ?? DASH],
                      [t("board.solace.clients.aclProfile"), detail.aclProfile ?? DASH],
                      [t("board.solace.clients.recv"), formatBytes(detail.recvBytes)],
                      [t("board.solace.clients.send"), formatBytes(detail.sendBytes)],
                    ]}
                  />

                  {detail.slowSubscriber && (
                    <div style={{ fontSize: "11px", color: "var(--c-warn)" }}>
                      {t("board.solace.clients.slowNote")}
                    </div>
                  )}
                  {detail.tlsDowngraded && (
                    <div style={{ fontSize: "11px", color: "var(--c-warn)" }}>
                      {t("board.solace.clients.downgradedNote")}
                    </div>
                  )}
                </div>
              </Panel>
            )}
          </div>
          <div style={{ fontSize: "11px", color: "var(--c-muted)", paddingTop: "8px" }}>
            {t("board.solace.clients.note")}
          </div>
        </PageBody>
      </BoardState>
    </Page>
  );
}
