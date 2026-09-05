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
import { useNsqDestinations } from "@/hooks/nsq/useNsqDestinations";
import { holdingUndelivered, topic as readTopic, type NsqTopic } from "@/mq/nsq/destinations";
import { formatBytes, formatCount } from "@/lib/format";

const MONO11 = { fontSize: "11px" } as const;

/** A dash, not a zero: an absent figure and a measured zero are not alike. */
const DASH = "—";

function count(value: number | null): string {
  return value == null ? DASH : formatCount(value);
}

/**
 * NSQ topics.
 *
 * One row per name, however many nsqd are carrying it. A topic exists on the
 * daemon it was created on and those copies are independent queues rather than
 * shards, so the node column is a real fact about placement - a topic on one
 * of three daemons is receiving a third of what a producer sends round-robin.
 *
 * The depth column can exceed the message count, and that is nsqd rather than
 * arithmetic: a message is copied into every channel, so a topic with two
 * channels each holding a hundred is holding two hundred. The detail panel
 * splits it, because where the messages are is the whole question - anything
 * sitting in the topic itself has reached no channel at all, which means the
 * topic is paused or has no channel to copy into.
 *
 * There is no depth chart and no message rate. nsqd counts messages since it
 * started and reports no rate of any kind, and a figure derived from two
 * samples here would be this app's arithmetic presented as the broker's.
 */
export function TopicsNsq() {
  const { t } = useTranslation();
  const state = useNsqDestinations();
  const [search, setSearch] = useState("");
  const [selected, setSelected] = useState<string | null>(null);

  const topics = useMemo(() => (state.data ?? []).map(readTopic), [state.data]);

  const shown = useMemo(() => {
    const needle = search.trim().toLowerCase();
    return topics.filter((entry) => needle === "" || entry.name.toLowerCase().includes(needle));
  }, [topics, search]);

  const detail = useMemo(
    () => shown.find((entry) => entry.name === selected) ?? shown[0] ?? null,
    [shown, selected],
  );

  return (
    <Page>
      <PageHeader
        title={t("shell.nav.nsq.topics")}
        subtitle={t("board.nsq.topics.count", { count: topics.length })}
        actions={
          <div style={{ display: "flex", gap: "8px", alignItems: "center" }}>
            <Input
              style={{ width: "200px" }}
              value={search}
              placeholder={t("board.nsq.topics.search")}
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
                    <TableHead>{t("board.nsq.topics.name")}</TableHead>
                    <TableHead className="num">{t("board.nsq.topics.depth")}</TableHead>
                    <TableHead className="num">{t("board.nsq.topics.channels")}</TableHead>
                    <TableHead className="num">{t("board.nsq.topics.inFlight")}</TableHead>
                    <TableHead className="num">{t("board.nsq.topics.deferred")}</TableHead>
                    <TableHead className="num">{t("board.nsq.topics.messages")}</TableHead>
                    <TableHead className="num">{t("board.nsq.topics.nodes")}</TableHead>
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
                        {entry.paused && (
                          <span className="pb pRDS" style={{ marginLeft: "6px" }}>
                            {t("board.nsq.topics.pausedBadge")}
                          </span>
                        )}
                        {entry.ephemeral && (
                          <span className="pb pNSQ" style={{ marginLeft: "6px" }}>
                            {t("board.nsq.topics.ephemeralBadge")}
                          </span>
                        )}
                      </TableCell>
                      <TableCell className="num">
                        <span className="mono3" style={MONO11}>
                          {count(entry.depth)}
                        </span>
                      </TableCell>
                      <TableCell className="num">
                        <span className="mono3" style={MONO11}>
                          {entry.channels.length}
                        </span>
                      </TableCell>
                      <TableCell className="num">
                        <span className="mono3" style={MONO11}>
                          {count(entry.inFlight)}
                        </span>
                      </TableCell>
                      <TableCell className="num">
                        <span className="mono3" style={MONO11}>
                          {count(entry.deferred)}
                        </span>
                      </TableCell>
                      <TableCell className="num">
                        <span className="mono3" style={MONO11}>
                          {count(entry.messages)}
                        </span>
                      </TableCell>
                      <TableCell className="num">
                        <span className="mono3" style={MONO11}>
                          {entry.nodes.length}
                        </span>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </Panel>

            {detail != null && <TopicDetail entry={detail} />}
          </div>
          <p
            style={{
              margin: "0 20px",
              fontSize: "11.5px",
              color: "var(--c-muted)",
              flex: "none",
            }}
          >
            {t("board.nsq.topics.note")}
          </p>
        </PageBody>
      </BoardState>
    </Page>
  );
}

function TopicDetail({ entry }: { entry: NsqTopic }) {
  const { t } = useTranslation();
  return (
    <Panel style={{ width: "300px", flex: "none", overflow: "auto" }}>
      <PanelHeader title={entry.name} />
      <div style={{ padding: "0 12px 12px" }}>
        <SectionLabel>{t("board.nsq.topics.section.depth")}</SectionLabel>
        <KV
          rows={[
            [t("board.nsq.topics.depth"), count(entry.depth)],
            [t("board.nsq.topics.topicDepth"), count(entry.topicDepth)],
            [t("board.nsq.topics.channelDepth"), count(entry.channelDepth)],
            [t("board.nsq.topics.backendDepth"), count(entry.backendDepth)],
          ]}
        />

        <SectionLabel>{t("board.nsq.topics.section.traffic")}</SectionLabel>
        <KV
          rows={[
            [t("board.nsq.topics.messages"), count(entry.messages)],
            [
              t("board.nsq.topics.bytes"),
              entry.bytes == null ? DASH : formatBytes(entry.bytes),
            ],
            [t("board.nsq.topics.inFlight"), count(entry.inFlight)],
            [t("board.nsq.topics.deferred"), count(entry.deferred)],
            [t("board.nsq.topics.requeued"), count(entry.requeued)],
            [t("board.nsq.topics.timedOut"), count(entry.timedOut)],
          ]}
        />

        <SectionLabel>{t("board.nsq.topics.section.placement")}</SectionLabel>
        <KV
          rows={[
            [t("board.nsq.topics.nodes"), entry.nodes.join(", ") || DASH],
            [t("board.nsq.topics.channels"), entry.channels.join(", ") || DASH],
          ]}
        />

        {/* Said where the number is, because this is where a reader meets a
            depth that no consumer can be blamed for. */}
        {holdingUndelivered(entry) && (
          <p style={{ marginTop: "10px", fontSize: "11px", color: "var(--c-muted)" }}>
            {t(entry.paused ? "board.nsq.topics.heldPaused" : "board.nsq.topics.heldNoChannel", {
              count: entry.topicDepth ?? 0,
            })}
          </p>
        )}
      </div>
    </Panel>
  );
}
