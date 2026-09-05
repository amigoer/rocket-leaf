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
import { KV, Panel, PanelHeader, SectionLabel, StatTile, Status, WarnBanner } from "@/components";
import { BoardState } from "@/design/boards/BoardState";
import { useNsqCluster, useNsqNodeConfig } from "@/hooks/nsq/useNsqCluster";
import {
  advertisesSomethingElse,
  directoryNode as readDirectoryNode,
  node as readNode,
  type NsqDirectoryNode,
} from "@/mq/nsq/cluster";
import { formatBytes, formatCount } from "@/lib/format";
import { present } from "@/api/client";

const MONO11 = { fontSize: "11px" } as const;

/** A dash, not a zero: an absent figure and a measured zero are not alike. */
const DASH = "—";

function count(value: number | null): string {
  return value == null ? DASH : formatCount(value);
}

/**
 * The NSQ cluster: the daemons that hold messages, and the tier that tells
 * consumers where they are.
 *
 * Two tables rather than one, because the two answer different questions and
 * can disagree. The nsqd list is the profile's own - there is no daemon that
 * knows about the others, so it is exactly the set every figure elsewhere in
 * the app is a sum over. The nsqlookupd list is what a consumer is told when
 * it asks, and the addresses it hands out are whatever each nsqd broadcast
 * about itself, which need not be reachable from here.
 *
 * That mismatch is the one thing this page warns about. A daemon this app
 * reaches at 127.0.0.1:4151 and advertises as a container hostname sends every
 * consumer using discovery somewhere it cannot go, and nothing else in the app
 * can see it.
 *
 * There is no disk figure and no rate. nsqd reports neither: a topic's
 * overflow file sits wherever --data-path points and the daemon never looks at
 * it, and every message figure is a total since start rather than a rate.
 */
