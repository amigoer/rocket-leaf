import { useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { ListArea, ListPane, Page, PageHeader, Toolbar } from "@/design/shell";
import { Button } from "@/components/ui/button";
import { Spinner } from "@/components/ui/spinner";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  DetailPanel,
  DetailPanelBody,
  DetailPanelHeader,
  JsonBlock,
  KV,
  SectionLabel,
  SelectField,
  Status,
} from "@/components";
import { useKinesisBrowse } from "@/hooks/kinesis/useKinesisBrowse";
import { useKinesisDestinations } from "@/hooks/kinesis/useKinesisDestinations";
import { useKinesisShards } from "@/hooks/kinesis/useKinesisShards";
import { stream as readStream } from "@/mq/kinesis/destinations";
import {
  arrivedAtMs,
  encryptionOf,
  partitionKeyOf,
  recordIdOf,
  sequenceNumberOf,
  shardIdOf,
} from "@/mq/kinesis/messages";
import { formatMessageTime } from "@/lib/time";
import type { MessageItem } from "@bindings/model/models";

const MONO11 = { fontSize: "11px" } as const;
const DASH = "—";

/** How many records one browse asks for. */
const BROWSE_LIMIT = 100;

/** The option value for "every shard", which is what a stream browse means. */
const EVERY_SHARD = "";

function preview(message: MessageItem): string {
  const body = message.body ?? "";
  return body.length > 120 ? `${body.slice(0, 120)}…` : body;
}

/**
 * Browsing a Kinesis stream.
 *
 * A button rather than a list that loads itself, and not for SQS's reason.
 * Reading here takes nothing: GetRecords removes no record, hides none and
 * marks none, so any number of readers can read the same one until retention
 * expires. What a read spends is the shard's own allowance - five GetRecords a
 * second and two megabytes a second, shared with every classic consumer on it
 * - so a page that refreshed on a timer would be taking read capacity from a
 * running application for nobody's benefit.
 *
 * The shard picker is the control no other messages page has. A stream browse
 * reads every shard, which costs every shard's budget; naming one costs that
 * shard's alone, and is the only way to look at a closed parent on its own.
 *
 * There is no key search. A partition key is not indexed anywhere - it decides
 * the shard by its hash and is otherwise just a field - so narrowing by one
 * would mean reading everything and discarding most of it.
 */
