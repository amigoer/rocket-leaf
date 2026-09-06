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
import { KV, Panel, PanelHeader, SectionLabel, Status } from "@/components";
import { BoardState } from "@/design/boards/BoardState";
import { useNsqConnections } from "@/hooks/nsq/useNsqCluster";
import { askingForNothing, client as readClient, type NsqClient } from "@/mq/nsq/clients";
import { formatCount } from "@/lib/format";

const MONO11 = { fontSize: "11px" } as const;

/** A dash, not a zero: an absent figure and a measured zero are not alike. */
const DASH = "—";

function count(value: number | null): string {
  return value == null ? DASH : formatCount(value);
}

/**
 * Who is connected, in the two roles nsqd reports them in.
 *
 * A consumer and a producer are found in different places and carry different
 * figures, so the role is a column rather than a detail: a consumer has a
 * channel and a ready count, a producer has neither and instead reports what
 * it has published per topic. A producer with no ready count is not a stalled
 * consumer, and the page must not read like it is.
 *
 * The ready count is still the column that earns the page. It is what a
 * consumer told nsqd it will accept, and a zero means it is connected, holding
 * its channel, and asking for nothing - a backlog that will not move however
 * healthy every other figure looks. Nothing else in this app can see that.
 *
 * One client process shows up once per daemon it found, which is not
 * duplication: those are separate connections holding separate messages.
 *
 * Anything publishing over HTTP is absent, and no page can fix it: /pub is a
 * request rather than a connection, so nsqd has nothing left to list once it
 * has answered.
 */
export function ClientsNsq() {
  const { t } = useTranslation();
  const state = useNsqConnections();
  const [search, setSearch] = useState("");
  const [selected, setSelected] = useState<string | null>(null);

  const clients = useMemo(() => (state.data ?? []).map(readClient), [state.data]);

  const shown = useMemo(() => {
    const needle = search.trim().toLowerCase();
    if (needle === "") return clients;
    return clients.filter((entry) =>
      [entry.clientId, entry.peer, entry.topic, entry.channel, entry.node].some((field) =>
        field.toLowerCase().includes(needle),
      ),
    );
  }, [clients, search]);

  const detail = useMemo(
    () => shown.find((entry) => entry.id === selected) ?? shown[0] ?? null,
    [shown, selected],
  );

  return (
    <Page>
      <PageHeader
        title={t("shell.nav.nsq.clients")}
        subtitle={t("board.nsq.clients.count", { count: clients.length })}
        actions={
          <div style={{ display: "flex", gap: "8px", alignItems: "center" }}>
            <Input
              style={{ width: "200px" }}
              value={search}
              placeholder={t("board.nsq.clients.search")}
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
                    <TableHead>{t("board.nsq.clients.peer")}</TableHead>
                    <TableHead>{t("board.nsq.clients.roleColumn")}</TableHead>
                    <TableHead>{t("board.nsq.clients.topic")}</TableHead>
                    <TableHead>{t("board.nsq.clients.channel")}</TableHead>
                    <TableHead className="num">{t("board.nsq.clients.ready")}</TableHead>
                    <TableHead className="num">{t("board.nsq.clients.inFlight")}</TableHead>
                    <TableHead className="num">{t("board.nsq.clients.finished")}</TableHead>
                    <TableHead>{t("board.nsq.clients.node")}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {shown.map((entry) => (
                    <TableRow
                      key={entry.id}
                      data-state={detail?.id === entry.id ? "selected" : undefined}
                      onClick={() => setSelected(entry.id)}
                      style={{ cursor: "pointer" }}
                    >
                      <TableCell>
                        <span className="mono3" style={MONO11}>
                          {entry.peer}
                        </span>
                        {askingForNothing(entry) && (
                          <Status tone="warn" style={{ fontSize: "10px", marginLeft: "6px" }}>
                            {t("board.nsq.clients.notReadyBadge")}
                          </Status>
                        )}
                      </TableCell>
                      <TableCell style={MONO11}>
                        {t(`board.nsq.clients.role.${entry.role}`)}
                      </TableCell>
                      <TableCell className="mono3" style={MONO11}>
                        {entry.topic || DASH}
                      </TableCell>
                      <TableCell className="mono3" style={MONO11}>
                        {entry.channel || DASH}
                      </TableCell>
                      <TableCell className="num mono3" style={MONO11}>
                        {count(entry.ready)}
                      </TableCell>
                      <TableCell className="num mono3" style={MONO11}>
                        {count(entry.inFlight)}
                      </TableCell>
                      <TableCell className="num mono3" style={MONO11}>
                        {count(entry.finished)}
                      </TableCell>
                      <TableCell className="mono3" style={MONO11}>
                        {entry.node}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </Panel>

            {detail != null && <ClientDetail entry={detail} />}
          </div>
          <p
            style={{
              margin: "0 20px",
              fontSize: "11.5px",
              color: "var(--c-muted)",
              flex: "none",
            }}
          >
            {t("board.nsq.clients.note")}
          </p>
        </PageBody>
      </BoardState>
    </Page>
  );
}

function ClientDetail({ entry }: { entry: NsqClient }) {
  const { t } = useTranslation();
  return (
    <Panel style={{ width: "300px", flex: "none", overflow: "auto" }}>
      <PanelHeader title={entry.peer} />
      <div style={{ padding: "0 12px 12px" }}>
        <SectionLabel>{t("board.nsq.clients.section.identity")}</SectionLabel>
        <KV
          rows={[
            [t("board.nsq.clients.clientId"), entry.clientId || DASH],
            [t("board.nsq.clients.hostname"), entry.hostname || DASH],
            [t("board.nsq.clients.userAgent"), entry.userAgent || DASH],
            [t("board.nsq.clients.protocol"), entry.protocol],
            [t("board.nsq.clients.state"), entry.state],
            [
              t("board.nsq.clients.connectedAt"),
              entry.connectedAtMs > 0
                ? new Date(entry.connectedAtMs).toLocaleString()
                : DASH,
            ],
          ]}
        />

        <SectionLabel>{t("board.nsq.clients.section.work")}</SectionLabel>
        <KV
          rows={
            entry.role === "producer"
              ? [
                  [t("board.nsq.clients.node"), entry.node],
                  [t("board.nsq.clients.published"), entry.published || DASH],
                  [t("board.nsq.clients.messages"), count(entry.messages)],
                ]
              : [
                  [t("board.nsq.clients.topic"), entry.topic],
                  [t("board.nsq.clients.channel"), entry.channel],
                  [t("board.nsq.clients.node"), entry.node],
                  [t("board.nsq.clients.ready"), count(entry.ready)],
                  [t("board.nsq.clients.inFlight"), count(entry.inFlight)],
                  [t("board.nsq.clients.messages"), count(entry.messages)],
                  [t("board.nsq.clients.finished"), count(entry.finished)],
                  [t("board.nsq.clients.requeued"), count(entry.requeued)],
                ]
          }
        />

        <SectionLabel>{t("board.nsq.clients.section.transport")}</SectionLabel>
        <KV
          rows={[
            [t("board.nsq.clients.tls"), entry.tls ? t("common.yes") : t("common.no")],
            [t("board.nsq.clients.snappy"), entry.snappy ? t("common.yes") : t("common.no")],
          ]}
        />

        {askingForNothing(entry) && (
          <p style={{ marginTop: "10px", fontSize: "11px", color: "var(--c-muted)" }}>
            {t("board.nsq.clients.notReady")}
          </p>
        )}
      </div>
    </Panel>
  );
}
