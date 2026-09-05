import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { Page, PageHeader, Toolbar } from "@/design/shell";
import { Button } from "@/components/ui/button";
import { Panel, SectionLabel, SelectField, Status } from "@/components";
import { useActiveMQStream } from "@/hooks/activemq/useActiveMQStream";
import { useActiveMQDestinations } from "@/hooks/activemq/useActiveMQDestinations";
import { destination as readDestination } from "@/mq/activemq/destinations";
import type { LiveMessage } from "@/api/models";

const MONO11 = { fontSize: "11px" } as const;

/**
 * Watching a topic as messages go past.
 *
 * Not the messages page with a filter on it, and the difference is the whole
 * reason this page exists. That one browses: it reads what the broker is
 * holding, takes nothing, and asking again returns the same messages. This one
 * attaches a subscriber. What arrives is a copy of what is published while it
 * listens, nothing is stored, and a message published before the stream
 * started was never anywhere this page could find it.
 *
 * Topics only, and that is a safety rule rather than a gap. A JMS consumer
 * consumes: attaching one to a queue would take messages off it and hand them
 * to a window somebody opened to look at. So the picker offers topics and the
 * driver refuses a queue even if one were named another way.
 *
 * Two figures are load-bearing rather than decorative. "Listening" separates a
 * session that dropped from a topic nobody is publishing to, and those are the
 * same empty page. "Dropped" separates a stream quietly losing messages from a
 * quiet one, and those are the same page too.
 */
export function ActiveMQWorkbench() {
  const { t } = useTranslation();
  const stream = useActiveMQStream();
  const destinationState = useActiveMQDestinations();
  const [topic, setTopic] = useState("");

  const topics = useMemo(
    () =>
      (destinationState.data ?? [])
        .map(readDestination)
        .filter((entry) => entry.kind === "topic")
        .map((entry) => ({ value: entry.name, label: entry.name })),
    [destinationState.data],
  );

  const chosen = topic !== "" ? topic : (topics[0]?.value ?? "");

  return (
    <Page>
      <PageHeader
        title={t("shell.nav.activemq.subscribe")}
        subtitle={
          stream.running
            ? t("board.activemq.subscribe.watching", { topics: stream.topics.join(", ") })
            : t("board.activemq.subscribe.idle")
        }
      />
      <Toolbar>
        <SelectField
          value={chosen}
          options={topics}
          onValueChange={(next) => setTopic(next)}
        />
        {stream.running ? (
          <Button size="sm" variant="destructive" onClick={stream.stop}>
            {t("board.activemq.subscribe.stop")}
          </Button>
        ) : (
          <Button
            size="sm"
            disabled={chosen === ""}
            onClick={() => stream.start({ topics: [chosen], buffer: 0 })}
          >
            {t("board.activemq.subscribe.start")}
          </Button>
        )}
        <Button size="sm" variant="outline" onClick={stream.clear}>
          {t("board.activemq.subscribe.clear")}
        </Button>
        <div style={{ marginLeft: "auto", display: "flex", gap: "12px", alignItems: "center" }}>
          <Status tone={stream.running ? (stream.live ? "ok" : "warn") : "off"}>
            {stream.running
              ? stream.live
                ? t("board.activemq.subscribe.listening")
                : t("board.activemq.subscribe.dropped")
              : t("board.activemq.subscribe.stopped")}
          </Status>
          <span className="mono3" style={{ ...MONO11, color: "var(--c-muted)" }}>
            {t("board.activemq.subscribe.counts", {
              received: stream.received,
              lost: stream.dropped,
            })}
          </span>
        </div>
      </Toolbar>
      {stream.error != null && (
        <p style={{ margin: "0 20px", fontSize: "11.5px", color: "var(--c-danger)" }}>
          {stream.error}
        </p>
      )}
      <Panel style={{ flex: 1, margin: "0 20px 12px", overflow: "auto" }}>
        <SectionLabel>{t("board.activemq.subscribe.arrivals")}</SectionLabel>
        {stream.messages.length === 0 ? (
          <p style={{ padding: "8px 12px", fontSize: "11px", color: "var(--c-muted)" }}>
            {stream.running
              ? t("board.activemq.subscribe.waiting")
              : t("board.activemq.subscribe.notStarted")}
          </p>
        ) : (
          <div style={{ padding: "0 12px 12px" }}>
            {stream.messages
              .slice()
              .reverse()
              .map((message: LiveMessage) => (
                <div
                  key={message.seq}
                  style={{
                    padding: "6px 0",
                    borderBottom: "1px solid var(--c-border)",
                    display: "flex",
                    gap: "10px",
                    alignItems: "baseline",
                  }}
                >
                  <span className="mono3" style={{ ...MONO11, color: "var(--c-muted)" }}>
                    {message.receivedAt}
                  </span>
                  <span className="mono3" style={{ ...MONO11, color: "var(--c-muted-2)" }}>
                    {message.destination}
                  </span>
                  <span className="mono3" style={{ ...MONO11, flex: 1, wordBreak: "break-all" }}>
                    {message.body}
                  </span>
                </div>
              ))}
          </div>
        )}
      </Panel>
      <p
        style={{
          margin: "0 20px 12px",
          fontSize: "11.5px",
          color: "var(--c-muted)",
          flex: "none",
        }}
      >
        {t("board.activemq.subscribe.note")}
      </p>
    </Page>
  );
}
