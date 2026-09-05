import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { Page, PageBody, PageHeader, RefreshButton } from "@/design/shell";
import { Input } from "@/components/ui/input";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
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
import { useActiveMQDestinations } from "@/hooks/activemq/useActiveMQDestinations";
import {
  browseWillBeCapped,
  destination as readDestination,
  type DestinationKind,
} from "@/mq/activemq/destinations";
import { formatBytes, formatCount } from "@/lib/format";

const MONO11 = { fontSize: "11px" } as const;

/**
 * A dash, not a zero.
 *
 * One MQKind covers two brokers that report different things, so a column is
 * genuinely empty on one of them. Printing 0 would be this app asserting a
 * figure the broker never gave.
 */
const DASH = "—";

function count(value: number | null): string {
  return value == null ? DASH : formatCount(value);
}

function Cell({ value }: { value: number | null }) {
  return (
    <span className="mono3" style={MONO11}>
      {count(value)}
    </span>
  );
}

type KindFilter = "all" | DestinationKind;

/**
 * ActiveMQ destinations - queues and topics, from either product.
 *
 * One table for both kinds rather than two pages. Their management surface is
 * identical: same attributes, same operations, both browsable. What separates
 * them is delivery, and that shows on the subscriptions page - a queue has no
 * named subscribers and a topic does. RabbitMQ splits queues from exchanges
 * because an exchange holds no messages at all; a JMS topic does.
 *
 * Several columns are filled by one product only, and the board leaves the
 * other blank rather than printing a zero. Classic counts producers per
 * destination and Artemis does not; Artemis reports messages waiting for a
 * scheduled delivery time and Classic keeps that count on a broker-wide
 * scheduler instead. A dash means the broker was not asked to guess.
 */
