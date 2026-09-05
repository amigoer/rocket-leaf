import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { Page, PageBody, PageHeader, RefreshButton } from "@/design/shell";
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
import {
  useActiveMQNodeConfig,
  useActiveMQNodes,
} from "@/hooks/activemq/useActiveMQCluster";
import { node as readNode } from "@/mq/activemq/cluster";
import { formatBytes, formatCount } from "@/lib/format";

const MONO11 = { fontSize: "11px" } as const;
const DASH = "—";

function count(value: number | null): string {
  return value == null ? DASH : formatCount(value);
}

function percent(value: number | null): string {
  return value == null ? DASH : `${value}%`;
}

function yesNo(value: boolean | null, yes: string, no: string): string {
  return value == null ? DASH : value ? yes : no;
}

/**
 * The ActiveMQ broker.
 *
 * This is a broker page more than a cluster page, and the difference is the
 * family's rather than the driver's. A JMS broker is a unit: destinations live
 * on the one that owns them, clients connect to it, and clustering here is a
 * bridge between brokers rather than a set of nodes sharing a namespace. So an
 * ordinary deployment shows one row, and the extra rows are links this broker
 * declares - Classic's network connectors, Artemis's cluster connections.
 *
 * A bridged row's state is unknown by construction and says so. The link is
 * configured here; the broker at the other end answers on its own console,
 * which this connection has no way to reach, so reporting it as online or
 * offline would be a guess either way.
 *
 * No rate anywhere. Both products keep cumulative enqueue and dequeue
 * counters, and dividing two samples of those would be this app's arithmetic
 * printed as the broker's figure.
 */
