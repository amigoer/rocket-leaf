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
import { KV, Panel, PanelHeader, SectionLabel, Status, useConfirm } from "@/components";
import { BoardState } from "@/design/boards/BoardState";
import { useNsqSubscriptions } from "@/hooks/nsq/useNsqSubscriptions";
import {
  channel as readChannel,
  channelKey,
  stalledByPause,
  type NsqChannel,
} from "@/mq/nsq/subscriptions";
import { formatCount } from "@/lib/format";
import { formatErrorMessage } from "@/lib/utils";
import { useConnectionScope } from "@/mq/ConnectionScope";
import * as nsqApi from "@/api/nsq";
import { ChannelDialogNsq } from "./ChannelDialogNsq";

const MONO11 = { fontSize: "11px" } as const;

/** A dash, not a zero: an absent figure and a measured zero are not alike. */
const DASH = "—";

function count(value: number | null): string {
  return value == null ? DASH : formatCount(value);
}

/**
 * NSQ channels, which are this family's consumer groups.
 *
 * A channel belongs to one topic and every channel under a topic receives a
 * copy of every message, so the topic is half the identity rather than a
 * column: "analytics" under two topics is two channels with nothing in common.
 *
 * The backlog is the depth, and it is the only lag NSQ has. There is no offset
 * behind it and therefore nothing on this page that moves one - a channel's
 * backlog changes by being consumed or by being emptied, and no third gesture
 * exists. A channel added to a topic that already has one starts at nothing,
 * because the copies were made as the messages arrived.
 *
 * In flight and deferred sit outside the backlog rather than inside it, which
 * is why they have columns: a channel showing zero depth with a hundred in
 * flight has a hundred messages out with consumers that have not answered.
 */
