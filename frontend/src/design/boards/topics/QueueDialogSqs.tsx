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
import { Switch } from "@/components/ui/switch";
import { Spinner } from "@/components/ui/spinner";
import { formatErrorMessage } from "@/lib/utils";
import { isFifoName, nameProblem, submittableName, withFifo } from "@/mq/sqs/names";
import type { SQSQueueInput } from "@/api/sqs";
import type { SqsQueue } from "@/mq/sqs/destinations";

/** SQS's own defaults, so a create form shows what a queue will actually get. */
const DEFAULTS = {
  visibilityTimeoutSec: 30,
  delaySec: 0,
  retentionSec: 345_600,
  maxMessageBytes: 262_144,
  receiveWaitSec: 0,
};

interface Draft {
  name: string;
  fifo: boolean;
  visibilityTimeoutSec: number;
  delaySec: number;
  retentionSec: number;
  maxMessageBytes: number;
  receiveWaitSec: number;
  deadLetterQueue: string;
  maxReceiveCount: number;
  contentBasedDeduplication: boolean;
}

function draftOf(editing: SqsQueue | null): Draft {
  if (editing == null) {
    return {
      name: "",
      fifo: false,
      ...DEFAULTS,
      deadLetterQueue: "",
      maxReceiveCount: 5,
      contentBasedDeduplication: false,
    };
  }
  return {
    name: editing.name,
    fifo: editing.fifo,
    visibilityTimeoutSec: editing.visibilityTimeoutSec ?? DEFAULTS.visibilityTimeoutSec,
    delaySec: editing.delaySec ?? DEFAULTS.delaySec,
    retentionSec: editing.retentionSec ?? DEFAULTS.retentionSec,
    maxMessageBytes: editing.maxMessageBytes ?? DEFAULTS.maxMessageBytes,
    receiveWaitSec: editing.receiveWaitSec ?? DEFAULTS.receiveWaitSec,
    deadLetterQueue: editing.deadLetterQueue ?? "",
    maxReceiveCount: editing.maxReceiveCount ?? 5,
    contentBasedDeduplication: editing.contentBasedDeduplication ?? false,
  };
}

function numberField(raw: string): number {
  const parsed = Number.parseInt(raw, 10);
  return Number.isNaN(parsed) || parsed < 0 ? 0 : parsed;
}

/**
 * Create or edit one queue.
 *
 * One dialog for both, because the field set is the same one - with two
 * exceptions that are the service's rather than a design choice. A name cannot
 * change, and whether a queue is FIFO cannot either: it is fixed at creation
 * and spelled in the name, so the switch renames the queue on a create and is
 * read-only on an edit.
 *
 * The dead-letter queue is named rather than picked from a list. It is an
 * ordinary queue that may not exist yet, may be in another account, and is
 * resolved to an ARN by the driver - a picker would imply the choice is
 * limited to what this connection can see.
 */
