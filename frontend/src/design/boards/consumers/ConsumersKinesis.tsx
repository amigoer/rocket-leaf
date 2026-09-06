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
import { useKinesisConsumers } from "@/hooks/kinesis/useKinesisConsumers";
import {
  consumer as readConsumer,
  settling,
  type KinesisConsumer,
} from "@/mq/kinesis/subscriptions";
import { formatMessageTime } from "@/lib/time";
import { formatErrorMessage } from "@/lib/utils";
import { useConnectionScope } from "@/mq/ConnectionScope";
import * as kinesisApi from "@/api/kinesis";
import { ConsumerDialogKinesis } from "./ConsumerDialogKinesis";

const MONO11 = { fontSize: "11px" } as const;

/** A dash, not a zero: an absent figure and a measured zero are not alike. */
const DASH = "—";

/**
 * Kinesis registered consumers.
 *
 * The page reads only enhanced fan-out consumers, because they are the only
 * readers a stream knows about. Registering one reserves a name and a
 * dedicated two megabytes a second of read throughput on every shard; an
 * application then subscribes against its ARN. Everything else that reads a
 * Kinesis stream - the KCL, a Lambda event source, anything calling GetRecords
 * - registers nothing at all, so it cannot be listed here and its absence is
 * the service's answer rather than a gap.
 *
 * There is no backlog column, and its absence is the honest answer rather than
 * a hole. A registered consumer carries no position: a classic consumer keeps
 * one in a DynamoDB table the KCL owns, and this kind keeps none. The
 * connection says so through a degraded capability, and the note under the
 * table says it in words.
 */
