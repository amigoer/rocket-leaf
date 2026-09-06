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
import { nameProblem, submittableName } from "@/mq/googlepubsub/names";
import type { GooglePubSubTopicInput } from "@/api/googlepubsub";
import type { PubSubTopic } from "@/mq/googlepubsub/topics";

/** The bounds Pub/Sub enforces, so the form can say them before the API does. */
const RETENTION_MIN_SEC = 600;
const RETENTION_MAX_SEC = 31 * 24 * 3600;

interface Draft {
  name: string;
  /** Zero means "leave it alone", which is also the service's own default. */
  retentionSec: number;
  /** One `key=value` per line, which is how a label set is edited as a set. */
  labels: string;
}

function labelsText(entry: PubSubTopic): string {
  return entry.labels.map(([key, value]) => `${key}=${value}`).join("\n");
}

/**
 * Parses the label box back into a map.
 *
 * Null means "the box was left empty and nothing about labels was said", which
 * an update has to distinguish from an empty map: the update mask names the
 * whole field, so sending an empty map removes every label the topic has.
 */
function parseLabels(text: string, editing: boolean): Record<string, string> | null {
  const lines = text
    .split("\n")
    .map((line) => line.trim())
    .filter((line) => line !== "");
  if (lines.length === 0) return editing ? {} : null;
  const labels: Record<string, string> = {};
  for (const line of lines) {
    const at = line.indexOf("=");
    if (at <= 0) continue;
    labels[line.slice(0, at).trim()] = line.slice(at + 1).trim();
  }
  return labels;
}

function draftOf(editing: PubSubTopic | null): Draft {
  if (editing == null) return { name: "", retentionSec: 0, labels: "" };
  return {
    name: editing.name,
    retentionSec: editing.retentionSec ?? 0,
    labels: labelsText(editing),
  };
}

function numberField(raw: string): number {
  const parsed = Number.parseInt(raw, 10);
  return Number.isNaN(parsed) || parsed < 0 ? 0 : parsed;
}

/**
 * Create or edit one topic.
 *
 * There is very little on the form and that is the family rather than a gap:
 * everything about delivery - the ack deadline, the retry policy, where dead
 * letters go, what is filtered out - belongs to the subscription. What a topic
 * owns is how long a published message stays available for a subscription to
 * seek back into, and a set of labels.
 *
 * A name cannot change, which is the service's rule rather than a design
 * choice: a topic is addressed by its full resource path and there is no
 * rename call anywhere in the API.
 *
 * The labels are edited as a block rather than row by row, because that is
 * what the update actually does - the mask names the field, so what is sent
 * replaces the whole set and a form editing one key would silently drop the
 * others.
 */
export function TopicDialogGooglePubSub({
  open,
  editing,
  onOpenChange,
  onSubmit,
}: {
  open: boolean;
  /** Null creates; a topic edits it. */
  editing: PubSubTopic | null;
  onOpenChange: (open: boolean) => void;
  onSubmit: (input: GooglePubSubTopicInput) => Promise<void>;
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
  const retentionOutOfRange =
    draft.retentionSec !== 0 &&
    (draft.retentionSec < RETENTION_MIN_SEC || draft.retentionSec > RETENTION_MAX_SEC);

  const submit = async () => {
    if (submitted == null || retentionOutOfRange || busy) return;
    setBusy(true);
    setError(null);
    try {
      await onSubmit({
        name: submitted,
        retentionSec: draft.retentionSec,
        labels: parseLabels(draft.labels, editing != null) ?? {},
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
                ? "board.google-pubsub.topics.createTitle"
                : "board.google-pubsub.topics.editTitle",
              { name: editing?.name ?? "" },
            )}
          </DialogTitle>
        </DialogHeader>
        <FieldGroup>
          <Field>
            <FieldLabel htmlFor="pubsub-topic-name">
              {t("board.google-pubsub.topics.name")}
            </FieldLabel>
            <Input
              id="pubsub-topic-name"
              value={draft.name}
              disabled={editing != null}
              placeholder="orders"
              onChange={(event) => set("name", event.target.value)}
            />
            <FieldDescription>
              {problem != null
                ? t(`board.google-pubsub.topics.nameProblem.${problem}`)
                : t("board.google-pubsub.topics.nameHint")}
            </FieldDescription>
          </Field>

          <Field>
            <FieldLabel htmlFor="pubsub-topic-retention">
              {t("board.google-pubsub.topics.retention")}
            </FieldLabel>
            <Input
              id="pubsub-topic-retention"
              type="number"
              value={String(draft.retentionSec)}
              onChange={(event) => set("retentionSec", numberField(event.target.value))}
            />
            <FieldDescription style={retentionOutOfRange ? { color: "var(--c-danger)" } : undefined}>
              {retentionOutOfRange
                ? t("board.google-pubsub.topics.retentionRange")
                : t("board.google-pubsub.topics.retentionHint")}
            </FieldDescription>
          </Field>

          <Field>
            <FieldLabel htmlFor="pubsub-topic-labels">
              {t("board.google-pubsub.topics.labels")}
            </FieldLabel>
            <textarea
              id="pubsub-topic-labels"
              className="mono3 min-h-16 rounded-md border border-(--c-line) bg-(--c-surface) px-2.5 py-2 text-xs outline-none focus-visible:border-(--c-accent)"
              spellCheck={false}
              value={draft.labels}
              placeholder="team=orders"
              onChange={(event) => set("labels", event.target.value)}
            />
            <FieldDescription>{t("board.google-pubsub.topics.labelsHint")}</FieldDescription>
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
            disabled={submitted == null || retentionOutOfRange || busy}
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
