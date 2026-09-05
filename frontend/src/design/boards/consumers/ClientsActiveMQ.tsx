import { useCallback, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { Page, PageBody, PageHeader, RefreshButton } from "@/design/shell";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { KV, Panel, PanelHeader, SectionLabel, useConfirm } from "@/components";
import { BoardState } from "@/design/boards/BoardState";
import { useActiveMQConnections } from "@/hooks/activemq/useActiveMQCluster";
import {
  connection as readConnection,
  type ActiveMQConnection,
} from "@/mq/activemq/cluster";
import { formatCount } from "@/lib/format";
import { formatErrorMessage } from "@/lib/utils";
import { useConnectionScope } from "@/mq/ConnectionScope";
import * as activemqApi from "@/api/activemq";

const MONO11 = { fontSize: "11px" } as const;
const DASH = "—";

/**
 * ActiveMQ connections.
 *
 * The sessions column is not a channel count, and the distinction matters
 * enough to keep the wording apart. A channel is AMQP 0-9-1's multiplexing
 * layer with its own prefetch and its own flow control, and an operator closes
 * one. A JMS session has none of that: it is a unit of work inside the
 * connection, it cannot be closed from outside, and there is no separate page
 * for them here because there would be nothing to do on it.
 *
 * The protocol column is read differently on each product and both are the
 * broker's own answer. Artemis reports the connection's Java class, mapped to
 * the protocol's name - the raw ActiveMQProtonRemotingConnection is not
 * something to put in a column. Classic reports which connector accepted the
 * socket, which is the same fact from the other side.
 */
export function ClientsActiveMQ() {
  const { t } = useTranslation();
  const state = useActiveMQConnections();
  const { id: connID } = useConnectionScope();
  const confirm = useConfirm();
  const [search, setSearch] = useState("");
  const [selected, setSelected] = useState<string | null>(null);

  const connections = useMemo(
    () => (state.data ?? []).map(readConnection),
    [state.data],
  );
  const shown = useMemo(() => {
    const needle = search.trim().toLowerCase();
    if (needle === "") return connections;
    return connections.filter(
      (entry) =>
        entry.peer.toLowerCase().includes(needle) ||
        entry.user.toLowerCase().includes(needle) ||
        entry.clientName.toLowerCase().includes(needle),
    );
  }, [connections, search]);

  const detail = useMemo(
    () => shown.find((entry) => entry.name === selected) ?? shown[0] ?? null,
    [shown, selected],
  );

  const close = useCallback(
    async (entry: ActiveMQConnection) => {
      const ok = await confirm({
        title: t("board.activemq.clients.closeTitle", { peer: entry.peer }),
        description: t("board.activemq.clients.closeDesc"),
        confirmLabel: t("board.activemq.clients.close"),
        danger: true,
      });
      if (!ok) return;
      try {
        await activemqApi.closeConnection(connID, entry.name);
        toast.success(t("board.activemq.clients.closed", { peer: entry.peer }));
        setSelected(null);
        await state.refresh();
      } catch (closeError) {
        toast.error(t("board.activemq.clients.closeFailed"), {
          description: formatErrorMessage(closeError),
        });
      }
    },
    [confirm, connID, state, t],
  );

  return (
    <Page>
      <PageHeader
        title={t("shell.nav.activemq.clients")}
        subtitle={t("board.activemq.clients.count", { count: connections.length })}
        actions={
          <div style={{ display: "flex", gap: "8px", alignItems: "center" }}>
            <Input
              style={{ width: "200px" }}
              value={search}
              placeholder={t("board.activemq.clients.search")}
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
                    <TableHead>{t("board.activemq.clients.peer")}</TableHead>
                    <TableHead>{t("board.activemq.clients.protocol")}</TableHead>
                    <TableHead>{t("board.activemq.clients.user")}</TableHead>
                    <TableHead>{t("board.activemq.clients.state")}</TableHead>
                    <TableHead className="num">
                      {t("board.activemq.clients.sessions")}
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
                          {entry.peer}
                        </span>
                      </TableCell>
                      <TableCell>{entry.protocol === "" ? DASH : entry.protocol}</TableCell>
                      <TableCell>{entry.user === "" ? DASH : entry.user}</TableCell>
                      <TableCell>
                        {entry.blocked === true
                          ? t("board.activemq.clients.blocked")
                          : entry.slow === true
                            ? t("board.activemq.clients.slow")
                            : t("board.activemq.clients.running")}
                      </TableCell>
                      <TableCell className="num">
                        <span className="mono3" style={MONO11}>
                          {formatCount(entry.sessions)}
                        </span>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </Panel>

            {detail != null && (
              <Panel style={{ width: "300px", flex: "none", overflow: "auto" }}>
                <PanelHeader title={detail.peer} />
                <div style={{ padding: "0 12px 12px" }}>
                  <SectionLabel>{t("board.activemq.clients.section.identity")}</SectionLabel>
                  <KV
                    rows={[
                      [t("board.activemq.clients.connectionId"), detail.name],
                      [
                        t("board.activemq.clients.clientId"),
                        detail.clientName === "" ? DASH : detail.clientName,
                      ],
                      [t("board.activemq.clients.user"), detail.user === "" ? DASH : detail.user],
                      [t("board.activemq.clients.protocol"), detail.protocol || DASH],
                      // Classic's is the connector that accepted the socket,
                      // which is where its protocol comes from; Artemis has no
                      // connector on a connection and this is blank.
                      [t("board.activemq.clients.connector"), detail.connector ?? DASH],
                      [t("board.activemq.clients.created"), detail.created ?? DASH],
                      [t("board.activemq.clients.sessions"), formatCount(detail.sessions)],
                    ]}
                  />
                  <Button
                    size="sm"
                    variant="destructive"
                    style={{ marginTop: "12px" }}
                    onClick={() => void close(detail)}
                  >
                    {t("board.activemq.clients.close")}
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
            {t("board.activemq.clients.sessionNote")}
          </p>
        </PageBody>
      </BoardState>
    </Page>
  );
}
