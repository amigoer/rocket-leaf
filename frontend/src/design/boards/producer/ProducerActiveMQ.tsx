import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { Page, PageBody, PageHeader } from "@/design/shell";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";
import { Field, FieldDescription, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Panel, PanelHeader, SectionLabel, SelectField } from "@/components";
import { useActiveMQDestinations } from "@/hooks/activemq/useActiveMQDestinations";
import { destination as readDestination } from "@/mq/activemq/destinations";
import {
  emptyActiveMQProducerDraft,
  parseHeaders,
  toPublishInput,
  type ActiveMQProducerDraft,
} from "./producerActiveMQDraft";
import * as activemqApi from "@/api/activemq";
import { useConnectionScope } from "@/mq/ConnectionScope";
import { formatErrorMessage } from "@/lib/utils";

const MONO = { fontFamily: "var(--f-mono)", fontSize: "11.5px" } as const;

/**
 * The ActiveMQ send console.
 *
 * Sending is a management operation on both products, so this works against a
 * broker with every wire acceptor switched off - which is unusual, and is the
 * same reason the message page needs no client.
 *
 * Three things the form deliberately does not offer.
 *
 * There is no delay. Both products have scheduled delivery and neither
 * management operation can express it: the annotations have to be a Long and
 * both send operations take Map<String,String>, so a delay set here would be
 * accepted, ignored, and reported as having worked.
 *
 * There is no binary body. Both operations take a String, so bytes would need
 * the optional AMQP tier.
 *
 * And the persistent switch is Artemis-only, which the hint says rather than
 * leaving it looking broken: Classic's sendTextMessage has no delivery-mode
 * parameter and the destination's own policy decides.
 */
