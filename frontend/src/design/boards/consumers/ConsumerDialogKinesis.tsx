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
import { useKinesisDestinations } from "@/hooks/kinesis/useKinesisDestinations";
import { nameProblem, submittableName } from "@/mq/kinesis/names";
import { formatErrorMessage } from "@/lib/utils";

/**
 * Register a consumer on a stream.
 *
 * Create only, and that is the object rather than a decision here: a
 * registered consumer is a name, an ARN, a status and a creation time, all of
 * which are the service's. There is nothing an edit could change, because
 * every setting a reader might want - retention, capacity, encryption -
 * belongs to the stream.
 *
 * The stream is a field rather than context because a consumer name is unique
 * only within its stream, so the same name may exist on several and the pair
 * is what identifies one.
 */
export function ConsumerDialogKinesis({
  open,
  onOpenChange,
  onSubmit,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSubmit: (stream: string, name: string) => Promise<void>;
}) {
  const { t } = useTranslation();
  const streams = useKinesisDestinations();
  const [stream, setStream] = useState("");
  const [name, setName] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (open) {
      setStream("");
      setName("");
      setError(null);
    }
  }, [open]);

  // A consumer name takes the same characters a stream name does, which is
  // what makes the rule shareable rather than a coincidence worth restating.
  const problem = name === "" ? null : nameProblem(name);
  const submitted = submittableName(name);
  const blocked = stream.trim() === "" || submitted == null;

  const submit = async () => {
    if (blocked || busy) return;
    setBusy(true);
    setError(null);
    try {
      await onSubmit(stream.trim(), submitted);
      onOpenChange(false);
    } catch (submitError) {
      setError(formatErrorMessage(submitError));
    } finally {
      setBusy(false);
    }
  };

  const streamOptions = (streams.data ?? []).map((entry) => ({
    value: entry.ref.name,
    label: entry.ref.name,
  }));

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("board.kinesis.consumers.registerTitle")}</DialogTitle>
        </DialogHeader>
        <FieldGroup>
          <Field>
            <FieldLabel htmlFor="kinesis-consumer-stream">
              {t("board.kinesis.consumers.stream")}
            </FieldLabel>
            {streamOptions.length > 0 ? (
              <SelectField value={stream} options={streamOptions} onValueChange={setStream} />
            ) : (
              <Input
                id="kinesis-consumer-stream"
                value={stream}
                placeholder="orders"
                onChange={(event) => setStream(event.target.value)}
              />
            )}
            <FieldDescription>{t("board.kinesis.consumers.streamHint")}</FieldDescription>
          </Field>

          <Field>
            <FieldLabel htmlFor="kinesis-consumer-name">
              {t("board.kinesis.consumers.name")}
            </FieldLabel>
            <Input
              id="kinesis-consumer-name"
              value={name}
              placeholder="analytics"
              onChange={(event) => setName(event.target.value)}
            />
            <FieldDescription>
              {problem != null
                ? t(`board.kinesis.streams.nameProblem.${problem}`)
                : t("board.kinesis.consumers.nameHint")}
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
          <Button disabled={blocked || busy} onClick={() => void submit()}>
            {busy && <Spinner />}
            {t("board.kinesis.consumers.register")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