export function MessagesKinesis() {
  const { t } = useTranslation();
  const streams = useKinesisDestinations();
  const browse = useKinesisBrowse();

  const names = useMemo(
    () => (streams.data ?? []).map((row) => readStream(row).name),
    [streams.data],
  );

  const [stream, setStream] = useState("");
  const [shard, setShard] = useState(EVERY_SHARD);
  const [selected, setSelected] = useState<string | null>(null);

  const chosen = stream !== "" ? stream : (names[0] ?? "");
  const shards = useKinesisShards(chosen === "" ? null : chosen);

  // A shard chosen on one stream means nothing on the next one.
  useEffect(() => {
    setShard(EVERY_SHARD);
  }, [chosen]);

  const panel = useMemo(
    () => browse.messages.find((message) => recordIdOf(message) === selected) ?? null,
    [browse.messages, selected],
  );

  return (
    <Page>
      <PageHeader
        title={t("board.kinesis.messages.title")}
        subtitle={t("board.kinesis.messages.subtitle")}
      />
      <Toolbar>
        <div style={{ width: "220px", flex: "none" }}>
          <SelectField<string>
            value={chosen}
            options={names.map((name) => ({ value: name, label: name }))}
            placeholder={t("board.kinesis.messages.pickStream")}
            onValueChange={setStream}
          />
        </div>
        <div style={{ width: "220px", flex: "none" }}>
          <SelectField<string>
            value={shard}
            options={[
              { value: EVERY_SHARD, label: t("board.kinesis.messages.everyShard") },
              ...shards.shards.map((entry) => ({
                value: entry.id,
                label: entry.closed
                  ? `${entry.id} (${t("board.kinesis.shards.closed")})`
                  : entry.id,
              })),
            ]}
            placeholder={t("board.kinesis.messages.everyShard")}
            onValueChange={setShard}
          />
        </div>
        <Button
          disabled={chosen === "" || browse.loading}
          onClick={() =>
            void browse.run({ stream: chosen, shard, limit: BROWSE_LIMIT, startTimeMs: 0 })
          }
        >
          {browse.loading && <Spinner className="size-3.5" />}
          {t("board.kinesis.messages.read")}
        </Button>
        <span className="flex-1" />
        <span style={{ fontSize: "11px", color: "var(--c-muted)" }}>
          {t("board.kinesis.messages.found", { count: browse.messages.length })}
        </span>
      </Toolbar>

      {/* Said before the button, and it is the opposite of what the other
          hosted families say here: nothing on this page is taken from anyone.
          What it costs is read capacity the stream's consumers share. */}
      <div
        style={{
          padding: "6px 12px",
          fontSize: "11px",
          color: "var(--c-muted)",
          borderBottom: "1px solid var(--c-border)",
        }}
      >
        {t("board.kinesis.messages.readNote")}
      </div>

      <ListArea>
        <ListPane>
          {browse.error != null ? (
            <div style={{ padding: "24px", fontSize: "11.5px", color: "var(--c-err)" }}>
              {browse.error}
            </div>
          ) : browse.messages.length === 0 ? (
            <div
              style={{
                padding: "24px",
                fontSize: "11.5px",
                color: "var(--c-muted)",
                textAlign: "center",
              }}
            >
              {browse.searched
                ? t("board.kinesis.messages.nothingThere")
                : t("board.kinesis.messages.startHere")}
            </div>
          ) : (
            <Table inset>
              <TableHeader>
                <TableRow>
                  <TableHead>{t("board.kinesis.messages.shard")}</TableHead>
                  <TableHead>{t("board.kinesis.messages.arrived")}</TableHead>
                  <TableHead>{t("board.kinesis.messages.partitionKey")}</TableHead>
                  <TableHead>{t("board.kinesis.messages.body")}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {browse.messages.map((message) => (
                  <TableRow
                    key={recordIdOf(message)}
                    selected={selected === recordIdOf(message)}
                    onClick={() => setSelected(recordIdOf(message))}
                  >
                    <TableCell className="mono3" style={MONO11}>
                      {shardIdOf(message) ?? DASH}
                    </TableCell>
                    <TableCell className="mono3" style={MONO11}>
                      {formatMessageTime(arrivedAtMs(message))}
                    </TableCell>
                    <TableCell className="mono3" style={MONO11}>
                      {partitionKeyOf(message) ?? DASH}
                    </TableCell>
                    <TableCell className="mono3" style={MONO11}>
                      {preview(message)}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </ListPane>

        {panel != null && (
          <DetailPanel width={420} onDismiss={() => setSelected(null)}>
            <DetailPanelHeader
              title={shardIdOf(panel) ?? recordIdOf(panel)}
              badge={
                <Status tone="off" style={{ fontSize: "10px" }}>
                  {panel.topic}
                </Status>
              }
              onClose={() => setSelected(null)}
            />
            <DetailPanelBody>
              <KV
                rows={[
                  [t("board.kinesis.messages.arrived"), formatMessageTime(arrivedAtMs(panel))],
                  [t("board.kinesis.messages.shard"), shardIdOf(panel) ?? DASH],
                  [
                    t("board.kinesis.messages.sequenceNumber"),
                    sequenceNumberOf(panel) ?? DASH,
                  ],
                  [t("board.kinesis.messages.partitionKey"), partitionKeyOf(panel) ?? DASH],
                  [t("board.kinesis.messages.encryption"), encryptionOf(panel) ?? DASH],
                ]}
              />

              {/* Spelled out because it is the field somebody will try to
                  paste somewhere: a sequence number addresses nothing on its
                  own, and every call that takes one takes the shard too. */}
              <SectionLabel>{t("board.kinesis.messages.section.address")}</SectionLabel>
              <KV rows={[[t("board.kinesis.messages.recordId"), recordIdOf(panel)]]} />
              <p style={{ margin: "6px 0 0", fontSize: "11px", color: "var(--c-muted)" }}>
                {t("board.kinesis.messages.addressNote")}
              </p>

              <SectionLabel>{t("board.kinesis.messages.body")}</SectionLabel>
              <JsonBlock>{panel.body}</JsonBlock>
            </DetailPanelBody>
          </DetailPanel>
        )}
      </ListArea>
    </Page>
  );
}
