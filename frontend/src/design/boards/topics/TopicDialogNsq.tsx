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
 * A name, and nothing else.
 *
 * Every other family's create dialog collects settings because its
 * destinations have some. An NSQ topic has none: retention, the memory queue
 * size and the disk overflow are nsqd's own flags and apply to every topic on
 * the daemon, so a field here would be one the daemon ignores.
 *
 * There is no kind and no node picker either. A topic is created on every nsqd
 * in the connection, because a producer that connected to one this dialog
 * skipped would auto-create it later with none of its channels - which reads
 * as a topic that lost its consumers.
 *
 * The name rule is checked before the button rather than after: nsqd refuses a
 * bad one as INVALID_TOPIC, or as INVALID_REQUEST when the character broke the
 * URL first, and neither answer says which character was the problem.
 */
export function TopicDialogNsq({
  open,
  onOpenChange,
  onCreate,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onCreate: (name: string) => Promise<void>;
}) {
  const { t } = useTranslation();
  const [name, setName] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (open) {
      setName("");
      setError(null);
    }
  }, [open]);

  const problem = name === "" ? null : nameProblem(name);
  const submitted = submittableName(name);

  const submit = async () => {
    if (submitted == null || busy) return;
    setBusy(true);
    setError(null);
    try {
      await onCreate(submitted);
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
          <DialogTitle>{t("board.nsq.topics.createTitle")}</DialogTitle>
        </DialogHeader>
        <FieldGroup>
          <Field>
            <FieldLabel htmlFor="nsq-topic-name">{t("board.nsq.topics.name")}</FieldLabel>
            <Input
              id="nsq-topic-name"
              value={name}
              placeholder="orders.created"
              onChange={(event) => setName(event.target.value)}
              onKeyDown={(event) => {
                if (event.key === "Enter") void submit();
              }}
            />
            <FieldDescription>
              {problem != null
                ? t(`board.nsq.topics.nameProblem.${problem}`)
                : isEphemeral(name)
                  ? t("board.nsq.topics.nameEphemeral")
                  : t("board.nsq.topics.nameHint")}
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
          <Button disabled={submitted == null || busy} onClick={() => void submit()}>
            {busy && <Spinner />}
            {t("common.confirm")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
