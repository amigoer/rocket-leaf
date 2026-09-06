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
import { nameProblem, submittableName } from "@/mq/googlepubsub/names";
import type { GooglePubSubSubscriptionInput } from "@/api/googlepubsub";
import type { PubSubSubscription } from "@/mq/googlepubsub/subscriptions";

/** The service's own defaults, so a create form shows what it will get. */
const DEFAULTS = {
  ackDeadlineSec: 10,
  retentionSec: 604_800,
  maxAttempts: 5,
  retryMinBackoffSec: 10,
  retryMaxBackoffSec: 600,
};

interface Draft {
  name: string;
  topic: string;
  ackDeadlineSec: number;
  retentionSec: number;
  retainAcked: boolean;
  exactlyOnce: boolean;
  filter: string;
  ordering: boolean;
  deadLetterTopic: string;
  maxAttempts: number;
  retryMinBackoffSec: number;
  retryMaxBackoffSec: number;
}

function draftOf(editing: PubSubSubscription | null, topic: string): Draft {
  if (editing == null) {
    return {
      name: "",
      topic,
      ...DEFAULTS,
      retainAcked: false,
      exactlyOnce: false,
      filter: "",
      ordering: false,
      deadLetterTopic: "",
    };
  }
  return {
    name: editing.name,
    topic: editing.topic,
    ackDeadlineSec: editing.ackDeadlineSec ?? DEFAULTS.ackDeadlineSec,
    retentionSec: editing.retentionSec ?? DEFAULTS.retentionSec,
    retainAcked: editing.retainAcked,
    exactlyOnce: editing.exactlyOnce,
    filter: editing.filter ?? "",
    ordering: editing.ordering,
    deadLetterTopic: editing.deadLetterTopic ?? "",
    maxAttempts: editing.maxDeliveryAttempts ?? DEFAULTS.maxAttempts,
    retryMinBackoffSec: editing.retryMinBackoffSec ?? 0,
    retryMaxBackoffSec: editing.retryMaxBackoffSec ?? 0,
  };
}

function numberField(raw: string): number {
  const parsed = Number.parseInt(raw, 10);
  return Number.isNaN(parsed) || parsed < 0 ? 0 : parsed;
}

/**
 * Create or edit one subscription.
 *
 * This is where the whole of the delivery configuration lives, which is the
 * thing that separates this family from the one before it: how long a message
 * is held, how long it is kept, how many attempts before it is given up on and
 * where it goes then. None of it belongs to the topic.
 *
 * Four fields go read-only on an edit and each for the service's own reason.
 * The name and the topic cannot change - a subscription reads exactly one
 * topic, chosen when it is made, and there is no rename call. The filter and
 * message ordering cannot either: both are fixed at creation, and a form that
 * let them be edited would collect a change the service throws away.
 */
