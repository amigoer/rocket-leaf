import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Field, FieldDescription, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Spinner } from "@/components/ui/spinner";
import { SelectField } from "@/components";
import { formatErrorMessage } from "@/lib/utils";
import { topicProblem } from "@/mq/solace/routing";

/**
 * Add one topic subscription to one queue.
 *
 * Two fields, and that is the whole of a Solace binding: there is no exchange
 * between a topic and a queue, so the source, the routing key and the handle
 * are one string. RabbitMQ's dialog collects four because its topology has an
 * object in the middle; this one has nothing to put there.
 *
 * The topic is checked before it is sent, because the broker will not check it
 * for you. Solace's wildcards look like a glob and are positional - "*" is one
 * whole level or the tail of one, ">" is the rest of the topic and only ever
 * the last character - and a pattern that breaks either is accepted as a
 * literal topic name and then matches nothing. That subscription is there, it
 * looks right on every listing, and it is dead.
 */
export function SubscriptionDialogSolace({
  open,
  queues,
  onOpenChange,
  onSubmit,
}: {
  open: boolean;
  queues: string[];
  onOpenChange: (open: boolean) => void;
  onSubmit: (queue: string, topic: string) => Promise<void>;
}) {
  const { t } = useTranslation();
  const [queue, setQueue] = useState("");
  const [topic, setTopic] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (open) {
      setQueue("");
      setTopic("");
      setError(null);
    }
  }, [open]);

  const trimmed = topic.trim();
  const problem = trimmed === "" ? null : topicProblem(trimmed);
  const blocked = queue === "" || trimmed === "" || problem != null;

  const submit = async () => {
    if (blocked || busy) return;
    setBusy(true);
    setError(null);
    try {
      await onSubmit(queue, trimmed);
      onOpenChange(false);
    } catch (submitError) {
      setError(formatErrorMessage(submitError));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("board.solace.routing.subscribeTitle")}</DialogTitle>
        </DialogHeader>
        <FieldGroup>
          <Field>
            <FieldLabel htmlFor="solace-sub-queue">{t("board.solace.routing.queue")}</FieldLabel>
            <SelectField<string>
              value={queue}
              options={queues.map((name) => ({ value: name, label: name }))}
              placeholder={t("board.solace.routing.pickQueue")}
              onValueChange={setQueue}
            />
            <FieldDescription>{t("board.solace.routing.queueHint")}</FieldDescription>
          </Field>

          <Field>
            <FieldLabel htmlFor="solace-sub-topic">{t("board.solace.routing.topic")}</FieldLabel>
            <Input
              id="solace-sub-topic"
              className="mono3"
              value={topic}
              placeholder="orders/eu/>"
              onChange={(event) => setTopic(event.target.value)}
            />
            <FieldDescription>
              {problem != null
                ? t(`board.solace.routing.topicProblem.${problem}`)
                : t("board.solace.routing.topicHint")}
            </FieldDescription>
          </Field>

          {error != null && (
            <FieldDescription style={{ color: "var(--c-danger)" }}>{error}</FieldDescription>
          )}
          <FieldDescription>{t("board.solace.routing.addNote")}</FieldDescription>
        </FieldGroup>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t("common.cancel")}
          </Button>
          <Button disabled={blocked || busy} onClick={() => void submit()}>
            {busy && <Spinner />}
            {t("board.solace.routing.subscribe")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