export function ProducerActiveMQ() {
  const { t } = useTranslation();
  const { id: connID } = useConnectionScope();
  const destinationState = useActiveMQDestinations();
  const [draft, setDraft] = useState<ActiveMQProducerDraft>(emptyActiveMQProducerDraft);
  const [busy, setBusy] = useState(false);

  const set = <K extends keyof ActiveMQProducerDraft>(
    key: K,
    value: ActiveMQProducerDraft[K],
  ) => setDraft((current) => ({ ...current, [key]: value }));

  const destinations = useMemo(
    () => (destinationState.data ?? []).map(readDestination),
    [destinationState.data],
  );
  const product = destinations[0]?.product ?? null;

  // Artemis sends through a queue: a multicast address has no queue MBean of
  // its own, so offering one would produce an error the form could prevent.
  const options = useMemo(
    () =>
      destinations
        .filter((entry) => product !== "artemis" || entry.kind === "queue")
        .map((entry) => ({ value: entry.name, label: entry.name })),
    [destinations, product],
  );

  const parsed = parseHeaders(draft.headers);
  const badLine = "badLine" in parsed ? parsed.badLine : null;
  const input = toPublishInput(draft);

  const send = async () => {
    if (input == null || busy) return;
    setBusy(true);
    try {
      const result = await activemqApi.publish(connID, input);
      if (result.unroutable > 0) {
        toast.warning(
          t("board.activemq.producer.partial", {
            sent: result.sent,
            refused: result.unroutable,
          }),
          { description: result.reason },
        );
      } else {
        toast.success(t("board.activemq.producer.sent", { count: result.sent }));
      }
    } catch (sendError) {
      toast.error(t("board.activemq.producer.failed"), {
        description: formatErrorMessage(sendError),
      });
    } finally {
      setBusy(false);
    }
  };

  return (
    <Page>
      <PageHeader title={t("shell.nav.activemq.producer")} />
      <PageBody>
        <Panel style={{ flex: 1, overflow: "auto" }}>
          <PanelHeader title={t("board.activemq.producer.title")} />
          <div style={{ padding: "0 12px 12px", maxWidth: "620px" }}>
            <FieldGroup>
              <Field>
                <FieldLabel>{t("board.activemq.producer.destination")}</FieldLabel>
                {/*
                  Unselected until the user picks, unlike the message board's
                  equivalent - that one browses and a default costs nothing,
                  this one sends. The placeholder is what keeps an empty
                  control reading as a choice rather than as a broken field.
                */}
                <SelectField
                  value={draft.destination}
                  options={options}
                  placeholder={t("board.activemq.producer.destinationPlaceholder")}
                  onValueChange={(next) => set("destination", next)}
                />
                {product === "artemis" && (
                  <FieldDescription>
                    {t("board.activemq.producer.artemisQueuesOnly")}
                  </FieldDescription>
                )}
              </Field>

              <Field>
                <FieldLabel>{t("board.activemq.producer.body")}</FieldLabel>
                <Textarea
                  style={MONO}
                  rows={8}
                  value={draft.body}
                  onChange={(event) => set("body", event.target.value)}
                />
                <FieldDescription>{t("board.activemq.producer.textOnly")}</FieldDescription>
              </Field>

              <SectionLabel>{t("board.activemq.producer.section.headers")}</SectionLabel>
              <Field>
                <FieldLabel>{t("board.activemq.producer.headers")}</FieldLabel>
                <Textarea
                  style={MONO}
                  rows={4}
                  placeholder={"tenant: acme\nattempt: 3"}
                  value={draft.headers}
                  onChange={(event) => set("headers", event.target.value)}
                />
                <FieldDescription style={badLine != null ? { color: "var(--c-danger)" } : undefined}>
                  {badLine != null
                    ? t("board.activemq.producer.badHeader", { line: badLine })
                    : t("board.activemq.producer.headersHint")}
                </FieldDescription>
              </Field>

              <Field>
                <FieldLabel>{t("board.activemq.producer.correlationId")}</FieldLabel>
                <Input
                  className="mono3"
                  value={draft.correlationId}
                  onChange={(event) => set("correlationId", event.target.value)}
                />
              </Field>
              <Field>
                <FieldLabel>{t("board.activemq.producer.replyTo")}</FieldLabel>
                <Input
                  className="mono3"
                  value={draft.replyTo}
                  onChange={(event) => set("replyTo", event.target.value)}
                />
              </Field>
              <Field>
                <FieldLabel>{t("board.activemq.producer.jmsType")}</FieldLabel>
                <Input
                  className="mono3"
                  value={draft.jmsType}
                  onChange={(event) => set("jmsType", event.target.value)}
                />
              </Field>

              <SectionLabel>{t("board.activemq.producer.section.delivery")}</SectionLabel>
              <Field>
                <FieldLabel>{t("board.activemq.producer.persistent")}</FieldLabel>
                <Switch
                  checked={draft.persistent}
                  disabled={product === "classic"}
                  onCheckedChange={(next: boolean) => set("persistent", next)}
                />
                <FieldDescription>
                  {product === "classic"
                    ? t("board.activemq.producer.persistentClassic")
                    : t("board.activemq.producer.persistentHint")}
                </FieldDescription>
              </Field>
              <Field>
                <FieldLabel>{t("board.activemq.producer.priority")}</FieldLabel>
                <Input
                  type="number"
                  value={draft.priority}
                  onChange={(event) => set("priority", Number(event.target.value))}
                />
              </Field>
              <Field>
                <FieldLabel>{t("board.activemq.producer.count")}</FieldLabel>
                <Input
                  type="number"
                  value={draft.count}
                  onChange={(event) => set("count", Number(event.target.value))}
                />
                <FieldDescription>{t("board.activemq.producer.countHint")}</FieldDescription>
              </Field>
            </FieldGroup>

            <Button
              style={{ marginTop: "12px" }}
              disabled={input == null || busy}
              onClick={() => void send()}
            >
              {t("board.activemq.producer.send")}
            </Button>
          </div>
        </Panel>
      </PageBody>
    </Page>
  );
}