export function QueueDialogSqs({
  open,
  editing,
  onOpenChange,
  onSubmit,
}: {
  open: boolean;
  /** Null creates; a queue edits it. */
  editing: SqsQueue | null;
  onOpenChange: (open: boolean) => void;
  onSubmit: (input: SQSQueueInput) => Promise<void>;
}) {
  const { t } = useTranslation();
  const [draft, setDraft] = useState<Draft>(() => draftOf(editing));
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (open) {
      setDraft(draftOf(editing));
      setError(null);
    }
  }, [open, editing]);

  const set = <K extends keyof Draft>(key: K, next: Draft[K]) =>
    setDraft((current) => ({ ...current, [key]: next }));

  const problem = draft.name === "" ? null : nameProblem(draft.name);
  const submitted = submittableName(draft.name);

  const submit = async () => {
    if (submitted == null || busy) return;
    setBusy(true);
    setError(null);
    try {
      await onSubmit({
        name: submitted,
        fifo: isFifoName(submitted),
        visibilityTimeoutSec: draft.visibilityTimeoutSec,
        delaySec: draft.delaySec,
        retentionSec: draft.retentionSec,
        maxMessageBytes: draft.maxMessageBytes,
        receiveWaitSec: draft.receiveWaitSec,
        deadLetterQueue: draft.deadLetterQueue.trim(),
        maxReceiveCount: draft.maxReceiveCount,
        contentBasedDeduplication: draft.contentBasedDeduplication,
        deduplicationScope: "",
        fifoThroughputLimit: "",
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
          <DialogTitle>
            {t(editing == null ? "board.sqs.queues.createTitle" : "board.sqs.queues.editTitle", {
              name: editing?.name ?? "",
            })}
          </DialogTitle>
        </DialogHeader>
        <FieldGroup>
          <Field>
            <FieldLabel htmlFor="sqs-queue-name">{t("board.sqs.queues.name")}</FieldLabel>
            <Input
              id="sqs-queue-name"
              value={draft.name}
              disabled={editing != null}
              placeholder="orders"
              onChange={(event) => set("name", event.target.value)}
            />
            <FieldDescription>
              {problem != null
                ? t(`board.sqs.queues.nameProblem.${problem}`)
                : t("board.sqs.queues.nameHint")}
            </FieldDescription>
          </Field>

          <Field>
            <FieldLabel htmlFor="sqs-queue-fifo">{t("board.sqs.queues.fifoBadge")}</FieldLabel>
            <Switch
              id="sqs-queue-fifo"
              checked={isFifoName(draft.name)}
              disabled={editing != null}
              onCheckedChange={(next: boolean) =>
                setDraft((current) => ({
                  ...current,
                  fifo: next,
                  name: withFifo(current.name, next),
                }))
              }
            />
            <FieldDescription>
              {t(editing == null ? "board.sqs.queues.fifoHint" : "board.sqs.queues.fifoFixed")}
            </FieldDescription>
          </Field>

          <Field>
            <FieldLabel htmlFor="sqs-visibility">
              {t("board.sqs.queues.visibilityTimeout")}
            </FieldLabel>
            <Input
              id="sqs-visibility"
              type="number"
              value={String(draft.visibilityTimeoutSec)}
              onChange={(event) => set("visibilityTimeoutSec", numberField(event.target.value))}
            />
            <FieldDescription>{t("board.sqs.queues.visibilityTimeoutHint")}</FieldDescription>
          </Field>

          <Field>
            <FieldLabel htmlFor="sqs-retention">{t("board.sqs.queues.retention")}</FieldLabel>
            <Input
              id="sqs-retention"
              type="number"
              value={String(draft.retentionSec)}
              onChange={(event) => set("retentionSec", numberField(event.target.value))}
            />
            <FieldDescription>{t("board.sqs.queues.retentionHint")}</FieldDescription>
          </Field>

          <Field>
            <FieldLabel htmlFor="sqs-delay">{t("board.sqs.queues.deliveryDelay")}</FieldLabel>
            <Input
              id="sqs-delay"
              type="number"
              value={String(draft.delaySec)}
              onChange={(event) => set("delaySec", numberField(event.target.value))}
            />
            <FieldDescription>{t("board.sqs.queues.deliveryDelayHint")}</FieldDescription>
          </Field>

          <Field>
            <FieldLabel htmlFor="sqs-wait">{t("board.sqs.queues.receiveWait")}</FieldLabel>
            <Input
              id="sqs-wait"
              type="number"
              value={String(draft.receiveWaitSec)}
              onChange={(event) => set("receiveWaitSec", numberField(event.target.value))}
            />
            <FieldDescription>{t("board.sqs.queues.receiveWaitHint")}</FieldDescription>
          </Field>

          <Field>
            <FieldLabel htmlFor="sqs-dlq">{t("board.sqs.queues.deadLetterQueue")}</FieldLabel>
            <Input
              id="sqs-dlq"
              value={draft.deadLetterQueue}
              placeholder="orders-dlq"
              onChange={(event) => set("deadLetterQueue", event.target.value)}
            />
            <FieldDescription>{t("board.sqs.queues.deadLetterQueueHint")}</FieldDescription>
          </Field>

          {draft.deadLetterQueue.trim() !== "" && (
            <Field>
              <FieldLabel htmlFor="sqs-max-receive">
                {t("board.sqs.queues.maxReceiveCount")}
              </FieldLabel>
              <Input
                id="sqs-max-receive"
                type="number"
                value={String(draft.maxReceiveCount)}
                onChange={(event) => set("maxReceiveCount", numberField(event.target.value))}
              />
              <FieldDescription>{t("board.sqs.queues.maxReceiveCountHint")}</FieldDescription>
            </Field>
          )}

          {isFifoName(draft.name) && (
            <Field>
              <FieldLabel htmlFor="sqs-dedup">{t("board.sqs.queues.contentDedup")}</FieldLabel>
              <Switch
                id="sqs-dedup"
                checked={draft.contentBasedDeduplication}
                onCheckedChange={(next: boolean) => set("contentBasedDeduplication", next)}
              />
              <FieldDescription>{t("board.sqs.queues.contentDedupHint")}</FieldDescription>
            </Field>
          )}
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