export function SubscriptionDialogGooglePubSub({
  open,
  editing,
  defaultTopic,
  onOpenChange,
  onSubmit,
}: {
  open: boolean;
  /** Null creates; a subscription edits it. */
  editing: PubSubSubscription | null;
  /** Pre-filled on a create, so arriving from a topic row skips a field. */
  defaultTopic?: string;
  onOpenChange: (open: boolean) => void;
  onSubmit: (input: GooglePubSubSubscriptionInput) => Promise<void>;
}) {
  const { t } = useTranslation();
  const [draft, setDraft] = useState<Draft>(() => draftOf(editing, defaultTopic ?? ""));
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (open) {
      setDraft(draftOf(editing, defaultTopic ?? ""));
      setError(null);
    }
  }, [open, editing, defaultTopic]);

  const set = <K extends keyof Draft>(key: K, next: Draft[K]) =>
    setDraft((current) => ({ ...current, [key]: next }));

  const creating = editing == null;
  const problem = draft.name === "" ? null : nameProblem(draft.name);
  const submitted = submittableName(draft.name);
  const missingTopic = creating && draft.topic.trim() === "";

  const submit = async () => {
    if (submitted == null || missingTopic || busy) return;
    setBusy(true);
    setError(null);
    try {
      await onSubmit({
        name: submitted,
        topic: draft.topic.trim(),
        ackDeadlineSec: draft.ackDeadlineSec,
        retentionSec: draft.retentionSec,
        retainAcked: draft.retainAcked,
        exactlyOnce: draft.exactlyOnce,
        filter: draft.filter.trim(),
        ordering: draft.ordering,
        pushEndpoint: "",
        deadLetterTopic: draft.deadLetterTopic.trim(),
        maxAttempts: draft.maxAttempts,
        retryMinBackoffSec: draft.retryMinBackoffSec,
        retryMaxBackoffSec: draft.retryMaxBackoffSec,
        labels: {},
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
              creating
                ? "board.google-pubsub.subscriptions.createTitle"
                : "board.google-pubsub.subscriptions.editTitle",
              { name: editing?.name ?? "" },
            )}
          </DialogTitle>
        </DialogHeader>
        <FieldGroup>
          <Field>
            <FieldLabel htmlFor="pubsub-sub-name">
              {t("board.google-pubsub.subscriptions.name")}
            </FieldLabel>
            <Input
              id="pubsub-sub-name"
              value={draft.name}
              disabled={!creating}
              placeholder="orders-worker"
              onChange={(event) => set("name", event.target.value)}
            />
            <FieldDescription>
              {problem != null
                ? t(`board.google-pubsub.topics.nameProblem.${problem}`)
                : t("board.google-pubsub.subscriptions.nameHint")}
            </FieldDescription>
          </Field>

          <Field>
            <FieldLabel htmlFor="pubsub-sub-topic">
              {t("board.google-pubsub.subscriptions.topic")}
            </FieldLabel>
            <Input
              id="pubsub-sub-topic"
              value={draft.topic}
              disabled={!creating}
              placeholder="orders"
              onChange={(event) => set("topic", event.target.value)}
            />
            <FieldDescription>
              {t("board.google-pubsub.subscriptions.topicHint")}
            </FieldDescription>
          </Field>

          <Field>
            <FieldLabel htmlFor="pubsub-sub-ack">
              {t("board.google-pubsub.subscriptions.ackDeadline")}
            </FieldLabel>
            <Input
              id="pubsub-sub-ack"
              type="number"
              value={String(draft.ackDeadlineSec)}
              onChange={(event) => set("ackDeadlineSec", numberField(event.target.value))}
            />
            <FieldDescription>
              {t("board.google-pubsub.subscriptions.ackDeadlineHint")}
            </FieldDescription>
          </Field>

          <Field>
            <FieldLabel htmlFor="pubsub-sub-retention">
              {t("board.google-pubsub.subscriptions.retention")}
            </FieldLabel>
            <Input
              id="pubsub-sub-retention"
              type="number"
              value={String(draft.retentionSec)}
              onChange={(event) => set("retentionSec", numberField(event.target.value))}
            />
            <FieldDescription>
              {t("board.google-pubsub.subscriptions.retentionHint")}
            </FieldDescription>
          </Field>

          <Field>
            <FieldLabel htmlFor="pubsub-sub-retain">
              {t("board.google-pubsub.subscriptions.retainAcked")}
            </FieldLabel>
            <Switch
              id="pubsub-sub-retain"
              checked={draft.retainAcked}
              onCheckedChange={(next: boolean) => set("retainAcked", next)}
            />
            <FieldDescription>
              {t("board.google-pubsub.subscriptions.retainAckedHint")}
            </FieldDescription>
          </Field>

          <Field>
            <FieldLabel htmlFor="pubsub-sub-exactly-once">
              {t("board.google-pubsub.subscriptions.exactlyOnce")}
            </FieldLabel>
            <Switch
              id="pubsub-sub-exactly-once"
              checked={draft.exactlyOnce}
              onCheckedChange={(next: boolean) => set("exactlyOnce", next)}
            />
            <FieldDescription>
              {t("board.google-pubsub.subscriptions.exactlyOnceHint")}
            </FieldDescription>
          </Field>

          <Field>
            <FieldLabel htmlFor="pubsub-sub-ordering">
              {t("board.google-pubsub.subscriptions.ordering")}
            </FieldLabel>
            <Switch
              id="pubsub-sub-ordering"
              checked={draft.ordering}
              disabled={!creating}
              onCheckedChange={(next: boolean) => set("ordering", next)}
            />
            <FieldDescription>
              {t(
                creating
                  ? "board.google-pubsub.subscriptions.orderingHint"
                  : "board.google-pubsub.subscriptions.orderingFixed",
              )}
            </FieldDescription>
          </Field>

          <Field>
            <FieldLabel htmlFor="pubsub-sub-filter">
              {t("board.google-pubsub.subscriptions.filter")}
            </FieldLabel>
            <Input
              id="pubsub-sub-filter"
              className="mono3"
              value={draft.filter}
              disabled={!creating}
              placeholder='attributes.kind = "order"'
              onChange={(event) => set("filter", event.target.value)}
            />
            <FieldDescription>
              {t(
                creating
                  ? "board.google-pubsub.subscriptions.filterHint"
                  : "board.google-pubsub.subscriptions.filterFixed",
              )}
            </FieldDescription>
          </Field>

          <Field>
            <FieldLabel htmlFor="pubsub-sub-dlt">
              {t("board.google-pubsub.subscriptions.deadLetterTopic")}
            </FieldLabel>
            <Input
              id="pubsub-sub-dlt"
              value={draft.deadLetterTopic}
              placeholder="orders-dead-letters"
              onChange={(event) => set("deadLetterTopic", event.target.value)}
            />
            <FieldDescription>
              {t("board.google-pubsub.subscriptions.deadLetterTopicHint")}
            </FieldDescription>
          </Field>

          {draft.deadLetterTopic.trim() !== "" && (
            <Field>
              <FieldLabel htmlFor="pubsub-sub-attempts">
                {t("board.google-pubsub.subscriptions.maxAttempts")}
              </FieldLabel>
              <Input
                id="pubsub-sub-attempts"
                type="number"
                value={String(draft.maxAttempts)}
                onChange={(event) => set("maxAttempts", numberField(event.target.value))}
              />
              <FieldDescription>
                {t("board.google-pubsub.subscriptions.maxAttemptsHint")}
              </FieldDescription>
            </Field>
          )}

          <Field>
            <FieldLabel htmlFor="pubsub-sub-retry-min">
              {t("board.google-pubsub.subscriptions.retryBackoff")}
            </FieldLabel>
            <div style={{ display: "flex", gap: "8px" }}>
              <Input
                id="pubsub-sub-retry-min"
                type="number"
                value={String(draft.retryMinBackoffSec)}
                onChange={(event) => set("retryMinBackoffSec", numberField(event.target.value))}
              />
              <Input
                type="number"
                aria-label={t("board.google-pubsub.subscriptions.retryBackoffMax")}
                value={String(draft.retryMaxBackoffSec)}
                onChange={(event) => set("retryMaxBackoffSec", numberField(event.target.value))}
              />
            </div>
            <FieldDescription>
              {t("board.google-pubsub.subscriptions.retryBackoffHint")}
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
          <Button disabled={submitted == null || missingTopic || busy} onClick={() => void submit()}>
            {busy && <Spinner />}
            {t("common.confirm")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