export function ConsumersKinesis() {
  const { t } = useTranslation();
  const state = useKinesisConsumers();
  const { id: connID } = useConnectionScope();
  const confirm = useConfirm();
  const [search, setSearch] = useState("");
  const [selected, setSelected] = useState<string | null>(null);
  const [formOpen, setFormOpen] = useState(false);

  const consumers = useMemo(() => (state.data ?? []).map(readConsumer), [state.data]);

  const shown = useMemo(() => {
    const needle = search.trim().toLowerCase();
    return consumers.filter(
      (entry) =>
        needle === "" ||
        entry.name.toLowerCase().includes(needle) ||
        entry.stream.toLowerCase().includes(needle),
    );
  }, [consumers, search]);

  // Keyed by the pair, because a name is unique only within its stream.
  const keyOf = (entry: KinesisConsumer) => `${entry.stream}/${entry.name}`;
  const detail = useMemo(
    () => shown.find((entry) => keyOf(entry) === selected) ?? shown[0] ?? null,
    [shown, selected],
  );

  const register = useCallback(
    async (stream: string, name: string) => {
      await kinesisApi.registerConsumer(connID, stream, name);
      toast.success(t("board.kinesis.consumers.registered", { name }));
      await state.refresh();
    },
    [connID, state, t],
  );

  const deregister = useCallback(
    async (entry: KinesisConsumer) => {
      const ok = await confirm({
        title: t("board.kinesis.consumers.deregisterTitle", { name: entry.name }),
        description: t("board.kinesis.consumers.deregisterDesc"),
        confirmLabel: t("board.kinesis.consumers.deregisterAction"),
        danger: true,
      });
      if (!ok) return;
      try {
        await kinesisApi.deregisterConsumer(connID, entry.stream, entry.name);
        toast.success(t("board.kinesis.consumers.deregistered", { name: entry.name }));
        setSelected(null);
        await state.refresh();
      } catch (removeError) {
        toast.error(t("board.kinesis.consumers.deregisterFailed"), {
          description: formatErrorMessage(removeError),
        });
      }
    },
    [confirm, connID, state, t],
  );

  return (
    <Page>
      <PageHeader
        title={t("shell.nav.kinesis.consumers")}
        subtitle={t("board.kinesis.consumers.count", { count: consumers.length })}
        actions={
          <div style={{ display: "flex", gap: "8px", alignItems: "center" }}>
            <Input
              style={{ width: "200px" }}
              value={search}
              placeholder={t("board.kinesis.consumers.search")}
              onChange={(event) => setSearch(event.target.value)}
            />
            <Button size="sm" onClick={() => setFormOpen(true)}>
              {t("board.kinesis.consumers.register")}
            </Button>
            <RefreshButton
              refreshing={state.refreshing}
              online={state.online}
              onClick={() => void state.refresh()}
            />
          </div>
        }
      />
      <ConsumerDialogKinesis open={formOpen} onOpenChange={setFormOpen} onSubmit={register} />
      <BoardState state={state}>
        <PageBody>
          <div style={{ display: "flex", gap: "12px", minHeight: 0, flex: 1 }}>
            <Panel style={{ flex: 1, overflow: "auto" }}>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t("board.kinesis.consumers.name")}</TableHead>
                    <TableHead>{t("board.kinesis.consumers.stream")}</TableHead>
                    <TableHead>{t("board.kinesis.consumers.status")}</TableHead>
                    <TableHead>{t("board.kinesis.consumers.registeredAt")}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {shown.map((entry) => (
                    <TableRow
                      key={keyOf(entry)}
                      data-state={detail != null && keyOf(detail) === keyOf(entry) ? "selected" : undefined}
                      onClick={() => setSelected(keyOf(entry))}
                      style={{ cursor: "pointer" }}
                    >
                      <TableCell>
                        <span className="mono3" style={MONO11}>
                          {entry.name}
                        </span>
                      </TableCell>
                      <TableCell>
                        <span className="mono3" style={MONO11}>
                          {entry.stream}
                        </span>
                      </TableCell>
                      <TableCell>
                        <span
                          className="mono3"
                          style={{
                            ...MONO11,
                            color: settling(entry) ? "var(--c-warn)" : undefined,
                          }}
                        >
                          {entry.status ?? DASH}
                        </span>
                      </TableCell>
                      <TableCell>
                        <span className="mono3" style={MONO11}>
                          {formatMessageTime(entry.registeredAtMs)}
                        </span>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </Panel>

            {detail != null && (
              <ConsumerDetail entry={detail} onDeregister={() => void deregister(detail)} />
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
            {t("board.kinesis.consumers.note")}
          </p>
        </PageBody>
      </BoardState>
    </Page>
  );
}

function ConsumerDetail({
  entry,
  onDeregister,
}: {
  entry: KinesisConsumer;
  onDeregister: () => void;
}) {
  const { t } = useTranslation();
  return (
    <Panel style={{ width: "320px", flex: "none", overflow: "auto" }}>
      <PanelHeader title={entry.name} />
      <div style={{ padding: "0 12px 12px" }}>
        <SectionLabel>{t("board.kinesis.consumers.section.registration")}</SectionLabel>
        <KV
          rows={[
            [t("board.kinesis.consumers.stream"), entry.stream],
            [t("board.kinesis.consumers.status"), entry.status ?? DASH],
            [
              t("board.kinesis.consumers.registeredAt"),
              formatMessageTime(entry.registeredAtMs),
            ],
            [t("board.kinesis.consumers.arn"), entry.arn ?? DASH],
          ]}
        />

        <div style={{ display: "flex", gap: "6px", marginTop: "12px" }}>
          <Button size="sm" variant="destructive" onClick={onDeregister}>
            {t("board.kinesis.consumers.deregisterAction")}
          </Button>
        </div>

        {/* Said where the row is: an application pointed at a consumer that is
            not ACTIVE fails with an error about the consumer, not the stream. */}
        {settling(entry) && (
          <p style={{ marginTop: "10px", fontSize: "11px", color: "var(--c-warn)" }}>
            {t("board.kinesis.consumers.settling", { status: entry.status })}
          </p>
        )}
      </div>
    </Panel>
  );
}