export function ChannelsNsq() {
  const { t } = useTranslation();
  const state = useNsqSubscriptions();
  const { id: connID } = useConnectionScope();
  const confirm = useConfirm();
  const [search, setSearch] = useState("");
  const [creating, setCreating] = useState(false);
  const [selected, setSelected] = useState<string | null>(null);

  const channels = useMemo(() => (state.data ?? []).map(readChannel), [state.data]);

  const shown = useMemo(() => {
    const needle = search.trim().toLowerCase();
    return channels.filter(
      (entry) =>
        needle === "" ||
        entry.name.toLowerCase().includes(needle) ||
        entry.topic.toLowerCase().includes(needle),
    );
  }, [channels, search]);

  const detail = useMemo(
    () => shown.find((entry) => channelKey(entry) === selected) ?? shown[0] ?? null,
    [shown, selected],
  );

  const create = useCallback(
    async (topic: string, name: string) => {
      await nsqApi.createChannel(connID, topic, name);
      toast.success(t("board.nsq.channels.created", { name }));
      await state.refresh();
    },
    [connID, state, t],
  );

  const empty = useCallback(
    async (entry: NsqChannel) => {
      const ok = await confirm({
        title: t("board.nsq.channels.emptyTitle", { name: entry.name }),
        description: t("board.nsq.channels.emptyDesc", { count: entry.backlog ?? 0 }),
        confirmLabel: t("board.nsq.channels.emptyAction"),
        danger: true,
      });
      if (!ok) return;
      try {
        await nsqApi.emptyChannel(connID, entry.topic, entry.name);
        toast.success(t("board.nsq.channels.emptied", { name: entry.name }));
        await state.refresh();
      } catch (emptyError) {
        toast.error(t("board.nsq.channels.emptyFailed"), {
          description: formatErrorMessage(emptyError),
        });
      }
    },
    [confirm, connID, state, t],
  );

  const remove = useCallback(
    async (entry: NsqChannel) => {
      const ok = await confirm({
        title: t("board.nsq.channels.deleteTitle", { name: entry.name }),
        description: t("board.nsq.channels.deleteDesc", { topic: entry.topic }),
        confirmLabel: t("common.delete"),
        danger: true,
      });
      if (!ok) return;
      try {
        await nsqApi.removeChannel(connID, entry.topic, entry.name);
        toast.success(t("board.nsq.channels.deleted", { name: entry.name }));
        setSelected(null);
        await state.refresh();
      } catch (deleteError) {
        toast.error(t("board.nsq.channels.deleteFailed"), {
          description: formatErrorMessage(deleteError),
        });
      }
    },
    [confirm, connID, state, t],
  );

  // No confirmation: pausing takes nothing away and the same button undoes it.
  const pause = useCallback(
    async (entry: NsqChannel) => {
      try {
        await nsqApi.setChannelPaused(connID, entry.topic, entry.name, !entry.paused);
        toast.success(
          t(entry.paused ? "board.nsq.channels.resumed" : "board.nsq.channels.paused", {
            name: entry.name,
          }),
        );
        await state.refresh();
      } catch (pauseError) {
        toast.error(t("board.nsq.channels.pauseFailed"), {
          description: formatErrorMessage(pauseError),
        });
      }
    },
    [connID, state, t],
  );

  return (
    <Page>
      <PageHeader
        title={t("shell.nav.nsq.consumers")}
        subtitle={t("board.nsq.channels.count", { count: channels.length })}
        actions={
          <div style={{ display: "flex", gap: "8px", alignItems: "center" }}>
            <Input
              style={{ width: "200px" }}
              value={search}
              placeholder={t("board.nsq.channels.search")}
              onChange={(event) => setSearch(event.target.value)}
            />
            <Button size="sm" onClick={() => setCreating(true)}>
              {t("board.nsq.channels.create")}
            </Button>
            <RefreshButton
              refreshing={state.refreshing}
              online={state.online}
              onClick={() => void state.refresh()}
            />
          </div>
        }
      />
      <ChannelDialogNsq open={creating} onOpenChange={setCreating} onCreate={create} />
      <BoardState state={state}>
        <PageBody>
          <div style={{ display: "flex", gap: "12px", minHeight: 0, flex: 1 }}>
            <Panel style={{ flex: 1, overflow: "auto" }}>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t("board.nsq.channels.name")}</TableHead>
                    <TableHead>{t("board.nsq.channels.topic")}</TableHead>
                    <TableHead className="num">{t("board.nsq.channels.backlog")}</TableHead>
                    <TableHead className="num">{t("board.nsq.channels.inFlight")}</TableHead>
                    <TableHead className="num">{t("board.nsq.channels.deferred")}</TableHead>
                    <TableHead className="num">{t("board.nsq.channels.clients")}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {shown.map((entry) => (
                    <TableRow
                      key={channelKey(entry)}
                      data-state={
                        detail != null && channelKey(detail) === channelKey(entry)
                          ? "selected"
                          : undefined
                      }
                      onClick={() => setSelected(channelKey(entry))}
                      style={{ cursor: "pointer" }}
                    >
                      <TableCell>
                        <span className="mono3" style={MONO11}>
                          {entry.name}
                        </span>
                        {entry.paused ? (
                          <Status tone="warn" style={{ fontSize: "10px", marginLeft: "6px" }}>
                            {t("board.nsq.channels.pausedBadge")}
                          </Status>
                        ) : (
                          entry.clients === 0 && (
                            <Status tone="off" style={{ fontSize: "10px", marginLeft: "6px" }}>
                              {t("board.nsq.channels.idleBadge")}
                            </Status>
                          )
                        )}
                        {entry.ephemeral && (
                          <Status tone="off" style={{ fontSize: "10px", marginLeft: "6px" }}>
                            {t("board.nsq.channels.ephemeralBadge")}
                          </Status>
                        )}
                      </TableCell>
                      <TableCell>
                        <span className="mono3" style={MONO11}>
                          {entry.topic}
                        </span>
                      </TableCell>
                      <TableCell className="num">
                        <span className="mono3" style={MONO11}>
                          {count(entry.backlog)}
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
                          {entry.clients}
                        </span>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </Panel>

            {detail != null && (
              <ChannelDetail
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
            {t("board.nsq.channels.note")}
          </p>
        </PageBody>
      </BoardState>
    </Page>
  );
}

function ChannelDetail({
  entry,
  onEmpty,
  onRemove,
  onTogglePause,
}: {
  entry: NsqChannel;
  onEmpty: () => void;
  onRemove: () => void;
  onTogglePause: () => void;
}) {
  const { t } = useTranslation();
  return (
    <Panel style={{ width: "300px", flex: "none", overflow: "auto" }}>
      <PanelHeader title={entry.name} />
      <div style={{ padding: "0 12px 12px" }}>
        <SectionLabel>{t("board.nsq.channels.section.backlog")}</SectionLabel>
        <KV
          rows={[
            [t("board.nsq.channels.topic"), entry.topic],
            [t("board.nsq.channels.backlog"), count(entry.backlog)],
            [t("board.nsq.channels.backendDepth"), count(entry.backendDepth)],
            [t("board.nsq.channels.inFlight"), count(entry.inFlight)],
            [t("board.nsq.channels.deferred"), count(entry.deferred)],
          ]}
        />

        <SectionLabel>{t("board.nsq.channels.section.delivery")}</SectionLabel>
        <KV
          rows={[
            [t("board.nsq.channels.messages"), count(entry.messages)],
            [t("board.nsq.channels.requeued"), count(entry.requeued)],
            [t("board.nsq.channels.timedOut"), count(entry.timedOut)],
            [t("board.nsq.channels.clients"), String(entry.clients)],
            [t("board.nsq.channels.nodes"), entry.nodes.join(", ") || DASH],
          ]}
        />

        <div style={{ display: "flex", gap: "6px", marginTop: "12px" }}>
          <Button size="sm" variant="outline" onClick={onTogglePause}>
            {t(entry.paused ? "board.nsq.channels.resumeAction" : "board.nsq.channels.pauseAction")}
          </Button>
          <Button size="sm" variant="destructive" onClick={onEmpty}>
            {t("board.nsq.channels.emptyAction")}
          </Button>
          <Button size="sm" variant="destructive" onClick={onRemove}>
            {t("common.delete")}
          </Button>
        </div>

        {/* The one backlog with no consumer to look at. */}
        {stalledByPause(entry) && (
          <p style={{ marginTop: "10px", fontSize: "11px", color: "var(--c-muted)" }}>
            {t("board.nsq.channels.stalled", { count: entry.backlog ?? 0 })}
          </p>
        )}
      </div>
    </Panel>
  );
}
