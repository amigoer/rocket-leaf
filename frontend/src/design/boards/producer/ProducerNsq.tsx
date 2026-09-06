import { useCallback, useState } from "react";
import { useTranslation } from "react-i18next";
import { Send } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Field, FieldDescription, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Spinner } from "@/components/ui/spinner";
import { Panel, PanelHeader, SectionLabel, SelectField, toast } from "@/components";
import { Page, PageBody, PageHeader } from "@/design/shell";
import { BoardState } from "@/design/boards/BoardState";
import { useConnectionScope } from "@/mq/ConnectionScope";
import { useNsqDestinations } from "@/hooks/nsq/useNsqDestinations";
import { useNsqNodes } from "@/hooks/nsq/useNsqNodes";
import {
  DEFAULT_MAX_DELAY_SEC,
  MAX_COUNT,
  emptyNsqProducerDraft,
  sendProblem,
  sendWarning,
  sendsOneAtATime,
  toPublishInput,
  type NsqProducerDraft,
} from "@/mq/nsq/publish";
import { formatErrorMessage } from "@/lib/utils";
import * as nsqApi from "@/api/nsq";

/**
 * The NSQ send console.
 *
 * Its own board rather than the shared one, because the shared one collects a
 * topic, tags, keys and a delay level - RocketMQ's vocabulary, of which only
 * the topic and the delay mean anything here. An NSQ message is bytes: there
 * is no key, no header table and no property map anywhere in the protocol, so
 * a tag field would collect something that could not be sent.
 *
 * What it has instead is the field no other console here needs: which nsqd
 * takes the message. A topic lives on the daemon it was created on, so the
 * choice decides who can consume what is sent - a consumer attached to another
 * daemon sees it only if it also finds this one through nsqlookupd.
 *
 * The delay ceiling is the daemon's rather than this form's, and the form says
 * so rather than pretending to enforce it. --max-req-timeout is one hour by
 * default and is not readable over the API, so a delay past it is a warning
 * and the send still goes: a deployment started with a longer one has to stay
 * usable, and the daemon has the last word either way.
 */
export function ProducerNsq() {
  const { t } = useTranslation();
  const { id: connID } = useConnectionScope();

  const topics = useNsqDestinations();
  const nodes = useNsqNodes();
  const [draft, setDraft] = useState<NsqProducerDraft>(emptyNsqProducerDraft);
  const [sending, setSending] = useState(false);
  const [sent, setSent] = useState<string | null>(null);

  const set = <K extends keyof NsqProducerDraft>(key: K, value: NsqProducerDraft[K]) =>
    setDraft((previous) => ({ ...previous, [key]: value }));

  const problem = sendProblem(draft);
  const warning = sendWarning(draft);

  const send = useCallback(async () => {
    const input = toPublishInput(draft);
    if (input == null || sending) return;
    setSending(true);
    try {
      const result = await nsqApi.publish(connID, input);
      setSent(t("board.nsq.producer.sentTo", { count: result.sent, node: result.node }));
      toast.success(t("board.nsq.producer.sent", { count: result.sent }));
      await topics.refresh();
    } catch (sendError) {
      toast.error(t("board.nsq.producer.failed"), {
        description: formatErrorMessage(sendError),
      });
    } finally {
      setSending(false);
    }
  }, [connID, draft, sending, t, topics]);

  const topicOptions = (topics.data ?? []).map((entry) => ({
    value: entry.ref.name,
    label: entry.ref.name,
  }));
  const nodeOptions = [
    { value: "", label: t("board.nsq.producer.firstNode") },
    ...(nodes.data ?? []).map((address) => ({ value: address, label: address })),
  ];

  return (
    <Page>
      <PageHeader
        title={t("shell.nav.nsq.producer")}
        subtitle={t("board.nsq.producer.subtitle")}
      />
      <BoardState state={topics}>
        <PageBody>
          <Panel style={{ maxWidth: "760px" }}>
            <PanelHeader title={t("board.nsq.producer.title")} />
            <div style={{ padding: "0 12px 12px" }}>
              <FieldGroup>
                <Field>
                  <FieldLabel htmlFor="nsq-send-topic">
                    {t("board.nsq.producer.topic")}
                  </FieldLabel>
                  {topicOptions.length > 0 ? (
                    <SelectField
                      value={draft.topic}
                      options={topicOptions}
                      onValueChange={(next) => set("topic", next)}
                    />
                  ) : (
                    <Input
                      id="nsq-send-topic"
                      value={draft.topic}
                      placeholder="orders.created"
                      onChange={(event) => set("topic", event.target.value)}
                    />
                  )}
                  <FieldDescription>{t("board.nsq.producer.topicHint")}</FieldDescription>
                </Field>

                <Field>
                  <FieldLabel htmlFor="nsq-send-body">{t("board.nsq.producer.body")}</FieldLabel>
                  <Textarea
                    id="nsq-send-body"
                    rows={8}
                    value={draft.body}
                    onChange={(event) => set("body", event.target.value)}
                  />
                  <FieldDescription>{t("board.nsq.producer.bodyHint")}</FieldDescription>
                </Field>

                <SectionLabel>{t("board.nsq.producer.section.delivery")}</SectionLabel>

                <Field>
                  <FieldLabel htmlFor="nsq-send-node">{t("board.nsq.producer.node")}</FieldLabel>
                  <SelectField
                    value={draft.node}
                    options={nodeOptions}
                    onValueChange={(next) => set("node", next)}
                  />
                  <FieldDescription>{t("board.nsq.producer.nodeHint")}</FieldDescription>
                </Field>

                <Field>
                  <FieldLabel htmlFor="nsq-send-delay">
                    {t("board.nsq.producer.delay")}
                  </FieldLabel>
                  <Input
                    id="nsq-send-delay"
                    type="number"
                    value={String(draft.delaySec)}
                    onChange={(event) =>
                      set("delaySec", Number.parseInt(event.target.value, 10) || 0)
                    }
                  />
                  <FieldDescription>
                    {warning != null
                      ? t(warning, { max: DEFAULT_MAX_DELAY_SEC })
                      : t("board.nsq.producer.delayHint", { max: DEFAULT_MAX_DELAY_SEC })}
                  </FieldDescription>
                </Field>

                <Field>
                  <FieldLabel htmlFor="nsq-send-count">
                    {t("board.nsq.producer.count")}
                  </FieldLabel>
                  <Input
                    id="nsq-send-count"
                    type="number"
                    value={String(draft.count)}
                    onChange={(event) =>
                      set("count", Number.parseInt(event.target.value, 10) || 0)
                    }
                  />
                  <FieldDescription>
                    {sendsOneAtATime(draft)
                      ? t("board.nsq.producer.oneAtATime", { count: draft.count })
                      : t("board.nsq.producer.countHint", { max: MAX_COUNT })}
                  </FieldDescription>
                </Field>
              </FieldGroup>

              <div style={{ display: "flex", gap: "8px", alignItems: "center", marginTop: "12px" }}>
                <Button disabled={problem != null || sending} onClick={() => void send()}>
                  {sending ? <Spinner /> : <Send size={13} aria-hidden />}
                  {t("board.nsq.producer.send")}
                </Button>
                {problem != null && (
                  <span style={{ fontSize: "11.5px", color: "var(--c-muted)" }}>{t(problem)}</span>
                )}
                {problem == null && sent != null && (
                  <span style={{ fontSize: "11.5px", color: "var(--c-muted)" }}>{sent}</span>
                )}
              </div>
            </div>
          </Panel>
        </PageBody>
      </BoardState>
    </Page>
  );
}