export function DestinationsActiveMQ() {
  const { t } = useTranslation();
  const state = useActiveMQDestinations();
  const [search, setSearch] = useState("");
  const [kind, setKind] = useState<KindFilter>("all");
  const [selected, setSelected] = useState<string | null>(null);

  const destinations = useMemo(
    () => (state.data ?? []).map(readDestination),
    [state.data],
  );

  const shown = useMemo(() => {
    const needle = search.trim().toLowerCase();
    return destinations.filter((entry) => {
      if (kind !== "all" && entry.kind !== kind) return false;
      return needle === "" || entry.name.toLowerCase().includes(needle);
    });
  }, [destinations, search, kind]);

  const detail = useMemo(
    () => shown.find((entry) => entry.name === selected) ?? shown[0] ?? null,
    [shown, selected],
  );

  // Which product answered decides what the table can show, and every row on
  // one connection comes from the same broker.
  const product = destinations[0]?.product ?? null;

  return (
    <Page>
      <PageHeader
        title={t("shell.nav.activemq.topics")}
        subtitle={t("board.activemq.destinations.count", {
          queues: destinations.filter((entry) => entry.kind === "queue").length,
          topics: destinations.filter((entry) => entry.kind === "topic").length,
        })}
        actions={
          <div style={{ display: "flex", gap: "8px", alignItems: "center" }}>
            <ToggleGroup
              type="single"
              variant="outline"
              size="sm"
              value={kind}
              // A cleared selection would show nothing at all, so an
              // unset value falls back to showing everything.
              onValueChange={(next: string) => setKind((next as KindFilter) || "all")}
            >
              {(["all", "queue", "topic"] as const).map((option) => (
                <ToggleGroupItem key={option} value={option} className="text-[11px]">
                  {t(`board.activemq.destinations.kind.${option}`)}
                </ToggleGroupItem>
              ))}
            </ToggleGroup>
            <Input
              style={{ width: "200px" }}
              value={search}
              placeholder={t("board.activemq.destinations.search")}
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
                    <TableHead>{t("board.activemq.destinations.name")}</TableHead>
                    <TableHead>{t("board.activemq.destinations.kindColumn")}</TableHead>
                    <TableHead className="num">
                      {t("board.activemq.destinations.depth")}
                    </TableHead>
                    <TableHead className="num">
                      {t("board.activemq.destinations.subscribers")}
                    </TableHead>
                    <TableHead className="num">
                      {t("board.activemq.destinations.inFlight")}
                    </TableHead>
                    <TableHead className="num">
                      {t("board.activemq.destinations.enqueued")}
                    </TableHead>
                    <TableHead className="num">
                      {t("board.activemq.destinations.dequeued")}
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
                          {entry.name}
                        </span>
                        {entry.isDeadLetter && (
                          <span className="pb pAMQ" style={{ marginLeft: "6px" }}>
                            {t("board.activemq.destinations.deadLetterBadge")}
                          </span>
                        )}
                        {entry.paused === true && (
                          <span className="pb pRDS" style={{ marginLeft: "6px" }}>
                            {t("board.activemq.destinations.pausedBadge")}
                          </span>
                        )}
                      </TableCell>
                      <TableCell>
                        {t(`board.activemq.destinations.kind.${entry.kind}`)}
                      </TableCell>
                      <TableCell className="num">
                        <Cell value={entry.depth} />
                      </TableCell>
                      <TableCell className="num">
                        <Cell value={entry.subscribers} />
                      </TableCell>
                      <TableCell className="num">
                        <Cell value={entry.inFlight} />
                      </TableCell>
                      <TableCell className="num">
                        <Cell value={entry.enqueued} />
                      </TableCell>
                      <TableCell className="num">
                        <Cell value={entry.dequeued} />
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </Panel>

            {detail != null && (
              <Panel style={{ width: "300px", flex: "none", overflow: "auto" }}>
                <PanelHeader title={detail.name} />
                <div style={{ padding: "0 12px 12px" }}>
                  <SectionLabel>{t("board.activemq.destinations.section.state")}</SectionLabel>
                  <KV
                    rows={[
                      [
                        t("board.activemq.destinations.kindColumn"),
                        t(`board.activemq.destinations.kind.${detail.kind}`),
                      ],
                      [t("board.activemq.destinations.depth"), count(detail.depth)],
                      [
                        t("board.activemq.destinations.bytes"),
                        detail.bytes == null ? DASH : formatBytes(detail.bytes),
                      ],
                      [t("board.activemq.destinations.scheduled"), count(detail.scheduled)],
                      [t("board.activemq.destinations.expired"), count(detail.expired)],
                    ]}
                  />

                  {/*
                    Artemis's two levels, shown only where they exist. Classic
                    addresses a destination directly and has no address above
                    it, so drawing these rows would invent a concept.
                  */}
                  {detail.product === "artemis" && (
                    <>
                      <SectionLabel>
                        {t("board.activemq.destinations.section.routing")}
                      </SectionLabel>
                      <KV
                        rows={[
                          [t("board.activemq.destinations.address"), detail.address ?? DASH],
                          [
                            t("board.activemq.destinations.routingTypes"),
                            detail.routingTypes ?? DASH,
                          ],
                          [t("board.activemq.destinations.queueCount"), count(detail.queueCount)],
                          [
                            t("board.activemq.destinations.deadLetterAddress"),
                            detail.deadLetterAddress ?? DASH,
                          ],
                          [
                            t("board.activemq.destinations.expiryAddress"),
                            detail.expiryAddress ?? DASH,
                          ],
                          [t("board.activemq.destinations.filter"), detail.filter ?? DASH],
                        ]}
                      />
                    </>
                  )}

                  {detail.product === "classic" && (
                    <>
                      <SectionLabel>
                        {t("board.activemq.destinations.section.classic")}
                      </SectionLabel>
                      <KV
                        rows={[
                          [t("board.activemq.destinations.producers"), count(detail.producers)],
                          [t("board.activemq.destinations.dispatched"), count(detail.dispatched)],
                          [
                            t("board.activemq.destinations.memoryPercent"),
                            detail.memoryPercent == null ? DASH : `${detail.memoryPercent}%`,
                          ],
                          [
                            t("board.activemq.destinations.averageSize"),
                            detail.averageMessageSize == null
                              ? DASH
                              : formatBytes(detail.averageMessageSize),
                          ],
                        ]}
                      />
                    </>
                  )}

                  {/*
                    Said here rather than on the message board alone, because
                    this is where a reader learns the destination is deeper
                    than a browse can reach - and the number they are looking
                    at is the one that will not fit.
                  */}
                  {browseWillBeCapped(detail) && (
                    <p
                      style={{
                        marginTop: "10px",
                        fontSize: "11px",
                        color: "var(--c-muted)",
                      }}
                    >
                      {t("board.activemq.destinations.browseCapped", {
                        cap: detail.browseCap,
                      })}
                    </p>
                  )}
                </div>
              </Panel>
            )}
          </div>
          {product != null && (
            <p
              style={{
                margin: "0 20px",
                fontSize: "11.5px",
                color: "var(--c-muted)",
                flex: "none",
              }}
            >
              {t(`board.activemq.destinations.note.${product}`)}
            </p>
          )}
        </PageBody>
      </BoardState>
    </Page>
  );
}
