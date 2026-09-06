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
import { Switch } from "@/components/ui/switch";
import { formatErrorMessage } from "@/lib/utils";
import { nameProblem, submittableName } from "@/mq/azureservicebus/names";
import type { AzureServiceBusSubscriptionInput } from "@/api/azureservicebus";
import type { ServiceBusSubscription } from "@/mq/azureservicebus/subscriptions";

const LOCK_MIN_SEC = 5;
const LOCK_MAX_SEC = 300;
const DELIVERY_MIN = 1;
const DELIVERY_MAX = 2000;

interface Draft {
  topic: string;
  name: string;
  lockDurationSec: number;
  maxDeliveryCount: number;
  ttlSec: number;
  deadLetterOnExpiry: boolean;
  deadLetterOnRuleError: boolean;
  requiresSession: boolean;
  forwardDeadLettersTo: string;
}

function draftOf(editing: ServiceBusSubscription | null): Draft {
  if (editing == null) {
    return {
      topic: "",
      name: "",
      lockDurationSec: 0,
      maxDeliveryCount: 0,
      ttlSec: 0,
      deadLetterOnExpiry: false,
      // On by default because the alternative loses messages silently: a rule
      // whose expression throws discards what it was evaluating unless this
      // is set, and a subscription with filters is the ordinary case here.
      deadLetterOnRuleError: true,
      requiresSession: false,
      forwardDeadLettersTo: "",
    };
  }
  return {
    topic: editing.topic,
    name: editing.name,
    lockDurationSec: editing.lockDurationSec ?? 0,
    maxDeliveryCount: editing.maxDeliveryCount ?? 0,
    ttlSec: forever(editing.ttlSec) ? 0 : (editing.ttlSec ?? 0),
    deadLetterOnExpiry: editing.deadLetterOnExpiry,
    deadLetterOnRuleError: editing.deadLetterOnRuleError,
    requiresSession: editing.requiresSession,
    forwardDeadLettersTo: editing.forwardDeadLettersTo ?? "",
  };
}

function forever(seconds: number | null): boolean {
  return seconds != null && seconds > 100 * 365 * 86400;
}

function numberField(raw: string): number {
  const parsed = Number.parseInt(raw, 10);
  return Number.isNaN(parsed) || parsed < 0 ? 0 : parsed;
}

/**
 * Create or edit one subscription.
 *
 * The topic is chosen once and cannot change: a subscription reads exactly one
 * topic, and moving it would mean a different object with a different backlog.
 * Sessions are fixed at creation too - the service refuses them in an update,
 * so the control is only drawn on a create rather than left to fail.
 *
 * What is deliberately not on this form is what reaches the subscription.
 * Every new one comes with a $Default rule matching everything, and narrowing
 * that belongs to the routing page: a rule is an object with a name, so
 * putting one here would hide a second write inside the first.
 */
