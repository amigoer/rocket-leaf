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
import {
  MIN_RETENTION_HOURS,
  nameProblem,
  retentionProblem,
  shardsProblem,
  submittableName,
} from "@/mq/kinesis/names";
import type { KinesisStreamInput } from "@/api/kinesis";
import type { KinesisStream } from "@/mq/kinesis/destinations";

/** What a new stream gets when nothing is typed, as the service would. */
const DEFAULTS = {
  shards: 1,
  retentionHours: MIN_RETENTION_HOURS,
};

interface Draft {
  name: string;
  onDemand: boolean;
  shards: number;
  retentionHours: number;
}

function draftOf(editing: KinesisStream | null): Draft {
  if (editing == null) {
    return { name: "", onDemand: false, ...DEFAULTS };
  }
  return {
    name: editing.name,
    onDemand: editing.mode === "ON_DEMAND",
    shards: editing.openShards ?? DEFAULTS.shards,
    retentionHours: editing.retentionHours ?? DEFAULTS.retentionHours,
  };
}

function numberField(raw: string): number {
  const parsed = Number.parseInt(raw, 10);
  return Number.isNaN(parsed) || parsed < 0 ? 0 : parsed;
}

/**
 * Create or edit one stream.
 *
 * One dialog for both, and the field set really is the same one - a name that
 * cannot change is the only exception. What differs is what an edit costs: a
 * create is one call, and a change to the mode, the shard count or the
 * retention is a separate asynchronous operation each, applied in that order
 * because switching to on demand takes the shard count out of the operator's
 * hands entirely.
 *
 * The shard field disappears with the mode rather than being disabled, because
 * an on-demand stream has no shard count to set: AWS chooses it, and
 * CreateStream refuses a number sent beside the mode.
 *
 * Resizing is not free and the dialog says so. The service reaches a new count
 * by splitting and merging, which leaves the old shards closed and still
 * holding their records until retention expires - so the shards page shows
 * more shards than this number for as long as that lasts.
 */
export function StreamDialogKinesis({
  open,
  editing,
  onOpenChange,
  onSubmit,
}: {
  open: boolean;
  /** Null creates; a stream edits it. */
  editing: KinesisStream | null;
  onOpenChange: (open: boolean) => void;
  onSubmit: (input: KinesisStreamInput) => Promise<void>;
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
  const shards = draft.onDemand ? null : shardsProblem(draft.shards);
  const retention = retentionProblem(draft.retentionHours);
  const blocked = submitted == null || shards != null || retention != null;

  const submit = async () => {
    if (blocked || busy) return;
    setBusy(true);
    setError(null);
    try {
      await onSubmit({
        name: submitted,
        onDemand: draft.onDemand,
        shards: draft.onDemand ? 0 : draft.shards,
        retentionHours: draft.retentionHours,
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
            {t(
              editing == null
                ? "board.kinesis.streams.createTitle"
                : "board.kinesis.streams.editTitle",
              { name: editing?.name ?? "" },
            )}
          </DialogTitle>
        </DialogHeader>
        <FieldGroup>
          <Field>
            <FieldLabel htmlFor="kinesis-stream-name">
              {t("board.kinesis.streams.name")}
            </FieldLabel>
            <Input
              id="kinesis-stream-name"
              value={draft.name}
              disabled={editing != null}
              placeholder="orders"
              onChange={(event) => set("name", event.target.value)}
            />
            <FieldDescription>
              {problem != null
                ? t(`board.kinesis.streams.nameProblem.${problem}`)
                : t("board.kinesis.streams.nameHint")}
            </FieldDescription>
          </Field>

          <Field>
            <FieldLabel htmlFor="kinesis-on-demand">
              {t("board.kinesis.streams.onDemandBadge")}
            </FieldLabel>
            <Switch
              id="kinesis-on-demand"
              checked={draft.onDemand}
              onCheckedChange={(next: boolean) => set("onDemand", next)}
            />
            <FieldDescription>{t("board.kinesis.streams.onDemandHint")}</FieldDescription>
          </Field>

          {!draft.onDemand && (
            <Field>
              <FieldLabel htmlFor="kinesis-shards">
                {t("board.kinesis.streams.openShards")}
              </FieldLabel>
              <Input
                id="kinesis-shards"
                type="number"
                value={String(draft.shards)}
                onChange={(event) => set("shards", numberField(event.target.value))}
              />
              <FieldDescription>
                {shards != null
                  ? t("board.kinesis.streams.shardsProblem.tooFew")
                  : t(
                      editing == null
                        ? "board.kinesis.streams.shardsHint"
                        : "board.kinesis.streams.resizeHint",
                    )}
              </FieldDescription>
            </Field>
          )}

          <Field>
            <FieldLabel htmlFor="kinesis-retention">
              {t("board.kinesis.streams.retention")}
            </FieldLabel>
            <Input
              id="kinesis-retention"
              type="number"
              value={String(draft.retentionHours)}
              onChange={(event) => set("retentionHours", numberField(event.target.value))}
            />
            <FieldDescription>
              {retention != null
                ? t(`board.kinesis.streams.retentionProblem.${retention}`)
                : t("board.kinesis.streams.retentionHint")}
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
            {t("common.confirm")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
