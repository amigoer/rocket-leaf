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
import { formatErrorMessage } from "@/lib/utils";
import { isEphemeral, nameProblem, submittableName } from "@/mq/nsq/names";

/**
 * A topic and a name, and nothing else.
 *
 * The topic is asked for rather than inferred because it is half the channel's
 * identity: "analytics" under two topics is two channels with separate
 * backlogs, and nsqd will not tell them apart for you.
 *
 * There is no starting position, and there could not be one. What a new
 * channel gets is whatever nsqd is still holding: on a topic that already has
 * a channel, nothing, because the copies were made as the messages arrived; on
 * a topic with none, the queue it has been holding on their behalf. Worth
 * saying on the form, because every other family in this app would let a new
 * group start from the beginning of the log.
 */
export function ChannelDialogNsq({
  open,
  onOpenChange,
  onCreate,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onCreate: (topic: string, channel: string) => Promise<void>;
}) {
  const { t } = useTranslation();
  const [topic, setTopic] = useState("");
  const [name, setName] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (open) {
      setTopic("");
      setName("");
      setError(null);
    }
  }, [open]);

  const topicProblem = topic === "" ? null : nameProblem(topic);
  const nameFault = name === "" ? null : nameProblem(name);
  const submittedTopic = submittableName(topic);
  const submittedName = submittableName(name);

  const submit = async () => {
    if (submittedTopic == null || submittedName == null || busy) return;
    setBusy(true);
    setError(null);
    try {
      await onCreate(submittedTopic, submittedName);
      onOpenChange(false);
    } catch (createError) {
      setError(formatErrorMessage(createError));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("board.nsq.channels.createTitle")}</DialogTitle>
        </DialogHeader>
        <FieldGroup>
          <Field>
            <FieldLabel htmlFor="nsq-channel-topic">{t("board.nsq.channels.topic")}</FieldLabel>
            <Input
              id="nsq-channel-topic"
              value={topic}
              placeholder="orders.created"
              onChange={(event) => setTopic(event.target.value)}
            />
            <FieldDescription>
              {topicProblem != null
                ? t(`board.nsq.topics.nameProblem.${topicProblem}`)
                : t("board.nsq.channels.topicHint")}
            </FieldDescription>
          </Field>
          <Field>
            <FieldLabel htmlFor="nsq-channel-name">{t("board.nsq.channels.name")}</FieldLabel>
            <Input
              id="nsq-channel-name"
              value={name}
              placeholder="analytics"
              onChange={(event) => setName(event.target.value)}
              onKeyDown={(event) => {
                if (event.key === "Enter") void submit();
              }}
            />
            <FieldDescription>
              {nameFault != null
                ? t(`board.nsq.topics.nameProblem.${nameFault}`)
                : isEphemeral(name)
                  ? t("board.nsq.channels.nameEphemeral")
                  : t("board.nsq.channels.nameHint")}
            </FieldDescription>
          </Field>
        </FieldGroup>
        {error != null && (
          <FieldDescription style={{ color: "var(--c-danger)" }}>{error}</FieldDescription>
        )}
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t("common.cancel")}
          </Button>
          <Button
            disabled={submittedTopic == null || submittedName == null || busy}
            onClick={() => void submit()}
          >
            {busy && <Spinner />}
            {t("common.confirm")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