export function ClusterNsq() {
  const { t } = useTranslation();
  const state = useNsqCluster();
  const [selected, setSelected] = useState<string | null>(null);

  const nodes = useMemo(() => present(state.data?.nodes).map(readNode), [state.data]);
  const directory = useMemo(
    () => present(state.data?.directory).map(readDirectoryNode),
    [state.data],
  );
  const overview = state.data?.overview;

  const detail = useMemo(
    () => nodes.find((entry) => entry.address === selected) ?? nodes[0] ?? null,
    [nodes, selected],
  );
  const config = useNsqNodeConfig(detail?.address ?? null);

  const unreachable = useMemo(
    () => advertisesSomethingElse(directory, nodes),
    [directory, nodes],
  );

  return (
    <Page>
      <PageHeader
        title={t("shell.nav.nsq.cluster")}
        subtitle={t("board.nsq.cluster.subtitle", {
          nodes: nodes.length,
          directory: directory.length,
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
          {unreachable.length > 0 && (
            <WarnBanner>
              {t("board.nsq.cluster.advertiseMismatch", { addresses: unreachable.join(", ") })}
            </WarnBanner>
          )}

          <div
            style={{
              display: "grid",
              gridTemplateColumns: "repeat(4, minmax(0, 1fr))",
              gap: "12px",
              flex: "none",
            }}
          >
            <StatTile
              label={t("board.nsq.cluster.nodes")}
              value={String(overview?.totalNodes ?? 0)}
            />
            <StatTile
              label={t("board.nsq.cluster.online")}
              value={String(overview?.onlineNodes ?? 0)}
            />
            <StatTile
              label={t("board.nsq.cluster.topics")}
              value={formatCount(overview?.destinations ?? 0)}
            />
            <StatTile
              label={t("board.nsq.cluster.channels")}
              value={formatCount(overview?.subscriptions ?? 0)}
            />
          </div>

          <div style={{ display: "flex", gap: "12px", minHeight: 0, flex: 1 }}>
            <div style={{ display: "flex", flexDirection: "column", gap: "12px", flex: 1, minWidth: 0 }}>
              <Panel style={{ overflow: "auto" }}>
                <PanelHeader title={t("board.nsq.cluster.nsqd")} />
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>{t("board.nsq.cluster.address")}</TableHead>
                      <TableHead>{t("board.nsq.cluster.version")}</TableHead>
                      <TableHead className="num">{t("board.nsq.cluster.topics")}</TableHead>
                      <TableHead className="num">{t("board.nsq.cluster.channels")}</TableHead>
                      <TableHead className="num">{t("board.nsq.cluster.clients")}</TableHead>
                      <TableHead className="num">{t("board.nsq.cluster.depth")}</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {nodes.map((entry) => (
                      <TableRow
                        key={entry.address}
                        data-state={detail?.address === entry.address ? "selected" : undefined}
                        onClick={() => setSelected(entry.address)}
                        style={{ cursor: "pointer" }}
                      >
                        <TableCell>
                          <span className="mono3" style={MONO11}>
                            {entry.address}
                          </span>
                          {entry.status !== "online" && (
                            <Status tone="warn" style={{ fontSize: "10px", marginLeft: "6px" }}>
                              {entry.health || t("board.nsq.cluster.unhealthy")}
                            </Status>
                          )}
                        </TableCell>
                        <TableCell className="mono3" style={MONO11}>
                          {entry.version}
                        </TableCell>
                        <TableCell className="num mono3" style={MONO11}>
                          {count(entry.topics)}
                        </TableCell>
                        <TableCell className="num mono3" style={MONO11}>
                          {count(entry.channels)}
                        </TableCell>
                        <TableCell className="num mono3" style={MONO11}>
                          {count(entry.clients)}
                        </TableCell>
                        <TableCell className="num mono3" style={MONO11}>
                          {count(entry.depth)}
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </Panel>

              <Panel style={{ overflow: "auto" }}>
                <PanelHeader title={t("board.nsq.cluster.lookupd")} />
                {directory.length === 0 ? (
                  <p style={{ padding: "0 12px 12px", fontSize: "11.5px", color: "var(--c-muted)" }}>
                    {t("board.nsq.cluster.noLookupd")}
                  </p>
                ) : (
                  <DirectoryTable directory={directory} />
                )}
              </Panel>
            </div>

            {detail != null && (
              <Panel style={{ width: "320px", flex: "none", overflow: "auto" }}>
                <PanelHeader title={detail.address} />
                <div style={{ padding: "0 12px 12px" }}>
                  <SectionLabel>{t("board.nsq.cluster.section.identity")}</SectionLabel>
                  <KV
                    rows={[
                      [t("board.nsq.cluster.hostname"), detail.hostname || DASH],
                      [t("board.nsq.cluster.broadcast"), detail.broadcastAddress || DASH],
                      [t("board.nsq.cluster.tcpPort"), detail.tcpPort || DASH],
                      [t("board.nsq.cluster.httpPort"), detail.httpPort || DASH],
                      [t("board.nsq.cluster.health"), detail.health || DASH],
                    ]}
                  />

                  <SectionLabel>{t("board.nsq.cluster.section.memory")}</SectionLabel>
                  <KV
                    rows={[
                      [
                        t("board.nsq.cluster.heapInUse"),
                        detail.heapInUse == null ? DASH : formatBytes(detail.heapInUse),
                      ],
                      [t("board.nsq.cluster.heapObjects"), count(detail.heapObjects)],
                      [t("board.nsq.cluster.gcRuns"), count(detail.gcRuns)],
                    ]}
                  />

                  <SectionLabel>{t("board.nsq.cluster.section.config")}</SectionLabel>
                  <KV
                    rows={Object.entries(config.data ?? {}).map(([key, value]) => [
                      key,
                      value ?? DASH,
                    ])}
                  />
                  <p style={{ marginTop: "10px", fontSize: "11px", color: "var(--c-muted)" }}>
                    {t("board.nsq.cluster.configNote")}
                  </p>
                </div>
              </Panel>
            )}
          </div>
        </PageBody>
      </BoardState>
    </Page>
  );
}

function DirectoryTable({ directory }: { directory: NsqDirectoryNode[] }) {
  const { t } = useTranslation();
  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>{t("board.nsq.cluster.address")}</TableHead>
          <TableHead>{t("board.nsq.cluster.version")}</TableHead>
          <TableHead className="num">{t("board.nsq.cluster.producers")}</TableHead>
          <TableHead className="num">{t("board.nsq.cluster.knownTopics")}</TableHead>
          <TableHead>{t("board.nsq.cluster.advertises")}</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {directory.map((entry) => (
          <TableRow key={entry.address}>
            <TableCell className="mono3" style={MONO11}>
              {entry.address}
            </TableCell>
            <TableCell className="mono3" style={MONO11}>
              {entry.version}
            </TableCell>
            <TableCell className="num mono3" style={MONO11}>
              {count(entry.producers)}
            </TableCell>
            <TableCell className="num mono3" style={MONO11}>
              {count(entry.topics)}
            </TableCell>
            <TableCell className="mono3" style={MONO11}>
              {entry.advertises.join(", ") || DASH}
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  );
}
