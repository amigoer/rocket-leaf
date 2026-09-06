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
import { useNsqDestinations } from "@/hooks/nsq/useNsqDestinations";
import { holdingUndelivered, topic as readTopic, type NsqTopic } from "@/mq/nsq/destinations";
import { formatBytes, formatCount } from "@/lib/format";
import { formatErrorMessage } from "@/lib/utils";
import { useConnectionScope } from "@/mq/ConnectionScope";
import * as nsqApi from "@/api/nsq";
import { TopicDialogNsq } from "./TopicDialogNsq";

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
  const { id: connID } = useConnectionScope();
  const confirm = useConfirm();
  const [search, setSearch] = useState("");
  const [creating, setCreating] = useState(false);
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

  const create = useCallback(
    async (name: string) => {
      await nsqApi.createTopic(connID, name);
      toast.success(t("board.nsq.topics.created", { name }));
      await state.refresh();
    },
    [connID, state, t],
  );

  const empty = useCallback(
    async (entry: NsqTopic) => {
      const ok = await confirm({
        title: t("board.nsq.topics.emptyTitle", { name: entry.name }),
        // The depth, because it is the number about to be discarded, and it
        // counts one copy per channel rather than one per message published.
        description: t("board.nsq.topics.emptyDesc", { count: entry.depth ?? 0 }),
        confirmLabel: t("board.nsq.topics.emptyAction"),
        danger: true,
      });
      if (!ok) return;
      try {
        await nsqApi.emptyTopic(connID, entry.name);
        toast.success(t("board.nsq.topics.emptied", { name: entry.name }));
        await state.refresh();
      } catch (emptyError) {
        toast.error(t("board.nsq.topics.emptyFailed"), {
          description: formatErrorMessage(emptyError),
        });
      }
    },
    [confirm, connID, state, t],
  );

  const remove = useCallback(
    async (entry: NsqTopic) => {
      const ok = await confirm({
        title: t("board.nsq.topics.deleteTitle", { name: entry.name }),
        description: t("board.nsq.topics.deleteDesc"),
        confirmLabel: t("common.delete"),
        danger: true,
      });
      if (!ok) return;
      try {
        await nsqApi.removeTopic(connID, entry.name);
        toast.success(t("board.nsq.topics.deleted", { name: entry.name }));
        setSelected(null);
        await state.refresh();
      } catch (deleteError) {
        toast.error(t("board.nsq.topics.deleteFailed"), {
          description: formatErrorMessage(deleteError),
        });
      }
    },
    [confirm, connID, state, t],
  );

  // No confirmation: pausing takes nothing away and is undone by the same
  // button. What it does is worth saying afterwards, because publishing
  // carries on and the messages pile up in the topic.
  const pause = useCallback(
    async (entry: NsqTopic) => {
      try {
        await nsqApi.setTopicPaused(connID, entry.name, !entry.paused);
        toast.success(
          t(entry.paused ? "board.nsq.topics.resumed" : "board.nsq.topics.paused", {
            name: entry.name,
          }),
        );
        await state.refresh();
      } catch (pauseError) {
        toast.error(t("board.nsq.topics.pauseFailed"), {
          description: formatErrorMessage(pauseError),
        });
      }
    },
    [connID, state, t],
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
            <Button size="sm" onClick={() => setCreating(true)}>
              {t("board.nsq.topics.create")}
            </Button>
            <RefreshButton
              refreshing={state.refreshing}
              online={state.online}
              onClick={() => void state.refresh()}
            />
          </div>
        }
      />
      <TopicDialogNsq open={creating} onOpenChange={setCreating} onCreate={create} />
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

            {detail != null && (
              <TopicDetail
                entry={detail}
                onEmpty={() => void empty(detail)}
                onRemove={() => void remove(detail)}
                onTogglePause={() => void pause(detail)}
              />
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
            {t("board.nsq.topics.note")}
          </p>
        </PageBody>
      </BoardState>
    </Page>
  );
}

function TopicDetail({
  entry,
  onEmpty,
  onRemove,
  onTogglePause,
}: {
  entry: NsqTopic;
  onEmpty: () => void;
  onRemove: () => void;
  onTogglePause: () => void;
}) {
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

        <div style={{ display: "flex", gap: "6px", marginTop: "12px" }}>
          <Button size="sm" variant="outline" onClick={onTogglePause}>
            {t(entry.paused ? "board.nsq.topics.resumeAction" : "board.nsq.topics.pauseAction")}
          </Button>
          <Button size="sm" variant="destructive" onClick={onEmpty}>
            {t("board.nsq.topics.emptyAction")}
          </Button>
          <Button size="sm" variant="destructive" onClick={onRemove}>
            {t("common.delete")}
          </Button>
        </div>

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
