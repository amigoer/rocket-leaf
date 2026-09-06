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
import { objectNameProblem, type IbmMqObjectKind } from "@/mq/ibmmq/names";
import type { IBMMQDestinationInput } from "@/api/ibmmq";

/** Local is the only type that stores anything, so it is the default. */
const QUEUE_TYPES = ["local", "alias", "remote", "model"] as const;

interface Draft {
  kind: IbmMqObjectKind;
  name: string;
  queueType: string;
  maxDepth: string;
  topicString: string;
  description: string;
}

const EMPTY: Draft = {
  kind: "queue",
  name: "",
  queueType: "local",
  maxDepth: "",
  topicString: "",
  description: "",
};

/**
 * Create one queue or one topic.
 *
 * One dialog for both, because a user should not have to know that the queue
 * manager takes them through two different interfaces - a queue is a REST
 * resource and a topic is an MQSC command. What they do have to know is that
 * the two are not variations on each other: the kind is the first field, and
 * everything below it changes with it.
 *
 * The topic string is required and is not the object's name. Publishers name
 * the string; the object is where that string's settings are attached, and two
 * objects covering overlapping strings is ordinary. Defaulting the string to
 * the name would quietly create an object nobody publishes through.
 *
 * There is no edit here, and that is deliberate rather than unfinished. ALTER
 * changes a live object underneath whatever has it open, and the fields worth
 * changing each have their own consequence for applications already connected,
 * so this driver reads them and offers no single control that writes them all.
 */
export function QueueDialogIbmMq({
  open,
  onOpenChange,
  onSubmit,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSubmit: (input: IBMMQDestinationInput) => Promise<void>;
}) {
  const { t } = useTranslation();
  const [draft, setDraft] = useState<Draft>(EMPTY);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (open) {
      setDraft(EMPTY);
      setError(null);
    }
  }, [open]);

  const set = <K extends keyof Draft>(key: K, next: Draft[K]) =>
    setDraft((current) => ({ ...current, [key]: next }));

  const name = draft.name.trim();
  const problem = name === "" ? null : objectNameProblem(name);
  const topicString = draft.topicString.trim();
  const blocked =
    name === "" || problem != null || (draft.kind === "topic" && topicString === "");

  const submit = async () => {
    if (blocked || busy) return;
    setBusy(true);
    setError(null);
    try {
      await onSubmit({
        name,
        kind: draft.kind,
        queueType: draft.kind === "queue" ? draft.queueType : "",
        maxDepth: draft.kind === "queue" ? Number.parseInt(draft.maxDepth, 10) || 0 : 0,
        topicString: draft.kind === "topic" ? topicString : "",
        description: draft.description.trim(),
      });
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
          <DialogTitle>{t("board.ibmmq.queues.createTitle")}</DialogTitle>
        </DialogHeader>
        <FieldGroup>
          <Field>
            <FieldLabel htmlFor="ibmmq-kind">{t("board.ibmmq.queues.kind")}</FieldLabel>
            <SelectField<IbmMqObjectKind>
              value={draft.kind}
              options={[
                { value: "queue", label: t("board.ibmmq.queues.kindQueue") },
                { value: "topic", label: t("board.ibmmq.queues.kindTopic") },
              ]}
              onValueChange={(next) => set("kind", next)}
            />
            <FieldDescription>{t("board.ibmmq.queues.kindHint")}</FieldDescription>
          </Field>

          <Field>
            <FieldLabel htmlFor="ibmmq-name">{t("board.ibmmq.queues.name")}</FieldLabel>
            <Input
              id="ibmmq-name"
              className="mono3"
              value={draft.name}
              placeholder="APP.ORDERS"
              onChange={(event) => set("name", event.target.value)}
            />
            <FieldDescription>
              {problem != null
                ? t(`board.ibmmq.queues.nameProblem.${problem}`)
                : t("board.ibmmq.queues.nameHint")}
            </FieldDescription>
          </Field>

          {draft.kind === "queue" ? (
            <>
              <Field>
                <FieldLabel htmlFor="ibmmq-queue-type">{t("board.ibmmq.queues.type")}</FieldLabel>
                <SelectField<string>
                  value={draft.queueType}
                  options={QUEUE_TYPES.map((value) => ({ value, label: value }))}
                  onValueChange={(next) => set("queueType", next)}
                />
                <FieldDescription>{t("board.ibmmq.queues.typeHint")}</FieldDescription>
              </Field>

              <Field>
                <FieldLabel htmlFor="ibmmq-max-depth">
                  {t("board.ibmmq.queues.maxDepth")}
                </FieldLabel>
                <Input
                  id="ibmmq-max-depth"
                  type="number"
                  value={draft.maxDepth}
                  placeholder="5000"
                  onChange={(event) => set("maxDepth", event.target.value)}
                />
                <FieldDescription>{t("board.ibmmq.queues.maxDepthHint")}</FieldDescription>
              </Field>
            </>
          ) : (
            <Field>
              <FieldLabel htmlFor="ibmmq-topic-string">
                {t("board.ibmmq.queues.topicString")}
              </FieldLabel>
              <Input
                id="ibmmq-topic-string"
                className="mono3"
                value={draft.topicString}
                placeholder="orders/created"
                onChange={(event) => set("topicString", event.target.value)}
              />
              <FieldDescription>{t("board.ibmmq.queues.topicStringHint")}</FieldDescription>
            </Field>
          )}

          <Field>
            <FieldLabel htmlFor="ibmmq-description">
              {t("board.ibmmq.queues.description")}
            </FieldLabel>
            <Input
              id="ibmmq-description"
              value={draft.description}
              onChange={(event) => set("description", event.target.value)}
            />
          </Field>

          {error != null && (
            <FieldDescription style={{ color: "var(--c-danger)" }}>{error}</FieldDescription>
          )}
        </FieldGroup>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t("common.cancel")}
          </Button>
          <Button disabled={blocked || busy} onClick={() => void submit()}>
            {busy && <Spinner />}
            {t("common.create")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