export function SubscriptionDialogAzureServiceBus({
  open,
  editing,
  onOpenChange,
  onSubmit,
}: {
  open: boolean;
  /** Null creates; a subscription edits it. */
  editing: ServiceBusSubscription | null;
  onOpenChange: (open: boolean) => void;
  onSubmit: (input: AzureServiceBusSubscriptionInput) => Promise<void>;
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

  const nameIssue = draft.name === "" ? null : nameProblem(draft.name, "child");
  const submittedName = submittableName(draft.name, "child");
  const submittedTopic = submittableName(draft.topic);
  const lockOutOfRange =
    draft.lockDurationSec !== 0 &&
    (draft.lockDurationSec < LOCK_MIN_SEC || draft.lockDurationSec > LOCK_MAX_SEC);
  const deliveryOutOfRange =
    draft.maxDeliveryCount !== 0 &&
    (draft.maxDeliveryCount < DELIVERY_MIN || draft.maxDeliveryCount > DELIVERY_MAX);

  const submit = async () => {
    if (
      submittedName == null ||
      submittedTopic == null ||
      lockOutOfRange ||
      deliveryOutOfRange ||
      busy
    ) {
      return;
    }
    setBusy(true);
    setError(null);
    try {
      await onSubmit({
        topic: submittedTopic,
        name: submittedName,
        lockDurationSec: draft.lockDurationSec,
        maxDeliveryCount: draft.maxDeliveryCount,
        ttlSec: draft.ttlSec,
        autoDeleteOnIdleSec: 0,
        deadLetterOnExpiry: draft.deadLetterOnExpiry,
        deadLetterOnRuleError: draft.deadLetterOnRuleError,
        requiresSession: draft.requiresSession,
        forwardTo: "",
        forwardDeadLettersTo: draft.forwardDeadLettersTo.trim(),
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
                ? "board.azure-servicebus.subscriptions.createTitle"
                : "board.azure-servicebus.subscriptions.editTitle",
              { name: editing?.name ?? "" },
            )}
          </DialogTitle>
        </DialogHeader>
        <FieldGroup>
          <Field>
            <FieldLabel htmlFor="sb-sub-topic">
              {t("board.azure-servicebus.subscriptions.topic")}
            </FieldLabel>
            <Input
              id="sb-sub-topic"
              value={draft.topic}
              disabled={editing != null}
              placeholder="events"
              onChange={(event) => set("topic", event.target.value)}
            />
            <FieldDescription>
              {t("board.azure-servicebus.subscriptions.topicHint")}
            </FieldDescription>
          </Field>

          <Field>
            <FieldLabel htmlFor="sb-sub-name">
              {t("board.azure-servicebus.subscriptions.name")}
            </FieldLabel>
            <Input
              id="sb-sub-name"
              value={draft.name}
              disabled={editing != null}
              placeholder="orders-worker"
              onChange={(event) => set("name", event.target.value)}
            />
            <FieldDescription>
              {nameIssue === "tooLong"
                ? t("board.azure-servicebus.subscriptions.nameProblem.tooLong")
                : nameIssue === "charset"
                  ? t("board.azure-servicebus.subscriptions.nameProblem.charset")
                  : nameIssue === "reserved"
                    ? t("board.azure-servicebus.subscriptions.nameProblem.reserved")
                    : nameIssue === "separator"
                      ? t("board.azure-servicebus.subscriptions.nameProblem.separator")
                      : nameIssue === "empty"
                        ? t("board.azure-servicebus.subscriptions.nameProblem.empty")
                        : t("board.azure-servicebus.subscriptions.nameHint")}
            </FieldDescription>
          </Field>

          <Field>
            <FieldLabel htmlFor="sb-sub-lock">
              {t("board.azure-servicebus.subscriptions.lockDuration")}
            </FieldLabel>
            <Input
              id="sb-sub-lock"
              type="number"
              value={String(draft.lockDurationSec)}
              onChange={(event) => set("lockDurationSec", numberField(event.target.value))}
            />
            <FieldDescription style={lockOutOfRange ? { color: "var(--c-danger)" } : undefined}>
              {lockOutOfRange
                ? t("board.azure-servicebus.entities.lockRange")
                : t("board.azure-servicebus.entities.lockHint")}
            </FieldDescription>
          </Field>

          <Field>
            <FieldLabel htmlFor="sb-sub-delivery">
              {t("board.azure-servicebus.subscriptions.maxDelivery")}
            </FieldLabel>
            <Input
              id="sb-sub-delivery"
              type="number"
              value={String(draft.maxDeliveryCount)}
              onChange={(event) => set("maxDeliveryCount", numberField(event.target.value))}
            />
            <FieldDescription style={deliveryOutOfRange ? { color: "var(--c-danger)" } : undefined}>
              {deliveryOutOfRange
                ? t("board.azure-servicebus.entities.maxDeliveryRange")
                : t("board.azure-servicebus.entities.maxDeliveryHint")}
            </FieldDescription>
          </Field>

          <Field>
            <FieldLabel htmlFor="sb-sub-ttl">
              {t("board.azure-servicebus.subscriptions.ttl")}
            </FieldLabel>
            <Input
              id="sb-sub-ttl"
              type="number"
              value={String(draft.ttlSec)}
              onChange={(event) => set("ttlSec", numberField(event.target.value))}
            />
            <FieldDescription>{t("board.azure-servicebus.entities.ttlHint")}</FieldDescription>
          </Field>

          <Field orientation="horizontal">
            <Switch
              id="sb-sub-dlq-expiry"
              checked={draft.deadLetterOnExpiry}
              onCheckedChange={(next) => set("deadLetterOnExpiry", next)}
            />
            <FieldLabel htmlFor="sb-sub-dlq-expiry">
              {t("board.azure-servicebus.entities.deadLetterOnExpiry")}
            </FieldLabel>
          </Field>

          <Field orientation="horizontal">
            <Switch
              id="sb-sub-dlq-rule"
              checked={draft.deadLetterOnRuleError}
              onCheckedChange={(next) => set("deadLetterOnRuleError", next)}
            />
            <FieldLabel htmlFor="sb-sub-dlq-rule">
              {t("board.azure-servicebus.subscriptions.deadLetterOnRuleError")}
            </FieldLabel>
          </Field>
          <FieldDescription>
            {t("board.azure-servicebus.subscriptions.deadLetterOnRuleErrorHint")}
          </FieldDescription>

          <Field>
            <FieldLabel htmlFor="sb-sub-forward-dl">
              {t("board.azure-servicebus.subscriptions.forwardDeadLettersTo")}
            </FieldLabel>
            <Input
              id="sb-sub-forward-dl"
              value={draft.forwardDeadLettersTo}
              placeholder="failed-events"
              onChange={(event) => set("forwardDeadLettersTo", event.target.value)}
            />
            <FieldDescription>
              {t("board.azure-servicebus.entities.forwardDeadLettersToHint")}
            </FieldDescription>
          </Field>

          {editing == null && (
            <>
              <Field orientation="horizontal">
                <Switch
                  id="sb-sub-sessions"
                  checked={draft.requiresSession}
                  onCheckedChange={(next) => set("requiresSession", next)}
                />
                <FieldLabel htmlFor="sb-sub-sessions">
                  {t("board.azure-servicebus.entities.sessions")}
                </FieldLabel>
              </Field>
              <FieldDescription>
                {t("board.azure-servicebus.subscriptions.sessionsHint")}
              </FieldDescription>
            </>
          )}

          {error != null && (
            <FieldDescription style={{ color: "var(--c-danger)" }}>{error}</FieldDescription>
          )}
        </FieldGroup>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t("common.cancel")}
          </Button>
          <Button
            disabled={
              submittedName == null ||
              submittedTopic == null ||
              lockOutOfRange ||
              deliveryOutOfRange ||
              busy
            }
            onClick={() => void submit()}
          >
            {busy && <Spinner />}
            {t(editing == null ? "common.create" : "common.save")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