export function BrokerActiveMQ() {
  const { t } = useTranslation();
  const state = useActiveMQNodes();
  const [selected, setSelected] = useState<string | null>(null);

  const nodes = useMemo(() => (state.data ?? []).map(readNode), [state.data]);
  const detail = useMemo(
    () => nodes.find((entry) => entry.name === selected) ?? nodes[0] ?? null,
    [nodes, selected],
  );
  // Only the broker itself has settings to read; a bridge is a name.
  const configState = useActiveMQNodeConfig(
    detail != null && !detail.bridge ? detail.address : null,
  );

  const settings = useMemo(() => {
    const document = configState.data;
    if (document?.settings == null) return [];
    return Object.entries(document.settings).sort(([left], [right]) =>
      left.localeCompare(right),
    );
  }, [configState.data]);

  return (
    <Page>
      <PageHeader
        title={t("shell.nav.activemq.cluster")}
        subtitle={t("board.activemq.cluster.count", {
          bridges: nodes.filter((entry) => entry.bridge).length,
        })}
        actions={
          <RefreshButton
            refreshing={state.refreshing}
            online={state.online}
            onClick={() => void state.refresh()}
          />
        }
      />
      <BoardState state={state}>
        <PageBody>
          <div style={{ display: "flex", gap: "12px", minHeight: 0, flex: 1 }}>
            <Panel style={{ flex: 1, overflow: "auto" }}>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t("board.activemq.cluster.name")}</TableHead>
                    <TableHead>{t("board.activemq.cluster.version")}</TableHead>
                    <TableHead>{t("board.activemq.cluster.uptime")}</TableHead>
                    <TableHead className="num">
                      {t("board.activemq.cluster.messages")}
                    </TableHead>
                    <TableHead className="num">
                      {t("board.activemq.cluster.connections")}
                    </TableHead>
                    <TableHead className="num">{t("board.activemq.cluster.disk")}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {nodes.map((entry) => (
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
                        {entry.bridge && (
                          <span className="pb pAMQ" style={{ marginLeft: "6px" }}>
                            {t("board.activemq.cluster.bridgeBadge")}
                          </span>
                        )}
                        {entry.backup === true && (
                          <span className="pb pPLS" style={{ marginLeft: "6px" }}>
                            {t("board.activemq.cluster.backupBadge")}
                          </span>
                        )}
                      </TableCell>
                      <TableCell>{entry.version === "" ? DASH : entry.version}</TableCell>
                      <TableCell>{entry.uptime ?? DASH}</TableCell>
                      <TableCell className="num">
                        <span className="mono3" style={MONO11}>
                          {count(entry.totalMessages)}
                        </span>
                      </TableCell>
                      <TableCell className="num">
                        <span className="mono3" style={MONO11}>
                          {count(entry.connections)}
                        </span>
                      </TableCell>
                      <TableCell className="num">
                        <span className="mono3" style={MONO11}>
                          {percent(entry.diskUsage)}
                        </span>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </Panel>

            {detail != null && (
              <Panel style={{ width: "320px", flex: "none", overflow: "auto" }}>
                <PanelHeader title={detail.name} />
                <div style={{ padding: "0 12px 12px" }}>
                  {detail.bridge ? (
                    <p style={{ fontSize: "11px", color: "var(--c-muted)" }}>
                      {t("board.activemq.cluster.bridgeNote")}
                    </p>
                  ) : (
                    <>
                      <SectionLabel>{t("board.activemq.cluster.section.identity")}</SectionLabel>
                      <KV
                        rows={[
                          [t("board.activemq.cluster.version"), detail.version || DASH],
                          [t("board.activemq.cluster.nodeId"), detail.nodeId ?? DASH],
                          [t("board.activemq.cluster.uptime"), detail.uptime ?? DASH],
                          [
                            t("board.activemq.cluster.persistent"),
                            yesNo(detail.persistent, t("common.yes"), t("common.no")),
                          ],
                          [t("board.activemq.cluster.dataDirectory"), detail.dataDirectory ?? DASH],
                        ]}
                      />

                      <SectionLabel>{t("board.activemq.cluster.section.load")}</SectionLabel>
                      <KV
                        rows={[
                          [t("board.activemq.cluster.messages"), count(detail.totalMessages)],
                          [t("board.activemq.cluster.enqueued"), count(detail.totalEnqueued)],
                          [t("board.activemq.cluster.dequeued"), count(detail.totalDequeued)],
                          [t("board.activemq.cluster.consumers"), count(detail.consumers)],
                          [t("board.activemq.cluster.producers"), count(detail.producers)],
                        ]}
                      />

                      <SectionLabel>{t("board.activemq.cluster.section.storage")}</SectionLabel>
                      <KV
                        rows={[
                          [t("board.activemq.cluster.disk"), percent(detail.diskUsage)],
                          [t("board.activemq.cluster.store"), percent(detail.storePercent)],
                          [t("board.activemq.cluster.temp"), percent(detail.tempPercent)],
                          [t("board.activemq.cluster.memory"), percent(detail.memoryPercent)],
                          [
                            t("board.activemq.cluster.memoryLimit"),
                            detail.memoryLimit == null ? DASH : formatBytes(detail.memoryLimit),
                          ],
                          [t("board.activemq.cluster.journalType"), detail.journalType ?? DASH],
                        ]}
                      />

                      {detail.product === "artemis" && (
                        <>
                          <SectionLabel>
                            {t("board.activemq.cluster.section.availability")}
                          </SectionLabel>
                          <KV
                            rows={[
                              [
                                t("board.activemq.cluster.clustered"),
                                yesNo(detail.clustered, t("common.yes"), t("common.no")),
                              ],
                              [t("board.activemq.cluster.haPolicy"), detail.haPolicy ?? DASH],
                              [
                                t("board.activemq.cluster.security"),
                                yesNo(detail.securityEnabled, t("common.yes"), t("common.no")),
                              ],
                              [
                                t("board.activemq.cluster.acceptors"),
                                detail.acceptors?.join(", ") ?? DASH,
                              ],
                            ]}
                          />
                        </>
                      )}

                      <SectionLabel>{t("board.activemq.cluster.section.settings")}</SectionLabel>
                      {settings.length === 0 ? (
                        <p style={{ fontSize: "11px", color: "var(--c-muted)" }}>
                          {t("board.activemq.cluster.noSettings")}
                        </p>
                      ) : (
                        <KV rows={settings} />
                      )}
                    </>
                  )}
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
            {t("board.activemq.cluster.note")}
          </p>
        </PageBody>
      </BoardState>
    </Page>
  );
}
